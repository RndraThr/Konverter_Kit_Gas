package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

const aggregateColumns = `id::text,schedule_id::text,program_id::text,regency_id::text,document_type,document_date::text,filename,recipient_count,page_count,version,status,checksum,storage_key,COALESCE(last_error,''),finalized_at`

const aggregateColumnsD = `d.id::text,d.schedule_id::text,d.program_id::text,d.regency_id::text,d.document_type,d.document_date::text,d.filename,d.recipient_count,d.page_count,d.version,d.status,d.checksum,d.storage_key,COALESCE(d.last_error,''),d.finalized_at`

func scanAggregate(row pgx.Row) (AggregateDocument, error) {
	var doc AggregateDocument
	err := row.Scan(&doc.ID, &doc.ScheduleID, &doc.ProgramID, &doc.RegencyID, &doc.DocumentType, &doc.DocumentDate, &doc.Filename, &doc.RecipientCount, &doc.PageCount, &doc.Version, &doc.Status, &doc.Checksum, &doc.StorageKey, &doc.LastError, &doc.FinalizedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AggregateDocument{}, ErrNotFound
	}
	return doc, err
}

func (r *Repository) GetActiveAggregate(ctx context.Context, scheduleID, documentType, documentDate string, scope auth.RegencyScope) (AggregateDocument, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+aggregateColumnsD+` FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.schedule_id=$1 AND d.document_type=$2 AND d.document_date=$3 AND d.status='active' AND ($4 OR ps.regency_id::text=ANY($5))`, scheduleID, documentType, documentDate, scope.Unrestricted, scope.RegencyIDs)
	doc, err := scanAggregate(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AggregateDocument{}, ErrNotFound
		}
		return AggregateDocument{}, fmt.Errorf("get active aggregate: %w", err)
	}
	return doc, nil
}

func (r *Repository) GetAggregateByID(ctx context.Context, id string, scope auth.RegencyScope) (AggregateDocument, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+aggregateColumnsD+` FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.id=$1 AND ($2 OR ps.regency_id::text=ANY($3))`, id, scope.Unrestricted, scope.RegencyIDs)
	doc, err := scanAggregate(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AggregateDocument{}, ErrNotFound
		}
		return AggregateDocument{}, fmt.Errorf("get aggregate: %w", err)
	}
	return doc, nil
}

func (r *Repository) ListAggregates(ctx context.Context, scheduleID, documentType, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+aggregateColumnsD+` FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.schedule_id=$1 AND d.document_type=$2 AND d.document_date=$3 AND ($4 OR ps.regency_id::text=ANY($5)) ORDER BY d.version DESC`, scheduleID, documentType, documentDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list aggregates: %w", err)
	}
	defer rows.Close()
	items := []AggregateDocument{}
	for rows.Next() {
		var doc AggregateDocument
		if err := rows.Scan(&doc.ID, &doc.ScheduleID, &doc.ProgramID, &doc.RegencyID, &doc.DocumentType, &doc.DocumentDate, &doc.Filename, &doc.RecipientCount, &doc.PageCount, &doc.Version, &doc.Status, &doc.Checksum, &doc.StorageKey, &doc.LastError, &doc.FinalizedAt); err != nil {
			return nil, err
		}
		items = append(items, doc)
	}
	return items, rows.Err()
}

func (r *Repository) ListActiveAggregatesForType(ctx context.Context, scheduleID, documentType string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+aggregateColumnsD+` FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.schedule_id=$1 AND d.document_type=$2 AND d.status='active' AND ($3 OR ps.regency_id::text=ANY($4)) ORDER BY d.document_date DESC`, scheduleID, documentType, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list active aggregates: %w", err)
	}
	defer rows.Close()
	items := []AggregateDocument{}
	for rows.Next() {
		var doc AggregateDocument
		if err := rows.Scan(&doc.ID, &doc.ScheduleID, &doc.ProgramID, &doc.RegencyID, &doc.DocumentType, &doc.DocumentDate, &doc.Filename, &doc.RecipientCount, &doc.PageCount, &doc.Version, &doc.Status, &doc.Checksum, &doc.StorageKey, &doc.LastError, &doc.FinalizedAt); err != nil {
			return nil, err
		}
		items = append(items, doc)
	}
	return items, rows.Err()
}

