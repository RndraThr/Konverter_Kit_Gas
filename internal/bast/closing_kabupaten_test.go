package bast

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateClosingKabupatenRowsRequiresMachineData(t *testing.T) {
	if err := validateClosingKabupatenRows(nil); !errors.Is(err, ErrAggregateNoRecipients) {
		t.Fatalf("empty err=%v", err)
	}
	if err := validateClosingKabupatenRows([]ClosingKabupatenRow{{Location: "L", MachineBrand: "", MachineType: "T", Count: 1}}); !errors.Is(err, ErrVerificationSnapshotIncomplete) {
		t.Fatalf("missing brand err=%v", err)
	}
	if err := validateClosingKabupatenRows([]ClosingKabupatenRow{{Location: "L", MachineBrand: "B", MachineType: "T", Count: 1}}); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestFormatClosingKabupatenDocumentNumber(t *testing.T) {
	got := formatClosingKabupatenDocumentNumber("wjo", "2026-10-03", 25, 1)
	if got != "001/25/WJO/KKT/CK/X/2026" {
		t.Fatalf("document number=%q", got)
	}
}

func TestFormatClosingKabupatenFilename(t *testing.T) {
	got := formatClosingKabupatenFilename("  Kabupaten   Wajo ", "2026-10-03", 2)
	if got != "CLOSING KABUPATEN - KABUPATEN WAJO - 2026-10-03 - V2.pdf" {
		t.Fatalf("filename=%q", got)
	}
}

func sampleClosingKabupatenSnapshot() ClosingKabupatenSnapshot {
	rows := []ClosingKabupatenRow{
		{Location: "Lapangan Desa Tempe", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 20},
		{Location: "Lapangan Desa Tempe", MachineBrand: "YANMAR", MachineType: "TF 85", Count: 5},
	}
	snapshot := ClosingKabupatenSnapshot{
		DocumentType: AggregateDocumentClosingKabupaten, DocumentDate: "2026-10-03", RegencyName: "Kabupaten Wajo",
		RegencyCode: "WJO", ProvinceName: "Sulawesi Selatan",
		ConsultantCompanyName: "PT Kian Santang Muliatama Tbk.", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: closingSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Budi", SupervisorName: "Andi", PertaminaRepName: "Rian"},
		Rows:        rows,
		GrandTotal:  25,
	}
	snapshot.DocumentNumber = formatClosingKabupatenDocumentNumber(snapshot.RegencyCode, snapshot.DocumentDate, snapshot.GrandTotal, 1)
	return snapshot
}

func TestRenderClosingKabupatenProducesPDF(t *testing.T) {
	rendered, err := RenderClosingKabupaten(sampleClosingKabupatenSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
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

func TestRenderClosingKabupatenIncludesProcurementHeadingUnderlinedTitleAndTotal(t *testing.T) {
	rendered, err := RenderClosingKabupaten(sampleClosingKabupatenSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{
		"PENGADAAN BARANG", "LIQUEFIED", "PETROLEUM GAS", "TAHUN ANGGARAN 2026", "PT PERTAMINA PATRA NIAGA",
		closingKabupatenTitle, "FORM REKAPITULASI CLOSING KABUPATEN / KOTA", "001/25/WJO/KKT/CK/X/2026", "Jumlah/Total", "PADA HARI INI",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered closing kabupaten is missing %q", want)
		}
	}
}

// TestRenderClosingKabupatenLongRowsNeverOverflowPageBottom guards against
// row height being checked after drawing instead of before.
func TestRenderClosingKabupatenLongRowsNeverOverflowPageBottom(t *testing.T) {
	rows := make([]ClosingKabupatenRow, 0, 40)
	for i := 0; i < 40; i++ {
		rows = append(rows, ClosingKabupatenRow{
			Location: "LOKASI DENGAN NAMA SANGAT PANJANG UNTUK MENGUJI PEMBUNGKUSAN TEKS", MachineBrand: "MEREK PANJANG NOMOR ANGKA", MachineType: "TIPE MESIN DENGAN DESKRIPSI YANG CUKUP PANJANG", Count: i + 1,
		})
	}
	snapshot := sampleClosingKabupatenSnapshot()
	snapshot.Rows = rows
	rendered, err := RenderClosingKabupaten(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount < 3 {
		t.Fatalf("expected many long rows to force pagination, got page count=%d", rendered.PageCount)
	}
}

// TestRenderClosingKabupatenSignatureNamesWrapInsteadOfOverlapping guards
// against long signatory names in the 4-column signature grid overflowing
// into the next column.
func TestRenderClosingKabupatenSignatureNamesWrapInsteadOfOverlapping(t *testing.T) {
	snapshot := sampleClosingKabupatenSnapshot()
	snapshot.Signatories = closingSignatories{
		AgricultureOfficeName: "Dinas Pertanian Kabupaten Wajo", AgricultureOfficeNIP: "123",
		InstallerName: "Ahmad Fauzi", SupervisorName: "Andi Saputra Wijaya Kusuma", PertaminaRepName: "Rian Hidayat",
	}
	rendered, err := RenderClosingKabupaten(snapshot, map[string][]byte{"logo-1": testPNG(t)})
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

func TestRenderClosingKabupatenUppercasesLegacyBusinessValues(t *testing.T) {
	snapshot := sampleClosingKabupatenSnapshot()
	snapshot.RegencyName = "Kabupaten Wajo"
	snapshot.ProvinceName = "Sulawesi Selatan"
	snapshot.Signatories.InstallerName = "Budi Santoso"
	snapshot.Signatories.SupervisorName = "Andi Saputra"
	snapshot.Rows = []ClosingKabupatenRow{{Location: "Lapangan Desa", MachineBrand: "Shark", MachineType: "Pompa Tani", Count: 1}}
	rendered, err := RenderClosingKabupaten(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"KABUPATEN WAJO", "SULAWESI SELATAN", "LAPANGAN DESA", "POMPA TANI", "BUDI SANTOSO", "ANDI SAPUTRA"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered closing kabupaten is missing uppercase legacy value %q", want)
		}
	}
	if strings.Contains(content, "Pompa Tani") || strings.Contains(content, "Budi Santoso") {
		t.Fatal("rendered closing kabupaten still contains mixed-case legacy business values")
	}
}

func TestGroupClosingKabupatenRowsKeepsFirstAppearanceOrder(t *testing.T) {
	snapshotA := []byte(`{"equipment":{"machine_brand":"SHARK","machine_type":"SPWP"}}`)
	snapshotB := []byte(`{"equipment":{"machine_brand":"YANMAR","machine_type":"TF"}}`)
	rows := groupClosingKabupatenRows([][]byte{snapshotA, snapshotA, snapshotB})
	if len(rows) != 2 || rows[0].MachineBrand != "SHARK" || rows[0].Count != 2 || rows[1].MachineBrand != "YANMAR" || rows[1].Count != 1 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestWithClosingKabupatenLocationFillsEveryRow(t *testing.T) {
	rows := withClosingKabupatenLocation([]ClosingKabupatenRow{{MachineBrand: "SHARK"}, {MachineBrand: "YANMAR"}}, "Lapangan Desa Tempe")
	for _, row := range rows {
		if row.Location != "Lapangan Desa Tempe" {
			t.Fatalf("row location not filled: %+v", row)
		}
	}
}
