package distribution

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

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

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Penyerahan Test','PYT',true) RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES('PNY-TEST','Program Test Penyerahan','farmer',2026,'active') RETURNING id::text`).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES('PKG-PNY',1,'Paket Test Penyerahan','farmer','{}'::jsonb,'published') RETURNING id::text`).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES('DOC-PNY',1,'Dok Test Penyerahan','farmer','published') RETURNING id::text`).Scan(&docTemplateID))
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
