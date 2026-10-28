package distmap

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/geo"
	"konkit/internal/recipients"
	"konkit/internal/recipients/recipienttest"

	"github.com/jackc/pgx/v5/pgxpool"
)

var unrestricted = auth.RegencyScope{Unrestricted: true}

// A point well inside Kabupaten Indramayu, a point ~298 m north of its northern tip (outside the
// polygon but inside the 500 m buffer), and a point in Kabupaten Cirebon ~21 km from Indramayu.
// Verified against the geoBoundaries asset with ST_Covers / ST_Distance.
const (
	insideLon, insideLat   = 108.177825, -6.444000
	nearLon, nearLat       = 108.199000, -6.218305
	cirebonLon, cirebonLat = 108.448129, -6.761500
)

// mapEnv is the recipient fixture plus a copy of the real Indramayu boundary linked to the
// fixture's farmer regency, so location checks run against real geometry without touching the
// boundary rows the application itself links.
type mapEnv struct {
	pool    *pgxpool.Pool
	fx      recipienttest.Fixture
	repo    *Repository
	counter int
	// rows created by the helpers, removed in cleanup
	people, nominations, allocations, schedules, locations []string
}

func newMapEnv(t *testing.T) *mapEnv {
	t.Helper()
	pool := recipienttest.Pool(t)
	ctx := context.Background()
	var boundaries int
	recipienttest.Must(t, pool.QueryRow(ctx, `SELECT count(*) FROM regency_boundaries WHERE boundary_name='Indramayu'`).Scan(&boundaries))
	if boundaries == 0 {
		if _, err := geo.EnsureRegencyBoundaries(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	env := &mapEnv{pool: pool, fx: recipienttest.Seed(t, pool), repo: NewRepository(pool)}
	env.exec(t, `INSERT INTO regency_boundaries(boundary_name,normalized_key,regency_id,geom,source,license,asset_checksum)
		SELECT 'Distmap Test Indramayu','DISTMAPTESTINDRAMAYU',$1,geom,source,license,'distmap-test'
		FROM regency_boundaries WHERE boundary_name='Indramayu'`, env.fx.FarmerRegencyID)
	// Registered after Seed, so it runs before the fixture's own cleanup (t.Cleanup is LIFO).
	t.Cleanup(func() {
		c := context.Background()
		schedules := append([]string{env.fx.ScheduleID}, env.schedules...)
		for _, q := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM bast_individual_documents WHERE distribution_slot_id IN (SELECT id FROM distribution_slots WHERE schedule_id = ANY($1::uuid[]))`, schedules},
			{`DELETE FROM distribution_slots WHERE schedule_id = ANY($1::uuid[])`, schedules},
			{`DELETE FROM package_allocations WHERE id = ANY($1::uuid[])`, env.allocations},
			{`DELETE FROM candidate_nominations WHERE id = ANY($1::uuid[])`, env.nominations},
			{`DELETE FROM people WHERE id = ANY($1::uuid[])`, env.people},
			{`DELETE FROM program_schedules WHERE id = ANY($1::uuid[])`, env.schedules},
			{`DELETE FROM bast_handover_locations WHERE id = ANY($1::uuid[])`, env.locations},
			{`DELETE FROM regency_boundaries WHERE asset_checksum = $1`, "distmap-test"},
		} {
			if _, err := pool.Exec(c, q.sql, q.arg); err != nil {
				t.Logf("cleanup %q: %v", q.sql, err)
			}
		}
	})
	return env
}

func (e *mapEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *mapEnv) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var out string
	recipienttest.Must(t, e.pool.QueryRow(context.Background(), sql, args...).Scan(&out))
	return out
}

// schedule adds a second schedule of the fixture programme in regencyID.
func (e *mapEnv) schedule(t *testing.T, regencyID string) string {
	t.Helper()
	id := e.scalar(t, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json)
		VALUES($1,$2,$3,$4,'Jadwal Peta Test','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`,
		e.fx.ProgramID, regencyID, e.fx.PackageTemplateID, e.fx.DocTemplateID)
	e.schedules = append(e.schedules, id)
	return id
}

func (e *mapEnv) allocation(t *testing.T, scheduleID string) (allocationID, personID string) {
	t.Helper()
	e.counter++
	personID = e.scalar(t, `INSERT INTO people(full_name) VALUES($1) RETURNING id::text`, fmt.Sprintf("Penerima Peta %d", e.counter))
	nominationID := e.scalar(t, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID)
	allocationID = e.scalar(t, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'ready','{}'::jsonb) RETURNING id::text`, scheduleID, nominationID, personID)
	e.people = append(e.people, personID)
	e.nominations = append(e.nominations, nominationID)
	e.allocations = append(e.allocations, allocationID)
	return allocationID, personID
}

// slot creates a distribution slot; every status except open gets its own recipient allocation.
func (e *mapEnv) slot(t *testing.T, scheduleID string, number int, status string) string {
	t.Helper()
	if status == "open" {
		return e.scalar(t, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,$2,'open') RETURNING id::text`, scheduleID, number)
	}
	allocationID, personID := e.allocation(t, scheduleID)
	return e.scalar(t, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id,completed_at)
		VALUES($1,$2,$3,$4,$5,CASE WHEN $3='completed' THEN now() END) RETURNING id::text`, scheduleID, number, status, allocationID, personID)
}

// docSlot adds a documentation slot to a distribution slot.
func (e *mapEnv) docSlot(t *testing.T, slotID, code, stage string, sortOrder int, required bool) string {
	t.Helper()
	return e.scalar(t, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,stage,sort_order)
		VALUES($1,$2,$2,$3,1,5,'camera',$4,$5) RETURNING id::text`, slotID, code, required, stage, sortOrder)
}

