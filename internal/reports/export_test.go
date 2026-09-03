package reports

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestBuildExcelWritesHeaderAndRows(t *testing.T) {
	completedAt := time.Date(2026, 9, 10, 8, 30, 0, 0, time.UTC)
	data, err := buildExcel([]Row{
		{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001", SectorIdentifier: "KP01", Village: "Tempe", District: "Sabbangparu", AllocationStatus: "distributed", DistributionStatus: "completed", DocumentationComplete: true, CompletedAt: &completedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := file.GetRows("Laporan")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
	if rows[0][0] != "No. Pembagian" || rows[0][1] != "Nama" {
		t.Fatalf("header=%v", rows[0])
	}
	if rows[1][1] != "Siti Aminah" || rows[1][2] != "7306014101900001" || rows[1][8] != "Lengkap" {
		t.Fatalf("data row=%v", rows[1])
	}
}

func TestBuildPDFProducesNonEmptyDocument(t *testing.T) {
	summary := Summary{TotalAllocations: 1, AllocationStatusCounts: []StatusCount{{Status: "distributed", Count: 1}}, DistributionStatusCounts: []StatusCount{{Status: "completed", Count: 1}}}
	data, err := buildPDF(summary, []Row{{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("not a pdf, first bytes=%q", data[:min(len(data), 16)])
	}
}
