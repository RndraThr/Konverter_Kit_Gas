package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDistributionRevisionSchema(t *testing.T) {
	migration, err := FS.ReadFile("00043_distribution_slot_revisions.sql")
	if err != nil {
		t.Fatal(err)
	}
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
		CREATE TEMP TABLE users (id uuid PRIMARY KEY);
		CREATE TEMP TABLE distribution_slots (id uuid PRIMARY KEY);
		CREATE TEMP TABLE bast_individual_documents (
			id uuid PRIMARY KEY,
			status text NOT NULL CONSTRAINT bast_individual_documents_status_check CHECK (status IN ('final','superseded'))
		);
		CREATE TEMP TABLE bast_daily_bundles (
			id uuid PRIMARY KEY,
			status text NOT NULL CONSTRAINT bast_daily_bundles_status_check CHECK (status IN ('active','superseded','failed'))
		);
		CREATE TEMP TABLE bast_aggregate_documents (
			id uuid PRIMARY KEY,
			status text NOT NULL CONSTRAINT bast_aggregate_documents_status_check CHECK (status IN ('active','superseded'))
		);
		INSERT INTO distribution_slots(id) VALUES ('00000000-0000-0000-0000-000000000001');
		INSERT INTO bast_individual_documents(id,status) VALUES ('00000000-0000-0000-0000-000000000002','final');
		INSERT INTO bast_daily_bundles(id,status) VALUES ('00000000-0000-0000-0000-000000000003','active');
		INSERT INTO bast_aggregate_documents(id,status) VALUES ('00000000-0000-0000-0000-000000000004','active');
	`)
	if err != nil {
		t.Fatal(err)
	}

	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	if _, err := tx.Exec(`
		UPDATE distribution_slots
		SET needs_recompletion=true,
			reopened_at=now(),
			reopened_stage='dokumen',
			revision_reason='Koreksi nomor seri'
		WHERE id='00000000-0000-0000-0000-000000000001'
	`); err != nil {
		t.Fatalf("valid revision metadata rejected: %v", err)
	}

	if _, err := tx.Exec(`SAVEPOINT invalid_stage`); err != nil {
		t.Fatal(err)
	}
	_, invalidStageErr := tx.Exec(`UPDATE distribution_slots SET reopened_stage='invalid' WHERE id='00000000-0000-0000-0000-000000000001'`)
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT invalid_stage`); err != nil {
		t.Fatal(err)
	}
	if invalidStageErr == nil {
		t.Fatal("invalid reopened_stage was accepted")
	}

	for name, query := range map[string]string{
		"individual": `UPDATE bast_individual_documents SET status='stale'`,
		"bundle":     `UPDATE bast_daily_bundles SET status='stale'`,
		"aggregate":  `UPDATE bast_aggregate_documents SET status='stale'`,
	} {
		if _, err := tx.Exec(query); err != nil {
			t.Fatalf("%s stale status rejected: %v", name, err)
		}
	}
}
