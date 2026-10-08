package bast

import (
	"errors"
	"testing"
)

// sampleTemplateValues mengikuti template KONKIT-2026 setelah migrasi 00049.
const sampleTemplateValues = `{
	"machine_options":[{"code":"shark-spwp8030","brand":"SHARK","type":"SPWP 80-30/3\"","tkdn_percent":61.42}],
	"converter_options":[{"code":"ergas","brand":"ERGAS","tkdn_percent":93.27}],
	"hose_options":[{"code":"triliunhose","suction_brand":"TRILLIUNHOSE","suction_spec":"6 M","discharge_brand":"YAMAKOYO","discharge_spec":"10 M","suction_tkdn_percent":98.45,"discharge_tkdn_percent":0}],
	"components":[
		{"code":"lpg_tank","label":"Tabung LPG 3 Kg","quantity":1,"unit":"Tabung","brand":"Pertamina","tkdn_percent":58.47},
		{"code":"regulator","label":"Regulator","quantity":1,"unit":"Pcs","brand":"Top Gas","tkdn_percent":56.73},
		{"code":"hose_clamp_accessories","label":"Selang, Clamp & Aksesorisnya","quantity":1,"unit":"Set","brand":"Mondea","tkdn_percent":87.58},
		{"code":"manual","label":"Buku Manual","quantity":1,"unit":"Pcs"},
		{"code":"oil","label":"OLI","quantity":2,"unit":"Ltr","brand":"Pertamina Enduro","tkdn_percent":53.42},
		{"code":"bracket","label":"Braket","quantity":1,"unit":"Ea","brand":"N.A","tkdn_percent":0},
		{"code":"lpg_refill","label":"Isi LPG 3 Kg","quantity":1,"unit":"Tabung","brand":"-","tkdn_percent":51.99,"handover_hidden":true},
		{"code":"anti_rust_plate","label":"Plat Anti Karat","quantity":1,"unit":"Pcs","brand":"N.A","tkdn_percent":0,"handover_hidden":true}
	]
}`

const twoMachineTemplateValues = `{
	"machine_options":[{"code":"shark","brand":"SHARK","type":"SPWP 80-30","tkdn_percent":61.42},{"code":"honda","brand":"HONDA","type":"WB30XT","tkdn_percent":40}],
	"components":[{"code":"oil","label":"OLI","quantity":2,"unit":"Ltr","brand":"Pertamina Enduro","tkdn_percent":53.42}]
}`

func mustEntries(raw string) []TemplateEntry {
	entries, err := TemplateEntriesFromValues([]byte(raw))
	if err != nil {
		panic(err)
	}
	return entries
}

func TestTemplateEntriesReadBrandsQuantitiesAndTKDNFromTemplate(t *testing.T) {
	entries := mustEntries(sampleTemplateValues)
	machine, _ := entryByRef(entries, ItemKindMachine)
	if machine.Variants[0].Brand != "SHARK" || machine.Variants[0].TKDNPercent != 61.42 || machine.QuantityPerPackage != 1 {
		t.Fatalf("machine=%+v", machine)
	}
	discharge, _ := entryByRef(entries, ItemKindHoseDischarge)
	if discharge.Variants[0].Brand != "YAMAKOYO" || discharge.Variants[0].Spec != "10 M" {
		t.Fatalf("discharge=%+v", discharge)
	}
	oil, _ := entryByRef(entries, "component:oil")
	if oil.QuantityPerPackage != 2 || oil.Variants[0].Brand != "Pertamina Enduro" || oil.Variants[0].TKDNPercent != 53.42 {
		t.Fatalf("oil=%+v", oil)
	}
}

func TestDefaultTKDNRowsMatchReferenceFromTemplate(t *testing.T) {
	resolved := resolveEntries(mustEntries(sampleTemplateValues), nil, "SHARK", nil)
	items := tkdnItemsFor(defaultTKDNRows(), resolved)
	if len(items) != 11 {
		t.Fatalf("items=%d %+v", len(items), items)
	}
	want := map[string][2]any{"Pompa Air Irigasi": {"SHARK", 61.42}, "Selang LPG": {"Mondea", 87.58}, "Minyak Pelumas (Oli)": {"Pertamina Enduro", 53.42}, "Isi LPG 3 kg": {"-", 51.99}}
	for _, item := range items {
		if w, ok := want[item.Name]; ok && (item.Brand != w[0] || item.TKDNPercent != w[1]) {
			t.Fatalf("%s=%+v", item.Name, item)
		}
	}
	if items[4].QuantityPerPackage != 2 {
		t.Fatalf("oli qty=%v", items[4].QuantityPerPackage)
	}
}

