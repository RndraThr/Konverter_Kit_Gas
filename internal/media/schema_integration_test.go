package media

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func mediaIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func TestMigrationCreatesDriveFolderCacheTable(t *testing.T) {
	pool := mediaIntegrationPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ('wajo/dokumentasi-foto-video/rakor', 'fake-drive-id-1')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor'`); err != nil {
			t.Logf("cleanup: delete drive_folder_cache failed: %v", err)
		}
	})

	var driveFolderID string
	if err := pool.QueryRow(ctx, `SELECT drive_folder_id FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor'`).Scan(&driveFolderID); err != nil {
		t.Fatal(err)
	}
	if driveFolderID != "fake-drive-id-1" {
		t.Fatalf("drive_folder_id = %q", driveFolderID)
	}

	// UNIQUE(path_key) enforced.
	if _, err := pool.Exec(ctx, `INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ('wajo/dokumentasi-foto-video/rakor', 'fake-drive-id-2')`); err == nil {
		t.Fatal("expected unique violation on duplicate path_key")
	}
}
