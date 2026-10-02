package bast

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"regexp"
	"strings"
	"testing"
)

func TestBundleFilenameUsesIndonesianDayAndDate(t *testing.T) {
	name, err := BundleFilename("2024-12-10")
	if err != nil {
		t.Fatal(err)
	}
	if name != "SELASA, 10 DESEMBER 2024.pdf" {
		t.Fatalf("filename=%q", name)
	}
}

func TestFormatIndonesianDateIncludesUppercaseWeekday(t *testing.T) {
	if got := formatIndonesianDate("2024-12-10"); got != "SELASA, 10 DESEMBER 2024" {
		t.Fatalf("date=%q", got)
	}
}

func TestRenderPetaniBundleUsesPrintableCheckmarkGlyph(t *testing.T) {
	document := makeRenderDocument(1, 1)
	result, err := RenderPetaniBundle(BundleRenderInput{
		LocalDate: "2024-12-10",
		Documents: []RecipientDocument{document},
		LogoBytes: map[string][]byte{"logo": testPNG(t)},
	})
	if err != nil {
		t.Fatal(err)
	}

	content := decodedPDFStreams(t, result.PDF)
	if !regexp.MustCompile(`\(3\)\s*Tj`).MatchString(content) {
		t.Fatal("rendered PDF does not use the ZapfDingbats checkmark glyph")
	}
	if regexp.MustCompile(`\(V\)\s*Tj`).MatchString(content) {
		t.Fatal("rendered PDF still uses the letter V as a checklist mark")
	}
}

func TestRenderPetaniBundleItalicizesLiquefiedPetroleumGas(t *testing.T) {
	document := makeRenderDocument(1, 1)
	result, err := RenderPetaniBundle(BundleRenderInput{
		LocalDate: "2024-12-10",
		Documents: []RecipientDocument{document},
		LogoBytes: map[string][]byte{"logo": testPNG(t)},
	})
	if err != nil {
		t.Fatal(err)
	}

	content := decodedPDFStreams(t, result.PDF)
	for _, phrase := range []string{"LIQUEFIED", "PETROLEUM GAS"} {
		pattern := regexp.MustCompile(`(?s)/F\S+ 9\.50 Tf.{0,120}\(` + regexp.QuoteMeta(phrase) + `\)\s*Tj`)
		if !pattern.MatchString(content) {
			index := strings.Index(content, phrase)
			start, end := maxInt(0, index-180), index+180
			if end > len(content) {
				end = len(content)
			}
			t.Fatalf("rendered PDF does not draw %q as a separately styled 9.5pt segment; nearby=%q", phrase, content[start:end])
		}
	}
}

func TestRenderPetaniBundleUsesApprovedFormContent(t *testing.T) {
	document := makeRenderDocument(1, 1182)
	result, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{document}, LogoBytes: map[string][]byte{"logo": testPNG(t)}})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, result.PDF)
	for _, want := range []string{
		"PENGADAAN BARANG PENYEDIAAN DAN PENDISTRIBUSIAN",
		"Hari / Tanggal",
		"SELASA, 10 DESEMBER 2024",
		"A.  Data Penerima",
		"B.  Data Paket Perdana yang akan diterima",
		"Seluruh Material/Produk/Barang tercantum diatas",
		"PELAKSANA PEMASANGAN",
		"DAN PENDISTRIBUSIAN",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered PDF is missing %q", want)
		}
	}
}

func TestRenderPetaniBundleUppercasesLegacyBusinessValues(t *testing.T) {
	document := makeRenderDocument(1, 1)
	document.Snapshot.Recipient.FullName = "Nama Lama"
	document.Snapshot.Recipient.Address = "Jalan Melati"
	document.Snapshot.Equipment.MachineType = "Pompa Tani"
	document.Snapshot.Components[0].Label = "Tabung Lama"
	document.Snapshot.Components[0].Unit = "Buah"
	document.Snapshot.Signatures.SupervisorName = "Andi Saputra"

	result, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{document}, LogoBytes: map[string][]byte{"logo": testPNG(t)}})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, result.PDF)
	for _, want := range []string{"NAMA LAMA", "JALAN MELATI", "POMPA TANI", "TABUNG LAMA", "BUAH", "ANDI SAPUTRA"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered PDF is missing uppercase legacy value %q", want)
		}
	}
	for _, unwanted := range []string{"Nama Lama", "Jalan Melati", "Pompa Tani", "Tabung Lama", "Andi Saputra"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("rendered PDF still contains mixed-case legacy value %q", unwanted)
		}
	}
}

