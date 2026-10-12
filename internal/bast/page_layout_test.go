package bast

import (
	"strings"
	"testing"
)

// Kop dan keterangan dokumen (Hari / Tanggal, Lokasi, ...) hanya dicetak di
// halaman pertama; halaman lanjutan dan halaman tanda tangan hanya memuat logo.
func TestAggregateDocumentsPrintHeaderInfoOnlyOnFirstPage(t *testing.T) {
	logo := map[string][]byte{"logo-1": testPNG(t)}
	logos := []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}}

	dp3 := DP3Snapshot{DocumentType: AggregateDocumentDP3, DocumentDate: "2026-09-24", RegencyName: "KAB. WONOGIRI", HandoverLocation: "BALAI DESA", ConsultantCompanyName: "PT KONSULTAN", FiscalYear: 2026, Logos: logos}
	for i := 0; i < 30; i++ {
		dp3.Recipients = append(dp3.Recipients, dp3RecipientSnapshot{FullName: "SITI", NIK: "1", Address: "JL. SAWAH", MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "5.5 HP", MachineFuelType: "BENSIN"})
	}
	recap := DailyRecapSnapshot{DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-09-24", HandoverLocation: "BALAI DESA", ConsultantCompanyName: "PT KONSULTAN", FiscalYear: 2026, Logos: logos, GrandTotal: 60,
		Variants: []DailyRecapVariant{{Label: "Varian 1", MachineBrand: "SHARK", MachineType: "SPWP", Count: 60}}}
	for i := 0; i < 60; i++ {
		recap.Recipients = append(recap.Recipients, dailyRecapRecipientSnapshot{SlotNumber: i + 1, FullName: "SITI", MachineBrand: "SHARK", MachineType: "SPWP", MachineSerial: "SN"})
	}

	cases := []struct {
		name   string
		render func() (RenderedAggregate, error)
	}{
		{"dp3", func() (RenderedAggregate, error) { return RenderDP3(dp3, logo) }},
		{"daily recap", func() (RenderedAggregate, error) { return RenderDailyRecap(recap, logo) }},
	}
	for _, tc := range cases {
		rendered, err := tc.render()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if rendered.PageCount < 3 {
			t.Fatalf("%s: page_count=%d, want a continuation page and a signature page", tc.name, rendered.PageCount)
		}
		content := decodedPDFStreams(t, rendered.PDF)
		// decodedPDFStreams memuat dua salinan teks yang dapat dicari (byte nol
		// dibuang dan TJ digabung), jadi satu halaman menghasilkan dua kemunculan.
		if got := strings.Count(content, "Hari / Tanggal"); got != 2 {
			t.Fatalf("%s: \"Hari / Tanggal\" found %d times, want it on the first page only", tc.name, got)
		}
	}
}

// Tanggal PDF diambil dari tanggal dokumen, bukan jam render, agar checksum
// isi yang sama selalu sama dan finalisasi ulang dianggap tidak berubah.
func TestRenderedPDFDatesFollowDocumentDate(t *testing.T) {
	logo := map[string][]byte{"logo-1": testPNG(t), "logo": testPNG(t)}
	bundle, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{makeRenderDocument(1, 1)}, LogoBytes: logo})
	if err != nil {
		t.Fatal(err)
	}
	closing, err := RenderClosingTitikSerah(sampleClosingSnapshot(), logo)
	if err != nil {
		t.Fatal(err)
	}
	for name, pdf := range map[string][]byte{"bundle": bundle.PDF, "closing": closing.PDF} {
		text := string(pdf)
		if strings.Count(text, "/CreationDate (D:2") != 1 || !strings.Contains(text, "/ModDate (D:20") {
			t.Fatalf("%s: missing PDF dates", name)
		}
	}
	if !strings.Contains(string(bundle.PDF), "/ModDate (D:20241210000000)") {
		t.Fatal("bundle ModDate must equal the document date")
	}
	if !strings.Contains(string(closing.PDF), "/ModDate (D:"+"20261003000000)") {
		t.Fatal("closing ModDate must equal the document date")
	}
}
