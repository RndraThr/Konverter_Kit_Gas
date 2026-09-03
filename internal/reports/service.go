package reports

import (
	"context"
	"strings"
)

type repository interface {
	Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error)
	Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error)
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Summary{}, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return Summary{}, err
	}
	return s.repository.Summary(ctx, scheduleID, filter)
}

func (s *Service) Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return nil, err
	}
	return s.repository.Rows(ctx, scheduleID, filter)
}
