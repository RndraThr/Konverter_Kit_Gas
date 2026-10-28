package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestPostGISDistributionMapMigration proves the spatial schema behind the distribution map:
// PostGIS is available, media_files.geom follows latitude/longitude (and ignores missing or 0,0
// coordinates), regency boundaries have a GiST index, and distribution_slots.linked_at is stamped
// when a slot becomes linked.
func TestPostGISDistributionMapMigration(t *testing.T) {
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

	// An isolated schema holds minimal stand-ins first in the search_path, so the migration runs against
	// them (even when the shared schema is already migrated) and never touches real data.
	if _, err := tx.Exec(`
		CREATE SCHEMA postgis_map_test;
		SET LOCAL search_path = postgis_map_test, public;
		CREATE TABLE regencies (id uuid PRIMARY KEY);
		CREATE TABLE media_files (
			id integer PRIMARY KEY,
			latitude numeric(9,6),
			longitude numeric(9,6)
		);
		CREATE TABLE distribution_slots (
			id integer PRIMARY KEY,
			status text NOT NULL DEFAULT 'open',
			distributed_at timestamptz,
			completed_at timestamptz,
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		INSERT INTO distribution_slots(id,status,distributed_at,completed_at,updated_at) VALUES
			(1,'open',NULL,NULL,'2026-01-01T00:00:00Z'),
			(2,'linked',NULL,NULL,'2026-01-02T00:00:00Z'),
			(3,'completed',NULL,NULL,'2026-01-03T00:00:00Z'),
			(4,'cancelled',NULL,NULL,'2026-01-04T00:00:00Z'),
			(5,'open',NULL,NULL,'2026-01-05T00:00:00Z'),
			-- Edited after delivery: updated_at is later than the delivery, so it cannot be the link time.
			(6,'completed','2026-01-03T00:00:00Z','2026-01-04T00:00:00Z','2026-02-01T00:00:00Z'),
			(7,'completed',NULL,'2026-01-05T00:00:00Z','2026-02-01T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00080_postgis_distribution_map.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	var version string
	if err := tx.QueryRow(`SELECT postgis_version()`).Scan(&version); err != nil || version == "" {
		t.Fatalf("postgis_version: %q %v", version, err)
	}

	if _, err := tx.Exec(`INSERT INTO media_files(id,latitude,longitude) VALUES
		(1,-6.45,108.19),(2,NULL,NULL),(3,0,0),(4,-6.45,NULL),(5,0,108.19)`); err != nil {
		t.Fatal(err)
	}
	var x, y float64
	if err := tx.QueryRow(`SELECT ST_X(geom),ST_Y(geom) FROM media_files WHERE id=1`).Scan(&x, &y); err != nil {
		t.Fatal(err)
	}
	if x != 108.19 || y != -6.45 {
		t.Fatalf("geom = (%v, %v), want (108.19, -6.45)", x, y)
	}
	for id, want := range map[int]bool{2: true, 3: true, 4: true, 5: false} {
		var isNull bool
		if err := tx.QueryRow(`SELECT geom IS NULL FROM media_files WHERE id=$1`, id).Scan(&isNull); err != nil {
			t.Fatal(err)
		}
		if isNull != want {
			t.Fatalf("media_files %d: geom IS NULL = %v, want %v", id, isNull, want)
		}
	}

	var sridOK bool
	if err := tx.QueryRow(`SELECT ST_SRID(geom)=4326 FROM media_files WHERE id=1`).Scan(&sridOK); err != nil || !sridOK {
		t.Fatalf("geom SRID must be 4326: %v", err)
	}

	for _, index := range []string{"regency_boundaries_geom_idx", "media_files_geom_idx"} {
		var found bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname='postgis_map_test' AND indexname=$1 AND indexdef ILIKE '%gist%')`, index).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("missing GiST index %s", index)
		}
	}

	// Backfill: linked/completed slots take the earliest of updated_at, distributed_at and
	// completed_at (a slot is linked before it is delivered); others stay empty.
	for id, want := range map[int]string{1: "", 2: "2026-01-02", 3: "2026-01-03", 4: "", 5: "", 6: "2026-01-03", 7: "2026-01-05"} {
		var got sql.NullString
		if err := tx.QueryRow(`SELECT to_char(linked_at AT TIME ZONE 'UTC','YYYY-MM-DD') FROM distribution_slots WHERE id=$1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got.String != want {
			t.Fatalf("slot %d linked_at = %q, want %q", id, got.String, want)
		}
	}

	// Trigger: open -> linked stamps linked_at; later status changes keep the value.
	if _, err := tx.Exec(`UPDATE distribution_slots SET status='linked' WHERE id=5`); err != nil {
		t.Fatal(err)
	}
	var stamped sql.NullTime
	if err := tx.QueryRow(`SELECT linked_at FROM distribution_slots WHERE id=5`).Scan(&stamped); err != nil || !stamped.Valid {
		t.Fatalf("linked_at not stamped on open->linked: %v %v", stamped, err)
	}
	if _, err := tx.Exec(`UPDATE distribution_slots SET status='completed' WHERE id=5`); err != nil {
		t.Fatal(err)
	}
	var kept sql.NullTime
	if err := tx.QueryRow(`SELECT linked_at FROM distribution_slots WHERE id=5`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if !kept.Valid || !kept.Time.Equal(stamped.Time) {
		t.Fatalf("linked_at changed on linked->completed: %v -> %v", stamped, kept)
	}

	// Reopen completed -> linked keeps the original link time.
	if _, err := tx.Exec(`UPDATE distribution_slots SET linked_at='2025-12-01T00:00:00Z' WHERE id=5`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE distribution_slots SET status='linked' WHERE id=5`); err != nil {
		t.Fatal(err)
	}
	assertLinkedAt := func(id int, want string) {
		t.Helper()
		var got sql.NullString
		if err := tx.QueryRow(`SELECT to_char(linked_at AT TIME ZONE 'UTC','YYYY-MM-DD') FROM distribution_slots WHERE id=$1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got.String != want {
			t.Fatalf("slot %d linked_at = %q, want %q", id, got.String, want)
		}
	}
	assertLinkedAt(5, "2025-12-01")

	// INSERT as linked: an explicit linked_at is kept, a missing one is stamped.
	if _, err := tx.Exec(`INSERT INTO distribution_slots(id,status,linked_at) VALUES (8,'linked','2025-11-01T00:00:00Z'),(9,'linked',NULL)`); err != nil {
		t.Fatal(err)
	}
	assertLinkedAt(8, "2025-11-01")
	var fresh bool
	if err := tx.QueryRow(`SELECT linked_at IS NOT NULL AND linked_at > now() - interval '1 minute' FROM distribution_slots WHERE id=9`).Scan(&fresh); err != nil || !fresh {
		t.Fatalf("INSERT as linked without linked_at must be stamped now: %v %v", fresh, err)
	}

	// open -> linked always stamps, even when an old value is left over from an earlier link.
	if _, err := tx.Exec(`UPDATE distribution_slots SET status='open', linked_at='2025-10-01T00:00:00Z' WHERE id=8`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE distribution_slots SET status='linked' WHERE id=8`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT linked_at > now() - interval '1 minute' FROM distribution_slots WHERE id=8`).Scan(&fresh); err != nil || !fresh {
		t.Fatalf("open -> linked must restamp linked_at: %v %v", fresh, err)
	}
}
