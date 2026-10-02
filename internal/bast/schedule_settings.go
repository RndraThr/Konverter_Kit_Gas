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
	UpdatedAt             time.Time `json:"updated_at"`
}

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
	if in.ScheduleID == "" {
		return ErrInvalidInput
	}
	return nil
}
