package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type dokumenRepositoryStub struct {
	candidate    CandidateMatch
	candidateErr error
	suggestions  []CandidateMatch
	seenSchedule string
	seenPrefix   string
	seenLimit    int
	linked       DistributionSlot
	linkErr      error
	seenLink     LinkSlotInput
}

func (r *dokumenRepositoryStub) SearchCandidate(_ context.Context, _, _ string, _ auth.RegencyScope) (CandidateMatch, error) {
	return r.candidate, r.candidateErr
}
func (r *dokumenRepositoryStub) SuggestCandidates(_ context.Context, scheduleID, nikPrefix string, limit int, _ auth.RegencyScope) ([]CandidateMatch, error) {
	r.seenSchedule, r.seenPrefix, r.seenLimit = scheduleID, nikPrefix, limit
	return r.suggestions, nil
}
func (r *dokumenRepositoryStub) LinkSlot(_ context.Context, _ auth.Principal, input LinkSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seenLink = input
	return r.linked, r.linkErr
}

func TestLinkSlotRequiresValidNIK(t *testing.T) {
	service := &Service{posDokumenRepository: &dokumenRepositoryStub{}}
	_, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{ScheduleID: "s1", SlotNumber: 1, NIK: "123"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestSuggestCandidatesNormalizesPrefixAndLimitsResults(t *testing.T) {
	repo := &dokumenRepositoryStub{suggestions: []CandidateMatch{{NIK: "7306014101900001", FullName: "SITI AMINAH"}}}
	service := &Service{posDokumenRepository: repo}

	items, err := service.SuggestCandidates(context.Background(), " schedule-1 ", "73-06", auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].FullName != "SITI AMINAH" {
		t.Fatalf("items = %+v", items)
	}
	if repo.seenSchedule != "schedule-1" || repo.seenPrefix != "7306" || repo.seenLimit != 8 {
		t.Fatalf("search = schedule:%q prefix:%q limit:%d", repo.seenSchedule, repo.seenPrefix, repo.seenLimit)
	}
}

func TestLinkSlotRequiresSlotNumber(t *testing.T) {
	service := &Service{posDokumenRepository: &dokumenRepositoryStub{}}
	_, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{ScheduleID: "s1", NIK: "1234567890123456"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSlotNumberRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkSlotDelegatesValidInput(t *testing.T) {
	repo := &dokumenRepositoryStub{linked: DistributionSlot{ID: "slot-1", SlotNumber: 5, Status: "linked"}}
	service := &Service{posDokumenRepository: repo}
	result, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{
		ScheduleID: "s1", SlotNumber: 5, NIK: "1234567890123456", Address: "  Jl. A  ",
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "linked" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenLink.Address != "JL. A" {
		t.Fatalf("address not uppercased: %q", repo.seenLink.Address)
	}
}
