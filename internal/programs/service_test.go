package programs

import (
	"context"
	"errors"
	"testing"
	"time"

	"konkit/internal/auth"
)

type repositoryStub struct {
	regencyInput  RegencyInput
	programInput  ProgramInput
	scheduleInput ScheduleInput
	actor         auth.Principal
	meta          auth.ClientMeta
}

func (r *repositoryStub) ListRegencies(context.Context) ([]Regency, error) { return nil, nil }
func (r *repositoryStub) SaveRegency(_ context.Context, actor auth.Principal, input RegencyInput, meta auth.ClientMeta) (Regency, error) {
	r.regencyInput = input
	r.actor = actor
	r.meta = meta
	return Regency{DocumentCode: input.DocumentCode}, nil
}
func (r *repositoryStub) ListPrograms(context.Context) ([]Program, error) { return nil, nil }
func (r *repositoryStub) SaveProgram(_ context.Context, _ auth.Principal, input ProgramInput, _ auth.ClientMeta) (Program, error) {
	r.programInput = input
	return Program{ProgramType: input.ProgramType}, nil
}
func (r *repositoryStub) ListSchedules(context.Context) ([]Schedule, error) { return nil, nil }
func (r *repositoryStub) SaveSchedule(_ context.Context, _ auth.Principal, input ScheduleInput, _ auth.ClientMeta) (Schedule, error) {
	r.scheduleInput = input
	return Schedule{DistributionNumberPadding: input.DistributionNumberPadding}, nil
}
func (r *repositoryStub) ListPackageTemplates(context.Context) ([]PackageTemplate, error) {
	return nil, nil
}
func (r *repositoryStub) SavePackageTemplate(context.Context, auth.Principal, PackageTemplateInput, auth.ClientMeta) (PackageTemplate, error) {
	return PackageTemplate{}, nil
}
func (r *repositoryStub) ListDocumentationTemplates(context.Context) ([]DocumentationTemplate, error) {
	return nil, nil
}
func (r *repositoryStub) SaveDocumentationTemplate(context.Context, auth.Principal, DocumentationTemplateInput, auth.ClientMeta) (DocumentationTemplate, error) {
	return DocumentationTemplate{}, nil
}

func TestSaveRegencyNormalizesDocumentCode(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	actor := auth.Principal{UserID: "actor-1"}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "test"}
	saved, err := service.SaveRegency(context.Background(), actor, RegencyInput{
		ProvinceName: " Sulawesi Selatan ", Name: " Wajo ", DocumentCode: " wjo ", IsActive: true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DocumentCode != "WJO" || repository.regencyInput.ProvinceName != "Sulawesi Selatan" || repository.regencyInput.Name != "Wajo" {
		t.Fatalf("input was not normalized: %+v", repository.regencyInput)
	}
	if repository.actor.UserID != actor.UserID || repository.meta.UserAgent != meta.UserAgent {
		t.Fatalf("audit context was not forwarded: actor=%+v meta=%+v", repository.actor, repository.meta)
	}

	for _, code := range []string{"WJ", "WAJO", "W1O"} {
		_, err := service.SaveRegency(context.Background(), auth.Principal{}, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Wajo", DocumentCode: code}, auth.ClientMeta{})
		if !errors.Is(err, ErrDocumentCodeInvalid) {
			t.Fatalf("code=%q err=%v", code, err)
		}
	}
}

func TestSaveProgramRequiresFarmerOrFisherman(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveProgram(context.Background(), auth.Principal{}, ProgramInput{
		Code: "TEST-2026", Name: "Test", ProgramType: "vehicle", FiscalYear: 2026, Status: "draft",
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrProgramTypeInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestSaveScheduleRejectsEndBeforeStart(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "documentation", Name: "Wajo Tahap 1",
		StartDate: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), Status: "draft",
		DistributionNumberPadding: 4,
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleDatesInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestSaveScheduleDefaultsDistributionNumberPadding(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "documentation", Name: "Wajo Tahap 1",
		StartDate: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Status: "active",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleInput.DistributionNumberPadding != 4 {
		t.Fatalf("padding=%d", repository.scheduleInput.DistributionNumberPadding)
	}
}

func TestSaveScheduleNormalizesNameToUppercase(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "documentation", Name: "  Wajo tahap 1  ",
		StartDate: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Status: "active",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleInput.Name != "WAJO TAHAP 1" {
		t.Fatalf("schedule name=%q", repository.scheduleInput.Name)
	}
}

func TestSavePackageTemplateRequiresEquipmentOptionsWhenPublishing(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "published",
		Values: map[string]any{"converter_brand": "ERGAS"},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrPackageOptionsRequired) {
		t.Fatalf("missing options err=%v", err)
	}

	_, err = service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "draft",
		Values: map[string]any{"converter_brand": "ERGAS"},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("draft without options should be allowed: %v", err)
	}

	_, err = service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "published",
		Values: map[string]any{
			"converter_brand": "ERGAS",
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\""}},
			"hose_options":    []any{map[string]any{"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6m/10m"}},
		},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("complete options should be allowed: %v", err)
	}
}

func TestSaveDocumentationTemplateRejectsDuplicateSlotCodes(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, DocumentationTemplateInput{
		TemplateCode: "DOK-PETANI", Name: "Dokumentasi", ProgramType: ProgramFarmer, Status: "draft",
		Slots: []DocumentationTemplateSlotInput{
			{SlotCode: "recipient", Label: "Penerima", MinFiles: 1, MaxFiles: 1, InputSource: "both"},
			{SlotCode: "recipient", Label: "Penerima lagi", MinFiles: 1, MaxFiles: 1, InputSource: "camera"},
		},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrTemplateSlotInvalid) {
		t.Fatalf("err=%v", err)
	}
}
