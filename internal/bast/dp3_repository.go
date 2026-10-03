package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetDP3Context(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3Context, error) {
	var result DP3Context
	var zoneName *string
	var placeholder *bool
	err := r.pool.QueryRow(ctx, `
		SELECT ps.id::text, ps.program_id::text, ps.regency_id::text, p.program_type, r.name,
			r.document_code, r.province_name,
			ps.start_date::text, p.fiscal_year, z.name, z.is_placeholder,
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
		&result.StartDate, &result.FiscalYear, &zoneName, &placeholder, &result.HasActiveLogo,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DP3Context{}, ErrNotFound
	}
	if err != nil {
		return DP3Context{}, fmt.Errorf("get DP3 context: %w", err)
	}
	if zoneName != nil {
		result.ZoneName = *zoneName
	}
	if placeholder != nil {
		result.ZonePlaceholder = *placeholder
	}
	return result, nil
}

func (r *Repository) ListDP3Recipients(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]DP3Recipient, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.full_name, COALESCE(p.nik,''), COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), r.name,
			pa.distribution_number, pa.package_snapshot_json, ds.verification_snapshot_json
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN people p ON p.id = cn.person_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN distribution_slots ds ON ds.allocation_id = pa.id AND ds.status='completed'
		WHERE pa.schedule_id=$1
		  AND pa.status IN ('ready','distributed')
		  AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY (pa.distribution_number IS NULL), pa.distribution_number ASC NULLS LAST, pa.created_at ASC, pa.id ASC
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list DP3 recipients: %w", err)
	}
	defer rows.Close()
	raw := []dp3RawRecipient{}
	for rows.Next() {
		var item dp3RawRecipient
		if err := rows.Scan(&item.FullName, &item.NIK, &item.Address, &item.Village, &item.District, &item.Regency, &item.DistributionNumber, &item.AllocationSnapshot, &item.VerificationSnapshot); err != nil {
			return nil, err
		}
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
