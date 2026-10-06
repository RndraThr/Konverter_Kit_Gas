package programs

import (
	"context"
	"errors"
	"testing"
	"time"

	"konkit/internal/auth"
)

type repositoryStub struct {
	regencyInput     RegencyInput
	programInput     ProgramInput
	scheduleInput    ScheduleInput
	packageInput     PackageTemplateInput
	documentInput    DocumentationTemplateInput
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
func (r *repositoryStub) SaveSchedule(_ context.Context, _ auth.Principal, input ScheduleInput, _ auth.RegencyScope, _ auth.ClientMeta) (Schedule, error) {
	r.scheduleInput = input
	return Schedule{}, nil
}
func (r *repositoryStub) ListPackageTemplates(context.Context) ([]PackageTemplate, error) {
	return nil, nil
}
func (r *repositoryStub) SavePackageTemplate(_ context.Context, _ auth.Principal, input PackageTemplateInput, _ auth.ClientMeta) (PackageTemplate, error) {
	r.packageInput = input
	return PackageTemplate{}, nil
}
func (r *repositoryStub) ListDocumentationTemplates(context.Context) ([]DocumentationTemplate, error) {
	return nil, nil
}
func (r *repositoryStub) SaveDocumentationTemplate(_ context.Context, _ auth.Principal, input DocumentationTemplateInput, _ auth.ClientMeta) (DocumentationTemplate, error) {
	r.documentInput = input
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
		ProvinceName: " Sulawesi Selatan ", Name: " Wajo ", DocumentCode: " wjo ", IsActive: true, Notes: " wilayah utama ",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DocumentCode != "WJO" || repository.regencyInput.ProvinceName != "SULAWESI SELATAN" || repository.regencyInput.Name != "WAJO" || repository.regencyInput.Notes != "WILAYAH UTAMA" {
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

func TestSaveProgramSetupUppercasesBusinessTextAndPreservesTechnicalKeys(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	if _, err := service.SaveProgram(context.Background(), auth.Principal{}, ProgramInput{
		Code: "petani-2026", Name: " bantuan petani ", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "draft", Notes: " tahap pertama ",
	}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.programInput.Name != "BANTUAN PETANI" || repository.programInput.Notes != "TAHAP PERTAMA" || repository.programInput.Code != "PETANI-2026" {
		t.Fatalf("program=%+v", repository.programInput)
	}

	start := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	if _, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package", DocumentationTemplateVersionID: "document",
		Name: " wajo tahap 1 ", Notes: " gelombang pagi ", SupervisorName: " andi saputra ", StartDate: start, EndDate: start.Add(24 * time.Hour), Status: "draft",
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.scheduleInput.Notes != "GELOMBANG PAGI" || repository.scheduleInput.SupervisorName != "ANDI SAPUTRA" {
		t.Fatalf("schedule=%+v", repository.scheduleInput)
	}

	if _, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{ProgramID: "program", Code: "zone-a", Name: " pesisir utara "}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.zoneInput.Name != "PESISIR UTARA" || repository.zoneInput.Code != "ZONE-A" {
		t.Fatalf("zone=%+v", repository.zoneInput)
	}
}

func TestSaveTemplatesUppercaseDisplayValuesAndPreserveCodes(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	values := map[string]any{
		"machine_options":   []any{map[string]any{"code": "shark-spwp8030", "brand": " shark ", "type": " spwp 80-30 ", "power": " 5.5 hp ", "fuel_type": " bensin "}},
		"hose_options":      []any{map[string]any{"code": "hose-set", "brand": " triliunhose ", "spec": " 6m/10m ", "suction_brand": " triliun ", "suction_spec": " 6 m ", "discharge_brand": " yamakoyo ", "discharge_spec": " 10 m "}},
		"converter_options": []any{map[string]any{"code": "ergas-kit", "brand": " ergas ", "spec": " paket lengkap "}},
		"components":        []any{map[string]any{"code": "lpg", "label": " tabung lpg 3 kg ", "unit": " tabung ", "quantity": float64(1)}},
	}
	if _, err := service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{TemplateCode: "pkg-petani", Name: " paket petani ", ProgramType: ProgramFarmer, Status: "draft", Values: values}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	machine := repository.packageInput.Values["machine_options"].([]any)[0].(map[string]any)
	hose := repository.packageInput.Values["hose_options"].([]any)[0].(map[string]any)
	converter := repository.packageInput.Values["converter_options"].([]any)[0].(map[string]any)
	component := repository.packageInput.Values["components"].([]any)[0].(map[string]any)
	if repository.packageInput.Name != "PAKET PETANI" || machine["code"] != "shark-spwp8030" || machine["brand"] != "SHARK" || machine["power"] != "5.5 HP" || hose["suction_spec"] != "6 M" || converter["spec"] != "PAKET LENGKAP" || component["label"] != "TABUNG LPG 3 KG" || component["unit"] != "TABUNG" {
		t.Fatalf("package input=%+v", repository.packageInput)
	}

	if _, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, DocumentationTemplateInput{
		TemplateCode: "doc-petani", Name: " dokumentasi petani ", ProgramType: ProgramFarmer, Status: "draft",
		Slots: []DocumentationTemplateSlotInput{{SlotCode: "serial_mesin", Label: " serial nomor mesin ", Stage: "mesin", MinFiles: 1, MaxFiles: 1, InputSource: "both", Instructions: " foto harus jelas "}},
	}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	slot := repository.documentInput.Slots[0]
	if repository.documentInput.Name != "DOKUMENTASI PETANI" || slot.SlotCode != "serial_mesin" || slot.Label != "SERIAL NOMOR MESIN" || slot.Instructions != "FOTO HARUS JELAS" {
		t.Fatalf("document input=%+v", repository.documentInput)
	}
}

func TestSavePackageTemplateDoesNotAddMissingStructuralKeys(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	if _, err := service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "pkg-minimal", Name: " paket minimal ", ProgramType: ProgramFarmer, Status: "draft",
		Values: map[string]any{"custom_note": "keep-as-is"},
	}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"machine_options", "hose_options", "converter_options", "components"} {
		if _, exists := repository.packageInput.Values[key]; exists {
			t.Fatalf("missing structural key %q was added: %+v", key, repository.packageInput.Values)
		}
	}
	if repository.packageInput.Values["custom_note"] != "keep-as-is" {
		t.Fatalf("unknown value changed: %+v", repository.packageInput.Values)
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
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleDatesInvalid) {
		t.Fatalf("err=%v", err)
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
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleInput.Name != "WAJO TAHAP 1" {
		t.Fatalf("schedule name=%q", repository.scheduleInput.Name)
	}
}

func TestSaveScheduleRejectsRegencyOutsideCallerScope(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	input := ScheduleInput{
		ProgramID: "program", RegencyID: "regency-other", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "document", Name: "Wajo Tahap 1",
		StartDate: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Status: "draft",
	}

	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, input, auth.RegencyScope{RegencyIDs: []string{"regency-mine"}}, auth.ClientMeta{})
	if !errors.Is(err, ErrRegencyOutOfScope) {
		t.Fatalf("err=%v, want ErrRegencyOutOfScope", err)
	}

	input.RegencyID = "regency-mine"
	if _, err := service.SaveSchedule(context.Background(), auth.Principal{}, input, auth.RegencyScope{RegencyIDs: []string{"regency-mine"}}, auth.ClientMeta{}); err != nil {
		t.Fatalf("in-scope save rejected: %v", err)
	}
	if repository.scheduleInput.RegencyID != "regency-mine" {
		t.Fatalf("regency=%+v", repository.scheduleInput)
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
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
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
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\"", "power": "5.5 HP", "fuel_type": "Bensin"}},
			"hose_options":    []any{map[string]any{"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6m/10m"}},
		},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("complete options should be allowed: %v", err)
	}

	_, err = service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "published",
		Values: map[string]any{
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\"", "power": "5.5 HP", "fuel_type": "Bensin"}},
			"hose_options": []any{map[string]any{
				"code": "hose-set", "suction_brand": "TRILLIUNHOSE", "suction_spec": "6 M",
				"discharge_brand": "YAMAKOYO", "discharge_spec": "10 M",
			}},
		},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("separated suction and discharge hose fields should be allowed: %v", err)
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

