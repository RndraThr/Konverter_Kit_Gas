package recipients

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/recipients/recipienttest"

	"github.com/jackc/pgx/v5/pgxpool"
)

type recipientFixture struct {
	scheduleID         string
	programID          string
	zoneID             string
	farmerRegencyID    string
	otherRegencyID     string
	distributedAllocID string
	needsReviewAllocID string
	cancelledAllocID   string
}

// seedRecipientFixture adapts the shared recipienttest fixture to the field names these tests use.
func seedRecipientFixture(t *testing.T, pool *pgxpool.Pool) recipientFixture {
	t.Helper()
	f := recipienttest.Seed(t, pool)
	return recipientFixture{
		scheduleID: f.ScheduleID, programID: f.ProgramID, zoneID: f.ZoneID,
		farmerRegencyID: f.FarmerRegencyID, otherRegencyID: f.OtherRegencyID,
		distributedAllocID: f.DistributedAllocID, needsReviewAllocID: f.NeedsReviewAllocID, cancelledAllocID: f.CancelledAllocID,
	}
}

func TestListIncludesAndFiltersProgramZone(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)

	page, err := repository.List(context.Background(), Filter{Page: 1, PageSize: 20, ZoneID: fixture.zoneID}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("expected active recipients in selected zone, got %+v", page)
	}
	for _, item := range page.Items {
		if item.ZoneID != fixture.zoneID || item.ZoneCode != "ZONA-TEST" || item.ZoneName != "Zona Test Penerima" {
			t.Fatalf("recipient lost program zone: %+v", item)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	recipienttest.Must(t, err)
}

// TestMapRegionsAggregatesRecipientsEvidenceAndSlotProgress proves the map numbers are produced by
// the same filter contract as the list (cancelled excluded by default), that evidence buckets line
// up with the list's evidence status, and that slot progress plus quota is reported for regencies
// that hold schedules and quota but no recipient yet.
func TestMapRegionsAggregatesRecipientsEvidenceAndSlotProgress(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE program_schedules SET slot_quota=10 WHERE id=$1`, fixture.scheduleID)
		return err
	}())
	must(t, func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id)
			SELECT $1,1,'completed',pa.id,pa.intended_person_id FROM package_allocations pa WHERE pa.id=$2
		`, fixture.scheduleID, fixture.distributedAllocID)
		return err
	}())
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,2,'open')`, fixture.scheduleID)
		return err
	}())
	// Registered after the fixture's own cleanup, so it runs first (t.Cleanup is LIFO) and removes
	// the slots that reference the fixture's allocations before those allocations are deleted.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE schedule_id=$1`, fixture.scheduleID)
	})
	// A quota-only regency: it has a schedule but no allocation at all.
	var quotaOnlyScheduleID string
	var quotaOnlyPackage, quotaOnlyDoc string
	must(t, pool.QueryRow(ctx, `SELECT package_template_version_id::text, documentation_template_version_id::text FROM program_schedules WHERE id=$1`, fixture.scheduleID).Scan(&quotaOnlyPackage, &quotaOnlyDoc))
	must(t, pool.QueryRow(ctx, `
		INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota,receipt_policy_json)
		VALUES($1,$2,$3,$4,'Jadwal Kuota Saja','2026-01-01','2026-12-31','active',7,'{}'::jsonb) RETURNING id::text
	`, fixture.programID, fixture.otherRegencyID, quotaOnlyPackage, quotaOnlyDoc).Scan(&quotaOnlyScheduleID))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, quotaOnlyScheduleID)
	})

	data, err := repository.MapRegions(ctx, Filter{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]MapRegion{}
	for _, region := range data.Regions {
		byID[region.RegencyID] = region
	}
	farmer, ok := byID[fixture.farmerRegencyID]
	if !ok {
		t.Fatalf("farmer regency missing from map: %+v", data.Regions)
	}
	// Cancelled allocations are excluded by the default filter, exactly like the list.
	if farmer.Recipients != 2 || farmer.Distributed != 1 || farmer.NeedsReview != 1 {
		t.Fatalf("farmer recipient aggregate=%+v", farmer)
	}
	if farmer.EvidenceNotConfigured != 2 || farmer.EvidenceComplete != 0 {
		t.Fatalf("farmer evidence aggregate=%+v", farmer)
	}
	if farmer.SlotQuota != 10 || farmer.SlotsOpen != 1 || farmer.SlotsCompleted != 1 {
		t.Fatalf("farmer slot aggregate=%+v", farmer)
	}
	if farmer.DocumentCode != "WRT" || farmer.ProvinceName != "Sulawesi Selatan" {
		t.Fatalf("farmer region identity=%+v", farmer)
	}

	// Zone is only reported when a programme is selected (the zone belongs to a programme).
	if farmer.ZoneID != "" || farmer.ZoneCode != "" {
		t.Fatalf("no program_id: region must not carry a zone, got %+v", farmer)
	}
	zoned, err := repository.MapRegions(ctx, Filter{ProgramID: fixture.programID}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	zonedByID := map[string]MapRegion{}
	for _, region := range zoned.Regions {
		zonedByID[region.RegencyID] = region
	}
	if z := zonedByID[fixture.farmerRegencyID]; z.ZoneID != fixture.zoneID || z.ZoneCode != "ZONA-TEST" || z.ZoneName != "Zona Test Penerima" {
		t.Fatalf("with program_id the region must carry its zone, got %+v", z)
	}
	// A regency of the programme without an assignment (quota-only regency) has no zone.
	if z := zonedByID[fixture.otherRegencyID]; z.ZoneID != "" || z.ZoneCode != "" || z.ZoneName != "" {
		t.Fatalf("unassigned regency must have an empty zone, got %+v", z)
	}

	quotaOnly, ok := byID[fixture.otherRegencyID]
	if !ok {
		t.Fatalf("quota-only regency missing from map: %+v", data.Regions)
	}
	if quotaOnly.Recipients != 0 || quotaOnly.SlotQuota != 7 || quotaOnly.DocumentCode != "BRT" {
		t.Fatalf("quota-only aggregate=%+v", quotaOnly)
	}

	// Totals cover every regency in scope, so assert them through a regency-scoped call rather than
	// against the shared konkit_test seed data.
	scoped, err := repository.MapRegions(ctx, Filter{RegencyID: fixture.farmerRegencyID}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Regions) != 1 {
		t.Fatalf("regency-scoped map=%+v", scoped.Regions)
	}
	if scoped.Totals.Recipients != 2 || scoped.Totals.SlotQuota != 10 || scoped.Totals.SlotsCompleted != 1 || scoped.Totals.SlotsOpen != 1 {
		t.Fatalf("scoped totals=%+v", scoped.Totals)
	}

	// The evidence filter narrows recipients exactly as it does in the list. Slot progress is not an
	// evidence concept, so the regency still appears with its quota and slots but zero recipients.
	filtered, err := repository.MapRegions(ctx, Filter{RegencyID: fixture.farmerRegencyID, EvidenceStatus: "complete"}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Totals.Recipients != 0 || filtered.Totals.EvidenceComplete != 0 {
		t.Fatalf("evidence filter must drop every fixture allocation: %+v", filtered.Totals)
	}
	for _, region := range filtered.Regions {
		if region.RegencyID == fixture.farmerRegencyID && (region.Recipients != 0 || region.SlotQuota != 10) {
			t.Fatalf("filtered regency=%+v", region)
		}
	}

	// Regency scope must be enforced: a caller limited to another regency sees nothing here.
	outOfScope, err := repository.MapRegions(ctx, Filter{}, auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range outOfScope.Regions {
		if region.RegencyID == fixture.farmerRegencyID {
			t.Fatalf("out-of-scope regency leaked into the map: %+v", region)
		}
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

func TestListAllIgnoresTheNumericPageLimit(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)

	page, err := repository.List(context.Background(), Filter{
		Page: 1, PageSize: 1, All: true, ScheduleID: fixture.scheduleID,
	}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !page.All || page.Total != 2 || len(page.Items) != 2 || page.PageSize != 2 {
		t.Fatalf("expected all 2 active recipients, got %+v", page)
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

func TestListIncludesOrderedEvidenceSlotCompleteness(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	var distributionID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(allocation_id,schedule_id,slot_number,status) VALUES($1,$2,1,'linked') RETURNING id::text`, fixture.needsReviewAllocID, fixture.scheduleID).Scan(&distributionID))
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE id=$1`, distributionID); err != nil {
			t.Logf("cleanup: delete distribution slot failed: %v", err)
		}
	})

	var portraitSlotID, handoverSlotID, optionalSlotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'recipient_portrait','Foto penerima','penyerahan',true,1,2,'both','complete',10) RETURNING id::text`, distributionID).Scan(&portraitSlotID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'signed_handover','BAST bertanda tangan','penyerahan',true,1,1,'both','missing',20) RETURNING id::text`, distributionID).Scan(&handoverSlotID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'package_detail','Detail paket','penyerahan',false,1,2,'both','complete',30) RETURNING id::text`, distributionID).Scan(&optionalSlotID))

	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status) VALUES($1,gen_random_uuid(),'portrait.jpg','image/jpeg',128,$3,'gallery','accepted'),($2,gen_random_uuid(),'package.jpg','image/jpeg',128,$3,'gallery','accepted')`, portraitSlotID, optionalSlotID, checksum)
		return err
	}())

	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	var got *Recipient
	for i := range page.Items {
		if page.Items[i].AllocationID == fixture.needsReviewAllocID {
			got = &page.Items[i]
			break
		}
	}
	if got == nil {
		t.Fatal("expected seeded recipient in list")
	}
	if len(got.EvidenceSlots) != 3 {
		t.Fatalf("expected 3 ordered evidence slots, got %+v", got.EvidenceSlots)
	}
	wantCodes := []string{"recipient_portrait", "signed_handover", "package_detail"}
	for i, wantCode := range wantCodes {
		if got.EvidenceSlots[i].SlotCode != wantCode {
			t.Fatalf("slot %d: expected %q, got %+v", i, wantCode, got.EvidenceSlots[i])
		}
	}
	if !got.EvidenceSlots[0].Complete || got.EvidenceSlots[0].AcceptedFiles != 1 {
		t.Fatalf("expected first required slot complete with one file, got %+v", got.EvidenceSlots[0])
	}
	if got.EvidenceSlots[1].Complete || got.EvidenceSlots[1].AcceptedFiles != 0 {
		t.Fatalf("expected second required slot incomplete with no files, got %+v", got.EvidenceSlots[1])
	}
	if got.EvidenceSlots[2].IsRequired || !got.EvidenceSlots[2].Complete || got.EvidenceSlots[2].AcceptedFiles != 1 {
		t.Fatalf("expected optional slot visible and complete, got %+v", got.EvidenceSlots[2])
	}
}

