package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) GetSourceContext(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (SourceContext, error) {
	var result SourceContext
	var slotQuota *int
	var profileID, series, zoneName *string
	var placeholder *bool
	err := r.pool.QueryRow(ctx, `
		SELECT p.id::text,r.id::text,p.program_type,r.document_code,r.name,z.name,z.is_placeholder,
			s.distribution_number_padding,s.slot_quota,profile.id::text,profile.document_series
		FROM programs p
		JOIN regencies r ON r.id=$2
		JOIN LATERAL (
			SELECT distribution_number_padding,slot_quota FROM program_schedules
			WHERE program_id=p.id AND regency_id=r.id
			ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END,created_at DESC LIMIT 1
		) s ON true
		LEFT JOIN program_regency_assignments a ON a.program_id=p.id AND a.regency_id=r.id
		LEFT JOIN program_zones z ON z.id=a.zone_id
		LEFT JOIN LATERAL (
			SELECT id,document_series FROM program_document_profile_versions
			WHERE program_id=p.id AND status='published' ORDER BY version DESC LIMIT 1
		) profile ON true
		WHERE p.id=$1 AND ($3 OR r.id::text=ANY($4))
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ProgramID, &result.RegencyID, &result.ProgramType, &result.RegencyCode, &result.RegencyName, &zoneName, &placeholder, &result.Padding, &slotQuota, &profileID, &series)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceContext{}, ErrNotFound
	}
	if err != nil {
		return SourceContext{}, fmt.Errorf("get BA source context: %w", err)
	}
	if slotQuota != nil {
		result.SlotQuota = *slotQuota
	}
	if zoneName != nil {
		result.ZoneName = *zoneName
	}
	if placeholder != nil {
		result.ZonePlaceholder = *placeholder
	}
	if profileID != nil {
		result.ProfileVersionID = *profileID
	}
	if series != nil {
		result.DocumentSeries = *series
	}
	return result, nil
}

func (r *Repository) ListCompletedSlots(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]CompletedSlot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text,ds.slot_number,ds.distributed_at
		FROM distribution_slots ds JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ps.program_id=$1 AND ps.regency_id=$2 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		AND ($3 OR ps.regency_id::text=ANY($4)) ORDER BY ds.slot_number,ds.id
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list completed BA slots: %w", err)
	}
	defer rows.Close()
	items := []CompletedSlot{}
	for rows.Next() {
		var item CompletedSlot
		if err := rows.Scan(&item.ID, &item.SlotNumber, &item.DistributedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetLockedTotal(ctx context.Context, programID, regencyID string) (LockResult, error) {
	var result LockResult
	err := r.pool.QueryRow(ctx, `SELECT program_id::text,regency_id::text,final_total,locked_at FROM program_regency_bast_settings WHERE program_id=$1 AND regency_id=$2`, programID, regencyID).Scan(&result.ProgramID, &result.RegencyID, &result.FinalTotal, &result.LockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LockResult{}, ErrNotFound
	}
	if err != nil {
		return LockResult{}, fmt.Errorf("get locked BA total: %w", err)
	}
	return result, nil
}

func (r *Repository) LockRegencyTotal(ctx context.Context, actor auth.Principal, source SourceContext, meta auth.ClientMeta) (LockResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LockResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result LockResult
	err = tx.QueryRow(ctx, `
		INSERT INTO program_regency_bast_settings(program_id,regency_id,final_total,locked_by)
		VALUES($1,$2,$3,NULLIF($4,'')::uuid)
		ON CONFLICT(program_id,regency_id) DO UPDATE SET final_total=EXCLUDED.final_total,locked_at=now(),locked_by=EXCLUDED.locked_by,updated_at=now()
		WHERE program_regency_bast_settings.final_total=EXCLUDED.final_total OR NOT EXISTS(
			SELECT 1 FROM bast_individual_documents d WHERE d.program_id=EXCLUDED.program_id AND d.regency_id=EXCLUDED.regency_id AND d.status='final'
		)
		RETURNING program_id::text,regency_id::text,final_total,locked_at
	`, source.ProgramID, source.RegencyID, source.SlotQuota, actor.UserID).Scan(&result.ProgramID, &result.RegencyID, &result.FinalTotal, &result.LockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LockResult{}, ErrFinalTotalLocked
	}
	if err != nil {
		return LockResult{}, fmt.Errorf("lock BA total: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.total_locked", ResourceType: "program_regency_bast_setting", ResourceID: source.ProgramID + ":" + source.RegencyID, Metadata: map[string]any{"final_total": source.SlotQuota}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return LockResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LockResult{}, err
	}
	return result, nil
}

func (r *Repository) ListActiveBundles(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]DailyBundle, error) {
	rows, err := r.pool.Query(ctx, `SELECT b.id::text,b.program_id::text,b.regency_id::text,b.local_date::text,b.filename,b.recipient_count,b.page_count,b.version,b.status,b.synced_at FROM bast_daily_bundles b WHERE b.program_id=$1 AND b.regency_id=$2 AND b.status='active' AND ($3 OR b.regency_id::text=ANY($4)) ORDER BY b.local_date DESC`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list BA bundles: %w", err)
	}
	defer rows.Close()
	items := []DailyBundle{}
	for rows.Next() {
		var item DailyBundle
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.LocalDate, &item.Filename, &item.RecipientCount, &item.PageCount, &item.Version, &item.Status, &item.SyncedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
