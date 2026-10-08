package programs

import (
	"errors"
	"time"
)

var (
	ErrNotFound                 = errors.New("program setup resource not found")
	ErrInvalidInput             = errors.New("program setup input is invalid")
	ErrDocumentCodeInvalid      = errors.New("regency document code must contain exactly three letters")
	ErrDocumentCodeInUse        = errors.New("regency document code is already used")
	ErrCodeInUse                = errors.New("program or template code is already used")
	ErrProgramTypeInvalid       = errors.New("program type must be farmer or fisherman")
	ErrScheduleDatesInvalid     = errors.New("schedule end date must not precede start date")
	ErrTemplateSlotInvalid      = errors.New("documentation template slot is invalid")
	ErrPackageOptionsRequired   = errors.New("package template requires at least one machine option and one hose option when published")
	ErrSlotQuotaInvalid         = errors.New("slot_quota must be greater than zero when set")
	ErrZoneNotConfigured        = errors.New("regency is assigned to the unconfigured placeholder zone")
	ErrZonePlaceholderImmutable = errors.New("placeholder zone cannot be created or modified")
	ErrZoneProgramMismatch      = errors.New("zone does not belong to the specified program")
	ErrZoneNotEmpty             = errors.New("zone still has assigned regencies")
	ErrRegencyOutOfScope        = errors.New("regency is outside the caller's assigned scope")
)

type ProgramType string

const (
	ProgramFarmer    ProgramType = "farmer"
	ProgramFisherman ProgramType = "fisherman"
)

type Regency struct {
	ID           string    `json:"id"`
	ProvinceName string    `json:"province_name"`
	Name         string    `json:"name"`
	DocumentCode string    `json:"document_code"`
	IsActive     bool      `json:"is_active"`
	Notes        string    `json:"notes,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RegencyInput struct {
	ID           string `json:"id,omitempty"`
	ProvinceName string `json:"province_name"`
	Name         string `json:"name"`
	DocumentCode string `json:"document_code"`
	IsActive     bool   `json:"is_active"`
	Notes        string `json:"notes"`
}

type Program struct {
	ID          string      `json:"id"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	ProgramType ProgramType `json:"program_type"`
	FiscalYear  int         `json:"fiscal_year"`
	Status      string      `json:"status"`
	Notes       string      `json:"notes,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type ProgramInput struct {
	ID          string      `json:"id,omitempty"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	ProgramType ProgramType `json:"program_type"`
	FiscalYear  int         `json:"fiscal_year"`
	Status      string      `json:"status"`
	Notes       string      `json:"notes"`
}