func TestResolveEntriesPicksBrandManualThenMachineThenFirst(t *testing.T) {
	entries := mustEntries(twoMachineTemplateValues)
	po := map[string]string{poKey(ItemKindMachine, "honda"): "PO-HONDA", poKey(ItemKindMachine, "shark"): "PO-SHARK"}

	got := resolveEntries(entries, nil, "Honda", po)[ItemKindMachine]
	if got.Variant.Code != "honda" || got.Source != ChoiceSourceMachine || got.PONumber != "PO-HONDA" {
		t.Fatalf("machine match=%+v", got)
	}
	got = resolveEntries(entries, map[string]string{ItemKindMachine: "shark"}, "HONDA", po)[ItemKindMachine]
	if got.Variant.Code != "shark" || got.Source != ChoiceSourceManual || got.PONumber != "PO-SHARK" {
		t.Fatalf("manual=%+v", got)
	}
	got = resolveEntries(entries, nil, "", nil)[ItemKindMachine]
	if got.Variant.Code != "shark" || got.Source != ChoiceSourceDefault {
		t.Fatalf("default=%+v", got)
	}
	if oil := resolveEntries(entries, nil, "", po)["component:oil"]; oil.Source != ChoiceSourceSingle {
		t.Fatalf("single=%+v", oil)
	}
}

func TestPemeriksaanRowsComeFromTemplateAndJoinDistinctPO(t *testing.T) {
	po := map[string]string{poKey(ItemKindHoseSuction, "triliunhose"): "PO-1", poKey(ItemKindHoseDischarge, "triliunhose"): "PO-2"}
	entries := mustEntries(sampleTemplateValues)
	resolved := resolveEntries(entries, nil, "", po)
	forms := map[string]PemeriksaanForm{}
	for _, form := range defaultPemeriksaanProfile("p").Forms {
		forms[form.Code] = form
	}
	// Urutan baris mengikuti template, bukan urutan centang di profil.
	reversed := PemeriksaanForm{Rows: []PemeriksaanRow{{Ref: ItemKindHoseDischarge}, {Ref: ItemKindHoseSuction}}}
	rows, number := pemeriksaanRowsFor(reversed, entries, resolved)
	if len(rows) != 2 || number != "PO-1 / PO-2" || rows[0].Description != "SELANG HISAP (6 M) MERK TRILLIUNHOSE" || rows[1].Description != "SELANG BUANG (10 M) MERK YAMAKOYO" {
		t.Fatalf("rows=%+v po=%q", rows, number)
	}
	// Keterangan dari Dokumen Pendukung form.
	rows, _ = pemeriksaanRowsFor(forms["mesin"], entries, resolved)
	if rows[0].Description != `MESIN SHARK TYPE SPWP 80-30/3"` || rows[0].Unit != "PAKET" || rows[0].QuantityPerPackage != 1 || rows[0].Notes != "Dilengkapi :\n- Manual Book\n- Garansi" {
		t.Fatalf("machine rows=%+v", rows)
	}
	rows, _ = pemeriksaanRowsFor(forms["oli"], entries, resolved)
	if rows[0].Description != "OLI MERK PERTAMINA ENDURO" || rows[0].Unit != "LTR" || rows[0].QuantityPerPackage != 2 || rows[0].Notes != "" {
		t.Fatalf("oil rows=%+v", rows)
	}
}

func TestTKDNRowsAndSelectionValidate(t *testing.T) {
	for _, rows := range [][]TKDNRow{nil, {{Name: "", Ref: "machine"}}, {{Name: "Pompa", Ref: "pump"}}, {{Name: "Oli", Ref: "component:Oli Besar"}}} {
		if err := normalizeTKDNRows(rows); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("rows=%+v err=%v", rows, err)
		}
	}
	entries := mustEntries(twoMachineTemplateValues)
	clean, err := normalizeItemSelection(entries, map[string]string{ItemKindMachine: "honda", "component:oil": "oil"})
	if err != nil || len(clean) != 1 || clean[ItemKindMachine] != "honda" {
		t.Fatalf("clean=%v err=%v", clean, err)
	}
	if _, err := normalizeItemSelection(entries, map[string]string{ItemKindMachine: "yamaha"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown variant err=%v", err)
	}
}

func TestDominantMachineBrandUsesHighestCount(t *testing.T) {
	rows := []ClosingKabupatenRow{{MachineBrand: "SHARK", Count: 3}, {MachineBrand: "HONDA", Count: 2}, {MachineBrand: "HONDA", Count: 2}}
	if got := dominantMachineBrand(rows); got != "HONDA" {
		t.Fatalf("brand=%q", got)
	}
}
