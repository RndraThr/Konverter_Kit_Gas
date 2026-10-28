// Package recipienttest holds the integration-test database helper and the shared recipient
// fixture, so packages that build on the Data Penerima filter (recipients, distmap) test against
// the same seeded programme, zone, schedule and allocations.
package recipienttest

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

// Pool opens TEST_DATABASE_URL (which must point at konkit_test), migrates it to the latest
// version, and closes the pool when the test ends. The test is skipped without a database.
func Pool(t *testing.T) *pgxpool.Pool {
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

// Fixture is one farmer programme with a zone, one active schedule in FarmerRegencyID (assigned
// to the zone), a second regency without schedules, and three allocations on the schedule.
type Fixture struct {
	ScheduleID         string
	ProgramID          string
	ZoneID             string
	FarmerRegencyID    string
	OtherRegencyID     string
	PackageTemplateID  string
	DocTemplateID      string
	DistributedAllocID string
	NeedsReviewAllocID string
	CancelledAllocID   string
}

// Seed inserts the fixture and registers its cleanup. Rows a test adds on top (slots, extra
// schedules) must be removed by a cleanup the test registers after calling Seed, so it runs first.
func Seed(t *testing.T, pool *pgxpool.Pool) Fixture {
	t.Helper()
	ctx := context.Background()
	var fixture Fixture

	Must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Wajo Recipients Test','WRT',true) RETURNING id::text`).Scan(&fixture.FarmerRegencyID))
	Must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Bone Recipients Test','BRT',true) RETURNING id::text`).Scan(&fixture.OtherRegencyID))
	Must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES('RCPT-TEST','Program Test Recipients','farmer',2026,'active') RETURNING id::text`).Scan(&fixture.ProgramID))
	Must(t, pool.QueryRow(ctx, `INSERT INTO program_zones(program_id,code,name,sort_order) VALUES($1,'ZONA-TEST','Zona Test Penerima',1) RETURNING id::text`, fixture.ProgramID).Scan(&fixture.ZoneID))
	Must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3)`, fixture.ProgramID, fixture.FarmerRegencyID, fixture.ZoneID)
		return err
	}())
	Must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES('PKG-RCPT',1,'Paket Test','farmer','{}'::jsonb,'published') RETURNING id::text`).Scan(&fixture.PackageTemplateID))
	Must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES('DOC-RCPT',1,'Dok Test','farmer','published') RETURNING id::text`).Scan(&fixture.DocTemplateID))
	Must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Recipients','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, fixture.ProgramID, fixture.FarmerRegencyID, fixture.PackageTemplateID, fixture.DocTemplateID).Scan(&fixture.ScheduleID))

	var personIDs, nominationIDs []string
	insertAllocation := func(name, status string, distNumber int) string {
		var personID, nominationID, allocationID string
		Must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name) VALUES($1) RETURNING id::text`, name).Scan(&personID))
		Must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
		Must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}'::jsonb) RETURNING id::text`, fixture.ScheduleID, nominationID, personID, distNumber, status).Scan(&allocationID))
		personIDs = append(personIDs, personID)
		nominationIDs = append(nominationIDs, nominationID)
		return allocationID
	}
	fixture.DistributedAllocID = insertAllocation("Distributed Person", "distributed", 1)
	fixture.NeedsReviewAllocID = insertAllocation("Needs Review Person", "needs_review", 2)
	fixture.CancelledAllocID = insertAllocation("Cancelled Person", "cancelled", 3)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_allocations WHERE schedule_id = $1`, fixture.ScheduleID); err != nil {
			t.Logf("cleanup: delete package_allocations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM candidate_nominations WHERE id = ANY($1)`, nominationIDs); err != nil {
			t.Logf("cleanup: delete candidate_nominations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM people WHERE id = ANY($1)`, personIDs); err != nil {
			t.Logf("cleanup: delete people failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id = $1`, fixture.ScheduleID); err != nil {
			t.Logf("cleanup: delete program_schedules failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM programs WHERE id = $1`, fixture.ProgramID); err != nil {
			t.Logf("cleanup: delete programs failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_template_versions WHERE id = $1`, fixture.PackageTemplateID); err != nil {
			t.Logf("cleanup: delete package_template_versions failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM documentation_template_versions WHERE id = $1`, fixture.DocTemplateID); err != nil {
			t.Logf("cleanup: delete documentation_template_versions failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id IN ($1,$2)`, fixture.FarmerRegencyID, fixture.OtherRegencyID); err != nil {
			t.Logf("cleanup: delete regencies failed: %v", err)
		}
	})
	return fixture
}

// Must fails the test immediately on a non-nil error.
func Must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