func TestListAndStatsApplyTheSameCombinedScheduleDistrictAndEvidenceFilters(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE people p SET district='Sabbangparu' FROM package_allocations pa JOIN candidate_nominations cn ON cn.id=pa.nomination_id WHERE pa.id=$1 AND p.id=COALESCE(pa.actual_recipient_person_id,pa.intended_person_id,cn.person_id)`, fixture.needsReviewAllocID)
		return err
	}())
	var distributionID, completeSlotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(allocation_id,schedule_id,slot_number,status) VALUES($1,$2,1,'linked') RETURNING id::text`, fixture.needsReviewAllocID, fixture.scheduleID).Scan(&distributionID))
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE id=$1`, distributionID); err != nil {
			t.Logf("cleanup: delete distribution slot failed: %v", err)
		}
	})
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'portrait','Foto penerima','penyerahan',true,1,1,'both','complete',10) RETURNING id::text`, distributionID).Scan(&completeSlotID))
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'handover','BAST','penyerahan',true,1,1,'both','missing',20)`, distributionID)
		return err
	}())
	const checksum = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status) VALUES($1,gen_random_uuid(),'portrait.jpg','image/jpeg',128,$2,'gallery','accepted')`, completeSlotID, checksum)
		return err
	}())

	filter := Filter{Page: 1, PageSize: 20, ScheduleID: fixture.scheduleID, District: "Sabbangparu", EvidenceStatus: "partial"}
	page, err := repository.List(ctx, filter, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].AllocationID != fixture.needsReviewAllocID {
		t.Fatalf("expected one recipient matching all filters, got total=%d items=%+v", page.Total, page.Items)
	}

	stats, err := repository.Stats(ctx, filter, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 || stats.ByAllocationStatus["needs_review"] != 1 || stats.ByEvidenceStatus["partial"] != 1 {
		t.Fatalf("expected filtered stats to describe the same recipient set, got %+v", stats)
	}
}

