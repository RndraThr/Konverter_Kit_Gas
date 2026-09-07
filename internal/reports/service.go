package reports

import (
	"context"
	"strings"

	"konkit/internal/auth"
)

type repository interface {
	Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error)
	Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error)
	RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, filter Filter, meta auth.ClientMeta) error
	ScheduleRegency(ctx context.Context, scheduleID string) (string, error)
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) checkScheduleScope(ctx context.Context, scheduleID string, scope auth.RegencyScope) error {
	regencyID, err := s.repository.ScheduleRegency(ctx, scheduleID)
	if err != nil {
		return err
	}
	if !scope.Allows(regencyID) {
		return ErrScheduleNotFound
	}
	return nil
}

func (s *Service) Summary(ctx context.Context, scheduleID string, filter Filter, scope auth.RegencyScope) (Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Summary{}, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return Summary{}, err
	}
	if err := s.checkScheduleScope(ctx, scheduleID, scope); err != nil {
		return Summary{}, err
	}
	return s.repository.Summary(ctx, scheduleID, filter)
}

func (s *Service) Rows(ctx context.Context, scheduleID string, filter Filter, scope auth.RegencyScope) ([]Row, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return nil, err
	}
	if err := s.checkScheduleScope(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	return s.repository.Rows(ctx, scheduleID, filter)
}

func (s *Service) ExportExcel(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	rows, err := s.Rows(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	data, err := buildExcel(rows)
	if err != nil {
		return nil, err
	}
	if err := s.repository.RecordExport(ctx, actor, strings.TrimSpace(scheduleID), "xlsx", filter, meta); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) ExportPDF(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	summary, err := s.Summary(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	rows, err := s.Rows(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	data, err := buildPDF(summary, rows)
	if err != nil {
		return nil, err
	}
	if err := s.repository.RecordExport(ctx, actor, strings.TrimSpace(scheduleID), "pdf", filter, meta); err != nil {
		return nil, err
	}
	return data, nil
}
