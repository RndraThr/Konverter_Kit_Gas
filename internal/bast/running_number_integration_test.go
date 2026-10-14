package bast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"
)

// DP3 dan Rekap Harian: nomor urut berlanjut lintas hari distribusi (hari
// kedua mulai setelah nomor terakhir hari pertama), hari mengikuti zona
// Jakarta, dan keduanya memakai nomor yang sama.
func TestDP3AndDailyRecapNumberRowsAcrossDistributionDays(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	code := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-4]%20)
	var programID, regencyID, packageID, documentationID, scheduleID string
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Tender Nomor Urut','farmer',2026,'active') RETURNING id::text`, []any{"BANUM-" + suffix}, &programID)
	mustQuery(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Jawa Tengah',$1,$2,true) RETURNING id::text`, []any{"Nomor Urut " + suffix, code}, &regencyID)
	mustQuery(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{"machine_options":[{"code":"m-1","brand":"SHARK","type":"SPWP","power":"5.5 HP","fuel_type":"Bensin"}]}','published') RETURNING id::text`, []any{"BANUMPKG-" + suffix}, &packageID)
	mustQuery(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, []any{"BANUMDOC-" + suffix}, &documentationID)
	mustQuery(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal','2024-12-01','2024-12-31','active',50,'{}') RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)

	people := []string{}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		for _, id := range people {
			_, _ = pool.Exec(bg, `DELETE FROM people WHERE id=$1`, id)
		}
		_, _ = pool.Exec(bg, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(bg, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(bg, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(bg, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(bg, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	// Hari 1 (1 Des WIB): nomor bagi 5 dan 2. Hari 2 (2 Des WIB): nomor bagi 7
	// (18.00 UTC tanggal 1 = 01.00 WIB tanggal 2) dan 1.
	for i, slot := range []struct {
		number int
		at     time.Time
	}{
		{5, time.Date(2024, 12, 1, 3, 0, 0, 0, time.UTC)},
		{2, time.Date(2024, 12, 1, 4, 0, 0, 0, time.UTC)},
		{7, time.Date(2024, 12, 1, 18, 0, 0, 0, time.UTC)},
		{1, time.Date(2024, 12, 2, 5, 0, 0, 0, time.UTC)},
	} {
		var personID string
		mustQuery(`INSERT INTO people(full_name,nik,address,village,district,verification_status) VALUES($1,$2,'Jl. Contoh','Desa','Kecamatan','verified') RETURNING id::text`,
			[]any{fmt.Sprintf("Penerima %d", slot.number), fmt.Sprintf("33%014d", (time.Now().UnixNano()+int64(i))%100000000000000)}, &personID)
		people = append(people, personID)
		if _, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at,recipient_person_id,machine_option_code,machine_serial_number) VALUES($1,$2,'completed',$3,$3,$4,'m-1',$5)`,
			scheduleID, slot.number, slot.at, personID, fmt.Sprintf("SN-%d", slot.number)); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewRepository(pool)
	scope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	type row struct{ number, slot int }
	expect := map[string][]row{
		"2024-12-01": {{1, 2}, {2, 5}},
		"2024-12-02": {{3, 1}, {4, 7}},
	}
	for date, want := range expect {
		recap, err := repository.ListDailyRecapRecipients(ctx, scheduleID, date, scope)
		if err != nil {
			t.Fatal(err)
		}
		dp3, err := repository.ListDP3Recipients(ctx, scheduleID, date, scope)
		if err != nil {
			t.Fatal(err)
		}
		if len(recap) != len(want) || len(dp3) != len(want) {
			t.Fatalf("%s: recap=%+v dp3=%+v", date, recap, dp3)
		}
		for i, w := range want {
			if recap[i].RowNumber != w.number || recap[i].SlotNumber != w.slot {
				t.Fatalf("%s recap row %d = %+v, want %+v", date, i, recap[i], w)
			}
			if dp3[i].RowNumber != w.number || dp3[i].DistributionNumber == nil || *dp3[i].DistributionNumber != w.slot || dp3[i].SlotID == "" {
				t.Fatalf("%s dp3 row %d = %+v, want %+v", date, i, dp3[i], w)
			}
			if dp3[i].MachinePower != "5.5 HP" || dp3[i].MachineFuelType != "Bensin" || dp3[i].MachineBrand != "SHARK" {
				t.Fatalf("%s dp3 machine from package template = %+v", date, dp3[i])
			}
		}
	}
	all, err := repository.ListDP3Recipients(ctx, scheduleID, "", scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || all[0].RowNumber != 1 || all[3].RowNumber != 4 {
		t.Fatalf("all days = %+v", all)
	}
}
