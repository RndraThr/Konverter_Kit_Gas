package bast

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ClosingRow adalah satu baris ringkasan varian mesin pada satu hari closing
// harian dalam rentang jadwal (tidak dibatasi satu tanggal seperti Rekap
// Harian — Closing Titik Serah merangkum seluruh hari yang sudah closing).
type ClosingRow struct {
	LocalDate    string `json:"local_date"`
	MachineBrand string `json:"machine_brand"`
	MachineType  string `json:"machine_type"`
	Count        int    `json:"count"`
}

type closingSignatories struct {
	AgricultureOfficeName string `json:"agriculture_office_name"`
	AgricultureOfficeNIP  string `json:"agriculture_office_nip"`
	InstallerName         string `json:"installer_name"`
	SupervisorName        string `json:"supervisor_name"`
	PertaminaRepName      string `json:"pertamina_rep_name"`
}

// ClosingTitikSerahSnapshot adalah bentuk bast_aggregate_documents.snapshot_json
// untuk Closing Titik Serah.
type ClosingTitikSerahSnapshot struct {
	DocumentType          string             `json:"document_type"`
	DocumentDate          string             `json:"document_date"`
	DocumentNumber        string             `json:"document_number"`
	RegencyName           string             `json:"regency_name"`
	RegencyCode           string             `json:"regency_code"`
	ProvinceName          string             `json:"province_name"`
	HandoverLocation      string             `json:"handover_location"`
	ConsultantCompanyName string             `json:"consultant_company_name"`
	FiscalYear            int                `json:"fiscal_year"`
	Logos                 []LogoSnapshot     `json:"logos"`
	Signatories           closingSignatories `json:"signatories"`
	Rows                  []ClosingRow       `json:"rows"`
	GrandTotal            int                `json:"grand_total"`
}

func buildClosingTitikSerahSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, version int, rows []ClosingRow) ClosingTitikSerahSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return ClosingTitikSerahSnapshot{
		DocumentType:          AggregateDocumentClosingTitikSerah,
		DocumentDate:          documentDate,
		DocumentNumber:        formatClosingDocumentNumber(ctx.RegencyCode, documentDate, total, version),
		RegencyName:           ctx.RegencyName,
		RegencyCode:           ctx.RegencyCode,
		ProvinceName:          ctx.ProvinceName,
		HandoverLocation:      settings.HandoverLocation,
		ConsultantCompanyName: settings.ConsultantCompanyName,
		FiscalYear:            ctx.FiscalYear,
		Logos:                 sortedLogos,
		Signatories: closingSignatories{
			AgricultureOfficeName: settings.AgricultureOfficeName,
			AgricultureOfficeNIP:  settings.AgricultureOfficeNIP,
			InstallerName:         settings.InstallerName,
			SupervisorName:        settings.SupervisorName,
			PertaminaRepName:      settings.PertaminaRepName,
		},
		Rows:       rows,
		GrandTotal: total,
	}
}

// formatClosingDocumentNumber mengikuti format dokumen referensi:
// {seq:03d}/{jumlahPaket}/{kodeKota}/KKT/CTS/{bulanRomawi}/{tahun}.
func formatClosingDocumentNumber(regencyCode, documentDate string, totalPaket, sequence int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	return fmt.Sprintf("%03d/%d/%s/KKT/CTS/%s/%d", sequence, totalPaket, strings.ToUpper(strings.TrimSpace(regencyCode)), romanMonth(date.Month()), date.Year())
}

func validateClosingSettings(settings ScheduleSettings) error {
	if strings.TrimSpace(settings.HandoverLocation) == "" {
		return ErrHandoverLocationRequired
	}
	if strings.TrimSpace(settings.AgricultureOfficeName) == "" || strings.TrimSpace(settings.AgricultureOfficeNIP) == "" || strings.TrimSpace(settings.InstallerName) == "" || strings.TrimSpace(settings.SupervisorName) == "" || strings.TrimSpace(settings.PertaminaRepName) == "" {
		return ErrSignatoryRequired
	}
	return nil
}

func validateClosingRows(rows []ClosingRow) error {
	if len(rows) == 0 {
		return ErrAggregateNoRecipients
	}
	for _, row := range rows {
		if strings.TrimSpace(row.MachineBrand) == "" || strings.TrimSpace(row.MachineType) == "" {
			return ErrVerificationSnapshotIncomplete
		}
	}
	return nil
}

func closingValidationStatus(err error) string {
	return dp3ValidationStatus(err)
}

func formatClosingFilename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("CLOSING TITIK SERAH - %s - %s - V%d.pdf", name, documentDate, version)
}
