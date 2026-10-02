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
}

func (r *Repository) ListDailyRecapDates(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]dailyRecapDateRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date::text AS local_date, count(*)
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
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
		if err := rows.Scan(&item.LocalDate, &item.RecipientCount); err != nil {
			return nil, err
		}
		dates = append(dates, item)
	}
	return dates, rows.Err()
}

func (r *Repository) ListDailyRecapRecipients(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]DailyRecapRecipient, error) {
	localDate = strings.TrimSpace(localDate)
	rows, err := r.pool.Query(ctx, `
		SELECT ds.slot_number, p.full_name, COALESCE(psi.display_value,''), COALESCE(ds.machine_serial_number,''), ds.verification_snapshot_json
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id AND psi.identifier_type='farmer_card'
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		  AND (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date = $2::date
		  AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY ds.slot_number, ds.id
	`, scheduleID, localDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list daily recap recipients: %w", err)
	}
	defer rows.Close()
	raw := []dailyRecapRawRecipient{}
	for rows.Next() {
		var item dailyRecapRawRecipient
		if err := rows.Scan(&item.SlotNumber, &item.FullName, &item.FarmerCardNumber, &item.MachineSerial, &item.VerificationSnapshot); err != nil {
			return nil, err
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return decodeDailyRecapRecipients(raw)
}
