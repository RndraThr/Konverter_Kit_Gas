package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStreamingDocumentationMediaMigrationDefaultsAndConstraints(t *testing.T) {
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
		CREATE TEMP TABLE documentation_template_slots (id integer PRIMARY KEY);
		CREATE TEMP TABLE documentation_slots (id integer PRIMARY KEY);
		CREATE TEMP TABLE media_files (
			mime_type text NOT NULL CONSTRAINT media_files_mime_type_check CHECK (mime_type IN ('image/jpeg','image/png','image/webp')),
			byte_size bigint NOT NULL CONSTRAINT media_files_byte_size_check CHECK (byte_size > 0 AND byte_size <= 10485760)
		);
		CREATE TEMP TABLE activity_media (
			byte_size bigint NOT NULL CONSTRAINT activity_media_byte_size_check CHECK (byte_size > 0 AND byte_size <= 104857600)
		);
		INSERT INTO documentation_template_slots(id) VALUES (1);
		INSERT INTO documentation_slots(id) VALUES (1);
	`)
	if err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00042_streaming_documentation_media.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	var templateKind, slotKind string
	if err := tx.QueryRow(`SELECT media_kind FROM documentation_template_slots WHERE id=1`).Scan(&templateKind); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT media_kind FROM documentation_slots WHERE id=1`).Scan(&slotKind); err != nil {
		t.Fatal(err)
	}
	if templateKind != "image" || slotKind != "image" {
		t.Fatalf("defaults template=%q slot=%q", templateKind, slotKind)
	}
	if _, err := tx.Exec(`INSERT INTO documentation_template_slots(id,media_kind) VALUES (2,'image_video'),(3,'video')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO media_files(mime_type,byte_size) VALUES ('video/mp4',524288000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO activity_media(byte_size) VALUES (524288000)`); err != nil {
		t.Fatal(err)
	}

	for name, query := range map[string]string{
		"invalid media_kind":      `INSERT INTO documentation_slots(id,media_kind) VALUES (2,'document')`,
		"oversize media_files":    `INSERT INTO media_files(mime_type,byte_size) VALUES ('video/mp4',524288001)`,
		"oversize activity_media": `INSERT INTO activity_media(byte_size) VALUES (524288001)`,
	} {
		if _, err := tx.Exec(`SAVEPOINT invalid_value`); err != nil {
			t.Fatal(err)
		}
		_, insertErr := tx.Exec(query)
		if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_value`); err != nil {
			t.Fatal(err)
		}
		if insertErr == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}
