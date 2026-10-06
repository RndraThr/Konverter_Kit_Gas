package programs

import (
	"encoding/json"
	"testing"
)

func TestSelectMachineAutoSelectsSingleVariant(t *testing.T) {
	values := map[string]any{
		"machine_options": []any{map[string]any{
			"code": "m1", "brand": "SHARK", "type": "SPWP 80-30/3\"", "power": "5.5 HP", "fuel_type": "Bensin",
		}},
	}
	selected, ok := SelectMachine(values, "")
	if !ok || selected.Code != "m1" || selected.Brand != "SHARK" || selected.Power != "5.5 HP" || selected.FuelType != "Bensin" {
		t.Fatalf("unexpected auto-selection: %+v ok=%v", selected, ok)
	}
}

func TestSelectMachineRequiresChoiceForMultipleVariants(t *testing.T) {
	values := map[string]any{
		"machine_options": []any{
			map[string]any{"code": "m1", "brand": "A", "type": "T1", "power": "P1", "fuel_type": "F1"},
			map[string]any{"code": "m2", "brand": "B", "type": "T2", "power": "P2", "fuel_type": "F2"},
		},
	}
	if _, ok := SelectMachine(values, ""); ok {
		t.Fatalf("expected no auto-selection with multiple variants")
	}
	selected, ok := SelectMachine(values, "m2")
	if !ok || selected.Code != "m2" || selected.Brand != "B" {
		t.Fatalf("unexpected explicit selection: %+v ok=%v", selected, ok)
	}
}

func TestSelectMachineRejectsUnknownCode(t *testing.T) {
	values := map[string]any{
		"machine_options": []any{map[string]any{"code": "m1", "brand": "A", "type": "T", "power": "P", "fuel_type": "F"}},
	}
	if _, ok := SelectMachine(values, "missing"); ok {
		t.Fatalf("expected unknown code to fail")
	}
}

func TestBuildAllocationSnapshotResolvesSelectedMachine(t *testing.T) {
	raw := []byte(`{"machine_options":[{"code":"m1","brand":"SHARK","type":"SPWP 80-30/3\"","power":"5.5 HP","fuel_type":"Bensin"}]}`)
	snapshot, err := BuildAllocationSnapshot(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	var decoded AllocationSnapshot
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SelectedMachine == nil || decoded.SelectedMachine.Brand != "SHARK" {
		t.Fatalf("unexpected snapshot: %s", snapshot)
	}
}

func TestBuildAllocationSnapshotMarksIncompleteWhenAmbiguous(t *testing.T) {
	raw := []byte(`{"machine_options":[{"code":"m1","brand":"A","type":"T","power":"P","fuel_type":"F"},{"code":"m2","brand":"B","type":"T","power":"P","fuel_type":"F"}]}`)
	snapshot, err := BuildAllocationSnapshot(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	var decoded AllocationSnapshot
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SelectedMachine != nil {
		t.Fatalf("expected nil selected_machine for ambiguous template, got %s", snapshot)
	}
}

func TestResolveMachineCodeFromCellMatchesCodeBrandOrBrandType(t *testing.T) {
	raw := []byte(`{"machine_options":[{"code":"shark-spwp8030","brand":"SHARK","type":"SPWP 80-30/3\"","power":"5.5 HP","fuel_type":"Bensin"},{"code":"yanmar-tf","brand":"YANMAR","type":"TF 65","power":"6.5 HP","fuel_type":"Solar"}]}`)
	cases := map[string]string{
		"shark-spwp8030":       "shark-spwp8030",
		"SHARK":                "shark-spwp8030",
		"shark spwp 80-30/3\"": "shark-spwp8030",
		"yanmar tf 65":         "yanmar-tf",
		"YANMAR":               "yanmar-tf",
	}
	for cell, want := range cases {
		got, ok := ResolveMachineCodeFromCell(raw, cell)
		if !ok || got != want {
			t.Fatalf("resolve(%q)=%q ok=%v want %q", cell, got, ok, want)
		}
	}
	if _, ok := ResolveMachineCodeFromCell(raw, "  "); ok {
		t.Fatalf("blank cell should not resolve")
	}
	if _, ok := ResolveMachineCodeFromCell(raw, "TidakAda"); ok {
		t.Fatalf("unknown brand should not resolve")
	}
}
