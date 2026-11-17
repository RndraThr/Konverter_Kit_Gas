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

// ListPlannedPackageRows memberi jumlah paket yang AKAN dibagikan untuk BA
// Pemeriksaan, yang dibuat sebelum distribusi: kuota nomor bagi jadwal, atau
// bila kuota kosong, jumlah penerima DCP3 yang masih aktif. Baris per merk
// mesin diambil dari paket tiap penerima; bila belum ada penerima, dari mesin
// default template paket.
func (r *Repository) ListPlannedPackageRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingKabupatenRow, int, error) {
	var quota int
	var packageValues []byte
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(ps.slot_quota, 0), COALESCE(pt.values_json, '{}'::jsonb)
		FROM program_schedules ps LEFT JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		WHERE ps.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&quota, &packageValues)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("get planned package count: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(pa.package_snapshot_json->'selected_machine'->>'brand',''), COALESCE(pa.package_snapshot_json->'selected_machine'->>'type',''), count(*)
		FROM package_allocations pa
		WHERE pa.schedule_id=$1 AND pa.status NOT IN ('cancelled','replaced')
		GROUP BY 1, 2 ORDER BY 3 DESC, 1, 2`, scheduleID)
	if err != nil {
		return nil, 0, fmt.Errorf("list planned packages: %w", err)
	}
	defer rows.Close()
	var planned []ClosingKabupatenRow
	allocated := 0
	for rows.Next() {
		var row ClosingKabupatenRow
		if err := rows.Scan(&row.MachineBrand, &row.MachineType, &row.Count); err != nil {
			return nil, 0, err
		}
		allocated += row.Count
		planned = append(planned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := quota
	if total <= 0 {
		total = allocated
	}
	if len(planned) == 0 && total > 0 {
		brand, typ := defaultTemplateMachine(packageValues)
		planned = []ClosingKabupatenRow{{MachineBrand: brand, MachineType: typ, Count: total}}
	}
	return planned, total, nil
}

// defaultTemplateMachine adalah mesin bertanda default di template paket,
// atau satu-satunya mesin.
func defaultTemplateMachine(packageValues []byte) (brand, typ string) {
	var values struct {
		Machines []struct {
			Brand     string `json:"brand"`
			Type      string `json:"type"`
			IsDefault bool   `json:"is_default"`
		} `json:"machine_options"`
	}
	if json.Unmarshal(packageValues, &values) != nil {
		return "", ""
	}
	for _, machine := range values.Machines {
		if machine.IsDefault {
			return machine.Brand, machine.Type
		}
	}
	if len(values.Machines) == 1 {
		return values.Machines[0].Brand, values.Machines[0].Type
	}
	return "", ""
}
