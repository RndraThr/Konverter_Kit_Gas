package monitoring

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type dashboardFixture struct {
	programID, regencyA, regencyB string
}

// seedDashboard: kabupaten A (Zona I, kuota 10) with a completed recipient
// whose photos are all approved and whose BA is final (plus an older
// superseded BA), a completed recipient with a photo not checked yet, a
// linked and a cancelled slot; kabupaten B (no zone, kuota 5) with one open
// slot. Activity media: two active rakor of A, one deleted, one of another
// program.
func seedDashboard(t *testing.T, pool *pgxpool.Pool) dashboardFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var f dashboardFixture
	var cleanups []func(context.Context)
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](context.Background())
		}
	})
	del := func(table, id string) {
		cleanups = append(cleanups, func(c context.Context) { _, _ = pool.Exec(c, `DELETE FROM `+table+` WHERE id=$1`, id) })
	}
	regency := func(name string) string {
		var code, id string
		must(t, pool.QueryRow(ctx, `SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int) FROM generate_series(0,17575) g
			WHERE NOT EXISTS (SELECT 1 FROM regencies WHERE document_code=chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int)) ORDER BY g DESC LIMIT 1`).Scan(&code))
		must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Jawa Timur',$1,$2) RETURNING id::text`, name+" "+suffix, code).Scan(&id))
		del("regencies", id)
		return id
	}
	f.regencyA, f.regencyB = regency("Dash A"), regency("Dash B")
	program := func(code string) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Dashboard','farmer',2026,'active') RETURNING id::text`, code+suffix).Scan(&id))
		del("programs", id)
		return id
	}
	f.programID = program("DASH-")
	otherProgram := program("DASH-OTHER-")
	var zoneID, pkg, doc string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_zones(program_id,code,name,sort_order) VALUES($1,'Z1','Zona I',1) RETURNING id::text`, f.programID).Scan(&zoneID))
	// B has no assignment, so no zone.
	_, err := pool.Exec(ctx, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3)`, f.programID, f.regencyA, zoneID)
	must(t, err)
	cleanups = append(cleanups, func(c context.Context) {
		_, _ = pool.Exec(c, `DELETE FROM program_regency_assignments WHERE program_id=$1`, f.programID)
		_, _ = pool.Exec(c, `DELETE FROM program_zones WHERE program_id=$1`, f.programID)
	})
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{}','published') RETURNING id::text`, "PKG-DASH-"+suffix).Scan(&pkg))
	del("package_template_versions", pkg)
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, "DOC-DASH-"+suffix).Scan(&doc))
	del("documentation_template_versions", doc)
	schedule := func(regencyID string, quota int) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota)
			VALUES($1,$2,$3,$4,'KAB. DASHBOARD','2026-01-01','2026-12-31','active',$5) RETURNING id::text`, f.programID, regencyID, pkg, doc, quota).Scan(&id))
		del("program_schedules", id)
		return id
	}
	scheduleA, scheduleB := schedule(f.regencyA, 10), schedule(f.regencyB, 5)

	var field string
	must(t, pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash,full_name) VALUES($1,$2,'x','Field') RETURNING id::text`, "dashfield"+suffix, "dashfield"+suffix+"@test.local").Scan(&field))
	del("users", field)

	// slot adds a slot; with a recipient when status is not open.
	slot := func(scheduleID string, number int, status, completedAt string) string {
		var allocationID any
		if status != "open" {
			var personID, nominationID, alloc string
			must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village) VALUES('Petani',$1,'Sukamaju') RETURNING id::text`, fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)).Scan(&personID))
			must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}','ready') RETURNING id::text`, personID).Scan(&nominationID))
			must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json)
				VALUES($1,$2,$3,$4,'ready','{}') RETURNING id::text`, scheduleID, nominationID, personID, number).Scan(&alloc))
			del("people", personID)
			del("candidate_nominations", nominationID)
			del("package_allocations", alloc)
			allocationID = alloc
		}
		var completed any
		if completedAt != "" {
			completed = completedAt
		}
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,distribution_date,completed_at)
			VALUES($1,$2,$3,$4,'2026-11-20',$5) RETURNING id::text`, scheduleID, number, status, allocationID, completed).Scan(&id))
		cleanups = append(cleanups, func(c context.Context) {
			_, _ = pool.Exec(c, `DELETE FROM bast_individual_documents WHERE distribution_slot_id=$1`, id)
			_, _ = pool.Exec(c, `DELETE FROM distribution_slots WHERE id=$1`, id)
		})
		return id
	}
	photo := func(slotID, review string) {
		var docSlot, media string
		must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status)
			VALUES($1,'ktp','Foto KTP','dokumen',true,1,1,'both','complete') RETURNING id::text`, slotID).Scan(&docSlot))
		must(t, pool.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status,uploaded_by)
			VALUES($1,gen_random_uuid(),'foto.jpg','image/jpeg',1024,$2,'camera','accepted',$3) RETURNING id::text`, docSlot, strings.Repeat("a", 64), field).Scan(&media))
		if review != "" {
			_, err := pool.Exec(ctx, `INSERT INTO media_reviews(media_id,status,reviewed_by) VALUES($1,$2,$3)`, media, review, field)
			must(t, err)
		}
		cleanups = append(cleanups, func(c context.Context) {
			_, _ = pool.Exec(c, `DELETE FROM media_reviews WHERE media_id=$1`, media)
			_, _ = pool.Exec(c, `DELETE FROM media_files WHERE id=$1`, media)
			_, _ = pool.Exec(c, `DELETE FROM documentation_slots WHERE id=$1`, docSlot)
		})
	}
	// 2026-11-20T20:00Z is 2026-11-21 03:00 WIB.
	done := slot(scheduleA, 1, "completed", "2026-11-19T20:00:00Z")
	photo(done, "approved")
	for i, status := range []string{"superseded", "final"} {
		_, err := pool.Exec(ctx, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,package_template_version_id,snapshot_json,revision,status)
			VALUES($1,$2,$3,'2026-11-20',1,10,$4,$5,'{}',$6,$7)`, done, f.programID, f.regencyA, fmt.Sprintf("1/10/DASH-%d", i), pkg, i+1, status)
		must(t, err)
	}
	checking := slot(scheduleA, 2, "completed", "2026-11-20T20:00:00Z")
	photo(checking, "")
	slot(scheduleA, 3, "linked", "")
	slot(scheduleA, 4, "cancelled", "")
	slot(scheduleB, 1, "open", "")

	media := func(programID, status string) {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO activity_media(regency_id,program_id,activity_type,storage_key,display_name,original_filename,media_type,mime_type,byte_size,checksum,source,status)
			VALUES($1,$2,'rakor',gen_random_uuid(),'rakor.jpg','rakor.jpg','image','image/jpeg',1024,$3,'camera',$4) RETURNING id::text`, f.regencyA, programID, strings.Repeat("b", 64), status).Scan(&id))
		del("activity_media", id)
	}
	media(f.programID, "active")
	media(f.programID, "active")
	media(f.programID, "deleted")
	media(otherProgram, "active")
	return f
}

