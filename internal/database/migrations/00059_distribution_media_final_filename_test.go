package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestDistributionMediaFinalFilenameSchema proves the move-job queue can carry the
// per-media final filename that the worker applies together with the parent move.
func TestDistributionMediaFinalFilenameSchema(t *testing.T) {
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
		CREATE TEMP TABLE distribution_media_move_jobs (
			media_file_id uuid PRIMARY KEY,
			target_path text[] NOT NULL,
			target_generation bigint NOT NULL
		);
		INSERT INTO distribution_media_move_jobs(media_file_id, target_path, target_generation)
		VALUES ('00000000-0000-0000-0000-000000000001', ARRAY['existing'], 1);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00059_distribution_media_final_filename.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// Existing queued jobs must survive the migration with a usable empty default; the worker
	// treats an empty target filename as "keep the name produced at upload time".
	var existing string
	if err := tx.QueryRow(`SELECT target_filename FROM distribution_media_move_jobs WHERE media_file_id='00000000-0000-0000-0000-000000000001'`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		t.Fatalf("existing job target_filename=%q, want empty", existing)
	}

	if _, err := tx.Exec(`
		INSERT INTO distribution_media_move_jobs(media_file_id, target_path, target_generation, target_filename)
		VALUES ('00000000-0000-0000-0000-000000000002', ARRAY['target'], 1, 'AHMAD - FOTO MESIN - 01.jpg')
	`); err != nil {
		t.Fatalf("explicit target filename rejected: %v", err)
	}
}
