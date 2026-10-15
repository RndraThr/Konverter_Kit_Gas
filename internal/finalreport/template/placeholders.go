package template

// Placeholder tokens baked into final_report_template.pptx's text runs.
// Task 9's substitution engine replaces every occurrence of each token with
// the corresponding live value before saving/presenting.
const (
	TokenRegencyLabel            = "{{REGENCY_LABEL}}"             // "KABUPATEN WAJO"
	TokenFiscalYear              = "{{FISCAL_YEAR}}"
	TokenAgricultureOfficeName   = "{{AGRICULTURE_OFFICE_NAME}}"
	TokenConsultantCompanyName   = "{{CONSULTANT_COMPANY_NAME}}"
	TokenMachineManufacturerName = "{{MACHINE_MANUFACTURER_NAME}}"
	TokenDeliveryHoseDate        = "{{DELIVERY_HOSE_DATE}}"
	TokenDeliveryMachineGroupDate = "{{DELIVERY_MACHINE_GROUP_DATE}}"
	TokenDeliveryGasCylinderDate = "{{DELIVERY_GAS_CYLINDER_DATE}}"
	TokenDeliveryRangeStart      = "{{DELIVERY_RANGE_START}}"
	TokenDeliveryRangeEnd        = "{{DELIVERY_RANGE_END}}"
	TokenRakordaDate             = "{{RAKORDA_DATE}}"
	TokenSosialisasiDate         = "{{SOSIALISASI_DATE}}"
	TokenLeaderCount             = "{{LEADER_COUNT}}"
	TokenAdminCount              = "{{ADMIN_COUNT}}"
	TokenTechnicianCount         = "{{TECHNICIAN_COUNT}}"
	TokenHelperCount             = "{{HELPER_COUNT}}"
	TokenHandoverLocation        = "{{HANDOVER_LOCATION}}"
	TokenDistributionStartDate   = "{{DISTRIBUTION_START_DATE}}"
	TokenDistributionEndDate     = "{{DISTRIBUTION_END_DATE}}"
	TokenDistributionTotal       = "{{DISTRIBUTION_TOTAL}}"
)

// AllTokens lists every token above, used by Task 9's tests to assert the
// committed template actually contains each one at least once.
var AllTokens = []string{
	TokenRegencyLabel, TokenFiscalYear, TokenAgricultureOfficeName, TokenConsultantCompanyName,
	TokenMachineManufacturerName, TokenDeliveryHoseDate, TokenDeliveryMachineGroupDate,
	TokenDeliveryGasCylinderDate, TokenDeliveryRangeStart, TokenDeliveryRangeEnd, TokenRakordaDate,
	TokenSosialisasiDate, TokenLeaderCount, TokenAdminCount, TokenTechnicianCount, TokenHelperCount,
	TokenHandoverLocation, TokenDistributionStartDate, TokenDistributionEndDate, TokenDistributionTotal,
}
