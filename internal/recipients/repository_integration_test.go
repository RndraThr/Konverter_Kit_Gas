package recipients

import (
	"context"
	"errors"
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

func TestListIncludesOrderedEvidenceSlotCompleteness(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	var distributionID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,status) VALUES($1,'draft') RETURNING id::text`, fixture.needsReviewAllocID).Scan(&distributionID))
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE id=$1`, distributionID); err != nil {
			t.Logf("cleanup: delete distribution record failed: %v", err)
		}
	})

	var portraitSlotID, handoverSlotID, optionalSlotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'recipient_portrait','Foto penerima',true,1,2,'both','complete',10) RETURNING id::text`, distributionID).Scan(&portraitSlotID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'signed_handover','BAST bertanda tangan',true,1,1,'both','missing',20) RETURNING id::text`, distributionID).Scan(&handoverSlotID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'package_detail','Detail paket',false,1,2,'both','complete',30) RETURNING id::text`, distributionID).Scan(&optionalSlotID))

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
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,status) VALUES($1,'draft') RETURNING id::text`, fixture.needsReviewAllocID).Scan(&distributionID))
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE id=$1`, distributionID); err != nil {
			t.Logf("cleanup: delete distribution record failed: %v", err)
		}
	})
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'portrait','Foto penerima',true,1,1,'both','complete',10) RETURNING id::text`, distributionID).Scan(&completeSlotID))
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'handover','BAST',true,1,1,'both','missing',20)`, distributionID)
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
	if stats.ByAllocationStatus["distributed"] < 1 || stats.ByAllocationStatus["needs_review"] < 1 || stats.ByAllocationStatus["cancelled"] < 1 {
		t.Fatalf("expected all three statuses represented: %+v", stats.ByAllocationStatus)
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
