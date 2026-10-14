package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestActivityMediaPDFMimeType proves the activity_media.mime_type CHECK
// constraint accepts 'application/pdf' after the migration is applied, while
// still rejecting values that were never valid (regression guard against
// the constraint simply being dropped instead of widened).
func TestActivityMediaPDFMimeType(t *testing.T) {
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

	// Recreate activity_media as it existed right before this migration,
	// i.e. with the original mime_type constraint (no application/pdf).
	_, err = tx.Exec(`
		CREATE TEMP TABLE activity_media (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			regency_id uuid NOT NULL REFERENCES regencies(id),
			mime_type text NOT NULL CONSTRAINT activity_media_mime_type_check CHECK (mime_type IN (
				'image/jpeg', 'image/png', 'image/webp',
				'video/mp4', 'video/webm', 'video/quicktime'
			))
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00068_activity_media_pdf_mime_type.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// 'application/pdf' now succeeds.
	if _, err := tx.Exec(`
		INSERT INTO activity_media(regency_id, mime_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'application/pdf')
	`); err != nil {
		t.Fatalf("insert mime_type=application/pdf rejected after migration: %v", err)
	}

	// The constraint still rejects values that were never valid.
	if _, err := tx.Exec(`SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	_, invalidErr := tx.Exec(`
		INSERT INTO activity_media(regency_id, mime_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'application/zip')
	`)
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	if invalidErr == nil {
		t.Fatal("application/zip was accepted; constraint appears to have been dropped entirely")
	}
}
