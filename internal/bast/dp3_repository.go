package bast

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetDP3Context(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3Context, error) {
	var result DP3Context
	var zoneID, zoneName *string
	var placeholder *bool
	err := r.pool.QueryRow(ctx, `
		SELECT ps.id::text, ps.program_id::text, ps.regency_id::text, p.program_type, r.name,
			r.document_code, r.province_name,
			ps.start_date::text, p.fiscal_year, z.id::text, z.name, z.is_placeholder,
			EXISTS(SELECT 1 FROM program_ba_logo_assets l WHERE l.program_id=p.id AND l.is_visible=true)
		FROM program_schedules ps
		JOIN programs p ON p.id=ps.program_id
		JOIN regencies r ON r.id=ps.regency_id
		LEFT JOIN program_regency_assignments a ON a.program_id=p.id AND a.regency_id=r.id
		LEFT JOIN program_zones z ON z.id=a.zone_id
		WHERE ps.id=$1 AND ($2 OR ps.regency_id::text=ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.ScheduleID, &result.ProgramID, &result.RegencyID, &result.ProgramType, &result.RegencyName,
		&result.RegencyCode, &result.ProvinceName,
		&result.StartDate, &result.FiscalYear, &zoneID, &zoneName, &placeholder, &result.HasActiveLogo,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DP3Context{}, ErrNotFound
	}
	if err != nil {
		return DP3Context{}, fmt.Errorf("get DP3 context: %w", err)
	}
	if zoneID != nil {
		result.ZoneID = *zoneID
	}
	if zoneName != nil {
		result.ZoneName = *zoneName
	}
	if placeholder != nil {
		result.ZonePlaceholder = *placeholder
	}
	return result, nil
}

// ListDP3Recipients mengembalikan penerima yang distribusinya selesai pada
// localDate (zona Jakarta), sama dengan Rekap Harian, bernomor urut berjalan
// lintas hari. localDate kosong berarti semua hari.
func (r *Repository) ListDP3Recipients(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]DP3Recipient, error) {
	rows, err := r.pool.Query(ctx, completedSlotsNumbered+`
		SELECT n.row_number, ds.id::text, ds.slot_number, p.full_name, COALESCE(p.nik,''), COALESCE(p.address,''),
		       COALESCE(p.village,''), COALESCE(p.district,''), r.name,
		       pa.package_snapshot_json, ds.verification_snapshot_json, COALESCE(ds.machine_option_code,''), pt.values_json
		FROM numbered n
		JOIN distribution_slots ds ON ds.id = n.id
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN regencies r ON r.id = ps.regency_id
		JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN package_allocations pa ON pa.id = ds.allocation_id
		WHERE ($2 = '' OR n.local_date::text = $2)
		  AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY n.row_number
	`, scheduleID, strings.TrimSpace(localDate), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list DP3 recipients: %w", err)
	}
	defer rows.Close()
	raw := []dp3RawRecipient{}
	for rows.Next() {
		var item dp3RawRecipient
		var slotNumber int
		if err := rows.Scan(&item.RowNumber, &item.SlotID, &slotNumber, &item.FullName, &item.NIK, &item.Address, &item.Village, &item.District, &item.Regency,
			&item.AllocationSnapshot, &item.VerificationSnapshot, &item.MachineOptionCode, &item.PackageTemplate); err != nil {
			return nil, err
		}
		item.DistributionNumber = &slotNumber
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return resolveDP3Recipients(raw)
}

func (r *Repository) ListActiveLogos(ctx context.Context, programID string) ([]LogoSnapshot, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,storage_key,mime_type,sort_order,max_width_mm::float8,max_height_mm::float8 FROM program_ba_logo_assets WHERE program_id=$1 AND is_visible=true ORDER BY sort_order,id`, programID)
	if err != nil {
		return nil, fmt.Errorf("list active BA logos: %w", err)
	}
	defer rows.Close()
	logos := []LogoSnapshot{}
	for rows.Next() {
		var logo LogoSnapshot
		if err := rows.Scan(&logo.AssetID, &logo.StorageKey, &logo.MimeType, &logo.SortOrder, &logo.MaxWidthMM, &logo.MaxHeightMM); err != nil {
			return nil, err
		}
		logos = append(logos, logo)
	}
	return logos, rows.Err()
}
