package distribution

import (
	"errors"
	"testing"
)

func TestBuildEquipmentVerificationSnapshotResolvesSelectedTemplateValues(t *testing.T) {
	packageJSON := []byte(`{
		"machine_options":[{"code":"machine-1","brand":"SHARK","type":"SPWP","power":"5.5 HP","fuel_type":"Bensin"}],
		"hose_options":[{"code":"hose-1","suction_brand":"TRILLIUNHOSE","suction_spec":"6 M","discharge_brand":"YAMAKOYO","discharge_spec":"10 M"}],
		"converter_options":[{"code":"converter-1","brand":"ERGAS"}]
	}`)

	got, err := buildEquipmentVerificationSnapshot(packageJSON, CreateSlotInput{
		MachineOptionCode: "machine-1", MachineSerialNumber: "M-001",
		HoseOptionCode: "hose-1", HoseSerialNumber: "H-001",
		ConverterOptionCode: "converter-1", ConverterSerialNumber: "C-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.MachineOptionCode != "machine-1" || got.MachineBrand != "SHARK" || got.MachineType != "SPWP" || got.MachineSerial != "M-001" {
		t.Fatalf("machine snapshot=%+v", got)
	}
	if got.MachinePower != "5.5 HP" || got.MachineFuelType != "Bensin" {
		t.Fatalf("machine power/fuel snapshot=%+v", got)
	}
	if got.HoseOptionCode != "hose-1" || got.HoseBrand != "TRILLIUNHOSE\nYAMAKOYO" || got.HoseSpec != "6 M\n10 M" || got.HoseSerial != "H-001" {
		t.Fatalf("hose snapshot=%+v", got)
	}
	if got.ConverterOptionCode != "converter-1" || got.ConverterBrand != "ERGAS" || got.ConverterSerial != "C-001" {
		t.Fatalf("converter snapshot=%+v", got)
	}
}

func TestBuildEquipmentVerificationSnapshotRejectsUnknownSelectedCode(t *testing.T) {
	packageJSON := []byte(`{
		"machine_options":[{"code":"machine-1","brand":"SHARK","type":"SPWP"}],
		"hose_options":[{"code":"hose-1","brand":"TRILLIUNHOSE","spec":"6 M / 10 M"}],
		"converter_options":[{"code":"converter-1","brand":"ERGAS"}]
	}`)

	_, err := buildEquipmentVerificationSnapshot(packageJSON, CreateSlotInput{
		MachineOptionCode: "machine-renamed", MachineSerialNumber: "M-001",
		HoseOptionCode: "hose-1", HoseSerialNumber: "H-001",
		ConverterOptionCode: "converter-1", ConverterSerialNumber: "C-001",
	})
	if !errors.Is(err, ErrEquipmentOptionNotFound) {
		t.Fatalf("err=%v, want ErrEquipmentOptionNotFound", err)
	}
}
