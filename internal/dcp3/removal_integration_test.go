package dcp3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"

	"github.com/xuri/excelize/v2"
)

func TestIntegrationRemoveImportDefaultsToSafeRecipientsAndRemovesChosenOnes(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	seed := time.Now().UnixNano() % 100000000000000
	nik := func(n int64) string { return fmt.Sprintf("75%014d", (seed+n)%100000000000000) }
	nikA, nikB, nikC, nikD := nik(0), nik(1), nik(2), nik(3)
	scheduleID, _, regencyID := createDCP3ScheduleFixture(t, pool, nikA, nikB, nikC, nikD)
	service := NewImportService(NewRepository(pool), ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-integration-test"}
	scope := auth.RegencyScope{Unrestricted: true}
	mapping := Mapping{FullName: "Nama", NIK: "NIK", Address: "Alamat", Village: "Desa", PhoneNumber: "No HP"}
	importRows := func(name string, rows [][]any) string {
		t.Helper()
		workbook := workbookBytes(t, func(file *excelize.File) {
			for index, row := range rows {
				_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
			}
		})
		preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, name, bytes.NewReader(workbook), meta, scope, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Commit(ctx, auth.Principal{}, preview.ID, mapping, meta, scope); err != nil {
			t.Fatal(err)
		}
		return preview.ID
	}
	header := []any{"Nama", "NIK", "Alamat", "Desa", "No HP"}
	batchID := importRows("dcp3-salah-kabupaten.xlsx", [][]any{header,
		{"Ani", nikA, "Jalan", "Tempe", "0811"},
		{"Budi", nikB, "Jalan", "Tempe", "0812"},
		{"Cici", nikC, "Jalan", "Tempe", "0813"},
		{"Dodi", nikD, "Jalan", "Tempe", "0814"},
	})
	link := func(slotNumber int, status, nik string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,allocation_id,recipient_person_id)
			SELECT pa.schedule_id, $3, $4, pa.id, pa.intended_person_id FROM package_allocations pa JOIN people p ON p.id=pa.intended_person_id
			WHERE pa.schedule_id=$1 AND p.nik=$2`, scheduleID, nik, slotNumber, status); err != nil {
			t.Fatal(err)
		}
	}
	link(1, "linked", nikB) // Budi is on nomor bagi 1, not completed.
	link(2, "completed", nikC)
	if _, err := pool.Exec(ctx, `UPDATE package_allocations pa SET status='distributed' FROM people p WHERE p.id=pa.intended_person_id AND p.nik=$1`, nikC); err != nil {
		t.Fatal(err)
	}

	imports, err := service.ListImports(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(imports) != 1 || imports[0].Recipients != 4 || imports[0].Linked != 1 || imports[0].Distributed != 1 {
		t.Fatalf("imports = %+v", imports)
	}
	if _, err := service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{}, true, meta, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrImportNotFound) {
		t.Fatalf("out of scope err = %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	preview, err := service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{}, true, meta, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	must(err)
	states := map[string]RemovalRecipient{}
	for _, r := range preview.Recipients {
		states[r.FullName] = r
	}
	if len(preview.Recipients) != 4 || !states["ANI"].Selected || !states["DODI"].Selected || states["BUDI"].Selected || states["CICI"].Selected ||
		states["BUDI"].State != RemovalLinked || states["CICI"].State != RemovalCompleted || states["CICI"].Warning == "" || states["ANI"].Warning != "" {
		t.Fatalf("preview recipients = %+v", preview.Recipients)
	}
	if preview.Removed != 2 || preview.Kept != 2 || preview.ImportDeleted {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{AllocationIDs: &[]string{"00000000-0000-0000-0000-000000000000"}}, false, meta, scope); !errors.Is(err, ErrRemovalSelection) {
		t.Fatalf("foreign recipient err = %v", err)
	}

	// The default choice removes only the recipients without a warning.
	result, err := service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{}, false, meta, scope)
	must(err)
	if result.Removed != 2 || result.Kept != 2 || result.ImportDeleted {
		t.Fatalf("default removal = %+v", result)
	}
	var count int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM people WHERE nik = ANY($1)`, []string{nikA, nikD}).Scan(&count))
	if count != 0 {
		t.Fatalf("people of removed rows left = %d", count)
	}
	var status string
	must(pool.QueryRow(ctx, `SELECT status FROM distribution_slots WHERE schedule_id=$1 AND slot_number=1`, scheduleID).Scan(&status))
	if status != "linked" {
		t.Fatalf("slot 1 = %s, an unchosen recipient must stay linked", status)
	}

	// Chosen on purpose: the linked and the distributed recipient go too.
	chosen := []string{states["BUDI"].AllocationID, states["CICI"].AllocationID}
	result, err = service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{AllocationIDs: &chosen}, false, meta, scope)
	must(err)
	if result.Removed != 2 || result.Unlinked != 2 || !result.ImportDeleted {
		t.Fatalf("chosen removal = %+v", result)
	}
	rows, err := pool.Query(ctx, `SELECT slot_number, status, allocation_id IS NULL, completed_at IS NULL FROM distribution_slots WHERE schedule_id=$1 ORDER BY slot_number`, scheduleID)
	must(err)
	for rows.Next() {
		var n int
		var st string
		var noAllocation, noCompletion bool
		must(rows.Scan(&n, &st, &noAllocation, &noCompletion))
		if st != "open" || !noAllocation || !noCompletion {
			t.Fatalf("slot %d = %s allocation-null=%v completed-null=%v", n, st, noAllocation, noCompletion)
		}
	}
	rows.Close()
	must(pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&count))
	if count != 0 {
		t.Fatalf("allocations left = %d", count)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID).Scan(&count))
	if count != 0 {
		t.Fatal("the emptied import was kept")
	}
}

func TestIntegrationRemoveImportDeletesTheFileSoItCanBeImportedAgain(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	seed := time.Now().UnixNano() % 100000000000000
	nikA := fmt.Sprintf("76%014d", seed)
	scheduleID, _, _ := createDCP3ScheduleFixture(t, pool, nikA)
	service := NewImportService(NewRepository(pool), ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-integration-test"}
	scope := auth.RegencyScope{Unrestricted: true}
	workbook := workbookBytes(t, func(file *excelize.File) {
		_ = file.SetSheetRow("Sheet1", "A1", &[]any{"Nama", "NIK", "Alamat", "Desa", "No HP"})
		_ = file.SetSheetRow("Sheet1", "A2", &[]any{"Eka", nikA, "Jalan", "Tempe", "0815"})
	})
	commit := func() string {
		t.Helper()
		preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "dcp3.xlsx", bytes.NewReader(workbook), meta, scope, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{FullName: "Nama", NIK: "NIK", Address: "Alamat", Village: "Desa", PhoneNumber: "No HP"}, meta, scope); err != nil {
			t.Fatal(err)
		}
		return preview.ID
	}
	batchID := commit()
	result, err := service.RemoveImport(ctx, auth.Principal{}, batchID, RemovalInput{}, false, meta, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ImportDeleted || result.Removed != 1 || result.Kept != 0 {
		t.Fatalf("removal = %+v", result)
	}
	var batches int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID).Scan(&batches); err != nil || batches != 0 {
		t.Fatalf("batches left = %d err=%v", batches, err)
	}
	commit() // the same file imports again
}
