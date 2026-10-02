package bast

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DailyRecapRecipient adalah satu baris Rekap Harian (distribusi selesai).
type DailyRecapRecipient struct {
	SlotNumber       int    `json:"slot_number"`
	FullName         string `json:"full_name"`
	FarmerCardNumber string `json:"farmer_card_number"`
	MachineBrand     string `json:"machine_brand"`
	MachineType      string `json:"machine_type"`
	MachineSerial    string `json:"machine_serial"`
	MachinePower     string `json:"machine_power"`
	MachineFuelType  string `json:"machine_fuel_type"`
}

// DailyRecapVariant adalah ringkasan Grand Total per varian mesin.
type DailyRecapVariant struct {
	Label        string `json:"label"`
	MachineBrand string `json:"machine_brand"`
	MachineType  string `json:"machine_type"`
	Count        int    `json:"count"`
}

type DailyRecapDate struct {
	LocalDate        string             `json:"local_date"`
	RecipientCount   int                `json:"recipient_count"`
	ValidationStatus string             `json:"validation_status"`
	Document         *AggregateDocument `json:"document,omitempty"`
}

type dailyRecapSignatories struct {
	AgricultureOfficeName string `json:"agriculture_office_name"`
	AgricultureOfficeNIP  string `json:"agriculture_office_nip"`
	InstallerName         string `json:"installer_name"`
	SupervisorName        string `json:"supervisor_name"`
	PertaminaRepName      string `json:"pertamina_rep_name"`
}

type dailyRecapRecipientSnapshot struct {
	SlotNumber       int    `json:"slot_number"`
	FullName         string `json:"full_name"`
	FarmerCardNumber string `json:"farmer_card_number"`
	MachineBrand     string `json:"machine_brand"`
	MachineType      string `json:"machine_type"`
	MachineSerial    string `json:"machine_serial"`
	MachinePower     string `json:"machine_power"`
	MachineFuelType  string `json:"machine_fuel_type"`
}

type DailyRecapSnapshot struct {
	DocumentType          string                        `json:"document_type"`
	DocumentDate          string                        `json:"document_date"`
	RegencyName           string                        `json:"regency_name"`
	HandoverLocation      string                        `json:"handover_location"`
	ConsultantCompanyName string                        `json:"consultant_company_name"`
	FiscalYear            int                           `json:"fiscal_year"`
	Logos                 []LogoSnapshot                `json:"logos"`
	Signatories           dailyRecapSignatories         `json:"signatories"`
	Recipients            []dailyRecapRecipientSnapshot `json:"recipients"`
	Variants              []DailyRecapVariant           `json:"variants"`
	GrandTotal            int                           `json:"grand_total"`
}

func buildDailyRecapSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, documentDate string, recipients []DailyRecapRecipient) DailyRecapSnapshot {
	sortedLogos := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sortedLogos, func(i, j int) bool { return sortedLogos[i].SortOrder < sortedLogos[j].SortOrder })
	rows := make([]dailyRecapRecipientSnapshot, 0, len(recipients))
	for _, recipient := range recipients {
		rows = append(rows, dailyRecapRecipientSnapshot{
			SlotNumber: recipient.SlotNumber, FullName: recipient.FullName,
			FarmerCardNumber: recipient.FarmerCardNumber, MachineBrand: recipient.MachineBrand,
			MachineType: recipient.MachineType, MachineSerial: recipient.MachineSerial,
			MachinePower: recipient.MachinePower, MachineFuelType: recipient.MachineFuelType,
		})
	}
	variants := groupDailyRecapVariants(recipients)
	return DailyRecapSnapshot{
		DocumentType:          AggregateDocumentDailyRecap,
		DocumentDate:          documentDate,
		RegencyName:           ctx.RegencyName,
		HandoverLocation:      settings.HandoverLocation,
		ConsultantCompanyName: settings.ConsultantCompanyName,
		FiscalYear:            ctx.FiscalYear,
		Logos:                 sortedLogos,
		Signatories: dailyRecapSignatories{
			AgricultureOfficeName: settings.AgricultureOfficeName,
			AgricultureOfficeNIP:  settings.AgricultureOfficeNIP,
			InstallerName:         settings.InstallerName,
			SupervisorName:        settings.SupervisorName,
			PertaminaRepName:      settings.PertaminaRepName,
		},
		Recipients: rows,
		Variants:   variants,
		GrandTotal: len(recipients),
	}
}

