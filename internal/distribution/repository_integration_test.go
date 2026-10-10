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
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Penyerahan','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

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
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,media_kind,sort_order)
		VALUES($1,'foto-mesin','Foto Mesin','mesin',true,1,3,'both',false,false,'image_video',1) RETURNING id::text
	`, docTemplateID).Scan(new(string)))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Stage','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	slot, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	if slot.DistributionDate != nil {
		t.Fatalf("new POS Mesin slot distribution_date=%v, want nil", slot.DistributionDate)
	}

	if len(slot.Documentation) == 0 {
		t.Fatal("slot.Documentation is empty, want at least one snapshotted documentation slot")
	}
	var found bool
	for _, summary := range slot.Documentation {
		if summary.Stage == "mesin" && summary.MediaKind == "image_video" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no SlotSummary with Stage=mesin and MediaKind=image_video among %+v", slot.Documentation)
	}
}

// TestCreateSlotRejectsScheduleOutsideCallerScope proves CreateSlot checks the target schedule's
// regency against the caller's RegencyScope before creating anything — a POS Mesin-permissioned
// officer scoped to one regency must not be able to create a distribution slot under a schedule
// belonging to a different regency just by knowing/guessing its schedule_id.
func TestCreateSlotRejectsScheduleOutsideCallerScope(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Scope Test','SCT',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var otherRegencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Scope Test Other','SCO',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&otherRegencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Scope','farmer',2026,'active') RETURNING id::text`, "SCT-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Scope','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-SCT-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Scope','farmer','published') RETURNING id::text`, "DOC-SCT-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Scope','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)

	// Caller scoped only to otherRegencyID — the schedule lives in regencyID — must be rejected.
	_, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{RegencyIDs: []string{otherRegencyID}}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("err=%v, want ErrScheduleRequired (schedule hidden outside caller scope)", err)
	}
	var slotCount int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM distribution_slots WHERE schedule_id=$1`, scheduleID).Scan(&slotCount))
	if slotCount != 0 {
		t.Fatalf("slot was created despite rejected scope: count=%d", slotCount)
	}

	// Caller scoped to the schedule's actual regency succeeds.
	created, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{RegencyIDs: []string{regencyID}}, auth.ClientMeta{})
	must(t, err)
	if created.SlotNumber != 1 {
		t.Fatalf("created.SlotNumber=%d, want 1", created.SlotNumber)
	}
}

// TestCreateSlotRejectsSlotBeyondQuota proves CreateSlot enforces program_schedules.slot_quota as a
// hard limit (spec: docs/superpowers/specs/2026-09-25-distribution-slot-catalog-design.md §4.2) —
// the (quota+1)th slot must be rejected, not silently created.
func TestCreateSlotRejectsSlotBeyondQuota(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Quota Test','QTA',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Quota','farmer',2026,'active') RETURNING id::text`, "QTA-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Quota','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-QTA-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Quota','farmer','published') RETURNING id::text`, "DOC-QTA-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json,slot_quota) VALUES($1,$2,$3,$4,'Jadwal Test Quota','2026-01-01','2026-12-31','active','{}'::jsonb,1) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	first, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	if first.SlotNumber != 1 {
		t.Fatalf("first.SlotNumber = %d, want 1", first.SlotNumber)
	}

	_, err = repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotQuotaExceeded) {
		t.Fatalf("err = %v, want ErrSlotQuotaExceeded", err)
	}
}

// TestCreateSlotHonoursExplicitSlotNumber proves the catalog-first flow: a caller may create a slot
// at an explicit number (any empty number within quota, not just max+1). Creating a second slot at a
// number already in use returns ErrSlotNumberTaken, and an explicit number above quota returns
// ErrSlotQuotaExceeded — while omitting the number still auto-allocates the next sequential one.
func TestCreateSlotHonoursExplicitSlotNumber(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Explicit Test','EXN',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Explicit','farmer',2026,'active') RETURNING id::text`, "EXN-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Explicit','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-EXN-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Explicit','farmer','published') RETURNING id::text`, "DOC-EXN-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json,slot_quota) VALUES($1,$2,$3,$4,'Jadwal Test Explicit','2026-01-01','2026-12-31','active','{}'::jsonb,5) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)

	// Explicit number 3 is creatable even though no lower-numbered slots exist yet.
	third, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID, SlotNumber: 3}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	if third.SlotNumber != 3 {
		t.Fatalf("third.SlotNumber = %d, want 3", third.SlotNumber)
	}

	// The same number cannot be created twice.
	_, err = repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID, SlotNumber: 3}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotNumberTaken) {
		t.Fatalf("err = %v, want ErrSlotNumberTaken", err)
	}

	// An explicit number above quota is rejected.
	_, err = repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID, SlotNumber: 6}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotQuotaExceeded) {
		t.Fatalf("err = %v, want ErrSlotQuotaExceeded", err)
	}

	// Omitting the number still auto-allocates the next sequential slot (max existing is 3 -> 4).
	auto, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	if auto.SlotNumber != 4 {
		t.Fatalf("auto.SlotNumber = %d, want 4", auto.SlotNumber)
	}
}

