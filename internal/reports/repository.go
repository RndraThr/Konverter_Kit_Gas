package reports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// ListDriveScheduleIDs returns the schedules whose Drive workbook must be
// kept in sync: every schedule with a completed distribution, plus those that
// already have a workbook (so one whose last distribution was reopened is
// rebuilt with the remaining recipients).
func (r *Repository) ListDriveScheduleIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ps.id::text FROM program_schedules ps
		WHERE EXISTS (SELECT 1 FROM distribution_slots ds WHERE ds.schedule_id = ps.id AND ds.status = 'completed' AND ds.distributed_at IS NOT NULL)
		   OR EXISTS (SELECT 1 FROM dp3_excel_exports e WHERE e.schedule_id = ps.id)
		ORDER BY ps.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list laporan DP3 schedules: %w", err)
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

func (r *Repository) GetScheduleInfo(ctx context.Context, scheduleID string, scope auth.RegencyScope) (ScheduleInfo, error) {
	var result ScheduleInfo
	err := r.pool.QueryRow(ctx, `
		SELECT ps.id::text, p.name, p.program_type, p.fiscal_year, r.name, COALESCE(r.province_name,''), COALESCE(z.name,'')
		FROM program_schedules ps
		JOIN programs p ON p.id = ps.program_id
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN program_regency_assignments a ON a.program_id = p.id AND a.regency_id = r.id
		LEFT JOIN program_zones z ON z.id = a.zone_id
		WHERE ps.id::text = $1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.ScheduleID, &result.ProgramName, &result.ProgramType, &result.FiscalYear, &result.RegencyName, &result.ProvinceName, &result.ZoneName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScheduleInfo{}, ErrScheduleNotFound
	}
	if err != nil {
		return ScheduleInfo{}, fmt.Errorf("get laporan DP3 schedule: %w", err)
	}
	return result, nil
}

// ListSlotDetails returns, per completed slot of the schedule, the recipient
// data the BA DP3 recipient list does not carry.
func (r *Repository) ListSlotDetails(ctx context.Context, scheduleID string) (map[string]SlotDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date),
		       COALESCE(psi.display_value,''), COALESCE(p.phone_number,''), COALESCE(ds.machine_serial_number,''),
		       COALESCE(ds.verification_snapshot_json->'equipment'->>'converter_brand',''), COALESCE(ds.converter_serial_number,''),
		       COALESCE(ds.hose_serial_number,'')
		FROM distribution_slots ds
		JOIN people p ON p.id = ds.recipient_person_id
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN programs pr ON pr.id = ps.program_id
		LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id
			AND psi.identifier_type = CASE WHEN pr.program_type = 'fisherman' THEN 'kusuka' ELSE 'farmer_card' END
		WHERE ds.schedule_id = $1 AND ds.status = 'completed' AND ds.distributed_at IS NOT NULL
	`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list laporan DP3 details: %w", err)
	}
	defer rows.Close()
	details := map[string]SlotDetail{}
	for rows.Next() {
		var slotID string
		var date time.Time
		var detail SlotDetail
		if err := rows.Scan(&slotID, &date, &detail.CardNumber, &detail.PhoneNumber, &detail.MachineSerial,
			&detail.ConverterBrand, &detail.ConverterSerial, &detail.HoseSerial); err != nil {
			return nil, err
		}
		detail.DistributionDate = date.Format("2006-01-02")
		details[slotID] = detail
	}
	return details, rows.Err()
}

func (r *Repository) GetDriveExport(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DriveExport, error) {
	var result DriveExport
	err := r.pool.QueryRow(ctx, `
		SELECT e.schedule_id::text, e.filename, e.storage_key, e.content_checksum, e.recipient_count, e.byte_size,
		       e.synced_at, e.last_error, e.last_attempt_at
		FROM dp3_excel_exports e
		JOIN program_schedules ps ON ps.id = e.schedule_id
		WHERE e.schedule_id::text = $1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.ScheduleID, &result.Filename, &result.StorageKey, &result.ContentChecksum, &result.RecipientCount, &result.ByteSize,
		&result.SyncedAt, &result.LastError, &result.LastAttemptAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DriveExport{}, ErrScheduleNotFound
	}
	if err != nil {
		return DriveExport{}, fmt.Errorf("get laporan DP3 drive export: %w", err)
	}
	return result, nil
}

// SaveDriveExport records a freshly uploaded workbook and returns the storage
// key of the one it replaces ("" when there was none).
func (r *Repository) SaveDriveExport(ctx context.Context, export DriveExport) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin laporan DP3 save: %w", err)
	}
	defer tx.Rollback(ctx)
	var oldKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM dp3_excel_exports WHERE schedule_id = $1 FOR UPDATE`, export.ScheduleID).Scan(&oldKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lock laporan DP3 export: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dp3_excel_exports (schedule_id, filename, storage_key, content_checksum, recipient_count, byte_size, synced_at, last_error, last_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, now(), '', now())
		ON CONFLICT (schedule_id) DO UPDATE SET filename = EXCLUDED.filename, storage_key = EXCLUDED.storage_key,
			content_checksum = EXCLUDED.content_checksum, recipient_count = EXCLUDED.recipient_count, byte_size = EXCLUDED.byte_size,
			synced_at = now(), last_error = '', last_attempt_at = now()
	`, export.ScheduleID, export.Filename, export.StorageKey, export.ContentChecksum, export.RecipientCount, export.ByteSize); err != nil {
		return "", fmt.Errorf("save laporan DP3 export: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit laporan DP3 save: %w", err)
	}
	return oldKey, nil
}

// MarkDriveChecked records a sync attempt that found nothing to upload.
func (r *Repository) MarkDriveChecked(ctx context.Context, scheduleID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE dp3_excel_exports SET last_error = '', last_attempt_at = now() WHERE schedule_id = $1`, scheduleID)
	if err != nil {
		return fmt.Errorf("mark laporan DP3 checked: %w", err)
	}
	return nil
}

func (r *Repository) RecordDriveFailure(ctx context.Context, scheduleID, message string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO dp3_excel_exports (schedule_id, last_error, last_attempt_at) VALUES ($1, $2, now())
		ON CONFLICT (schedule_id) DO UPDATE SET last_error = EXCLUDED.last_error, last_attempt_at = now()
	`, scheduleID, message)
	if err != nil {
		return fmt.Errorf("record laporan DP3 failure: %w", err)
	}
	return nil
}

func (r *Repository) RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, meta auth.ClientMeta) error {
	return audit.Record(ctx, r.pool, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "reports.exported",
		ResourceType: "program_schedule",
		ResourceID:   scheduleID,
		Metadata:     map[string]any{"format": format, "report": "dp3"},
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	})
}

// CountAllocations counts every allocation (recipient) of the schedule.
func (r *Repository) CountAllocations(ctx context.Context, scheduleID string) (int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id::text = $1`, scheduleID).Scan(&total); err != nil {
		return 0, fmt.Errorf("count schedule allocations: %w", err)
	}
	return total, nil
}
