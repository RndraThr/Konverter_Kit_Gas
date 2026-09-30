package programs

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"konkit/internal/auth"
	"konkit/internal/media"
)

var (
	documentCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
	stableCodePattern   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,49}$`)
	slotCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	zoneCodePattern     = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,49}$`)
)

type repository interface {
	ListRegencies(context.Context, auth.RegencyScope) ([]Regency, error)
	SaveRegency(context.Context, auth.Principal, RegencyInput, auth.ClientMeta) (Regency, error)
	ListPrograms(context.Context) ([]Program, error)
	SaveProgram(context.Context, auth.Principal, ProgramInput, auth.ClientMeta) (Program, error)
	ListSchedules(context.Context, auth.RegencyScope) ([]Schedule, error)
	SaveSchedule(context.Context, auth.Principal, ScheduleInput, auth.ClientMeta) (Schedule, error)
	ListPackageTemplates(context.Context) ([]PackageTemplate, error)
	SavePackageTemplate(context.Context, auth.Principal, PackageTemplateInput, auth.ClientMeta) (PackageTemplate, error)
	ListDocumentationTemplates(context.Context) ([]DocumentationTemplate, error)
	SaveDocumentationTemplate(context.Context, auth.Principal, DocumentationTemplateInput, auth.ClientMeta) (DocumentationTemplate, error)
	ListZones(context.Context, string, auth.RegencyScope) ([]ProgramZone, error)
	SaveZone(context.Context, auth.Principal, ZoneInput, auth.ClientMeta) (ProgramZone, error)
	AssignRegency(context.Context, auth.Principal, RegencyAssignmentInput, auth.RegencyScope, auth.ClientMeta) (ProgramZone, error)
	ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (StorageContext, error)
}

type documentProfileRepository interface {
	ListDocumentProfiles(context.Context, string) ([]DocumentProfile, error)
	SaveDocumentProfile(context.Context, auth.Principal, DocumentProfileInput, auth.ClientMeta) (DocumentProfile, error)
	PublishDocumentProfile(context.Context, auth.Principal, string, string, auth.ClientMeta) (DocumentProfile, error)
	SaveDocumentLogo(context.Context, auth.Principal, DocumentLogo, auth.ClientMeta) (DocumentLogo, string, error)
	UpdateDocumentLogo(context.Context, auth.Principal, DocumentLogoUpdateInput, auth.ClientMeta) (DocumentLogo, error)
	GetDocumentLogo(context.Context, string, string) (DocumentLogo, error)
	GetDocumentProfile(context.Context, string) (DocumentProfile, error)
	GetProgram(context.Context, string) (Program, error)
}

type Service struct {
	repository        repository
	profileRepository documentProfileRepository
	storage           media.Storage
}

func NewService(repository repository, storage ...media.Storage) *Service {
	service := &Service{repository: repository}
	service.profileRepository, _ = repository.(documentProfileRepository)
	if len(storage) > 0 {
		service.storage = storage[0]
	}
	return service
}

func (s *Service) ListDocumentProfiles(ctx context.Context, programID string) ([]DocumentProfile, error) {
	programID = strings.TrimSpace(programID)
	if programID == "" {
		return nil, ErrInvalidInput
	}
	if s.profileRepository == nil {
		return nil, ErrNotFound
	}
	profiles, err := s.profileRepository.ListDocumentProfiles(ctx, programID)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		for j := range profiles[i].Logos {
			profiles[i].Logos[j].ContentURL = fmt.Sprintf("/api/v1/program-setup/programs/%s/document-profiles/%s/logos/%s/content", programID, profiles[i].ID, profiles[i].Logos[j].ID)
		}
	}
	return profiles, nil
}

func (s *Service) SaveDocumentProfile(ctx context.Context, actor auth.Principal, input DocumentProfileInput, meta auth.ClientMeta) (DocumentProfile, error) {
	input.ID, input.ProgramID = strings.TrimSpace(input.ID), strings.TrimSpace(input.ProgramID)
	input.Title, input.Subtitle = strings.TrimSpace(input.Title), strings.TrimSpace(input.Subtitle)
	input.ProcurementDescription, input.DocumentSeries = strings.TrimSpace(input.ProcurementDescription), strings.ToUpper(strings.TrimSpace(input.DocumentSeries))
	if input.ProgramID == "" || input.Title == "" || input.ProcurementDescription == "" || input.DocumentSeries == "" {
		return DocumentProfile{}, ErrInvalidInput
	}
	if s.profileRepository == nil {
		return DocumentProfile{}, ErrNotFound
	}
	return s.profileRepository.SaveDocumentProfile(ctx, actor, input, meta)
}

