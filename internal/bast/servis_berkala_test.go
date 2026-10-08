package bast

import (
	"errors"
	"strings"
	"testing"
)

func sampleServisBerkalaSnapshot() ServisBerkalaSnapshot {
	return ServisBerkalaSnapshot{
		DocumentType: AggregateDocumentServisBerkala, DocumentDate: "2026-10-06",
		DocumentNumber: formatServisBerkalaDocumentNumber("wng", "2026-10-06", 12, 1),
		RegencyName:    "KAB. WONOGIRI", RegencyCode: "WNG", ProvinceName: "JAWA TENGAH", ZoneName: "ZONA 1", FiscalYear: 2026,
		TotalPackages: 12,
		Services: []ServisPeriod{
			{Start: "2027-05-03", End: "2027-05-08"},
			{Start: "2027-11-01", End: "2027-11-06"},
		},
		ConsultantCompanyName: "PT KIAN SANTANG MULIATAMA TBK.",
		Logos:                 []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
		Signatories:           closingSignatories{AgricultureOfficeName: "IR. HARYANTO, M.SI", AgricultureOfficeNIP: "197203151998031004", InstallerName: "DWI SANTOSO"},
	}
}

func TestFormatServisBerkalaDocumentNumberFollowsReference(t *testing.T) {
	if got := formatServisBerkalaDocumentNumber("wng", "2026-10-06", 12, 1); got != "01/12/WNG/KKT/SB/X/2026" {
		t.Fatalf("document number=%q", got)
	}
	if got := formatServisBerkalaDocumentNumber("bgk", "2026-05-02", 320, 3); got != "003/320/BGK/KKT/SB/V/2026" {
		t.Fatalf("document number=%q", got)
	}
}

func TestFormatServisRangeUsesIndonesianTitleCaseDates(t *testing.T) {
	if got := formatServisRange(ServisPeriod{Start: "2027-05-03", End: "2027-05-08"}); got != "3 Mei 2027 s/d 8 Mei 2027" {
		t.Fatalf("range=%q", got)
	}
}

func TestValidateServisPeriodsRequiresBothOrderedRanges(t *testing.T) {
	valid := []ServisPeriod{{Start: "2027-05-03", End: "2027-05-08"}, {Start: "2027-11-01", End: "2027-11-06"}}
	if err := validateServisPeriods(valid); err != nil {
		t.Fatalf("valid err=%v", err)
	}
	for _, periods := range [][]ServisPeriod{
		{{Start: "2027-05-03", End: "2027-05-08"}, {Start: "", End: "2027-11-06"}},
		{{Start: "2027-05-09", End: "2027-05-08"}, {Start: "2027-11-01", End: "2027-11-06"}},
	} {
		if err := validateServisPeriods(periods); !errors.Is(err, ErrServisScheduleRequired) {
			t.Fatalf("periods=%+v err=%v", periods, err)
		}
	}
}

func TestServisBerkalaNarrativeBoldsProcurementThroughZone(t *testing.T) {
	segments := servisBerkalaNarrative(2026, "ZONA 1")
	if segments[0].text != "Sehubungan dengan " || segments[0].style != "" || segments[2].text != "Liquefied Petroleum Gas" || segments[2].style != "BI" {
		t.Fatalf("segments=%+v", segments)
	}
	if segments[3].style != "B" || !strings.HasSuffix(segments[3].text, "(Termasuk Pendistribusian Dan Pemasangan) Zona 1.") {
		t.Fatalf("bold closing=%q", segments[3].text)
	}
	if segments[4].style != "" || !strings.HasPrefix(segments[4].text, " Pelaksana Pekerjaan memberikan Servis Berkala sebanyak 2 (dua) kali") {
		t.Fatalf("plain tail=%q", segments[4].text)
	}
}

func TestRenderServisBerkalaProducesSinglePage(t *testing.T) {
	rendered, err := RenderServisBerkala(sampleServisBerkalaSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount != 1 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("page_count=%d", rendered.PageCount)
	}

	snapshot := sampleServisBerkalaSnapshot()
	snapshot.Services[1].Start = ""
	if _, err := RenderServisBerkala(snapshot, map[string][]byte{"logo-1": testPNG(t)}); !errors.Is(err, ErrServisScheduleRequired) {
		t.Fatalf("incomplete schedule err=%v", err)
	}
}

func TestScheduleSettingsInputValidatesServisDates(t *testing.T) {
	input := ScheduleSettingsInput{ScheduleID: "sched-1", Servis1Start: " 2027-05-03 ", Servis1End: "2027-05-08"}
	if err := input.normalize(); err != nil || input.Servis1Start != "2027-05-03" {
		t.Fatalf("input=%+v err=%v", input, err)
	}
	for _, bad := range []ScheduleSettingsInput{
		{ScheduleID: "sched-1", Servis1Start: "2027-05-09", Servis1End: "2027-05-08"},
		{ScheduleID: "sched-1", Servis2Start: "03-05-2027"},
	} {
		if err := bad.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("input=%+v err=%v", bad, err)
		}
	}
}
