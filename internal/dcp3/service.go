package dcp3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

var nonDigit = regexp.MustCompile(`[^0-9]+`)

type importRepository interface {
	CreatePreview(context.Context, auth.Principal, string, string, string, WorkbookPreview, auth.ClientMeta, auth.RegencyScope) (ImportPreview, error)
	GetPreview(context.Context, string, auth.RegencyScope) (ImportPreview, error)
	Commit(context.Context, auth.Principal, string, Mapping, auth.ClientMeta) (ImportResult, error)
}

type ImportService struct {
	repository importRepository
	limits     ParseLimits
}

func NewImportService(repository importRepository, limits ParseLimits) *ImportService {
	return &ImportService{repository: repository, limits: limits}
}

func (s *ImportService) Preview(ctx context.Context, actor auth.Principal, scheduleID, filename string, source io.Reader, meta auth.ClientMeta, scope auth.RegencyScope, headerRow int) (ImportPreview, error) {
	data, err := io.ReadAll(io.LimitReader(source, s.limits.MaxBytes+1))
	if err != nil {
		return ImportPreview{}, err
	}
	if int64(len(data)) > s.limits.MaxBytes {
		return ImportPreview{}, ErrWorkbookTooLarge
	}
	preview, err := ParseWorkbook(bytes.NewReader(data), s.limits, headerRow-1)
	if err != nil {
		return ImportPreview{}, err
	}
	digest := sha256.Sum256(data)
	return s.repository.CreatePreview(ctx, actor, strings.TrimSpace(scheduleID), strings.TrimSpace(filename), hex.EncodeToString(digest[:]), preview, meta, scope)
}

func (s *ImportService) RawPreview(ctx context.Context, source io.Reader) ([][]string, error) {
	return RawRows(source, s.limits, 10)
}

func (s *ImportService) GetPreview(ctx context.Context, id string, scope auth.RegencyScope) (ImportPreview, error) {
	return s.repository.GetPreview(ctx, strings.TrimSpace(id), scope)
}

func (s *ImportService) Commit(ctx context.Context, actor auth.Principal, batchID string, mapping Mapping, meta auth.ClientMeta, scope auth.RegencyScope) (ImportResult, error) {
	preview, err := s.repository.GetPreview(ctx, strings.TrimSpace(batchID), scope)
	if err != nil {
		return ImportResult{}, err
	}
	if preview.Status != "draft" {
		return ImportResult{}, ErrImportState
	}
	if err := ValidateMapping(preview.ProgramType, preview.Headers, mapping); err != nil {
		return ImportResult{}, err
	}
	return s.repository.Commit(ctx, actor, preview.ID, mapping, meta)
}

func ValidateMapping(programType programs.ProgramType, headers []string, mapping Mapping) error {
	if programType != programs.ProgramFarmer && programType != programs.ProgramFisherman {
		return ErrMappingInvalid
	}
	available := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		available[header] = struct{}{}
	}
	if mapping.SourceSequence == "" || mapping.FullName == "" {
		return ErrMappingInvalid
	}
	for _, selected := range []string{mapping.SourceSequence, mapping.FullName, mapping.NIK, mapping.FarmerCardNumber, mapping.KUSUKANumber, mapping.Address, mapping.Village, mapping.District, mapping.PhoneNumber} {
		if selected == "" {
			continue
		}
		if _, exists := available[selected]; !exists {
			return ErrMappingInvalid
		}
	}
	return nil
}

func NormalizeRow(programType programs.ProgramType, row RawImportRow, mapping Mapping) NormalizedRow {
	result := NormalizedRow{
		SourceRowNumber:    row.SourceRowNumber,
		FullName:           strings.TrimSpace(row.Values[mapping.FullName]),
		Address:            strings.TrimSpace(row.Values[mapping.Address]),
		Village:            strings.TrimSpace(row.Values[mapping.Village]),
		District:           strings.TrimSpace(row.Values[mapping.District]),
		PhoneNumber:        digits(row.Values[mapping.PhoneNumber]),
		ValidationStatus:   RowValid,
		ValidationMessages: []string{},
		SourceValues:       row.Values,
	}
	sequence, err := strconv.Atoi(strings.TrimSpace(row.Values[mapping.SourceSequence]))
	if err != nil || sequence <= 0 {
		result.raise(RowNeedsReview, "Nomor urut sumber tidak valid")
	} else {
		result.SourceSequenceNumber = &sequence
	}
	if result.FullName == "" {
		result.raise(RowInvalid, "Nama penerima wajib diisi")
	}
	rawNIK := strings.TrimSpace(row.Values[mapping.NIK])
	result.NIK = digits(rawNIK)
	if rawNIK == "" {
		result.raise(RowWarning, "NIK belum tersedia")
	} else if len(result.NIK) != 16 {
		result.raise(RowNeedsReview, "NIK harus terdiri dari 16 digit")
	}
	identifierColumn := mapping.FarmerCardNumber
	result.IdentifierType = IdentifierFarmerCard
	if programType == programs.ProgramFisherman {
		identifierColumn = mapping.KUSUKANumber
		result.IdentifierType = IdentifierKUSUKA
	}
	result.SectorIdentifierDisplay = strings.TrimSpace(row.Values[identifierColumn])
	result.SectorIdentifier = normalizeIdentifier(result.SectorIdentifierDisplay)
	if result.SectorIdentifier == "" {
		result.raise(RowWarning, "Nomor kartu sektor belum tersedia")
	}
	if result.Address == "" {
		result.raise(RowWarning, "Alamat belum tersedia")
	}
	if result.PhoneNumber == "" {
		result.raise(RowWarning, "Nomor telepon belum tersedia")
	}
	return result
}

func (row *NormalizedRow) raise(status RowStatus, message string) {
	if rowStatusRank(status) > rowStatusRank(row.ValidationStatus) {
		row.ValidationStatus = status
	}
	row.ValidationMessages = append(row.ValidationMessages, message)
}

func rowStatusRank(status RowStatus) int {
	switch status {
	case RowInvalid:
		return 4
	case RowNeedsReview:
		return 3
	case RowWarning:
		return 2
	case RowValid:
		return 1
	default:
		return 0
	}
}

func digits(value string) string { return nonDigit.ReplaceAllString(value, "") }

func normalizeIdentifier(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToUpper(character)
		}
		return -1
	}, value)
}
