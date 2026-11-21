package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"
	"konkit/internal/distribution"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func testPool(t *testing.T) *pgxpool.Pool {
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

type fixture struct {
	programID, scheduleID, regencyID, slotID string
	mesinMedia, dokumenMedia                 string
	field, uploader, leader, outsider, admin string
}

// seed: one kabupaten with one completed recipient whose photos were taken
// by two officers, plus a leader, an outsider and the reviewer.
func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var f fixture
	var code string
	must(t, pool.QueryRow(ctx, `SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int) FROM generate_series(0,17575) g
		WHERE NOT EXISTS (SELECT 1 FROM regencies WHERE document_code=chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int)) ORDER BY g DESC LIMIT 1`).Scan(&code))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Jawa Timur',$1,$2) RETURNING id::text`, "Monitoring "+suffix, code).Scan(&f.regencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Monitoring','farmer',2026,'active') RETURNING id::text`, "MON-"+suffix).Scan(&f.programID))
	var zoneID, pkg, doc string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_zones(program_id,code,name,sort_order) VALUES($1,'Z1','Zona I',1) RETURNING id::text`, f.programID).Scan(&zoneID))
	_, err := pool.Exec(ctx, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3)`, f.programID, f.regencyID, zoneID)
	must(t, err)
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{}','published') RETURNING id::text`, "PKG-MON-"+suffix).Scan(&pkg))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, "DOC-MON-"+suffix).Scan(&doc))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota)
		VALUES($1,$2,$3,$4,'KAB. MONITORING','2026-01-01','2026-12-31','active',10) RETURNING id::text`, f.programID, f.regencyID, pkg, doc).Scan(&f.scheduleID))

	user := func(name string) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash,full_name) VALUES($1,$2,'x',$3) RETURNING id::text`,
			name+suffix, name+suffix+"@test.local", strings.ToUpper(name[:1])+name[1:]).Scan(&id))
		return id
	}
	f.field, f.uploader, f.leader, f.outsider, f.admin = user("field"), user("uploader"), user("leader"), user("outsider"), user("admin")

	var personID, nominationID, allocationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village) VALUES('Budi Petani',$1,'Sukamaju') RETURNING id::text`, fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)).Scan(&personID))
	must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, personID).Scan(&nominationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,actual_recipient_person_id,distribution_number,status,package_snapshot_json)
		VALUES($1,$2,$3,$3,1,'distributed','{}') RETURNING id::text`, f.scheduleID, nominationID, personID).Scan(&allocationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id,machine_serial_number,distribution_date,distributed_at,distributed_by,completed_at)
		VALUES($1,1,'completed',$2,$3,'SN-001','2026-11-20','2026-11-20T03:00:00Z',$4,'2026-11-20T03:00:00Z') RETURNING id::text`, f.scheduleID, allocationID, personID, f.field).Scan(&f.slotID))
	media := func(code, label, stage, uploader string) string {
		var docSlot, id string
		must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status)
			VALUES($1,$2,$3,$4,true,1,1,'both','complete') RETURNING id::text`, f.slotID, code, label, stage).Scan(&docSlot))
		must(t, pool.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status,uploaded_by)
			VALUES($1,gen_random_uuid(),'foto.jpg','image/jpeg',1024,$2,'camera','accepted',$3) RETURNING id::text`, docSlot, strings.Repeat("a", 64), uploader).Scan(&id))
		return id
	}
	f.dokumenMedia = media("ktp", "Foto KTP", "dokumen", f.uploader)
	f.mesinMedia = media("mesin", "Foto Mesin", "mesin", f.field)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM notifications WHERE user_id = ANY($1::uuid[])`, []string{f.field, f.uploader, f.leader, f.outsider, f.admin})
		_, _ = pool.Exec(c, `DELETE FROM distribution_slots WHERE id=$1`, f.slotID)
		_, _ = pool.Exec(c, `DELETE FROM package_allocations WHERE id=$1`, allocationID)
		_, _ = pool.Exec(c, `DELETE FROM candidate_nominations WHERE id=$1`, nominationID)
		_, _ = pool.Exec(c, `DELETE FROM people WHERE id=$1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM program_schedules WHERE id=$1`, f.scheduleID)
		_, _ = pool.Exec(c, `DELETE FROM programs WHERE id=$1`, f.programID)
		_, _ = pool.Exec(c, `DELETE FROM package_template_versions WHERE id=$1`, pkg)
		_, _ = pool.Exec(c, `DELETE FROM documentation_template_versions WHERE id=$1`, doc)
		_, _ = pool.Exec(c, `DELETE FROM regencies WHERE id=$1`, f.regencyID)
		_, _ = pool.Exec(c, `DELETE FROM audit_logs WHERE actor_user_id = ANY($1::uuid[])`, []string{f.admin})
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{f.field, f.uploader, f.leader, f.outsider, f.admin})
	})
	return f
}