// photo adds a media file with the given coordinates to a documentation slot.
func (e *mapEnv) photo(t *testing.T, docSlotID string, lon, lat float64, status string, capturedAt time.Time) {
	t.Helper()
	e.counter++
	e.exec(t, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,captured_at,latitude,longitude,status)
		VALUES($1,$2,'foto.jpg','image/jpeg',100,repeat('a',64),'camera',$3,$4,$5,$6)`,
		docSlotID, fmt.Sprintf("distmap-test/%s/%d.jpg", docSlotID, e.counter), capturedAt, lat, lon, status)
}

// locatedSlot is a slot with one accepted, non-required machine photo at lon/lat.
func (e *mapEnv) locatedSlot(t *testing.T, scheduleID string, number int, status string, lon, lat float64) string {
	t.Helper()
	slotID := e.slot(t, scheduleID, number, status)
	e.photo(t, e.docSlot(t, slotID, "foto_mesin", "mesin", 1, false), lon, lat, "accepted", time.Now())
	return slotID
}

func (e *mapEnv) baDocument(t *testing.T, slotID string, revision int, status string) {
	t.Helper()
	e.exec(t, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,package_template_version_id,snapshot_json,revision,status)
		SELECT ds.id,$2,ps.regency_id,'2026-10-28',ds.slot_number,100,'BA-'||ds.slot_number||'-'||$3::int,$4,'{}'::jsonb,$3::int,$5
		FROM distribution_slots ds JOIN program_schedules ps ON ps.id=ds.schedule_id WHERE ds.id=$1`,
		slotID, e.fx.ProgramID, revision, e.fx.PackageTemplateID, status)
}

func (e *mapEnv) handoverLocation(t *testing.T, regencyID, name string) string {
	t.Helper()
	id := e.scalar(t, `INSERT INTO bast_handover_locations(regency_id,name,name_key) VALUES($1,$2,upper($2)) RETURNING id::text`, regencyID, name)
	e.locations = append(e.locations, id)
	return id
}

func (e *mapEnv) points(t *testing.T, q PointQuery, scope auth.RegencyScope) (map[string]Point, PointsMeta) {
	t.Helper()
	items, meta, err := e.repo.Points(context.Background(), q, scope)
	if err != nil {
		t.Fatal(err)
	}
	bySlot := map[string]Point{}
	for _, p := range items {
		if _, dup := bySlot[p.SlotID]; dup {
			t.Fatalf("slot %s returned twice", p.SlotID)
		}
		bySlot[p.SlotID] = p
	}
	return bySlot, meta
}

func (e *mapEnv) programQuery() PointQuery {
	return PointQuery{Filter: recipients.Filter{ProgramID: e.fx.ProgramID}}
}

// offset moves lon/lat by the given metres east and north.
func offset(lon, lat, east, north float64) (float64, float64) {
	return lon + east/(111320*math.Cos(lat*math.Pi/180)), lat + north/110574
}