func TestListSortsAllowlistedColumnsInBothDirections(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	scope := auth.RegencyScope{Unrestricted: true}

	ascending, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, ScheduleID: fixture.scheduleID, SortBy: "full_name", SortDirection: "asc"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(ascending.Items) != 2 || ascending.Items[0].FullName != "Distributed Person" || ascending.Items[1].FullName != "Needs Review Person" {
		t.Fatalf("unexpected ascending order: %+v", ascending.Items)
	}

	descending, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, ScheduleID: fixture.scheduleID, SortBy: "full_name", SortDirection: "desc"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(descending.Items) != 2 || descending.Items[0].FullName != "Needs Review Person" || descending.Items[1].FullName != "Distributed Person" {
		t.Fatalf("unexpected descending order: %+v", descending.Items)
	}
}

func TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	stats, err := repository.Stats(ctx, Filter{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ByAllocationStatus["distributed"] < 1 || stats.ByAllocationStatus["needs_review"] < 1 || stats.ByAllocationStatus["cancelled"] != 0 {
		t.Fatalf("expected active statuses and no cancelled recipients: %+v", stats.ByAllocationStatus)
	}
	_ = fixture.distributedAllocID
}

func TestCreateInsertsRecipientWithoutImportRowAndRejectsOutOfScopeSchedule(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}

	created, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Manual Recipient", NIK: "1234567890123456", SectorIdentifier: "KP-99",
		Address: "Jalan Test", Village: "Desa Test", District: "Kecamatan Test", PhoneNumber: "0812345678",
	}, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		var personID, nominationID string
		if err := pool.QueryRow(cleanupCtx, `
			SELECT COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)::text, pa.nomination_id::text
			FROM package_allocations pa JOIN candidate_nominations cn ON cn.id = pa.nomination_id WHERE pa.id = $1
		`, created.AllocationID).Scan(&personID, &nominationID); err != nil {
			t.Logf("cleanup: lookup created recipient failed: %v", err)
			return
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_allocations WHERE id = $1`, created.AllocationID); err != nil {
			t.Logf("cleanup: delete created package_allocations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM candidate_nominations WHERE id = $1`, nominationID); err != nil {
			t.Logf("cleanup: delete created candidate_nominations failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM people WHERE id = $1`, personID); err != nil {
			t.Logf("cleanup: delete created people failed: %v", err)
		}
	})
	if created.FullName != "Manual Recipient" || created.NIK != "1234567890123456" || created.SectorIdentifier != "KP-99" || created.AllocationStatus != "ready" {
		t.Fatalf("unexpected created recipient: %+v", created)
	}
	if created.DistributionNumber != nil {
		t.Fatalf("manual recipient must remain an unmounted DP3 candidate, got distribution number %d", *created.DistributionNumber)
	}

	var importRowID, batchID *string
	if err := pool.QueryRow(ctx, `
		SELECT cn.import_row_id::text, cn.batch_id::text FROM package_allocations pa JOIN candidate_nominations cn ON cn.id = pa.nomination_id WHERE pa.id = $1
	`, created.AllocationID).Scan(&importRowID, &batchID); err != nil {
		t.Fatal(err)
	}
	if importRowID != nil || batchID != nil {
		t.Fatalf("manual recipient must have null import linkage, got row=%v batch=%v", importRowID, batchID)
	}

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	if _, err := repository.Create(ctx, actor, CreateInput{ScheduleID: fixture.scheduleID, FullName: "Should Fail"}, meta, scoped); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for out-of-scope schedule, got %v", err)
	}
}