// TestListSlotCatalogReportsStatusAndCompleteness proves ListSlotCatalog returns one row per existing
// distribution_slots, with documentation_complete correctly reflecting whether every required
// documentation_slots row for that slot has met its min_files — using the same completeness rule as
// CompleteSlot (repository.go's incomplete check), just aggregated per slot instead of a single EXISTS.
func TestListSlotCatalogReportsStatusAndCompleteness(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Catalog Test','CTG',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Catalog','farmer',2026,'active') RETURNING id::text`, "CTG-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Catalog','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-CTG-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Catalog','farmer','published') RETURNING id::text`, "DOC-CTG-"+suffix).Scan(&docTemplateID))
	must(t, pool.QueryRow(ctx, `
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		VALUES($1,'foto-wajib','Foto Wajib','penyerahan',true,1,1,'both',false,false,1) RETURNING id::text
	`, docTemplateID).Scan(new(string)))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Catalog','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	incomplete, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	complete, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	must(t, pool.QueryRow(ctx, `UPDATE distribution_slots SET needs_recompletion=true,reopened_at=now(),reopened_stage='dokumen',revision_reason='catalog attention test' WHERE schedule_id=$1 AND slot_number=$2 RETURNING id::text`, scheduleID, incomplete.SlotNumber).Scan(new(string)))

	// Satisfy the "complete" slot's one required documentation_slots row with an accepted media file.
	var completeDocSlotID string
	must(t, pool.QueryRow(ctx, `SELECT id::text FROM documentation_slots WHERE distribution_slot_id=(SELECT id FROM distribution_slots WHERE schedule_id=$1 AND slot_number=$2)`, scheduleID, complete.SlotNumber).Scan(&completeDocSlotID))
	checksum := strings.Repeat("b", 64)
	must(t, pool.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status) VALUES($1,gen_random_uuid(),'f.jpg','image/jpeg',10,$2,'camera','accepted') RETURNING id::text`, completeDocSlotID, checksum).Scan(new(string)))

	entries, err := repo.ListSlotCatalog(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	must(t, err)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	byNumber := map[int]SlotCatalogEntry{}
	for _, entry := range entries {
		byNumber[entry.SlotNumber] = entry
	}
	if got := byNumber[incomplete.SlotNumber]; got.Status != "open" || got.DocumentationComplete || !got.NeedsRecompletion {
		t.Fatalf("incomplete slot entry = %+v, want status=open documentation_complete=false needs_recompletion=true", got)
	}
	if got := byNumber[complete.SlotNumber]; got.Status != "open" || !got.DocumentationComplete {
		t.Fatalf("complete slot entry = %+v, want status=open documentation_complete=true", got)
	}
	// The media upload is newer than the slot rows, so it drives last_activity_at.
	if byNumber[complete.SlotNumber].LastActivityAt.Before(byNumber[incomplete.SlotNumber].LastActivityAt) {
		t.Fatalf("last_activity_at: complete=%v should not be before incomplete=%v", byNumber[complete.SlotNumber].LastActivityAt, byNumber[incomplete.SlotNumber].LastActivityAt)
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
	distributionSlotID  string
	scheduleID          string
	programID           string
	packageTemplateID   string
	slotNumber          int
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
	suffix := strings.ReplaceAll(t.Name(), "/", "-")
	nextRegencyCode := func() string {
		var code string
		must(t, pool.QueryRow(ctx, `
			SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int)
			FROM generate_series(0,17575) AS g
			WHERE NOT EXISTS (
				SELECT 1 FROM regencies
				WHERE document_code=chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int)
			)
			ORDER BY g DESC LIMIT 1
		`).Scan(&code))
		return code
	}
	primaryCode := nextRegencyCode()
	suffix += "-" + primaryCode

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, "Media Test "+suffix+" "+primaryCode, primaryCode).Scan(&regencyID))
	secondaryCode := nextRegencyCode()
	var otherRegencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, "Media Test Other "+suffix+" "+secondaryCode, secondaryCode).Scan(&otherRegencyID))

	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Media','farmer',2026,'active') RETURNING id::text`, "MED-TEST-"+suffix).Scan(&programID))
	var zoneID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_zones(program_id,code,name,sort_order,is_placeholder) VALUES($1,'ZONE-1','Zona 1',1,false) RETURNING id::text`, programID).Scan(&zoneID))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3) RETURNING zone_id::text`, programID, regencyID, zoneID).Scan(&zoneID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Media','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-MED-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Media','farmer','published') RETURNING id::text`, "DOC-MED-"+suffix).Scan(&docTemplateID))

	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Media','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

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

	return mediaFixture{documentationSlotID: documentationSlotID, mediaID: mediaID, distributionSlotID: distributionSlotID, scheduleID: scheduleID, programID: programID, packageTemplateID: packageTemplateID, slotNumber: 1, regencyID: regencyID, otherRegencyID: otherRegencyID}
}

func addCandidateForMediaFixture(t *testing.T, pool *pgxpool.Pool, fixture mediaFixture) string {
	t.Helper()
	ctx := context.Background()
	nik := fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)
	var personID, nominationID, allocationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Queue Candidate',$1) RETURNING id::text`, nik).Scan(&personID))
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, personID).Scan(&nominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'candidate','{}') RETURNING id::text`, fixture.scheduleID, nominationID, personID).Scan(&allocationID))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE id=$1`, allocationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE id=$1`, nominationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1`, personID)
	})
	return nik
}

