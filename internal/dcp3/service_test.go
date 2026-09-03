package dcp3

import (
	"errors"
	"testing"

	"konkit/internal/programs"
)

func TestValidateMappingRequiresSequenceAndName(t *testing.T) {
	headers := []string{"No", "Nama", "NIK"}
	for _, mapping := range []Mapping{
		{FullName: "Nama"},
		{SourceSequence: "No"},
		{SourceSequence: "Tidak Ada", FullName: "Nama"},
	} {
		if err := ValidateMapping(programs.ProgramFarmer, headers, mapping); !errors.Is(err, ErrMappingInvalid) {
			t.Fatalf("mapping=%+v err=%v", mapping, err)
		}
	}
	if err := ValidateMapping(programs.ProgramFarmer, headers, Mapping{SourceSequence: "No", FullName: "Nama", NIK: "NIK"}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRowNormalizesFarmerIdentity(t *testing.T) {
	row := RawImportRow{SourceRowNumber: 7, Values: map[string]string{
		"No": " 12 ", "Nama": "  Siti Aminah ", "NIK": "7312-3456 7890 1234",
		"No Kartu Petani": " kp 01-22 ", "Alamat": "", "No HP": "+62 812-3456",
	}}
	normalized := NormalizeRow(programs.ProgramFarmer, row, Mapping{
		SourceSequence: "No", FullName: "Nama", NIK: "NIK", FarmerCardNumber: "No Kartu Petani",
		Address: "Alamat", PhoneNumber: "No HP",
	})
	if normalized.SourceSequenceNumber == nil || *normalized.SourceSequenceNumber != 12 {
		t.Fatalf("sequence=%v", normalized.SourceSequenceNumber)
	}
	if normalized.FullName != "Siti Aminah" || normalized.NIK != "7312345678901234" || normalized.SectorIdentifier != "KP0122" || normalized.PhoneNumber != "628123456" {
		t.Fatalf("normalized=%+v", normalized)
	}
	if normalized.ValidationStatus != RowWarning || len(normalized.ValidationMessages) == 0 {
		t.Fatalf("expected optional-field warning: %+v", normalized)
	}
}

func TestNormalizeRowUsesKUSUKAForFisherman(t *testing.T) {
	row := RawImportRow{SourceRowNumber: 2, Values: map[string]string{"No": "1", "Nama": "Hasan", "KUSUKA": " 31-kusuka/9 "}}
	normalized := NormalizeRow(programs.ProgramFisherman, row, Mapping{SourceSequence: "No", FullName: "Nama", KUSUKANumber: "KUSUKA"})
	if normalized.IdentifierType != IdentifierKUSUKA || normalized.SectorIdentifier != "31KUSUKA9" {
		t.Fatalf("normalized=%+v", normalized)
	}
}

func TestNormalizeRowFlagsMalformedProvidedNIK(t *testing.T) {
	row := RawImportRow{SourceRowNumber: 2, Values: map[string]string{"No": "1", "Nama": "Hasan", "NIK": "123"}}
	normalized := NormalizeRow(programs.ProgramFarmer, row, Mapping{SourceSequence: "No", FullName: "Nama", NIK: "NIK"})
	if normalized.ValidationStatus != RowNeedsReview {
		t.Fatalf("status=%s messages=%v", normalized.ValidationStatus, normalized.ValidationMessages)
	}
}
