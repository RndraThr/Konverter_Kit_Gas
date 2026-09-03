package reports

import (
	"context"
	"errors"
	"testing"
)

type repositoryStub struct {
	summary    Summary
	rows       []Row
	scheduleID string
	filter     Filter
}

func (r *repositoryStub) Summary(_ context.Context, scheduleID string, filter Filter) (Summary, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.summary, nil
}

func (r *repositoryStub) Rows(_ context.Context, scheduleID string, filter Filter) ([]Row, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.rows, nil
}

func TestSummaryRequiresScheduleAndValidatesFilter(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Summary(context.Background(), "", Filter{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{AllocationStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid allocation status err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{DistributionStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid distribution status err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{DocumentationStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid documentation status err=%v", err)
	}
}

func TestRowsRequiresScheduleAndTrimsInput(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Rows(context.Background(), "   ", Filter{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
}

func TestRowsPassesTrimmedScheduleAndFilterToRepository(t *testing.T) {
	repository := &repositoryStub{rows: []Row{{DistributionNumber: 7, FullName: "Siti Aminah"}}}
	service := NewService(repository)

	rows, err := service.Rows(context.Background(), " schedule-1 ", Filter{AllocationStatus: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleID != "schedule-1" || repository.filter.AllocationStatus != "ready" {
		t.Fatalf("schedule=%q filter=%+v", repository.scheduleID, repository.filter)
	}
	if len(rows) != 1 || rows[0].FullName != "Siti Aminah" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestSummaryPassesTrimmedScheduleAndFilterToRepository(t *testing.T) {
	repository := &repositoryStub{summary: Summary{TotalAllocations: 3}}
	service := NewService(repository)

	summary, err := service.Summary(context.Background(), " schedule-1 ", Filter{DocumentationStatus: "incomplete"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleID != "schedule-1" || repository.filter.DocumentationStatus != "incomplete" {
		t.Fatalf("schedule=%q filter=%+v", repository.scheduleID, repository.filter)
	}
	if summary.TotalAllocations != 3 {
		t.Fatalf("summary=%+v", summary)
	}
}
