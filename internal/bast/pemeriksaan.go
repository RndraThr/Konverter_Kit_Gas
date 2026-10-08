package bast

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"konkit/internal/textnorm"
)

// pemeriksaanDocumentPrefix + kode form menjadi document_type agregat, sehingga
// setiap form barang mempunyai versi dan file sendiri.
const pemeriksaanDocumentPrefix = "pemeriksaan:"

var pemeriksaanCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// PemeriksaanRow adalah satu baris tabel "A. Berdasarkan". Di profil hanya Ref
// (barang template) yang disimpan; di snapshot deskripsi, unit, jumlah per
// paket (dari template), dan keterangan sudah diisi.
type PemeriksaanRow struct {
	Ref                string  `json:"ref"`
	Description        string  `json:"description"`
	Unit               string  `json:"unit"`
	QuantityPerPackage float64 `json:"quantity_per_package"`
	Notes              string  `json:"notes"`
}

// PemeriksaanChecklist menyimpan pilihan bagian "B. Hasil Pemeriksaan
// Barang" yang dicetak tercentang.
type PemeriksaanChecklist struct {
	Packaging     string   `json:"packaging"`
	Quantity      string   `json:"quantity"`
	Specification string   `json:"specification"`
	Condition     string   `json:"condition"`
	Documents     []string `json:"documents"`
	OtherDocument string   `json:"other_document"`
	FunctionTest  string   `json:"function_test"`
	Conclusion    string   `json:"conclusion"`
}

// PemeriksaanForm adalah satu form BA Pemeriksaan untuk satu jenis barang.
type PemeriksaanForm struct {
	Code      string               `json:"code"`
	Title     string               `json:"title"`
	Rows      []PemeriksaanRow     `json:"rows"`
	Checklist PemeriksaanChecklist `json:"checklist"`
	Note      string               `json:"note"`
}

// PemeriksaanProfile adalah daftar form BA Pemeriksaan satu program/tender.
type PemeriksaanProfile struct {
	ProgramID string            `json:"program_id"`
	Forms     []PemeriksaanForm `json:"forms"`
	IsDefault bool              `json:"is_default"`
	UpdatedAt *time.Time        `json:"updated_at,omitempty"`
}

var pemeriksaanChoices = map[string][]string{
	"packaging":     {"baik", "rusak"},
	"quantity":      {"lengkap", "kurang"},
	"specification": {"sesuai", "tidak_sesuai"},
	"condition":     {"baru_baik", "tidak_baik", "bekas"},
	"documents":     {"sertifikat", "manual_book", "garansi", "lainnya"},
	"function_test": {"ya", "tidak"},
	"conclusion":    {"diterima", "tidak_diterima"},
}

const defaultPemeriksaanNote = "Barang diterima dengan baik dan sesuai spesifikasi dalam PO"

func defaultPemeriksaanChecklist(documents ...string) PemeriksaanChecklist {
	// append ke slice kosong agar JSON selalu [] (bukan null) untuk form
	// tanpa dokumen pendukung default.
	return PemeriksaanChecklist{Packaging: "baik", Quantity: "lengkap", Specification: "sesuai", Condition: "baru_baik", Documents: append([]string{}, documents...), FunctionTest: "ya", Conclusion: "diterima"}
}

// ensurePemeriksaanLists mengganti slice nil dengan slice kosong agar API
// selalu mengirim array, termasuk untuk profil lama yang tersimpan dengan null.
func ensurePemeriksaanLists(forms []PemeriksaanForm) {
	for i := range forms {
		if forms[i].Checklist.Documents == nil {
			forms[i].Checklist.Documents = []string{}
		}
		if forms[i].Rows == nil {
			forms[i].Rows = []PemeriksaanRow{}
		}
	}
}

// defaultPemeriksaanProfile mengikuti tujuh form referensi BA Pemeriksaan
// Petani dan dipakai sampai program menyimpan daftarnya sendiri. Setiap baris
// menunjuk barang template; merk dan jumlah mengikuti template jadwal.
func defaultPemeriksaanProfile(programID string) PemeriksaanProfile {
	refs := func(values ...string) []PemeriksaanRow {
		rows := make([]PemeriksaanRow, 0, len(values))
		for _, ref := range values {
			rows = append(rows, PemeriksaanRow{Ref: ref})
		}
		return rows
	}
	form := func(code, title string, rows []PemeriksaanRow, documents ...string) PemeriksaanForm {
		return PemeriksaanForm{Code: code, Title: title, Rows: rows, Checklist: defaultPemeriksaanChecklist(documents...), Note: defaultPemeriksaanNote}
	}
	return PemeriksaanProfile{ProgramID: programID, IsDefault: true, Forms: []PemeriksaanForm{
		form("mesin", "Mesin Pompa", refs(ItemKindMachine), "manual_book", "garansi"),
		form("konverter_kit", "Konverter Kit", refs(ItemKindConverter), "manual_book", "garansi"),
		form("regulator", "Regulator", refs("component:regulator")),
		form("selang_gas", "Selang Gas", refs("component:hose_clamp_accessories")),
		form("selang_hisap_buang", "Selang Hisap & Buang", refs(ItemKindHoseSuction, ItemKindHoseDischarge)),
		form("oli", "Oli", refs("component:oil")),
		form("tabung_gas", "Tabung Gas", refs("component:lpg_tank")),
	}}
}

