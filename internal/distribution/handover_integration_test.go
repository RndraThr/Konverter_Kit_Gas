package distribution

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"
)

// TestIntegrationHandoverLocationAtDistribution memastikan petugas memilih
// titik serah per nomor bagi: nama baru membuat lokasi kabupaten (dengan ID
// buatan HP), varian ketik memakai lokasi yang sama, jadwal lain di kabupaten
// itu melihat lokasinya, dan nomor bagi selesai tidak diubah dari POS.
func TestIntegrationHandoverLocationAtDistribution(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Jawa Tengah','Titik Serah Test','TST',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	_, _ = pool.Exec(ctx, `DELETE FROM bast_handover_locations WHERE regency_id=$1`, regencyID)
	var programID, packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Titik Serah','farmer',2026,'active') RETURNING id::text`, "TST-"+suffix).Scan(&programID))
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket TS','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-TST-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok TS','farmer','published') RETURNING id::text`, "DOC-TST-"+suffix).Scan(&docTemplateID))
	schedule := func(name string) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,$5,'2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID, name).Scan(&id))
		return id
	}
	scheduleA, scheduleB := schedule("Jadwal A"), schedule("Jadwal B")
	for _, number := range []int{1, 2} {
		_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,$2,'open')`, scheduleA, number)
		must(t, err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status) VALUES($1,1,'completed')`, scheduleB)
	must(t, err)

	service := NewService(NewRepository(pool))
	scope := auth.RegencyScope{Unrestricted: true}
	const phoneID = "6f1d2c3b-4a5e-4f60-8a7b-9c0d1e2f3a4b"
	slot, err := service.SetHandoverLocation(ctx, auth.Principal{}, SetHandoverLocationInput{ScheduleID: scheduleA, SlotNumber: 1, LocationID: phoneID, LocationName: "  Balai  Desa Tempe "}, auth.ClientMeta{}, scope)
	must(t, err)
	if slot.HandoverLocationID == nil || *slot.HandoverLocationID != phoneID || slot.HandoverLocationName != "Balai Desa Tempe" {
		t.Fatalf("slot location=%v %q", slot.HandoverLocationID, slot.HandoverLocationName)
	}
	// Another phone types the same place differently: same location.
	slot, err = service.SetHandoverLocation(ctx, auth.Principal{}, SetHandoverLocationInput{ScheduleID: scheduleA, SlotNumber: 2, LocationID: "0a0b0c0d-1111-4222-8333-944455556666", LocationName: "balai desa, tempe"}, auth.ClientMeta{}, scope)
	must(t, err)
	if slot.HandoverLocationID == nil || *slot.HandoverLocationID != phoneID {
		t.Fatalf("typed variant should reuse the location, got %v", slot.HandoverLocationID)
	}
	locations, err := service.ListHandoverLocations(ctx, scheduleB, scope)
	must(t, err)
	if len(locations) != 1 || locations[0].ID != phoneID {
		t.Fatalf("schedule B locations=%+v", locations)
	}
	if _, err := service.SetHandoverLocation(ctx, auth.Principal{}, SetHandoverLocationInput{ScheduleID: scheduleB, SlotNumber: 1, LocationID: phoneID}, auth.ClientMeta{}, scope); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("completed slot err=%v", err)
	}
	if _, err := service.SetHandoverLocation(ctx, auth.Principal{}, SetHandoverLocationInput{ScheduleID: scheduleA, SlotNumber: 1, LocationName: " . "}, auth.ClientMeta{}, scope); !errors.Is(err, ErrHandoverLocationRequired) {
		t.Fatalf("blank name err=%v", err)
	}

	// The only titik serah of the regency fills a completion without a choice.
	var slotB string
	must(t, pool.QueryRow(ctx, `SELECT id::text FROM distribution_slots WHERE schedule_id=$1 AND slot_number=1`, scheduleB).Scan(&slotB))
	tx, err := pool.Begin(ctx)
	must(t, err)
	must(t, defaultHandoverLocation(ctx, tx, slotB))
	must(t, tx.Commit(ctx))
	var filled string
	must(t, pool.QueryRow(ctx, `SELECT COALESCE(handover_location_id::text,'') FROM distribution_slots WHERE id=$1`, slotB).Scan(&filled))
	if filled != phoneID {
		t.Fatalf("default location=%q", filled)
	}
}
