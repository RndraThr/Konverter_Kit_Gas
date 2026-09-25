package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// distributionIntegrationPool provisions a migrated pool against TEST_DATABASE_URL for integration
// tests in this package. The old repository integration tests that used to live in this file
// (Search/GetWorkspace/SaveDraft/Complete against the now-dropped distribution_records table) were
// removed here to unblock compilation of the new POS Mesin unit tests; Task 8 rebuilds POS-shaped
// integration coverage in this file.
func distributionIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestCompleteSlotOpenSlotReturnsErrSlotNotLinked guards against the regression where CompleteSlot's
// locking query INNER JOINed package_allocations/people on ds.allocation_id/ds.recipient_person_id.
// A freshly-created 'open' slot (never linked at POS Dokumen) has both columns NULL, so the INNER
// JOINs silently dropped the row before status could be inspected, and pgx.ErrNoRows was
// misreported as ErrSlotNotFound instead of the correct ErrSlotNotLinked. The fix switched those to
// LEFT JOINs so the slot row is always locked and fetched regardless of link state.
func TestCompleteSlotOpenSlotReturnsErrSlotNotLinked(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Penyerahan Test','PYT',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Penyerahan','farmer',2026,'active') RETURNING id::text`, "PNY-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Penyerahan','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-PNY-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Penyerahan','farmer','published') RETURNING id::text`, "DOC-PNY-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Penyerahan','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	var slotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,1,'open') RETURNING id::text`, scheduleID).Scan(&slotID))

	repo := NewRepository(pool)
	_, err := repo.CompleteSlot(ctx, auth.Principal{}, CompleteSlotInput{ScheduleID: scheduleID, SlotNumber: 1}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrSlotNotLinked) {
		t.Fatalf("err = %v, want ErrSlotNotLinked", err)
	}
}

// TestCreateSlotSnapshotsDocumentationStage proves CreateSlot's snapshot INSERT and
// listSlotDocumentation's read-back SELECT both carry documentation_template_slots.stage through to
// SlotSummary.Stage on the live schema, not just that the query compiles.
func TestCreateSlotSnapshotsDocumentationStage(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Stage Test','STG',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Stage','farmer',2026,'active') RETURNING id::text`, "STG-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Stage','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-STG-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Stage','farmer','published') RETURNING id::text`, "DOC-STG-"+suffix).Scan(&docTemplateID))
	must(t, pool.QueryRow(ctx, `
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		VALUES($1,'foto-mesin','Foto Mesin','mesin',true,1,3,'both',false,false,1) RETURNING id::text
	`, docTemplateID).Scan(new(string)))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Stage','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	slot, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	must(t, err)

	if len(slot.Documentation) == 0 {
		t.Fatal("slot.Documentation is empty, want at least one snapshotted documentation slot")
	}
	var found bool
	for _, summary := range slot.Documentation {
		if summary.Stage == "mesin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no SlotSummary with Stage=mesin among %+v", slot.Documentation)
	}
}

// mediaFixture seeds a distribution_slot with a documentation_slot and one accepted media_files row,
// scoped under regencyID, plus a sibling otherRegencyID with no data — used to prove GetMediaSlot/
// GetMedia/DeleteMedia's re-scoped joins (documentation_slots -> distribution_slots ->
// program_schedules, replacing the dropped distribution_records -> package_allocations hop) both
// resolve real rows in scope and correctly reject out-of-scope regency access.
type mediaFixture struct {
	documentationSlotID string
	mediaID             string
	regencyID           string
	otherRegencyID      string
}

