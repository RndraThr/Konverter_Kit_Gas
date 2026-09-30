package programs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
)

type documentProfileRepositoryStub struct {
	*repositoryStub
	saveLogoErr error
}

func (r *documentProfileRepositoryStub) ListDocumentProfiles(context.Context, string) ([]DocumentProfile, error) {
	return nil, nil
}
func (r *documentProfileRepositoryStub) SaveDocumentProfile(context.Context, auth.Principal, DocumentProfileInput, auth.ClientMeta) (DocumentProfile, error) {
	return DocumentProfile{}, nil
}
func (r *documentProfileRepositoryStub) PublishDocumentProfile(context.Context, auth.Principal, string, string, auth.ClientMeta) (DocumentProfile, error) {
	return DocumentProfile{}, nil
}
func (r *documentProfileRepositoryStub) SaveDocumentLogo(context.Context, auth.Principal, DocumentLogo, auth.ClientMeta) (DocumentLogo, string, error) {
	return DocumentLogo{}, "", r.saveLogoErr
}
func (r *documentProfileRepositoryStub) UpdateDocumentLogo(context.Context, auth.Principal, DocumentLogoUpdateInput, auth.ClientMeta) (DocumentLogo, error) {
	return DocumentLogo{}, nil
}
func (r *documentProfileRepositoryStub) GetDocumentLogo(context.Context, string, string) (DocumentLogo, error) {
	return DocumentLogo{}, nil
}
func (r *documentProfileRepositoryStub) GetDocumentProfile(context.Context, string) (DocumentProfile, error) {
	return DocumentProfile{ID: "profile-1", ProgramID: "program-1", Version: 1, Status: "draft"}, nil
}
func (r *documentProfileRepositoryStub) GetProgram(context.Context, string) (Program, error) {
	return Program{ID: "program-1", Code: "TENDER-1"}, nil
}

type documentStorageStub struct{ deleted string }

func (s *documentStorageStub) Put(_ context.Context, key string, _ []string, source io.Reader) (string, int64, string, error) {
	data, _ := io.ReadAll(source)
	return "stored-" + key, int64(len(data)), strings.Repeat("a", 64), nil
}
func (s *documentStorageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s *documentStorageStub) Delete(_ context.Context, key string) error {
	s.deleted = key
	return nil
}
func (s *documentStorageStub) EnsureFolders(context.Context, [][]string) error { return nil }

var _ media.Storage = (*documentStorageStub)(nil)

func TestUploadDocumentLogoDeletesStoredFileWhenMetadataFails(t *testing.T) {
	repository := &documentProfileRepositoryStub{repositoryStub: &repositoryStub{}, saveLogoErr: errors.New("database unavailable")}
	storage := &documentStorageStub{}
	service := NewService(repository, storage)
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
	_, err := service.UploadDocumentLogo(context.Background(), auth.Principal{}, DocumentLogoInput{ProfileVersionID: "profile-1", SlotCode: "organizer", OriginalFilename: "logo.png", Data: png}, auth.ClientMeta{})
	if err == nil || storage.deleted == "" {
		t.Fatalf("err=%v deleted=%q", err, storage.deleted)
	}
}

type repositoryStub struct {
	regencyInput     RegencyInput
	programInput     ProgramInput
	scheduleInput    ScheduleInput
	zoneInput        ZoneInput
	assignmentInput  RegencyAssignmentInput
	resolveProgramID string
	resolveRegencyID string
	seenScope        auth.RegencyScope
	actor            auth.Principal
	meta             auth.ClientMeta
}

