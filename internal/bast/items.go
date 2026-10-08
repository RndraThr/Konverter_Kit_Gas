package bast

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Barang Realisasi TKDN dan BA Pemeriksaan berasal dari Template Paket jadwal.
// Template menyimpan data barang (merk, tipe/spek, jumlah, % TKDN); profil BA
// hanya menyusun dokumen dengan menunjuk barang template lewat referensi:
//
//	machine, converter, hose_suction, hose_discharge, component:<kode>
//
// Barang opsi (mesin, konkit, selang) bisa punya beberapa merk; satu dipilih
// per kabupaten. No. PO disimpan per zona per barang+merk (bast_zone_po_numbers).

// Jenis barang template; juga item_kind pada bast_zone_po_numbers.
const (
	ItemKindMachine       = "machine"
	ItemKindConverter     = "converter"
	ItemKindHoseSuction   = "hose_suction"
	ItemKindHoseDischarge = "hose_discharge"
	ItemKindComponent     = "component"
)

var itemRefPattern = regexp.MustCompile(`^(machine|converter|hose_suction|hose_discharge|component:[a-z0-9_-]{1,60})$`)

// TemplateVariant adalah satu merk barang template.
type TemplateVariant struct {
	Code        string  `json:"code"`
	Brand       string  `json:"brand"`
	Type        string  `json:"type"`
	Spec        string  `json:"spec"`
	TKDNPercent float64 `json:"tkdn_percent"`
}

// TemplateEntry adalah satu barang template yang dapat dirujuk profil BA.
type TemplateEntry struct {
	Ref                string            `json:"ref"`
	Kind               string            `json:"kind"`
	Label              string            `json:"label"`
	Unit               string            `json:"unit"`
	QuantityPerPackage float64           `json:"quantity_per_package"`
	Variants           []TemplateVariant `json:"variants"`
}

