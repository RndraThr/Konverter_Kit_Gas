package distribution

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"konkit/internal/auth"
)

type mesinRepositoryStub struct {
	created   DistributionSlot
	createErr error
	seenInput CreateSlotInput
}

func (r *mesinRepositoryStub) CreateSlot(_ context.Context, _ auth.Principal, input CreateSlotInput, _ auth.RegencyScope, _ auth.ClientMeta) (DistributionSlot, error) {
	r.seenInput = input
	return r.created, r.createErr
}

func (r *mesinRepositoryStub) UpdateEquipment(_ context.Context, _ auth.Principal, _ UpdateEquipmentInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	return r.created, r.createErr
}

func TestCreateSlotRequiresScheduleID(t *testing.T) {
	service := &Service{posMesinRepository: &mesinRepositoryStub{}}
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateSlotDoesNotAcceptOrRequireDistributionDate(t *testing.T) {
	repository := &mesinRepositoryStub{created: DistributionSlot{ID: "slot-1", SlotNumber: 7, Status: "open"}}
	service := &Service{posMesinRepository: repository}
	result, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{ScheduleID: " schedule-1 ", SlotNumber: 7}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "slot-1" || repository.seenInput.ScheduleID != "schedule-1" || repository.seenInput.SlotNumber != 7 {
		t.Fatalf("result=%+v input=%+v", result, repository.seenInput)
	}
	if _, exists := reflect.TypeOf(CreateSlotInput{}).FieldByName("DistributionDate"); exists {
		t.Fatal("CreateSlotInput must not accept distribution_date at POS Mesin")
	}
}

func TestCreateSlotDelegatesExplicitSlotNumber(t *testing.T) {
	repo := &mesinRepositoryStub{created: DistributionSlot{ID: "slot-1", SlotNumber: 1, Status: "open"}}
	service := &Service{posMesinRepository: repo}
	result, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", SlotNumber: 1,
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SlotNumber != 1 || result.Status != "open" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenInput.SlotNumber != 1 {
		t.Fatalf("slot number not delegated: %+v", repo.seenInput)
	}
}
