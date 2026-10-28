package recipients

import (
	"context"
	"testing"

	"konkit/internal/recipients/recipienttest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func recipientsIntegrationPool(t *testing.T) *pgxpool.Pool { return recipienttest.Pool(t) }

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
