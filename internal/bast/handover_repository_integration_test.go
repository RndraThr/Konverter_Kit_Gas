package bast

import (
	"context"
	"errors"
	"strings"
	"testing"

	"konkit/internal/auth"
)

// TestIntegrationHandoverLocations memastikan titik serah dipakai bersama
// satu kabupaten, nama ganda ditolak saat ubah nama, gabungkan memindahkan
// penerima dan menghapus varian, koreksi satu penerima tersimpan di nomor
// bagi, dan perubahan menandai BA Serah Terima final perlu dibuat ulang.
func TestIntegrationHandoverLocations(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID, programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM bast_handover_locations WHERE regency_id=$1 AND name_key LIKE 'ITEST %'`, regencyID)
	var personID, slotID string
	if err := pool.QueryRow(ctx, `INSERT INTO people (full_name, village, district) VALUES ('ITEST PENERIMA', 'TEMPE', 'TEMPE') RETURNING id::text`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO distribution_slots (schedule_id, slot_number, status, recipient_person_id) VALUES ($1, (SELECT COALESCE(max(slot_number),0)+1 FROM distribution_slots WHERE schedule_id=$1), 'linked', $2) RETURNING id::text`, scheduleID, personID).Scan(&slotID); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE id=$1`, slotID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=$1`, personID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE schedule_id=$1 AND (document_type LIKE 'closing_titik_serah:%' OR document_type='closing_kabupaten')`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_handover_locations WHERE regency_id=$1 AND name_key LIKE 'ITEST %'`, regencyID)
	}
	t.Cleanup(cleanup)
	scope := auth.RegencyScope{Unrestricted: true}
	meta := auth.ClientMeta{UserAgent: "handover-integration-test"}

	insert := func(name string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO bast_handover_locations (regency_id, name, name_key) VALUES ($1, $2, upper($2)) RETURNING id::text`, regencyID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	right := insert("ITEST BALAI DESA")
	typo := insert("ITEST BALAI DSEA")

	locations, err := repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, location := range locations {
		names = append(names, location.Name)
	}
	if !strings.Contains(strings.Join(names, "|"), "ITEST BALAI DESA|ITEST BALAI DSEA") {
		t.Fatalf("regency locations=%v", names)
	}

	if err := repository.AssignHandoverLocation(ctx, auth.Principal{}, scheduleID, slotID, typo, scope, meta); err != nil {
		t.Fatal(err)
	}
	recipients, err := repository.ListHandoverRecipients(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipient := range recipients {
		if recipient.SlotID == slotID && recipient.LocationID != typo {
			t.Fatalf("assigned location=%q", recipient.LocationID)
		}
	}

	if _, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: closingDocumentType(right), DocumentDate: "2026-12-01", Filename: "CLOSING TITIK SERAH.pdf",
		RecipientCount: 3, PageCount: 1, Checksum: strings.Repeat("c", 64), StorageKey: "key-" + right,
		Snapshot: []byte(`{"document_type":"closing_titik_serah","handover_location":"BALAI","grand_total":3,"rows":[{"machine_brand":"SHARK","machine_type":"SPWP","count":3}]}`), ExpectedVersion: 1,
	}, meta); err != nil {
		t.Fatal(err)
	}
	statuses, err := repository.HandoverDocumentStatuses(ctx, scheduleID)
	if err != nil || !statuses[right].Active {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	snapshots, err := repository.ListActiveClosingSnapshots(ctx, scheduleID)
	if err != nil || snapshots[right].GrandTotal != 3 {
		t.Fatalf("snapshots=%+v err=%v", snapshots, err)
	}

	// Ubah nama ke nama lokasi lain ditolak; varian ketik dibandingkan lewat kuncinya.
	if err := repository.RenameHandoverLocation(ctx, auth.Principal{}, scheduleID, typo, "ITEST BALAI DESA", scope, meta); !errors.Is(err, ErrHandoverLocationDuplicate) {
		t.Fatalf("duplicate rename err=%v", err)
	}
	if err := repository.DeleteHandoverLocation(ctx, auth.Principal{}, scheduleID, typo, scope, meta); !errors.Is(err, ErrHandoverLocationInUse) {
		t.Fatalf("delete in use err=%v", err)
	}

	// Gabungkan: penerima pindah ke lokasi yang benar, varian terhapus, BA final perlu dibuat ulang.
	if err := repository.MergeHandoverLocations(ctx, auth.Principal{}, scheduleID, typo, right, scope, meta); err != nil {
		t.Fatal(err)
	}
	var moved string
	if err := pool.QueryRow(ctx, `SELECT handover_location_id::text FROM distribution_slots WHERE id=$1`, slotID).Scan(&moved); err != nil || moved != right {
		t.Fatalf("moved=%q err=%v", moved, err)
	}
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bast_handover_locations WHERE id::text=$1)`, typo).Scan(&exists)
	if exists {
		t.Fatal("merged location should be deleted")
	}
	statuses, _ = repository.HandoverDocumentStatuses(ctx, scheduleID)
	if statuses[right].Active || !statuses[right].Stale {
		t.Fatalf("closing document should be stale after merge: %+v", statuses)
	}

	if err := repository.RenameHandoverLocation(ctx, auth.Principal{}, scheduleID, right, "ITEST Balai Desa Baru", scope, meta); err != nil {
		t.Fatal(err)
	}
	var name, key string
	_ = pool.QueryRow(ctx, `SELECT name, name_key FROM bast_handover_locations WHERE id::text=$1`, right).Scan(&name, &key)
	if name != "ITEST Balai Desa Baru" || key != "ITEST BALAI DESA BARU" {
		t.Fatalf("renamed name=%q key=%q", name, key)
	}
	if err := repository.AssignHandoverLocation(ctx, auth.Principal{}, scheduleID, slotID, "00000000-0000-0000-0000-000000000000", scope, meta); !errors.Is(err, ErrHandoverLocationInvalid) {
		t.Fatalf("foreign location err=%v", err)
	}
}
