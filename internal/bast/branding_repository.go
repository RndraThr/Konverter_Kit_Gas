package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// GetProgramCode returns the program code, used both to validate program
// existence and to build the storage folder path. ErrNotFound when missing.
func (r *Repository) GetProgramCode(ctx context.Context, programID string) (string, error) {
	var code string
	err := r.pool.QueryRow(ctx, `SELECT code FROM programs WHERE id=$1`, programID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return code, err
}

// ListLogos returns all branding logos for a program ordered by sort_order.
func (r *Repository) ListLogos(ctx context.Context, programID string) ([]LogoAsset, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,program_id::text,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm::float8,max_height_mm::float8,is_visible FROM program_ba_logo_assets WHERE program_id=$1 ORDER BY sort_order,id`, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []LogoAsset
	for rows.Next() {
		var item LogoAsset
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.SlotCode, &item.StorageKey, &item.OriginalFilename, &item.MimeType, &item.ByteSize, &item.Checksum, &item.SortOrder, &item.MaxWidthMM, &item.MaxHeightMM, &item.IsVisible); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SaveLogo upserts a logo by (program_id, slot_code). It returns the stored
// asset and the previous storage key (if any) so the caller can delete the
// superseded blob.
func (r *Repository) SaveLogo(ctx context.Context, actor auth.Principal, logo LogoAsset, meta auth.ClientMeta) (LogoAsset, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LogoAsset{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var oldKey string
	_ = tx.QueryRow(ctx, `SELECT storage_key FROM program_ba_logo_assets WHERE program_id=$1 AND slot_code=$2`, logo.ProgramID, logo.SlotCode).Scan(&oldKey)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO program_ba_logo_assets(program_id,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm,max_height_mm,is_visible) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true) ON CONFLICT(program_id,slot_code) DO UPDATE SET storage_key=excluded.storage_key,original_filename=excluded.original_filename,mime_type=excluded.mime_type,byte_size=excluded.byte_size,checksum=excluded.checksum,sort_order=excluded.sort_order,max_width_mm=excluded.max_width_mm,max_height_mm=excluded.max_height_mm,is_visible=true,updated_at=now() RETURNING id::text`, logo.ProgramID, logo.SlotCode, logo.StorageKey, logo.OriginalFilename, logo.MimeType, logo.ByteSize, logo.Checksum, logo.SortOrder, logo.MaxWidthMM, logo.MaxHeightMM).Scan(&id)
	if err != nil {
		return LogoAsset{}, "", fmt.Errorf("save ba logo: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.branding_logo_saved", ResourceType: "program_ba_logo_asset", ResourceID: id, Metadata: map[string]any{"program_id": logo.ProgramID, "slot_code": logo.SlotCode}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return LogoAsset{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return LogoAsset{}, "", err
	}
	item, err := r.GetLogo(ctx, logo.ProgramID, id)
	return item, oldKey, err
}

// UpdateLogo changes ordering, size bounds, and visibility of a logo.
func (r *Repository) UpdateLogo(ctx context.Context, actor auth.Principal, input LogoPatchInput, meta auth.ClientMeta) (LogoAsset, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LogoAsset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `UPDATE program_ba_logo_assets SET sort_order=$3,max_width_mm=$4,max_height_mm=$5,is_visible=$6,updated_at=now() WHERE id=$1 AND program_id=$2 RETURNING id::text`, input.ID, input.ProgramID, input.SortOrder, input.MaxWidthMM, input.MaxHeightMM, input.IsVisible).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return LogoAsset{}, ErrNotFound
	}
	if err != nil {
		return LogoAsset{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.branding_logo_updated", ResourceType: "program_ba_logo_asset", ResourceID: id, Metadata: map[string]any{"program_id": input.ProgramID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return LogoAsset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LogoAsset{}, err
	}
	return r.GetLogo(ctx, input.ProgramID, id)
}

// GetLogo returns one logo scoped to its program. ErrNotFound when missing.
func (r *Repository) GetLogo(ctx context.Context, programID, logoID string) (LogoAsset, error) {
	var item LogoAsset
	err := r.pool.QueryRow(ctx, `SELECT id::text,program_id::text,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm::float8,max_height_mm::float8,is_visible FROM program_ba_logo_assets WHERE id=$1 AND program_id=$2`, logoID, programID).Scan(&item.ID, &item.ProgramID, &item.SlotCode, &item.StorageKey, &item.OriginalFilename, &item.MimeType, &item.ByteSize, &item.Checksum, &item.SortOrder, &item.MaxWidthMM, &item.MaxHeightMM, &item.IsVisible)
	if errors.Is(err, pgx.ErrNoRows) {
		return LogoAsset{}, ErrNotFound
	}
	return item, err
}