func seedMediaFixture(t *testing.T, pool *pgxpool.Pool) mediaFixture {
	t.Helper()
	ctx := context.Background()

	// Suffix every unique code/name with the test name so concurrent-within-run fixtures created by
	// this helper's several callers never collide on regencies_name_province_uq or similar unique
	// constraints (each test's rows are torn down via t.Cleanup below, but names must still be
	// distinct across tests that run in the same process before their cleanups fire).
	suffix := t.Name()

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,'MDT',true) RETURNING id::text`, "Media Test "+suffix).Scan(&regencyID))
	var otherRegencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,'MDO',true) RETURNING id::text`, "Media Test Other "+suffix).Scan(&otherRegencyID))

	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Media','farmer',2026,'active') RETURNING id::text`, "MED-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Media','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-MED-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Media','farmer','published') RETURNING id::text`, "DOC-MED-"+suffix).Scan(&docTemplateID))

	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Media','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	var distributionSlotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,1,'open') RETURNING id::text`, scheduleID).Scan(&distributionSlotID))

	var documentationSlotID string
	must(t, pool.QueryRow(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		VALUES($1,'foto-alat','Foto Alat','penyerahan',true,1,3,'both',false,false,1) RETURNING id::text
	`, distributionSlotID).Scan(&documentationSlotID))

	checksum := strings.Repeat("a", 64)
	var mediaID string
	must(t, pool.QueryRow(ctx, `
		INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status)
		VALUES($1,gen_random_uuid(),'foto.jpg','image/jpeg',1024,$2,'camera','accepted') RETURNING id::text
	`, documentationSlotID, checksum).Scan(&mediaID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM distribution_slots WHERE id = $1`, distributionSlotID); err != nil {
			t.Logf("cleanup: delete distribution_slots failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id = $1`, scheduleID); err != nil {
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
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id IN ($1,$2)`, regencyID, otherRegencyID); err != nil {
			t.Logf("cleanup: delete regencies failed: %v", err)
		}
	})

	return mediaFixture{documentationSlotID: documentationSlotID, mediaID: mediaID, regencyID: regencyID, otherRegencyID: otherRegencyID}
}

// TestGetMediaSlotScopedToDistributionSlots proves the re-scoped join
// (documentation_slots -> distribution_slots -> program_schedules) resolves a real documentation
// slot when the caller's regency scope includes the schedule's regency.
func TestGetMediaSlotScopedToDistributionSlots(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	slot, err := repo.GetMediaSlot(ctx, fixture.documentationSlotID, auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}})
	if err != nil {
		t.Fatal(err)
	}
	if slot.ID != fixture.documentationSlotID {
		t.Fatalf("slot.ID = %q, want %q", slot.ID, fixture.documentationSlotID)
	}
	if slot.AcceptedFiles != 1 {
		t.Fatalf("slot.AcceptedFiles = %d, want 1", slot.AcceptedFiles)
	}
}

// TestGetMediaSlotRejectsOutOfScopeRegency proves a caller scoped to a different regency cannot
// resolve a documentation slot belonging to another regency's schedule through the new join chain.
func TestGetMediaSlotRejectsOutOfScopeRegency(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	_, err := repo.GetMediaSlot(ctx, fixture.documentationSlotID, auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}})
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("err = %v, want ErrMediaNotFound", err)
	}
}

// TestGetMediaScopedToDistributionSlots proves GetMedia's re-scoped join resolves a real accepted
// media_files row when the caller's regency scope includes the schedule's regency.
func TestGetMediaScopedToDistributionSlots(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	item, err := repo.GetMedia(ctx, fixture.mediaID, auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != fixture.mediaID {
		t.Fatalf("item.ID = %q, want %q", item.ID, fixture.mediaID)
	}
	if item.SlotID != fixture.documentationSlotID {
		t.Fatalf("item.SlotID = %q, want %q", item.SlotID, fixture.documentationSlotID)
	}
}

// TestDeleteMediaRejectsOutOfScopeRegency proves DeleteMedia's re-scoped join refuses to mutate a
// media_files row belonging to another regency's schedule, and leaves the row untouched.
func TestDeleteMediaRejectsOutOfScopeRegency(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	_, err := repo.DeleteMedia(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}})
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("err = %v, want ErrMediaNotFound", err)
	}

	var status string
	must(t, pool.QueryRow(ctx, `SELECT status FROM media_files WHERE id=$1`, fixture.mediaID).Scan(&status))
	if status != "accepted" {
		t.Fatalf("status = %q, want accepted (out-of-scope delete must not mutate the row)", status)
	}
}

// TestDeleteMediaWithinScopeSucceeds proves DeleteMedia's re-scoped join still allows an in-scope
// caller to soft-delete a real media_files row end-to-end against the live schema.
func TestDeleteMediaWithinScopeSucceeds(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	deleted, err := repo.DeleteMedia(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != "deleted" {
		t.Fatalf("deleted.Status = %q, want deleted", deleted.Status)
	}
}

// TestLinkSlotReturnsRecipientIdentity proves getSlotByID's SELECT (the single read path every
// mutating POS method returns through) joins distribution_slots -> people on recipient_person_id and
// surfaces full_name/nik on the returned DistributionSlot. Without that join, DistributionSlot.FullName
// and .NIK are always empty even after a real link, and SlotDokumenSection's "Terhubung" summary
// silently renders a blank name/NIK for every linked/completed slot.
func TestLinkSlotReturnsRecipientIdentity(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Identity Test','IDT',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Identity','farmer',2026,'active') RETURNING id::text`, "IDT-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Identity','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-IDT-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Identity','farmer','published') RETURNING id::text`, "DOC-IDT-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Identity','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	const wantName = "Identity Candidate"
	// Generated rather than a fixed literal: internal/recipients's integration tests already use
	// fixed NIKs against the same shared konkit_test database, and go test ./... runs different
	// packages' tests in parallel by default -- a hardcoded NIK here would intermittently collide
	// with the unique index on people.nik (see internal/reports/repository_integration_test.go's
	// insertAllocation for the same rationale).
	wantNIK := fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)
	var personID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES($1,$2) RETURNING id::text`, wantName, wantNIK).Scan(&personID))
	var nominationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'candidate','{}'::jsonb) RETURNING id::text`, scheduleID, nominationID, personID).Scan(new(string)))

	repo := NewRepository(pool)
	created, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	must(t, err)

	linked, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: wantNIK}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if linked.FullName != wantName {
		t.Fatalf("linked.FullName = %q, want %q", linked.FullName, wantName)
	}
	if linked.NIK != wantNIK {
		t.Fatalf("linked.NIK = %q, want %q", linked.NIK, wantNIK)
	}
}