func TestIntegrationMediaMoveQueueDateFirstAndRecipientFirst(t *testing.T) {
	for _, order := range []string{"date-first", "recipient-first"} {
		t.Run(order, func(t *testing.T) {
			pool := distributionIntegrationPool(t)
			ctx := context.Background()
			fixture := seedMediaFixture(t, pool)
			nik := addCandidateForMediaFixture(t, pool, fixture)
			must(t, func() error {
				_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='staging' WHERE id=$1`, fixture.mediaID)
				return err
			}())
			repo := NewRepository(pool)
			dateInput := SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}
			linkInput := LinkSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, NIK: nik}

			if order == "date-first" {
				if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, dateInput, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := repo.LinkSlot(ctx, auth.Principal{}, linkInput, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
				t.Fatal(err)
			}
			var jobsBefore int
			must(t, pool.QueryRow(ctx, `SELECT count(*) FROM distribution_media_move_jobs WHERE media_file_id=$1`, fixture.mediaID).Scan(&jobsBefore))
			if jobsBefore != 0 {
				t.Fatalf("jobs before slot ready=%d, want 0", jobsBefore)
			}

			if order == "date-first" {
				if _, err := repo.LinkSlot(ctx, auth.Principal{}, linkInput, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, dateInput, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
				t.Fatal(err)
			}

			var state, jobStatus, targetFilename string
			var generation, jobGeneration int64
			var targetPath []string
			must(t, pool.QueryRow(ctx, `
				SELECT m.storage_state,m.storage_target_generation,j.status,j.target_generation,j.target_path,j.target_filename
				FROM media_files m JOIN distribution_media_move_jobs j ON j.media_file_id=m.id WHERE m.id=$1
			`, fixture.mediaID).Scan(&state, &generation, &jobStatus, &jobGeneration, &targetPath, &targetFilename))
			if state != "moving" || jobStatus != "queued" || generation != 1 || jobGeneration != 1 || !strings.HasSuffix(strings.Join(targetPath, "/"), "DOKUMENTASI (FOTO)/PENDISTRIBUSIAN/20 Oktober 2026/1") {
				t.Fatalf("state=%q job=%q generations=%d/%d target=%v", state, jobStatus, generation, jobGeneration, targetPath)
			}
			// The queued job must already carry the recipient-prefixed final filename, because only
			// the queue step knows both the recipient and the media's position in the slot.
			if targetFilename != "QUEUE CANDIDATE - FOTO ALAT - 01.jpg" {
				t.Fatalf("target filename=%q, want the recipient-prefixed final name", targetFilename)
			}

			stored, err := repo.SaveMedia(ctx, auth.Principal{}, MediaFileInput{
				SlotID: fixture.documentationSlotID, StorageKey: "ready-upload-" + order, OriginalFilename: "ready.jpg", MimeType: "image/jpeg",
				ByteSize: 128, Checksum: strings.Repeat("b", 64), Source: "camera", StorageState: "final",
			}, auth.ClientMeta{})
			if err != nil {
				t.Fatal(err)
			}
			var readyUploadJobs int
			must(t, pool.QueryRow(ctx, `SELECT count(*) FROM distribution_media_move_jobs WHERE media_file_id=$1`, stored.ID).Scan(&readyUploadJobs))
			if readyUploadJobs != 0 || stored.StorageState != "final" {
				t.Fatalf("ready upload jobs=%d state=%q", readyUploadJobs, stored.StorageState)
			}

			raced, err := repo.SaveMedia(ctx, auth.Principal{}, MediaFileInput{
				SlotID: fixture.documentationSlotID, StorageKey: "raced-upload-" + order, OriginalFilename: "raced.jpg", MimeType: "image/jpeg",
				ByteSize: 128, Checksum: strings.Repeat("c", 64), Source: "camera", StorageState: "staging",
			}, auth.ClientMeta{})
			if err != nil {
				t.Fatal(err)
			}
			var racedState, racedFilename string
			var racedJobs int
			must(t, pool.QueryRow(ctx, `SELECT storage_state FROM media_files WHERE id=$1`, raced.ID).Scan(&racedState))
			must(t, pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(target_filename),'') FROM distribution_media_move_jobs WHERE media_file_id=$1`, raced.ID).Scan(&racedJobs, &racedFilename))
			if racedState != "moving" || racedJobs != 1 {
				t.Fatalf("raced upload state=%q jobs=%d", racedState, racedJobs)
			}
			// The upload raced the slot becoming ready, so the job queued by SaveMedia must name the
			// file after the recipient too. Its position among the slot's accepted media is not fixed
			// because both test rows share a timestamp, so only the prefix is asserted here.
			if !strings.HasPrefix(racedFilename, "QUEUE CANDIDATE - FOTO ALAT - ") {
				t.Fatalf("raced upload target filename=%q, want the recipient-prefixed final name", racedFilename)
			}
		})
	}
}