func groupDailyRecapVariants(recipients []DailyRecapRecipient) []DailyRecapVariant {
	type key struct{ brand, typ, power, fuel string }
	order := []key{}
	counts := map[key]int{}
	for _, recipient := range recipients {
		k := key{
			brand: strings.TrimSpace(recipient.MachineBrand),
			typ:   strings.TrimSpace(recipient.MachineType),
			power: strings.TrimSpace(recipient.MachinePower),
			fuel:  strings.TrimSpace(recipient.MachineFuelType),
		}
		if _, exists := counts[k]; !exists {
			order = append(order, k)
		}
		counts[k]++
	}
	variants := make([]DailyRecapVariant, 0, len(order))
	for i, k := range order {
		variants = append(variants, DailyRecapVariant{
			Label:        fmt.Sprintf("Varian %d", i+1),
			MachineBrand: k.brand,
			MachineType:  k.typ,
			Count:        counts[k],
		})
	}
	return variants
}

func validateDailyRecapSettings(settings ScheduleSettings) error {
	if strings.TrimSpace(settings.HandoverLocation) == "" {
		return ErrHandoverLocationRequired
	}
	if strings.TrimSpace(settings.AgricultureOfficeName) == "" || strings.TrimSpace(settings.AgricultureOfficeNIP) == "" || strings.TrimSpace(settings.InstallerName) == "" || strings.TrimSpace(settings.SupervisorName) == "" || strings.TrimSpace(settings.PertaminaRepName) == "" {
		return ErrSignatoryRequired
	}
	return nil
}

func validateDailyRecapRecipients(recipients []DailyRecapRecipient) error {
	if len(recipients) == 0 {
		return ErrAggregateNoRecipients
	}
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.FullName) == "" {
			return ErrRecipientIdentityIncomplete
		}
		if strings.TrimSpace(recipient.MachineBrand) == "" || strings.TrimSpace(recipient.MachineType) == "" || strings.TrimSpace(recipient.MachineSerial) == "" {
			return ErrVerificationSnapshotIncomplete
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

func dailyRecapValidationStatus(err error) string {
	return dp3ValidationStatus(err)
}

// decodeDailyRecapRecipients memetakan baris SQL mentah menjadi recipients dengan mesin dari
// snapshot verifikasi distribusi saja.
func decodeDailyRecapRecipients(raw []dailyRecapRawRecipient) ([]DailyRecapRecipient, error) {
	recipients := make([]DailyRecapRecipient, 0, len(raw))
	for _, row := range raw {
		recipient := DailyRecapRecipient{
			SlotNumber: row.SlotNumber, FullName: row.FullName, FarmerCardNumber: row.FarmerCardNumber,
		}
		if len(row.VerificationSnapshot) > 0 {
			var envelope struct {
				Equipment *dp3MachineSnapshot `json:"equipment"`
			}
			if err := json.Unmarshal(row.VerificationSnapshot, &envelope); err == nil && envelope.Equipment != nil {
				recipient.MachineBrand = envelope.Equipment.MachineBrand
				recipient.MachineType = envelope.Equipment.MachineType
				recipient.MachineSerial = row.MachineSerial
				recipient.MachinePower = envelope.Equipment.MachinePower
				recipient.MachineFuelType = envelope.Equipment.MachineFuelType
			}
		}
		recipients = append(recipients, recipient)
	}
	return recipients, nil
}

type dailyRecapRawRecipient struct {
	SlotNumber          int
	FullName            string
	FarmerCardNumber    string
	MachineSerial       string
	VerificationSnapshot []byte
}

func formatDailyRecapFilename(regencyName, documentDate string, version int) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("REKAP HARIAN - %s - %s - V%d.pdf", name, documentDate, version)
}