var all = auth.RegencyScope{Unrestricted: true}

func TestCalendarSetsPhasesAndToday(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	admin := auth.Principal{UserID: f.admin}
	days := []PhaseDay{{"2026-11-18", PhaseMaterial}, {"2026-11-19", PhaseRakor}, {"2026-11-20", PhaseDistribusi}, {"2026-11-21", PhaseDistribusi}, {"2026-11-22", PhasePenutupan}}
	rakornas := []string{"2026-11-23", "2026-11-23"}
	must(t, service.SaveCalendar(ctx, admin, CalendarInput{ProgramID: f.programID, Rows: []CalendarRow{{ScheduleID: f.scheduleID, Days: days}}, Rakornas: &rakornas}, auth.ClientMeta{}))
	if err := service.SaveCalendar(ctx, admin, CalendarInput{ProgramID: f.programID, Rows: []CalendarRow{{ScheduleID: f.scheduleID, Days: []PhaseDay{{"2026-11-18", "libur"}}}}}, auth.ClientMeta{}); !errors.Is(err, ErrInvalidPhase) {
		t.Fatalf("unknown phase err = %v", err)
	}

	overview, err := service.Overview(ctx, f.programID, "2026-11-21", all)
	must(t, err)
	if len(overview.Rows) != 1 {
		t.Fatalf("rows = %+v", overview.Rows)
	}
	row := overview.Rows[0]
	// The schedule's own dates are printed on DP3: the calendar leaves them alone.
	if row.StartDate != "2026-01-01" || row.EndDate != "2026-12-31" || len(row.Days) != 5 || row.ZoneName != "Zona I" {
		t.Fatalf("row = %+v", row)
	}
	if row.TodayPhase != PhaseDistribusi || row.DistributionDay != 2 || row.DistributionOf != 2 {
		t.Fatalf("today = %s %d/%d", row.TodayPhase, row.DistributionDay, row.DistributionOf)
	}
	if row.Quota != 10 || row.Completed != 1 || row.Pending != 1 || row.Approved != 0 {
		t.Fatalf("numbers = %+v", row)
	}
	if !slices.Equal(overview.Rakornas, []string{"2026-11-23"}) {
		t.Fatalf("rakornas = %v", overview.Rakornas)
	}
}

func TestAssignmentsRejectUnknownUser(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	admin := auth.Principal{UserID: f.admin}
	must(t, service.SaveAssignments(ctx, admin, AssignmentInput{ScheduleID: f.scheduleID, Reviewers: []string{f.admin, f.admin}, Leaders: []string{f.leader}}, auth.ClientMeta{}))
	if err := service.SaveAssignments(ctx, admin, AssignmentInput{ScheduleID: f.scheduleID, Reviewers: []string{"00000000-0000-0000-0000-000000000000"}}, auth.ClientMeta{}); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("unknown user err = %v", err)
	}
	overview, err := service.Overview(ctx, f.programID, "2026-11-21", all)
	must(t, err)
	if r := overview.Rows[0]; len(r.Reviewers) != 1 || r.Reviewers[0].ID != f.admin || len(r.Leaders) != 1 || r.Leaders[0].ID != f.leader {
		t.Fatalf("assignments kept after failed save: %+v", r)
	}
}

