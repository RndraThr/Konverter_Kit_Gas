package reports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/bast"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

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

// freeRegencyCode is a random three-letter document code no regency uses yet.
func freeRegencyCode(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	for range 100 {
		code := string([]byte{byte('A' + rand.IntN(26)), byte('A' + rand.IntN(26)), byte('A' + rand.IntN(26))})
		var taken bool
		if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM regencies WHERE document_code = $1)`, code).Scan(&taken); err != nil {
			t.Fatal(err)
		}
		if !taken {
			return code
		}
	}
	t.Fatal("no free regency code")
	return ""
}

func TestRepositoryReadsRecipientDetailsAndKeepsDriveState(t *testing.T) {
	pool := reportsIntegrationPool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var programID, regencyID, packageID, documentationID, scheduleID, slotID, personID string
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Konkit Excel Test','farmer',2026,'active') RETURNING id::text`, []any{"DP3X-" + suffix}, &programID)
	mustQuery(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, []any{"Wajo Excel " + suffix, freeRegencyCode(t, pool)}, &regencyID)
	mustQuery(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{}','published') RETURNING id::text`, []any{"DP3XPKG-" + suffix}, &packageID)
	mustQuery(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, []any{"DP3XDOC-" + suffix}, &documentationID)
	mustQuery(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal','2026-11-01','2026-11-30','active',10,'{}') RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)
	mustQuery(`INSERT INTO people(full_name,nik,address,village,district,phone_number,verification_status) VALUES('Siti Aminah',$1,'Jl. Poros','Tempe','Sabbangparu','081234567890','verified') RETURNING id::text`, []any{fmt.Sprintf("73%014d", time.Now().UnixNano()%1e14)}, &personID)
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card',$2,'KT-01')`, personID, "KT01-"+suffix); err != nil {
		t.Fatal(err)
	}
	mustQuery(`INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at,distribution_date,recipient_person_id,machine_serial_number,converter_serial_number,hose_serial_number,verification_snapshot_json)
		VALUES($1,3,'completed',now(),now(),'2026-11-05',$2,'M-001','C-001','H-001','{"equipment":{"converter_brand":"ERGAS"}}') RETURNING id::text`, []any{scheduleID, personID}, &slotID)

	ids, err := repo.ListDriveScheduleIDs(ctx)
	if err != nil || !slices.Contains(ids, scheduleID) {
		t.Fatalf("schedule with a completed slot not listed: %v err=%v", ids, err)
	}
	info, err := repo.GetScheduleInfo(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	if err != nil || info.ProgramName != "Konkit Excel Test" || info.FiscalYear != 2026 || info.ProvinceName != "Sulawesi Selatan" {
		t.Fatalf("context: %+v err=%v", info, err)
	}
	if _, err := repo.GetScheduleInfo(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{programID}}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("context outside scope: %v", err)
	}
	details, err := repo.ListSlotDetails(ctx, scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	want := SlotDetail{DistributionDate: "2026-11-05", CardNumber: "KT-01", PhoneNumber: "081234567890",
		MachineSerial: "M-001", ConverterBrand: "ERGAS", ConverterSerial: "C-001", HoseSerial: "H-001"}
	if details[slotID] != want {
		t.Fatalf("details = %+v, want %+v", details[slotID], want)
	}

	report, err := NewService(repo, bast.NewRepository(pool), &memoryStorage{}, time.UTC).Report(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	if err != nil || len(report.Rows) != 1 {
		t.Fatalf("report: %+v err=%v", report, err)
	}
	if row := report.Rows[0]; row.FullName != "Siti Aminah" || row.DistributionNumber != 3 || row.CardNumber != "KT-01" || row.MachineSerial != "M-001" || row.DistributionDate != "2026-11-05" {
		t.Fatalf("report row: %+v", row)
	}
	if _, err := repo.GetScheduleInfo(ctx, "not-a-uuid", auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("malformed schedule id: %v", err)
	}

	if err := repo.RecordDriveFailure(ctx, scheduleID, "drive down"); err != nil {
		t.Fatal(err)
	}
	export, err := repo.GetDriveExport(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	if err != nil || export.StorageKey != "" || export.SyncedAt != nil || export.LastError != "drive down" {
		t.Fatalf("failure before any upload: %+v err=%v", export, err)
	}
	old, err := repo.SaveDriveExport(ctx, DriveExport{ScheduleID: scheduleID, Filename: "a.xlsx", StorageKey: "key-1", ContentChecksum: "c1", RecipientCount: 1, ByteSize: 10})
	if err != nil || old != "" {
		t.Fatalf("first save: old=%q err=%v", old, err)
	}
	old, err = repo.SaveDriveExport(ctx, DriveExport{ScheduleID: scheduleID, Filename: "a.xlsx", StorageKey: "key-2", ContentChecksum: "c2", RecipientCount: 1, ByteSize: 12})
	if err != nil || old != "key-1" {
		t.Fatalf("second save must return the replaced key: old=%q err=%v", old, err)
	}
	export, err = repo.GetDriveExport(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	if err != nil || export.StorageKey != "key-2" || export.ContentChecksum != "c2" || export.SyncedAt == nil || export.LastError != "" {
		t.Fatalf("export after save: %+v err=%v", export, err)
	}
	if err := repo.RecordDriveFailure(ctx, scheduleID, "again"); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDriveChecked(ctx, scheduleID); err != nil {
		t.Fatal(err)
	}
	export, _ = repo.GetDriveExport(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	if export.LastError != "" || export.StorageKey != "key-2" {
		t.Fatalf("check must clear the error and keep the file: %+v", export)
	}
	if _, err := repo.GetDriveExport(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{programID}}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("export outside scope: %v", err)
	}
}
