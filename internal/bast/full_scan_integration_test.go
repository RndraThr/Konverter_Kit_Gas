package bast

import (
	"context"
	"testing"

	"konkit/internal/auth"
)

func TestIntegrationFullScanStorageAndOverview(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	var scheduleID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID); err != nil {
		t.Fatal(err)
	}
	date := "2026-10-07"
	scope := auth.RegencyScope{Unrestricted: true}
	meta := auth.ClientMeta{UserAgent: "full-scan-integration-test"}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_full_scans WHERE schedule_id=$1`, scheduleID)
	})

	// No scan yet: the BA is made by the system as usual.
	if exists, _, _, err := repository.ResolveFullScan(ctx, scheduleID, "tkdn", date); err != nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}

	if err := repository.PutFullScan(ctx, auth.Principal{}, scheduleID, "tkdn", date, "CODE1234", 2, []byte("%PDF-1.4 scan"), scope, meta); err != nil {
		t.Fatal(err)
	}
	exists, pdf, code, err := repository.ResolveFullScan(ctx, scheduleID, "tkdn", date)
	if err != nil || !exists || string(pdf) != "%PDF-1.4 scan" || code != "CODE1234" {
		t.Fatalf("resolve exists=%v pdf=%q code=%q err=%v", exists, pdf, code, err)
	}
	if scan, err := repository.GetFullScan(ctx, scheduleID, "tkdn", date, scope); err != nil || scan == nil || scan.PageCount != 2 {
		t.Fatalf("scan=%+v err=%v", scan, err)
	}

	overview, err := repository.Overview(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	tkdn := overview.Types["tkdn"]
	if tkdn == nil || !tkdn.FullScanSupported || len(tkdn.FullScans) != 1 || tkdn.FullScans[0] != date {
		t.Fatalf("tkdn=%+v", tkdn)
	}
	if individual := overview.Types["individual"]; individual == nil || !individual.FullScanSupported {
		t.Fatalf("individual=%+v", individual)
	}

	if err := repository.DeleteFullScan(ctx, auth.Principal{}, scheduleID, "tkdn", date, scope, meta); err != nil {
		t.Fatal(err)
	}
	if exists, _, _, _ := repository.ResolveFullScan(ctx, scheduleID, "tkdn", date); exists {
		t.Fatal("scan still there after delete")
	}
}
