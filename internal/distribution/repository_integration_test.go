package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"
	mediastore "konkit/internal/media"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestCompleteEnforcesFinalDistributionRules(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := createDistributionFixture(t, pool)
	service := NewService(NewRepository(pool))
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.userID}
	meta := auth.ClientMeta{UserAgent: fixture.userAgent}

	unrestricted := auth.RegencyScope{Unrestricted: true}
	if _, err := service.Complete(ctx, actor, fixture.allocationID, meta, unrestricted); !errors.Is(err, ErrPreviouslyReceived) {
		t.Fatalf("previous receipt err=%v", err)
	}
	if _, err := service.Complete(ctx, actor, fixture.historyAllocationID, meta, unrestricted); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("already completed err=%v", err)
	}
	if _, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta, unrestricted); !errors.Is(err, ErrIdentityIncomplete) {
		t.Fatalf("identity err=%v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card','KP02','KP 02')`, fixture.secondaryPersonID); err != nil {
		t.Fatal(err)
	}
	var slotID string
	if err := pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) SELECT id,'recipient_package','Penerima dan paket',true,1,1,'both','missing' FROM distribution_records WHERE allocation_id=$1 RETURNING id::text`, fixture.secondaryAllocationID).Scan(&slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta, unrestricted); !errors.Is(err, ErrDocumentationIncomplete) {
		t.Fatalf("documentation err=%v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source) VALUES($1,gen_random_uuid(),'recipient.jpg','image/jpeg',100,$2,'gallery')`, slotID, fmt.Sprintf("%064d", time.Now().UnixNano())); err != nil {
		t.Fatal(err)
	}
	record, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta, unrestricted)
	if err != nil || record.Status != "completed" || record.CompletedAt.IsZero() {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	var allocationStatus, distributionStatus string
	var snapshot []byte
	if err := pool.QueryRow(ctx, `SELECT a.status,d.status,d.verification_snapshot_json FROM package_allocations a JOIN distribution_records d ON d.allocation_id=a.id WHERE a.id=$1`, fixture.secondaryAllocationID).Scan(&allocationStatus, &distributionStatus, &snapshot); err != nil {
		t.Fatal(err)
	}
	if allocationStatus != "distributed" || distributionStatus != "completed" || !strings.Contains(string(snapshot), "KP02") || !strings.Contains(string(snapshot), "recipient_package") {
		t.Fatalf("allocation=%q distribution=%q snapshot=%s", allocationStatus, distributionStatus, snapshot)
	}
	if _, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta, unrestricted); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("repeat completion err=%v", err)
	}
}

