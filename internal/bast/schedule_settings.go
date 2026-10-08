package bast

import (
	"strings"
	"time"

	"konkit/internal/textnorm"
)

// ScheduleSettings menyimpan konfigurasi Berita Acara per jadwal yang dipakai
// header dan halaman tanda tangan DP3 serta Rekap Harian.
type ScheduleSettings struct {
	ScheduleID            string    `json:"schedule_id"`
	HandoverLocation      string    `json:"handover_location"`
	ConsultantCompanyName string    `json:"consultant_company_name"`
	AgricultureOfficeName string    `json:"agriculture_office_name"`
	AgricultureOfficeNIP  string    `json:"agriculture_office_nip"`
	InstallerName         string    `json:"installer_name"`
	SupervisorName        string    `json:"supervisor_name"`
	PertaminaRepName      string    `json:"pertamina_rep_name"`
	RakordaLocation       string    `json:"rakorda_location"`
	RakordaRowCount       int       `json:"rakorda_row_count"`
	SosialisasiLocation   string    `json:"sosialisasi_location"`
	SosialisasiRowCount   int       `json:"sosialisasi_row_count"`
	Training10Location    string    `json:"training_10_location"`
	Training10RowCount    int       `json:"training_10_row_count"`
	Training100Location   string    `json:"training_100_location"`
	Training100RowCount   int       `json:"training_100_row_count"`
	Servis1Start          string    `json:"servis_1_start"`
	Servis1End            string    `json:"servis_1_end"`
	Servis2Start          string    `json:"servis_2_start"`
	Servis2End            string    `json:"servis_2_end"`
	UpdatedAt             time.Time `json:"updated_at"`
}

const (
	defaultRakordaRowCount     = 45
	defaultSosialisasiRowCount = 51
	defaultTraining10RowCount  = 51
	defaultTraining100RowCount = 51
)

// ScheduleSettingsInput membawa nilai konfigurasi BA per jadwal dari request.
type ScheduleSettingsInput struct {
	ScheduleID            string `json:"schedule_id"`
	HandoverLocation      string `json:"handover_location"`
	ConsultantCompanyName string `json:"consultant_company_name"`
	AgricultureOfficeName string `json:"agriculture_office_name"`
	AgricultureOfficeNIP  string `json:"agriculture_office_nip"`
	InstallerName         string `json:"installer_name"`
	SupervisorName        string `json:"supervisor_name"`
	PertaminaRepName      string `json:"pertamina_rep_name"`
	RakordaLocation       string `json:"rakorda_location"`
	RakordaRowCount       int    `json:"rakorda_row_count"`
	SosialisasiLocation   string `json:"sosialisasi_location"`
	SosialisasiRowCount   int    `json:"sosialisasi_row_count"`
	Training10Location    string `json:"training_10_location"`
	Training10RowCount    int    `json:"training_10_row_count"`
	Training100Location   string `json:"training_100_location"`
	Training100RowCount   int    `json:"training_100_row_count"`
	// Jadwal Servis Berkala (YYYY-MM-DD, boleh kosong sampai ditetapkan).
	Servis1Start string `json:"servis_1_start"`
	Servis1End   string `json:"servis_1_end"`
	Servis2Start string `json:"servis_2_start"`
	Servis2End   string `json:"servis_2_end"`
}

func (in *ScheduleSettingsInput) normalize() error {
	in.ScheduleID = strings.TrimSpace(in.ScheduleID)
	in.HandoverLocation = textnorm.BusinessUpper(in.HandoverLocation)
	in.ConsultantCompanyName = textnorm.BusinessUpper(in.ConsultantCompanyName)
	in.AgricultureOfficeName = textnorm.BusinessUpper(in.AgricultureOfficeName)
	in.AgricultureOfficeNIP = strings.TrimSpace(in.AgricultureOfficeNIP)
	in.InstallerName = textnorm.BusinessUpper(in.InstallerName)
	in.SupervisorName = textnorm.BusinessUpper(in.SupervisorName)
	in.PertaminaRepName = textnorm.BusinessUpper(in.PertaminaRepName)
	in.RakordaLocation = textnorm.BusinessUpper(in.RakordaLocation)
	in.SosialisasiLocation = textnorm.BusinessUpper(in.SosialisasiLocation)
	in.Training10Location = textnorm.BusinessUpper(in.Training10Location)
	in.Training100Location = textnorm.BusinessUpper(in.Training100Location)
	if in.RakordaRowCount == 0 {
		in.RakordaRowCount = defaultRakordaRowCount
	}
	if in.SosialisasiRowCount == 0 {
		in.SosialisasiRowCount = defaultSosialisasiRowCount
	}
	if in.Training10RowCount == 0 {
		in.Training10RowCount = defaultTraining10RowCount
	}
	if in.Training100RowCount == 0 {
		in.Training100RowCount = defaultTraining100RowCount
	}
	if in.ScheduleID == "" {
		return ErrInvalidInput
	}
	for _, rows := range []int{in.RakordaRowCount, in.SosialisasiRowCount, in.Training10RowCount, in.Training100RowCount} {
		if rows < 5 || rows > 200 {
			return ErrInvalidInput
		}
	}
	for _, period := range []struct{ start, end *string }{{&in.Servis1Start, &in.Servis1End}, {&in.Servis2Start, &in.Servis2End}} {
		*period.start, *period.end = strings.TrimSpace(*period.start), strings.TrimSpace(*period.end)
		if !validOptionalDate(*period.start) || !validOptionalDate(*period.end) {
			return ErrInvalidInput
		}
		if *period.start != "" && *period.end != "" && *period.start > *period.end {
			return ErrInvalidInput
		}
	}
	return nil
}

func validOptionalDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
