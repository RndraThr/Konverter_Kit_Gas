package finalreport

import (
	"context"
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