func (r *repositoryStub) ListRegencies(context.Context, auth.RegencyScope) ([]Regency, error) {
	return nil, nil
}
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
func (r *repositoryStub) ListSchedules(context.Context, auth.RegencyScope) ([]Schedule, error) {
	return nil, nil
}
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
func (r *repositoryStub) ListZones(context.Context, string, auth.RegencyScope) ([]ProgramZone, error) {
	return nil, nil
}
func (r *repositoryStub) SaveZone(_ context.Context, actor auth.Principal, input ZoneInput, meta auth.ClientMeta) (ProgramZone, error) {
	r.zoneInput = input
	r.actor = actor
	r.meta = meta
	return ProgramZone{ProgramID: input.ProgramID, Code: input.Code, Name: input.Name, SortOrder: input.SortOrder}, nil
}
func (r *repositoryStub) AssignRegency(_ context.Context, actor auth.Principal, input RegencyAssignmentInput, scope auth.RegencyScope, meta auth.ClientMeta) (ProgramZone, error) {
	r.assignmentInput = input
	r.seenScope = scope
	r.actor = actor
	r.meta = meta
	return ProgramZone{ID: input.ZoneID, ProgramID: input.ProgramID}, nil
}
func (r *repositoryStub) ResolveStorageContext(_ context.Context, programID string, regencyID string, scope auth.RegencyScope) (StorageContext, error) {
	r.resolveProgramID = programID
	r.resolveRegencyID = regencyID
	r.seenScope = scope
	return StorageContext{ProgramID: programID, RegencyID: regencyID}, nil
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

func TestSaveScheduleRejectsNonPositiveSlotQuota(t *testing.T) {
	service := NewService(&repositoryStub{})
	zero := 0
	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "document", Name: "Test", Status: "draft",
		StartDate: time.Now(), EndDate: time.Now().Add(24 * time.Hour),
		SlotQuota: &zero,
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotQuotaInvalid) {
		t.Fatalf("err = %v, want ErrSlotQuotaInvalid", err)
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
			{SlotCode: "recipient", Label: "Penerima", Stage: "mesin", MinFiles: 1, MaxFiles: 1, InputSource: "both"},
			{SlotCode: "recipient", Label: "Penerima lagi", Stage: "mesin", MinFiles: 1, MaxFiles: 1, InputSource: "camera"},
		},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrTemplateSlotInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestSaveDocumentationTemplateRejectsInvalidStage(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, DocumentationTemplateInput{
		TemplateCode: "DOK-TEST-STAGE", Name: "Uji Stage", ProgramType: ProgramFarmer, Status: "draft",
		Slots: []DocumentationTemplateSlotInput{
			{SlotCode: "bukti", Label: "Bukti", Stage: "distribution", MinFiles: 1, MaxFiles: 1, InputSource: "both"},
		},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrTemplateSlotInvalid) {
		t.Fatalf("expected ErrTemplateSlotInvalid for stage=%q, got %v", "distribution", err)
	}
}

func TestSaveZoneNormalizesCodeAndTrimsName(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	zone, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
		ProgramID: "program-1", Code: " zone-a ", Name: "  Zona A  ", SortOrder: 2,
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if zone.Code != "ZONE-A" || repository.zoneInput.Code != "ZONE-A" || repository.zoneInput.Name != "Zona A" {
		t.Fatalf("zone input was not normalized: %+v", repository.zoneInput)
	}
}

func TestSaveZoneRejectsBlankName(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
		ProgramID: "program-1", Code: "ZONE-A", Name: "   ", SortOrder: 0,
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestSaveZoneRejectsUnstableCode(t *testing.T) {
	service := NewService(&repositoryStub{})
	for _, code := range []string{"", "zone a", "zone/a", "-ZONE"} {
		_, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
			ProgramID: "program-1", Code: code, Name: "Zona A",
		}, auth.ClientMeta{})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("code=%q err=%v", code, err)
		}
	}
}

func TestSaveZoneRejectsNegativeSortOrder(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
		ProgramID: "program-1", Code: "ZONE-A", Name: "Zona A", SortOrder: -1,
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestSaveZoneRejectsMissingProgramID(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
		Code: "ZONE-A", Name: "Zona A",
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestAssignRegencyForwardsScopeAndTrimsInput(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	_, err := service.AssignRegency(context.Background(), auth.Principal{}, RegencyAssignmentInput{
		ProgramID: " program-1 ", RegencyID: " regency-1 ", ZoneID: " zone-1 ",
	}, scope, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.assignmentInput.ProgramID != "program-1" || repository.assignmentInput.RegencyID != "regency-1" || repository.assignmentInput.ZoneID != "zone-1" {
		t.Fatalf("assignment input was not trimmed: %+v", repository.assignmentInput)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope was not forwarded: %+v", repository.seenScope)
	}
}

func TestAssignRegencyRejectsBlankZoneID(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.AssignRegency(context.Background(), auth.Principal{}, RegencyAssignmentInput{
		ProgramID: "program-1", RegencyID: "regency-1", ZoneID: "  ",
	}, auth.RegencyScope{}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveStorageContextForwardsScope(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	_, err := service.ResolveStorageContext(context.Background(), " program-1 ", " regency-1 ", scope)
	if err != nil {
		t.Fatal(err)
	}
	if repository.resolveProgramID != "program-1" || repository.resolveRegencyID != "regency-1" {
		t.Fatalf("ids were not trimmed: program=%q regency=%q", repository.resolveProgramID, repository.resolveRegencyID)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope was not forwarded: %+v", repository.seenScope)
	}
}

type scopedRepositoryStub struct {
	repositoryStub
	seenScope auth.RegencyScope
}

func (r *scopedRepositoryStub) ListRegencies(_ context.Context, scope auth.RegencyScope) ([]Regency, error) {
	r.seenScope = scope
	return nil, nil
}
func (r *scopedRepositoryStub) ListSchedules(_ context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	r.seenScope = scope
	return nil, nil
}

func TestListRegenciesAndSchedulesForwardRegencyScope(t *testing.T) {
	repository := &scopedRepositoryStub{}
	service := NewService(repository)
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}

	if _, err := service.ListRegencies(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("ListRegencies scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.ListSchedules(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("ListSchedules scope=%+v", repository.seenScope)
	}
}
