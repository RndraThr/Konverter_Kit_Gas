package recipients

import (
	"context"
	"testing"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type recipientFixture struct {
	scheduleID         string
	farmerRegencyID    string
	otherRegencyID     string
	distributedAllocID string
	needsReviewAllocID string
	cancelledAllocID   string
}

func seedRecipientFixture(t *testing.T, pool *pgxpool.Pool) recipientFixture {
	t.Helper()
	ctx := context.Background()
	var fixture recipientFixture

	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Wajo Recipients Test','WRT',true) RETURNING id::text`).Scan(&fixture.farmerRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Bone Recipients Test','BRT',true) RETURNING id::text`).Scan(&fixture.otherRegencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES('RCPT-TEST','Program Test Recipients','farmer',2026,'active') RETURNING id::text`).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES('PKG-RCPT',1,'Paket Test','farmer','{}'::jsonb,'published') RETURNING id::text`).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES('DOC-RCPT',1,'Dok Test','farmer','published') RETURNING id::text`).Scan(&docTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Recipients','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, fixture.farmerRegencyID, packageTemplateID, docTemplateID).Scan(&fixture.scheduleID))

	var personIDs, nominationIDs []string
	insertAllocation := func(name, status string, distNumber int) string {
		var personID, nominationID, allocationID string
		must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name) VALUES($1) RETURNING id::text`, name).Scan(&personID))
		must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
		must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}'::jsonb) RETURNING id::text`, fixture.scheduleID, nominationID, personID, distNumber, status).Scan(&allocationID))
		personIDs = append(personIDs, personID)
		nominationIDs = append(nominationIDs, nominationID)
		return allocationID
	}
	fixture.distributedAllocID = insertAllocation("Distributed Person", "distributed", 1)
	fixture.needsReviewAllocID = insertAllocation("Needs Review Person", "needs_review", 2)
	fixture.cancelledAllocID = insertAllocation("Cancelled Person", "cancelled", 3)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_allocations WHERE schedule_id = $1`, fixture.scheduleID); err != nil {
			t.Logf("cleanup: delete package_allocations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM candidate_nominations WHERE id = ANY($1)`, nominationIDs); err != nil {
			t.Logf("cleanup: delete candidate_nominations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM people WHERE id = ANY($1)`, personIDs); err != nil {
			t.Logf("cleanup: delete people failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id = $1`, fixture.scheduleID); err != nil {
			t.Logf("cleanup: delete program_schedules failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM programs WHERE id = $1`, programID); err != nil {
			t.Logf("cleanup: delete programs failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_template_versions WHERE id = $1`, packageTemplateID); err != nil {
			t.Logf("cleanup: delete package_template_versions failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM documentation_template_versions WHERE id = $1`, docTemplateID); err != nil {
			t.Logf("cleanup: delete documentation_template_versions failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id IN ($1,$2)`, fixture.farmerRegencyID, fixture.otherRegencyID); err != nil {
			t.Logf("cleanup: delete regencies failed: %v", err)
		}
	})
	return fixture
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestListExcludesCancelledByDefaultAndCombinesFilters(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	unrestricted := auth.RegencyScope{Unrestricted: true}

	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	var sawCancelled bool
	for _, item := range page.Items {
		if item.AllocationID == fixture.cancelledAllocID {
			sawCancelled = true
		}
	}
	if sawCancelled {
		t.Fatal("default list must exclude cancelled allocations")
	}

	explicit, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, AllocationStatus: "cancelled"}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range explicit.Items {
		if item.AllocationID == fixture.cancelledAllocID {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit cancelled filter must return the cancelled allocation")
	}

	combined, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, Search: "Needs Review", AllocationStatus: "needs_review"}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Items) != 1 || combined.Items[0].AllocationID != fixture.needsReviewAllocID {
		t.Fatalf("expected exactly the needs_review match, got %+v", combined.Items)
	}
}

func TestListRespectsRegencyScope(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, scoped)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ScheduleID == fixture.scheduleID {
			t.Fatalf("scoped caller must not see recipients from an out-of-scope regency: %+v", item)
		}
	}
}

func TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	stats, err := repository.Stats(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ByAllocationStatus["distributed"] < 1 || stats.ByAllocationStatus["needs_review"] < 1 || stats.ByAllocationStatus["cancelled"] < 1 {
		t.Fatalf("expected all three statuses represented: %+v", stats.ByAllocationStatus)
	}
	_ = fixture.distributedAllocID
}
