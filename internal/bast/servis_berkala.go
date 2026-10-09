package bast

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"konkit/internal/textnorm"
)

const AggregateDocumentServisBerkala = "servis_berkala"

var ErrServisScheduleRequired = errors.New("servis berkala schedule is incomplete")

// ServisPeriod adalah satu rentang tanggal servis (YYYY-MM-DD).
type ServisPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ServisBerkalaSnapshot adalah bentuk bast_aggregate_documents.snapshot_json
// untuk BA Agenda Servis Berkala.
type ServisBerkalaSnapshot struct {
	DocumentType          string             `json:"document_type"`
	DocumentDate          string             `json:"document_date"`
	DocumentNumber        string             `json:"document_number"`
	RegencyName           string             `json:"regency_name"`
	RegencyCode           string             `json:"regency_code"`
	ProvinceName          string             `json:"province_name"`
	ZoneName              string             `json:"zone_name"`
	FiscalYear            int                `json:"fiscal_year"`
	TotalPackages         int                `json:"total_packages"`
	Services              []ServisPeriod     `json:"services"`
	ConsultantCompanyName string             `json:"consultant_company_name"`
	Logos                 []LogoSnapshot     `json:"logos"`
	Signatories           closingSignatories `json:"signatories"`
}

// ServisBerkalaSummary memberi panel info kesiapan dokumen tanpa merender PDF.
type ServisBerkalaSummary struct {
	TotalPackages int            `json:"total_packages"`
	Services      []ServisPeriod `json:"services"`
	ScheduleReady bool           `json:"schedule_ready"`
}

func servisPeriods(settings ScheduleSettings) []ServisPeriod {
	return []ServisPeriod{
		{Start: settings.Servis1Start, End: settings.Servis1End},
		{Start: settings.Servis2Start, End: settings.Servis2End},
	}
}

func validateServisPeriods(periods []ServisPeriod) error {
	for _, period := range periods {
		if !validRequiredDate(period.Start) || !validRequiredDate(period.End) || period.Start > period.End {
			return ErrServisScheduleRequired
		}
	}
	return nil
}

func validRequiredDate(value string) bool {
	return value != "" && validOptionalDate(value)
}

func buildServisBerkalaSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, version, totalPackages int) ServisBerkalaSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	return ServisBerkalaSnapshot{
		DocumentType:          AggregateDocumentServisBerkala,
		DocumentDate:          documentDate,
		DocumentNumber:        formatServisBerkalaDocumentNumber(ctx.RegencyCode, documentDate, totalPackages, version),
		RegencyName:           textnorm.BusinessUpper(ctx.RegencyName),
		RegencyCode:           ctx.RegencyCode,
		ProvinceName:          textnorm.BusinessUpper(ctx.ProvinceName),
		ZoneName:              ctx.ZoneName,
		FiscalYear:            ctx.FiscalYear,
		TotalPackages:         totalPackages,
		Services:              servisPeriods(settings),
		ConsultantCompanyName: strings.TrimSpace(settings.ConsultantCompanyName),
		Logos:                 sortedLogos,
		Signatories: closingSignatories{
			AgricultureOfficeName: textnorm.BusinessUpper(settings.AgricultureOfficeName),
			AgricultureOfficeNIP:  strings.TrimSpace(settings.AgricultureOfficeNIP),
			InstallerName:         textnorm.BusinessUpper(settings.InstallerName),
		},
	}
}

// formatServisBerkalaDocumentNumber mengikuti dokumen referensi:
// {seq}/{jumlahPaket}/{kodeKota}/KKT/SB/{bulanRomawi}/{tahun}, dengan lebar
// digit seq mengikuti jumlah digit jumlah paket seperti Closing Kabupaten.
func formatServisBerkalaDocumentNumber(regencyCode, documentDate string, totalPackages, sequence int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	return fmt.Sprintf("%0*d/%d/%s/KKT/SB/%s/%d", digitWidth(totalPackages), sequence, totalPackages, strings.ToUpper(strings.TrimSpace(regencyCode)), romanMonth(date.Month()), date.Year())
}

func formatServisBerkalaFilename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("SERVIS BERKALA - %s - %s - V%d.pdf", name, documentDate, version)
}

// formatServisRange menulis "12 Mei 2026 s/d 20 Mei 2026" seperti referensi.
func formatServisRange(period ServisPeriod) string {
	return formatTitleDate(period.Start) + " s/d " + formatTitleDate(period.End)
}

func formatTitleDate(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "……………"
	}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%d %s %d", date.Day(), months[date.Month()], date.Year())
}
