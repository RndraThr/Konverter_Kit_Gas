package distribution

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"konkit/internal/auth"
)

var onlyDigits = regexp.MustCompile(`^[0-9]+$`)
var stripNonDigits = regexp.MustCompile(`[^0-9]+`)

type repository interface {
	Search(context.Context, string, string, int) ([]SearchRecord, error)
	GetWorkspace(context.Context, string) (RecipientWorkspace, error)
	SaveDraft(context.Context, auth.Principal, string, DraftInput, auth.ClientMeta) (RecipientWorkspace, error)
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) Search(ctx context.Context, scheduleID, query string, limit int) ([]SearchResult, error) {
	scheduleID, query = strings.TrimSpace(scheduleID), strings.TrimSpace(query)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if query == "" {
		return nil, ErrQueryRequired
	}
	if len([]rune(query)) < 2 && !onlyDigits.MatchString(query) {
		return nil, ErrQueryTooShort
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	records, err := s.repository.Search(ctx, scheduleID, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(records))
	for _, record := range records {
		results = append(results, SearchResult{
			AllocationID: record.AllocationID, DistributionNumber: record.DistributionNumber,
			FullName: record.FullName, MaskedNIK: maskNIK(record.NIK), Location: record.Location,
			ProgramType: record.ProgramType, Eligibility: record.Eligibility,
			AllocationStatus: record.AllocationStatus, Documentation: record.Documentation,
		})
	}
	return results, nil
}

func (s *Service) GetWorkspace(ctx context.Context, allocationID string) (RecipientWorkspace, error) {
	if strings.TrimSpace(allocationID) == "" {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	return s.repository.GetWorkspace(ctx, strings.TrimSpace(allocationID))
}

func (s *Service) SaveDraft(ctx context.Context, actor auth.Principal, allocationID string, input DraftInput, meta auth.ClientMeta) (RecipientWorkspace, error) {
	current, err := s.GetWorkspace(ctx, allocationID)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	input.NIK = stripNonDigits.ReplaceAllString(input.NIK, "")
	input.Address = strings.TrimSpace(input.Address)
	input.Village = strings.TrimSpace(input.Village)
	input.District = strings.TrimSpace(input.District)
	input.PhoneNumber = stripNonDigits.ReplaceAllString(input.PhoneNumber, "")
	input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)
	input.IdentityChangeReason = strings.TrimSpace(input.IdentityChangeReason)
	if input.NIK != "" && len(input.NIK) != 16 {
		return RecipientWorkspace{}, ErrNIKInvalid
	}
	if input.NIK != current.NIK && input.IdentityChangeReason == "" {
		return RecipientWorkspace{}, ErrIdentityChangeReasonRequired
	}
	return s.repository.SaveDraft(ctx, actor, strings.TrimSpace(allocationID), input, meta)
}

func maskNIK(value string) string {
	value = stripNonDigits.ReplaceAllString(value, "")
	if len(value) < 8 {
		return ""
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func normalizeIdentifier(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToUpper(character)
		}
		return -1
	}, value)
}
