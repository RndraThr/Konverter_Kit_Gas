package bast

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrAggregateNoRecipients          = errors.New("no recipients are available for this schedule")
	ErrHandoverLocationRequired       = errors.New("handover location is required")
	ErrSignatoryRequired              = errors.New("document signatories are incomplete")
	ErrRecipientIdentityIncomplete    = errors.New("recipient identity is incomplete")
	ErrAllocationSnapshotIncomplete   = errors.New("allocation snapshot is incomplete")
	ErrVerificationSnapshotIncomplete = errors.New("verification snapshot is incomplete")
	ErrMachinePowerRequired           = errors.New("machine power is missing")
	ErrMachineFuelRequired            = errors.New("machine fuel type is missing")
)

// DP3Context membawa identitas jadwal/program/kabupaten serta konfigurasi yang
// dibutuhkan renderer DP3.
type DP3Context struct {
	ScheduleID      string
	ProgramID       string
	RegencyID       string
	ProgramType     string
	RegencyName     string
	ZoneName        string
	ZonePlaceholder bool
	FiscalYear      int
	StartDate       string
	HasActiveLogo   bool
}

// DP3Recipient adalah satu baris nominatif DP3 setelah mesin ter-resolve.
type DP3Recipient struct {
	FullName           string `json:"full_name"`
	NIK                string `json:"nik"`
	Address            string `json:"address"`
	Village            string `json:"village"`
	District           string `json:"district"`
	Regency            string `json:"regency"`
	DistributionNumber *int   `json:"distribution_number"`
	MachineBrand       string `json:"machine_brand"`
	MachineType        string `json:"machine_type"`
	MachinePower       string `json:"machine_power"`
	MachineFuelType    string `json:"machine_fuel_type"`
	Source             string `json:"source"`
}

func (r DP3Recipient) Mounted() bool { return r.DistributionNumber != nil }

type DP3Summary struct {
	TotalRecipients     int    `json:"total_recipients"`
	NumberedRecipients  int    `json:"numbered_recipients"`
	UnmountedRecipients int    `json:"unmounted_recipients"`
	ValidationStatus    string `json:"validation_status"`
}

type dp3Signatories struct {
	AgricultureOfficeName string `json:"agriculture_office_name"`
	AgricultureOfficeNIP  string `json:"agriculture_office_nip"`
	InstallerName         string `json:"installer_name"`
	SupervisorName        string `json:"supervisor_name"`
}

type dp3RecipientSnapshot struct {
	FullName           string `json:"full_name"`
	NIK                string `json:"nik"`
	Address            string `json:"address"`
	Village            string `json:"village"`
	District           string `json:"district"`
	Regency            string `json:"regency"`
	DistributionNumber *int   `json:"distribution_number"`
	MachineBrand       string `json:"machine_brand"`
	MachineType        string `json:"machine_type"`
	MachinePower       string `json:"machine_power"`
	MachineFuelType    string `json:"machine_fuel_type"`
	Source             string `json:"source"`
}

// DP3Snapshot adalah bentuk bast_aggregate_documents.snapshot_json untuk DP3.
type DP3Snapshot struct {
	DocumentType           string                  `json:"document_type"`
	DocumentDate           string                  `json:"document_date"`
	RegencyName            string                  `json:"regency_name"`
	HandoverLocation       string                  `json:"handover_location"`
	ConsultantCompanyName  string                  `json:"consultant_company_name"`
	FiscalYear             int                     `json:"fiscal_year"`
	Logos                  []LogoSnapshot          `json:"logos"`
	Signatories            dp3Signatories          `json:"signatories"`
	Recipients             []dp3RecipientSnapshot  `json:"recipients"`
}

func buildDP3Snapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, recipients []DP3Recipient) (DP3Snapshot, error) {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	rows := make([]dp3RecipientSnapshot, 0, len(recipients))
	for _, recipient := range recipients {
		rows = append(rows, dp3RecipientSnapshot{
			FullName: recipient.FullName, NIK: recipient.NIK, Address: recipient.Address,
			Village: recipient.Village, District: recipient.District, Regency: recipient.Regency,
			DistributionNumber: recipient.DistributionNumber, MachineBrand: recipient.MachineBrand,
			MachineType: recipient.MachineType, MachinePower: recipient.MachinePower,
			MachineFuelType: recipient.MachineFuelType, Source: recipient.Source,
		})
	}
	return DP3Snapshot{
		DocumentType:          AggregateDocumentDP3,
		DocumentDate:          documentDate,
		RegencyName:           ctx.RegencyName,
		HandoverLocation:      settings.HandoverLocation,
		ConsultantCompanyName: settings.ConsultantCompanyName,
		FiscalYear:            ctx.FiscalYear,
		Logos:                 sortedLogos,
		Signatories: dp3Signatories{
			AgricultureOfficeName: settings.AgricultureOfficeName,
			AgricultureOfficeNIP:  settings.AgricultureOfficeNIP,
			InstallerName:         settings.InstallerName,
			SupervisorName:        settings.SupervisorName,
		},
		Recipients: rows,
	}, nil
}

func validateDP3Context(ctx DP3Context) error {
	if ctx.ProgramType != "farmer" {
		return ErrTemplateUnavailable
	}
	if ctx.ZonePlaceholder || strings.TrimSpace(ctx.ZoneName) == "" {
		return ErrZoneNotConfigured
	}
	if !ctx.HasActiveLogo {
		return ErrBrandingNotConfigured
	}
	return nil
}