func TestUpdateChangesIdentityFieldsWithinScope(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}

	updated, err := repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{
		FullName: "Updated Name", NIK: "9999999999999999", Address: "Alamat Baru", Village: "Desa Baru", District: "Kecamatan Baru", PhoneNumber: "0899999999",
	}, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "Updated Name" || updated.NIK != "9999999999999999" || updated.Address != "Alamat Baru" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	if _, err := repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{FullName: "Nope"}, meta, scoped); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for out-of-scope update, got %v", err)
	}
}

func TestCancelHidesFromDefaultListAndRestoreReturnsToReady(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	if err := repository.Cancel(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); err != nil {
		t.Fatal(err)
	}
	if err := repository.Cancel(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); !errors.Is(err, ErrAlreadyCancelled) {
		t.Fatalf("expected ErrAlreadyCancelled, got %v", err)
	}

	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.AllocationID == fixture.needsReviewAllocID {
			t.Fatal("cancelled recipient must not appear in default list")
		}
	}

	if err := repository.Restore(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); err != nil {
		t.Fatal(err)
	}
	if err := repository.Restore(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); !errors.Is(err, ErrNotCancelled) {
		t.Fatalf("expected ErrNotCancelled, got %v", err)
	}
	restored, err := getRecipientByID(ctx, pool, fixture.needsReviewAllocID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AllocationStatus != "ready" {
		t.Fatalf("expected status ready after restore, got %q", restored.AllocationStatus)
	}
}