func TestDashboardCountsEachStage(t *testing.T) {
	pool := testPool(t)
	f := seedDashboard(t, pool)
	got, err := NewService(pool, nil, nil).Dashboard(context.Background(), f.programID, all)
	must(t, err)

	if want := (DashboardStages{Quota: 15, Linked: 3, Completed: 2, Approved: 1, BAFinal: 1}); got.Totals != want {
		t.Fatalf("totals=%+v want %+v", got.Totals, want)
	}
	if len(got.Regencies) != 2 {
		t.Fatalf("regencies=%+v", got.Regencies)
	}
	a, b := got.Regencies[0], got.Regencies[1]
	if a.RegencyID != f.regencyA || a.ZoneName != "Zona I" || a.DashboardStages != (DashboardStages{10, 3, 2, 1, 1}) {
		t.Fatalf("regency A=%+v", a)
	}
	if b.RegencyID != f.regencyB || b.ZoneName != "" || b.DashboardStages != (DashboardStages{Quota: 5}) {
		t.Fatalf("regency B=%+v", b)
	}
	if len(got.Zones) != 2 || got.Zones[0].ZoneName != "Zona I" || got.Zones[0].DashboardStages != a.DashboardStages ||
		got.Zones[1].ZoneName != "" || got.Zones[1].DashboardStages != b.DashboardStages {
		t.Fatalf("zones=%+v", got.Zones)
	}
	if want := []DashboardDay{{"2026-11-20", 1}, {"2026-11-21", 1}}; !reflect.DeepEqual(got.Daily, want) {
		t.Fatalf("daily=%+v want %+v", got.Daily, want)
	}
	if want := []DashboardActivity{{"rakor", 2}}; !reflect.DeepEqual(got.Activities, want) {
		t.Fatalf("activities=%+v want %+v", got.Activities, want)
	}
}

func TestDashboardFollowsRegencyScope(t *testing.T) {
	pool := testPool(t)
	f := seedDashboard(t, pool)
	got, err := NewService(pool, nil, nil).Dashboard(context.Background(), f.programID, auth.RegencyScope{RegencyIDs: []string{f.regencyB}})
	must(t, err)
	if len(got.Regencies) != 1 || got.Regencies[0].RegencyID != f.regencyB || got.Totals.Quota != 5 {
		t.Fatalf("scoped=%+v", got)
	}
	if got.Daily == nil || len(got.Daily) != 0 || got.Activities == nil || len(got.Activities) != 0 {
		t.Fatalf("daily=%v activities=%v, want empty non-nil", got.Daily, got.Activities)
	}
}

func TestDashboardUnknownProgram(t *testing.T) {
	pool := testPool(t)
	seedDashboard(t, pool)
	service := NewService(pool, nil, nil)
	if _, err := service.Dashboard(context.Background(), "00000000-0000-0000-0000-000000000000", all); !errors.Is(err, ErrProgramNotFound) {
		t.Fatalf("err=%v", err)
	}
	got, err := service.Dashboard(context.Background(), "", all)
	must(t, err)
	overview, err := service.Overview(context.Background(), "", "", all)
	must(t, err)
	if got.ProgramID == "" || got.ProgramID != overview.ProgramID {
		t.Fatalf("default program=%q overview=%q", got.ProgramID, overview.ProgramID)
	}
}
