package bast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// WetScanMeta menjelaskan pindaian halaman tanda tangan yang tersimpan.
type WetScanMeta struct {
	ContentCode string    `json:"content_code"`
	ByteSize    int       `json:"byte_size"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// ResolveWetSignature mengembalikan apakah mode tanda tangan basah berlaku
// untuk BA ini (global jadwal atau khusus BA) dan pindaian yang tersimpan.
func (r *Repository) ResolveWetSignature(ctx context.Context, scheduleID, documentKey, date string) (bool, []byte, string, error) {
	effective, err := r.effectiveSignatureRules(ctx, scheduleID, wetBaseKey(documentKey))
	if err != nil || !effective.wet {
		return false, nil, "", err
	}
	var image []byte
	var code string
	err = r.pool.QueryRow(ctx, `SELECT image_png, content_code FROM bast_wet_signature_scans WHERE schedule_id::text=$1 AND document_key=$2 AND document_date=$3::date`, scheduleID, documentKey, date).Scan(&image, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil, "", nil
	}
	if err != nil {
		return false, nil, "", fmt.Errorf("resolve wet signature: %w", err)
	}
	return true, image, code, nil
}

// WetSignatureEnabled mengembalikan apakah mode tanda tangan basah berlaku
// untuk BA jadwal ini, dengan memeriksa cakupan kabupaten pengguna.
func (r *Repository) WetSignatureEnabled(ctx context.Context, scheduleID, documentKey string, scope auth.RegencyScope) (bool, error) {
	if _, err := r.GetScheduleSettings(ctx, scheduleID, scope); err != nil {
		return false, err
	}
	effective, err := r.effectiveSignatureRules(ctx, scheduleID, documentKey)
	if err != nil {
		return false, err
	}
	return effective.wet, nil
}

func (r *Repository) GetWetScan(ctx context.Context, scheduleID, documentKey, date string, scope auth.RegencyScope) (*WetScanMeta, error) {
	var meta WetScanMeta
	err := r.pool.QueryRow(ctx, `
		SELECT s.content_code, s.byte_size, s.uploaded_at FROM bast_wet_signature_scans s
		JOIN program_schedules ps ON ps.id = s.schedule_id
		WHERE s.schedule_id::text=$1 AND s.document_key=$2 AND s.document_date=$3::date AND ($4 OR ps.regency_id::text = ANY($5))
	`, scheduleID, documentKey, date, scope.Unrestricted, scope.RegencyIDs).Scan(&meta.ContentCode, &meta.ByteSize, &meta.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get wet scan: %w", err)
	}
	return &meta, nil
}

func (r *Repository) OpenWetScan(ctx context.Context, scheduleID, documentKey, date string, scope auth.RegencyScope) ([]byte, error) {
	var image []byte
	err := r.pool.QueryRow(ctx, `
		SELECT s.image_png FROM bast_wet_signature_scans s
		JOIN program_schedules ps ON ps.id = s.schedule_id
		WHERE s.schedule_id::text=$1 AND s.document_key=$2 AND s.document_date=$3::date AND ($4 OR ps.regency_id::text = ANY($5))
	`, scheduleID, documentKey, date, scope.Unrestricted, scope.RegencyIDs).Scan(&image)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open wet scan: %w", err)
	}
	return image, nil
}

func (r *Repository) PutWetScan(ctx context.Context, actor auth.Principal, scheduleID, documentKey, date, code string, image []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin wet scan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bast_wet_signature_scans(schedule_id, document_key, document_date, content_code, image_png, byte_size, uploaded_by)
		VALUES ($1::uuid, $2, $3::date, $4, $5, $6, NULLIF($7,'')::uuid)
		ON CONFLICT (schedule_id, document_key, document_date) DO UPDATE SET content_code=EXCLUDED.content_code, image_png=EXCLUDED.image_png, byte_size=EXCLUDED.byte_size, uploaded_by=EXCLUDED.uploaded_by, uploaded_at=now()
	`, scheduleID, documentKey, date, code, image, len(image), actor.UserID); err != nil {
		return fmt.Errorf("save wet scan: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.wet_signature_uploaded", ResourceType: "bast_wet_signature_scan", ResourceID: scheduleID, Metadata: map[string]any{"document_key": documentKey, "document_date": date, "content_code": code}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeleteWetScan(ctx context.Context, actor auth.Principal, scheduleID, documentKey, date string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete wet scan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_wet_signature_scans WHERE schedule_id::text=$1 AND document_key=$2 AND document_date=$3::date`, scheduleID, documentKey, date); err != nil {
		return fmt.Errorf("delete wet scan: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.wet_signature_deleted", ResourceType: "bast_wet_signature_scan", ResourceID: scheduleID, Metadata: map[string]any{"document_key": documentKey, "document_date": date}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
