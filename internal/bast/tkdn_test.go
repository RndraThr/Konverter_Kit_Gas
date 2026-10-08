package bast

import (
	"errors"
	"testing"
)

func sampleTKDNSnapshot() TKDNSnapshot {
	profile := defaultTKDNProfile("program-1")
	items := tkdnItemsFor(defaultTKDNRows(), resolveEntries(mustEntries(sampleTemplateValues), nil, "SHARK", nil))
	return TKDNSnapshot{
		DocumentType: AggregateDocumentTKDN, DocumentDate: "2026-11-02",
		DocumentNumber: formatTKDNDocumentNumber("wng", "2026-11-02", 12, 1),
		ProviderName:   "PT KIAN SANTANG MULIATAMA TBK.", RegencyName: "KAB. WONOGIRI", RegencyCode: "WNG", ProvinceName: "JAWA TENGAH",
		FiscalYear: 2026, TotalPackages: 12, Items: items, TotalTKDN: profile.TotalTKDN,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
		Signatories: closingSignatories{AgricultureOfficeName: "IR. HARYANTO, M.SI", AgricultureOfficeNIP: "197203151998031004"},
	}
}

func TestFormatTKDNDocumentNumberFollowsReference(t *testing.T) {
	if got := formatTKDNDocumentNumber("wng", "2026-11-02", 12, 1); got != "01/12/WNG/KKT/RTKDN/XI/2026" {
		t.Fatalf("document number=%q", got)
	}
}

func TestTKDNNumberFormattingUsesIndonesianSeparators(t *testing.T) {
	cases := map[string]string{
		formatTKDNPercent(61.42):      "61,42%",
		formatTKDNPercent(0):          "0,00%",
		formatTKDNQuantity(2, 12):     "24",
		formatTKDNQuantity(1, 1578):   "1.578",
		formatTKDNQuantity(0.5, 3):    "1,50",
		formatIndonesianNumber(68, 2): "68,00",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestGroupTKDNItemsMergesConsecutiveGroups(t *testing.T) {
	groups := groupTKDNItems(tkdnItemsFor(defaultTKDNRows(), resolveEntries(mustEntries(sampleTemplateValues), nil, "", nil)))
	// Referensi: 8 nomor; nomor 2 (konverter) 3 sub-barang, nomor 4 (tabung) 2 sub-barang.
	if len(groups) != 8 || len(groups[1].items) != 3 || groups[1].rowCount() != 4 || len(groups[3].items) != 2 || groups[0].rowCount() != 1 {
		t.Fatalf("groups=%+v", groups)
	}
}

func TestTKDNProfileNormalizeValidatesTotal(t *testing.T) {
	profile := TKDNProfile{ProgramID: " p ", TotalTKDN: 68.534, Rows: defaultTKDNRows()}
	if err := profile.normalize(); err != nil {
		t.Fatal(err)
	}
	if profile.ProgramID != "p" || profile.TotalTKDN != 68.53 {
		t.Fatalf("profile=%+v", profile)
	}
	for _, bad := range []TKDNProfile{{ProgramID: "p", TotalTKDN: 101, Rows: defaultTKDNRows()}, {ProgramID: "p", TotalTKDN: 50}, {TotalTKDN: 50, Rows: defaultTKDNRows()}} {
		if err := bad.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("profile=%+v err=%v", bad, err)
		}
	}
}

func TestRenderTKDNPutsSignaturesOnSecondPage(t *testing.T) {
	rendered, err := RenderTKDN(sampleTKDNSnapshot(), map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount != 2 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("page_count=%d", rendered.PageCount)
	}
}
