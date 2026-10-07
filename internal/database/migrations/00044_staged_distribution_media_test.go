package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStagedDistributionMediaSchema(t *testing.T) {
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
		CREATE TEMP TABLE distribution_slots (
			id uuid PRIMARY KEY,
			distribution_date date NOT NULL DEFAULT ((CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Jakarta')::date)
		);
		CREATE TEMP TABLE documentation_slots (id uuid PRIMARY KEY);
		CREATE TEMP TABLE media_files (
			id uuid PRIMARY KEY,
			documentation_slot_id uuid NOT NULL REFERENCES documentation_slots(id) ON DELETE CASCADE,
			status text NOT NULL DEFAULT 'accepted'
		);
		INSERT INTO documentation_slots(id) VALUES ('00000000-0000-0000-0000-000000000001');
		INSERT INTO media_files(id, documentation_slot_id)
		VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00044_staged_distribution_media.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	if _, err := tx.Exec(`INSERT INTO distribution_slots(id, distribution_date) VALUES ('00000000-0000-0000-0000-000000000003', NULL)`); err != nil {
		t.Fatalf("null distribution date rejected: %v", err)
	}

	var storageState string
	if err := tx.QueryRow(`SELECT storage_state FROM media_files WHERE id='00000000-0000-0000-0000-000000000002'`).Scan(&storageState); err != nil {
		t.Fatal(err)
	}
	if storageState != "final" {
		t.Fatalf("existing media storage_state=%q, want final", storageState)
	}

	assertRejected := func(name, query string) {
		t.Helper()
		if _, err := tx.Exec(`SAVEPOINT invalid_value`); err != nil {
			t.Fatal(err)
		}
		_, rejectedErr := tx.Exec(query)
		if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_value`); err != nil {
			t.Fatal(err)
		}
		if rejectedErr == nil {
			t.Fatalf("%s was accepted", name)
		}
	}

	assertRejected("invalid media storage state", `UPDATE media_files SET storage_state='lost' WHERE id='00000000-0000-0000-0000-000000000002'`)
	assertRejected("invalid move job status", `
		INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_generation,status)
		VALUES ('00000000-0000-0000-0000-000000000002', ARRAY['target'], 1, 'done')
	`)

	if _, err := tx.Exec(`
		INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_generation)
		VALUES ('00000000-0000-0000-0000-000000000002', ARRAY['target'], 1)
	`); err != nil {
		t.Fatalf("valid move job rejected: %v", err)
	}
	assertRejected("duplicate move job for media", `
		INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_generation)
		VALUES ('00000000-0000-0000-0000-000000000002', ARRAY['other-target'], 2)
	`)
}
