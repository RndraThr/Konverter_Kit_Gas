package recipients

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

func recipientsIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func TestMigrationAllowsNullImportLinkageAndSeedsPermissions(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	ctx := context.Background()

	var batchNullable, rowNullable string
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='candidate_nominations' AND column_name='batch_id'`).Scan(&batchNullable); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='candidate_nominations' AND column_name='import_row_id'`).Scan(&rowNullable); err != nil {
		t.Fatal(err)
	}
	if batchNullable != "YES" || rowNullable != "YES" {
		t.Fatalf("expected batch_id/import_row_id nullable, got batch=%s row=%s", batchNullable, rowNullable)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('recipients.view','recipients.manage')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 recipients permissions seeded, got %d", count)
	}

	var superAdminGrants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp
		JOIN roles ON roles.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE roles.code = 'super_admin' AND p.code IN ('recipients.view','recipients.manage')
	`).Scan(&superAdminGrants); err != nil {
		t.Fatal(err)
	}
	if superAdminGrants != 2 {
		t.Fatalf("expected super_admin granted both new permissions, got %d", superAdminGrants)
	}
}