type packageTemplateValues struct {
	MachineOptions []struct {
		Code        string  `json:"code"`
		Brand       string  `json:"brand"`
		Type        string  `json:"type"`
		TKDNPercent float64 `json:"tkdn_percent"`
	} `json:"machine_options"`
	ConverterOptions []struct {
		Code        string  `json:"code"`
		Brand       string  `json:"brand"`
		Spec        string  `json:"spec"`
		TKDNPercent float64 `json:"tkdn_percent"`
	} `json:"converter_options"`
	HoseOptions []struct {
		Code                 string  `json:"code"`
		Brand                string  `json:"brand"`
		Spec                 string  `json:"spec"`
		SuctionBrand         string  `json:"suction_brand"`
		SuctionSpec          string  `json:"suction_spec"`
		DischargeBrand       string  `json:"discharge_brand"`
		DischargeSpec        string  `json:"discharge_spec"`
		SuctionTKDNPercent   float64 `json:"suction_tkdn_percent"`
		DischargeTKDNPercent float64 `json:"discharge_tkdn_percent"`
	} `json:"hose_options"`
	Components []struct {
		Code        string  `json:"code"`
		Label       string  `json:"label"`
		Quantity    float64 `json:"quantity"`
		Unit        string  `json:"unit"`
		Brand       string  `json:"brand"`
		TKDNPercent float64 `json:"tkdn_percent"`
	} `json:"components"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// TemplateEntriesFromValues mengurai values_json template paket menjadi barang
// yang dapat dirujuk Realisasi TKDN dan BA Pemeriksaan.
func TemplateEntriesFromValues(raw []byte) ([]TemplateEntry, error) {
	var values packageTemplateValues
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("decode package template values: %w", err)
		}
	}
	entries := []TemplateEntry{}
	if len(values.MachineOptions) > 0 {
		entry := TemplateEntry{Ref: ItemKindMachine, Kind: ItemKindMachine, Label: "Mesin", Unit: "PAKET", QuantityPerPackage: 1}
		for _, o := range values.MachineOptions {
			entry.Variants = append(entry.Variants, TemplateVariant{Code: o.Code, Brand: o.Brand, Type: o.Type, TKDNPercent: o.TKDNPercent})
		}
		entries = append(entries, entry)
	}
	if len(values.ConverterOptions) > 0 {
		entry := TemplateEntry{Ref: ItemKindConverter, Kind: ItemKindConverter, Label: "Konkit/Reducer", Unit: "PAKET", QuantityPerPackage: 1}
		for _, o := range values.ConverterOptions {
			entry.Variants = append(entry.Variants, TemplateVariant{Code: o.Code, Brand: o.Brand, Spec: o.Spec, TKDNPercent: o.TKDNPercent})
		}
		entries = append(entries, entry)
	}
	if len(values.HoseOptions) > 0 {
		suction := TemplateEntry{Ref: ItemKindHoseSuction, Kind: ItemKindHoseSuction, Label: "Selang Hisap", Unit: "PAKET", QuantityPerPackage: 1}
		discharge := TemplateEntry{Ref: ItemKindHoseDischarge, Kind: ItemKindHoseDischarge, Label: "Selang Buang", Unit: "PAKET", QuantityPerPackage: 1}
		for _, o := range values.HoseOptions {
			suction.Variants = append(suction.Variants, TemplateVariant{Code: o.Code, Brand: firstNonEmpty(o.SuctionBrand, o.Brand), Spec: firstNonEmpty(o.SuctionSpec, o.Spec), TKDNPercent: o.SuctionTKDNPercent})
			discharge.Variants = append(discharge.Variants, TemplateVariant{Code: o.Code, Brand: firstNonEmpty(o.DischargeBrand, o.Brand), Spec: firstNonEmpty(o.DischargeSpec, o.Spec), TKDNPercent: o.DischargeTKDNPercent})
		}
		entries = append(entries, suction, discharge)
	}
	for _, c := range values.Components {
		if strings.TrimSpace(c.Code) == "" {
			continue
		}
		entries = append(entries, TemplateEntry{
			Ref: ItemKindComponent + ":" + c.Code, Kind: ItemKindComponent, Label: c.Label, Unit: c.Unit, QuantityPerPackage: c.Quantity,
			Variants: []TemplateVariant{{Code: c.Code, Brand: firstNonEmpty(c.Brand, "-"), TKDNPercent: c.TKDNPercent}},
		})
	}
	return entries, nil
}

func entryByRef(entries []TemplateEntry, ref string) (TemplateEntry, bool) {
	index := slices.IndexFunc(entries, func(e TemplateEntry) bool { return e.Ref == ref })
	if index < 0 {
		return TemplateEntry{}, false
	}
	return entries[index], true
}

// Sumber pemilihan merk satu barang pada satu jadwal.
const (
	ChoiceSourceSingle  = "single"
	ChoiceSourceManual  = "manual"
	ChoiceSourceMachine = "machine"
	ChoiceSourceDefault = "default"
)

// ResolvedEntry adalah barang template dengan merk terpilih untuk satu kabupaten.
type ResolvedEntry struct {
	Entry    TemplateEntry
	Variant  TemplateVariant
	Source   string
	PONumber string
}

func poKey(kind, code string) string { return kind + "|" + code }

// resolveEntries memilih satu merk per barang untuk satu kabupaten: pilihan
// manual jadwal, lalu merk mesin terdistribusi (barang mesin), lalu merk
// pertama. No. PO diambil dari poNumbers zona kabupaten.
func resolveEntries(entries []TemplateEntry, selection map[string]string, machineBrand string, poNumbers map[string]string) map[string]ResolvedEntry {
	resolved := make(map[string]ResolvedEntry, len(entries))
	for _, entry := range entries {
		if len(entry.Variants) == 0 {
			continue
		}
		chosen, source := entry.Variants[0], ChoiceSourceDefault
		if len(entry.Variants) == 1 {
			source = ChoiceSourceSingle
		} else if index := findVariant(entry, selection[entry.Ref]); index >= 0 {
			chosen, source = entry.Variants[index], ChoiceSourceManual
		} else if entry.Kind == ItemKindMachine {
			for _, v := range entry.Variants {
				if brandMatches(v.Brand, machineBrand) {
					chosen, source = v, ChoiceSourceMachine
					break
				}
			}
		}
		resolved[entry.Ref] = ResolvedEntry{Entry: entry, Variant: chosen, Source: source, PONumber: poNumbers[poKey(entry.Kind, chosen.Code)]}
	}
	return resolved
}

func findVariant(entry TemplateEntry, code string) int {
	if code == "" {
		return -1
	}
	return slices.IndexFunc(entry.Variants, func(v TemplateVariant) bool { return v.Code == code })
}

// dominantMachineBrand memilih merk mesin dengan jumlah paket terbanyak.
func dominantMachineBrand(rows []ClosingKabupatenRow) string {
	counts := map[string]int{}
	best, bestCount := "", 0
	for _, row := range rows {
		brand := strings.TrimSpace(row.MachineBrand)
		if brand == "" {
			continue
		}
		counts[brand] += row.Count
		if counts[brand] > bestCount {
			best, bestCount = brand, counts[brand]
		}
	}
	return best
}

func brandMatches(variantBrand, machineBrand string) bool {
	a, b := strings.ToUpper(strings.TrimSpace(variantBrand)), strings.ToUpper(strings.TrimSpace(machineBrand))
	return a != "" && b != "" && (a == b || strings.Contains(a, b) || strings.Contains(b, a))
}

// inspectionDescription menyusun deskripsi BA Pemeriksaan dari data template.
func inspectionDescription(entry TemplateEntry, v TemplateVariant) string {
	var text string
	switch entry.Kind {
	case ItemKindMachine:
		text = "MESIN " + v.Brand + " TYPE " + v.Type
	case ItemKindConverter:
		text = "KONVERTER KIT " + v.Brand
	case ItemKindHoseSuction, ItemKindHoseDischarge:
		label := "SELANG HISAP"
		if entry.Kind == ItemKindHoseDischarge {
			label = "SELANG BUANG"
		}
		if strings.TrimSpace(v.Spec) != "" {
			label += " (" + strings.TrimSpace(v.Spec) + ")"
		}
		text = label + " MERK " + v.Brand
	default:
		text = entry.Label
		if brand := strings.TrimSpace(v.Brand); brand != "" && brand != "-" && !strings.EqualFold(brand, "N.A") {
			text += " MERK " + brand
		}
	}
	return strings.ToUpper(strings.Join(strings.Fields(text), " "))
}

var inspectionDocumentLabels = map[string]string{"sertifikat": "Sertifikat", "manual_book": "Manual Book", "garansi": "Garansi"}

// inspectionNotes menyusun kolom Keterangan dari Dokumen Pendukung yang dicentang.
func inspectionNotes(checklist PemeriksaanChecklist) string {
	var lines []string
	for _, document := range checklist.Documents {
		label := inspectionDocumentLabels[document]
		if document == "lainnya" {
			label = strings.TrimSpace(checklist.OtherDocument)
		}
		if label != "" {
			lines = append(lines, "- "+label)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Dilengkapi :\n" + strings.Join(lines, "\n")
}

// TKDNRow adalah satu baris susunan Realisasi TKDN program: nama dan kelompok
// tampil di dokumen, Ref menunjuk barang template sumber merk dan % TKDN.
type TKDNRow struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Ref   string `json:"ref"`
}

// defaultTKDNRows mengikuti dokumen referensi Realisasi TKDN Petani.
func defaultTKDNRows() []TKDNRow {
	const converter, lpg = "Konverter Kit dan kelengkapannya", "Tabung LPG 3 kg beserta isinya"
	return []TKDNRow{
		{Name: "Pompa Air Irigasi", Ref: ItemKindMachine},
		{Name: "Konverter Kit", Group: converter, Ref: ItemKindConverter},
		{Name: "Selang LPG", Group: converter, Ref: "component:hose_clamp_accessories"},
		{Name: "Regulator LPG", Group: converter, Ref: "component:regulator"},
		{Name: "Minyak Pelumas (Oli)", Ref: "component:oil"},
		{Name: "Tabung LPG 3 kg", Group: lpg, Ref: "component:lpg_tank"},
		{Name: "Isi LPG 3 kg", Group: lpg, Ref: "component:lpg_refill"},
		{Name: "Bracket", Ref: "component:bracket"},
		{Name: "Selang Hisap", Ref: ItemKindHoseSuction},
		{Name: "Selang Buang", Ref: ItemKindHoseDischarge},
		{Name: "Plat Anti Karat", Ref: "component:anti_rust_plate"},
	}
}

func normalizeTKDNRows(rows []TKDNRow) error {
	if len(rows) == 0 || len(rows) > 50 {
		return ErrInvalidInput
	}
	for i := range rows {
		rows[i].Name = strings.Join(strings.Fields(rows[i].Name), " ")
		rows[i].Group = strings.Join(strings.Fields(rows[i].Group), " ")
		rows[i].Ref = strings.TrimSpace(rows[i].Ref)
		if rows[i].Name == "" || !itemRefPattern.MatchString(rows[i].Ref) {
			return ErrInvalidInput
		}
	}
	return nil
}

// tkdnItemsFor menurunkan baris Realisasi TKDN dari susunan program dan barang
// template kabupaten. Baris yang barangnya tidak ada di template dilewati.
func tkdnItemsFor(rows []TKDNRow, resolved map[string]ResolvedEntry) []TKDNItem {
	items := make([]TKDNItem, 0, len(rows))
	for _, row := range rows {
		r, ok := resolved[row.Ref]
		if !ok {
			continue
		}
		items = append(items, TKDNItem{Group: row.Group, Name: row.Name, Brand: r.Variant.Brand, QuantityPerPackage: r.Entry.QuantityPerPackage, TKDNPercent: r.Variant.TKDNPercent})
	}
	return items
}

// pemeriksaanRowsFor menurunkan baris tabel "A. Berdasarkan" satu form dan
// No. PO-nya. Baris mengikuti urutan barang di template; deskripsi dan unit dari
// template, keterangan dari Dokumen Pendukung. Bila barang dalam satu form
// ber-PO berbeda, semua nomor dicetak.
func pemeriksaanRowsFor(form PemeriksaanForm, entries []TemplateEntry, resolved map[string]ResolvedEntry) ([]PemeriksaanRow, string) {
	wanted := map[string]bool{}
	for _, row := range form.Rows {
		wanted[row.Ref] = true
	}
	notes := inspectionNotes(form.Checklist)
	rows := []PemeriksaanRow{}
	var numbers []string
	for _, entry := range entries {
		r, ok := resolved[entry.Ref]
		if !wanted[entry.Ref] || !ok {
			continue
		}
		rows = append(rows, PemeriksaanRow{Ref: entry.Ref, Description: inspectionDescription(entry, r.Variant), Unit: strings.ToUpper(firstNonEmpty(entry.Unit, "PCS")), QuantityPerPackage: entry.QuantityPerPackage, Notes: notes})
		if r.PONumber != "" && !slices.Contains(numbers, r.PONumber) {
			numbers = append(numbers, r.PONumber)
		}
	}
	return rows, strings.Join(numbers, " / ")
}

type VariantRef struct {
	Code  string `json:"code"`
	Brand string `json:"brand"`
}

// ScheduleItemChoice adalah satu barang "Merk & No. PO" kabupaten jadwal.
type ScheduleItemChoice struct {
	Ref       string       `json:"ref"`
	Name      string       `json:"name"`
	Variants  []VariantRef `json:"variants"`
	Selected  string       `json:"selected"`
	Source    string       `json:"source"`
	PONumber  string       `json:"po_number"`
	Inspected bool         `json:"inspected"`
}

type ScheduleItems struct {
	ScheduleID   string               `json:"schedule_id"`
	ProgramID    string               `json:"program_id"`
	ZoneName     string               `json:"zone_name"`
	MachineBrand string               `json:"machine_brand"`
	Items        []ScheduleItemChoice `json:"items"`
}

// referencedRefs mengembalikan barang yang dirujuk susunan TKDN atau form
// Pemeriksaan, berurutan, beserta nama tampilnya dan apakah diperiksa.
func referencedRefs(rows []TKDNRow, forms []PemeriksaanForm) ([]string, map[string]string, map[string]bool) {
	var order []string
	names, inspected := map[string]string{}, map[string]bool{}
	add := func(ref, name string) {
		if _, ok := names[ref]; !ok {
			order = append(order, ref)
			names[ref] = name
		}
	}
	for _, row := range rows {
		add(row.Ref, row.Name)
	}
	for _, form := range forms {
		for _, row := range form.Rows {
			add(row.Ref, form.Title)
			inspected[row.Ref] = true
		}
	}
	return order, names, inspected
}

// normalizeItemSelection menyimpan hanya pilihan untuk barang bermerk ganda.
func normalizeItemSelection(entries []TemplateEntry, selection map[string]string) (map[string]string, error) {
	clean := map[string]string{}
	for ref, code := range selection {
		ref, code = strings.TrimSpace(ref), strings.TrimSpace(code)
		if code == "" {
			continue
		}
		entry, ok := entryByRef(entries, ref)
		if !ok || findVariant(entry, code) < 0 {
			return nil, ErrInvalidInput
		}
		if len(entry.Variants) > 1 {
			clean[ref] = code
		}
	}
	return clean, nil
}
