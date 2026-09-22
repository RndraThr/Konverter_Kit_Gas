package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type penyerahanRepositoryStub struct {
	found       DistributionSlot
	searchErr   error
	completed   DistributionSlot
	completeErr error
}

func (r *penyerahanRepositoryStub) SearchLinkedSlot(_ context.Context, _, _ string, _ auth.RegencyScope) (DistributionSlot, error) {
	return r.found, r.searchErr
}
func (r *penyerahanRepositoryStub) CompleteSlot(_ context.Context, _ auth.Principal, _ CompleteSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	return r.completed, r.completeErr
}

func TestCompleteSlotRequiresSlotNumber(t *testing.T) {
	service := &Service{posPenyerahanRepository: &penyerahanRepositoryStub{}}
	_, err := service.CompleteSlot(context.Background(), auth.Principal{}, CompleteSlotInput{ScheduleID: "s1"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSlotNumberRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompleteSlotDelegatesValidInput(t *testing.T) {
	repo := &penyerahanRepositoryStub{completed: DistributionSlot{ID: "slot-1", Status: "completed"}}
	service := &Service{posPenyerahanRepository: repo}
	result, err := service.CompleteSlot(context.Background(), auth.Principal{}, CompleteSlotInput{ScheduleID: "s1", SlotNumber: 1}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
}
