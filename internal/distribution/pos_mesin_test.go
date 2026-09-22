package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type mesinRepositoryStub struct {
	created   DistributionSlot
	createErr error
	seenInput CreateSlotInput
}

func (r *mesinRepositoryStub) CreateSlot(_ context.Context, _ auth.Principal, input CreateSlotInput, _ auth.ClientMeta) (DistributionSlot, error) {
	r.seenInput = input
	return r.created, r.createErr
}

func TestCreateSlotRequiresScheduleID(t *testing.T) {
	service := &Service{posMesinRepository: &mesinRepositoryStub{}}
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateSlotTrimsEquipmentFieldsAndDelegates(t *testing.T) {
	repo := &mesinRepositoryStub{created: DistributionSlot{ID: "slot-1", SlotNumber: 1, Status: "open"}}
	service := &Service{posMesinRepository: repo}
	result, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", MachineSerialNumber: "  MS-001  ", HoseSerialNumber: " HS-001 ", ConverterSerialNumber: " CV-001 ",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SlotNumber != 1 || result.Status != "open" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenInput.MachineSerialNumber != "MS-001" || repo.seenInput.HoseSerialNumber != "HS-001" || repo.seenInput.ConverterSerialNumber != "CV-001" {
		t.Fatalf("seenInput not trimmed: %+v", repo.seenInput)
	}
}
