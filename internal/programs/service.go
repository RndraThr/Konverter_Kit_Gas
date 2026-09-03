package programs

import (
	"context"
	"regexp"
	"strings"
	"time"

	"konkit/internal/auth"
)

var (
	documentCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
	stableCodePattern   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,49}$`)
	slotCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
)

type repository interface {
	ListRegencies(context.Context) ([]Regency, error)
	SaveRegency(context.Context, auth.Principal, RegencyInput, auth.ClientMeta) (Regency, error)
	ListPrograms(context.Context) ([]Program, error)
	SaveProgram(context.Context, auth.Principal, ProgramInput, auth.ClientMeta) (Program, error)
	ListSchedules(context.Context) ([]Schedule, error)
	SaveSchedule(context.Context, auth.Principal, ScheduleInput, auth.ClientMeta) (Schedule, error)
	ListPackageTemplates(context.Context) ([]PackageTemplate, error)
	SavePackageTemplate(context.Context, auth.Principal, PackageTemplateInput, auth.ClientMeta) (PackageTemplate, error)
	ListDocumentationTemplates(context.Context) ([]DocumentationTemplate, error)
	SaveDocumentationTemplate(context.Context, auth.Principal, DocumentationTemplateInput, auth.ClientMeta) (DocumentationTemplate, error)
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) ListRegencies(ctx context.Context) ([]Regency, error) {
	return s.repository.ListRegencies(ctx)
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

func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return s.repository.ListSchedules(ctx)
}

func (s *Service) SaveSchedule(ctx context.Context, actor auth.Principal, input ScheduleInput, meta auth.ClientMeta) (Schedule, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.PackageTemplateVersionID = strings.TrimSpace(input.PackageTemplateVersionID)
	input.DocumentationTemplateVersionID = strings.TrimSpace(input.DocumentationTemplateVersionID)
	input.Name = strings.ToUpper(strings.TrimSpace(input.Name))
	input.Notes = strings.TrimSpace(input.Notes)
	if input.EndDate.Before(input.StartDate) {
		return Schedule{}, ErrScheduleDatesInvalid
	}
	if input.DistributionNumberPadding == 0 {
		input.DistributionNumberPadding = 4
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
	return s.repository.SavePackageTemplate(ctx, actor, input, meta)
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
		slot.Stage = strings.TrimSpace(slot.Stage)
		if slot.Stage == "" {
			slot.Stage = "distribution"
		}
		slot.InputSource = strings.ToLower(strings.TrimSpace(slot.InputSource))
		slot.Instructions = strings.TrimSpace(slot.Instructions)
		_, duplicate := seen[slot.SlotCode]
		if duplicate || !slotCodePattern.MatchString(slot.SlotCode) || slot.Label == "" || slot.MinFiles < 0 || slot.MaxFiles < slot.MinFiles || !oneOf(slot.InputSource, "camera", "gallery", "both") {
			return DocumentationTemplate{}, ErrTemplateSlotInvalid
		}
		seen[slot.SlotCode] = struct{}{}
	}
	return s.repository.SaveDocumentationTemplate(ctx, actor, input, meta)
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
