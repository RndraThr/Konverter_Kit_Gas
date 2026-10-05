package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type distributionDateRepositoryStub struct {
	seen SetDistributionDateInput
}

func (r *distributionDateRepositoryStub) SetDistributionDate(_ context.Context, _ auth.Principal, input SetDistributionDateInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seen = input
	return DistributionSlot{ID: "slot-1", DistributionDate: input.DistributionDate}, nil
}

func TestSetDistributionDateValidatesAndDelegates(t *testing.T) {
	repository := &distributionDateRepositoryStub{}
	service := NewService(repository)

	if _, err := service.SetDistributionDate(context.Background(), auth.Principal{}, SetDistributionDateInput{ScheduleID: "schedule-1", SlotNumber: 3}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrDistributionDateRequired) {
		t.Fatalf("missing date err=%v", err)
	}

	result, err := service.SetDistributionDate(context.Background(), auth.Principal{}, SetDistributionDateInput{ScheduleID: " schedule-1 ", SlotNumber: 3, DistributionDate: "2026-10-20"}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.DistributionDate != "2026-10-20" || repository.seen.ScheduleID != "schedule-1" {
		t.Fatalf("result=%+v seen=%+v", result, repository.seen)
	}
}