func TestIntegrationDateChangeDuringMoveQueuesNewestGeneration(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := seedMediaFixture(t, pool)
	nik := addCandidateForMediaFixture(t, pool, fixture)
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='staging' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	repo := NewRepository(pool)
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, NIK: nik}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_media_move_jobs SET status='processing',locked_at=now() WHERE media_file_id=$1`, fixture.mediaID)
		return err
	}())
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-21"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	var mediaGeneration, jobGeneration int64
	var status string
	var target []string
	must(t, pool.QueryRow(ctx, `SELECT m.storage_target_generation,j.target_generation,j.status,j.target_path FROM media_files m JOIN distribution_media_move_jobs j ON j.media_file_id=m.id WHERE m.id=$1`, fixture.mediaID).Scan(&mediaGeneration, &jobGeneration, &status, &target))
	if mediaGeneration != 2 || jobGeneration != 2 || status != "queued" || !strings.HasSuffix(strings.Join(target, "/"), "/21 Oktober 2026/1") {
		t.Fatalf("mediaGen=%d jobGen=%d status=%q target=%v", mediaGeneration, jobGeneration, status, target)
	}
}

func TestIntegrationDeleteQueuedMediaCannotRetry(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := seedMediaFixture(t, pool)
	nik := addCandidateForMediaFixture(t, pool, fixture)
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='staging' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	repo := NewRepository(pool)
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, NIK: nik}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RetryMediaMove(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("cross-regency retry err=%v, want ErrMediaNotFound", err)
	}
	if _, err := repo.DeleteMedia(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	var jobs int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM distribution_media_move_jobs WHERE media_file_id=$1`, fixture.mediaID).Scan(&jobs))
	if jobs != 0 {
		t.Fatalf("queued job survived media deletion: %d", jobs)
	}
	if _, err := repo.RetryMediaMove(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("retry deleted media err=%v, want ErrMediaNotFound", err)
	}
}

func TestCompleteSlotRejectsFailedMediaMoveBeforePending(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := seedMediaFixture(t, pool)
	nik := addCandidateForMediaFixture(t, pool, fixture)
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='staging' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	repo := NewRepository(pool)
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	sectorIdentifier := fmt.Sprintf("KPMOVE%d", time.Now().UnixNano())
	if _, err := repo.LinkSlot(ctx, auth.Principal{}, LinkSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, NIK: nik, SectorIdentifier: sectorIdentifier}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompleteSlot(ctx, auth.Principal{}, CompleteSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaMovePending) {
		t.Fatalf("moving completion err=%v, want ErrMediaMovePending", err)
	}
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='move_failed',storage_last_error='drive timeout' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	if _, err := repo.CompleteSlot(ctx, auth.Principal{}, CompleteSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaMoveFailed) {
		t.Fatalf("failed completion err=%v, want ErrMediaMoveFailed", err)
	}
}

