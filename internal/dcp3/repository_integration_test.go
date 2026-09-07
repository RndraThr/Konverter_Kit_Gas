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
	scheduleID, conflictingPersonID, _ := createDCP3ScheduleFixture(t, pool)
	repository := NewRepository(pool)
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-integration-test"}
	workbook := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{
			{"No", "Nama", "NIK", "No Kartu Petani", "Alamat", "Desa", "Kecamatan", "No HP"},
			{1, "Siti Aminah", "7312345678901234", "KP-01", "Jalan Sawah", "Tempe", "Sabbangparu", "08121"},
			{1, "Siti Duplikat", "7312345678901234", "KP-02", "Jalan Dua", "Tempe", "Sabbangparu", "08122"},
			{3, "Hasan Konflik", "7312345678901299", "KP-99", "Jalan Tiga", "Tempe", "Sabbangparu", "08123"},
		}
		for index, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
		}
	})

	unrestricted := auth.RegencyScope{Unrestricted: true}
	preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3-petani.xlsx", bytes.NewReader(workbook), meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ProgramType != "farmer" || len(preview.Rows) != 3 || len(preview.Headers) != 8 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	_, err = service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3-petani-copy.xlsx", bytes.NewReader(workbook), meta, unrestricted)
	if !errors.Is(err, ErrDuplicateImport) {
		t.Fatalf("expected duplicate import, got %v", err)
	}

	result, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{
		SourceSequence: "No", FullName: "Nama", NIK: "NIK", FarmerCardNumber: "No Kartu Petani",
		Address: "Alamat", Village: "Desa", District: "Kecamatan", PhoneNumber: "No HP",
	}, meta, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRows != 3 || result.ValidRows != 1 || result.WarningRows != 2 || result.InvalidRows != 0 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	committedPreview, err := service.GetPreview(ctx, preview.ID, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if committedPreview.Status != "imported" || len(committedPreview.Headers) != 8 {
		t.Fatalf("committed preview lost its source metadata: %+v", committedPreview)
	}

	var allocationCount, distributionCount, slotCount int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT distribution_number) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&allocationCount, &distributionCount); err != nil {
		t.Fatal(err)
	}
	if allocationCount != 3 || distributionCount != 3 {
		t.Fatalf("allocations=%d distinct_numbers=%d", allocationCount, distributionCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM distribution_records d JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1`, scheduleID).Scan(&distributionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM documentation_slots s JOIN distribution_records d ON d.id=s.distribution_id JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1`, scheduleID).Scan(&slotCount); err != nil {
		t.Fatal(err)
	}
	if distributionCount != 3 || slotCount != 12 {
		t.Fatalf("distributions=%d slots=%d", distributionCount, slotCount)
	}

	var reviewRows, packageSnapshots int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dcp3_import_rows WHERE batch_id=$1 AND validation_status='needs_review'`, preview.ID).Scan(&reviewRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1 AND package_snapshot_json->>'converter_brand'='ERGAS'`, scheduleID).Scan(&packageSnapshots); err != nil {
		t.Fatal(err)
	}
	if reviewRows != 2 || packageSnapshots != 3 {
		t.Fatalf("review_rows=%d package_snapshots=%d", reviewRows, packageSnapshots)
	}

	var conflictingOwner string
	if err := pool.QueryRow(ctx, `SELECT person_id::text FROM person_sector_identifiers WHERE identifier_type='farmer_card' AND normalized_value='KP99'`).Scan(&conflictingOwner); err != nil {
		t.Fatal(err)
	}
	if conflictingOwner != conflictingPersonID {
		t.Fatalf("conflicting identifier owner changed: %s", conflictingOwner)
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
	if _, err := service.Preview(ctx, auth.Principal{}, scheduleID, "out-of-scope.xlsx", bytes.NewReader(workbook), meta, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound for out-of-scope schedule, got %v", err)
	}

	inScope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "in-scope.xlsx", bytes.NewReader(workbook), meta, inScope)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.GetPreview(ctx, preview.ID, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound reading preview out of scope, got %v", err)
	}
	if _, err := service.GetPreview(ctx, preview.ID, inScope); err != nil {
		t.Fatalf("in-scope read should succeed: %v", err)
	}
	if _, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{SourceSequence: "No", FullName: "Nama"}, meta, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound committing out of scope, got %v", err)
	}
}

func createDCP3ScheduleFixture(t *testing.T, pool *pgxpool.Pool) (string, string, string) {
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_files WHERE documentation_slot_id IN (SELECT s.id FROM documentation_slots s JOIN distribution_records d ON d.id=s.distribution_id JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slots WHERE distribution_id IN (SELECT d.id FROM distribution_records d JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM eligibility_checks WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE allocation_id IN (SELECT id FROM package_allocations WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM person_sector_identifiers WHERE person_id=$1`, personID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1 OR nik IN ('7312345678901234','7312345678901299')`, personID)
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
