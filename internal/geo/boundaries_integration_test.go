package geo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"slices"
	"testing"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func geoIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require konkit_test, got %q", config.ConnConfig.Database)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestBoundaryAssetMatchesFrontendCopy(t *testing.T) {
	frontend, err := os.ReadFile("../../frontend/src/features/dashboard/data/indonesia-regencies.json")
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(frontend) != sha256.Sum256(regencyBoundaryAsset) {
		t.Fatal("internal/geo/data/indonesia-regencies.json differs from the frontend asset; copy it again")
	}
}

func TestEnsureRegencyBoundaries(t *testing.T) {
	pool := geoIntegrationPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `DELETE FROM regency_boundaries`); err != nil {
		t.Fatal(err)
	}
	var indramayuID, ghostID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies WHERE name = 'KAB. INDRAMAYU'`).Scan(&indramayuID); err != nil {
		if err := pool.QueryRow(ctx, `INSERT INTO regencies (province_name, name, document_code) VALUES ('JAWA BARAT', 'KAB. INDRAMAYU', 'ZZA') RETURNING id::text`).Scan(&indramayuID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM regencies WHERE document_code = 'ZZA'`) })
	}
	if err := pool.QueryRow(ctx, `INSERT INTO regencies (province_name, name, document_code) VALUES ('TEST', 'KAB. TIDAK ADA', 'ZZB') RETURNING id::text`).Scan(&ghostID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM regencies WHERE id = $1`, ghostID) })

	first, err := EnsureRegencyBoundaries(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if first.Imported != 518 {
		t.Fatalf("Imported = %d, want 518", first.Imported)
	}
	var linkedTo string
	if err := pool.QueryRow(ctx, `SELECT regency_id::text FROM regency_boundaries WHERE normalized_key = 'INDRAMAYU'`).Scan(&linkedTo); err != nil || linkedTo != indramayuID {
		t.Fatalf("Indramayu boundary regency_id = %q (err %v), want %q", linkedTo, err, indramayuID)
	}
	if !slices.Contains(first.Unmatched, "KAB. TIDAK ADA") {
		t.Fatalf("Unmatched %v should contain KAB. TIDAK ADA", first.Unmatched)
	}
	if slices.Contains(first.Unmatched, "KAB. INDRAMAYU") {
		t.Fatal("KAB. INDRAMAYU must be matched")
	}
	if first.Matched < 1 {
		t.Fatalf("Matched = %d", first.Matched)
	}
	var invalid, empty int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE NOT ST_IsValid(geom)), count(*) FILTER (WHERE ST_IsEmpty(geom)) FROM regency_boundaries`).Scan(&invalid, &empty); err != nil {
		t.Fatal(err)
	}
	if invalid != 0 || empty != 0 {
		t.Fatalf("invalid=%d empty=%d geometries", invalid, empty)
	}

	second, err := EnsureRegencyBoundaries(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 {
		t.Fatalf("second Imported = %d, want 0", second.Imported)
	}
	if second.Matched != first.Matched {
		t.Fatalf("second Matched = %d, want %d", second.Matched, first.Matched)
	}
}