func (p *PemeriksaanProfile) normalize() error {
	p.ProgramID = strings.TrimSpace(p.ProgramID)
	if p.ProgramID == "" || len(p.Forms) == 0 || len(p.Forms) > 30 {
		return ErrInvalidInput
	}
	seen := map[string]bool{}
	for i := range p.Forms {
		form := &p.Forms[i]
		form.Code = strings.ToLower(strings.TrimSpace(form.Code))
		form.Title = strings.Join(strings.Fields(form.Title), " ")
		form.Note = strings.TrimSpace(form.Note)
		if !pemeriksaanCodePattern.MatchString(form.Code) || seen[form.Code] || form.Title == "" {
			return ErrInvalidInput
		}
		seen[form.Code] = true
		if len(form.Rows) > 10 {
			return ErrInvalidInput
		}
		for j := range form.Rows {
			// Profil hanya menyimpan barang template; isi baris dihitung dari template.
			form.Rows[j] = PemeriksaanRow{Ref: strings.TrimSpace(form.Rows[j].Ref)}
			if !itemRefPattern.MatchString(form.Rows[j].Ref) {
				return ErrInvalidInput
			}
		}
		if err := form.Checklist.normalize(); err != nil {
			return err
		}
	}
	return nil
}

func (c *PemeriksaanChecklist) normalize() error {
	singles := map[string]*string{"packaging": &c.Packaging, "quantity": &c.Quantity, "specification": &c.Specification, "condition": &c.Condition, "function_test": &c.FunctionTest, "conclusion": &c.Conclusion}
	for key, value := range singles {
		*value = strings.TrimSpace(*value)
		if *value != "" && !slices.Contains(pemeriksaanChoices[key], *value) {
			return ErrInvalidInput
		}
	}
	documents := []string{}
	for _, document := range c.Documents {
		document = strings.TrimSpace(document)
		if !slices.Contains(pemeriksaanChoices["documents"], document) {
			return ErrInvalidInput
		}
		if !slices.Contains(documents, document) {
			documents = append(documents, document)
		}
	}
	c.Documents = documents
	c.OtherDocument = strings.TrimSpace(c.OtherDocument)
	return nil
}

func pemeriksaanDocumentType(code string) string { return pemeriksaanDocumentPrefix + code }

// PemeriksaanSnapshot adalah snapshot_json satu form BA Pemeriksaan.
type PemeriksaanSnapshot struct {
	DocumentType  string             `json:"document_type"`
	DocumentDate  string             `json:"document_date"`
	PONumber      string             `json:"po_number"`
	SupplierName  string             `json:"supplier_name"`
	RegencyName   string             `json:"regency_name"`
	ProvinceName  string             `json:"province_name"`
	FiscalYear    int                `json:"fiscal_year"`
	TotalPackages int                `json:"total_packages"`
	Form          PemeriksaanForm    `json:"form"`
	Logos         []LogoSnapshot     `json:"logos"`
	Signatories   closingSignatories `json:"signatories"`
}

// PemeriksaanFormSummary adalah satu form dengan baris dan No. PO yang sudah
// diturunkan dari Template Paket jadwal untuk kabupaten.
type PemeriksaanFormSummary struct {
	Code     string           `json:"code"`
	Title    string           `json:"title"`
	PONumber string           `json:"po_number"`
	Rows     []PemeriksaanRow `json:"rows"`
}

// PemeriksaanSummary memberi panel jumlah paket, daftar form, dan isi tiap
// form untuk kabupaten jadwal.
type PemeriksaanSummary struct {
	TotalPackages int                      `json:"total_packages"`
	Profile       PemeriksaanProfile       `json:"profile"`
	Forms         []PemeriksaanFormSummary `json:"forms"`
	Entries       []TemplateEntry          `json:"entries"`
}

func buildPemeriksaanSnapshot(ctx DP3Context, settings ScheduleSettings, form PemeriksaanForm, poNumber string, logos []LogoSnapshot, documentDate string, totalPackages int) PemeriksaanSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	return PemeriksaanSnapshot{
		DocumentType:  pemeriksaanDocumentType(form.Code),
		DocumentDate:  documentDate,
		PONumber:      poNumber,
		SupplierName:  strings.TrimSpace(settings.ConsultantCompanyName),
		RegencyName:   textnorm.BusinessUpper(ctx.RegencyName),
		ProvinceName:  textnorm.BusinessUpper(ctx.ProvinceName),
		FiscalYear:    ctx.FiscalYear,
		TotalPackages: totalPackages,
		Form:          form,
		Logos:         sortedLogos,
		Signatories: closingSignatories{
			AgricultureOfficeName: textnorm.BusinessUpper(settings.AgricultureOfficeName),
			AgricultureOfficeNIP:  strings.TrimSpace(settings.AgricultureOfficeNIP),
			InstallerName:         textnorm.BusinessUpper(settings.InstallerName),
			SupervisorName:        textnorm.BusinessUpper(settings.SupervisorName),
			PertaminaRepName:      textnorm.BusinessUpper(settings.PertaminaRepName),
		},
	}
}

func formatPemeriksaanFilename(formTitle, regencyName, documentDate string, version int) string {
	title := strings.ToUpper(strings.NewReplacer("/", "-", "\\", "-").Replace(strings.Join(strings.Fields(formTitle), " ")))
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("BA PEMERIKSAAN %s - %s - %s - V%d.pdf", title, name, documentDate, version)
}

// pemeriksaanNarrative menyusun "Pada Hari ini, Selasa Tanggal 3 November
// Tahun 2026, Telah dilakukan Pemeriksaan Fisik Barang ..." sesuai referensi.
func pemeriksaanNarrative(documentDate string, fiscalYear int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("Pada Hari ini, %s Tanggal %d %s Tahun %d, Telah dilakukan Pemeriksaan Fisik Barang atas Paket Konversi BBM ke BBG bagi Petani Sasaran Program Konversi Tahun %d, dengan rincian sebagai berikut:",
		days[date.Weekday()], date.Day(), months[date.Month()], date.Year(), fiscalYear)
}
