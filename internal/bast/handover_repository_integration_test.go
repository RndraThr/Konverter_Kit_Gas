package bast

import (
	"context"
	"errors"
	"strings"
	"testing"

	"konkit/internal/auth"
)

// TestIntegrationHandoverLocations memastikan daftar titik serah tersimpan
// berurutan (termasuk pertukaran nama), dokumen BA Serah Terima per lokasi
// diterima constraint, perubahan titik serah menandai dokumen final perlu
// dibuat ulang, dan pindah manual hanya menerima lokasi jadwal itu.
func TestIntegrationHandoverLocations(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID, programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE schedule_id=$1 AND (document_type LIKE 'closing_titik_serah:%' OR document_type='closing_kabupaten')`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_handover_locations WHERE schedule_id=$1`, scheduleID)
	}
	cleanup()
	t.Cleanup(cleanup)
	scope := auth.RegencyScope{Unrestricted: true}
	meta := auth.ClientMeta{UserAgent: "handover-integration-test"}

	if err := repository.SaveHandoverLocations(ctx, auth.Principal{}, scheduleID, []HandoverLocationInput{
		{Name: "Balai Desa Tempe", Areas: []HandoverArea{{District: "TEMPE"}}},
		{Name: "Lapangan Sabbangparu", Areas: []HandoverArea{{District: "SABBANGPARU", Village: "MADDUKELLENG"}}},
	}, scope, meta); err != nil {
		t.Fatal(err)
	}
	locations, err := repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil || len(locations) != 2 || locations[0].Name != "Balai Desa Tempe" || locations[1].Areas[0].Village != "MADDUKELLENG" {
		t.Fatalf("locations=%+v err=%v", locations, err)
	}
	first, second := locations[0], locations[1]

	activate := func(locationID string) {
		t.Helper()
		if _, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
			ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
			DocumentType: closingDocumentType(locationID), DocumentDate: "2026-12-01", Filename: "CLOSING TITIK SERAH.pdf",
			RecipientCount: 3, PageCount: 1, Checksum: strings.Repeat("c", 64), StorageKey: "key-" + locationID,
			Snapshot: []byte(`{"document_type":"closing_titik_serah","handover_location":"BALAI","grand_total":3,"rows":[{"machine_brand":"SHARK","machine_type":"SPWP","count":3}]}`), ExpectedVersion: 1,
		}, meta); err != nil {
			t.Fatal(err)
		}
	}
	activate(first.ID)
	statuses, err := repository.HandoverDocumentStatuses(ctx, scheduleID)
	if err != nil || !statuses[first.ID].Active || statuses[first.ID].ActiveDate != "2026-12-01" {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	snapshots, err := repository.ListActiveClosingSnapshots(ctx, scheduleID)
	if err != nil || snapshots[first.ID].GrandTotal != 3 || snapshots[first.ID].Rows[0].MachineBrand != "SHARK" {
		t.Fatalf("snapshots=%+v err=%v", snapshots, err)
	}

	// Tukar nama dan urutan: indeks unik nama tidak boleh menolak.
	if err := repository.SaveHandoverLocations(ctx, auth.Principal{}, scheduleID, []HandoverLocationInput{
		{ID: second.ID, Name: "Balai Desa Tempe"},
		{ID: first.ID, Name: "Lapangan Sabbangparu", Areas: first.Areas},
	}, scope, meta); err != nil {
		t.Fatal(err)
	}
	locations, _ = repository.ListHandoverLocations(ctx, scheduleID, scope)
	if len(locations) != 2 || locations[0].ID != second.ID || locations[0].Name != "Balai Desa Tempe" || locations[1].Name != "Lapangan Sabbangparu" {
		t.Fatalf("swapped locations=%+v", locations)
	}
	statuses, _ = repository.HandoverDocumentStatuses(ctx, scheduleID)
	if statuses[first.ID].Active || !statuses[first.ID].Stale {
		t.Fatalf("closing document should be stale after change: %+v", statuses)
	}

	if err := repository.SaveHandoverLocations(ctx, auth.Principal{}, scheduleID, []HandoverLocationInput{{ID: "00000000-0000-0000-0000-000000000000", Name: "X"}}, scope, meta); !errors.Is(err, ErrHandoverLocationInvalid) {
		t.Fatalf("foreign id err=%v", err)
	}

	var slotID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM distribution_slots WHERE schedule_id=$1 AND recipient_person_id IS NOT NULL ORDER BY slot_number LIMIT 1`, scheduleID).Scan(&slotID); err == nil {
		if err := repository.SetHandoverOverride(ctx, auth.Principal{}, scheduleID, slotID, second.ID, scope, meta); err != nil {
			t.Fatal(err)
		}
		recipients, err := repository.ListHandoverRecipients(ctx, scheduleID, scope)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, recipient := range recipients {
			if recipient.SlotID == slotID {
				found = recipient.Override == second.ID
			}
		}
		if !found {
			t.Fatalf("override not listed for slot %s", slotID)
		}
		if err := repository.SetHandoverOverride(ctx, auth.Principal{}, scheduleID, slotID, "00000000-0000-0000-0000-000000000000", scope, meta); !errors.Is(err, ErrHandoverLocationInvalid) {
			t.Fatalf("foreign location err=%v", err)
		}
		if err := repository.SetHandoverOverride(ctx, auth.Principal{}, scheduleID, slotID, "", scope, meta); err != nil {
			t.Fatal(err)
		}
	}

	// Lokasi yang dihapus: dokumennya menjadi riwayat.
	if err := repository.SaveHandoverLocations(ctx, auth.Principal{}, scheduleID, []HandoverLocationInput{{ID: second.ID, Name: "Balai Desa Tempe"}}, scope, meta); err != nil {
		t.Fatal(err)
	}
	statuses, _ = repository.HandoverDocumentStatuses(ctx, scheduleID)
	if _, exists := statuses[first.ID]; exists {
		t.Fatalf("removed location documents should be superseded: %+v", statuses)
	}
}
