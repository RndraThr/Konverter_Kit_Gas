package dcp3

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

type importRepositoryStub struct {
	preview       ImportPreview
	result        ImportResult
	seenScope     auth.RegencyScope
	getPreviewErr error
}

func (r *importRepositoryStub) CreatePreview(_ context.Context, _ auth.Principal, _, _, _ string, _ WorkbookPreview, _ auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error) {
	r.seenScope = scope
	return r.preview, nil
}
func (r *importRepositoryStub) GetPreview(_ context.Context, _ string, scope auth.RegencyScope) (ImportPreview, error) {
	r.seenScope = scope
	if r.getPreviewErr != nil {
		return ImportPreview{}, r.getPreviewErr
	}
	return r.preview, nil
}
func (r *importRepositoryStub) Commit(_ context.Context, _ auth.Principal, batchID string, _ Mapping, _ auth.ClientMeta) (ImportResult, error) {
	return r.result, nil
}

func TestPreviewGetPreviewAndCommitForwardRegencyScope(t *testing.T) {
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	repository := &importRepositoryStub{}

	repository.preview = ImportPreview{ID: "batch-1", Status: "draft", ProgramType: programs.ProgramFarmer, Headers: []string{"No", "Nama"}}
	if _, err := repository.CreatePreview(context.Background(), auth.Principal{}, "schedule-1", "file.xlsx", "checksum", WorkbookPreview{}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("CreatePreview scope=%+v", repository.seenScope)
	}

	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	repository.seenScope = auth.RegencyScope{}
	if _, err := service.GetPreview(context.Background(), "batch-1", scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("GetPreview scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.Commit(context.Background(), auth.Principal{}, "batch-1", Mapping{SourceSequence: "No", FullName: "Nama"}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("Commit did not forward scope through its internal GetPreview call: %+v", repository.seenScope)
	}
}

func TestGetPreviewReturnsRepositoryErrorForOutOfScopeBatch(t *testing.T) {
	repository := &importRepositoryStub{getPreviewErr: ErrPreviewNotFound}
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	if _, err := service.GetPreview(context.Background(), "batch-1", auth.RegencyScope{}); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("err=%v", err)
	}
}

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
