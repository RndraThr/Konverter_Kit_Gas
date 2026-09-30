package bast

import (
	"errors"
	"testing"
)

func TestBuildSnapshotNormalizesFarmerDocument(t *testing.T) {
	source := SourceData{ProgramType: "farmer", DocumentNumber: "0001/50/KSM-KKT-WJO/XII/2024", LocalDate: "2024-12-10", Profile: ProfileSnapshot{VersionID: "profile-1", Title: "BAST", Logos: []LogoSnapshot{{AssetID: "b", SortOrder: 2}, {AssetID: "a", SortOrder: 1}}}, Recipient: RecipientSnapshot{FullName: "Siti Aminah", NIK: "7306014101900001", SectorIdentifier: "KP-01", Address: "Alamat panjang", PhoneNumber: "08123456789"}, Equipment: EquipmentSnapshot{MachineBrand: "SHARK", MachineType: "SPWP 80-30 / 3 inch", MachineSerial: "M-001", HoseBrand: "TRILIUNHOSE", HoseSpec: "6m/10m", ConverterBrand: "ERGAS"}, Components: []ComponentSnapshot{{Code: "lpg", Label: "Tabung LPG 3 Kg", Quantity: 1, Unit: "Tabung"}}, ExecutorName: "Muhamad Wildan M", SupervisorName: "Andi Amrullah"}
	snapshot, err := BuildSnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Profile.Logos[0].AssetID != "a" || !snapshot.Components[0].Checked {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.Signatures.ReceiverName != "Siti Aminah" || snapshot.Signatures.ExecutorName != "Muhamad Wildan M" || snapshot.Signatures.SupervisorName != "Andi Amrullah" {
		t.Fatalf("signatures=%+v", snapshot.Signatures)
	}
}

func TestBuildSnapshotRejectsMissingFarmerIdentityAndFisherman(t *testing.T) {
	base := SourceData{ProgramType: "farmer", DocumentNumber: "N", LocalDate: "2024-12-10", Profile: ProfileSnapshot{VersionID: "profile-1"}, Recipient: RecipientSnapshot{FullName: "Siti", NIK: "7306014101900001"}}
	if _, err := BuildSnapshot(base); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing farmer card err=%v", err)
	}
	base.ProgramType = "fisherman"
	base.Recipient.SectorIdentifier = "KUSUKA"
	if _, err := BuildSnapshot(base); !errors.Is(err, ErrTemplateUnavailable) {
		t.Fatalf("fisherman err=%v", err)
	}
}
