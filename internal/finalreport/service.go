package finalreport

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

type programContextResolver interface {
	ResolveStorageContext(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (programs.StorageContext, error)
}

type Service struct {
	repo            *Repository
	programContexts programContextResolver
}

func NewService(repo *Repository, programContexts programContextResolver) *Service {
	return &Service{repo: repo, programContexts: programContexts}
}

// NewServiceForTest builds a Service with a nil repository, for tests that
// only exercise input validation (which short-circuits before any
// repository call).
func NewServiceForTest(repo *Repository, programContexts programContextResolver) *Service {
	return &Service{repo: repo, programContexts: programContexts}
}

func (s *Service) GetSettings(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Settings, error) {
	programID, regencyID = strings.TrimSpace(programID), strings.TrimSpace(regencyID)
	if programID == "" {
		return Settings{}, ErrProgramRequired
	}
	if regencyID == "" {
		return Settings{}, ErrRegencyRequired
	}
	return s.repo.GetSettings(ctx, programID, regencyID, scope)
}

func (s *Service) PutSettings(ctx context.Context, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error) {
	input.ProgramID, input.RegencyID = strings.TrimSpace(input.ProgramID), strings.TrimSpace(input.RegencyID)
	if input.ProgramID == "" {
		return Settings{}, ErrProgramRequired
	}
	if input.RegencyID == "" {
		return Settings{}, ErrRegencyRequired
	}
	if input.LeaderCount < 0 || input.AdminCount < 0 || input.TechnicianCount < 0 || input.HelperCount < 0 {
		return Settings{}, ErrCountNegative
	}
	input.MachineManufacturerName = strings.TrimSpace(input.MachineManufacturerName)
	return s.repo.PutSettings(ctx, actor, input, scope)
}

func (s *Service) ListPhotoPicks(ctx context.Context, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	return s.repo.ListPhotoPicks(ctx, programID, regencyID, slideKey, scope)
}

func (s *Service) ReplacePhotoPicks(ctx context.Context, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	if len(activityMediaIDs) > 6 {
		return nil, ErrTooManyPicks
	}
	return s.repo.ReplacePhotoPicks(ctx, programID, regencyID, slideKey, activityMediaIDs, scope)
}

func (s *Service) ListDocuments(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]Document, error) {
	return s.repo.ListDocuments(ctx, programID, regencyID, scope)
}

func (s *Service) Preview(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Preview, error) {
	data, err := s.repo.GetPreviewSourceData(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}
	settings, err := s.repo.GetSettings(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}

	picks := map[string][]PhotoPick{}
	for _, key := range []string{SlideKeyPengirimanPenerimaan, SlideKeyRapatKoordinasi, SlideKeySosialisasi} {
		list, err := s.repo.ListPhotoPicks(ctx, programID, regencyID, key, scope)
		if err != nil {
			return Preview{}, err
		}
		picks[key] = list
	}

	deliveryStart, deliveryEnd := earliestLatest(settings.DeliveryHoseDate, settings.DeliveryMachineGroupDate, settings.DeliveryGasCylinderDate)

	return Preview{
		RegencyLabel:             orBlank(strings.ToUpper("Kabupaten " + data.regencyName)),
		FiscalYear:               orBlank(itoaOrEmpty(data.fiscalYear)),
		AgricultureOfficeName:    orBlank(data.agricultureOfficeName),
		ConsultantCompanyName:    orBlank(data.consultantCompanyName),
		MachineManufacturerName:  orBlank(settings.MachineManufacturerName),
		DeliveryHoseDate:         orBlank(settings.DeliveryHoseDate),
		DeliveryMachineGroupDate: orBlank(settings.DeliveryMachineGroupDate),
		DeliveryGasCylinderDate:  orBlank(settings.DeliveryGasCylinderDate),
		DeliveryRangeStart:       orBlank(deliveryStart),
		DeliveryRangeEnd:         orBlank(deliveryEnd),
		RakordaDate:              orBlank(data.rakordaDate),
		SosialisasiDate:          orBlank(data.sosialisasiDate),
		LeaderCount:              itoaOrEmpty(settings.LeaderCount),
		AdminCount:               itoaOrEmpty(settings.AdminCount),
		TechnicianCount:          itoaOrEmpty(settings.TechnicianCount),
		HelperCount:              itoaOrEmpty(settings.HelperCount),
		HandoverLocation:         orBlank(data.handoverLocation),
		DistributionStartDate:    orBlank(data.distributionStart),
		DistributionEndDate:      orBlank(data.distributionEnd),
		DistributionTotal:        orBlank(itoaOrEmptyNonZero(data.distributionTotal)),
		PhotoPicks:               picks,
	}, nil
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return "0"
	}
	return strconv.Itoa(n)
}

func itoaOrEmptyNonZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// earliestLatest returns the min/max of the non-empty dates among the 3
// delivery-date settings fields, formatted the same way the SQL-sourced
// dates are ("DD Month YYYY" is not produced here — these are already
// plain ISO strings from Settings, displayed as-is).
func earliestLatest(dates ...string) (string, string) {
	var nonEmpty []string
	for _, d := range dates {
		if d != "" {
			nonEmpty = append(nonEmpty, d)
		}
	}
	if len(nonEmpty) == 0 {
		return "", ""
	}
	sort.Strings(nonEmpty)
	return nonEmpty[0], nonEmpty[len(nonEmpty)-1]
}
