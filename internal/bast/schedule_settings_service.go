package bast

import (
	"context"
	"strings"

	"konkit/internal/auth"
)

type scheduleSettingsRepository interface {
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	UpsertScheduleSettings(context.Context, auth.Principal, ScheduleSettingsInput, auth.RegencyScope, auth.ClientMeta) (ScheduleSettings, error)
}

// ScheduleSettingsService mengelola konfigurasi BA per jadwal.
type ScheduleSettingsService struct {
	repository scheduleSettingsRepository
}

func NewScheduleSettingsService(repository scheduleSettingsRepository) *ScheduleSettingsService {
	return &ScheduleSettingsService{repository: repository}
}

func (s *ScheduleSettingsService) Get(ctx context.Context, scheduleID string, scope auth.RegencyScope) (ScheduleSettings, error) {
	return s.repository.GetScheduleSettings(ctx, strings.TrimSpace(scheduleID), scope)
}

func (s *ScheduleSettingsService) Put(ctx context.Context, actor auth.Principal, input ScheduleSettingsInput, scope auth.RegencyScope, meta auth.ClientMeta) (ScheduleSettings, error) {
	if err := input.normalize(); err != nil {
		return ScheduleSettings{}, err
	}
	return s.repository.UpsertScheduleSettings(ctx, actor, input, scope, meta)
}
