package bast

import (
	"context"
	"strings"
	"testing"

	"konkit/internal/auth"
)

func TestIntegrationAggregateDocumentLifecycleIncludingStaleHistory(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID, programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID); err != nil {
		t.Fatal(err)
	}
	documentDate := "2026-10-02"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_date=$2`, scheduleID, documentDate)
	})
	scope := auth.RegencyScope{Unrestricted: true}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "aggregate-integration-test"}
	checksumA := strings.Repeat("a", 64)
	checksumB := strings.Repeat("b", 64)

	// Versi pertama aktif.
	first, err := repository.ActivateAggregate(ctx, auth.Principal{UserID: ""}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: "DP3 - TEST - V1.pdf",
		RecipientCount: 2, PageCount: 2, Checksum: checksumA, StorageKey: "key-1", Snapshot: []byte(`{"document_type":"dp3"}`),
		ExpectedVersion: 1, ExpectedActiveID: "",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if first.Document.Version != 1 || first.Document.Status != "active" || first.Unchanged {
		t.Fatalf("first activation=%+v", first)
	}

	// Idempoten: checksum sama mengembalikan versi aktif tanpa versi baru.
	again, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: "DP3 - TEST - V1.pdf",
		RecipientCount: 2, PageCount: 2, Checksum: checksumA, StorageKey: "key-1-dup", Snapshot: []byte(`{"document_type":"dp3"}`),
		ExpectedVersion: 2, ExpectedActiveID: first.Document.ID,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Unchanged || again.Document.ID != first.Document.ID {
		t.Fatalf("idempotent activation=%+v", again)
	}

	// Konten berubah → versi baru, versi lama superseded.
	second, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: "DP3 - TEST - V2.pdf",
		RecipientCount: 3, PageCount: 2, Checksum: checksumB, StorageKey: "key-2", Snapshot: []byte(`{"document_type":"dp3","v":2}`),
		ExpectedVersion: 2, ExpectedActiveID: first.Document.ID,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if second.Document.Version != 2 || second.Document.Status != "active" || second.OldStorageKey != "key-1" {
		t.Fatalf("second activation=%+v", second)
	}

	versions, err := repository.ListAggregates(ctx, scheduleID, AggregateDocumentDP3, documentDate, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Version != 2 || versions[1].Version != 1 || versions[1].Status != "superseded" {
		t.Fatalf("versions=%+v", versions)
	}

	active, err := repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentDP3, documentDate, scope)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != second.Document.ID || active.Version != 2 {
		t.Fatalf("active=%+v", active)
	}

	// Conflict: expected version berubah oleh operator lain.
	if _, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: "DP3 - TEST - V9.pdf",
		RecipientCount: 4, PageCount: 2, Checksum: strings.Repeat("c", 64), StorageKey: "key-3", Snapshot: []byte(`{}`),
		ExpectedVersion: 3, ExpectedActiveID: first.Document.ID,
	}, meta); err == nil {
		t.Fatal("expected conflict when expected active id is stale")
	}

	if _, err := pool.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale' WHERE id=$1`, second.Document.ID); err != nil {
		t.Fatal(err)
	}
	stale, err := repository.GetAggregateByID(ctx, second.Document.ID, scope)
	if err != nil || stale.Status != "stale" {
		t.Fatalf("stale document=%+v err=%v", stale, err)
	}
	third, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: "DP3 - TEST - V3.pdf",
		RecipientCount: 3, PageCount: 2, Checksum: strings.Repeat("d", 64), StorageKey: "key-3", Snapshot: []byte(`{"document_type":"dp3","v":3}`),
		ExpectedVersion: 3, ExpectedActiveID: "",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if third.Document.Version != 3 || third.Document.Status != "active" {
		t.Fatalf("third activation=%+v", third)
	}
}