func TestSaveDocumentationTemplateNormalizesAndValidatesMediaKind(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	input := DocumentationTemplateInput{
		TemplateCode: "DOK-MEDIA", Name: "Dokumentasi Media", ProgramType: ProgramFarmer, Status: "draft",
		Slots: []DocumentationTemplateSlotInput{
			{SlotCode: "default_image", Label: "Default image", Stage: "mesin", MinFiles: 1, MaxFiles: 1, InputSource: "both"},
			{SlotCode: "video", Label: "Video", Stage: "penyerahan", MinFiles: 1, MaxFiles: 1, InputSource: "gallery", MediaKind: " VIDEO "},
			{SlotCode: "mixed", Label: "Mixed", Stage: "dokumen", MinFiles: 1, MaxFiles: 2, InputSource: "both", MediaKind: "image_video"},
		},
	}
	if _, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, input, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if got := repository.documentInput.Slots[0].MediaKind; got != "image" {
		t.Fatalf("default media_kind=%q, want image", got)
	}
	if got := repository.documentInput.Slots[1].MediaKind; got != "video" {
		t.Fatalf("normalized media_kind=%q, want video", got)
	}
	if got := repository.documentInput.Slots[2].MediaKind; got != "image_video" {
		t.Fatalf("mixed media_kind=%q, want image_video", got)
	}

	input.Slots[0].MediaKind = "document"
	if _, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, input, auth.ClientMeta{}); !errors.Is(err, ErrTemplateSlotInvalid) {
		t.Fatalf("invalid media_kind err=%v, want ErrTemplateSlotInvalid", err)
	}
}

func TestSaveZoneNormalizesCodeAndUppercasesName(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	zone, err := service.SaveZone(context.Background(), auth.Principal{}, ZoneInput{
		ProgramID: "program-1", Code: " zone-a ", Name: "  Zona A  ", SortOrder: 2,
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if zone.Code != "ZONE-A" || repository.zoneInput.Code != "ZONE-A" || repository.zoneInput.Name != "ZONA A" {
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