func (s *Service) PublishDocumentProfile(ctx context.Context, actor auth.Principal, programID, profileID string, meta auth.ClientMeta) (DocumentProfile, error) {
	if s.profileRepository == nil {
		return DocumentProfile{}, ErrNotFound
	}
	return s.profileRepository.PublishDocumentProfile(ctx, actor, strings.TrimSpace(programID), strings.TrimSpace(profileID), meta)
}

func (s *Service) UploadDocumentLogo(ctx context.Context, actor auth.Principal, input DocumentLogoInput, meta auth.ClientMeta) (DocumentLogo, error) {
	if s.storage == nil {
		return DocumentLogo{}, ErrDocumentLogoInvalid
	}
	input.ProfileVersionID, input.SlotCode = strings.TrimSpace(input.ProfileVersionID), strings.ToLower(strings.TrimSpace(input.SlotCode))
	input.OriginalFilename = strings.TrimSpace(input.OriginalFilename)
	if input.ProfileVersionID == "" || !slotCodePattern.MatchString(input.SlotCode) || len(input.Data) == 0 || len(input.Data) > 10<<20 || input.SortOrder < 0 {
		return DocumentLogo{}, ErrDocumentLogoInvalid
	}
	mimeType := http.DetectContentType(input.Data[:min(len(input.Data), 512)])
	if mimeType != "image/png" && mimeType != "image/jpeg" {
		return DocumentLogo{}, ErrDocumentLogoInvalid
	}
	if input.MaxWidthMM <= 0 {
		input.MaxWidthMM = 35
	}
	if input.MaxHeightMM <= 0 {
		input.MaxHeightMM = 18
	}
	if s.profileRepository == nil {
		return DocumentLogo{}, ErrNotFound
	}
	profile, err := s.profileRepository.GetDocumentProfile(ctx, input.ProfileVersionID)
	if err != nil {
		return DocumentLogo{}, err
	}
	program, err := s.profileRepository.GetProgram(ctx, profile.ProgramID)
	if err != nil {
		return DocumentLogo{}, err
	}
	key, err := newDocumentAssetKey()
	if err != nil {
		return DocumentLogo{}, err
	}
	storageKey, size, checksum, err := s.storage.Put(ctx, key, []string{"PROGRAM ASSETS", program.Code, fmt.Sprintf("DOCUMENT PROFILE V%d", profile.Version)}, bytes.NewReader(input.Data))
	if err != nil {
		return DocumentLogo{}, err
	}
	logo, oldKey, err := s.profileRepository.SaveDocumentLogo(ctx, actor, DocumentLogo{ProfileVersionID: input.ProfileVersionID, SlotCode: input.SlotCode, StorageKey: storageKey, OriginalFilename: input.OriginalFilename, MimeType: mimeType, ByteSize: size, Checksum: checksum, SortOrder: input.SortOrder, MaxWidthMM: input.MaxWidthMM, MaxHeightMM: input.MaxHeightMM, IsVisible: true}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return DocumentLogo{}, err
	}
	if oldKey != "" && oldKey != storageKey {
		_ = s.storage.Delete(context.Background(), oldKey)
	}
	return logo, nil
}

func (s *Service) UpdateDocumentLogo(ctx context.Context, actor auth.Principal, input DocumentLogoUpdateInput, meta auth.ClientMeta) (DocumentLogo, error) {
	input.ID, input.ProfileVersionID = strings.TrimSpace(input.ID), strings.TrimSpace(input.ProfileVersionID)
	if input.ID == "" || input.ProfileVersionID == "" || input.SortOrder < 0 || input.MaxWidthMM <= 0 || input.MaxHeightMM <= 0 {
		return DocumentLogo{}, ErrInvalidInput
	}
	if s.profileRepository == nil {
		return DocumentLogo{}, ErrNotFound
	}
	return s.profileRepository.UpdateDocumentLogo(ctx, actor, input, meta)
}

func (s *Service) OpenDocumentLogo(ctx context.Context, profileID, logoID string) (DocumentLogoContent, error) {
	if s.storage == nil {
		return DocumentLogoContent{}, ErrNotFound
	}
	if s.profileRepository == nil {
		return DocumentLogoContent{}, ErrNotFound
	}
	logo, err := s.profileRepository.GetDocumentLogo(ctx, strings.TrimSpace(profileID), strings.TrimSpace(logoID))
	if err != nil {
		return DocumentLogoContent{}, err
	}
	reader, err := s.storage.Open(ctx, logo.StorageKey)
	if err != nil {
		return DocumentLogoContent{}, err
	}
	return DocumentLogoContent{Reader: reader, MimeType: logo.MimeType, Filename: logo.OriginalFilename}, nil
}

