package dcp3

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/xuri/excelize/v2"
)

func TestIntegrationCommitImportsIdentitiesAllocationsAndDocumentation(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	nikSeed := time.Now().UnixNano() % 100000000000000
	validNIK := fmt.Sprintf("73%014d", nikSeed)
	conflictingNIK := fmt.Sprintf("73%014d", (nikSeed+1)%100000000000000)
	validCard := fmt.Sprintf("KP-%014d", nikSeed)
	duplicateCard := fmt.Sprintf("KP-%014d", (nikSeed+2)%100000000000000)
	groupNIK1 := fmt.Sprintf("73%014d", (nikSeed+3)%100000000000000)
	groupNIK2 := fmt.Sprintf("73%014d", (nikSeed+4)%100000000000000)
	// DCP3 files often fill the card column with a group letter number shared by several farmers.
	groupLetter := fmt.Sprintf("140/ /19.01.%d/2024", nikSeed)
	scheduleID, conflictingPersonID, _ := createDCP3ScheduleFixture(t, pool, validNIK, conflictingNIK, groupNIK1, groupNIK2)
	repository := NewRepository(pool)
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-integration-test"}
	workbook := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{
			{"No", "Nama", "NIK", "No Kartu Petani", "Alamat", "Desa", "Kecamatan", "No HP"},
			{1, "Siti Aminah", validNIK, validCard, "Jalan Sawah", "Tempe", "Sabbangparu", "08121"},
			{1, "Siti Duplikat", validNIK, duplicateCard, "Jalan Dua", "Tempe", "Sabbangparu", "08122"},
			{3, "Hasan Konflik", conflictingNIK, "KP-99", "Jalan Tiga", "Tempe", "Sabbangparu", "08123"},
			{4, "Ani Kelompok", groupNIK1, groupLetter, "Jalan Empat", "Tempe", "Sabbangparu", "08124"},
			{5, "Budi Kelompok", groupNIK2, groupLetter, "Jalan Lima", "Tempe", "Sabbangparu", "08125"},
		}
		for index, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
		}
	})

	unrestricted := auth.RegencyScope{Unrestricted: true}
	preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3-petani.xlsx", bytes.NewReader(workbook), meta, unrestricted, 1)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ProgramType != "farmer" || len(preview.Rows) != 5 || len(preview.Headers) != 8 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	// Reading the same file again while it is only a draft replaces the draft.
	reread, err := service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3-petani-copy.xlsx", bytes.NewReader(workbook), meta, unrestricted, 1)
	if err != nil {
		t.Fatalf("re-reading a draft file: %v", err)
	}
	if _, err := service.GetPreview(ctx, preview.ID, unrestricted); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("old draft should be replaced, got %v", err)
	}
	preview = reread

	result, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{
		FullName: "Nama", NIK: "NIK", FarmerCardNumber: "No Kartu Petani",
		Address: "Alamat", Village: "Desa", District: "Kecamatan", PhoneNumber: "No HP",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	// Hasan Konflik and Budi Kelompok carry a card that someone else also holds: a card number
	// may be shared, so they are valid like the rest. The duplicated NIK needs review and adds
	// no second allocation.
	if result.TotalRows != 5 || result.ValidRows != 4 || result.WarningRows != 0 || result.ReviewRows != 1 || result.InvalidRows != 0 ||
		result.CreatedRows != 4 || result.SkippedRows != 1 || result.UpdatedRows != 0 || result.KeptRows != 0 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	problemRows := map[int]RowStatus{}
	for _, problem := range result.Problems {
		problemRows[problem.SourceRowNumber] = problem.Status
	}
	if len(problemRows) != 1 || problemRows[3] != RowNeedsReview {
		t.Fatalf("unexpected problems: %+v", result.Problems)
	}
	// Once imported, the same file is refused for this schedule.
	if _, err := service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3-petani-again.xlsx", bytes.NewReader(workbook), meta, unrestricted, 1); !errors.Is(err, ErrDuplicateImport) {
		t.Fatalf("expected duplicate import after commit, got %v", err)
	}
	committedPreview, err := service.GetPreview(ctx, preview.ID, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if committedPreview.Status != "imported" || len(committedPreview.Headers) != 8 {
		t.Fatalf("committed preview lost its source metadata: %+v", committedPreview)
	}

	var allocationCount, numberedAllocations int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(distribution_number) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&allocationCount, &numberedAllocations); err != nil {
		t.Fatal(err)
	}
	if allocationCount != 4 || numberedAllocations != 0 {
		t.Fatalf("allocations=%d numbered=%d (import must leave distribution_number NULL)", allocationCount, numberedAllocations)
	}
	var slotCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM distribution_slots WHERE schedule_id=$1`, scheduleID).Scan(&slotCount); err != nil {
		t.Fatal(err)
	}
	if slotCount != 0 {
		t.Fatalf("import must not create distribution_slots rows, got %d", slotCount)
	}

	var reviewRows, packageSnapshots int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dcp3_import_rows WHERE batch_id=$1 AND validation_status='needs_review'`, preview.ID).Scan(&reviewRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1 AND package_snapshot_json->'selected_machine'->>'brand'='SHARK'`, scheduleID).Scan(&packageSnapshots); err != nil {
		t.Fatal(err)
	}
	if reviewRows != 1 || packageSnapshots != 4 {
		t.Fatalf("review_rows=%d package_snapshots=%d", reviewRows, packageSnapshots)
	}

	var earlierOwnerKeepsCard bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM person_sector_identifiers WHERE identifier_type='farmer_card' AND normalized_value='KP99' AND person_id=$1)`, conflictingPersonID).Scan(&earlierOwnerKeepsCard); err != nil {
		t.Fatal(err)
	}
	if !earlierOwnerKeepsCard {
		t.Fatal("the earlier holder of KP-99 lost the card")
	}

	// Every row with a usable NIK reaches a person and a ready allocation, and each keeps the
	// card number from the file, shared or not.
	var readyWithPerson int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1 AND status='ready' AND intended_person_id IS NOT NULL`, scheduleID).Scan(&readyWithPerson); err != nil {
		t.Fatal(err)
	}
	if readyWithPerson != 4 {
		t.Fatalf("ready allocations with a person = %d, want 4", readyWithPerson)
	}
	cards := map[string]string{}
	cardRows, err := pool.Query(ctx, `SELECT p.nik, COALESCE(i.display_value,'') FROM people p LEFT JOIN person_sector_identifiers i ON i.person_id=p.id WHERE p.nik = ANY($1)`, []string{conflictingNIK, groupNIK1, groupNIK2})
	if err != nil {
		t.Fatal(err)
	}
	for cardRows.Next() {
		var nik, card string
		if err := cardRows.Scan(&nik, &card); err != nil {
			t.Fatal(err)
		}
		cards[nik] = card
	}
	if err := cardRows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 || cards[conflictingNIK] != "KP-99" || cards[groupNIK1] != groupLetter || cards[groupNIK2] != groupLetter {
		t.Fatalf("people and cards = %+v", cards)
	}
}

// A revised DCP3 file imported into the same schedule overwrites recipients
// whose distribution has not started, keeps the ones already in progress and
// adds new ones, without listing anyone twice.
func TestIntegrationReimportOverwritesRecipientsWithoutProgress(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	seed := time.Now().UnixNano() % 100000000000000
	nikA := fmt.Sprintf("74%014d", seed)
	nikB := fmt.Sprintf("74%014d", (seed+1)%100000000000000)
	nikE := fmt.Sprintf("74%014d", (seed+2)%100000000000000)
	nikD := fmt.Sprintf("74%014d", (seed+3)%100000000000000)
	shortNIK := fmt.Sprintf("74%013d", seed%10000000000000)
	scheduleID, _, _ := createDCP3ScheduleFixture(t, pool, nikA, nikB, nikE, nikD)
	t.Cleanup(func() {
		ctx := context.Background()
		rows, _ := pool.Query(ctx, `SELECT DISTINCT COALESCE(pa.intended_person_id, cn.person_id)::text FROM package_allocations pa JOIN candidate_nominations cn ON cn.id=pa.nomination_id WHERE pa.schedule_id=$1 AND COALESCE(pa.intended_person_id, cn.person_id) IS NOT NULL`, scheduleID)
		people := []string{}
		for rows != nil && rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				people = append(people, id)
			}
		}
		_, _ = pool.Exec(ctx, `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(ctx, `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(ctx, `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(ctx, `DELETE FROM person_sector_identifiers WHERE person_id::text = ANY($1)`, people)
		_, _ = pool.Exec(ctx, `DELETE FROM people WHERE id::text = ANY($1) AND nik IS NULL`, people)
	})
	service := NewImportService(NewRepository(pool), ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-integration-test"}
	scope := auth.RegencyScope{Unrestricted: true}
	mapping := Mapping{FullName: "Nama", NIK: "NIK", Address: "Alamat", Village: "Desa", PhoneNumber: "No HP"}
	importRows := func(name string, rows [][]any) ImportResult {
		t.Helper()
		workbook := workbookBytes(t, func(file *excelize.File) {
			for index, row := range rows {
				_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
			}
		})
		preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, name, bytes.NewReader(workbook), meta, scope, 1)
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Commit(ctx, auth.Principal{}, preview.ID, mapping, meta, scope)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	header := []any{"Nama", "NIK", "Alamat", "Desa", "No HP"}
	first := importRows("dcp3-awal.xlsx", [][]any{header,
		{"Ani", nikA, "Jalan Lama", "Tempe", "0811"},
		{"Budi", nikB, "Jalan Lama", "Tempe", "0812"},
		{"Cici", "", "Jalan Lama", "Tempe", "0813"},
		// A NIK that is not 16 digits still gets a recipient, flagged for review.
		{"Dodi", shortNIK, "Jalan Lama", "Tempe", "0814"},
	})
	if first.CreatedRows != 4 || first.ReviewRows != 1 {
		t.Fatalf("first import: %+v", first)
	}
	var hidden int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1 AND intended_person_id IS NULL`, scheduleID).Scan(&hidden); err != nil || hidden != 0 {
		t.Fatalf("allocations without a person = %d, err=%v", hidden, err)
	}
	// Budi's distribution has started: a slot is linked to the allocation.
	if _, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id)
		SELECT pa.schedule_id, 1, 'linked', pa.id, pa.intended_person_id FROM package_allocations pa JOIN people p ON p.id=pa.intended_person_id
		WHERE pa.schedule_id=$1 AND p.nik=$2`, scheduleID, nikB); err != nil {
		t.Fatal(err)
	}

	second := importRows("dcp3-revisi.xlsx", [][]any{header,
		{"Ani", nikA, "Jalan Baru", "Tempe", "0821"},
		{"Budi", nikB, "Jalan Baru", "Tempe", "0822"},
		{"Cici", "", "Jalan Baru", "Tempe", "0823"},
		// Dodi's NIK is fixed: the recipient is completed, not added again.
		{"Dodi", nikD, "Jalan Baru", "Tempe", "0824"},
		{"Eka", nikE, "Jalan Baru", "Tempe", "0825"},
		{"", "", "Catatan kaki", "", ""},
	})
	if second.CreatedRows != 1 || second.UpdatedRows != 3 || second.KeptRows != 1 || second.SkippedRows != 1 || second.InvalidRows != 1 {
		t.Fatalf("second import: %+v", second)
	}
	var allocations, people int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT intended_person_id) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&allocations, &people); err != nil {
		t.Fatal(err)
	}
	if allocations != 5 || people != 5 {
		t.Fatalf("allocations=%d people=%d, want 5 each", allocations, people)
	}
	addresses := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT p.full_name, p.address FROM package_allocations pa JOIN people p ON p.id=pa.intended_person_id WHERE pa.schedule_id=$1`, scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, address string
		if err := rows.Scan(&name, &address); err != nil {
			t.Fatal(err)
		}
		addresses[name] = address
	}
	if addresses["ANI"] != "JALAN BARU" || addresses["CICI"] != "JALAN BARU" || addresses["BUDI"] != "JALAN LAMA" || addresses["EKA"] != "JALAN BARU" || addresses["DODI"] != "JALAN BARU" {
		t.Fatalf("addresses after re-import = %+v", addresses)
	}
}

func TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	scheduleID, _, regencyID := createDCP3ScheduleFixture(t, pool)
	repository := NewRepository(pool)
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-scope-integration-test"}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE user_agent='dcp3-scope-integration-test'`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
	})
	workbook := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{{"No", "Nama"}, {1, "Siti Aminah"}}
		for index, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
		}
	})
	outOfScope := auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}
	if _, err := service.Preview(ctx, auth.Principal{}, scheduleID, "out-of-scope.xlsx", bytes.NewReader(workbook), meta, outOfScope, 1); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound for out-of-scope schedule, got %v", err)
	}

	inScope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "in-scope.xlsx", bytes.NewReader(workbook), meta, inScope, 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.GetPreview(ctx, preview.ID, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound reading preview out of scope, got %v", err)
	}
	if _, err := service.GetPreview(ctx, preview.ID, inScope); err != nil {
		t.Fatalf("in-scope read should succeed: %v", err)
	}
	if _, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{FullName: "Nama"}, meta, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound committing out of scope, got %v", err)
	}
}

