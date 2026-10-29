package bast

import (
	"context"
	"fmt"
	"strings"

	"konkit/internal/auth"
)

type dailyRecapDateRow struct {
	LocalDate      string
	RecipientCount int
	// HandoverLocations: titik serah penerima tanggal itu, digabung koma.
	HandoverLocations string
}

func (r *Repository) ListDailyRecapDates(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]dailyRecapDateRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date)::text AS local_date, count(*),
		       COALESCE(string_agg(DISTINCT hl.name, ', ' ORDER BY hl.name), '')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN bast_handover_locations hl ON hl.id = ds.handover_location_id
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		  AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY local_date
		ORDER BY local_date DESC
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list daily recap dates: %w", err)
	}
	defer rows.Close()
	dates := []dailyRecapDateRow{}
	for rows.Next() {
		var item dailyRecapDateRow
		if err := rows.Scan(&item.LocalDate, &item.RecipientCount, &item.HandoverLocations); err != nil {
			return nil, err
		}
		dates = append(dates, item)
	}
	return dates, rows.Err()
}

// completedSlotsNumbered memberi setiap distribusi selesai satu jadwal nomor
// urut berjalan lintas hari: urut per Tanggal distribusi yang diisi petugas
// (bukan saat Selesaikan diproses), lalu nomor bagi. Hari kedua melanjutkan nomor terakhir hari pertama. Dipakai bersama
// Rekap Harian dan DP3 agar nomor keduanya selalu sama.
const completedSlotsNumbered = `
	WITH numbered AS (
		SELECT ds.id, COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date) AS local_date,
		       row_number() OVER (ORDER BY COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date), ds.slot_number, ds.id) AS row_number
		FROM distribution_slots ds
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
	)`

func (r *Repository) ListDailyRecapRecipients(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]DailyRecapRecipient, error) {
	localDate = strings.TrimSpace(localDate)
	rows, err := r.pool.Query(ctx, completedSlotsNumbered+`
		SELECT n.row_number, ds.slot_number, p.full_name, COALESCE(psi.display_value,''), COALESCE(ds.machine_serial_number,''),
		       COALESCE(ds.machine_option_code,''), ds.verification_snapshot_json, pt.values_json, COALESCE(hl.name,'')
		FROM numbered n
		JOIN distribution_slots ds ON ds.id = n.id
		LEFT JOIN bast_handover_locations hl ON hl.id = ds.handover_location_id
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id AND psi.identifier_type='farmer_card'
		WHERE n.local_date = $2::date
		  AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY n.row_number
	`, scheduleID, localDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list daily recap recipients: %w", err)
	}
	defer rows.Close()
	raw := []dailyRecapRawRecipient{}
	for rows.Next() {
		var item dailyRecapRawRecipient
		if err := rows.Scan(&item.RowNumber, &item.SlotNumber, &item.FullName, &item.FarmerCardNumber, &item.MachineSerial, &item.MachineOptionCode, &item.VerificationSnapshot, &item.PackageTemplate, &item.HandoverLocation); err != nil {
			return nil, err
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return decodeDailyRecapRecipients(raw)
}