func newDocumentAssetKey() (string, error) { return uuid.NewString(), nil }

func (s *Service) ListRegencies(ctx context.Context, scope auth.RegencyScope) ([]Regency, error) {
	return s.repository.ListRegencies(ctx, scope)
}

func (s *Service) SaveRegency(ctx context.Context, actor auth.Principal, input RegencyInput, meta auth.ClientMeta) (Regency, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProvinceName = strings.TrimSpace(input.ProvinceName)
	input.Name = strings.TrimSpace(input.Name)
	input.DocumentCode = strings.ToUpper(strings.TrimSpace(input.DocumentCode))
	input.Notes = strings.TrimSpace(input.Notes)
	if input.ProvinceName == "" || input.Name == "" {
		return Regency{}, ErrInvalidInput
	}
	if !documentCodePattern.MatchString(input.DocumentCode) {
		return Regency{}, ErrDocumentCodeInvalid
	}
	return s.repository.SaveRegency(ctx, actor, input, meta)
}

func (s *Service) ListPrograms(ctx context.Context) ([]Program, error) {
	return s.repository.ListPrograms(ctx)
}

func (s *Service) SaveProgram(ctx context.Context, actor auth.Principal, input ProgramInput, meta auth.ClientMeta) (Program, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	input.Notes = strings.TrimSpace(input.Notes)
	if !validProgramType(input.ProgramType) {
		return Program{}, ErrProgramTypeInvalid
	}
	if !stableCodePattern.MatchString(input.Code) || input.Name == "" || input.FiscalYear < 2000 || input.FiscalYear > time.Now().Year()+5 || !oneOf(input.Status, "draft", "active", "completed", "archived") {
		return Program{}, ErrInvalidInput
	}
	return s.repository.SaveProgram(ctx, actor, input, meta)
}

func (s *Service) ListSchedules(ctx context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	return s.repository.ListSchedules(ctx, scope)
}

func (s *Service) SaveSchedule(ctx context.Context, actor auth.Principal, input ScheduleInput, meta auth.ClientMeta) (Schedule, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.PackageTemplateVersionID = strings.TrimSpace(input.PackageTemplateVersionID)
	input.DocumentationTemplateVersionID = strings.TrimSpace(input.DocumentationTemplateVersionID)
	input.Name = strings.ToUpper(strings.TrimSpace(input.Name))
	input.Notes = strings.TrimSpace(input.Notes)
	input.SupervisorName = strings.TrimSpace(input.SupervisorName)
	if input.EndDate.Before(input.StartDate) {
		return Schedule{}, ErrScheduleDatesInvalid
	}
	if input.DistributionNumberPadding == 0 {
		input.DistributionNumberPadding = 4
	}
	if input.SlotQuota != nil && *input.SlotQuota < 1 {
		return Schedule{}, ErrSlotQuotaInvalid
	}
	if input.ProgramID == "" || input.RegencyID == "" || input.PackageTemplateVersionID == "" || input.DocumentationTemplateVersionID == "" || input.Name == "" || input.StartDate.IsZero() || input.EndDate.IsZero() || input.DistributionNumberPadding < 1 || input.DistributionNumberPadding > 8 || !oneOf(input.Status, "draft", "active", "completed", "cancelled") {
		return Schedule{}, ErrInvalidInput
	}
	if input.ReceiptPolicy == nil {
		input.ReceiptPolicy = map[string]any{"mode": "block_repeat"}
	}
	return s.repository.SaveSchedule(ctx, actor, input, meta)
}

func (s *Service) ListPackageTemplates(ctx context.Context) ([]PackageTemplate, error) {
	return s.repository.ListPackageTemplates(ctx)
}

func (s *Service) SavePackageTemplate(ctx context.Context, actor auth.Principal, input PackageTemplateInput, meta auth.ClientMeta) (PackageTemplate, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.TemplateCode = strings.ToUpper(strings.TrimSpace(input.TemplateCode))
	input.Name = strings.TrimSpace(input.Name)
	if !validProgramType(input.ProgramType) {
		return PackageTemplate{}, ErrProgramTypeInvalid
	}
	if !stableCodePattern.MatchString(input.TemplateCode) || input.Name == "" || !oneOf(input.Status, "draft", "published", "retired") {
		return PackageTemplate{}, ErrInvalidInput
	}
	if input.Values == nil {
		input.Values = map[string]any{}
	}
	if input.Status == "published" && !hasEquipmentOptions(input.Values) {
		return PackageTemplate{}, ErrPackageOptionsRequired
	}
	return s.repository.SavePackageTemplate(ctx, actor, input, meta)
}

