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

// GetTKDNProfile mengembalikan daftar TKDN tersimpan milik program, atau daftar
// bawaan referensi (IsDefault=true) bila program belum menyimpannya.
func (r *Repository) GetTKDNProfile(ctx context.Context, programID string) (TKDNProfile, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM programs WHERE id=$1)`, programID).Scan(&exists); err != nil {
		return TKDNProfile{}, fmt.Errorf("check TKDN program: %w", err)
	}
	if !exists {
		return TKDNProfile{}, ErrNotFound
	}
	var payload []byte
	var total float64
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT items_json,total_tkdn::float8,updated_at FROM bast_tkdn_profiles WHERE program_id=$1`, programID).Scan(&payload, &total, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultTKDNProfile(programID), nil
	}
	if err != nil {
		return TKDNProfile{}, fmt.Errorf("get TKDN profile: %w", err)
	}
	profile := TKDNProfile{ProgramID: programID, TotalTKDN: total, UpdatedAt: &updatedAt}
	if err := json.Unmarshal(payload, &profile.Rows); err != nil {
		return TKDNProfile{}, fmt.Errorf("decode TKDN rows: %w", err)
	}
	// Profil lama (sebelum susunan merujuk template) tidak punya baris.
	if len(profile.Rows) == 0 {
		profile.Rows = defaultTKDNRows()
	}
	return profile, nil
}

func (r *Repository) UpsertTKDNProfile(ctx context.Context, actor auth.Principal, profile TKDNProfile, meta auth.ClientMeta) (TKDNProfile, error) {
	payload, err := json.Marshal(profile.Rows)
	if err != nil {
		return TKDNProfile{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TKDNProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO bast_tkdn_profiles(program_id,items_json,total_tkdn,updated_by)
		VALUES($1,$2,$3,NULLIF($4,'')::uuid)
		ON CONFLICT(program_id) DO UPDATE SET items_json=EXCLUDED.items_json,total_tkdn=EXCLUDED.total_tkdn,updated_by=EXCLUDED.updated_by,updated_at=now()
		RETURNING updated_at
	`, profile.ProgramID, payload, profile.TotalTKDN, actor.UserID).Scan(&updatedAt)
	if err != nil {
		return TKDNProfile{}, fmt.Errorf("upsert TKDN profile: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.tkdn_profile_saved", ResourceType: "bast_tkdn_profile", ResourceID: profile.ProgramID, Metadata: map[string]any{"rows": len(profile.Rows), "total_tkdn": profile.TotalTKDN}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return TKDNProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TKDNProfile{}, err
	}
	profile.IsDefault, profile.UpdatedAt = false, &updatedAt
	return profile, nil
}
