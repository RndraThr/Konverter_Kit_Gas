package bast

import (
	"context"
	"errors"
	"strings"
	"testing"

	"konkit/internal/auth"
)

// TestIntegrationGasElpijiReceiptsListAndCancel memastikan constraint menerima
// jenis gas_elpiji, daftar penerimaan aktif urut tanggal beserta snapshot, dan
// pembatalan hanya mengenai penerimaan aktif.
func TestIntegrationGasElpijiReceiptsListAndCancel(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID, programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_type IN ('gas_elpiji','gas_elpiji_gabungan')`, scheduleID)
	})
	scope := auth.RegencyScope{Unrestricted: true}
	meta := auth.ClientMeta{UserAgent: "gas-elpiji-integration-test"}
	activate := func(date, checksum string) AggregateDocument {
		t.Helper()
		result, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
			ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
			DocumentType: AggregateDocumentGasElpiji, DocumentDate: date, Filename: "BA GAS ELPIJI - " + date + " - V1.pdf",
			RecipientCount: 10, PageCount: 1, Checksum: strings.Repeat(checksum, 64), StorageKey: "key-" + date,
			Snapshot: []byte(`{"document_type":"gas_elpiji","document_date":"` + date + `","cylinder_count":10}`), ExpectedVersion: 1,
		}, meta)
		if err != nil {
			t.Fatal(err)
		}
		return result.Document
	}
	later := activate("2026-12-12", "b")
	earlier := activate("2026-12-02", "a")

	items, err := repository.ListActiveAggregatesWithSnapshot(ctx, scheduleID, AggregateDocumentGasElpiji, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Document.ID != earlier.ID || items[1].Document.ID != later.ID || !strings.Contains(string(items[0].Snapshot), `"2026-12-02"`) {
		t.Fatalf("items=%+v", items)
	}

	// Pembatalan memerlukan jenis yang cocok dan dokumen yang masih aktif.
	if _, err := repository.RetireAggregate(ctx, auth.Principal{}, later.ID, AggregateDocumentGasElpijiMerged, scope, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong type err=%v", err)
	}
	retired, err := repository.RetireAggregate(ctx, auth.Principal{}, later.ID, AggregateDocumentGasElpiji, scope, meta)
	if err != nil || retired.StorageKey != "key-2026-12-12" || retired.Status != "superseded" {
		t.Fatalf("retired=%+v err=%v", retired, err)
	}
	if _, err := repository.RetireAggregate(ctx, auth.Principal{}, later.ID, AggregateDocumentGasElpiji, scope, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second cancel err=%v", err)
	}
	if _, err := repository.RetireAggregate(ctx, auth.Principal{}, earlier.ID, AggregateDocumentGasElpiji, auth.RegencyScope{RegencyIDs: []string{}}, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope err=%v", err)
	}
	items, err = repository.ListActiveAggregatesWithSnapshot(ctx, scheduleID, AggregateDocumentGasElpiji, scope)
	if err != nil || len(items) != 1 || items[0].Document.ID != earlier.ID {
		t.Fatalf("after cancel items=%+v err=%v", items, err)
	}

	// Draf isian: simpan, timpa, baca, daftar, lalu hapus.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_gas_elpiji_drafts WHERE schedule_id=$1`, scheduleID)
	})
	draft := GasElpijiInput{ScheduleID: scheduleID, DocumentDate: "2026-12-20", WarehouseLocation: "SPBE", CylinderCount: 5}
	if err := repository.SaveGasElpijiDraft(ctx, auth.Principal{}, draft); err != nil {
		t.Fatal(err)
	}
	draft.CylinderCount = 7
	if err := repository.SaveGasElpijiDraft(ctx, auth.Principal{}, draft); err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetGasElpijiDraft(ctx, scheduleID, "2026-12-20")
	if err != nil || got.CylinderCount != 7 || got.WarehouseLocation != "SPBE" {
		t.Fatalf("draft=%+v err=%v", got, err)
	}
	drafts, err := repository.ListGasElpijiDrafts(ctx, scheduleID)
	if err != nil || len(drafts) != 1 {
		t.Fatalf("drafts=%+v err=%v", drafts, err)
	}
	if err := repository.DeleteGasElpijiDraft(ctx, scheduleID, "2026-12-20"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetGasElpijiDraft(ctx, scheduleID, "2026-12-20"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted draft err=%v", err)
	}
	if scan, code, err := repository.GetWetScanImage(ctx, scheduleID, gasElpijiDocumentKey, "2026-12-20"); err != nil || scan != nil || code != "" {
		t.Fatalf("missing scan=%v code=%q err=%v", scan, code, err)
	}
}