func TestCompleteSerializesConcurrentReceiptsForTheSamePerson(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := createDistributionFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card','KP02','KP 02')`, fixture.secondaryPersonID); err != nil {
		t.Fatal(err)
	}
	secondAllocationID := insertDistributionFixture(t, pool, fixture.scheduleID, fixture.secondaryPersonID, 9, "draft")
	for _, allocationID := range []string{fixture.secondaryAllocationID, secondAllocationID} {
		if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) SELECT id,'recipient_package','Penerima dan paket',true,0,1,'both','complete' FROM distribution_records WHERE allocation_id=$1`, allocationID); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(NewRepository(pool))
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, allocationID := range []string{fixture.secondaryAllocationID, secondAllocationID} {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()
			<-start
			_, err := service.Complete(ctx, auth.Principal{}, id, auth.ClientMeta{UserAgent: fixture.userAgent}, auth.RegencyScope{Unrestricted: true})
			errs <- err
		}(allocationID)
	}
	close(start)
	wait.Wait()
	close(errs)
	succeeded, blocked := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrPreviouslyReceived):
			blocked++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("succeeded=%d blocked=%d", succeeded, blocked)
	}
}

func TestIntegrationSaveDraftAndCompletePersistEquipmentFields(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := createDistributionFixture(t, pool)
	storage, err := mediastore.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(pool), storage)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.userID}
	meta := auth.ClientMeta{UserAgent: fixture.userAgent}

	// Use fixture.secondaryAllocationID (Siti Nur), not fixture.allocationID: createDistributionFixture
	// deliberately makes fixture.allocationID (Siti Aminah) "previously received" via historyAllocationID
	// in a different schedule, so Complete() on fixture.allocationID always returns ErrPreviouslyReceived
	// (see TestCompleteEnforcesFinalDistributionRules). secondaryAllocationID has no such history and has
	// no documentation_slots yet, so seed one directly satisfied (min_files=0) the same way the existing
	// TestCompleteSerializesConcurrentReceiptsForTheSamePerson test does.
	if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) SELECT id,'recipient_package','Penerima dan paket',true,0,1,'both','complete' FROM distribution_records WHERE allocation_id=$1`, fixture.secondaryAllocationID); err != nil {
		t.Fatal(err)
	}

	workspace, err := service.SaveDraft(ctx, actor, fixture.secondaryAllocationID, DraftInput{
		NIK: "7306014101900002", SectorIdentifier: "KP02",
		MachineOptionCode: "shark-spwp8030", MachineSerialNumber: "SP 06IABD 421291",
		HoseOptionCode: "triliunhose", HoseSerialNumber: "",
		ConverterSerialNumber: "240A005582",
	}, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.MachineOptionCode != "shark-spwp8030" || workspace.MachineSerialNumber != "SP 06IABD 421291" || workspace.ConverterSerialNumber != "240A005582" {
		t.Fatalf("workspace equipment=%+v", workspace)
	}

	record, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "completed" {
		t.Fatalf("record=%+v", record)
	}
	var snapshot string
	if err := pool.QueryRow(ctx, `SELECT verification_snapshot_json::text FROM distribution_records WHERE allocation_id=$1`, fixture.secondaryAllocationID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, `"machine_serial_number": "SP 06IABD 421291"`) || !strings.Contains(snapshot, `"converter_serial_number": "240A005582"`) {
		t.Fatalf("snapshot missing equipment data: %s", snapshot)
	}
}

func TestIntegrationSearchRanksIdentifiersAndShowsCrossScheduleHistory(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := createDistributionFixture(t, pool)
	storage, err := mediastore.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(pool), storage)
	ctx := context.Background()

	unrestricted := auth.RegencyScope{Unrestricted: true}
	byNumber, err := service.Search(ctx, fixture.scheduleID, "7", 50, unrestricted)
	if err != nil || len(byNumber) != 1 || byNumber[0].FullName != "Siti Aminah" {
		t.Fatalf("number search=%+v err=%v", byNumber, err)
	}
	if byNumber[0].MaskedNIK != "7306********0001" || byNumber[0].Eligibility != "previously_received" {
		t.Fatalf("sensitive or eligibility result=%+v", byNumber[0])
	}
	if len(byNumber[0].Documentation) != 2 || byNumber[0].Documentation[0].Status != "complete" || byNumber[0].Documentation[1].Status != "missing" {
		t.Fatalf("documentation=%+v", byNumber[0].Documentation)
	}

	byNIK, err := service.Search(ctx, fixture.scheduleID, "7306014101900001", 20, unrestricted)
	if err != nil || len(byNIK) == 0 || byNIK[0].AllocationID != fixture.allocationID {
		t.Fatalf("NIK search=%+v err=%v", byNIK, err)
	}
	byName, err := service.Search(ctx, fixture.scheduleID, "Siti", 20, unrestricted)
	if err != nil || len(byName) != 2 || byName[0].FullName != "Siti Aminah" {
		t.Fatalf("name search=%+v err=%v", byName, err)
	}

	workspace, err := service.GetWorkspace(ctx, fixture.allocationID, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Eligibility != "previously_received" || len(workspace.ReceiptHistory) != 1 {
		t.Fatalf("workspace=%+v", workspace)
	}
	if workspace.ReceiptHistory[0].Regency != "Bone Riwayat" || workspace.ReceiptHistory[0].Program != "Program Distribusi Test" {
		t.Fatalf("history=%+v", workspace.ReceiptHistory)
	}

	workspace, err = service.SaveDraft(ctx, auth.Principal{UserID: fixture.userID}, fixture.allocationID, DraftInput{
		NIK: "7306014101900001", Address: "Jalan Sawah 10", Village: "Tempe", District: "Sabbangparu",
		PhoneNumber: "0812-345", SectorIdentifier: "KP 01",
	}, auth.ClientMeta{UserAgent: fixture.userAgent}, unrestricted)
	if err != nil || workspace.PhoneNumber != "0812345" || workspace.SectorIdentifier != "KP01" {
		t.Fatalf("saved workspace=%+v err=%v", workspace, err)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, make([]byte, 32)...)
	media, err := service.UploadMedia(ctx, auth.Principal{}, UploadMediaInput{SlotID: workspace.Documentation[1].ID, OriginalFilename: "bast.jpg", Source: "gallery", Data: jpeg}, auth.ClientMeta{UserAgent: fixture.userAgent})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GetWorkspace(ctx, fixture.allocationID, unrestricted)
	if err != nil || workspace.Documentation[1].Status != "complete" || len(workspace.Documentation[1].Files) != 1 {
		t.Fatalf("uploaded slot=%+v err=%v", workspace.Documentation[1], err)
	}
	if err := service.DeleteMedia(ctx, auth.Principal{}, media.ID, auth.ClientMeta{UserAgent: fixture.userAgent}); err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GetWorkspace(ctx, fixture.allocationID, unrestricted)
	if err != nil || workspace.Documentation[1].Status != "missing" {
		t.Fatalf("deleted slot=%+v err=%v", workspace.Documentation[1], err)
	}
}

func TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := createDistributionFixture(t, pool)
	repository := NewRepository(pool)
	service := NewService(repository)

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}}
	otherRegencyOnly := auth.RegencyScope{RegencyIDs: []string{fixture.historyRegencyID}}

	results, err := service.Search(ctx, fixture.scheduleID, "Siti", 20, otherRegencyOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("out-of-scope search should return no matches, got %+v", results)
	}
	results, err = service.Search(ctx, fixture.scheduleID, "Siti", 20, scoped)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("in-scope search should return matches")
	}

	if _, err := service.GetWorkspace(ctx, fixture.allocationID, otherRegencyOnly); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound reading out-of-scope allocation, got %v", err)
	}
	if _, err := service.GetWorkspace(ctx, fixture.allocationID, scoped); err != nil {
		t.Fatalf("in-scope read should succeed: %v", err)
	}

	if _, err := service.SaveDraft(ctx, auth.Principal{}, fixture.allocationID, DraftInput{}, auth.ClientMeta{UserAgent: fixture.userAgent}, otherRegencyOnly); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound saving draft out of scope, got %v", err)
	}

	if _, err := service.Complete(ctx, auth.Principal{}, fixture.historyAllocationID, auth.ClientMeta{UserAgent: fixture.userAgent}, scoped); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound completing allocation from a different regency, got %v", err)
	}
}

type distributionFixture struct {
	scheduleID, allocationID, historyAllocationID, secondaryAllocationID string
	secondaryPersonID, userID, userAgent                                 string
	regencyID, historyRegencyID                                          string
}

