package bast

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
)

func TestRepositoryResolvesScopedBAContextAndCompletedSlots(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var programID, regencyID, zoneID, packageID, documentationID, scheduleID string
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
	first := time.Date(2024, 12, 9, 18, 0, 0, 0, time.UTC)
	var firstSlotID string
	for _, slot := range []struct {
		number int
		at     time.Time
	}{{10, first}, {1, first.Add(-30 * time.Minute)}} {
		var slotID string
		if err := pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at) VALUES($1,$2,'completed',$3,$3) RETURNING id::text`, scheduleID, slot.number, slot.at).Scan(&slotID); err != nil {
			t.Fatal(err)
		}
		if slot.number == 1 {
			firstSlotID = slotID
		}
	}
	var personID string
	mustQuery(`INSERT INTO people(full_name,nik,address,village,district,phone_number,verification_status) VALUES('Siti Aminah','7306014101900001','Alamat Awal','Tempe','Sabbangparu','08123456789','verified') RETURNING id::text`, nil, &personID)
	if _, err := pool.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,'farmer_card',$2,'KP-01')`, personID, "KP01-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE distribution_slots SET recipient_person_id=$2,machine_option_code='machine-1',machine_serial_number='M-001',hose_option_code='hose-1',hose_serial_number='H-001',converter_option_code='converter-1',converter_serial_number='C-001',verification_snapshot_json='{"equipment":{"machine_option_code":"machine-1","machine_brand":"SHARK SNAPSHOT","machine_type":"SPWP SNAPSHOT","machine_serial":"M-001","hose_option_code":"hose-1","hose_brand":"TRILLIUNHOSE SNAPSHOT\nYAMAKOYO SNAPSHOT","hose_spec":"6 M SNAPSHOT\n10 M SNAPSHOT","hose_serial":"H-001","converter_option_code":"converter-1","converter_brand":"ERGAS SNAPSHOT","converter_serial":"C-001"}}'::jsonb WHERE id=$1`, firstSlotID, personID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE package_template_versions SET values_json='{"machine_options":[{"code":"machine-1","brand":"SHARK","type":"SPWP"}],"hose_options":[{"code":"hose-1","suction_brand":"TRILLIUNHOSE","suction_spec":"6 M","discharge_brand":"YAMAKOYO","discharge_spec":"10 M"}],"converter_options":[{"code":"converter-1","brand":"ERGAS"}],"components":[{"code":"lpg","label":"Tabung LPG 3 Kg","quantity":1,"unit":"Tabung"}]}'::jsonb WHERE id=$1`, packageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_ba_logo_assets(program_id,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm,max_height_mm,is_visible) VALUES($1,'organizer',$2,'logo.png','image/png',10,$3,1,35,18,true)`, programID, "bast-logo-"+suffix, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_daily_bundles WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_individual_documents WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_bast_settings WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_assignments WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_zones WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1`, personID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	repository := NewRepository(pool)
	scope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	contextData, err := repository.GetSourceContext(ctx, programID, regencyID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if contextData.ZoneName != "Zona 1" || contextData.SlotQuota != 50 || contextData.DocumentSeries != "KSM-KKT" || contextData.RegencyCode != code || !contextData.HasActiveLogo {
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
	location, _ := time.LoadLocation("Asia/Jakarta")
	service := NewService(repository, location)
	recipients, err := service.ListRecipients(ctx, programID, regencyID, "2024-12-10", scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 2 {
		t.Fatalf("recipients=%+v", recipients)
	}
	document, err := service.FinalizeRecipient(ctx, auth.Principal{}, programID, regencyID, recipients[0], scope, auth.ClientMeta{UserAgent: "bast-finalize-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE people SET full_name='Nama Berubah' WHERE id=$1`, personID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE distribution_slots SET machine_serial_number='M-CHANGED' WHERE id=$1`, firstSlotID); err != nil {
		t.Fatal(err)
	}
	reloaded, err := repository.getIndividualDocument(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Snapshot.Recipient.FullName != "Siti Aminah" || reloaded.Snapshot.Equipment.MachineSerial != "M-001" {
		t.Fatalf("snapshot mutated: %+v", reloaded.Snapshot)
	}
	if reloaded.Snapshot.Equipment.MachineBrand != "SHARK SNAPSHOT" || reloaded.Snapshot.Equipment.MachineType != "SPWP SNAPSHOT" {
		t.Fatalf("machine snapshot=%+v", reloaded.Snapshot.Equipment)
	}
	if reloaded.Snapshot.Equipment.HoseBrand != "TRILLIUNHOSE SNAPSHOT\nYAMAKOYO SNAPSHOT" || reloaded.Snapshot.Equipment.HoseSpec != "6 M SNAPSHOT\n10 M SNAPSHOT" || reloaded.Snapshot.Equipment.ConverterBrand != "ERGAS SNAPSHOT" {
		t.Fatalf("separated hose snapshot=%+v", reloaded.Snapshot.Equipment)
	}
	activation := BundleActivation{ProgramID: programID, RegencyID: regencyID, LocalDate: "2024-12-10", Filename: "SELASA, 10 DESEMBER 2024.pdf", PageCount: 1, Checksum: strings.Repeat("b", 64), StorageKey: "bundle-key-1", Items: []BundleItemActivation{{IndividualDocumentID: document.ID, SlotNumber: document.SlotNumber, PageStart: 1, PageEnd: 1}}}
	activated, err := repository.ActivateBundle(ctx, auth.Principal{}, activation, auth.ClientMeta{UserAgent: "bast-bundle-test"})
	if err != nil {
		t.Fatal(err)
	}
	if activated.Bundle.Version != 1 || activated.Bundle.RecipientCount != 1 || activated.Bundle.StorageKey != "bundle-key-1" {
		t.Fatalf("activated=%+v", activated)
	}
	opened, err := repository.GetActiveBundleByID(ctx, activated.Bundle.ID, scope)
	if err != nil || opened.ID != activated.Bundle.ID {
		t.Fatalf("opened=%+v err=%v", opened, err)
	}
	if _, err := repository.GetActiveBundleByID(ctx, activated.Bundle.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-scope bundle err=%v", err)
	}
	conflicting := activation
	conflicting.Checksum = strings.Repeat("c", 64)
	conflicting.StorageKey = "bundle-key-2"
	if _, err := repository.ActivateBundle(ctx, auth.Principal{}, conflicting, auth.ClientMeta{}); !errors.Is(err, ErrBundleConflict) {
		t.Fatalf("conflict err=%v", err)
	}
}
