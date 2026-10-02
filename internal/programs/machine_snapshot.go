package programs

import (
	"encoding/json"
	"strings"
)

// MachineOptionData adalah data mesin yang sudah ter-resolve dari template paket,
// disalin ke snapshot alokasi per penerima agar dokumen DP3/Rekap Harian dapat
// dirender tanpa lookup ke template paket terkini.
type MachineOptionData struct {
	Code     string `json:"code"`
	Brand    string `json:"brand"`
	Type     string `json:"type"`
	Power    string `json:"power"`
	FuelType string `json:"fuel_type"`
}

// AllocationSnapshot adalah bentuk package_allocations.package_snapshot_json.
// SelectedMachine nil menandakan snapshot alokasi belum lengkap (beberapa varian
// mesin dan belum ada pilihan per penerima).
type AllocationSnapshot struct {
	SelectedMachine *MachineOptionData `json:"selected_machine"`
}

// SelectMachine meresolve satu varian mesin dari values_json template paket.
// Kode kosong otomatis memilih bila tepat satu varian; mengembalikan ok=false
// bila kode kosong dan varian lebih dari satu, atau kode tidak cocok dengan
// varian mana pun.
func SelectMachine(values map[string]any, code string) (MachineOptionData, bool) {
	options := MachineOptions(values)
	code = strings.TrimSpace(code)
	if code == "" {
		if len(options) == 1 {
			return options[0], true
		}
		return MachineOptionData{}, false
	}
	for _, option := range options {
		if option.Code == code {
			return option, true
		}
	}
	return MachineOptionData{}, false
}

// BuildAllocationSnapshot membangun package_snapshot_json dari values_json
// template paket dan kode mesin terpilih. Bila kode kosong dan hanya ada satu
// varian, varian itu dipilih otomatis; bila tidak dapat dipilih, SelectedMachine
// dibiarkan null agar penerima ditandai allocation_snapshot_incomplete.
func BuildAllocationSnapshot(packageValuesJSON []byte, selectedMachineCode string) ([]byte, error) {
	var values map[string]any
	if len(packageValuesJSON) == 0 {
		values = map[string]any{}
	} else if err := json.Unmarshal(packageValuesJSON, &values); err != nil {
		return nil, err
	}
	snapshot := AllocationSnapshot{}
	if selected, ok := SelectMachine(values, selectedMachineCode); ok {
		snapshot.SelectedMachine = &selected
	}
	return json.Marshal(snapshot)
}

// ResolveMachineCodeFromCell meresolve kode varian mesin dari nilai sel workbook.
// Pencocokan case-insensitive terhadap kode, merek, atau gabungan "merek tipe".
// Mengembalikan ok=false bila sel kosong atau tidak cocok dengan varian mana pun.
func ResolveMachineCodeFromCell(packageValuesJSON []byte, cellValue string) (string, bool) {
	var values map[string]any
	if err := json.Unmarshal(packageValuesJSON, &values); err != nil {
		return "", false
	}
	cellValue = strings.TrimSpace(cellValue)
	if cellValue == "" {
		return "", false
	}
	options := MachineOptions(values)
	for _, option := range options {
		if strings.EqualFold(option.Code, cellValue) {
			return option.Code, true
		}
		if strings.EqualFold(option.Brand, cellValue) {
			return option.Code, true
		}
		if strings.EqualFold(strings.TrimSpace(option.Brand+" "+option.Type), cellValue) {
			return option.Code, true
		}
	}
	return "", false
}

// MachineOptions mengembalikan daftar varian mesin dari values_json template paket.
func MachineOptions(values map[string]any) []MachineOptionData {
	raw, ok := values["machine_options"].([]any)
	if !ok {
		return nil
	}
	options := make([]MachineOptionData, 0, len(raw))
	for _, entry := range raw {
		option, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		options = append(options, MachineOptionData{
			Code:     stringField(option["code"]),
			Brand:    stringField(option["brand"]),
			Type:     stringField(option["type"]),
			Power:    stringField(option["power"]),
			FuelType: stringField(option["fuel_type"]),
		})
	}
	return options
}

func stringField(value any) string {
	text, _ := value.(string)
	return text
}