// addPhoto adds a current photo to one documentation slot of the fixture's slot.
func addPhoto(t *testing.T, pool *pgxpool.Pool, f fixture, code, uploader string) string {
	t.Helper()
	var id string
	must(t, pool.QueryRow(context.Background(), `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status,uploaded_by)
		SELECT id, gen_random_uuid(), 'baru.jpg', 'image/jpeg', 1024, $3, 'camera', 'accepted', $4 FROM documentation_slots WHERE distribution_slot_id = $1 AND slot_code = $2
		RETURNING id::text`, f.slotID, code, strings.Repeat("b", 64), uploader).Scan(&id))
	return id
}

func queueItem(t *testing.T, service *Service, f fixture) QueueItem {
	t.Helper()
	queue, err := service.Queue(context.Background(), f.scheduleID, all)
	must(t, err)
	if len(queue) != 1 {
		t.Fatalf("queue = %+v", queue)
	}
	return queue[0]
}

func TestPhotosAreApprovedAsTheyCome(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	admin := auth.Principal{UserID: f.admin}
	// POS Mesin and the KTP photo taken a day ahead: the recipient is not completed yet.
	_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='linked', completed_at=NULL, distributed_at=NULL WHERE id=$1`, f.slotID)
	must(t, err)
	if item := queueItem(t, service, f); item.State != StatePending || item.PendingPhotos != 2 {
		t.Fatalf("before = %+v", item)
	}

	detail, err := service.Approve(ctx, admin, f.scheduleID, 1, ApproveInput{}, all, auth.ClientMeta{})
	must(t, err)
	if detail.State != StatePartial || len(detail.Reviews) != 2 || detail.Reviews[f.mesinMedia].ReviewedBy != "Admin" {
		t.Fatalf("approved = %s %+v", detail.State, detail.Reviews)
	}
	if _, err := service.Approve(ctx, admin, f.scheduleID, 1, ApproveInput{}, all, auth.ClientMeta{}); !errors.Is(err, ErrNothingToApprove) {
		t.Fatalf("approve again err = %v", err)
	}

	// A new photo needs its own check; the approved ones stay approved.
	added := addPhoto(t, pool, f, "ktp", f.uploader)
	if item := queueItem(t, service, f); item.State != StatePending || item.PendingPhotos != 1 || item.ApprovedPhotos != 2 {
		t.Fatalf("after new photo = %+v", item)
	}
	if _, err := service.Approve(ctx, admin, f.scheduleID, 1, ApproveInput{MediaIDs: []string{"00000000-0000-0000-0000-000000000000"}}, all, auth.ClientMeta{}); !errors.Is(err, ErrFlagUnknown) {
		t.Fatalf("foreign photo err = %v", err)
	}
	_, err = service.Approve(ctx, admin, f.scheduleID, 1, ApproveInput{MediaIDs: []string{added}}, all, auth.ClientMeta{})
	must(t, err)

	_, err = pool.Exec(ctx, `UPDATE distribution_slots SET status='completed', completed_at=now() WHERE id=$1`, f.slotID)
	must(t, err)
	if item := queueItem(t, service, f); item.State != StateApproved || item.Photos != 3 {
		t.Fatalf("completed = %+v", item)
	}
	overview, err := service.Overview(ctx, f.programID, "2026-11-20", all)
	must(t, err)
	if r := overview.Rows[0]; r.Approved != 1 || r.Pending != 0 || r.PendingPhotos != 0 {
		t.Fatalf("overview = %+v", r)
	}
}

func TestRevisionRejectsPhotosReopensAndNotifies(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	repo := distribution.NewRepository(pool)
	service := NewService(pool, repo, nil)
	admin := auth.Principal{UserID: f.admin}
	must(t, service.SaveAssignments(ctx, admin, AssignmentInput{ScheduleID: f.scheduleID, Leaders: []string{f.leader, f.admin}}, auth.ClientMeta{}))

	if _, err := service.RequestRevision(ctx, admin, f.scheduleID, 1, RevisionInput{Note: "ulang"}, all, auth.ClientMeta{}); !errors.Is(err, ErrRevisionEmpty) {
		t.Fatalf("empty revision err = %v", err)
	}
	other := seed(t, pool)
	if _, err := service.RequestRevision(ctx, admin, f.scheduleID, 1, RevisionInput{Flags: []FlagInput{{MediaID: other.mesinMedia}}}, all, auth.ClientMeta{}); !errors.Is(err, ErrFlagUnknown) {
		t.Fatalf("foreign photo err = %v", err)
	}

	detail, err := service.RequestRevision(ctx, admin, f.scheduleID, 1, RevisionInput{
		Flags: []FlagInput{{MediaID: f.dokumenMedia, Reason: "KTP buram"}, {MediaID: f.mesinMedia, Reason: "Nomor mesin tidak terbaca"}},
		Note:  "Mohon foto ulang",
	}, all, auth.ClientMeta{})
	must(t, err)
	slot := detail.Slot
	if detail.State != StateRevision || slot.Status != "linked" || !slot.NeedsRecompletion || slot.ReopenedStage != "mesin" {
		t.Fatalf("reopened = %s %+v", detail.State, slot)
	}
	if slot.RevisionReason != "Foto KTP: KTP buram; Foto Mesin: Nomor mesin tidak terbaca; Mohon foto ulang" {
		t.Fatalf("reason = %q", slot.RevisionReason)
	}
	if len(slot.RevisionFlags) != 2 || detail.Reviews[f.dokumenMedia].Status != "rejected" || detail.Reviews[f.dokumenMedia].Reason != "KTP buram" {
		t.Fatalf("flags = %+v reviews = %+v", slot.RevisionFlags, detail.Reviews)
	}

	rows, err := pool.Query(ctx, `SELECT user_id::text, category, link FROM notifications WHERE user_id = ANY($1::uuid[])`, []string{f.field, f.uploader, f.leader, f.outsider, f.admin})
	must(t, err)
	got := map[string]string{}
	for rows.Next() {
		var user, category, link string
		must(t, rows.Scan(&user, &category, &link))
		if category != "distribution_revision" || link != "/nomor-bagi/1?schedule="+f.scheduleID+"&revisi=1" {
			t.Fatalf("notification %s %s", category, link)
		}
		got[user] = link
	}
	rows.Close()
	for _, want := range []string{f.field, f.uploader, f.leader} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing notification for %s: %v", want, got)
		}
	}
	if _, ok := got[f.outsider]; ok || len(got) != 3 {
		t.Fatalf("unexpected recipients: %v", got)
	}

	// The field replaces the machine photo: its mark goes, the new photo waits for a check.
	_, err = pool.Exec(ctx, `UPDATE media_files SET status='deleted' WHERE id=$1`, f.mesinMedia)
	must(t, err)
	addPhoto(t, pool, f, "mesin", f.field)
	again, err := repo.SearchSlot(ctx, f.scheduleID, "1", all)
	must(t, err)
	if len(again.RevisionFlags) != 1 || again.RevisionFlags[0].MediaID != f.dokumenMedia {
		t.Fatalf("flags after replace = %+v", again.RevisionFlags)
	}
	if item := queueItem(t, service, f); item.State != StatePending || item.RejectedPhotos != 1 || item.PendingPhotos != 1 {
		t.Fatalf("queue after replace = %+v", item)
	}
}

func TestRevisionBeforeCompletionKeepsTheSlotOpen(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='linked', completed_at=NULL WHERE id=$1`, f.slotID)
	must(t, err)
	detail, err := service.RequestRevision(ctx, auth.Principal{UserID: f.admin}, f.scheduleID, 1, RevisionInput{Flags: []FlagInput{{MediaID: f.mesinMedia, Reason: "Gelap"}}}, all, auth.ClientMeta{})
	must(t, err)
	if detail.Slot.Status != "linked" || detail.Slot.NeedsRecompletion || len(detail.Slot.RevisionFlags) != 1 || detail.State != StatePending {
		t.Fatalf("open slot revision = %s %+v", detail.State, detail.Slot)
	}
}
