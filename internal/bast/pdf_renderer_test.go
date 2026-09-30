package bast

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
			Profile: ProfileSnapshot{
				Title:                  "BERITA ACARA SERAH TERIMA",
				Subtitle:               "(FORM PENERIMA PAKET)",
				ProcurementDescription: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana LPG untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran 2024",
				Logos:                  []LogoSnapshot{{AssetID: "logo", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 15}},
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