func TestPointsMilestones(t *testing.T) {
	env := newMapEnv(t)
	s := env.fx.ScheduleID

	open := env.locatedSlot(t, s, 1, "open", insideLon, insideLat)
	linked := env.locatedSlot(t, s, 2, "linked", insideLon, insideLat)

	// Delivered but the required handover photo is missing: evidence partial → stage 3.
	delivered := env.locatedSlot(t, s, 3, "completed", insideLon, insideLat)
	env.docSlot(t, delivered, "foto_serah", "penyerahan", 2, true)

	evidenced := env.locatedSlot(t, s, 4, "completed", insideLon, insideLat)
	env.photo(t, env.docSlot(t, evidenced, "foto_serah", "penyerahan", 2, true), insideLon, insideLat, "accepted", time.Now())

	signed := env.locatedSlot(t, s, 5, "completed", insideLon, insideLat)
	env.photo(t, env.docSlot(t, signed, "foto_serah", "penyerahan", 2, true), insideLon, insideLat, "accepted", time.Now())
	env.baDocument(t, signed, 1, "superseded")
	env.baDocument(t, signed, 2, "final")

	// Only an older, superseded BA revision and no current final: must stay at stage 4.
	staleBA := env.locatedSlot(t, s, 6, "completed", insideLon, insideLat)
	env.photo(t, env.docSlot(t, staleBA, "foto_serah", "penyerahan", 2, true), insideLon, insideLat, "accepted", time.Now())
	env.baDocument(t, staleBA, 1, "superseded")

	cancelled := env.locatedSlot(t, s, 7, "cancelled", insideLon, insideLat)

	got, meta := env.points(t, env.programQuery(), unrestricted)
	want := map[string]int{open: 1, linked: 2, delivered: 3, evidenced: 4, signed: 5, staleBA: 4}
	for slotID, stage := range want {
		p, ok := got[slotID]
		if !ok {
			t.Fatalf("slot at stage %d missing from points", stage)
		}
		if p.Stage != stage || len(p.StageAt) != stage {
			t.Fatalf("slot %s: want stage %d with %d timestamps, got stage %d with %v", p.Code, stage, stage, p.Stage, p.StageAt)
		}
		for i, at := range p.StageAt {
			if at.IsZero() {
				t.Fatalf("slot %s: stage_at[%d] is zero", p.Code, i)
			}
		}
		if p.ScheduleID != s || p.RegencyID != env.fx.FarmerRegencyID || p.ZoneID != env.fx.ZoneID {
			t.Fatalf("slot %s: wrong schedule/regency/zone %+v", p.Code, p)
		}
	}
	if _, ok := got[cancelled]; ok {
		t.Fatal("cancelled slot must not be shown")
	}
	if len(got) != len(want) || meta.Total != len(want) || meta.Located != len(want) || meta.Unlocated != 0 {
		t.Fatalf("unexpected result size: %d points, meta %+v", len(got), meta)
	}
	if got[open].Code != "1" || got[signed].Code != "5" {
		t.Fatalf("code should be the slot number, got %q and %q", got[open].Code, got[signed].Code)
	}
}

