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

func TestIntegrationRemoveImportUnlinksOpenSlotsAndKeepsDistributed(t *testing.T) {
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
	if _, err := service.RemoveImport(ctx, auth.Principal{}, batchID, true, meta, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrImportNotFound) {
		t.Fatalf("out of scope err = %v", err)
	}

	preview, err := service.RemoveImport(ctx, auth.Principal{}, batchID, true, meta, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Removed != 3 || len(preview.Unlinked) != 1 || *preview.Unlinked[0].SlotNumber != 1 || len(preview.Kept) != 1 || preview.Kept[0].FullName != "CICI" || preview.ImportDeleted {
		t.Fatalf("preview = %+v", preview)
	}
	var count int
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&count))
	if count != 4 {
		t.Fatalf("a dry run changed data: %d allocations", count)
	}

	if _, err := service.RemoveImport(ctx, auth.Principal{}, batchID, false, meta, scope); err != nil {
		t.Fatal(err)
	}
	var status string
	var allocation *string
	must(pool.QueryRow(ctx, `SELECT status, allocation_id::text FROM distribution_slots WHERE schedule_id=$1 AND slot_number=1`, scheduleID).Scan(&status, &allocation))
	if status != "open" || allocation != nil {
		t.Fatalf("slot 1 = %s %v, want open and unlinked", status, allocation)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&count))
	if count != 1 {
		t.Fatalf("allocations left = %d, want only the distributed one", count)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM people WHERE nik = ANY($1)`, []string{nikA, nikB, nikD}).Scan(&count))
	if count != 0 {
		t.Fatalf("people of removed rows left = %d", count)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM people WHERE nik = $1`, nikC).Scan(&count))
	if count != 1 {
		t.Fatal("the distributed recipient was removed")
	}
	imports, err = service.ListImports(ctx, scheduleID, scope)
	must(err)
	if len(imports) != 1 || imports[0].Recipients != 1 {
		t.Fatalf("imports after removal = %+v", imports)
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
	result, err := service.RemoveImport(ctx, auth.Principal{}, batchID, false, meta, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ImportDeleted || result.Removed != 1 {
		t.Fatalf("removal = %+v", result)
	}
	var batches int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID).Scan(&batches); err != nil || batches != 0 {
		t.Fatalf("batches left = %d err=%v", batches, err)
	}
	commit() // the same file imports again
}
