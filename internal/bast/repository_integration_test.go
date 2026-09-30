package bast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"
)

func TestRepositoryResolvesScopedBAContextAndCompletedSlots(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var programID, regencyID, zoneID, packageID, documentationID, scheduleID, profileID string
	code := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Tender BA Test','farmer',2026,'active') RETURNING id::text`, []any{"BACTX-" + suffix}, &programID)
	mustQuery(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, []any{"Wajo BA " + suffix, code}, &regencyID)
	mustQuery(`INSERT INTO program_zones(program_id,code,name,sort_order,is_placeholder) VALUES($1,'ZONA-1','Zona 1',1,false) RETURNING id::text`, []any{programID}, &zoneID)
	if _, err := pool.Exec(ctx, `INSERT INTO program_regency_assignments(program_id,regency_id,zone_id) VALUES($1,$2,$3)`, programID, regencyID, zoneID); err != nil {
		t.Fatal(err)
	}
	mustQuery(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket BA','farmer','{}','published') RETURNING id::text`, []any{"BAPKG-" + suffix}, &packageID)
	mustQuery(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok BA','farmer','published') RETURNING id::text`, []any{"BADOC-" + suffix}, &documentationID)
	mustQuery(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,slot_quota,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal BA','2024-12-01','2024-12-31','active',4,50,'{}') RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)
	mustQuery(`INSERT INTO program_document_profile_versions(program_id,version,title,procurement_description,document_series,status,published_at) VALUES($1,1,'BAST','Pengadaan','KSM-KKT','published',now()) RETURNING id::text`, []any{programID}, &profileID)
	first := time.Date(2024, 12, 9, 18, 0, 0, 0, time.UTC)
	for _, slot := range []struct {
		number int
		at     time.Time
	}{{10, first}, {1, first.Add(-30 * time.Minute)}} {
		if _, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at) VALUES($1,$2,'completed',$3,$3)`, scheduleID, slot.number, slot.at); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_bast_settings WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_document_profile_versions WHERE id=$1`, profileID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_assignments WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_zones WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	repository := NewRepository(pool)
	scope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	contextData, err := repository.GetSourceContext(ctx, programID, regencyID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if contextData.ZoneName != "Zona 1" || contextData.SlotQuota != 50 || contextData.DocumentSeries != "KSM-KKT" || contextData.RegencyCode != code {
		t.Fatalf("context=%+v", contextData)
	}
	if _, err := repository.GetSourceContext(ctx, programID, regencyID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); err != ErrNotFound {
		t.Fatalf("out-of-scope err=%v", err)
	}
	slots, err := repository.ListCompletedSlots(ctx, programID, regencyID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 || slots[0].SlotNumber != 1 || slots[1].SlotNumber != 10 {
		t.Fatalf("slots=%+v", slots)
	}
	locked, err := repository.LockRegencyTotal(ctx, auth.Principal{}, contextData, auth.ClientMeta{UserAgent: "bast-repository-test"})
	if err != nil {
		t.Fatal(err)
	}
	if locked.FinalTotal != 50 {
		t.Fatalf("locked=%+v", locked)
	}
}
