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

func TestCreateSlotRequiresValidDistributionDate(t *testing.T) {
	service := &Service{posMesinRepository: &mesinRepositoryStub{}}
	for _, date := range []string{"", "20-10-2026"} {
		_, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{ScheduleID: "schedule-1", DistributionDate: date}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
		if !errors.Is(err, ErrDistributionDateRequired) {
			t.Fatalf("date %q err = %v", date, err)
		}
	}
}

func TestCreateSlotIgnoresLegacyEquipmentFieldsAndDelegates(t *testing.T) {
	repo := &mesinRepositoryStub{created: DistributionSlot{ID: "slot-1", SlotNumber: 1, Status: "open"}}
	service := &Service{posMesinRepository: repo}
	result, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", DistributionDate: "2026-10-20", MachineSerialNumber: "  MS-001  ", HoseSerialNumber: " HS-001 ", ConverterSerialNumber: " CV-001 ",
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SlotNumber != 1 || result.Status != "open" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenInput.MachineOptionCode != "" || repo.seenInput.MachineSerialNumber != "" || repo.seenInput.HoseOptionCode != "" || repo.seenInput.HoseSerialNumber != "" || repo.seenInput.ConverterOptionCode != "" || repo.seenInput.ConverterSerialNumber != "" {
		t.Fatalf("legacy equipment leaked through create: %+v", repo.seenInput)
	}
}