func TestRenderPetaniBundleSortsRecipientsAndStartsEachOnNewPage(t *testing.T) {
	documents := []RecipientDocument{
		makeRenderDocument(10, 10),
		makeRenderDocument(2, 10),
		makeRenderDocument(1, 10),
	}
	result, err := RenderPetaniBundle(BundleRenderInput{
		LocalDate: "2024-12-10",
		Documents: documents,
		LogoBytes: map[string][]byte{"logo": testPNG(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(result.PDF, []byte("%PDF-")) {
		t.Fatal("result is not a PDF")
	}
	if result.PageCount != 3 {
		t.Fatalf("page_count=%d events=%v", result.PageCount, result.Events)
	}
	want := []string{
		"PAGE 1 RECIPIENT 1 START",
		"PAGE 2 RECIPIENT 2 START",
		"PAGE 3 RECIPIENT 10 START",
	}
	if strings.Join(result.Events, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s", strings.Join(result.Events, "\n"))
	}
}

func TestRenderPetaniBundleContinuesLongRecipientBeforeNextRecipient(t *testing.T) {
	long := makeRenderDocument(1, 2)
	long.Snapshot.Components = make([]ComponentSnapshot, 58)
	for i := range long.Snapshot.Components {
		long.Snapshot.Components[i] = ComponentSnapshot{
			Code:     "component",
			Label:    "Komponen tambahan dengan nama panjang untuk menguji pembungkusan baris tabel",
			Quantity: 1,
			Unit:     "Pcs",
			Checked:  true,
		}
	}
	result, err := RenderPetaniBundle(BundleRenderInput{
		LocalDate: "2024-12-10",
		Documents: []RecipientDocument{makeRenderDocument(2, 2), long},
		LogoBytes: map[string][]byte{"logo": testPNG(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount < 3 {
		t.Fatalf("expected continuation page, page_count=%d", result.PageCount)
	}
	events := strings.Join(result.Events, "\n")
	if !strings.Contains(events, "RECIPIENT 1 CONTINUATION") {
		t.Fatalf("missing continuation event:\n%s", events)
	}
	last := result.Events[len(result.Events)-1]
	if !strings.Contains(last, "RECIPIENT 2 START") {
		t.Fatalf("next recipient did not start last: %q", last)
	}
}

func TestRenderPetaniBundleRejectsMismatchedDateAndMissingLogo(t *testing.T) {
	doc := makeRenderDocument(1, 1)
	doc.LocalDate = "2024-12-11"
	doc.Snapshot.LocalDate = "2024-12-11"
	if _, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{doc}, LogoBytes: map[string][]byte{"logo": testPNG(t)}}); err == nil {
		t.Fatal("expected date mismatch error")
	}
	doc.LocalDate = "2024-12-10"
	doc.Snapshot.LocalDate = "2024-12-10"
	if _, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{doc}}); err == nil {
		t.Fatal("expected missing logo error")
	}
}

func TestRenderPetaniBundleFiftyRecipientsProducesOrderedFiftyPages(t *testing.T) {
	documents := make([]RecipientDocument, 0, 50)
	for slot := 50; slot >= 1; slot-- {
		documents = append(documents, makeRenderDocument(slot, 50))
	}
	result, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: documents, LogoBytes: map[string][]byte{"logo": testPNG(t)}})
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount != 50 || len(result.Recipients) != 50 {
		t.Fatalf("pages=%d recipients=%d", result.PageCount, len(result.Recipients))
	}
	if result.Recipients[0].SlotNumber != 1 || result.Recipients[0].PageStart != 1 || result.Recipients[49].SlotNumber != 50 || result.Recipients[49].PageStart != 50 {
		t.Fatalf("page ranges=%+v ... %+v", result.Recipients[0], result.Recipients[49])
	}
}

func makeRenderDocument(slot, total int) RecipientDocument {
	return RecipientDocument{
		DistributionSlotID: "slot",
		SlotNumber:         slot,
		FinalTotal:         total,
		DocumentNumber:     fmt.Sprintf("%04d/%d/KSM-KKT-WJO/XII/2024", slot, total),
		LocalDate:          "2024-12-10",
		Snapshot: Snapshot{
			ProgramType:    "farmer",
			DocumentNumber: "BAST-WAJO",
			LocalDate:      "2024-12-10",
			Render: RenderIdentity{
				FiscalYear: 2024,
				Logos:      []LogoSnapshot{{AssetID: "logo", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 15}},
			},
			Recipient:  RecipientSnapshot{FullName: "Penerima", NIK: "7306014101900001", SectorIdentifier: "KARTU-01", Address: "Jalan Tani", Village: "Desa", District: "Kecamatan", Regency: "Wajo", PhoneNumber: "08123456789"},
			Equipment:  EquipmentSnapshot{MachineBrand: "SHARK", MachineType: "SPWP 80-30 / 3 inch", MachineSerial: "M-001", HoseBrand: "TRILIUNHOSE", HoseSpec: "Panjang Selang Hisap: 6m; Panjang Selang Buang: 10m", HoseSerial: "-", ConverterBrand: "ERGAS", ConverterSerial: "240005562"},
			Components: []ComponentSnapshot{{Code: "lpg", Label: "Tabung LPG 3 Kg", Quantity: 1, Unit: "Tabung", Checked: true}, {Code: "regulator", Label: "Regulator", Quantity: 1, Unit: "Pcs", Checked: true}},
			Signatures: SignatureSnapshot{ReceiverName: "Penerima", ExecutorName: "Pelaksana", SupervisorName: "Pengawas"},
		},
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 120, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 25, G: 115, B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodedPDFStreams(t *testing.T, data []byte) string {
	t.Helper()
	streamPattern := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	var decoded strings.Builder
	for _, match := range streamPattern.FindAllSubmatch(data, -1) {
		reader, err := zlib.NewReader(bytes.NewReader(match[1]))
		if err != nil {
			continue
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("decode PDF stream: read=%v close=%v", readErr, closeErr)
		}
		decoded.Write(content)
	}
	return decoded.String()
}
