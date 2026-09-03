package reports

import (
	"context"
	"database/sql"
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

func TestIntegrationSummaryAndRowsReflectAllocationsAndFilters(t *testing.T) {
	pool := reportsIntegrationPool(t)
	fixture := createReportsFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	summary, err := repository.Summary(ctx, fixture.scheduleID, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalAllocations != 3 {
		t.Fatalf("total=%d summary=%+v", summary.TotalAllocations, summary)
	}
	if summary.DocumentationIncomplete != 2 {
		t.Fatalf("documentation incomplete=%d summary=%+v", summary.DocumentationIncomplete, summary)
	}

	rows, err := repository.Rows(ctx, fixture.scheduleID, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].DistributionNumber != 1 || rows[0].NIK != fixture.primaryNIK {
		t.Fatalf("first row=%+v want nik=%q", rows[0], fixture.primaryNIK)
	}
	if !rows[1].DocumentationComplete || rows[1].DistributionStatus != "completed" {
		t.Fatalf("second row=%+v", rows[1])
	}

	filtered, err := repository.Rows(ctx, fixture.scheduleID, Filter{DocumentationStatus: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].DistributionNumber != 2 {
		t.Fatalf("filtered rows=%+v", filtered)
	}

	byAllocation, err := repository.Rows(ctx, fixture.scheduleID, Filter{AllocationStatus: "needs_review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byAllocation) != 1 || byAllocation[0].DistributionNumber != 3 {
		t.Fatalf("allocation-filtered rows=%+v", byAllocation)
	}
}

func TestIntegrationRecordExportWritesAuditEvent(t *testing.T) {
	pool := reportsIntegrationPool(t)
	fixture := createReportsFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	err := repository.RecordExport(ctx, auth.Principal{UserID: ""}, fixture.scheduleID, "xlsx", Filter{AllocationStatus: "ready"}, auth.ClientMeta{UserAgent: "reports-integration"})
	if err != nil {
		t.Fatal(err)
	}
	var action, metadata string
	if err := pool.QueryRow(ctx, `SELECT action,metadata::text FROM audit_logs WHERE resource_id=$1 AND user_agent='reports-integration'`, fixture.scheduleID).Scan(&action, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "reports.exported" || !containsAll(metadata, `"format": "xlsx"`, `"allocation_status": "ready"`) {
		t.Fatalf("action=%q metadata=%q", action, metadata)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

type reportsFixture struct {
	scheduleID string
	primaryNIK string
}

// insertAllocation generates its own 16-digit NIK per call (rather than a
// fixed literal) because internal/distribution's integration tests already
// use fixed NIKs like "7306014101900001" against the same shared
// konkit_test database, and Go runs different packages' tests in parallel
// by default -- a hardcoded NIK here would intermittently collide with the
// unique index on people.nik.
func createReportsFixture(t *testing.T, pool *pgxpool.Pool) reportsFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())

	var packageID, documentationID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM package_template_versions WHERE template_code='PETANI-LPG' AND version=1`).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code='DOK-PETANI' AND version=1`).Scan(&documentationID); err != nil {
		t.Fatal(err)
	}

	var programID, regencyID, scheduleID string
	if err := pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Laporan Test','farmer',2026,'active') RETURNING id::text`, "RPT-"+suffix).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan',$1,$2) RETURNING id::text`, "Wajo Laporan "+suffix, codeFromSuffix("R", suffix)).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,'Wajo Tahap Laporan','2026-09-01','2026-09-30','active') RETURNING id::text`, programID, regencyID, packageID, documentationID).Scan(&scheduleID); err != nil {
		t.Fatal(err)
	}

	personIDs := make([]string, 0, 3)
	insertAllocation := func(number int, personName, allocationStatus, distributionStatus string, documentationComplete bool) (personID, nik string) {
		nik = fmt.Sprintf("9%015d", time.Now().UnixNano()%1_000_000_000_000_000)
		if err := pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village,district) VALUES($1,$2,'Tempe','Sabbangparu') RETURNING id::text`, personName, nik).Scan(&personID); err != nil {
			t.Fatal(err)
		}
		personIDs = append(personIDs, personID)
		var batchID, rowID, nominationID, allocationID string
		if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,status) VALUES($1,$2,$3,'Penerima','imported') RETURNING id::text`, scheduleID, fmt.Sprintf("laporan-%d.xlsx", number), fmt.Sprintf("%064d", time.Now().UnixNano()+int64(number))).Scan(&batchID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,source_sequence_number,raw_data_json,validation_status) VALUES($1,2,$2,'{}','valid') RETURNING id::text`, batchID, number).Scan(&rowID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO candidate_nominations(batch_id,import_row_id,person_id,program_type,source_snapshot_json,status) VALUES($1,$2,$3,'farmer','{}','ready') RETURNING id::text`, batchID, rowID, personID).Scan(&nominationID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}') RETURNING id::text`, scheduleID, nominationID, personID, number, allocationStatus).Scan(&allocationID); err != nil {
			t.Fatal(err)
		}
		var distributionID string
		if err := pool.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id,status) VALUES($1,$2,$3) RETURNING id::text`, allocationID, personID, distributionStatus).Scan(&distributionID); err != nil {
			t.Fatal(err)
		}
		slotStatus := "missing"
		if documentationComplete {
			slotStatus = "complete"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) VALUES($1,'recipient_package','Penerima dan paket',true,1,1,'both',$2)`, distributionID, slotStatus); err != nil {
			t.Fatal(err)
		}
		return personID, nik
	}

	_, primaryNIK := insertAllocation(1, "Siti Aminah", "ready", "draft", false)
	insertAllocation(2, "Siti Nur", "distributed", "completed", true)
	insertAllocation(3, "Aminah Wati", "needs_review", "draft", false)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slots WHERE distribution_id IN (SELECT d.id FROM distribution_records d JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE resource_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE allocation_id IN (SELECT id FROM package_allocations WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=ANY($1)`, personIDs)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
	})

	return reportsFixture{scheduleID: scheduleID, primaryNIK: primaryNIK}
}

func codeFromSuffix(prefix, suffix string) string {
	return prefix + string(rune('A'+suffix[len(suffix)-2]%20)) + string(rune('A'+suffix[len(suffix)-1]%20))
}

func reportsIntegrationPool(t *testing.T) *pgxpool.Pool {
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