func TestRestoreReturnsReplacedRecipientToReady(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	unrestricted := auth.RegencyScope{Unrestricted: true}

	// Diganti dari nomor bagi: nomor dan penerima aktual sudah dilepas.
	if _, err := pool.Exec(ctx, `UPDATE package_allocations SET status='replaced', distribution_number=NULL, actual_recipient_person_id=NULL WHERE id=$1`, fixture.needsReviewAllocID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Restore(ctx, auth.Principal{}, fixture.needsReviewAllocID, auth.ClientMeta{UserAgent: "test"}, unrestricted); err != nil {
		t.Fatal(err)
	}
	restored, err := getRecipientByID(ctx, pool, fixture.needsReviewAllocID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AllocationStatus != "ready" {
		t.Fatalf("expected status ready after restoring a replaced recipient, got %q", restored.AllocationStatus)
	}
	if err := repository.Restore(ctx, auth.Principal{}, fixture.distributedAllocID, auth.ClientMeta{}, unrestricted); !errors.Is(err, ErrNotCancelled) {
		t.Fatalf("a distributed recipient must not be restored, got %v", err)
	}
}

func TestCancelRejectsAlreadyDistributedAllocation(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	if err := repository.Cancel(ctx, actor, fixture.distributedAllocID, meta, unrestricted); !errors.Is(err, ErrCancelNotAllowed) {
		t.Fatalf("expected ErrCancelNotAllowed for a distributed allocation, got %v", err)
	}

	recipient, err := getRecipientByID(ctx, pool, fixture.distributedAllocID)
	if err != nil {
		t.Fatal(err)
	}
	if recipient.AllocationStatus != "distributed" {
		t.Fatalf("expected status to remain distributed after rejected cancel, got %q", recipient.AllocationStatus)
	}
}

func TestCreateRejectsNIKAlreadyRegisteredToAnotherPerson(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	first, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "First Owner", NIK: "1111222233334444",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCreatedRecipient(t, pool, first.AllocationID) })

	_, err = repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Second Person", NIK: "1111222233334444",
	}, meta, unrestricted)
	if !errors.Is(err, ErrNIKInUse) {
		t.Fatalf("expected ErrNIKInUse for a duplicate NIK, got %v", err)
	}
}

func TestUpdateRejectsNIKAlreadyRegisteredToAnotherPerson(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	first, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Owner Of NIK", NIK: "5555666677778888",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCreatedRecipient(t, pool, first.AllocationID) })

	_, err = repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{
		FullName: "Needs Review Person", NIK: "5555666677778888",
	}, meta, unrestricted)
	if !errors.Is(err, ErrNIKInUse) {
		t.Fatalf("expected ErrNIKInUse when updating to a NIK owned by another person, got %v", err)
	}
}

func TestCreateRejectsSectorIdentifierAlreadyRegisteredToAnotherPerson(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	first, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "First Card Owner", SectorIdentifier: "KP-SHARED-001",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCreatedRecipient(t, pool, first.AllocationID) })

	_, err = repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Second Card Owner", SectorIdentifier: "KP-SHARED-001",
	}, meta, unrestricted)
	if !errors.Is(err, ErrSectorIdentifierInUse) {
		t.Fatalf("expected ErrSectorIdentifierInUse for a duplicate sector identifier on create, got %v", err)
	}
}

func TestUpdateRejectsSectorIdentifierAlreadyRegisteredToAnotherPerson(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	first, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Card Owner", SectorIdentifier: "KP-SHARED-002",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCreatedRecipient(t, pool, first.AllocationID) })

	_, err = repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{
		FullName: "Needs Review Person", SectorIdentifier: "KP-SHARED-002",
	}, meta, unrestricted)
	if !errors.Is(err, ErrSectorIdentifierInUse) {
		t.Fatalf("expected ErrSectorIdentifierInUse when updating to a sector identifier owned by another person, got %v", err)
	}
}

func cleanupCreatedRecipient(t *testing.T, pool *pgxpool.Pool, allocationID string) {
	t.Helper()
	cleanupCtx := context.Background()
	var personID, nominationID string
	if err := pool.QueryRow(cleanupCtx, `
		SELECT COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)::text, pa.nomination_id::text
		FROM package_allocations pa JOIN candidate_nominations cn ON cn.id = pa.nomination_id WHERE pa.id = $1
	`, allocationID).Scan(&personID, &nominationID); err != nil {
		t.Logf("cleanup: lookup created recipient failed: %v", err)
		return
	}
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM package_allocations WHERE id = $1`, allocationID); err != nil {
		t.Logf("cleanup: delete created package_allocations failed: %v", err)
	}
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM candidate_nominations WHERE id = $1`, nominationID); err != nil {
		t.Logf("cleanup: delete created candidate_nominations failed: %v", err)
	}
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM people WHERE id = $1`, personID); err != nil {
		t.Logf("cleanup: delete created people failed: %v", err)
	}
}
