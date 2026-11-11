package bast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// ListDP3ExcelScheduleIDs returns the schedules whose Excel DP3 must be kept
// in sync: every schedule with a completed distribution, plus those that
// already have a workbook (so one whose last distribution was reopened is
// rebuilt with the remaining recipients).
func (r *Repository) ListDP3ExcelScheduleIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ps.id::text FROM program_schedules ps
		WHERE EXISTS (SELECT 1 FROM distribution_slots ds WHERE ds.schedule_id = ps.id AND ds.status = 'completed' AND ds.distributed_at IS NOT NULL)
		   OR EXISTS (SELECT 1 FROM dp3_excel_exports e WHERE e.schedule_id = ps.id)
		ORDER BY ps.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list DP3 excel schedules: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) GetDP3ExcelContext(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3ExcelContext, error) {
	var result DP3ExcelContext
	err := r.pool.QueryRow(ctx, `
		SELECT ps.id::text, p.name, p.program_type, p.fiscal_year, r.name, COALESCE(r.province_name,''), COALESCE(z.name,'')
		FROM program_schedules ps
		JOIN programs p ON p.id = ps.program_id
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN program_regency_assignments a ON a.program_id = p.id AND a.regency_id = r.id
		LEFT JOIN program_zones z ON z.id = a.zone_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.ScheduleID, &result.ProgramName, &result.ProgramType, &result.FiscalYear, &result.RegencyName, &result.ProvinceName, &result.ZoneName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DP3ExcelContext{}, ErrNotFound
	}
	if err != nil {
		return DP3ExcelContext{}, fmt.Errorf("get DP3 excel context: %w", err)
	}
	return result, nil
}

// ListDP3ExcelDetails returns, per completed slot of the schedule, the
// recipient data the DP3 PDF does not carry.
func (r *Repository) ListDP3ExcelDetails(ctx context.Context, scheduleID string) (map[string]DP3ExcelDetail, error) {
	rows, err := r.pool.Query(ctx, completedSlotsNumbered+`
		SELECT ds.id::text, n.local_date, COALESCE(psi.display_value,''), COALESCE(p.phone_number,''),
		       COALESCE(ds.machine_serial_number,''),
		       COALESCE(ds.verification_snapshot_json->'equipment'->>'converter_brand',''), COALESCE(ds.converter_serial_number,''),
		       COALESCE(ds.hose_serial_number,'')
		FROM numbered n
		JOIN distribution_slots ds ON ds.id = n.id
		JOIN people p ON p.id = ds.recipient_person_id
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN programs pr ON pr.id = ps.program_id
		LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id
			AND psi.identifier_type = CASE WHEN pr.program_type = 'fisherman' THEN 'kusuka' ELSE 'farmer_card' END
	`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list DP3 excel details: %w", err)
	}
	defer rows.Close()
	details := map[string]DP3ExcelDetail{}
	for rows.Next() {
		var slotID string
		var date time.Time
		var detail DP3ExcelDetail
		if err := rows.Scan(&slotID, &date, &detail.FarmerCardNumber, &detail.PhoneNumber, &detail.MachineSerial,
			&detail.ConverterBrand, &detail.ConverterSerial, &detail.HoseSerial); err != nil {
			return nil, err
		}
		detail.DistributionDate = date.Format("2006-01-02")
		details[slotID] = detail
	}
	return details, rows.Err()
}

func (r *Repository) GetDP3ExcelExport(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3ExcelExport, error) {
	var result DP3ExcelExport
	err := r.pool.QueryRow(ctx, `
		SELECT e.schedule_id::text, e.filename, e.storage_key, e.content_checksum, e.recipient_count, e.byte_size,
		       e.synced_at, e.last_error, e.last_attempt_at
		FROM dp3_excel_exports e
		JOIN program_schedules ps ON ps.id = e.schedule_id
		WHERE e.schedule_id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.ScheduleID, &result.Filename, &result.StorageKey, &result.ContentChecksum, &result.RecipientCount, &result.ByteSize,
		&result.SyncedAt, &result.LastError, &result.LastAttemptAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DP3ExcelExport{}, ErrNotFound
	}
	if err != nil {
		return DP3ExcelExport{}, fmt.Errorf("get DP3 excel export: %w", err)
	}
	return result, nil
}

// SaveDP3ExcelExport records a freshly uploaded workbook and returns the
// storage key of the one it replaces ("" when there was none).
func (r *Repository) SaveDP3ExcelExport(ctx context.Context, export DP3ExcelExport) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin DP3 excel save: %w", err)
	}
	defer tx.Rollback(ctx)
	var oldKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM dp3_excel_exports WHERE schedule_id = $1 FOR UPDATE`, export.ScheduleID).Scan(&oldKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lock DP3 excel export: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dp3_excel_exports (schedule_id, filename, storage_key, content_checksum, recipient_count, byte_size, synced_at, last_error, last_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, now(), '', now())
		ON CONFLICT (schedule_id) DO UPDATE SET filename = EXCLUDED.filename, storage_key = EXCLUDED.storage_key,
			content_checksum = EXCLUDED.content_checksum, recipient_count = EXCLUDED.recipient_count, byte_size = EXCLUDED.byte_size,
			synced_at = now(), last_error = '', last_attempt_at = now()
	`, export.ScheduleID, export.Filename, export.StorageKey, export.ContentChecksum, export.RecipientCount, export.ByteSize); err != nil {
		return "", fmt.Errorf("save DP3 excel export: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit DP3 excel save: %w", err)
	}
	return oldKey, nil
}

// MarkDP3ExcelChecked records a sync attempt that found nothing to upload.
func (r *Repository) MarkDP3ExcelChecked(ctx context.Context, scheduleID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE dp3_excel_exports SET last_error = '', last_attempt_at = now() WHERE schedule_id = $1`, scheduleID)
	if err != nil {
		return fmt.Errorf("mark DP3 excel checked: %w", err)
	}
	return nil
}

func (r *Repository) RecordDP3ExcelFailure(ctx context.Context, scheduleID, message string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO dp3_excel_exports (schedule_id, last_error, last_attempt_at) VALUES ($1, $2, now())
		ON CONFLICT (schedule_id) DO UPDATE SET last_error = EXCLUDED.last_error, last_attempt_at = now()
	`, scheduleID, message)
	if err != nil {
		return fmt.Errorf("record DP3 excel failure: %w", err)
	}
	return nil
}
