package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestActivityMediaDokumenPendukungType proves the activity_media.activity_type
// CHECK constraint accepts 'dokumen_pendukung' after the migration is applied,
// while still rejecting values that were never valid (regression guard against
// the constraint simply being dropped instead of widened).
func TestActivityMediaDokumenPendukungType(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	// Minimal stand-in for regencies, satisfying the activity_media FK.
	_, err = tx.Exec(`
		CREATE TEMP TABLE regencies (id uuid PRIMARY KEY);
		INSERT INTO regencies(id) VALUES ('00000000-0000-0000-0000-000000000001');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	// Recreate activity_media as it existed right after 00009_activities.sql,
	// i.e. before this migration's fix, with the original 9-value constraint.
	_, err = tx.Exec(`
		CREATE TEMP TABLE activity_media (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			regency_id uuid NOT NULL REFERENCES regencies(id),
			activity_type text NOT NULL CONSTRAINT activity_media_activity_type_check CHECK (activity_type IN (
				'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
				'training_10', 'training_100',
				'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
				'unloading_selang', 'unloading_tabung_gas'
			))
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Sanity check: before the migration, dokumen_pendukung is rejected.
	if _, err := tx.Exec(`SAVEPOINT before_migration`); err != nil {
		t.Fatal(err)
	}
	_, preMigrationErr := tx.Exec(`
		INSERT INTO activity_media(regency_id, activity_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'dokumen_pendukung')
	`)
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT before_migration`); err != nil {
		t.Fatal(err)
	}
	if preMigrationErr == nil {
		t.Fatal("dokumen_pendukung was accepted before the migration was applied")
	}

	migration, err := FS.ReadFile("00066_activity_media_dokumen_pendukung_type.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// dokumen_pendukung now succeeds.
	if _, err := tx.Exec(`
		INSERT INTO activity_media(regency_id, activity_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'dokumen_pendukung')
	`); err != nil {
		t.Fatalf("insert dokumen_pendukung rejected after migration: %v", err)
	}

	// The constraint still rejects values that were never valid, proving it
	// was widened rather than dropped.
	if _, err := tx.Exec(`SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	_, invalidErr := tx.Exec(`
		INSERT INTO activity_media(regency_id, activity_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'not_a_real_type')
	`)
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	if invalidErr == nil {
		t.Fatal("not_a_real_type was accepted; constraint appears to have been dropped entirely")
	}
}