type PackageTemplate struct {
	ID           string         `json:"id"`
	TemplateCode string         `json:"template_code"`
	Version      int            `json:"version"`
	Name         string         `json:"name"`
	ProgramType  ProgramType    `json:"program_type"`
	Values       map[string]any `json:"values"`
	Status       string         `json:"status"`
	PublishedAt  *time.Time     `json:"published_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type PackageTemplateInput struct {
	ID           string         `json:"id,omitempty"`
	TemplateCode string         `json:"template_code"`
	Name         string         `json:"name"`
	ProgramType  ProgramType    `json:"program_type"`
	Values       map[string]any `json:"values"`
	Status       string         `json:"status"`
}

type DocumentationTemplateSlot struct {
	ID                string `json:"id"`
	SlotCode          string `json:"slot_code"`
	Label             string `json:"label"`
	Stage             string `json:"stage"`
	IsRequired        bool   `json:"is_required"`
	MinFiles          int    `json:"min_files"`
	MaxFiles          int    `json:"max_files"`
	InputSource       string `json:"input_source"`
	MediaKind         string `json:"media_kind"`
	RequireLocation   bool   `json:"require_location"`
	RequireCapturedAt bool   `json:"require_captured_at"`
	Instructions      string `json:"instructions,omitempty"`
	SortOrder         int    `json:"sort_order"`
}

type DocumentationTemplateSlotInput struct {
	SlotCode          string `json:"slot_code"`
	Label             string `json:"label"`
	Stage             string `json:"stage"`
	IsRequired        bool   `json:"is_required"`
	MinFiles          int    `json:"min_files"`
	MaxFiles          int    `json:"max_files"`
	InputSource       string `json:"input_source"`
	MediaKind         string `json:"media_kind"`
	RequireLocation   bool   `json:"require_location"`
	RequireCapturedAt bool   `json:"require_captured_at"`
	Instructions      string `json:"instructions"`
	SortOrder         int    `json:"sort_order"`
}

type DocumentationTemplate struct {
	ID           string                      `json:"id"`
	TemplateCode string                      `json:"template_code"`
	Version      int                         `json:"version"`
	Name         string                      `json:"name"`
	ProgramType  ProgramType                 `json:"program_type"`
	Status       string                      `json:"status"`
	PublishedAt  *time.Time                  `json:"published_at,omitempty"`
	Slots        []DocumentationTemplateSlot `json:"slots"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

type DocumentationTemplateInput struct {
	ID           string                           `json:"id,omitempty"`
	TemplateCode string                           `json:"template_code"`
	Name         string                           `json:"name"`
	ProgramType  ProgramType                      `json:"program_type"`
	Status       string                           `json:"status"`
	Slots        []DocumentationTemplateSlotInput `json:"slots"`
}

type Schedule struct {
	ID                             string                 `json:"id"`
	ProgramID                      string                 `json:"program_id"`
	RegencyID                      string                 `json:"regency_id"`
	PackageTemplateVersionID       string                 `json:"package_template_version_id"`
	DocumentationTemplateVersionID string                 `json:"documentation_template_version_id"`
	Name                           string                 `json:"name"`
	StartDate                      time.Time              `json:"start_date"`
	EndDate                        time.Time              `json:"end_date"`
	Status                         string                 `json:"status"`
	SlotQuota                      *int                   `json:"slot_quota,omitempty"`
	ReceiptPolicy                  map[string]any         `json:"receipt_policy"`
	SupervisorName                 string                 `json:"supervisor_name,omitempty"`
	Notes                          string                 `json:"notes,omitempty"`
	Program                        *Program               `json:"program,omitempty"`
	Regency                        *Regency               `json:"regency,omitempty"`
	PackageTemplate                *PackageTemplate       `json:"package_template,omitempty"`
	DocumentationTemplate          *DocumentationTemplate `json:"documentation_template,omitempty"`
	CreatedAt                      time.Time              `json:"created_at"`
	UpdatedAt                      time.Time              `json:"updated_at"`
}

type ProgramZone struct {
	ID            string    `json:"id"`
	ProgramID     string    `json:"program_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	SortOrder     int       `json:"sort_order"`
	IsPlaceholder bool      `json:"is_placeholder"`
	Regencies     []Regency `json:"regencies,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ZoneInput struct {
	ID        string `json:"id,omitempty"`
	ProgramID string `json:"program_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type RegencyAssignmentInput struct {
	ProgramID string `json:"program_id"`
	RegencyID string `json:"regency_id"`
	ZoneID    string `json:"zone_id"`
}

type StorageContext struct {
	ProgramID   string      `json:"program_id"`
	ProgramType ProgramType `json:"program_type"`
	ZoneID      string      `json:"zone_id"`
	ZoneName    string      `json:"zone_name"`
	RegencyID   string      `json:"regency_id"`
	RegencyName string      `json:"regency_name"`
}

type ScheduleInput struct {
	ID                             string         `json:"id,omitempty"`
	ProgramID                      string         `json:"program_id"`
	RegencyID                      string         `json:"regency_id"`
	PackageTemplateVersionID       string         `json:"package_template_version_id"`
	DocumentationTemplateVersionID string         `json:"documentation_template_version_id"`
	Name                           string         `json:"name"`
	StartDate                      time.Time      `json:"start_date"`
	EndDate                        time.Time      `json:"end_date"`
	Status                         string         `json:"status"`
	SlotQuota                      *int           `json:"slot_quota,omitempty"`
	ReceiptPolicy                  map[string]any `json:"receipt_policy"`
	SupervisorName                 string         `json:"supervisor_name"`
	Notes                          string         `json:"notes"`
}
