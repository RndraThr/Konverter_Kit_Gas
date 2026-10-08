package bast

import (
	"errors"
	"testing"
)

func TestBuildSnapshotNormalizesFarmerDocument(t *testing.T) {
	source := SourceData{ProgramType: "farmer", DocumentNumber: "0001/50/KSM-KKT-WJO/XII/2024", LocalDate: "2024-12-10", Render: RenderIdentity{FiscalYear: 2024, Logos: []LogoSnapshot{{AssetID: "b", SortOrder: 2}, {AssetID: "a", SortOrder: 1}}}, Recipient: RecipientSnapshot{FullName: "Siti Aminah", NIK: "7306014101900001", SectorIdentifier: "KP-01", Address: "Alamat panjang", PhoneNumber: "08123456789"}, Equipment: EquipmentSnapshot{MachineBrand: "SHARK", MachineType: "SPWP 80-30 / 3 inch", MachineSerial: "M-001", HoseBrand: "TRILIUNHOSE", HoseSpec: "6m/10m", ConverterBrand: "ERGAS"}, Components: []ComponentSnapshot{{Code: "lpg", Label: "Tabung LPG 3 Kg", Quantity: 1, Unit: "Tabung"}}, ExecutorName: "Muhamad Wildan M", SupervisorName: "Andi Amrullah", DistributionSlotID: "slot-1"}
	snapshot, err := BuildSnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Render.FiscalYear != 2024 || snapshot.Render.Logos[0].AssetID != "a" || !snapshot.Components[0].Checked {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.Signatures.ReceiverName != "Siti Aminah" || snapshot.Signatures.ExecutorName != "Muhamad Wildan M" || snapshot.Signatures.SupervisorName != "Andi Amrullah" || snapshot.Signatures.ReceiverSignatureKey != "slot-1" {
		t.Fatalf("signatures=%+v", snapshot.Signatures)
	}
	if snapshot.Equipment.HoseSerial != "-" {
		t.Fatalf("hose serial=%q, want -", snapshot.Equipment.HoseSerial)
	}
}

func TestBuildSnapshotRejectsMissingFarmerIdentityAndFisherman(t *testing.T) {
	base := SourceData{ProgramType: "farmer", DocumentNumber: "N", LocalDate: "2024-12-10", Render: RenderIdentity{Logos: []LogoSnapshot{{AssetID: "logo"}}}, Recipient: RecipientSnapshot{FullName: "Siti", NIK: "7306014101900001"}}
	if _, err := BuildSnapshot(base); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing farmer card err=%v", err)
	}
	base.ProgramType = "fisherman"
	base.Recipient.SectorIdentifier = "KUSUKA"
	if _, err := BuildSnapshot(base); !errors.Is(err, ErrTemplateUnavailable) {
		t.Fatalf("fisherman err=%v", err)
	}
}

func TestBuildSnapshotRequiresActiveBrandingLogo(t *testing.T) {
	source := SourceData{ProgramType: "farmer", DocumentNumber: "N", LocalDate: "2024-12-10", Recipient: RecipientSnapshot{FullName: "Siti", NIK: "7306014101900001", SectorIdentifier: "KP-01"}}
	if _, err := BuildSnapshot(source); !errors.Is(err, ErrBrandingNotConfigured) {
		t.Fatalf("missing logo err=%v", err)
	}
}

func TestBuildSnapshotRejectsUnresolvedEquipmentInsteadOfRenderingBlankFields(t *testing.T) {
	source := SourceData{
		ProgramType: "farmer", DocumentNumber: "N", LocalDate: "2024-12-10",
		Render:     RenderIdentity{Logos: []LogoSnapshot{{AssetID: "logo"}}},
		Recipient:  RecipientSnapshot{FullName: "Siti", NIK: "7306014101900001", SectorIdentifier: "KP-01"},
		Equipment:  EquipmentSnapshot{MachineType: "SPWP", HoseBrand: "TRILLIUNHOSE", HoseSpec: "6 M / 10 M", ConverterBrand: "ERGAS"},
		Components: []ComponentSnapshot{{Code: "lpg", Label: "Tabung LPG", Quantity: 1, Unit: "Tabung"}},
	}
	if _, err := BuildSnapshot(source); !errors.Is(err, ErrEquipmentUnavailable) {
		t.Fatalf("err=%v, want ErrEquipmentUnavailable", err)
	}
}

func TestDecodeSnapshotMapsLegacyProfileLogos(t *testing.T) {
	raw := []byte(`{"program_type":"farmer","profile":{"logos":[{"asset_id":"legacy-logo","storage_key":"legacy.png","sort_order":1}]}}`)
	snapshot, err := DecodeSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Render.FiscalYear != 0 || len(snapshot.Render.Logos) != 1 || snapshot.Render.Logos[0].AssetID != "legacy-logo" {
		t.Fatalf("render=%+v", snapshot.Render)
	}
}