func validateDP3Settings(settings ScheduleSettings) error {
	if strings.TrimSpace(settings.HandoverLocation) == "" {
		return ErrHandoverLocationRequired
	}
	if strings.TrimSpace(settings.AgricultureOfficeName) == "" || strings.TrimSpace(settings.AgricultureOfficeNIP) == "" || strings.TrimSpace(settings.InstallerName) == "" || strings.TrimSpace(settings.SupervisorName) == "" {
		return ErrSignatoryRequired
	}
	return nil
}

func validateDP3Recipients(recipients []DP3Recipient) error {
	if len(recipients) == 0 {
		return ErrAggregateNoRecipients
	}
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.FullName) == "" || strings.TrimSpace(recipient.NIK) == "" {
			return ErrRecipientIdentityIncomplete
		}
		if strings.TrimSpace(recipient.Address) == "" && strings.TrimSpace(recipient.Village) == "" && strings.TrimSpace(recipient.District) == "" {
			return ErrRecipientIdentityIncomplete
		}
		if strings.TrimSpace(recipient.MachineBrand) == "" || strings.TrimSpace(recipient.MachineType) == "" {
			if recipient.Mounted() {
				return ErrVerificationSnapshotIncomplete
			}
			return ErrAllocationSnapshotIncomplete
		}
		if strings.TrimSpace(recipient.MachinePower) == "" {
			return ErrMachinePowerRequired
		}
		if strings.TrimSpace(recipient.MachineFuelType) == "" {
			return ErrMachineFuelRequired
		}
	}
	return nil
}

// dp3ValidationStatus memetakan error validasi DP3 ke kode status yang dibedakan UI.
func dp3ValidationStatus(err error) string {
	switch {
	case err == nil:
		return "ready"
	case errors.Is(err, ErrZoneNotConfigured):
		return "zone_not_configured"
	case errors.Is(err, ErrBrandingNotConfigured):
		return "ba_logo_required"
	case errors.Is(err, ErrHandoverLocationRequired):
		return "handover_location_required"
	case errors.Is(err, ErrSignatoryRequired):
		return "signatory_required"
	case errors.Is(err, ErrAggregateNoRecipients):
		return "no_recipients"
	case errors.Is(err, ErrRecipientIdentityIncomplete):
		return "recipient_identity_incomplete"
	case errors.Is(err, ErrAllocationSnapshotIncomplete):
		return "allocation_snapshot_incomplete"
	case errors.Is(err, ErrVerificationSnapshotIncomplete):
		return "verification_snapshot_incomplete"
	case errors.Is(err, ErrMachinePowerRequired):
		return "machine_power_required"
	case errors.Is(err, ErrMachineFuelRequired):
		return "machine_fuel_required"
	default:
		return "error"
	}
}

// resolveDP3Recipients memetakan baris SQL mentah menjadi []DP3Recipient dengan
// prioritas snapshot verifikasi di atas snapshot alokasi.
func resolveDP3Recipients(rows []dp3RawRecipient) ([]DP3Recipient, error) {
	recipients := make([]DP3Recipient, 0, len(rows))
	for _, row := range rows {
		recipient := DP3Recipient{
			FullName: row.FullName, NIK: row.NIK, Address: row.Address, Village: row.Village,
			District: row.District, Regency: row.Regency, DistributionNumber: row.DistributionNumber,
		}
		if equipment, ok := equipmentFromDP3Verification(row.VerificationSnapshot); ok {
			recipient.MachineBrand, recipient.MachineType = equipment.MachineBrand, equipment.MachineType
			recipient.MachinePower, recipient.MachineFuelType = equipment.MachinePower, equipment.MachineFuelType
			recipient.Source = "verification_snapshot"
		} else {
			machine, ok := selectedMachineFromAllocationSnapshot(row.AllocationSnapshot)
			if !ok {
				recipient.Source = "allocation_snapshot"
			} else {
				recipient.MachineBrand, recipient.MachineType = machine.Brand, machine.Type
				recipient.MachinePower, recipient.MachineFuelType = machine.Power, machine.FuelType
				recipient.Source = "allocation_snapshot"
			}
		}
		recipients = append(recipients, recipient)
	}
	return recipients, nil
}

type dp3RawRecipient struct {
	FullName           string
	NIK                string
	Address            string
	Village            string
	District           string
	Regency            string
	DistributionNumber *int
	AllocationSnapshot []byte
	VerificationSnapshot []byte
}

type dp3MachineSnapshot struct {
	MachineBrand    string `json:"machine_brand"`
	MachineType     string `json:"machine_type"`
	MachinePower    string `json:"machine_power"`
	MachineFuelType string `json:"machine_fuel_type"`
}

func equipmentFromDP3Verification(raw []byte) (dp3MachineSnapshot, bool) {
	if len(raw) == 0 {
		return dp3MachineSnapshot{}, false
	}
	var envelope struct {
		Equipment *dp3MachineSnapshot `json:"equipment"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Equipment == nil {
		return dp3MachineSnapshot{}, false
	}
	return *envelope.Equipment, true
}

type dp3SelectedMachine struct {
	Code     string `json:"code"`
	Brand    string `json:"brand"`
	Type     string `json:"type"`
	Power    string `json:"power"`
	FuelType string `json:"fuel_type"`
}

func selectedMachineFromAllocationSnapshot(raw []byte) (dp3SelectedMachine, bool) {
	if len(raw) == 0 {
		return dp3SelectedMachine{}, false
	}
	var envelope struct {
		SelectedMachine *dp3SelectedMachine `json:"selected_machine"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.SelectedMachine == nil {
		return dp3SelectedMachine{}, false
	}
	return *envelope.SelectedMachine, true
}

func formatDP3Filename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("DP3 - %s - %s - V%d.pdf", name, documentDate, version)
}
