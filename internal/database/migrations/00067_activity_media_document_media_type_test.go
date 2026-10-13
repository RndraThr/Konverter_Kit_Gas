package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestActivityMediaDocumentMediaType proves the activity_media.media_type
// CHECK constraint accepts 'document' after the migration is applied, while
// still rejecting values that were never valid (regression guard against
// the constraint simply being dropped instead of widened).
func TestActivityMediaDocumentMediaType(t *testing.T) {
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
	// i.e. with the original media_type constraint allowing only image/video.
	_, err = tx.Exec(`
		CREATE TEMP TABLE activity_media (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			regency_id uuid NOT NULL REFERENCES regencies(id),
			media_type text NOT NULL CONSTRAINT activity_media_media_type_check CHECK (media_type IN ('image', 'video'))
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00067_activity_media_document_media_type.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// 'document' now succeeds.
	if _, err := tx.Exec(`
		INSERT INTO activity_media(regency_id, media_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'document')
	`); err != nil {
		t.Fatalf("insert media_type=document rejected after migration: %v", err)
	}

	// The constraint still rejects values that were never valid.
	if _, err := tx.Exec(`SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	_, invalidErr := tx.Exec(`
		INSERT INTO activity_media(regency_id, media_type)
		VALUES ('00000000-0000-0000-0000-000000000001', 'not_a_real_kind')
	`)
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_value`); err != nil {
		t.Fatal(err)
	}
	if invalidErr == nil {
		t.Fatal("not_a_real_kind was accepted; constraint appears to have been dropped entirely")
	}
}