func hasEquipmentOptions(values map[string]any) bool {
	return validOptionList(values["machine_options"], "brand", "type") && validOptionList(values["hose_options"], "brand", "spec")
}

func validOptionList(raw any, secondField string, thirdField string) bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, entry := range list {
		option, ok := entry.(map[string]any)
		if !ok || !nonEmptyString(option["code"]) || !nonEmptyString(option[secondField]) || !nonEmptyString(option[thirdField]) {
			return false
		}
	}
	return true
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func (s *Service) ListDocumentationTemplates(ctx context.Context) ([]DocumentationTemplate, error) {
	return s.repository.ListDocumentationTemplates(ctx)
}

func (s *Service) SaveDocumentationTemplate(ctx context.Context, actor auth.Principal, input DocumentationTemplateInput, meta auth.ClientMeta) (DocumentationTemplate, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.TemplateCode = strings.ToUpper(strings.TrimSpace(input.TemplateCode))
	input.Name = strings.TrimSpace(input.Name)
	if !validProgramType(input.ProgramType) {
		return DocumentationTemplate{}, ErrProgramTypeInvalid
	}
	if !stableCodePattern.MatchString(input.TemplateCode) || input.Name == "" || !oneOf(input.Status, "draft", "published", "retired") {
		return DocumentationTemplate{}, ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(input.Slots))
	for index := range input.Slots {
		slot := &input.Slots[index]
		slot.SlotCode = strings.ToLower(strings.TrimSpace(slot.SlotCode))
		slot.Label = strings.TrimSpace(slot.Label)
		slot.Stage = strings.ToLower(strings.TrimSpace(slot.Stage))
		slot.InputSource = strings.ToLower(strings.TrimSpace(slot.InputSource))
		slot.Instructions = strings.TrimSpace(slot.Instructions)
		_, duplicate := seen[slot.SlotCode]
		if duplicate || !slotCodePattern.MatchString(slot.SlotCode) || slot.Label == "" || slot.MinFiles < 0 || slot.MaxFiles < slot.MinFiles || !oneOf(slot.InputSource, "camera", "gallery", "both") || !oneOf(slot.Stage, "mesin", "dokumen", "penyerahan") {
			return DocumentationTemplate{}, ErrTemplateSlotInvalid
		}
		seen[slot.SlotCode] = struct{}{}
	}
	return s.repository.SaveDocumentationTemplate(ctx, actor, input, meta)
}

func (s *Service) ListZones(ctx context.Context, programID string, scope auth.RegencyScope) ([]ProgramZone, error) {
	return s.repository.ListZones(ctx, strings.TrimSpace(programID), scope)
}

func (s *Service) SaveZone(ctx context.Context, actor auth.Principal, input ZoneInput, meta auth.ClientMeta) (ProgramZone, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	if input.ProgramID == "" || input.Name == "" || input.SortOrder < 0 {
		return ProgramZone{}, ErrInvalidInput
	}
	if !zoneCodePattern.MatchString(input.Code) {
		return ProgramZone{}, ErrInvalidInput
	}
	return s.repository.SaveZone(ctx, actor, input, meta)
}

func (s *Service) AssignRegency(ctx context.Context, actor auth.Principal, input RegencyAssignmentInput, scope auth.RegencyScope, meta auth.ClientMeta) (ProgramZone, error) {
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.ZoneID = strings.TrimSpace(input.ZoneID)
	if input.ProgramID == "" || input.RegencyID == "" || input.ZoneID == "" {
		return ProgramZone{}, ErrInvalidInput
	}
	return s.repository.AssignRegency(ctx, actor, input, scope, meta)
}

func (s *Service) ResolveStorageContext(ctx context.Context, programID string, regencyID string, scope auth.RegencyScope) (StorageContext, error) {
	programID = strings.TrimSpace(programID)
	regencyID = strings.TrimSpace(regencyID)
	if programID == "" || regencyID == "" {
		return StorageContext{}, ErrInvalidInput
	}
	return s.repository.ResolveStorageContext(ctx, programID, regencyID, scope)
}

func validProgramType(value ProgramType) bool {
	return value == ProgramFarmer || value == ProgramFisherman
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
