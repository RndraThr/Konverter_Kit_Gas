package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestRecipientReplacementsSchema proves the replacement history table accepts a valid row and
// rejects the two shapes the application must never write: an unknown origin and an empty reason.
func TestRecipientReplacementsSchema(t *testing.T) {
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
		CREATE TEMP TABLE distribution_slots (id uuid PRIMARY KEY);
		CREATE TEMP TABLE package_allocations (id uuid PRIMARY KEY);
		CREATE TEMP TABLE people (id uuid PRIMARY KEY);
		CREATE TEMP TABLE users (id uuid PRIMARY KEY);
		INSERT INTO distribution_slots(id) VALUES ('00000000-0000-0000-0000-000000000001');
		INSERT INTO package_allocations(id) VALUES
			('00000000-0000-0000-0000-000000000002'),
			('00000000-0000-0000-0000-000000000003');
		INSERT INTO people(id) VALUES
			('00000000-0000-0000-0000-000000000004'),
			('00000000-0000-0000-0000-000000000005');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path = pg_temp`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00060_recipient_replacements.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO recipient_replacements(
			distribution_slot_id,old_allocation_id,old_person_id,new_allocation_id,new_person_id,origin,reason)
		VALUES (
			'00000000-0000-0000-0000-000000000001',
			'00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000004',
			'00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000005',
			'existing_allocation','Penerima awal tidak dapat hadir')
	`); err != nil {
		t.Fatalf("valid replacement rejected: %v", err)
	}

	assertRejected := func(name, origin, reason string) {
		t.Helper()
		if _, err := tx.Exec(`SAVEPOINT invalid_replacement`); err != nil {
			t.Fatal(err)
		}
		_, rejectedErr := tx.Exec(`
			INSERT INTO recipient_replacements(
				distribution_slot_id,old_allocation_id,old_person_id,new_allocation_id,new_person_id,origin,reason)
			VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000004',
				'00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000005',
				$1,$2)
		`, origin, reason)
		if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_replacement`); err != nil {
			t.Fatal(err)
		}
		if rejectedErr == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	assertRejected("unknown origin", "dcp3_allocation", "alasan sah")
	assertRejected("blank reason", "new_allocation", "   ")
}
