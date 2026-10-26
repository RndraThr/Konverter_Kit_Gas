package finalreport

// Preview is the fully-assembled, live data for one Program+Kabupaten,
// shaped 1:1 with the placeholder tokens in template/placeholders.go. Every
// string field is "Belum diisi" when its source data doesn't exist yet —
// Preview/Present/Generate never fail because of incomplete data.
type Preview struct {
	RegencyLabel             string                 `json:"regency_label"`
	FiscalYear               string                 `json:"fiscal_year"`
	AgricultureOfficeName    string                 `json:"agriculture_office_name"`
	ConsultantCompanyName    string                 `json:"consultant_company_name"`
	MachineManufacturerName  string                 `json:"machine_manufacturer_name"`
	DeliveryHoseDate         string                 `json:"delivery_hose_date"`
	DeliveryMachineGroupDate string                 `json:"delivery_machine_group_date"`
	DeliveryGasCylinderDate  string                 `json:"delivery_gas_cylinder_date"`
	DeliveryRangeStart       string                 `json:"delivery_range_start"`
	DeliveryRangeEnd         string                 `json:"delivery_range_end"`
	RakordaDate              string                 `json:"rakorda_date"`
	SosialisasiDate          string                 `json:"sosialisasi_date"`
	LeaderCount              string                 `json:"leader_count"`
	AdminCount               string                 `json:"admin_count"`
	TechnicianCount          string                 `json:"technician_count"`
	HelperCount              string                 `json:"helper_count"`
	HandoverLocation         string                 `json:"handover_location"`
	DistributionStartDate    string                 `json:"distribution_start_date"`
	DistributionEndDate      string                 `json:"distribution_end_date"`
	DistributionTotal        string                 `json:"distribution_total"`
	PhotoPicks               map[string][]PhotoPick `json:"photo_picks"`
}

const blank = "Belum diisi"

func orBlank(value string) string {
	if value == "" {
		return blank
	}
	return value
}