func TestPointsLocationSource(t *testing.T) {
	env := newMapEnv(t)
	s := env.fx.ScheduleID
	now := time.Now()
	aLon, aLat := insideLon, insideLat
	bLon, bLat := offset(insideLon, insideLat, 500, 0)
	cLon, cLat := offset(insideLon, insideLat, 0, 500)
	dLon, dLat := offset(insideLon, insideLat, 500, 500)

	// Handover photo wins over an earlier-sorted document photo.
	byStage := env.slot(t, s, 1, "completed")
	env.photo(t, env.docSlot(t, byStage, "foto_ktp", "dokumen", 1, false), aLon, aLat, "accepted", now)
	env.photo(t, env.docSlot(t, byStage, "foto_serah", "penyerahan", 5, false), bLon, bLat, "accepted", now)

	// Two handover slots: the smaller sort_order wins; within it the earliest capture wins.
	bySort := env.slot(t, s, 2, "completed")
	env.photo(t, env.docSlot(t, bySort, "foto_serah_b", "penyerahan", 3, false), aLon, aLat, "accepted", now.Add(-time.Hour))
	first := env.docSlot(t, bySort, "foto_serah_a", "penyerahan", 2, false)
	env.photo(t, first, bLon, bLat, "accepted", now)
	env.photo(t, first, cLon, cLat, "accepted", now.Add(-time.Minute))

	// A rejected photo is ignored even though it would rank first.
	rejected := env.slot(t, s, 3, "completed")
	env.photo(t, env.docSlot(t, rejected, "foto_serah", "penyerahan", 1, false), aLon, aLat, "rejected", now)
	env.photo(t, env.docSlot(t, rejected, "foto_ktp", "dokumen", 2, false), dLon, dLat, "accepted", now)

	// 0,0 (no GPS fix) and a swapped pair (longitude in -11..6) count as having no location.
	zero := env.slot(t, s, 4, "completed")
	env.photo(t, env.docSlot(t, zero, "foto_serah", "penyerahan", 1, false), 0, 0, "accepted", now)
	swapped := env.slot(t, s, 5, "completed")
	env.photo(t, env.docSlot(t, swapped, "foto_serah", "penyerahan", 1, false), insideLat, 1.5, "accepted", now)

	// The recipient is replaced after the photos were uploaded: the point follows the slot.
	newAllocation, newPerson := env.allocation(t, s)
	env.exec(t, `UPDATE package_allocations SET status='replaced' WHERE id=(SELECT allocation_id FROM distribution_slots WHERE id=$1)`, byStage)
	env.exec(t, `UPDATE distribution_slots SET allocation_id=$2, recipient_person_id=$3 WHERE id=$1`, byStage, newAllocation, newPerson)

	got, meta := env.points(t, env.programQuery(), unrestricted)
	assertAt := func(slotID string, lon, lat float64) {
		t.Helper()
		p, ok := got[slotID]
		if !ok {
			t.Fatalf("slot %s missing", slotID)
		}
		if math.Abs(p.Lon-lon) > 1e-6 || math.Abs(p.Lat-lat) > 1e-6 {
			t.Fatalf("slot %s: want %.6f,%.6f got %.6f,%.6f", p.Code, lon, lat, p.Lon, p.Lat)
		}
	}
	assertAt(byStage, bLon, bLat)
	assertAt(bySort, cLon, cLat)
	assertAt(rejected, dLon, dLat)
	if _, ok := got[zero]; ok {
		t.Fatal("0,0 photo must not produce a point")
	}
	if _, ok := got[swapped]; ok {
		t.Fatal("swapped lat/lng photo must not produce a point")
	}
	if meta.Total != 5 || meta.Located != 3 || meta.Unlocated != 2 {
		t.Fatalf("unexpected meta %+v", meta)
	}

	unlocated, unlocatedTotal, err := env.repo.UnlocatedSlots(context.Background(), env.programQuery(), unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if unlocatedTotal != 2 || len(unlocated) != 2 || unlocated[0].SlotID != zero || unlocated[1].SlotID != swapped {
		t.Fatalf("unexpected unlocated slots %+v", unlocated)
	}
	// Completed with no required evidence slot: evidence counts as complete, so stage 4.
	if unlocated[0].Code != "4" || unlocated[0].RegencyID != env.fx.FarmerRegencyID || unlocated[0].Stage != 4 {
		t.Fatalf("unexpected unlocated slot fields %+v", unlocated[0])
	}
}

func TestPointsLocationStatus(t *testing.T) {
	env := newMapEnv(t)
	s := env.fx.ScheduleID
	inside := env.locatedSlot(t, s, 1, "linked", insideLon, insideLat)
	near := env.locatedSlot(t, s, 2, "linked", nearLon, nearLat)
	cirebon := env.locatedSlot(t, s, 3, "linked", cirebonLon, cirebonLat)
	// The other fixture regency has no boundary row: its points are shown but unmatched.
	unmatched := env.locatedSlot(t, env.schedule(t, env.fx.OtherRegencyID), 1, "linked", insideLon, insideLat)

	got, meta := env.points(t, env.programQuery(), unrestricted)
	want := map[string]string{inside: "inside", near: "inside", cirebon: "outside", unmatched: "unmatched"}
	for slotID, status := range want {
		p, ok := got[slotID]
		if !ok {
			t.Fatalf("slot %s missing", slotID)
		}
		if p.LocationStatus != status {
			t.Fatalf("slot %s in %s: want %s got %s", p.Code, p.RegencyID, status, p.LocationStatus)
		}
	}
	if meta.Outside != 1 || meta.Located != 4 {
		t.Fatalf("unexpected meta %+v", meta)
	}
}

func TestPointsSitesAndScope(t *testing.T) {
	env := newMapEnv(t)
	s := env.fx.ScheduleID
	balai := env.handoverLocation(t, env.fx.FarmerRegencyID, "Balai Desa Peta Test")

	// Titik serah members: two 300 m apart, one 2 km away (likely the wrong titik serah).
	hlA := env.locatedSlot(t, s, 1, "completed", insideLon, insideLat)
	lon, lat := offset(insideLon, insideLat, 300, 0)
	hlB := env.locatedSlot(t, s, 2, "completed", lon, lat)
	lon, lat = offset(insideLon, insideLat, 2000, 0)
	hlFar := env.locatedSlot(t, s, 3, "completed", lon, lat)
	env.exec(t, `UPDATE distribution_slots SET handover_location_id=$1 WHERE id = ANY($2::uuid[])`, balai, []string{hlA, hlB, hlFar})
	for _, id := range []string{hlA, hlB, hlFar} {
		env.docSlot(t, id, "foto_serah", "penyerahan", 2, true) // required and empty: stays at stage 3
	}

	// Without a titik serah: three within 20 m group together, one 100 m further stands alone.
	baseLon, baseLat := offset(insideLon, insideLat, 0, -1500)
	geo1 := env.locatedSlot(t, s, 4, "linked", baseLon, baseLat)
	lon, lat = offset(baseLon, baseLat, 10, 0)
	geo2 := env.locatedSlot(t, s, 5, "linked", lon, lat)
	lon, lat = offset(baseLon, baseLat, 20, 0)
	geo3 := env.locatedSlot(t, s, 6, "linked", lon, lat)
	lon, lat = offset(baseLon, baseLat, 120, 0)
	geoAlone := env.locatedSlot(t, s, 7, "linked", lon, lat)

	got, _ := env.points(t, env.programQuery(), unrestricted)
	if got[hlA].SiteID != "hl:"+balai || got[hlB].SiteID != got[hlA].SiteID || got[hlFar].SiteID != got[hlA].SiteID {
		t.Fatalf("titik serah members must share hl site: %q %q %q", got[hlA].SiteID, got[hlB].SiteID, got[hlFar].SiteID)
	}
	if got[hlA].SiteName != "Balai Desa Peta Test" || got[hlB].SiteName != got[hlA].SiteName {
		t.Fatalf("titik serah name missing: %q", got[hlA].SiteName)
	}
	if got[hlA].SiteOutlier || got[hlB].SiteOutlier || !got[hlFar].SiteOutlier {
		t.Fatalf("only the member 2 km from the median is an outlier: %v %v %v", got[hlA].SiteOutlier, got[hlB].SiteOutlier, got[hlFar].SiteOutlier)
	}
	geoSite := got[geo1].SiteID
	if geoSite == "" || geoSite[:4] != "geo:" || got[geo2].SiteID != geoSite || got[geo3].SiteID != geoSite {
		t.Fatalf("nearby slots without titik serah must share a geo site: %q %q %q", geoSite, got[geo2].SiteID, got[geo3].SiteID)
	}
	if got[geoAlone].SiteID == geoSite || got[geoAlone].SiteID[:4] != "geo:" {
		t.Fatalf("slot 100 m away must get its own geo site, got %q", got[geoAlone].SiteID)
	}
	if got[geo1].SiteName != "" || got[geo1].SiteOutlier {
		t.Fatalf("geo sites have no name and no outliers: %+v", got[geo1])
	}

	if other, _ := env.points(t, env.programQuery(), auth.RegencyScope{RegencyIDs: []string{env.fx.OtherRegencyID}}); len(other) != 0 {
		t.Fatalf("scope of another regency must see no points, got %d", len(other))
	}

	q := env.programQuery()
	q.BBox = &[4]float64{100, 0, 101, 1}
	if boxed, meta := env.points(t, q, unrestricted); len(boxed) != 0 || meta.Located != 7 {
		t.Fatalf("bbox away from the points must return none (meta still counts the filter), got %d %+v", len(boxed), meta)
	}
	q.BBox = &[4]float64{108, -7, 109, -6}
	if boxed, _ := env.points(t, q, unrestricted); len(boxed) != 7 {
		t.Fatalf("bbox around the points must return all 7, got %d", len(boxed))
	}

	q = env.programQuery()
	q.Stage = 3
	delivered, _ := env.points(t, q, unrestricted)
	if len(delivered) != 3 {
		t.Fatalf("stage 3 filter: want the 3 completed slots, got %d", len(delivered))
	}
	for _, p := range delivered {
		if p.Stage != 3 {
			t.Fatalf("stage filter leaked stage %d", p.Stage)
		}
	}
	// Site grouping is decided before the stage filter, so the titik serah keeps its outlier.
	if !delivered[hlFar].SiteOutlier {
		t.Fatal("stage filter must not change site membership")
	}
}

// A slot whose recipient sits in a cancelled schedule is not on the map, whether it is found through
// the recipient filter (allocated) or as an open slot (unallocated).
func TestPointsExcludeCancelledSchedules(t *testing.T) {
	env := newMapEnv(t)
	live := env.locatedSlot(t, env.fx.ScheduleID, 1, "linked", insideLon, insideLat)

	cancelled := env.schedule(t, env.fx.FarmerRegencyID)
	allocatedSlot := env.locatedSlot(t, cancelled, 1, "linked", insideLon, insideLat)
	openSlot := env.slot(t, cancelled, 2, "open")
	env.exec(t, `UPDATE program_schedules SET status='cancelled' WHERE id=$1`, cancelled)

	got, meta := env.points(t, env.programQuery(), unrestricted)
	if _, ok := got[allocatedSlot]; ok {
		t.Fatal("a recipient slot of a cancelled schedule must not be a point")
	}
	if _, ok := got[live]; !ok || len(got) != 1 {
		t.Fatalf("only the live schedule's slot must remain, got %d points", len(got))
	}
	if meta.Total != 1 || meta.Unlocated != 0 {
		t.Fatalf("cancelled schedule slots must not be counted, got %+v", meta)
	}
	unlocated, total, err := env.repo.UnlocatedSlots(context.Background(), env.programQuery(), unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(unlocated) != 0 {
		t.Fatalf("cancelled schedule slots must not be unlocated either (open slot %s), got %+v", openSlot, unlocated)
	}
}

// The unlocated list is capped at 500 rows, ordered by kabupaten name then slot number, and reports
// the uncapped total so the UI can say how many slots have no location.
func TestUnlocatedSlotsAreCappedAndOrdered(t *testing.T) {
	env := newMapEnv(t)
	ctx := context.Background()
	// A kabupaten whose name sorts before the fixture's "Wajo Recipients Test".
	early := env.scalar(t, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Aaa Peta Test','AAP',true) RETURNING id::text`)
	earlySchedule := env.schedule(t, early)
	// Runs before the env cleanup, so the schedule goes first, then its kabupaten.
	t.Cleanup(func() {
		c := context.Background()
		_, _ = env.pool.Exec(c, `DELETE FROM distribution_slots WHERE schedule_id=$1`, earlySchedule)
		_, _ = env.pool.Exec(c, `DELETE FROM program_schedules WHERE id=$1`, earlySchedule)
		_, _ = env.pool.Exec(c, `DELETE FROM program_regency_assignments WHERE regency_id=$1`, early)
		_, _ = env.pool.Exec(c, `DELETE FROM regencies WHERE id=$1`, early)
	})
	// 505 open slots in the late kabupaten (inserted first, so storage order cannot explain the result),
	// 3 in the early one.
	env.exec(t, `INSERT INTO distribution_slots(schedule_id,slot_number,status) SELECT $1, g, 'open' FROM generate_series(1,505) g`, env.fx.ScheduleID)
	env.exec(t, `INSERT INTO distribution_slots(schedule_id,slot_number,status) SELECT $1, g, 'open' FROM generate_series(1,3) g`, earlySchedule)

	items, total, err := env.repo.UnlocatedSlots(ctx, env.programQuery(), unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if total != 508 || len(items) != 500 {
		t.Fatalf("want total 508 with 500 rows, got total=%d rows=%d", total, len(items))
	}
	for i := 0; i < 3; i++ {
		if items[i].RegencyID != early || items[i].Code != fmt.Sprint(i+1) {
			t.Fatalf("row %d must be early kabupaten slot %d, got %+v", i, i+1, items[i])
		}
	}
	if items[3].RegencyID != env.fx.FarmerRegencyID || items[3].Code != "1" || items[499].Code != "497" {
		t.Fatalf("remaining rows must follow by numeric slot number, got %+v ... %+v", items[3], items[499])
	}
}
