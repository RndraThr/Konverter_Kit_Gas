package bast

import "testing"

func TestEquipmentFromVerificationSnapshotUsesCompletedDistributionValues(t *testing.T) {
	raw := []byte(`{"equipment":{"machine_option_code":"machine-old","machine_brand":"SHARK LAMA","machine_type":"SPWP LAMA","machine_serial":"M-001","hose_option_code":"hose-old","hose_brand":"SELANG LAMA","hose_spec":"6 M / 10 M","hose_serial":"H-001","converter_option_code":"converter-old","converter_brand":"ERGAS LAMA","converter_serial":"C-001"}}`)

	got, ok, err := equipmentFromVerificationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("verification snapshot was not detected")
	}
	if got.MachineBrand != "SHARK LAMA" || got.MachineType != "SPWP LAMA" || got.MachineSerial != "M-001" {
		t.Fatalf("machine=%+v", got)
	}
	if got.HoseBrand != "SELANG LAMA" || got.HoseSpec != "6 M / 10 M" || got.HoseSerial != "H-001" {
		t.Fatalf("hose=%+v", got)
	}
	if got.ConverterBrand != "ERGAS LAMA" || got.ConverterSerial != "C-001" {
		t.Fatalf("converter=%+v", got)
	}
}

func TestEquipmentFromVerificationSnapshotFallsBackForLegacySlot(t *testing.T) {
	got, ok, err := equipmentFromVerificationSnapshot([]byte(`{"fixture":"legacy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != (EquipmentSnapshot{}) {
		t.Fatalf("got=%+v ok=%v, want empty legacy fallback", got, ok)
	}
}
