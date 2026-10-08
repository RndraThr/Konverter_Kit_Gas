package bast

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type scheduleSettingsStub struct {
	got         ScheduleSettings
	upsertInput ScheduleSettingsInput
	upsertErr   error
}

func (s *scheduleSettingsStub) GetScheduleSettings(_ context.Context, id string, _ auth.RegencyScope) (ScheduleSettings, error) {
	if id == "" {
		return ScheduleSettings{}, ErrNotFound
	}
	return ScheduleSettings{ScheduleID: id}, nil
}

func (s *scheduleSettingsStub) UpsertScheduleSettings(_ context.Context, _ auth.Principal, input ScheduleSettingsInput, _ auth.RegencyScope, _ auth.ClientMeta) (ScheduleSettings, error) {
	s.upsertInput = input
	if s.upsertErr != nil {
		return ScheduleSettings{}, s.upsertErr
	}
	return ScheduleSettings{ScheduleID: input.ScheduleID, HandoverLocation: input.HandoverLocation}, nil
}

func TestScheduleSettingsPutRejectsMissingSchedule(t *testing.T) {
	service := NewScheduleSettingsService(&scheduleSettingsStub{})
	_, err := service.Put(context.Background(), auth.Principal{}, ScheduleSettingsInput{}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestScheduleSettingsPutTrimsAndPersists(t *testing.T) {
	stub := &scheduleSettingsStub{}
	service := NewScheduleSettingsService(stub)
	result, err := service.Put(context.Background(), auth.Principal{}, ScheduleSettingsInput{
		ScheduleID: " sched-1 ", HandoverLocation: "  Lokasi Serah  ", ConsultantCompanyName: " konsultan jaya ", AgricultureOfficeName: " dinas pertanian ", AgricultureOfficeNIP: " 19800101 ", InstallerName: " budi ", SupervisorName: " andi ", PertaminaRepName: " siti ",
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduleID != "sched-1" || stub.upsertInput.HandoverLocation != "LOKASI SERAH" || stub.upsertInput.ConsultantCompanyName != "KONSULTAN JAYA" || stub.upsertInput.AgricultureOfficeName != "DINAS PERTANIAN" || stub.upsertInput.InstallerName != "BUDI" || stub.upsertInput.SupervisorName != "ANDI" || stub.upsertInput.PertaminaRepName != "SITI" || stub.upsertInput.AgricultureOfficeNIP != "19800101" {
		t.Fatalf("result=%+v input=%+v", result, stub.upsertInput)
	}
}

func TestScheduleSettingsGetPassesThrough(t *testing.T) {
	service := NewScheduleSettingsService(&scheduleSettingsStub{})
	result, err := service.Get(context.Background(), " sched-1 ", auth.RegencyScope{Unrestricted: true})
	if err != nil || result.ScheduleID != "sched-1" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestScheduleSettingsInputNormalizesRakordaDefaults(t *testing.T) {
	input := ScheduleSettingsInput{ScheduleID: " sched-1 ", RakordaLocation: "  aula kantor bupati  "}
	if err := input.normalize(); err != nil {
		t.Fatal(err)
	}
	if input.RakordaLocation != "AULA KANTOR BUPATI" {
		t.Fatalf("rakorda_location=%q", input.RakordaLocation)
	}
	if input.RakordaRowCount != 45 {
		t.Fatalf("rakorda_row_count=%d, want 45", input.RakordaRowCount)
	}
	if input.SosialisasiRowCount != 51 {
		t.Fatalf("sosialisasi_row_count=%d, want 51", input.SosialisasiRowCount)
	}
	if input.Training10RowCount != 51 {
		t.Fatalf("training_10_row_count=%d, want 51", input.Training10RowCount)
	}
}

func TestScheduleSettingsInputRejectsAttendanceRowCountOutsideRange(t *testing.T) {
	for _, rows := range []int{4, 201} {
		for _, input := range []ScheduleSettingsInput{
			{ScheduleID: "sched-1", RakordaRowCount: rows},
			{ScheduleID: "sched-1", SosialisasiRowCount: rows},
			{ScheduleID: "sched-1", Training10RowCount: rows},
			{ScheduleID: "sched-1", Training100RowCount: rows},
		} {
			if err := input.normalize(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("input=%+v err=%v", input, err)
			}
		}
	}
}
