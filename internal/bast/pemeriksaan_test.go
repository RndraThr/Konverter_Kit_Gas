package bast

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func samplePemeriksaanSnapshots() []PemeriksaanSnapshot {
	profile := defaultPemeriksaanProfile("program-1")
	ctx := DP3Context{ScheduleID: "s", ProgramID: "program-1", RegencyName: "Kab. Wonogiri", ProvinceName: "Jawa Tengah", FiscalYear: 2026}
	settings := ScheduleSettings{ConsultantCompanyName: "PT KIAN SANTANG MULIATAMA TBK.", AgricultureOfficeName: "ir. haryanto", AgricultureOfficeNIP: "197203151998031004"}
	logos := []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}}
	entries := mustEntries(sampleTemplateValues)
	resolved := resolveEntries(entries, nil, "SHARK", nil)
	snapshots := make([]PemeriksaanSnapshot, 0, len(profile.Forms))
	for _, form := range profile.Forms {
		form.Rows, _ = pemeriksaanRowsFor(form, entries, resolved)
		snapshots = append(snapshots, buildPemeriksaanSnapshot(ctx, settings, form, "4500123456", logos, "2026-11-03", 12))
	}
	return snapshots
}

func TestDefaultPemeriksaanProfileFollowsSevenReferenceForms(t *testing.T) {
	profile := defaultPemeriksaanProfile("p")
	if err := profile.normalize(); err != nil {
		t.Fatal(err)
	}
	codes := []string{}
	for _, form := range profile.Forms {
		codes = append(codes, form.Code)
	}
	if strings.Join(codes, ",") != "mesin,konverter_kit,regulator,selang_gas,selang_hisap_buang,oli,tabung_gas" {
		t.Fatalf("codes=%v", codes)
	}
	// Setiap form bawaan punya barang di Master Barang bawaan.
	entries := mustEntries(sampleTemplateValues)
	resolved := resolveEntries(entries, nil, "", nil)
	for _, form := range profile.Forms {
		if rows, _ := pemeriksaanRowsFor(form, entries, resolved); len(rows) == 0 {
			t.Fatalf("form %s has no template items", form.Code)
		}
	}
	if rows, _ := pemeriksaanRowsFor(profile.Forms[4], entries, resolved); len(rows) != 2 {
		t.Fatalf("selang hisap & buang rows=%+v", rows)
	}
	if rows, _ := pemeriksaanRowsFor(profile.Forms[5], entries, resolved); rows[0].QuantityPerPackage != 2 {
		t.Fatalf("oli rows=%+v", rows)
	}
	checklist := profile.Forms[0].Checklist
	if checklist.Packaging != "baik" || checklist.Conclusion != "diterima" || len(checklist.Documents) != 2 {
		t.Fatalf("default checklist=%+v", checklist)
	}
}

// Regresi: form tanpa dokumen pendukung default dulu terkirim sebagai
// "documents": null dan membuat editor frontend crash (halaman putih).
func TestPemeriksaanChecklistDocumentsAlwaysEncodeAsArray(t *testing.T) {
	for _, form := range defaultPemeriksaanProfile("p").Forms {
		payload, err := json.Marshal(form.Checklist)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), `"documents":null`) {
			t.Fatalf("form %s checklist=%s", form.Code, payload)
		}
	}
	forms := []PemeriksaanForm{{Code: "oli"}}
	ensurePemeriksaanLists(forms)
	if forms[0].Checklist.Documents == nil || forms[0].Rows == nil {
		t.Fatalf("lists not initialised: %+v", forms[0])
	}
}

func TestPemeriksaanProfileNormalizeRejectsInvalidForms(t *testing.T) {
	valid := func() PemeriksaanForm {
		return PemeriksaanForm{Code: "oli", Title: "Oli", Rows: []PemeriksaanRow{{Description: "OLI", Unit: "LITER", QuantityPerPackage: 2}}, Checklist: defaultPemeriksaanChecklist()}
	}
	duplicate := PemeriksaanProfile{ProgramID: "p", Forms: []PemeriksaanForm{valid(), valid()}}
	badCode := valid()
	badCode.Code = "Oli Pertamina"
	badChoice := valid()
	badChoice.Checklist.Packaging = "lumayan"
	for _, profile := range []PemeriksaanProfile{
		duplicate,
		{ProgramID: "p", Forms: []PemeriksaanForm{badCode}},
		{ProgramID: "p", Forms: []PemeriksaanForm{badChoice}},
		{ProgramID: "p"},
	} {
		if err := profile.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("profile=%+v err=%v", profile, err)
		}
	}
}

func TestPemeriksaanNarrativeAndFilename(t *testing.T) {
	if got := pemeriksaanNarrative("2026-11-03", 2026); !strings.HasPrefix(got, "Pada Hari ini, Selasa Tanggal 3 November Tahun 2026, Telah dilakukan Pemeriksaan Fisik Barang") || !strings.HasSuffix(got, "Program Konversi Tahun 2026, dengan rincian sebagai berikut:") {
		t.Fatalf("narrative=%q", got)
	}
	if got := formatPemeriksaanFilename("Selang Hisap & Buang", "Kab. Wonogiri", "2026-11-03", 2); got != "BA PEMERIKSAAN SELANG HISAP & BUANG - KAB. WONOGIRI - 2026-11-03 - V2.pdf" {
		t.Fatalf("filename=%q", got)
	}
	if pemeriksaanDocumentType("oli") != "pemeriksaan:oli" {
		t.Fatal("document type prefix changed")
	}
}

func TestRenderPemeriksaanOneFormIsTwoPagesAndPreviewCombinesAll(t *testing.T) {
	snapshots := samplePemeriksaanSnapshots()
	logo := map[string][]byte{"logo-1": testPNG(t)}
	single, err := RenderPemeriksaan(snapshots[:1], logo)
	if err != nil {
		t.Fatal(err)
	}
	if single.PageCount != 2 {
		t.Fatalf("single form page_count=%d, want 2", single.PageCount)
	}
	combined, err := RenderPemeriksaan(snapshots, logo)
	if err != nil {
		t.Fatal(err)
	}
	if combined.PageCount != 2*len(snapshots) {
		t.Fatalf("combined page_count=%d, want %d", combined.PageCount, 2*len(snapshots))
	}
}
