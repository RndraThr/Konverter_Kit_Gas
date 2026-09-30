package bast

import (
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound            = errors.New("bast resource not found")
	ErrInvalidInput        = errors.New("bast input is invalid")
	ErrFinalTotalMissing   = errors.New("final regency total has not been locked")
	ErrFinalTotalLocked    = errors.New("final regency total cannot change after documents are finalized")
	ErrSlotOutOfRange      = errors.New("distribution slot number exceeds the final regency total")
	ErrZoneNotConfigured   = errors.New("regency has not been assigned to a configured zone")
	ErrProfileNotPublished = errors.New("a published document profile is required")
	ErrTemplateUnavailable = errors.New("individual handover template is not available for this program type")
	ErrNoRecipients        = errors.New("no completed recipients are available for this date")
	ErrBundleConflict      = errors.New("daily bundle was changed by another operator")
)

type DailyBundle struct {
	ID             string     `json:"id"`
	ProgramID      string     `json:"program_id"`
	RegencyID      string     `json:"regency_id"`
	LocalDate      string     `json:"local_date"`
	Filename       string     `json:"filename"`
	RecipientCount int        `json:"recipient_count"`
	PageCount      int        `json:"page_count"`
	Version        int        `json:"version"`
	Status         string     `json:"status"`
	Checksum       string     `json:"checksum"`
	StorageKey     string     `json:"-"`
	LastError      string     `json:"last_error,omitempty"`
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
}

type BundleRequest struct {
	ProgramID string `json:"program_id"`
	RegencyID string `json:"regency_id"`
	LocalDate string `json:"local_date"`
}

type BundlePreview struct {
	PDF            []byte `json:"-"`
	Filename       string `json:"filename"`
	RecipientCount int    `json:"recipient_count"`
	PageCount      int    `json:"page_count"`
	Checksum       string `json:"checksum"`
}

type BundleContent struct {
	Reader   io.ReadCloser
	Filename string
}

type BundleItemActivation struct {
	IndividualDocumentID string
	SlotNumber           int
	PageStart            int
	PageEnd              int
}

type BundleActivation struct {
	ProgramID        string
	RegencyID        string
	LocalDate        string
	ProfileVersionID string
	Filename         string
	PageCount        int
	Checksum         string
	StorageKey       string
	ExpectedActiveID string
	Items            []BundleItemActivation
}

type BundleActivationResult struct {
	Bundle        DailyBundle
	OldStorageKey string
	Unchanged     bool
}

type DateSummary struct {
	LocalDate        string       `json:"local_date"`
	RecipientCount   int          `json:"recipient_count"`
	ValidationStatus string       `json:"validation_status"`
	Bundle           *DailyBundle `json:"bundle,omitempty"`
}

type LogoSnapshot struct {
	AssetID     string  `json:"asset_id"`
	StorageKey  string  `json:"storage_key"`
	MimeType    string  `json:"mime_type"`
	SortOrder   int     `json:"sort_order"`
	MaxWidthMM  float64 `json:"max_width_mm"`
	MaxHeightMM float64 `json:"max_height_mm"`
}
type ProfileSnapshot struct {
	VersionID              string         `json:"version_id"`
	Title                  string         `json:"title"`
	Subtitle               string         `json:"subtitle"`
	ProcurementDescription string         `json:"procurement_description"`
	DocumentSeries         string         `json:"document_series"`
	Logos                  []LogoSnapshot `json:"logos"`
}
type RecipientSnapshot struct {
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	SectorIdentifier string `json:"sector_identifier"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	Regency          string `json:"regency"`
	PhoneNumber      string `json:"phone_number"`
}
type EquipmentSnapshot struct {
	MachineBrand    string `json:"machine_brand"`
	MachineType     string `json:"machine_type"`
	MachineSerial   string `json:"machine_serial"`
	HoseBrand       string `json:"hose_brand"`
	HoseSpec        string `json:"hose_spec"`
	HoseSerial      string `json:"hose_serial"`
	ConverterBrand  string `json:"converter_brand"`
	ConverterSerial string `json:"converter_serial"`
}
type ComponentSnapshot struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Quantity int    `json:"quantity"`
	Unit     string `json:"unit"`
	Checked  bool   `json:"checked"`
}
type SignatureSnapshot struct {
	ReceiverName   string `json:"receiver_name"`
	ExecutorName   string `json:"executor_name"`
	SupervisorName string `json:"supervisor_name"`
}
type Snapshot struct {
	ProgramType    string              `json:"program_type"`
	DocumentNumber string              `json:"document_number"`
	LocalDate      string              `json:"local_date"`
	Profile        ProfileSnapshot     `json:"profile"`
	Recipient      RecipientSnapshot   `json:"recipient"`
	Equipment      EquipmentSnapshot   `json:"equipment"`
	Components     []ComponentSnapshot `json:"components"`
	Signatures     SignatureSnapshot   `json:"signatures"`
}

type SourceData struct {
	ProgramID                string
	RegencyID                string
	ProgramType              string
	DocumentNumber           string
	LocalDate                string
	Profile                  ProfileSnapshot
	Recipient                RecipientSnapshot
	Equipment                EquipmentSnapshot
	Components               []ComponentSnapshot
	ExecutorName             string
	SupervisorName           string
	PackageTemplateVersionID string
}

type IndividualDocument struct {
	ID                       string    `json:"id"`
	DistributionSlotID       string    `json:"distribution_slot_id"`
	ProgramID                string    `json:"program_id"`
	RegencyID                string    `json:"regency_id"`
	DocumentNumber           string    `json:"document_number"`
	LocalDate                string    `json:"local_date"`
	SlotNumber               int       `json:"slot_number"`
	FinalTotal               int       `json:"final_total"`
	ProfileVersionID         string    `json:"profile_version_id"`
	PackageTemplateVersionID string    `json:"package_template_version_id"`
	Revision                 int       `json:"revision"`
	Status                   string    `json:"status"`
	Snapshot                 Snapshot  `json:"snapshot"`
	FinalizedAt              time.Time `json:"finalized_at"`
}

type RecipientDocument struct {
	DistributionSlotID string   `json:"distribution_slot_id"`
	SlotNumber         int      `json:"slot_number"`
	FinalTotal         int      `json:"final_total"`
	Padding            int      `json:"padding"`
	DocumentNumber     string   `json:"document_number"`
	LocalDate          string   `json:"local_date"`
	Snapshot           Snapshot `json:"snapshot"`
}

type LockResult struct {
	ProgramID  string    `json:"program_id"`
	RegencyID  string    `json:"regency_id"`
	FinalTotal int       `json:"final_total"`
	LockedAt   time.Time `json:"locked_at"`
}

type NumberInput struct {
	SlotNumber     int
	FinalTotal     int
	Padding        int
	DocumentSeries string
	RegencyCode    string
	LocalDate      time.Time
}

type SourceContext struct {
	ProgramID        string
	RegencyID        string
	ProgramType      string
	RegencyCode      string
	RegencyName      string
	ZoneName         string
	ZonePlaceholder  bool
	Padding          int
	SlotQuota        int
	ProfileVersionID string
	DocumentSeries   string
}

type CompletedSlot struct {
	ID            string
	SlotNumber    int
	DistributedAt time.Time
}
