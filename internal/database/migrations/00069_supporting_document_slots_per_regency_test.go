package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestSupportingDocumentSlotsPerRegencySchema proves the slot table moved from
// schedule_id to regency_id: the old column/FK is gone, the new one accepts a
// real regency and rejects a schedule_id-shaped insert.
func TestSupportingDocumentSlotsPerRegencySchema(t *testing.T) {
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

	if _, err := tx.Exec(`
		CREATE TEMP TABLE program_schedules (id uuid PRIMARY KEY);
		CREATE TEMP TABLE regencies (id uuid PRIMARY KEY);
		INSERT INTO regencies(id) VALUES ('00000000-0000-0000-0000-000000000002');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	runUp := func(filename string) {
		migration, err := FS.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
		upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
		if _, err := tx.Exec(upSQL); err != nil {
			t.Fatalf("execute %s: %v", filename, err)
		}
	}
	runUp("00065_supporting_document_slots.sql")
	runUp("00069_supporting_document_slots_per_regency.sql")

	// Scoped to pg_temp specifically — konkit_test's real public schema may
	// already have its own (differently-versioned) supporting_document_slots
	// table from other integration tests, which an unscoped
	// information_schema query would also match.
	var scheduleColumnExists bool
	if err := tx.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema LIKE 'pg_temp%' AND table_name = 'supporting_document_slots' AND column_name = 'schedule_id')
	`).Scan(&scheduleColumnExists); err != nil {
		t.Fatal(err)
	}
	if scheduleColumnExists {
		t.Fatal("schedule_id column still exists after the per-regency migration")
	}

	var regencyColumnExists bool
	if err := tx.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema LIKE 'pg_temp%' AND table_name = 'supporting_document_slots' AND column_name = 'regency_id')
	`).Scan(&regencyColumnExists); err != nil {
		t.Fatal(err)
	}
	if !regencyColumnExists {
		t.Fatal("regency_id column was not added")
	}

	if _, err := tx.Exec(`
		INSERT INTO supporting_document_slots(regency_id, name) VALUES ('00000000-0000-0000-0000-000000000002', 'Surat Tugas')
	`); err != nil {
		t.Fatalf("insert valid regency_id row rejected: %v", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO supporting_document_slots(regency_id, name) VALUES ('00000000-0000-0000-0000-000000000099', 'Ghost')
	`); err == nil {
		t.Fatal("insert with a nonexistent regency_id should have been rejected by the FK constraint")
	}
}
