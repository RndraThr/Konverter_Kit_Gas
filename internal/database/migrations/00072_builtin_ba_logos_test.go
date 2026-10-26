package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestBuiltinBALogosSeed proves the backfill gives a logo-less program the four built-in logos,
// leaves a program with an uploaded logo alone, and the trigger seeds every new program.
func TestBuiltinBALogosSeed(t *testing.T) {
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

	// A throwaway schema rather than pg_temp: functions are never looked up in pg_temp, and the
	// migration's functions must not collide with the real ones. The rollback drops it all.
	if _, err := tx.Exec(`
		CREATE SCHEMA migration_00072_test;
		SET LOCAL search_path = migration_00072_test, public;
		CREATE TABLE programs (id uuid PRIMARY KEY);
		CREATE TABLE program_ba_logo_assets (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
			slot_code text NOT NULL,
			storage_key text NOT NULL UNIQUE,
			original_filename text NOT NULL,
			mime_type text NOT NULL,
			byte_size bigint NOT NULL,
			checksum char(64) NOT NULL,
			sort_order integer NOT NULL DEFAULT 0,
			max_width_mm numeric(6,2) NOT NULL DEFAULT 35,
			max_height_mm numeric(6,2) NOT NULL DEFAULT 18,
			is_visible boolean NOT NULL DEFAULT true,
			UNIQUE (program_id, slot_code)
		);
		INSERT INTO programs(id) VALUES
			('00000000-0000-0000-0000-000000000001'),
			('00000000-0000-0000-0000-000000000002');
		INSERT INTO program_ba_logo_assets(program_id, slot_code, storage_key, original_filename, mime_type, byte_size, checksum)
			VALUES ('00000000-0000-0000-0000-000000000002', 'logo_1', 'drive-id', 'own.png', 'image/png', 10, repeat('a', 64));
	`); err != nil {
		t.Fatal(err)
	}
	migration, err := FS.ReadFile("00072_builtin_ba_logos.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.NewReplacer("-- +goose Up", "", "-- +goose StatementBegin", "", "-- +goose StatementEnd", "").Replace(upSQL)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute 00072: %v", err)
	}

	count := func(programID string) int {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM program_ba_logo_assets WHERE program_id = $1 AND storage_key LIKE 'builtin:%'`, programID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count("00000000-0000-0000-0000-000000000001"); got != 4 {
		t.Fatalf("backfilled logos = %d, want 4", got)
	}
	if got := count("00000000-0000-0000-0000-000000000002"); got != 0 {
		t.Fatalf("program with its own logo got %d built-in logos, want 0", got)
	}
	if _, err := tx.Exec(`INSERT INTO programs(id) VALUES ('00000000-0000-0000-0000-000000000003')`); err != nil {
		t.Fatal(err)
	}
	if got := count("00000000-0000-0000-0000-000000000003"); got != 4 {
		t.Fatalf("new program logos = %d, want 4", got)
	}
	var key string
	if err := tx.QueryRow(`SELECT storage_key FROM program_ba_logo_assets WHERE program_id = '00000000-0000-0000-0000-000000000003' AND slot_code = 'logo_1'`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != "builtin:00000000-0000-0000-0000-000000000003/04-pertamina-transparan.png" {
		t.Fatalf("storage key = %q", key)
	}
}
