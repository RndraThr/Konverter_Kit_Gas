package migrations

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestFarmerDocumentationSlotsMigrationCreatesTheApprovedPOSFlow(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`CREATE TEMP TABLE documentation_template_versions (
		id text PRIMARY KEY,
		template_code text NOT NULL
	); CREATE TEMP TABLE documentation_template_slots (
		template_version_id text NOT NULL,
		slot_code text NOT NULL,
		label text NOT NULL,
		stage text NOT NULL,
		is_required boolean NOT NULL,
		min_files integer NOT NULL,
		max_files integer NOT NULL,
		input_source text NOT NULL,
		require_location boolean NOT NULL DEFAULT false,
		require_captured_at boolean NOT NULL DEFAULT false,
		instructions text,
		sort_order integer NOT NULL,
		UNIQUE(template_version_id, slot_code)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO documentation_template_versions(id,template_code) VALUES
		('farmer-template','DOK-PETANI'),('fisher-template','DOK-NELAYAN');
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,sort_order) VALUES
		('farmer-template','legacy','Legacy farmer','penyerahan',true,1,1,'both',10),
		('fisher-template','legacy','Legacy fisher','penyerahan',true,1,1,'both',10)`); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00039_farmer_documentation_slots.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	var total, required, machine, documents, handover int
	if err := tx.QueryRow(`SELECT count(*),count(*) FILTER (WHERE is_required),count(*) FILTER (WHERE stage='mesin'),count(*) FILTER (WHERE stage='dokumen'),count(*) FILTER (WHERE stage='penyerahan') FROM documentation_template_slots WHERE template_version_id='farmer-template'`).Scan(&total, &required, &machine, &documents, &handover); err != nil {
		t.Fatal(err)
	}
	if total != 13 || required != 11 || machine != 2 || documents != 6 || handover != 5 {
		t.Fatalf("unexpected farmer slot summary: total=%d required=%d mesin=%d dokumen=%d penyerahan=%d", total, required, machine, documents, handover)
	}

	rows, err := tx.Query(`SELECT slot_code,label,stage,is_required,min_files,max_files,input_source FROM documentation_template_slots WHERE template_version_id='farmer-template' ORDER BY sort_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	wantCodes := []string{
		"new_machine_with_number", "new_machine_serial_with_number",
		"recipient_id_card", "family_card", "farmer_card_or_certificate", "land_area_certificate", "recipient_with_id_card", "profession_certificate",
		"recipient_with_package", "recipient_with_technician", "recipient_training", "recipient_with_old_machine", "old_machine_serial",
	}
	wantLabels := []string{
		"Foto Mesin Baru dan Nomor Urut",
		"Foto Nomor Seri Mesin Baru dan Nomor Urut",
		"Foto KTP dan Nomor Urut",
		"Foto KK dan Nomor Urut",
		"Foto Kartu Tani atau Surat Keterangan dan Nomor Urut",
		"Foto Surat Keterangan Luas Lahan dan Nomor Urut",
		"Foto Penerima dengan KTP dan Nomor Urut",
		"Foto Surat Keterangan Profesi dan Nomor Urut (apabila status pekerjaan di KTP bukan petani)",
		"Foto Penerima dengan Paket Distribusi beserta Toolkit, Manual Book, Kartu Garansi, dan Nomor Urut (di depan banner backdrop)",
		"Foto Penerima dengan Teknisi dan Nomor Urut (di depan banner backdrop)",
		"Foto Penerima untuk Training dan Nomor Urut (di depan banner pelatihan teknis)",
		"Foto Penerima dengan Mesin Lama dan Nomor Urut",
		"Foto Nomor Seri Mesin Lama dan Nomor Urut",
	}
	var gotCodes []string
	for rows.Next() {
		var code, label, stage, source string
		var required bool
		var minFiles, maxFiles int
		if err := rows.Scan(&code, &label, &stage, &required, &minFiles, &maxFiles, &source); err != nil {
			t.Fatal(err)
		}
		index := len(gotCodes)
		if index >= len(wantLabels) || label != wantLabels[index] || minFiles != 1 || maxFiles != 1 || source != "both" {
			t.Fatalf("invalid slot %s: label=%q min=%d max=%d source=%s", code, label, minFiles, maxFiles, source)
		}
		gotCodes = append(gotCodes, code)
	}
	if strings.Join(gotCodes, ",") != strings.Join(wantCodes, ",") {
		t.Fatalf("unexpected slot order: %v", gotCodes)
	}

	var fishermanSlots int
	if err := tx.QueryRow(`SELECT count(*) FROM documentation_template_slots WHERE template_version_id='fisher-template'`).Scan(&fishermanSlots); err != nil {
		t.Fatal(err)
	}
	if fishermanSlots != 1 {
		t.Fatalf("fisherman template must stay untouched, got %d slots", fishermanSlots)
	}
}
