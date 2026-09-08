package dcp3

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseWorkbookReturnsFirstVisibleSheetAndSourceRows(t *testing.T) {
	data := workbookBytes(t, func(file *excelize.File) {
		file.SetSheetName("Sheet1", "Tersembunyi")
		index, err := file.NewSheet("DCP3 Petani")
		if err != nil {
			t.Fatal(err)
		}
		file.SetActiveSheet(index)
		if err := file.SetSheetVisible("Tersembunyi", false); err != nil {
			t.Fatal(err)
		}
		rows := [][]any{
			{" No ", "Nama", "NIK", "No Kartu Petani", "Alamat", "No HP"},
			{1, "Siti Aminah", "7312345678901234", "KP-01", "Wajo", "0812"},
			{"", "", "", "", "", ""},
			{2, "Hasan", "7312345678901235", "KP-02", "Bone", "0813"},
		}
		for rowIndex, row := range rows {
			for columnIndex, value := range row {
				cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
				_ = file.SetCellValue("DCP3 Petani", cell, value)
			}
		}
		_ = file.SetCellFormula("DCP3 Petani", "F4", `CONCAT("08","13")`)
	})

	preview, err := ParseWorkbook(bytes.NewReader(data), ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SheetName != "DCP3 Petani" || len(preview.Headers) != 6 || preview.Headers[0] != "No" {
		t.Fatalf("unexpected header preview: %+v", preview)
	}
	if len(preview.Rows) != 2 || preview.Rows[0].SourceRowNumber != 2 || preview.Rows[1].SourceRowNumber != 4 {
		t.Fatalf("unexpected rows: %+v", preview.Rows)
	}
	if preview.Rows[1].Values[5] != `=CONCAT("08","13")` {
		t.Fatalf("formula was evaluated instead of preserved: %q", preview.Rows[1].Values[5])
	}
}

func TestParseWorkbookRejectsDuplicateHeaders(t *testing.T) {
	data := workbookBytes(t, func(file *excelize.File) {
		_ = file.SetSheetRow("Sheet1", "A1", &[]any{"Nama", " nama "})
	})
	_, err := ParseWorkbook(bytes.NewReader(data), ParseLimits{MaxBytes: 1 << 20, MaxRows: 10, MaxColumns: 10}, 0)
	if !errors.Is(err, ErrHeadersInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestParseWorkbookEnforcesBounds(t *testing.T) {
	tests := []struct {
		name   string
		limits ParseLimits
		want   error
	}{
		{name: "bytes", limits: ParseLimits{MaxBytes: 8, MaxRows: 10, MaxColumns: 10}, want: ErrWorkbookTooLarge},
		{name: "rows", limits: ParseLimits{MaxBytes: 1 << 20, MaxRows: 1, MaxColumns: 10}, want: ErrTooManyRows},
		{name: "columns", limits: ParseLimits{MaxBytes: 1 << 20, MaxRows: 10, MaxColumns: 1}, want: ErrTooManyColumns},
	}
	data := workbookBytes(t, func(file *excelize.File) {
		_ = file.SetSheetRow("Sheet1", "A1", &[]any{"No", "Nama"})
		_ = file.SetSheetRow("Sheet1", "A2", &[]any{1, "Siti"})
		_ = file.SetSheetRow("Sheet1", "A3", &[]any{2, "Hasan"})
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseWorkbook(bytes.NewReader(data), test.limits, 0)
			if !errors.Is(err, test.want) {
				t.Fatalf("want %v, got %v", test.want, err)
			}
		})
	}
}

func TestParseWorkbookRejectsMalformedFile(t *testing.T) {
	_, err := ParseWorkbook(strings.NewReader("not an xlsx file"), ParseLimits{MaxBytes: 1024, MaxRows: 10, MaxColumns: 10}, 0)
	if !errors.Is(err, ErrWorkbookInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestParseWorkbookUsesSpecifiedHeaderRowIndex(t *testing.T) {
	data := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{
			{"USULAN CALON PENERIMA BANTUAN"},
			{"KABUPATEN SUKABUMI"},
			{"No", "Nama", "NIK"},
			{1, "Siti Aminah", "7312345678901234"},
			{2, "Hasan", "7312345678901235"},
		}
		for rowIndex, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", rowIndex+1), &row)
		}
	})

	preview, err := ParseWorkbook(bytes.NewReader(data), ParseLimits{MaxBytes: 1 << 20, MaxRows: 10, MaxColumns: 10}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Headers) != 3 || preview.Headers[0] != "No" || preview.Headers[1] != "Nama" {
		t.Fatalf("headers=%+v", preview.Headers)
	}
	if len(preview.Rows) != 2 || preview.Rows[0].SourceRowNumber != 4 || preview.Rows[1].SourceRowNumber != 5 {
		t.Fatalf("rows=%+v", preview.Rows)
	}
}

func TestParseWorkbookRejectsOutOfRangeHeaderRowIndex(t *testing.T) {
	data := workbookBytes(t, func(file *excelize.File) {
		_ = file.SetSheetRow("Sheet1", "A1", &[]any{"No", "Nama"})
		_ = file.SetSheetRow("Sheet1", "A2", &[]any{1, "Siti"})
	})
	limits := ParseLimits{MaxBytes: 1 << 20, MaxRows: 10, MaxColumns: 10}
	if _, err := ParseWorkbook(bytes.NewReader(data), limits, -1); !errors.Is(err, ErrHeadersInvalid) {
		t.Fatalf("negative index err=%v", err)
	}
	if _, err := ParseWorkbook(bytes.NewReader(data), limits, 5); !errors.Is(err, ErrHeadersInvalid) {
		t.Fatalf("out-of-range index err=%v", err)
	}
}

func TestRawRowsReturnsUpToRequestedRowCountWithoutHeaderValidation(t *testing.T) {
	data := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{
			{"USULAN CALON PENERIMA BANTUAN"},
			{"KABUPATEN SUKABUMI"},
			{"No", "Nama", "NIK"},
			{1, "Siti Aminah", "7312345678901234"},
			{2, "Hasan", "7312345678901235"},
		}
		for rowIndex, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", rowIndex+1), &row)
		}
	})

	rows, err := RawRows(bytes.NewReader(data), ParseLimits{MaxBytes: 1 << 20, MaxRows: 10, MaxColumns: 10}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 raw rows, got %d: %+v", len(rows), rows)
	}
	if rows[0][0] != "USULAN CALON PENERIMA BANTUAN" || rows[2][0] != "No" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestRawRowsRejectsMalformedFile(t *testing.T) {
	_, err := RawRows(strings.NewReader("not an xlsx file"), ParseLimits{MaxBytes: 1024, MaxRows: 10, MaxColumns: 10}, 10)
	if !errors.Is(err, ErrWorkbookInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func workbookBytes(t *testing.T, setup func(*excelize.File)) []byte {
	t.Helper()
	file := excelize.NewFile()
	setup(file)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
