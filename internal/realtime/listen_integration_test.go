package realtime

import (
	"context"
	"database/sql"
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
)

func realtimeIntegrationPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
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
	return pool, databaseURL
}

// A change to a candidate allocation reaches a subscriber of that regency
// through the trigger, NOTIFY and the listener; other regencies hear nothing.
func TestIntegrationTriggerReachesSubscribers(t *testing.T) {
	pool, databaseURL := realtimeIntegrationPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	var regencyID, programID, packageTemplateID, docTemplateID, scheduleID string
	must(pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Realtime Test','RTM',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	must(pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Realtime','farmer',2026,'active') RETURNING id::text`, "RTM-"+suffix).Scan(&programID))
	must(pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket RTM','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-RTM-"+suffix).Scan(&packageTemplateID))
	must(pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok RTM','farmer','published') RETURNING id::text`, "DOC-RTM-"+suffix).Scan(&docTemplateID))
	must(pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal RTM','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	hub := NewHub()
	mine := hub.Subscribe(auth.RegencyScope{RegencyIDs: []string{regencyID}})
	other := hub.Subscribe(auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}})
	go Listen(ctx, databaseURL, hub)
	time.Sleep(300 * time.Millisecond) // let LISTEN start

	nik := fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)
	var personID, nominationID string
	must(pool.QueryRow(ctx, `INSERT INTO people(full_name,nik) VALUES('Realtime Candidate',$1) RETURNING id::text`, nik).Scan(&personID))
	must(pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
	_, err := pool.Exec(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'candidate','{}'::jsonb)`, scheduleID, nominationID, personID)
	must(err)

	select {
	case e := <-mine.Events():
		if e.Kind != "candidates" || e.ScheduleID != scheduleID || e.RegencyID != regencyID {
			t.Fatalf("event = %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no realtime event received")
	}
	select {
	case e := <-other.Events():
		t.Fatalf("other regency received %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
}
