package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type dokumenRepositoryStub struct {
	candidate     CandidateMatch
	candidateErr  error
	suggestions   []CandidateMatch
	seenSchedule  string
	seenPrefix    string
	seenLimit     int
	linked        DistributionSlot
	linkErr       error
	seenLink      LinkSlotInput
	seenUpdate    UpdateRecipientInput
	seenReplace   ReplaceRecipientInput
	seenEquipment UpdateEquipmentInput
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
func (r *dokumenRepositoryStub) UpdateRecipient(_ context.Context, _ auth.Principal, input UpdateRecipientInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seenUpdate = input
	return DistributionSlot{ID: "slot-1", Status: "linked"}, nil
}
func (r *dokumenRepositoryStub) ReplaceRecipient(_ context.Context, _ auth.Principal, input ReplaceRecipientInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seenReplace = input
	return DistributionSlot{ID: "slot-1", Status: "linked"}, nil
}
func (r *dokumenRepositoryStub) UpdateEquipment(_ context.Context, _ auth.Principal, input UpdateEquipmentInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seenEquipment = input
	return DistributionSlot{ID: "slot-1", Status: "linked"}, nil
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

func TestUpdateRecipientNormalizesEditableFields(t *testing.T) {
	repo := &dokumenRepositoryStub{}
	service := NewService(repo)
	_, err := service.UpdateRecipient(context.Background(), auth.Principal{}, UpdateRecipientInput{
		ScheduleID: " schedule-1 ", SlotNumber: 3, Address: " jl. tani ", Village: " desa baru ",
		District: " wajo ", PhoneNumber: "0812-3456", SectorIdentifier: " kp-01 ",
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if repo.seenUpdate.ScheduleID != "schedule-1" || repo.seenUpdate.Address != "JL. TANI" || repo.seenUpdate.Village != "DESA BARU" || repo.seenUpdate.District != "WAJO" || repo.seenUpdate.PhoneNumber != "08123456" || repo.seenUpdate.SectorIdentifier != "KP01" {
		t.Fatalf("input not normalized: %+v", repo.seenUpdate)
	}
}

func TestReplaceRecipientNormalizesNIKAndEditableFields(t *testing.T) {
	repo := &dokumenRepositoryStub{}
	service := NewService(repo)
	_, err := service.ReplaceRecipient(context.Background(), auth.Principal{}, ReplaceRecipientInput{
		ScheduleID: " schedule-1 ", SlotNumber: 3, NIK: "9171-0317-0701-0004", Address: " jl. tani ",
		Village: " desa baru ", District: " wajo ", PhoneNumber: "0812-3456", SectorIdentifier: " kp-01 ",
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if repo.seenReplace.NIK != "9171031707010004" || repo.seenReplace.Address != "JL. TANI" || repo.seenReplace.PhoneNumber != "08123456" || repo.seenReplace.SectorIdentifier != "KP01" {
		t.Fatalf("input not normalized: %+v", repo.seenReplace)
	}
}

func TestReplaceRecipientRejectsInvalidNIK(t *testing.T) {
	service := NewService(&dokumenRepositoryStub{})
	_, err := service.ReplaceRecipient(context.Background(), auth.Principal{}, ReplaceRecipientInput{ScheduleID: "schedule-1", SlotNumber: 1, NIK: "123"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("err=%v", err)
	}
}

type revisionRepositoryStub struct{ seen ReopenSlotInput }

func (r *revisionRepositoryStub) ReopenSlot(_ context.Context, _ auth.Principal, input ReopenSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seen = input
	return DistributionSlot{ID: "slot-1", Status: "linked", NeedsRecompletion: true}, nil
}

func TestReopenSlotRequiresReasonAndValidStage(t *testing.T) {
	service := NewService(&revisionRepositoryStub{})
	for _, input := range []ReopenSlotInput{
		{ScheduleID: "schedule-1", SlotNumber: 1, Stage: "dokumen", Reason: "  "},
		{ScheduleID: "schedule-1", SlotNumber: 1, Stage: "invalid", Reason: "Koreksi"},
	} {
		_, err := service.ReopenSlot(context.Background(), auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{})
		if !errors.Is(err, ErrRevisionReasonRequired) && !errors.Is(err, ErrRevisionStageInvalid) {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
}

func TestReopenSlotTrimsReasonAndDelegates(t *testing.T) {
	repo := &revisionRepositoryStub{}
	service := NewService(repo)
	result, err := service.ReopenSlot(context.Background(), auth.Principal{}, ReopenSlotInput{ScheduleID: " schedule-1 ", SlotNumber: 1, Stage: "dokumen", Reason: "  Koreksi nomor seri  "}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NeedsRecompletion || repo.seen.ScheduleID != "schedule-1" || repo.seen.Reason != "Koreksi nomor seri" {
		t.Fatalf("result=%+v input=%+v", result, repo.seen)
	}
}
