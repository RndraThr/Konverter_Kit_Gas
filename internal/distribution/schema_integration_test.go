package distribution

import (
	"context"
	"testing"
)

func TestMigrationCreatesDistributionSlotsAndSeedsPOSPermissions(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'distribution_slots')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected distribution_slots table to exist")
	}
	var recordsExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'distribution_records')`).Scan(&recordsExists); err != nil {
		t.Fatal(err)
	}
	if recordsExists {
		t.Fatal("expected distribution_records table to be dropped")
	}

	var nullable string
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='package_allocations' AND column_name='distribution_number'`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != "YES" {
		t.Fatalf("expected package_allocations.distribution_number to be nullable, got is_nullable=%s", nullable)
	}

	var fkTarget string
	if err := pool.QueryRow(ctx, `
		SELECT ccu.table_name FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu ON tc.constraint_name = ccu.constraint_name
		WHERE tc.table_name = 'documentation_slots' AND tc.constraint_type = 'FOREIGN KEY' AND ccu.table_name != 'documentation_slots'
	`).Scan(&fkTarget); err != nil {
		t.Fatal(err)
	}
	if fkTarget != "distribution_slots" {
		t.Fatalf("expected documentation_slots to FK into distribution_slots, got %q", fkTarget)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('distribution.pos_mesin','distribution.pos_dokumen','distribution.pos_penyerahan')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 POS permissions seeded, got %d", count)
	}

	var superAdminGrants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp
		JOIN roles ON roles.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE roles.code = 'super_admin' AND p.code IN ('distribution.pos_mesin','distribution.pos_dokumen','distribution.pos_penyerahan')
	`).Scan(&superAdminGrants); err != nil {
		t.Fatal(err)
	}
	if superAdminGrants != 3 {
		t.Fatalf("expected super_admin granted all 3 POS permissions, got %d", superAdminGrants)
	}
}
