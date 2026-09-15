package recipients

import (
	"context"
	"regexp"
	"strings"

	"konkit/internal/auth"
)

var nikPattern = regexp.MustCompile(`^[0-9]{16}$`)

type repository interface {
	List(context.Context, Filter, auth.RegencyScope) (Page, error)
	Stats(context.Context, auth.RegencyScope) (Stats, error)
	Create(context.Context, auth.Principal, CreateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)
	Update(context.Context, auth.Principal, string, UpdateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)
	Cancel(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	Restore(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return s.repository.List(ctx, filter, scope)
}

func (s *Service) Stats(ctx context.Context, scope auth.RegencyScope) (Stats, error) {
	return s.repository.Stats(ctx, scope)
}

func validateIdentity(fullName, nik string) error {
	if strings.TrimSpace(fullName) == "" {
		return ErrFullNameRequired
	}
	if nik = strings.TrimSpace(nik); nik != "" && !nikPattern.MatchString(nik) {
		return ErrNIKInvalid
	}
	return nil
}

func (s *Service) Create(ctx context.Context, actor auth.Principal, input CreateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.NIK = strings.TrimSpace(input.NIK)
	if err := validateIdentity(input.FullName, input.NIK); err != nil {
		return Recipient{}, err
	}
	return s.repository.Create(ctx, actor, input, meta, scope)
}

func (s *Service) Update(ctx context.Context, actor auth.Principal, allocationID string, input UpdateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.NIK = strings.TrimSpace(input.NIK)
	if err := validateIdentity(input.FullName, input.NIK); err != nil {
		return Recipient{}, err
	}
	return s.repository.Update(ctx, actor, strings.TrimSpace(allocationID), input, meta, scope)
}

func (s *Service) Cancel(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	return s.repository.Cancel(ctx, actor, strings.TrimSpace(allocationID), meta, scope)
}

func (s *Service) Restore(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	return s.repository.Restore(ctx, actor, strings.TrimSpace(allocationID), meta, scope)
}
