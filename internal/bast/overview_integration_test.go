package bast

import (
	"context"
	"strings"
	"testing"

	"konkit/internal/auth"
)

func TestIntegrationOverviewSummarizesFinalStaleAndWetScans(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID, programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID); err != nil {
		t.Fatal(err)
	}
	finalDate, staleDate := "2026-10-05", "2026-10-06"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_aggregate_documents WHERE schedule_id=$1 AND document_date IN ($2,$3)`, scheduleID, finalDate, staleDate)
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_wet_signature_scans WHERE schedule_id=$1 AND document_date=$2`, scheduleID, finalDate)
	})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "overview-integration-test"}
	for i, date := range []string{finalDate, staleDate} {
		activated, err := repository.ActivateAggregate(ctx, auth.Principal{}, AggregateActivation{
			ScheduleID: scheduleID, ProgramID: programID, RegencyID: regencyID,
			DocumentType: AggregateDocumentDP3, DocumentDate: date, Filename: "DP3 - OVERVIEW.pdf",
			RecipientCount: 1, PageCount: 1, Checksum: strings.Repeat(string(rune('d'+i)), 64), StorageKey: "overview-" + date,
			Snapshot: []byte(`{"document_type":"dp3"}`), ExpectedVersion: 1,
		}, meta)
		if err != nil {
			t.Fatal(err)
		}
		if date == staleDate {
			if _, err := pool.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale' WHERE id=$1`, activated.Document.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bast_wet_signature_scans(schedule_id, document_key, document_date, content_code, image_png, byte_size) VALUES ($1,'dp3',$2,'CODE',$3,1)`, scheduleID, finalDate, []byte{1}); err != nil {
		t.Fatal(err)
	}

	overview, err := repository.Overview(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	dp3 := overview.Types["dp3"]
	if dp3 == nil || dp3.Dates[finalDate] != OverviewFinal || dp3.Dates[staleDate] != OverviewStale || !dp3.WetSupported {
		t.Fatalf("dp3=%+v", dp3)
	}
	if len(dp3.WetScans) != 1 || dp3.WetScans[0] != finalDate {
		t.Fatalf("wet scans=%+v", dp3.WetScans)
	}
	for _, key := range []string{"individual", "rakorda", "training-100", "pemeriksaan"} {
		if overview.Types[key] == nil {
			t.Fatalf("missing type %s", key)
		}
	}
	if overview.Types["rakorda"].WetSupported {
		t.Fatal("rakorda has no wet signature mode")
	}

	if _, err := repository.Overview(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); err != ErrNotFound {
		t.Fatalf("out of scope err=%v", err)
	}
}
