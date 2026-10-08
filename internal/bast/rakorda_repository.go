package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) InsertRakordaUpload(ctx context.Context, actor auth.Principal, item RakordaUpload, scope auth.RegencyScope, meta auth.ClientMeta) (RakordaUpload, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RakordaUpload{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM program_schedules ps WHERE ps.id=$1 AND ($2 OR ps.regency_id::text=ANY($3)))`, item.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&allowed); err != nil || !allowed {
		if err != nil {
			return RakordaUpload{}, err
		}
		return RakordaUpload{}, ErrNotFound
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO bast_rakorda_uploads(schedule_id,document_kind,event_date,original_name,mime_type,byte_size,storage_key,storage_backend,uploaded_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,'configured',$8)
		RETURNING id::text,schedule_id::text,document_kind,event_date::text,original_name,mime_type,byte_size,storage_key,created_at
	`, item.ScheduleID, item.DocumentKind, item.EventDate, item.OriginalName, item.MimeType, item.ByteSize, item.StorageKey, actor.UserID).Scan(
		&item.ID, &item.ScheduleID, &item.DocumentKind, &item.EventDate, &item.OriginalName, &item.MimeType, &item.ByteSize, &item.StorageKey, &item.CreatedAt,
	)
	if err != nil {
		return RakordaUpload{}, fmt.Errorf("insert %s upload: %w", item.DocumentKind, err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast." + string(item.DocumentKind) + "_uploaded", ResourceType: "bast_rakorda_upload", ResourceID: item.ID, Metadata: map[string]any{"schedule_id": item.ScheduleID, "event_date": item.EventDate, "document_kind": item.DocumentKind}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return RakordaUpload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RakordaUpload{}, err
	}
	return item, nil
}

func (r *Repository) ListRakordaUploads(ctx context.Context, kind RakordaKind, scheduleID, eventDate string, scope auth.RegencyScope) ([]RakordaUpload, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id::text,u.schedule_id::text,u.document_kind,u.event_date::text,u.original_name,u.mime_type,u.byte_size,u.storage_key,u.created_at
		FROM bast_rakorda_uploads u JOIN program_schedules ps ON ps.id=u.schedule_id
		WHERE u.schedule_id=$1 AND u.document_kind=$2 AND ($3='' OR u.event_date=$3::date) AND ($4 OR ps.regency_id::text=ANY($5))
		ORDER BY u.event_date DESC,u.created_at DESC
	`, scheduleID, kind, eventDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list %s uploads: %w", kind, err)
	}
	defer rows.Close()
	items := []RakordaUpload{}
	for rows.Next() {
		var item RakordaUpload
		if err := rows.Scan(&item.ID, &item.ScheduleID, &item.DocumentKind, &item.EventDate, &item.OriginalName, &item.MimeType, &item.ByteSize, &item.StorageKey, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetRakordaUpload(ctx context.Context, kind RakordaKind, id string, scope auth.RegencyScope) (RakordaUpload, error) {
	var item RakordaUpload
	err := r.pool.QueryRow(ctx, `
		SELECT u.id::text,u.schedule_id::text,u.document_kind,u.event_date::text,u.original_name,u.mime_type,u.byte_size,u.storage_key,u.created_at
		FROM bast_rakorda_uploads u JOIN program_schedules ps ON ps.id=u.schedule_id
		WHERE u.id=$1 AND u.document_kind=$2 AND ($3 OR ps.regency_id::text=ANY($4))
	`, id, kind, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ScheduleID, &item.DocumentKind, &item.EventDate, &item.OriginalName, &item.MimeType, &item.ByteSize, &item.StorageKey, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RakordaUpload{}, ErrNotFound
	}
	if err != nil {
		return RakordaUpload{}, fmt.Errorf("get %s upload: %w", kind, err)
	}
	return item, nil
}

func (r *Repository) DeleteRakordaUpload(ctx context.Context, actor auth.Principal, kind RakordaKind, id string, scope auth.RegencyScope, meta auth.ClientMeta) (RakordaUpload, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RakordaUpload{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item RakordaUpload
	err = tx.QueryRow(ctx, `
		DELETE FROM bast_rakorda_uploads u USING program_schedules ps
		WHERE u.id=$1 AND u.document_kind=$2 AND ps.id=u.schedule_id AND ($3 OR ps.regency_id::text=ANY($4))
		RETURNING u.id::text,u.schedule_id::text,u.document_kind,u.event_date::text,u.original_name,u.mime_type,u.byte_size,u.storage_key,u.created_at
	`, id, kind, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ScheduleID, &item.DocumentKind, &item.EventDate, &item.OriginalName, &item.MimeType, &item.ByteSize, &item.StorageKey, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RakordaUpload{}, ErrNotFound
	}
	if err != nil {
		return RakordaUpload{}, fmt.Errorf("delete %s upload: %w", kind, err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast." + string(kind) + "_upload_deleted", ResourceType: "bast_rakorda_upload", ResourceID: item.ID, Metadata: map[string]any{"schedule_id": item.ScheduleID, "document_kind": kind}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return RakordaUpload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RakordaUpload{}, err
	}
	return item, nil
}
