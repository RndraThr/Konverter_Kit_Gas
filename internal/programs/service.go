package programs

import (
	"context"
	"regexp"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/textnorm"
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

type Service struct {
	repository repository
}

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) ListRegencies(ctx context.Context, scope auth.RegencyScope) ([]Regency, error) {
	return s.repository.ListRegencies(ctx, scope)
}

func (s *Service) SaveRegency(ctx context.Context, actor auth.Principal, input RegencyInput, meta auth.ClientMeta) (Regency, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProvinceName = textnorm.BusinessUpper(input.ProvinceName)
	input.Name = textnorm.BusinessUpper(input.Name)
	input.DocumentCode = strings.ToUpper(strings.TrimSpace(input.DocumentCode))
	input.Notes = textnorm.BusinessUpper(input.Notes)
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
	input.Name = textnorm.BusinessUpper(input.Name)
	input.Notes = textnorm.BusinessUpper(input.Notes)
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
	input.Name = textnorm.BusinessUpper(input.Name)
	input.Notes = textnorm.BusinessUpper(input.Notes)
	input.SupervisorName = textnorm.BusinessUpper(input.SupervisorName)
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
	input.Name = textnorm.BusinessUpper(input.Name)
	input.Values = normalizePackageDisplayValues(input.Values)
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
	return validMachineOptionList(values["machine_options"]) && validHoseOptionList(values["hose_options"])
}

// validMachineOptionList memastikan setiap opsi mesin memiliki code, brand, type, power, dan
// fuel_type. Field power/fuel_type dibutuhkan snapshot DP3/Rekap Harian agar data mesin per
// penerima dapat dirender tanpa lookup ke template terkini.
func validMachineOptionList(raw any) bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, entry := range list {
		option, ok := entry.(map[string]any)
		if !ok || !nonEmptyString(option["code"]) || !nonEmptyString(option["brand"]) || !nonEmptyString(option["type"]) || !nonEmptyString(option["power"]) || !nonEmptyString(option["fuel_type"]) {
			return false
		}
	}
	return true
}

func validHoseOptionList(raw any) bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, entry := range list {
		option, ok := entry.(map[string]any)
		if !ok || !nonEmptyString(option["code"]) {
			return false
		}
		legacy := nonEmptyString(option["brand"]) && nonEmptyString(option["spec"])
		separated := nonEmptyString(option["suction_brand"]) && nonEmptyString(option["suction_spec"]) && nonEmptyString(option["discharge_brand"]) && nonEmptyString(option["discharge_spec"])
		if !legacy && !separated {
			return false
		}
	}
	return true
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func normalizePackageDisplayValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	normalized := make(map[string]any, len(values))
	for key, value := range values {
		normalized[key] = value
	}
	if value, exists := values["machine_options"]; exists {
		normalized["machine_options"] = normalizeDisplayCollection(value, "brand", "type", "power", "fuel_type")
	}
	if value, exists := values["hose_options"]; exists {
		normalized["hose_options"] = normalizeDisplayCollection(value, "brand", "spec", "suction_brand", "suction_spec", "discharge_brand", "discharge_spec")
	}
	if value, exists := values["converter_options"]; exists {
		normalized["converter_options"] = normalizeDisplayCollection(value, "brand", "spec")
	}
	if value, exists := values["components"]; exists {
		normalized["components"] = normalizeDisplayCollection(value, "label", "unit")
	}
	return normalized
}

func normalizeDisplayCollection(value any, fields ...string) any {
	items, ok := value.([]any)
	if !ok {
		return value
	}
	normalized := make([]any, len(items))
	for index, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			normalized[index] = item
			continue
		}
		copyEntry := make(map[string]any, len(entry))
		for key, entryValue := range entry {
			copyEntry[key] = entryValue
		}
		for _, field := range fields {
			if text, exists := copyEntry[field].(string); exists {
				copyEntry[field] = textnorm.BusinessUpper(text)
			}
		}
		normalized[index] = copyEntry
	}
	return normalized
}

func (s *Service) ListDocumentationTemplates(ctx context.Context) ([]DocumentationTemplate, error) {
	return s.repository.ListDocumentationTemplates(ctx)
}

func (s *Service) SaveDocumentationTemplate(ctx context.Context, actor auth.Principal, input DocumentationTemplateInput, meta auth.ClientMeta) (DocumentationTemplate, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.TemplateCode = strings.ToUpper(strings.TrimSpace(input.TemplateCode))
	input.Name = textnorm.BusinessUpper(input.Name)
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
		slot.Label = textnorm.BusinessUpper(slot.Label)
		slot.Stage = strings.ToLower(strings.TrimSpace(slot.Stage))
		slot.InputSource = strings.ToLower(strings.TrimSpace(slot.InputSource))
		slot.Instructions = textnorm.BusinessUpper(slot.Instructions)
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
	input.Name = textnorm.BusinessUpper(input.Name)
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