func createDistributionFixture(t *testing.T, pool *pgxpool.Pool) distributionFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	userAgent := "distribution-integration-" + suffix
	var packageID, documentationID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM package_template_versions WHERE template_code='PETANI-LPG' AND version=1`).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code='DOK-PETANI' AND version=1`).Scan(&documentationID); err != nil {
		t.Fatal(err)
	}
	var programID, regencyID, historyRegencyID, scheduleID, historyScheduleID string
	if err := pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Distribusi Test','farmer',2026,'active') RETURNING id::text`, "DIST-"+suffix).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan',$1,$2) RETURNING id::text`, "Wajo Distribusi "+suffix, codeFromSuffix("W", suffix)).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan','Bone Riwayat',$1) RETURNING id::text`, codeFromSuffix("B", suffix)).Scan(&historyRegencyID); err != nil {
		t.Fatal(err)
	}
	insertSchedule := func(regency, name string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,$5,'2026-09-01','2026-09-30','active') RETURNING id::text`, programID, regency, packageID, documentationID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	scheduleID = insertSchedule(regencyID, "Wajo Tahap Test")
	historyScheduleID = insertSchedule(historyRegencyID, "Bone Tahap Lama")
	var sitiID, sitiNurID string
	if err := pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village,district) VALUES('Siti Aminah','7306014101900001','Tempe','Sabbangparu') RETURNING id::text`).Scan(&sitiID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village,district) VALUES('Siti Nur','7306014101900002','Pammana','Pammana') RETURNING id::text`).Scan(&sitiNurID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card','KP01','KP 01')`, sitiID); err != nil {
		t.Fatal(err)
	}
	allocationID := insertDistributionFixture(t, pool, scheduleID, sitiID, 7, "draft")
	secondaryAllocationID := insertDistributionFixture(t, pool, scheduleID, sitiNurID, 8, "draft")
	historyAllocationID := insertDistributionFixture(t, pool, historyScheduleID, sitiID, 1, "completed")
	if _, err := pool.Exec(ctx, `UPDATE distribution_records SET completed_at='2025-12-10T09:00:00Z',distributed_at='2025-12-10T09:00:00Z' WHERE allocation_id=$1`, historyAllocationID); err != nil {
		t.Fatal(err)
	}
	var distributionID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM distribution_records WHERE allocation_id=$1`, allocationID).Scan(&distributionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'recipient_package','Penerima dan paket',true,1,1,'both','complete',10),($1,'signed_bast','BAST bertanda tangan',true,1,1,'both','missing',20)`, distributionID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE user_agent=$1`, userAgent)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slots WHERE distribution_id IN (SELECT d.id FROM distribution_records d JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id IN ($1,$2))`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE allocation_id IN (SELECT id FROM package_allocations WHERE schedule_id IN ($1,$2))`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE schedule_id IN ($1,$2)`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id IN ($1,$2))`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id IN ($1,$2)`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id IN ($1,$2)`, scheduleID, historyScheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM person_sector_identifiers WHERE person_id IN ($1,$2)`, sitiID, sitiNurID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id IN ($1,$2)`, sitiID, sitiNurID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id IN ($1,$2)`, regencyID, historyRegencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
	})
	return distributionFixture{scheduleID: scheduleID, allocationID: allocationID, historyAllocationID: historyAllocationID, secondaryAllocationID: secondaryAllocationID, secondaryPersonID: sitiNurID, userAgent: userAgent, regencyID: regencyID, historyRegencyID: historyRegencyID}
}

func insertDistributionFixture(t *testing.T, pool *pgxpool.Pool, scheduleID, personID string, number int, status string) string {
	t.Helper()
	ctx := context.Background()
	var batchID, rowID, nominationID, allocationID string
	if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,status) VALUES($1,$2,$3,'Penerima','imported') RETURNING id::text`, scheduleID, fmt.Sprintf("fixture-%d.xlsx", number), fmt.Sprintf("%064d", time.Now().UnixNano()+int64(number))).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,source_sequence_number,raw_data_json,validation_status) VALUES($1,2,$2,'{}','valid') RETURNING id::text`, batchID, number).Scan(&rowID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO candidate_nominations(batch_id,import_row_id,person_id,program_type,source_snapshot_json,status) VALUES($1,$2,$3,'farmer','{"Nama":"Siti Aminah"}','ready') RETURNING id::text`, batchID, rowID, personID).Scan(&nominationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,'ready','{}') RETURNING id::text`, scheduleID, nominationID, personID, number).Scan(&allocationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id,status) VALUES($1,$2,$3)`, allocationID, personID, status); err != nil {
		t.Fatal(err)
	}
	return allocationID
}

func codeFromSuffix(prefix, suffix string) string {
	return prefix + string(rune('A'+suffix[len(suffix)-2]%20)) + string(rune('A'+suffix[len(suffix)-1]%20))
}

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
