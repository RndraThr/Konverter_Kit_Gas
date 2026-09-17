package activities

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

type regencyInfo struct {
	ID           string
	Name         string
	DocumentCode string
}

func (r *Repository) GetRegency(ctx context.Context, regencyID string, scope auth.RegencyScope) (regencyInfo, error) {
	var info regencyInfo
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, name, document_code FROM regencies
		WHERE id = $1 AND ($2 OR id::text = ANY($3))
	`, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(&info.ID, &info.Name, &info.DocumentCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return regencyInfo{}, ErrRegencyNotFound
	}
	if err != nil {
		return regencyInfo{}, fmt.Errorf("get regency: %w", err)
	}
	return info, nil
}

const activityMediaSelect = `
SELECT m.id::text, m.regency_id::text, r.name, r.document_code, m.activity_type, m.storage_key,
	m.display_name, m.original_filename, m.media_type, m.mime_type, m.byte_size, m.checksum,
	m.source, m.status, m.uploaded_by, m.uploaded_at, m.created_at, m.updated_at
FROM activity_media m
JOIN regencies r ON r.id = m.regency_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanActivityMedia(row rowScanner) (ActivityMedia, error) {
	var item ActivityMedia
	var uploadedBy *string
	if err := row.Scan(&item.ID, &item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode, &item.ActivityType,
		&item.StorageKey, &item.DisplayName, &item.OriginalFilename, &item.MediaType, &item.MimeType,
		&item.ByteSize, &item.Checksum, &item.Source, &item.Status, &uploadedBy, &item.UploadedAt,
		&item.CreatedAt, &item.UpdatedAt); err != nil {
		return ActivityMedia{}, fmt.Errorf("scan activity media: %w", err)
	}
	item.UploadedBy = uploadedBy
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}
	rows, err := r.pool.Query(ctx, activityMediaSelect+`
		WHERE m.status = 'active' AND m.regency_id = $1 AND m.activity_type = $2
		AND ($3 OR r.id::text = ANY($4))
		ORDER BY m.uploaded_at DESC
		LIMIT $5 OFFSET $6
	`, filter.RegencyID, filter.ActivityType, scope.Unrestricted, scope.RegencyIDs, pageSize, (page-1)*pageSize)
	if err != nil {
		return Page{}, fmt.Errorf("list activity media: %w", err)
	}
	defer rows.Close()

	items := make([]ActivityMedia, 0, pageSize)
	for rows.Next() {
		item, err := scanActivityMedia(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate activity media: %w", err)
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM activity_media m JOIN regencies r ON r.id = m.regency_id
		WHERE m.status = 'active' AND m.regency_id = $1 AND m.activity_type = $2
		AND ($3 OR r.id::text = ANY($4))
	`, filter.RegencyID, filter.ActivityType, scope.Unrestricted, scope.RegencyIDs).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count activity media: %w", err)
	}

	return Page{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

type insertInput struct {
	RegencyID        string
	ActivityType     string
	StorageKey       string
	DisplayName      string
	OriginalFilename string
	MediaType        string
	MimeType         string
	ByteSize         int64
	Checksum         string
	Source           string
}

func (r *Repository) Insert(ctx context.Context, actor auth.Principal, input insertInput, meta auth.ClientMeta) (ActivityMedia, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ActivityMedia{}, fmt.Errorf("begin insert activity media: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO activity_media (regency_id, activity_type, storage_key, display_name, original_filename,
			media_type, mime_type, byte_size, checksum, source, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id::text
	`, input.RegencyID, input.ActivityType, input.StorageKey, input.DisplayName, input.OriginalFilename,
		input.MediaType, input.MimeType, input.ByteSize, input.Checksum, input.Source, actor.UserID).Scan(&id); err != nil {
		return ActivityMedia{}, fmt.Errorf("insert activity media: %w", err)
	}
	if err := recordActivityMediaAudit(ctx, tx, actor, meta, "uploaded", id, map[string]any{
		"regency_id": input.RegencyID, "activity_type": input.ActivityType,
		"mime_type": input.MimeType, "byte_size": input.ByteSize, "source": input.Source,
	}); err != nil {
		return ActivityMedia{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ActivityMedia{}, fmt.Errorf("commit insert activity media: %w", err)
	}
	return r.GetByID(ctx, id, auth.RegencyScope{Unrestricted: true})
}

func (r *Repository) GetByID(ctx context.Context, id string, scope auth.RegencyScope) (ActivityMedia, error) {
	row := r.pool.QueryRow(ctx, activityMediaSelect+`
		WHERE m.id = $1 AND m.status = 'active' AND ($2 OR r.id::text = ANY($3))
	`, id, scope.Unrestricted, scope.RegencyIDs)
	item, err := scanActivityMedia(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityMedia{}, ErrNotFound
	}
	if err != nil {
		return ActivityMedia{}, err
	}
	return item, nil
}

func (r *Repository) SoftDelete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin delete activity media: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storageKey, activityType, regencyID string
	err = tx.QueryRow(ctx, `
		SELECT m.storage_key, m.activity_type, m.regency_id::text
		FROM activity_media m
		JOIN regencies r ON r.id = m.regency_id
		WHERE m.id = $1 AND m.status = 'active' AND ($2 OR r.id::text = ANY($3))
		FOR UPDATE OF m
	`, id, scope.Unrestricted, scope.RegencyIDs).Scan(&storageKey, &activityType, &regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock activity media: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_media SET status='deleted', updated_at=now() WHERE id=$1`, id); err != nil {
		return "", fmt.Errorf("delete activity media: %w", err)
	}
	if err := recordActivityMediaAudit(ctx, tx, actor, meta, "deleted", id, map[string]any{
		"activity_type": activityType, "regency_id": regencyID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit delete activity media: %w", err)
	}
	return storageKey, nil
}

// restoreAfterFailedStorageDelete undoes a soft delete when the storage
// backend delete that must follow it fails, mirroring
// internal/distribution's mediaRepository.RestoreMedia. Internal-use only —
// not exposed via any API route.
func (r *Repository) restoreAfterFailedStorageDelete(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, `UPDATE activity_media SET status='active', updated_at=now() WHERE id=$1`, id); err != nil {
		return fmt.Errorf("restore activity media: %w", err)
	}
	return nil
}

func recordActivityMediaAudit(ctx context.Context, tx pgx.Tx, actor auth.Principal, meta auth.ClientMeta, action, resourceID string, metadata map[string]any) error {
	return audit.Record(ctx, tx, audit.Event{
		ActorUserID: actor.UserID, Action: "activity_media." + action, ResourceType: "activity_media",
		ResourceID: resourceID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent,
	})
}
