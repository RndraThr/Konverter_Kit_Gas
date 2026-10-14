package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestSupportingDocumentSlotsDefaultSeed proves the default seed inserts the
// 7 expected names per regency, with the regency's own name interpolated
// into the last 3.
func TestSupportingDocumentSlotsDefaultSeed(t *testing.T) {
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
		CREATE TEMP TABLE regencies (id uuid PRIMARY KEY, name text);
		INSERT INTO regencies(id, name) VALUES
			('00000000-0000-0000-0000-000000000003', 'Kabupaten Wajo'),
			('00000000-0000-0000-0000-000000000004', 'Kabupaten Bone');
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
	runUp("00070_supporting_document_slots_default_seed.sql")

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM supporting_document_slots`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 14 {
		t.Fatalf("total slots = %d, want 14 (7 per regency x 2 regencies)", total)
	}

	var wajoCount int
	if err := tx.QueryRow(`SELECT count(*) FROM supporting_document_slots WHERE regency_id = '00000000-0000-0000-0000-000000000003'`).Scan(&wajoCount); err != nil {
		t.Fatal(err)
	}
	if wajoCount != 7 {
		t.Fatalf("Wajo slot count = %d, want 7", wajoCount)
	}

	var loGasName string
	if err := tx.QueryRow(`SELECT name FROM supporting_document_slots WHERE regency_id = '00000000-0000-0000-0000-000000000003' AND name LIKE 'LO Gas %'`).Scan(&loGasName); err != nil {
		t.Fatal(err)
	}
	if loGasName != "LO Gas Kabupaten Wajo" {
		t.Fatalf("loGasName = %q, want %q", loGasName, "LO Gas Kabupaten Wajo")
	}

	var suratKuasaName string
	if err := tx.QueryRow(`SELECT name FROM supporting_document_slots WHERE regency_id = '00000000-0000-0000-0000-000000000004' AND name LIKE 'Surat Kuasa%'`).Scan(&suratKuasaName); err != nil {
		t.Fatal(err)
	}
	if suratKuasaName != "Surat Kuasa Pengambilan Gas LPG 3Kg - Kabupaten Bone" {
		t.Fatalf("suratKuasaName = %q, want %q", suratKuasaName, "Surat Kuasa Pengambilan Gas LPG 3Kg - Kabupaten Bone")
	}

	var doMesinCount int
	if err := tx.QueryRow(`SELECT count(*) FROM supporting_document_slots WHERE name = 'DO Mesin'`).Scan(&doMesinCount); err != nil {
		t.Fatal(err)
	}
	if doMesinCount != 2 {
		t.Fatalf("DO Mesin count = %d, want 2 (one per regency, name unchanged by regency)", doMesinCount)
	}
}