func createDCP3ScheduleFixture(t *testing.T, pool *pgxpool.Pool, cleanupNIKs ...string) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	code := fmt.Sprintf("%c%c%c", 'K', 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-1]%20)
	var regencyID, programID, packageID, documentationID, scheduleID, personID string
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan',$1,$2) RETURNING id::text`, "DCP3 Test "+suffix, code).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'DCP3 Test','farmer',2026,'active') RETURNING id::text`, "DCP3-"+suffix).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM package_template_versions WHERE template_code='PETANI-LPG' AND version=1`).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code='DOK-PETANI' AND version=1`).Scan(&documentationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,'DCP3 Test','2026-09-01','2026-09-30','active') RETURNING id::text`, programID, regencyID, packageID, documentationID).Scan(&scheduleID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Pemilik Kartu','7312345678901288') RETURNING id::text`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card','KP99','KP-99')`, personID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_files WHERE documentation_slot_id IN (SELECT s.id FROM documentation_slots s JOIN distribution_slots ds ON ds.id=s.distribution_slot_id WHERE ds.schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slots WHERE distribution_slot_id IN (SELECT id FROM distribution_slots WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM eligibility_checks WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM person_sector_identifiers WHERE person_id=$1`, personID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM person_sector_identifiers WHERE person_id IN (SELECT id FROM people WHERE nik = ANY($1))`, cleanupNIKs)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1 OR nik = ANY($2)`, personID, cleanupNIKs)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE user_agent='dcp3-integration-test'`)
	})
	return scheduleID, personID, regencyID
}

func dcp3IntegrationPool(t *testing.T) *pgxpool.Pool {
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
