package distribution

import (
	"encoding/json"
	"fmt"
	"strings"
)

type packageEquipmentValues struct {
	MachineOptions []struct {
		Code     string `json:"code"`
		Brand    string `json:"brand"`
		Type     string `json:"type"`
		Power    string `json:"power"`
		FuelType string `json:"fuel_type"`
	} `json:"machine_options"`
	HoseOptions []struct {
		Code           string `json:"code"`
		Brand          string `json:"brand"`
		Spec           string `json:"spec"`
		SuctionBrand   string `json:"suction_brand"`
		SuctionSpec    string `json:"suction_spec"`
		DischargeBrand string `json:"discharge_brand"`
		DischargeSpec  string `json:"discharge_spec"`
	} `json:"hose_options"`
	ConverterOptions []struct {
		Code  string `json:"code"`
		Brand string `json:"brand"`
	} `json:"converter_options"`
}

type equipmentVerificationSnapshot struct {
	MachineOptionCode   string `json:"machine_option_code"`
	MachineBrand        string `json:"machine_brand"`
	MachineType         string `json:"machine_type"`
	MachinePower        string `json:"machine_power"`
	MachineFuelType     string `json:"machine_fuel_type"`
	MachineSerial       string `json:"machine_serial"`
	HoseOptionCode      string `json:"hose_option_code"`
	HoseBrand           string `json:"hose_brand"`
	HoseSpec            string `json:"hose_spec"`
	HoseSerial          string `json:"hose_serial"`
	ConverterOptionCode string `json:"converter_option_code"`
	ConverterBrand      string `json:"converter_brand"`
	ConverterSerial     string `json:"converter_serial"`
}

func buildEquipmentVerificationSnapshot(packageJSON []byte, selected CreateSlotInput) (equipmentVerificationSnapshot, error) {
	var values packageEquipmentValues
	if err := json.Unmarshal(packageJSON, &values); err != nil {
		return equipmentVerificationSnapshot{}, fmt.Errorf("decode package equipment: %w", err)
	}

	result := equipmentVerificationSnapshot{
		MachineOptionCode:   strings.TrimSpace(selected.MachineOptionCode),
		MachineSerial:       strings.TrimSpace(selected.MachineSerialNumber),
		HoseOptionCode:      strings.TrimSpace(selected.HoseOptionCode),
		HoseSerial:          strings.TrimSpace(selected.HoseSerialNumber),
		ConverterOptionCode: strings.TrimSpace(selected.ConverterOptionCode),
		ConverterSerial:     strings.TrimSpace(selected.ConverterSerialNumber),
	}
	machineFound, hoseFound, converterFound := false, false, false
	for _, option := range values.MachineOptions {
		if option.Code == result.MachineOptionCode {
			result.MachineBrand, result.MachineType = option.Brand, option.Type
			result.MachinePower, result.MachineFuelType = option.Power, option.FuelType
			machineFound = true
			break
		}
	}
	for _, option := range values.HoseOptions {
		if option.Code == result.HoseOptionCode {
			result.HoseBrand = joinEquipmentPair(option.SuctionBrand, option.DischargeBrand, option.Brand)
			result.HoseSpec = joinEquipmentPair(option.SuctionSpec, option.DischargeSpec, option.Spec)
			hoseFound = true
			break
		}
	}
	for _, option := range values.ConverterOptions {
		if option.Code == result.ConverterOptionCode {
			result.ConverterBrand = option.Brand
			converterFound = true
			break
		}
	}
	if !machineFound || !hoseFound || !converterFound {
		return equipmentVerificationSnapshot{}, ErrEquipmentOptionNotFound
	}
	return result, nil
}

func joinEquipmentPair(first, second, legacy string) string {
	first, second, legacy = strings.TrimSpace(first), strings.TrimSpace(second), strings.TrimSpace(legacy)
	if first == "" && second == "" {
		return legacy
	}
	if first == "" {
		first = legacy
	}
	if second == "" {
		second = legacy
	}
	return first + "\n" + second
}
