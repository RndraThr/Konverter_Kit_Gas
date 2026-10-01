package bast

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"konkit/internal/auth"
)

type repository interface {
	GetSourceContext(context.Context, string, string, auth.RegencyScope) (SourceContext, error)
	ListCompletedSlots(context.Context, string, string, auth.RegencyScope) ([]CompletedSlot, error)
	GetLockedTotal(context.Context, string, string) (LockResult, error)
	LockRegencyTotal(context.Context, auth.Principal, SourceContext, auth.ClientMeta) (LockResult, error)
	ListActiveBundles(context.Context, string, string, auth.RegencyScope) ([]DailyBundle, error)
	LoadSourceData(context.Context, RecipientDocument, SourceContext, auth.RegencyScope) (SourceData, error)
	SaveFinalDocument(context.Context, auth.Principal, RecipientDocument, SourceData, Snapshot, auth.ClientMeta) (IndividualDocument, error)
}

func (s *Service) FinalizeRecipient(ctx context.Context, actor auth.Principal, programID, regencyID string, recipient RecipientDocument, scope auth.RegencyScope, meta auth.ClientMeta) (IndividualDocument, error) {
	contextData, err := s.repository.GetSourceContext(ctx, programID, regencyID, scope)
	if err != nil {
		return IndividualDocument{}, err
	}
	if err := validateContext(contextData); err != nil {
		return IndividualDocument{}, err
	}
	source, err := s.repository.LoadSourceData(ctx, recipient, contextData, scope)
	if err != nil {
		return IndividualDocument{}, err
	}
	snapshot, err := BuildSnapshot(source)
	if err != nil {
		return IndividualDocument{}, err
	}
	return s.repository.SaveFinalDocument(ctx, actor, recipient, source, snapshot, meta)
}

type Service struct {
	repository repository
	location   *time.Location
}

func NewService(repository repository, location *time.Location) *Service {
	if location == nil {
		location = time.UTC
	}
	return &Service{repository: repository, location: location}
}

func BuildDocumentNumber(input NumberInput) (string, error) {
	if input.SlotNumber < 1 || input.FinalTotal < 1 || input.SlotNumber > input.FinalTotal || input.Padding < 1 || strings.TrimSpace(input.DocumentSeries) == "" || strings.TrimSpace(input.RegencyCode) == "" || input.LocalDate.IsZero() {
		return "", ErrInvalidInput
	}
	return fmt.Sprintf("%0*d/%d/%s-%s/%s/%d", input.Padding, input.SlotNumber, input.FinalTotal, strings.ToUpper(strings.TrimSpace(input.DocumentSeries)), strings.ToUpper(strings.TrimSpace(input.RegencyCode)), romanMonth(input.LocalDate.Month()), input.LocalDate.Year()), nil
}

func romanMonth(month time.Month) string {
	return [...]string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII"}[month]
}

func (s *Service) LockRegencyTotal(ctx context.Context, actor auth.Principal, programID, regencyID string, scope auth.RegencyScope, meta auth.ClientMeta) (LockResult, error) {
	contextData, err := s.repository.GetSourceContext(ctx, strings.TrimSpace(programID), strings.TrimSpace(regencyID), scope)
	if err != nil {
		return LockResult{}, err
	}
	if err := validateContext(contextData); err != nil {
		return LockResult{}, err
	}
	if contextData.SlotQuota < 1 {
		return LockResult{}, ErrFinalTotalMissing
	}
	slots, err := s.repository.ListCompletedSlots(ctx, programID, regencyID, scope)
	if err != nil {
		return LockResult{}, err
	}
	for _, slot := range slots {
		if slot.SlotNumber < 1 || slot.SlotNumber > contextData.SlotQuota {
			return LockResult{}, ErrSlotOutOfRange
		}
	}
	return s.repository.LockRegencyTotal(ctx, actor, contextData, meta)
}

func (s *Service) ListDates(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]DateSummary, error) {
	contextData, err := s.repository.GetSourceContext(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	contextErr := validateContext(contextData)
	lock, lockErr := s.repository.GetLockedTotal(ctx, programID, regencyID)
	slots, err := s.repository.ListCompletedSlots(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	bundles, err := s.repository.ListActiveBundles(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	bundleByDate := map[string]*DailyBundle{}
	for i := range bundles {
		bundleByDate[bundles[i].LocalDate] = &bundles[i]
	}
	counts := map[string]int{}
	for _, slot := range slots {
		counts[slot.DistributedAt.In(s.location).Format("2006-01-02")]++
	}
	dates := make([]string, 0, len(counts))
	for date := range counts {
		dates = append(dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	result := make([]DateSummary, 0, len(dates))
	for _, date := range dates {
		status := "ready"
		if contextErr != nil {
			status = "configuration_required"
		} else if lockErr != nil || lock.FinalTotal < 1 {
			status = "total_not_locked"
		}
		result = append(result, DateSummary{LocalDate: date, RecipientCount: counts[date], ValidationStatus: status, Bundle: bundleByDate[date]})
	}
	return result, nil
}

func (s *Service) ListRecipients(ctx context.Context, programID, regencyID, localDate string, scope auth.RegencyScope) ([]RecipientDocument, error) {
	wantedDate, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return nil, ErrInvalidInput
	}
	_ = wantedDate
	contextData, err := s.repository.GetSourceContext(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	if err := validateContext(contextData); err != nil {
		return nil, err
	}
	lock, err := s.repository.GetLockedTotal(ctx, programID, regencyID)
	if err != nil {
		return nil, ErrFinalTotalMissing
	}
	slots, err := s.repository.ListCompletedSlots(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].SlotNumber < slots[j].SlotNumber })
	items := []RecipientDocument{}
	for _, slot := range slots {
		date := slot.DistributedAt.In(s.location)
		if date.Format("2006-01-02") != localDate {
			continue
		}
		number, err := BuildDocumentNumber(NumberInput{SlotNumber: slot.SlotNumber, FinalTotal: lock.FinalTotal, Padding: contextData.Padding, DocumentSeries: contextData.DocumentSeries, RegencyCode: contextData.RegencyCode, LocalDate: date})
		if err != nil {
			return nil, ErrSlotOutOfRange
		}
		items = append(items, RecipientDocument{DistributionSlotID: slot.ID, SlotNumber: slot.SlotNumber, FinalTotal: lock.FinalTotal, Padding: contextData.Padding, DocumentNumber: number, LocalDate: localDate})
	}
	return items, nil
}

func validateContext(value SourceContext) error {
	if value.ProgramType != "farmer" {
		return ErrTemplateUnavailable
	}
	if value.ZonePlaceholder || strings.TrimSpace(value.ZoneName) == "" {
		return ErrZoneNotConfigured
	}
	if !value.HasActiveLogo {
		return ErrBrandingNotConfigured
	}
	return nil
}
