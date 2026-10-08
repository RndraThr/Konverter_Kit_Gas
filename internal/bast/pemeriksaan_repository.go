package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// GetPemeriksaanProfile mengembalikan daftar form tersimpan milik program, atau
// tujuh form bawaan referensi (IsDefault=true) bila belum disimpan.
func (r *Repository) GetPemeriksaanProfile(ctx context.Context, programID string) (PemeriksaanProfile, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM programs WHERE id=$1)`, programID).Scan(&exists); err != nil {
		return PemeriksaanProfile{}, fmt.Errorf("check pemeriksaan program: %w", err)
	}
	if !exists {
		return PemeriksaanProfile{}, ErrNotFound
	}
	var payload []byte
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT forms_json,updated_at FROM bast_pemeriksaan_profiles WHERE program_id=$1`, programID).Scan(&payload, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultPemeriksaanProfile(programID), nil
	}
	if err != nil {
		return PemeriksaanProfile{}, fmt.Errorf("get pemeriksaan profile: %w", err)
	}
	profile := PemeriksaanProfile{ProgramID: programID, UpdatedAt: &updatedAt}
	if err := json.Unmarshal(payload, &profile.Forms); err != nil {
		return PemeriksaanProfile{}, fmt.Errorf("decode pemeriksaan profile: %w", err)
	}
	ensurePemeriksaanLists(profile.Forms)
	return profile, nil
}

func (r *Repository) UpsertPemeriksaanProfile(ctx context.Context, actor auth.Principal, profile PemeriksaanProfile, meta auth.ClientMeta) (PemeriksaanProfile, error) {
	payload, err := json.Marshal(profile.Forms)
	if err != nil {
		return PemeriksaanProfile{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PemeriksaanProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO bast_pemeriksaan_profiles(program_id,forms_json,updated_by)
		VALUES($1,$2,NULLIF($3,'')::uuid)
		ON CONFLICT(program_id) DO UPDATE SET forms_json=EXCLUDED.forms_json,updated_by=EXCLUDED.updated_by,updated_at=now()
		RETURNING updated_at
	`, profile.ProgramID, payload, actor.UserID).Scan(&updatedAt)
	if err != nil {
		return PemeriksaanProfile{}, fmt.Errorf("upsert pemeriksaan profile: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.pemeriksaan_profile_saved", ResourceType: "bast_pemeriksaan_profile", ResourceID: profile.ProgramID, Metadata: map[string]any{"forms": len(profile.Forms)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return PemeriksaanProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PemeriksaanProfile{}, err
	}
	profile.IsDefault, profile.UpdatedAt = false, &updatedAt
	return profile, nil
}

// ListAggregatesByTypePrefix mengembalikan semua versi dokumen agregat yang
// document_type-nya diawali prefix (mis. "pemeriksaan:") pada satu tanggal.
func (r *Repository) ListAggregatesByTypePrefix(ctx context.Context, scheduleID, prefix, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+aggregateColumnsD+` FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.schedule_id=$1 AND starts_with(d.document_type,$2) AND d.document_date=$3 AND ($4 OR ps.regency_id::text=ANY($5)) ORDER BY d.document_type, d.version DESC`, scheduleID, prefix, documentDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list aggregates by prefix: %w", err)
	}
	defer rows.Close()
	items := []AggregateDocument{}
	for rows.Next() {
		doc, err := scanAggregate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, doc)
	}
	return items, rows.Err()
}