func TestSetDistributionDateRemainsEditableAfterMediaUpload(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	input := SetDistributionDateInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, DistributionDate: "2026-10-20"}

	updated, err := repo.SetDistributionDate(ctx, auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DistributionDate == nil || *updated.DistributionDate != "2026-10-20" {
		t.Fatalf("distribution_date=%v", updated.DistributionDate)
	}
	input.DistributionDate = "2026-10-21"
	updated, err = repo.SetDistributionDate(ctx, auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil || updated.DistributionDate == nil || *updated.DistributionDate != "2026-10-21" {
		t.Fatalf("second update=%+v err=%v", updated, err)
	}
}

func TestUpdateEquipmentIgnoresMediaFromOtherStages(t *testing.T) {
	// seedMediaFixture's one documentation_slots row is stage='penyerahan', so its
	// attached media must not lock POS Mesin's equipment fields — only media
	// recorded against the 'mesin' stage should.
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	input := UpdateEquipmentInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, MachineOptionCode: "shark-spwp8030", MachineSerialNumber: "msn-1", ConverterOptionCode: "ergas", ConverterSerialNumber: "cnv-1"}

	updated, err := repo.UpdateEquipment(ctx, auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MachineOptionCode != "shark-spwp8030" || updated.MachineSerialNumber != "msn-1" || updated.ConverterOptionCode != "ergas" || updated.ConverterSerialNumber != "cnv-1" {
		t.Fatalf("slot=%+v", updated)
	}
}

func TestUpdateEquipmentSerialsPreservesEquipmentOptions(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	ctx := context.Background()
	must(t, func() error {
		_, err := pool.Exec(ctx, `
			UPDATE distribution_slots
			SET machine_option_code='shark-spwp8030',converter_option_code='ergas',hose_option_code='hose-set'
			WHERE id=$1
		`, fixture.distributionSlotID)
		return err
	}())

	updated, err := NewRepository(pool).UpdateEquipmentSerials(ctx, auth.Principal{}, UpdateEquipmentSerialsInput{
		ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber,
		MachineSerialNumber: "MESIN-01", ConverterSerialNumber: "KONKIT-02",
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MachineOptionCode != "shark-spwp8030" || updated.ConverterOptionCode != "ergas" || updated.HoseOptionCode != "hose-set" {
		t.Fatalf("equipment options changed: %+v", updated)
	}
	if updated.MachineSerialNumber != "MESIN-01" || updated.ConverterSerialNumber != "KONKIT-02" {
		t.Fatalf("serials not updated: %+v", updated)
	}
}

func TestUpdateEquipmentAllowsMesinMediaUpload(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := t.Name()

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,'MQT',true) RETURNING id::text`, "Equipment Test "+suffix).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Equipment','farmer',2026,'active') RETURNING id::text`, "EQP-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Equipment','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-EQP-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Equipment','farmer','published') RETURNING id::text`, "DOC-EQP-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Equipment','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))
	var distributionSlotID string
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,machine_option_code) VALUES($1,1,'open','shark-spwp8030') RETURNING id::text`, scheduleID).Scan(&distributionSlotID))
	var documentationSlotID string
	must(t, pool.QueryRow(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		VALUES($1,'foto-mesin','Foto Mesin','mesin',true,1,1,'both',false,false,1) RETURNING id::text
	`, distributionSlotID).Scan(&documentationSlotID))
	var mediaID string
	must(t, pool.QueryRow(ctx, `
		INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status)
		VALUES($1,gen_random_uuid(),'foto.jpg','image/jpeg',1024,$2,'camera','accepted') RETURNING id::text
	`, documentationSlotID, strings.Repeat("c", 64)).Scan(&mediaID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM distribution_slots WHERE id = $1`, distributionSlotID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id = $1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM programs WHERE id = $1`, programID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM package_template_versions WHERE id = $1`, packageTemplateID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM documentation_template_versions WHERE id = $1`, docTemplateID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id = $1`, regencyID)
	})

	repo := NewRepository(pool)
	input := UpdateEquipmentInput{ScheduleID: scheduleID, SlotNumber: 1, MachineOptionCode: "yanmar-tf85", MachineSerialNumber: "msn-2"}

	updated, err := repo.UpdateEquipment(ctx, auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MachineOptionCode != "yanmar-tf85" || updated.MachineSerialNumber != "msn-2" {
		t.Fatalf("slot=%+v", updated)
	}
}

func TestReopenCompletedSlotRejectsOpenSlot(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	_, err := NewRepository(pool).ReopenSlot(context.Background(), auth.Principal{}, ReopenSlotInput{
		ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, Stage: "mesin", Reason: "Koreksi tanggal",
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrRevisionNotCompleted) {
		t.Fatalf("err=%v, want ErrRevisionNotCompleted", err)
	}
}

func TestReopenCompletedSlotResetsCompletionAndInvalidatesBA(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	ctx := context.Background()

	var personID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Penerima Revisi',$1) RETURNING id::text`, fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)).Scan(&personID))
	var nominationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, personID).Scan(&nominationID))
	var allocationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,actual_recipient_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$3,1,'distributed','{}') RETURNING id::text`, fixture.scheduleID, nominationID, personID).Scan(&allocationID))
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET allocation_id=$2,recipient_person_id=$3,status='completed',verification_snapshot_json='{"equipment":{"machine_serial":"OLD"}}',distributed_at=now(),completed_at=now() WHERE id=$1`, fixture.distributionSlotID, allocationID, personID)
		return err
	}())

	var individualID string
	must(t, pool.QueryRow(ctx, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,package_template_version_id,snapshot_json,revision,status) VALUES($1,$2,$3,'2026-10-07',1,1,'1/1/TEST',$4,'{}',1,'final') RETURNING id::text`, fixture.distributionSlotID, fixture.programID, fixture.regencyID, fixture.packageTemplateID).Scan(&individualID))
	var bundleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO bast_daily_bundles(program_id,regency_id,local_date,document_type,filename,recipient_count,page_count,checksum,version,status) VALUES($1,$2,'2026-10-07','individual','bundle.pdf',1,1,$3,1,'active') RETURNING id::text`, fixture.programID, fixture.regencyID, strings.Repeat("d", 64)).Scan(&bundleID))
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO bast_daily_bundle_items(bundle_id,individual_document_id,item_order,page_start,page_end) VALUES($1,$2,1,1,1)`, bundleID, individualID)
		return err
	}())
	var aggregateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO bast_aggregate_documents(schedule_id,program_id,regency_id,document_type,document_date,filename,recipient_count,page_count,version,status,checksum,snapshot_json) VALUES($1,$2,$3,'dp3','2026-10-07','dp3.pdf',1,1,1,'active',$4,'{}') RETURNING id::text`, fixture.scheduleID, fixture.programID, fixture.regencyID, strings.Repeat("e", 64)).Scan(&aggregateID))

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_daily_bundle_items WHERE bundle_id=$1`, bundleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_daily_bundles WHERE id=$1`, bundleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_individual_documents WHERE id=$1`, individualID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE id=$1`, aggregateID)
		_, _ = pool.Exec(context.Background(), `UPDATE distribution_slots SET allocation_id=NULL,recipient_person_id=NULL,status='open' WHERE id=$1`, fixture.distributionSlotID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE id=$1`, allocationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE id=$1`, nominationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1`, personID)
	})

	result, err := NewRepository(pool).ReopenSlot(ctx, auth.Principal{}, ReopenSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: 1, Stage: "dokumen", Reason: "Koreksi nomor seri"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "linked" || !result.NeedsRecompletion || result.ReopenedStage != "dokumen" || result.RevisionReason != "Koreksi nomor seri" || result.DistributedAt != nil {
		t.Fatalf("reopened slot=%+v", result)
	}
	var allocationStatus, individualStatus, bundleStatus, aggregateStatus string
	var snapshot string
	must(t, pool.QueryRow(ctx, `SELECT status FROM package_allocations WHERE id=$1`, allocationID).Scan(&allocationStatus))
	must(t, pool.QueryRow(ctx, `SELECT verification_snapshot_json::text FROM distribution_slots WHERE id=$1`, fixture.distributionSlotID).Scan(&snapshot))
	must(t, pool.QueryRow(ctx, `SELECT status FROM bast_individual_documents WHERE id=$1`, individualID).Scan(&individualStatus))
	must(t, pool.QueryRow(ctx, `SELECT status FROM bast_daily_bundles WHERE id=$1`, bundleID).Scan(&bundleStatus))
	must(t, pool.QueryRow(ctx, `SELECT status FROM bast_aggregate_documents WHERE id=$1`, aggregateID).Scan(&aggregateStatus))
	if allocationStatus != "ready" || snapshot != "{}" || individualStatus != "stale" || bundleStatus != "stale" || aggregateStatus != "stale" {
		t.Fatalf("allocation=%s snapshot=%s individual=%s bundle=%s aggregate=%s", allocationStatus, snapshot, individualStatus, bundleStatus, aggregateStatus)
	}

	// Reset only the operational rows, then prove the row lock serializes two
	// officers attempting to reopen the same completed slot.
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE package_allocations SET status='distributed' WHERE id=$1`, allocationID)
		return err
	}())
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='completed',needs_recompletion=false,distributed_at=now(),completed_at=now() WHERE id=$1`, fixture.distributionSlotID)
		return err
	}())
	errorsCh := make(chan error, 2)
	for range 2 {
		go func() {
			_, reopenErr := NewRepository(pool).ReopenSlot(ctx, auth.Principal{}, ReopenSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: 1, Stage: "dokumen", Reason: "Koreksi bersamaan"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
			errorsCh <- reopenErr
		}()
	}
	succeeded, conflicted := 0, 0
	for range 2 {
		reopenErr := <-errorsCh
		switch {
		case reopenErr == nil:
			succeeded++
		case errors.Is(reopenErr, ErrRevisionNotCompleted):
			conflicted++
		default:
			t.Fatalf("concurrent reopen err=%v", reopenErr)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent reopen succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestSaveMediaAcceptsOpaqueGoogleDriveStorageKey(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)

	stored, err := repo.SaveMedia(context.Background(), auth.Principal{}, MediaFileInput{
		SlotID:           fixture.documentationSlotID,
		StorageKey:       "1HcRnPZPMu_tXXDRDDuYuAqyLAMFJNvdr",
		OriginalFilename: "foto.jpg",
		MimeType:         "image/jpeg",
		ByteSize:         1024,
		Checksum:         strings.Repeat("b", 64),
		Source:           "camera",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if stored.StorageKey != "1HcRnPZPMu_tXXDRDDuYuAqyLAMFJNvdr" {
		t.Fatalf("storage key = %q", stored.StorageKey)
	}
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
	if slot.SlotNumber != 1 || slot.Label != "Foto Alat" {
		t.Fatalf("slot identity = number %d label %q", slot.SlotNumber, slot.Label)
	}
	if slot.ProgramType != "farmer" || slot.ZoneName != "Zona 1" || !strings.HasPrefix(slot.RegencyName, "Media Test "+strings.ReplaceAll(t.Name(), "/", "-")) {
		t.Fatalf("slot storage context = %+v", slot)
	}
	documentationStage, err := repo.DocumentationSlotStage(ctx, fixture.documentationSlotID, auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}})
	if err != nil || documentationStage != "penyerahan" {
		t.Fatalf("documentation stage=%q err=%v", documentationStage, err)
	}
	mediaStage, err := repo.MediaStage(ctx, fixture.mediaID, auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}})
	if err != nil || mediaStage != "penyerahan" {
		t.Fatalf("media stage=%q err=%v", mediaStage, err)
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

func TestDeleteMediaRejectsCompletedDistributionSlot(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	ctx := context.Background()
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='completed' WHERE id=$1`, fixture.distributionSlotID)
		return err
	}())
	_, err := NewRepository(pool).DeleteMedia(ctx, auth.Principal{}, fixture.mediaID, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("err=%v, want ErrAlreadyCompleted", err)
	}
	var status string
	must(t, pool.QueryRow(ctx, `SELECT status FROM media_files WHERE id=$1`, fixture.mediaID).Scan(&status))
	if status != "accepted" {
		t.Fatalf("media status=%q", status)
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
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Identity','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

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
	created, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
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

	sectorID := "KP" + suffix
	replacementSectorID := "KR" + suffix
	updated, err := repo.UpdateRecipient(ctx, auth.Principal{}, UpdateRecipientInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, Address: "JL. BARU", Village: "DESA BARU", District: "WAJO", PhoneNumber: "081234567", SectorIdentifier: sectorID}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.NIK != wantNIK || updated.Address != "JL. BARU" || updated.SectorIdentifier != sectorID {
		t.Fatalf("updated recipient=%+v", updated)
	}

	newNIK := fmt.Sprintf("%016d", (time.Now().UnixNano()+1)%1e16)
	var newPersonID, newNominationID, newAllocationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Replacement Candidate',$1) RETURNING id::text`, newNIK).Scan(&newPersonID))
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, newPersonID).Scan(&newNominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'candidate','{}') RETURNING id::text`, scheduleID, newNominationID, newPersonID).Scan(&newAllocationID))
	replaced, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: newNIK, Address: "JL. PENGGANTI", SectorIdentifier: replacementSectorID, Reason: "Penerima awal tidak dapat hadir"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.NIK != newNIK || replaced.Address != "JL. PENGGANTI" || replaced.SectorIdentifier != replacementSectorID {
		t.Fatalf("replaced recipient=%+v", replaced)
	}
	var oldAllocationStatus string
	var oldDistributionNumber *int
	must(t, pool.QueryRow(ctx, `SELECT status,distribution_number FROM package_allocations WHERE nomination_id=$1`, nominationID).Scan(&oldAllocationStatus, &oldDistributionNumber))
	// The replaced allocation is released from the slot and marked replaced, so Data Penerima and
	// reports keep showing it as history instead of silently returning it to the candidate pool.
	if oldAllocationStatus != "replaced" || oldDistributionNumber != nil {
		t.Fatalf("old allocation status=%q number=%v", oldAllocationStatus, oldDistributionNumber)
	}
	var recordedReason, recordedOrigin string
	must(t, pool.QueryRow(ctx, `SELECT reason,origin FROM recipient_replacements WHERE distribution_slot_id=$1`, created.ID).Scan(&recordedReason, &recordedOrigin))
	if recordedReason != "Penerima awal tidak dapat hadir" || recordedOrigin != "existing_allocation" {
		t.Fatalf("replacement history reason=%q origin=%q", recordedReason, recordedOrigin)
	}

	conflictNIK := fmt.Sprintf("%016d", (time.Now().UnixNano()+2)%1e16)
	var conflictPersonID, conflictNominationID, conflictAllocationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Used Candidate',$1) RETURNING id::text`, conflictNIK).Scan(&conflictPersonID))
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, conflictPersonID).Scan(&conflictNominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,actual_recipient_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$3,99,'ready','{}') RETURNING id::text`, scheduleID, conflictNominationID, conflictPersonID).Scan(&conflictAllocationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id) VALUES($1,99,'linked',$2,$3) RETURNING id::text`, scheduleID, conflictAllocationID, conflictPersonID).Scan(new(string)))
	beforeAllocation := replaced.AllocationID
	_, err = repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: conflictNIK, Reason: "Coba pakai kandidat yang sudah terpasang"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrCandidateAlreadyAssigned) {
		t.Fatalf("conflict err=%v, want ErrCandidateAlreadyAssigned", err)
	}
	afterConflict, err := repo.SearchSlot(ctx, scheduleID, fmt.Sprint(created.SlotNumber), auth.RegencyScope{Unrestricted: true})
	if err != nil || afterConflict.AllocationID == nil || beforeAllocation == nil || *afterConflict.AllocationID != *beforeAllocation || afterConflict.NIK != newNIK {
		t.Fatalf("replacement rollback slot=%+v err=%v", afterConflict, err)
	}

	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='completed' WHERE id=$1`, created.ID)
		return err
	}())
	if _, err := repo.UpdateRecipient(ctx, auth.Principal{}, UpdateRecipientInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, Address: "DITOLAK"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("completed update err=%v", err)
	}
	if _, err := repo.ReplaceRecipient(ctx, auth.Principal{}, ReplaceRecipientInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, NIK: wantNIK, Reason: "Percobaan pada slot selesai"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("completed replace err=%v", err)
	}
	if _, err := repo.SetDistributionDate(ctx, auth.Principal{}, SetDistributionDateInput{ScheduleID: scheduleID, SlotNumber: created.SlotNumber, DistributionDate: "2026-10-22"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("completed date update err=%v", err)
	}
}

// TestFindSerialMatches proves duplicates are found case-insensitively across
// schedules, the edited slot is excluded, and cancelled slots are ignored.
func TestFindSerialMatches(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Serial Test','SRL',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Serial','farmer',2026,'active') RETURNING id::text`, "SRL-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Serial','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-SRL-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Serial','farmer','published') RETURNING id::text`, "DOC-SRL-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Serial','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	first, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	cancelled, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	must(t, err)
	machine := "MS-DUP-" + suffix
	converter := "CV-DUP-" + suffix
	_, err = pool.Exec(ctx, `UPDATE distribution_slots SET machine_serial_number=$1, converter_serial_number=$2 WHERE schedule_id=$3 AND slot_number=$4`, machine, converter, scheduleID, first.SlotNumber)
	must(t, err)
	_, err = pool.Exec(ctx, `UPDATE distribution_slots SET machine_serial_number=$1, status='cancelled' WHERE schedule_id=$2 AND slot_number=$3`, machine, scheduleID, cancelled.SlotNumber)
	must(t, err)

	matches, err := repo.FindSerialMatches(ctx, strings.ToUpper(machine), "", 0)
	must(t, err)
	if len(matches) != 1 || matches[0].SlotNumber != first.SlotNumber || matches[0].Field != "machine" || matches[0].RegencyName != "Serial Test" {
		t.Fatalf("machine matches = %+v, want only slot %d as machine", matches, first.SlotNumber)
	}
	matches, err = repo.FindSerialMatches(ctx, strings.ToUpper(converter), "", 0)
	must(t, err)
	if len(matches) != 1 || matches[0].Field != "converter" {
		t.Fatalf("converter matches = %+v, want one converter match", matches)
	}
	matches, err = repo.FindSerialMatches(ctx, strings.ToUpper(machine), scheduleID, first.SlotNumber)
	must(t, err)
	if len(matches) != 0 {
		t.Fatalf("excluded slot still matched: %+v", matches)
	}
}

