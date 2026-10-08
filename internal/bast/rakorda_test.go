package bast

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
)

func TestBuildRakordaSnapshotValidatesInput(t *testing.T) {
	ctx := DP3Context{ScheduleID: "schedule-1", ProgramID: "program-1", ProgramType: "farmer", RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 4", FiscalYear: 2026, HasActiveLogo: true}
	settings := ScheduleSettings{RakordaLocation: "Aula Kantor Bupati", RakordaRowCount: 45}

	snapshot, err := buildRakordaSnapshot(rakordaSpec, ctx, settings, nil, "2026-10-03", nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Location != "AULA KANTOR BUPATI" || snapshot.RowCount != 45 || snapshot.RegencyName != "WAJO" || snapshot.ZoneName != "ZONA 4" {
		t.Fatalf("snapshot=%+v", snapshot)
	}

	settings.RakordaRowCount = 4
	if _, err := buildRakordaSnapshot(rakordaSpec, ctx, settings, nil, "2026-10-03", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}

	settings.RakordaRowCount = 45
	placeholderCtx := ctx
	placeholderCtx.ZonePlaceholder = true
	if _, err := buildRakordaSnapshot(rakordaSpec, placeholderCtx, settings, nil, "2026-10-03", nil); !errors.Is(err, ErrZoneNotConfigured) {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildSosialisasiSnapshotUsesItsOwnSettingsAndSignatories(t *testing.T) {
	ctx := DP3Context{ScheduleID: "schedule-1", ProgramID: "program-1", ProgramType: "farmer", RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 4", FiscalYear: 2026}
	settings := ScheduleSettings{
		RakordaLocation: "Aula Kantor Bupati", RakordaRowCount: 45,
		SosialisasiLocation: "balai desa tempe", SosialisasiRowCount: 51,
		AgricultureOfficeName: "ir. haryanto", AgricultureOfficeNIP: " 197203151998031004 ", PertaminaRepName: "rina",
	}
	snapshot, err := buildRakordaSnapshot(sosialisasiSpec, ctx, settings, nil, "2026-10-03", nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Location != "BALAI DESA TEMPE" || snapshot.RowCount != 51 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.Signatories.AgricultureOfficeName != "IR. HARYANTO" || snapshot.Signatories.AgricultureOfficeNIP != "197203151998031004" || snapshot.Signatories.PertaminaRepName != "RINA" {
		t.Fatalf("signatories=%+v", snapshot.Signatories)
	}

	settings.SosialisasiLocation = ""
	if _, err := buildRakordaSnapshot(sosialisasiSpec, ctx, settings, nil, "2026-10-03", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing sosialisasi location err=%v", err)
	}
}

func TestRenderRakordaProducesNumberedMultiPageAttendanceSheet(t *testing.T) {
	snapshot := RakordaSnapshot{
		DocumentDate: "2026-10-03", Location: "AULA KANTOR BUPATI",
		RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026,
		RowCount: 45, Logos: []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
	}
	rendered, err := RenderRakorda(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount != 2 {
		t.Fatalf("page_count=%d, want 2", rendered.PageCount)
	}
	if len(rendered.PDF) < 1000 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("invalid PDF, len=%d", len(rendered.PDF))
	}
}

func TestRenderSosialisasiAddsContinuationAndSignaturePages(t *testing.T) {
	snapshot := RakordaSnapshot{
		DocumentDate: "2026-10-03", Location: "BALAI DESA TEMPE",
		RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026,
		RowCount: 51, Logos: []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
		Signatories: closingSignatories{AgricultureOfficeName: "IR. HARYANTO", AgricultureOfficeNIP: "197203151998031004"},
	}
	rendered, err := RenderSosialisasi(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	// 51 baris: halaman pertama memuat kop + awal tabel, sisa baris di halaman
	// lanjutan, lalu satu halaman tanda tangan, sama seperti dokumen referensi.
	if rendered.PageCount != 3 {
		t.Fatalf("page_count=%d, want 3", rendered.PageCount)
	}
	if string(rendered.PDF[:4]) != "%PDF" {
		t.Fatal("invalid PDF")
	}

	snapshot.RowCount = 4
	if _, err := RenderSosialisasi(snapshot, map[string][]byte{"logo-1": testPNG(t)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestRenderTraining10PutsSignaturesOnTheirOwnLastPage(t *testing.T) {
	logos := []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}}
	pages := func(rows int) int {
		t.Helper()
		rendered, err := RenderTraining10(RakordaSnapshot{DocumentDate: "2026-10-03", Location: "BALAI DESA TEMPE", RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026, RowCount: rows, Logos: logos}, map[string][]byte{"logo-1": testPNG(t)})
		if err != nil {
			t.Fatal(err)
		}
		return rendered.PageCount
	}
	// Tanda tangan selalu di halaman tersendiri: 10 baris = 1 halaman tabel + 1;
	// 51 baris (20 di halaman 1, 31 di halaman 2, seperti referensi) = 2 + 1.
	if got := pages(10); got != 2 {
		t.Fatalf("10 rows page_count=%d, want 2", got)
	}
	if got := pages(51); got != 3 {
		t.Fatalf("51 rows page_count=%d, want 3", got)
	}
}

func TestActivitySignatureTableMatchesMainTableWidthAndKeepsNIPAt95pt(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	registerArial(pdf)
	layout := layoutActivitySignatures(pdf, closingSignatories{AgricultureOfficeName: "IR. HARYANTO, M.SI", AgricultureOfficeNIP: "197203151998031004"}, activitySignatureHeaders)
	total := 0.0
	for _, width := range layout.widths {
		if width != layout.widths[0] {
			t.Fatalf("columns differ: %v", layout.widths)
		}
		total += width
	}
	mainTable := 0.0
	for _, width := range activityColumnWidths {
		mainTable += width
	}
	if total != mainTable {
		t.Fatalf("signature table=%.1fmm, main table=%.1fmm", total, mainTable)
	}
	if layout.nipPt != activitySignatureNamePt {
		t.Fatalf("18-digit NIP shrank to %.1fpt, want %.1fpt", layout.nipPt, activitySignatureNamePt)
	}
}

func TestActivityNarrativeFollowsReference(t *testing.T) {
	cases := []struct {
		document activityDocument
		opening  string
	}{
		{sosialisasiDocument, "Pada Hari ini, Sabtu, Tanggal 3 Oktober Tahun 2026, Telah dilakukan Sosialisasi kepada 10% penerima "},
		{training10Document, "Pada Hari ini, Sabtu, Tanggal 3 Oktober Tahun 2026, Telah dilakukan Training kepada 10% penerima "},
		{training100Document, "Pada Hari ini, Sabtu, Tanggal 3 Oktober Tahun 2026, Telah dilakukan Training kepada penerima "},
	}
	for _, tc := range cases {
		segments := activityNarrative(tc.document, "2026-10-03", 2026, "ZONA 4")
		if len(segments) != 4 || segments[0].text != tc.opening || segments[0].style != "" {
			t.Fatalf("opening=%+v", segments)
		}
		// Uraian pengadaan sampai nama zona tebal; "Liquefied Petroleum Gas" tebal-miring.
		if segments[1].style != "B" || segments[2].text != "Liquefied Petroleum Gas" || segments[2].style != "BI" || segments[3].style != "B" {
			t.Fatalf("styles=%+v", segments)
		}
		if !strings.HasSuffix(segments[3].text, "(Termasuk Pendistribusian Dan Pemasangan) Zona 4") {
			t.Fatalf("closing=%q", segments[3].text)
		}
	}
	if training100Document.title != "BERITA ACARA TELAH DILAKSANAKANNYA TRAINING" {
		t.Fatalf("title=%q", training100Document.title)
	}
}

func TestRenderTraining100ProducesAttendanceSheet(t *testing.T) {
	rendered, err := RenderTraining100(RakordaSnapshot{DocumentDate: "2026-10-03", Location: "BALAI DESA TEMPE", RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026, RowCount: 51, Logos: []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1}}}, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount != 3 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("page_count=%d", rendered.PageCount)
	}
}

func TestFormatRakordaUploadFilenameUsesDocumentIdentity(t *testing.T) {
	if got := formatRakordaUploadFilename(rakordaSpec.label, "Kabupaten Wajo", "2026-10-20", "image/png"); got != "DAFTAR HADIR RAKORDA - KABUPATEN WAJO - 20 OKTOBER 2026.png" {
		t.Fatalf("filename=%q", got)
	}
	if got := formatRakordaUploadFilename(sosialisasiSpec.label, "Kabupaten Wajo", "2026-10-20", "application/pdf"); got != "BA SOSIALISASI - KABUPATEN WAJO - 20 OKTOBER 2026.pdf" {
		t.Fatalf("filename=%q", got)
	}
	if got := formatRakordaUploadFilename(training10Spec.label, "Kabupaten Wajo", "2026-10-20", "image/jpeg"); got != "BA TRAINING 10% - KABUPATEN WAJO - 20 OKTOBER 2026.jpg" {
		t.Fatalf("filename=%q", got)
	}
	if got := formatRakordaUploadFilename(training100Spec.label, "Kabupaten Wajo", "2026-10-20", "image/jpeg"); got != "BA TRAINING 100% - KABUPATEN WAJO - 20 OKTOBER 2026.jpg" {
		t.Fatalf("filename=%q", got)
	}
}

func TestTrainingParticipantsFollowDistributionAndTakeFirstTenPercent(t *testing.T) {
	if tenPercentCount(100) != 10 || tenPercentCount(15) != 2 || tenPercentCount(3) != 1 || tenPercentCount(0) != 0 {
		t.Fatal("ten percent rounding changed")
	}
	all := make([]ActivityParticipant, 23)
	for i := range all {
		all[i] = ActivityParticipant{Name: fmt.Sprintf("Peserta %d", i+1), Occupation: "Petani", SignatureKey: fmt.Sprintf("slot-%d", i+1)}
	}
	ctx := DP3Context{ScheduleID: "s", ProgramID: "p", RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 4", FiscalYear: 2026}
	settings := ScheduleSettings{Training10Location: "Balai Desa", Training100Location: "Balai Desa", Training10RowCount: 51, Training100RowCount: 51}
	ten, err := buildRakordaSnapshot(training10Spec, ctx, settings, nil, "2026-10-03", all)
	if err != nil {
		t.Fatal(err)
	}
	if ten.RowCount != 3 || ten.Participants[0].Name != "Peserta 1" || ten.Participants[2].Name != "Peserta 3" {
		t.Fatalf("10%% snapshot=%+v", ten)
	}
	full, err := buildRakordaSnapshot(training100Spec, ctx, settings, nil, "2026-10-03", all)
	if err != nil || full.RowCount != 23 {
		t.Fatalf("100%% rows=%d err=%v", full.RowCount, err)
	}
	if _, err := buildRakordaSnapshot(training100Spec, ctx, settings, nil, "2026-10-03", nil); !errors.Is(err, ErrAggregateNoRecipients) {
		t.Fatalf("no participants err=%v", err)
	}
	// Sosialisasi tetap lembar kosong walau ada data penerima.
	if blank, _ := buildRakordaSnapshot(sosialisasiSpec, ctx, ScheduleSettings{SosialisasiLocation: "Balai", SosialisasiRowCount: 51}, nil, "2026-10-03", all); blank.Participants != nil || blank.RowCount != 51 {
		t.Fatalf("sosialisasi=%+v", blank)
	}

	full.Logos = []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1}}
	full.Participants[0].Address = strings.Repeat("Jalan Panjang Sekali ", 8)
	rendered, err := RenderTraining100(full, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	// Halaman pertama memuat 20 baris (seperti referensi), sisa 3 baris di
	// halaman kedua, lalu tanda tangan di halaman tersendiri.
	if rendered.PageCount != 3 {
		t.Fatalf("page_count=%d", rendered.PageCount)
	}
}
