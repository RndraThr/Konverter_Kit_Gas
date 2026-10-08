package bast

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"konkit/internal/textnorm"
)

const AggregateDocumentTKDN = "tkdn"

// TKDNItem adalah satu barang pada daftar Realisasi TKDN. Barang berurutan
// dengan Group yang sama dicetak sebagai satu nomor berjudul Group, dengan
// setiap barang sebagai sub-baris "- Nama".
type TKDNItem struct {
	Group              string  `json:"group"`
	Name               string  `json:"name"`
	Brand              string  `json:"brand"`
	QuantityPerPackage float64 `json:"quantity_per_package"`
	TKDNPercent        float64 `json:"tkdn_percent"`
}

// TKDNProfile menyimpan susunan baris Realisasi TKDN satu program/tender dan
// nilai TKDN gabungan yang diisi manual (dihitung dari bobot harga di luar
// sistem). Merk dan % TKDN per barang berasal dari Template Paket jadwal.
type TKDNProfile struct {
	ProgramID string     `json:"program_id"`
	Rows      []TKDNRow  `json:"rows"`
	TotalTKDN float64    `json:"total_tkdn"`
	IsDefault bool       `json:"is_default"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// defaultTKDNProfile mengikuti dokumen referensi Realisasi TKDN Petani dan
// dipakai sampai program menyimpan nilainya sendiri.
func defaultTKDNProfile(programID string) TKDNProfile {
	return TKDNProfile{ProgramID: programID, IsDefault: true, Rows: defaultTKDNRows(), TotalTKDN: 68.53}
}

func (p *TKDNProfile) normalize() error {
	p.ProgramID = strings.TrimSpace(p.ProgramID)
	if p.ProgramID == "" || !validPercent(p.TotalTKDN) {
		return ErrInvalidInput
	}
	if err := normalizeTKDNRows(p.Rows); err != nil {
		return err
	}
	p.TotalTKDN = round2(p.TotalTKDN)
	return nil
}

func validPercent(value float64) bool { return value >= 0 && value <= 100 }

func round2(value float64) float64 { return math.Round(value*100) / 100 }

// TKDNSnapshot adalah bentuk bast_aggregate_documents.snapshot_json untuk
// Realisasi TKDN.
type TKDNSnapshot struct {
	DocumentType   string             `json:"document_type"`
	DocumentDate   string             `json:"document_date"`
	DocumentNumber string             `json:"document_number"`
	ProviderName   string             `json:"provider_name"`
	RegencyName    string             `json:"regency_name"`
	RegencyCode    string             `json:"regency_code"`
	ProvinceName   string             `json:"province_name"`
	FiscalYear     int                `json:"fiscal_year"`
	TotalPackages  int                `json:"total_packages"`
	Items          []TKDNItem         `json:"items"`
	TotalTKDN      float64            `json:"total_tkdn"`
	Logos          []LogoSnapshot     `json:"logos"`
	Signatories    closingSignatories `json:"signatories"`
}

// TKDNSummary memberi panel jumlah paket, susunan TKDN, baris TKDN kabupaten
// jadwal, dan barang template yang dapat dirujuk.
type TKDNSummary struct {
	TotalPackages int             `json:"total_packages"`
	Profile       TKDNProfile     `json:"profile"`
	Items         []TKDNItem      `json:"items"`
	Entries       []TemplateEntry `json:"entries"`
}

func buildTKDNSnapshot(ctx DP3Context, settings ScheduleSettings, profile TKDNProfile, items []TKDNItem, logos []LogoSnapshot, documentDate string, version, totalPackages int) TKDNSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	return TKDNSnapshot{
		DocumentType:   AggregateDocumentTKDN,
		DocumentDate:   documentDate,
		DocumentNumber: formatTKDNDocumentNumber(ctx.RegencyCode, documentDate, totalPackages, version),
		ProviderName:   strings.TrimSpace(settings.ConsultantCompanyName),
		RegencyName:    textnorm.BusinessUpper(ctx.RegencyName),
		RegencyCode:    ctx.RegencyCode,
		ProvinceName:   textnorm.BusinessUpper(ctx.ProvinceName),
		FiscalYear:     ctx.FiscalYear,
		TotalPackages:  totalPackages,
		Items:          append([]TKDNItem(nil), items...),
		TotalTKDN:      profile.TotalTKDN,
		Logos:          sortedLogos,
		Signatories: closingSignatories{
			AgricultureOfficeName: textnorm.BusinessUpper(settings.AgricultureOfficeName),
			AgricultureOfficeNIP:  strings.TrimSpace(settings.AgricultureOfficeNIP),
			InstallerName:         textnorm.BusinessUpper(settings.InstallerName),
			SupervisorName:        textnorm.BusinessUpper(settings.SupervisorName),
			PertaminaRepName:      textnorm.BusinessUpper(settings.PertaminaRepName),
		},
	}
}

// formatTKDNDocumentNumber mengikuti referensi:
// {seq}/{jumlahPaket}/{kodeKota}/KKT/RTKDN/{bulanRomawi}/{tahun}.
func formatTKDNDocumentNumber(regencyCode, documentDate string, totalPackages, sequence int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	return fmt.Sprintf("%0*d/%d/%s/KKT/RTKDN/%s/%d", digitWidth(totalPackages), sequence, totalPackages, strings.ToUpper(strings.TrimSpace(regencyCode)), romanMonth(date.Month()), date.Year())
}

func formatTKDNFilename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("TKDN - %s - %s - V%d.pdf", name, documentDate, version)
}

// formatIndonesianNumber menulis angka dengan koma desimal, tanpa desimal bila
// bulat ("1.200", "61,42").
func formatIndonesianNumber(value float64, decimals int) string {
	text := strconv.FormatFloat(value, 'f', decimals, 64)
	whole, fraction, _ := strings.Cut(text, ".")
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 && whole[i-1] != '-' {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	if fraction == "" {
		return grouped.String()
	}
	return grouped.String() + "," + fraction
}

func formatTKDNPercent(value float64) string {
	return formatIndonesianNumber(value, 2) + "%"
}

func formatTKDNQuantity(perPackage float64, packages int) string {
	total := round2(perPackage * float64(packages))
	if total == math.Trunc(total) {
		return formatIndonesianNumber(total, 0)
	}
	return formatIndonesianNumber(total, 2)
}
