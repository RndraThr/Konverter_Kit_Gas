package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestSupportingDocumentSlotsSchema proves the supporting document slots table is created
// with the correct columns and constraints.
func TestSupportingDocumentSlotsSchema(t *testing.T) {
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
		CREATE TEMP TABLE program_schedules (id uuid PRIMARY KEY);
		INSERT INTO program_schedules(id) VALUES ('00000000-0000-0000-0000-000000000001');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00065_supporting_document_slots.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// Verify the table exists
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'supporting_document_slots')`).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("supporting_document_slots table was not created")
	}

	// Verify a row can be inserted successfully
	if _, err := tx.Exec(`
		INSERT INTO supporting_document_slots(schedule_id, name)
		VALUES ('00000000-0000-0000-0000-000000000001', 'KTP')
	`); err != nil {
		t.Fatalf("insert valid row rejected: %v", err)
	}
}
