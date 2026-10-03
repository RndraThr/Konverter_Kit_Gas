package bast

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ClosingKabupatenRow adalah satu baris ringkasan varian mesin per lokasi/
// titik serah dalam satu kabupaten/kota (rekapitulasi lintas seluruh titik
// serah pada jadwal, bukan lintas hari seperti Closing Titik Serah).
type ClosingKabupatenRow struct {
	Location     string `json:"location"`
	MachineBrand string `json:"machine_brand"`
	MachineType  string `json:"machine_type"`
	Count        int    `json:"count"`
}

// ClosingKabupatenSnapshot adalah bentuk bast_aggregate_documents.snapshot_json
// untuk Closing Kabupaten/Kota.
type ClosingKabupatenSnapshot struct {
	DocumentType          string               `json:"document_type"`
	DocumentDate          string               `json:"document_date"`
	DocumentNumber        string               `json:"document_number"`
	RegencyName           string               `json:"regency_name"`
	RegencyCode           string               `json:"regency_code"`
	ProvinceName          string               `json:"province_name"`
	ConsultantCompanyName string               `json:"consultant_company_name"`
	FiscalYear            int                  `json:"fiscal_year"`
	Logos                 []LogoSnapshot       `json:"logos"`
	Signatories           closingSignatories   `json:"signatories"`
	Rows                  []ClosingKabupatenRow `json:"rows"`
	GrandTotal            int                  `json:"grand_total"`
}

func buildClosingKabupatenSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, version int, rows []ClosingKabupatenRow) ClosingKabupatenSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return ClosingKabupatenSnapshot{
		DocumentType:          AggregateDocumentClosingKabupaten,
		DocumentDate:          documentDate,
		DocumentNumber:        formatClosingKabupatenDocumentNumber(ctx.RegencyCode, documentDate, total, version),
		RegencyName:           ctx.RegencyName,
		RegencyCode:           ctx.RegencyCode,
		ProvinceName:          ctx.ProvinceName,
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

// formatClosingKabupatenDocumentNumber mengikuti format dokumen referensi:
// {seq:03d}/{jumlahPaket}/{kodeKota}/KKT/CK/{bulanRomawi}/{tahun}.
func formatClosingKabupatenDocumentNumber(regencyCode, documentDate string, totalPaket, sequence int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	return fmt.Sprintf("%03d/%d/%s/KKT/CK/%s/%d", sequence, totalPaket, strings.ToUpper(strings.TrimSpace(regencyCode)), romanMonth(date.Month()), date.Year())
}

func validateClosingKabupatenSettings(settings ScheduleSettings) error {
	// HandoverLocation tetap divalidasi walau tidak dicetak sebagai metadata
	// dokumen (hanya Kabupaten/Provinsi) — nilainya dipakai mengisi kolom
	// "Lokasi / Titik Serah" pada setiap baris tabel.
	if strings.TrimSpace(settings.HandoverLocation) == "" {
		return ErrHandoverLocationRequired
	}
	if strings.TrimSpace(settings.AgricultureOfficeName) == "" || strings.TrimSpace(settings.AgricultureOfficeNIP) == "" || strings.TrimSpace(settings.InstallerName) == "" || strings.TrimSpace(settings.SupervisorName) == "" || strings.TrimSpace(settings.PertaminaRepName) == "" {
		return ErrSignatoryRequired
	}
	return nil
}

func validateClosingKabupatenRows(rows []ClosingKabupatenRow) error {
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

func closingKabupatenValidationStatus(err error) string {
	return dp3ValidationStatus(err)
}

func formatClosingKabupatenFilename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("CLOSING KABUPATEN - %s - %s - V%d.pdf", name, documentDate, version)
}