// TestMobileSyncDelta proves the sync endpoints return everything on a full
// sync, nothing for a cursor in the future, and changed rows after an update.
func TestMobileSyncDelta(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Sync Test','SYN',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Sync','farmer',2026,'active') RETURNING id::text`, "SYN-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Sync','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-SYN-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Sync','farmer','published') RETURNING id::text`, "DOC-SYN-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Sync','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	nik := fmt.Sprintf("%016d", (time.Now().UnixNano()+7)%1e16)
	var personID, nominationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Sync Candidate',$1) RETURNING id::text`, nik).Scan(&personID))
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'candidate','{}'::jsonb) RETURNING id::text`, scheduleID, nominationID, personID).Scan(new(string)))

	repo := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}
	slot, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, scope, auth.ClientMeta{})
	must(t, err)

	full, err := repo.SyncSlots(ctx, scheduleID, nil, scope)
	must(t, err)
	if len(full.Items) != 1 || full.Items[0].SlotNumber != slot.SlotNumber || full.ServerTime.IsZero() {
		t.Fatalf("full slot sync = %+v", full)
	}
	candidates, err := repo.SyncCandidates(ctx, scheduleID, nil, scope)
	must(t, err)
	if len(candidates.Items) != 1 || candidates.Items[0].NIK != nik || candidates.Items[0].Linked {
		t.Fatalf("full candidate sync = %+v", candidates.Items)
	}

	cursor := full.ServerTime.Add(time.Second)
	empty, err := repo.SyncSlots(ctx, scheduleID, &cursor, scope)
	must(t, err)
	if len(empty.Items) != 0 {
		t.Fatalf("delta before change = %d items, want 0", len(empty.Items))
	}
	// A later change on the slot shows up in the next delta.
	_, err = pool.Exec(ctx, `UPDATE distribution_slots SET updated_at = $1 WHERE schedule_id=$2 AND slot_number=$3`, cursor.Add(time.Minute), scheduleID, slot.SlotNumber)
	must(t, err)
	delta, err := repo.SyncSlots(ctx, scheduleID, &cursor, scope)
	must(t, err)
	if len(delta.Items) != 1 {
		t.Fatalf("delta after change = %d items, want 1", len(delta.Items))
	}
}

func TestMyActivityIsPerUser(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Activity Test','ACT',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Activity','farmer',2026,'active') RETURNING id::text`, "ACT-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Activity','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-ACT-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Activity','farmer','published') RETURNING id::text`, "DOC-ACT-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Activity','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	newUser := func(name string) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash,full_name) VALUES($1,$1||'@konkit.test','x',$1) RETURNING id::text`, name+"-"+suffix).Scan(&id))
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
		return id
	}
	alice, bob := newUser("activity.alice"), newUser("activity.bob")

	repo := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}
	first, err := repo.CreateSlot(ctx, auth.Principal{UserID: alice}, CreateSlotInput{ScheduleID: scheduleID}, scope, auth.ClientMeta{})
	must(t, err)
	second, err := repo.CreateSlot(ctx, auth.Principal{UserID: bob}, CreateSlotInput{ScheduleID: scheduleID}, scope, auth.ClientMeta{})
	must(t, err)
	third, err := repo.CreateSlot(ctx, auth.Principal{UserID: alice}, CreateSlotInput{ScheduleID: scheduleID}, scope, auth.ClientMeta{})
	must(t, err)

	mine, err := repo.MyActivity(ctx, alice, scheduleID, scope)
	must(t, err)
	if len(mine) != 2 || mine[0].SlotNumber != third.SlotNumber || mine[1].SlotNumber != first.SlotNumber {
		t.Fatalf("alice activity = %+v, want slots %d then %d", mine, third.SlotNumber, first.SlotNumber)
	}
	theirs, err := repo.MyActivity(ctx, bob, scheduleID, scope)
	must(t, err)
	if len(theirs) != 1 || theirs[0].SlotNumber != second.SlotNumber {
		t.Fatalf("bob activity = %+v, want slot %d", theirs, second.SlotNumber)
	}
	// Outside the caller's regencies nothing is returned.
	none, err := repo.MyActivity(ctx, alice, scheduleID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}})
	must(t, err)
	if len(none) != 0 {
		t.Fatalf("out-of-scope activity = %+v", none)
	}
}
