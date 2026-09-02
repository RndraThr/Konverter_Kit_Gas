package settings

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"konkit/internal/auth"
)

type repository interface {
	List(context.Context) ([]Setting, error)
	Update(context.Context, auth.Principal, map[string]string, auth.ClientMeta) ([]Setting, error)
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]Setting, error) {
	return s.repository.List(ctx)
}

func (s *Service) Update(ctx context.Context, actor auth.Principal, values map[string]string, meta auth.ClientMeta) ([]Setting, error) {
	if len(values) == 0 {
		return nil, ErrInvalidSetting
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		definition, exists := Definitions[key]
		if !exists {
			return nil, ErrInvalidSetting
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, ErrInvalidSetting
		}
		if definition.MaxLength > 0 && utf8.RuneCountInString(value) > definition.MaxLength {
			return nil, ErrInvalidSetting
		}
		if len(definition.Allowed) > 0 && !slices.Contains(definition.Allowed, value) {
			return nil, ErrInvalidSetting
		}
		normalized[key] = value
	}
	return s.repository.Update(ctx, actor, normalized, meta)
}