func (r *Repository) NextAggregateVersion(ctx context.Context, scheduleID, documentType, documentDate string) (int, error) {
	var version int
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_type=$2 AND document_date=$3`, scheduleID, documentType, documentDate).Scan(&version); err != nil {
		return 0, fmt.Errorf("next aggregate version: %w", err)
	}
	return version, nil
}

func (r *Repository) ActivateAggregate(ctx context.Context, actor auth.Principal, input AggregateActivation, meta auth.ClientMeta) (AggregateActivationResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AggregateActivationResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := input.ScheduleID + ":" + input.DocumentType + ":" + input.DocumentDate
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return AggregateActivationResult{}, err
	}

	var current AggregateDocument
	err = tx.QueryRow(ctx, `SELECT `+aggregateColumns+` FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_type=$2 AND document_date=$3 AND status='active' FOR UPDATE`, input.ScheduleID, input.DocumentType, input.DocumentDate).Scan(&current.ID, &current.ScheduleID, &current.ProgramID, &current.RegencyID, &current.DocumentType, &current.DocumentDate, &current.Filename, &current.RecipientCount, &current.PageCount, &current.Version, &current.Status, &current.Checksum, &current.StorageKey, &current.LastError, &current.FinalizedAt)
	currentExists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AggregateActivationResult{}, err
	}
	if currentExists && current.Checksum == input.Checksum {
		if err := tx.Commit(ctx); err != nil {
			return AggregateActivationResult{}, err
		}
		return AggregateActivationResult{Document: current, Unchanged: true}, nil
	}
	if (currentExists && current.ID != input.ExpectedActiveID) || (!currentExists && input.ExpectedActiveID != "") {
		return AggregateActivationResult{}, ErrAggregateConflict
	}

	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_type=$2 AND document_date=$3`, input.ScheduleID, input.DocumentType, input.DocumentDate).Scan(&version); err != nil {
		return AggregateActivationResult{}, err
	}
	if version != input.ExpectedVersion {
		return AggregateActivationResult{}, ErrAggregateConflict
	}
	if currentExists {
		if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='superseded',updated_at=now() WHERE id=$1`, current.ID); err != nil {
			return AggregateActivationResult{}, err
		}
	}

	var doc AggregateDocument
	err = tx.QueryRow(ctx, `INSERT INTO bast_aggregate_documents(schedule_id,program_id,regency_id,document_type,document_date,filename,recipient_count,page_count,version,status,checksum,storage_key,snapshot_json,finalized_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'active',$10,$11,$12::jsonb,NULLIF($13,'')::uuid) RETURNING `+aggregateColumns,
		input.ScheduleID, input.ProgramID, input.RegencyID, input.DocumentType, input.DocumentDate, input.Filename, input.RecipientCount, input.PageCount, version, input.Checksum, input.StorageKey, string(input.Snapshot), actor.UserID).Scan(&doc.ID, &doc.ScheduleID, &doc.ProgramID, &doc.RegencyID, &doc.DocumentType, &doc.DocumentDate, &doc.Filename, &doc.RecipientCount, &doc.PageCount, &doc.Version, &doc.Status, &doc.Checksum, &doc.StorageKey, &doc.LastError, &doc.FinalizedAt)
	if err != nil {
		return AggregateActivationResult{}, fmt.Errorf("insert aggregate document: %w", err)
	}
	action := "bast." + input.DocumentType + "_finalized"
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: action, ResourceType: "bast_aggregate_document", ResourceID: doc.ID, Metadata: map[string]any{"schedule_id": input.ScheduleID, "document_date": input.DocumentDate, "document_type": input.DocumentType, "recipient_count": input.RecipientCount, "version": version, "checksum": input.Checksum}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return AggregateActivationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AggregateActivationResult{}, err
	}
	return AggregateActivationResult{Document: doc, OldStorageKey: current.StorageKey}, nil
}

func (r *Repository) RecordAggregateCleanupFailure(ctx context.Context, id, message string) error {
	_, err := r.pool.Exec(ctx, `UPDATE bast_aggregate_documents SET last_error=$2,updated_at=now() WHERE id=$1 AND status='active'`, id, message)
	return err
}
