package bast

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateClosingRowsRequiresMachineData(t *testing.T) {
	if err := validateClosingRows(nil); !errors.Is(err, ErrAggregateNoRecipients) {
		t.Fatalf("empty err=%v", err)
	}
	if err := validateClosingRows([]ClosingRow{{LocalDate: "2026-10-01", MachineBrand: "", MachineType: "T", Count: 1}}); !errors.Is(err, ErrVerificationSnapshotIncomplete) {
		t.Fatalf("missing brand err=%v", err)
	}
	if err := validateClosingRows([]ClosingRow{{LocalDate: "2026-10-01", MachineBrand: "B", MachineType: "T", Count: 1}}); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestFormatClosingDocumentNumber(t *testing.T) {
	got := formatClosingDocumentNumber("wjo", "2026-10-03", 23, 1)
	if got != "01/23/WJO/KKT/CTS/X/2026" {
		t.Fatalf("document number=%q", got)
	}
}

func TestFormatClosingDocumentNumberPadsToTotalDigitWidth(t *testing.T) {
	got := formatClosingDocumentNumber("bgk", "2026-10-03", 200, 7)
	if got != "007/200/BGK/KKT/CTS/X/2026" {
		t.Fatalf("document number=%q", got)
	}
}

func TestFormatClosingFilename(t *testing.T) {
	got := formatClosingFilename("  Kabupaten   Wajo ", "2026-10-03", 2)
	if got != "CLOSING TITIK SERAH - KABUPATEN WAJO - 2026-10-03 - V2.pdf" {
		t.Fatalf("filename=%q", got)
	}
}

func sampleClosingSnapshot() ClosingTitikSerahSnapshot {
	rows := []ClosingRow{
		{LocalDate: "2026-10-01", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 12},
		{LocalDate: "2026-10-02", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 8},
	}
	snapshot := ClosingTitikSerahSnapshot{
		DocumentType: AggregateDocumentClosingTitikSerah, DocumentDate: "2026-10-03", RegencyName: "Kabupaten Wajo",
		RegencyCode: "WJO", ProvinceName: "Sulawesi Selatan",
		HandoverLocation: "Lapangan Desa Tempe", ConsultantCompanyName: "PT Kian Santang Muliatama Tbk.", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: closingSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Budi", SupervisorName: "Andi", PertaminaRepName: "Rian"},
		Rows:        rows,
		GrandTotal:  20,
	}
	snapshot.DocumentNumber = formatClosingDocumentNumber(snapshot.RegencyCode, snapshot.DocumentDate, snapshot.GrandTotal, 1)
	return snapshot
}

func TestRenderClosingTitikSerahProducesPDF(t *testing.T) {
	rendered, err := RenderClosingTitikSerah(sampleClosingSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.PDF) < 5 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatal("not a PDF")
	}
	if rendered.PageCount < 2 {
		t.Fatalf("page count=%d, expected table + signature page", rendered.PageCount)
	}
}

func TestRenderClosingTitikSerahIncludesProcurementHeadingUnderlinedTitleAndTotal(t *testing.T) {
	rendered, err := RenderClosingTitikSerah(sampleClosingSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{
		"PENGADAAN BARANG", "LIQUEFIED", "PETROLEUM GAS", "TAHUN ANGGARAN 2026", "PT PERTAMINA PATRA NIAGA",
		closingTitle, "FORM REKAPITULASI CLOSING LOKASI / TITIK SERAH", "01/20/WJO/KKT/CTS/X/2026", "Jumlah/Total", "PADA HARI INI",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered closing titik serah is missing %q", want)
		}
	}
}

// TestRenderClosingTitikSerahLongRowsNeverOverflowPageBottom guards against
// row height being checked after drawing instead of before.
func TestRenderClosingTitikSerahLongRowsNeverOverflowPageBottom(t *testing.T) {
	rows := make([]ClosingRow, 0, 40)
	for i := 0; i < 40; i++ {
		rows = append(rows, ClosingRow{
			LocalDate: fmt.Sprintf("2026-10-%02d", (i%28)+1), MachineBrand: "MEREK PANJANG NOMOR ANGKA", MachineType: "TIPE MESIN DENGAN DESKRIPSI YANG CUKUP PANJANG", Count: i + 1,
		})
	}
	snapshot := sampleClosingSnapshot()
	snapshot.Rows = rows
	rendered, err := RenderClosingTitikSerah(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount < 3 {
		t.Fatalf("expected many long rows to force pagination, got page count=%d", rendered.PageCount)
	}
}

// TestRenderClosingTitikSerahSignatureNamesWrapInsteadOfOverlapping guards
// against long signatory names in the 4-column signature grid overflowing
// into the next column.
func TestRenderClosingTitikSerahSignatureNamesWrapInsteadOfOverlapping(t *testing.T) {
	snapshot := sampleClosingSnapshot()
	snapshot.Signatories = closingSignatories{
		AgricultureOfficeName: "Dinas Pertanian Kabupaten Wajo", AgricultureOfficeNIP: "123",
		InstallerName: "Ahmad Fauzi", SupervisorName: "Andi Saputra Wijaya Kusuma", PertaminaRepName: "Rian Hidayat",
	}
	rendered, err := RenderClosingTitikSerah(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, glued := range []string{"WAJONama", "KUSUMANama"} {
		if strings.Contains(content, glued) {
			t.Fatalf("signature names overlap into the next column: found %q in rendered text", glued)
		}
	}
}

func TestRenderClosingTitikSerahUppercasesLegacyBusinessValues(t *testing.T) {
	snapshot := sampleClosingSnapshot()
	snapshot.RegencyName = "Kabupaten Wajo"
	snapshot.ProvinceName = "Sulawesi Selatan"
	snapshot.HandoverLocation = "Lapangan Desa"
	snapshot.Signatories.InstallerName = "Budi Santoso"
	snapshot.Signatories.SupervisorName = "Andi Saputra"
	snapshot.Rows = []ClosingRow{{LocalDate: "2026-10-01", MachineBrand: "Shark", MachineType: "Pompa Tani", Count: 1}}
	rendered, err := RenderClosingTitikSerah(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"KABUPATEN WAJO", "SULAWESI SELATAN", "LAPANGAN DESA", "POMPA TANI", "BUDI SANTOSO", "ANDI SAPUTRA"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered closing titik serah is missing uppercase legacy value %q", want)
		}
	}
	if strings.Contains(content, "Pompa Tani") || strings.Contains(content, "Budi Santoso") {
		t.Fatal("rendered closing titik serah still contains mixed-case legacy business values")
	}
}
