package distribution

import (
	"errors"
	"io"
	"time"
)

var (
	ErrScheduleRequired         = errors.New("distribution schedule is required")
	ErrQueryRequired            = errors.New("recipient search query is required")
	ErrNIKInvalid               = errors.New("NIK must contain 16 digits")
	ErrIdentifierConflict       = errors.New("recipient identifier is already in use")
	ErrMediaUnavailable         = errors.New("media storage is unavailable")
	ErrMediaNotFound            = errors.New("documentation media not found")
	ErrMediaTypeInvalid         = errors.New("documentation file must be JPEG, PNG, WebP, MP4, WebM, or MOV")
	ErrMediaPolicyInvalid       = errors.New("documentation media type is not allowed for this slot")
	ErrMediaTooLarge            = errors.New("documentation file exceeds the allowed size")
	ErrVideoUploadBusy          = errors.New("video upload capacity is currently full")
	ErrMediaSourceInvalid       = errors.New("documentation source is not allowed for this slot")
	ErrMediaLocationRequired    = errors.New("documentation location is required")
	ErrMediaCapturedAtRequired  = errors.New("documentation capture time is required")
	ErrMediaLimitReached        = errors.New("documentation slot has reached its file limit")
	ErrMediaMovePending         = errors.New("documentation media is still being moved")
	ErrMediaMoveFailed          = errors.New("documentation media move failed")
	ErrIdentityIncomplete       = errors.New("recipient identity is incomplete")
	ErrDocumentationIncomplete  = errors.New("required documentation is incomplete")
	ErrEquipmentOptionNotFound  = errors.New("selected equipment option is not available in the schedule package template")
	ErrPreviouslyReceived       = errors.New("recipient has previously received a package")
	ErrAlreadyCompleted         = errors.New("distribution is already completed")
	ErrSlotNotFound             = errors.New("distribution slot not found")
	ErrSlotNotOpen              = errors.New("distribution slot is not open")
	ErrSlotNotLinked            = errors.New("distribution slot is not linked to a recipient")
	ErrCandidateNotFound        = errors.New("no unlinked DCP3 candidate matches this NIK for this schedule")
	ErrSlotNumberRequired       = errors.New("slot_number is required")
	ErrSlotQuotaExceeded        = errors.New("distribution slot quota has been reached for this schedule")
	ErrSlotNumberTaken          = errors.New("distribution slot number is already used for this schedule")
	ErrDistributionDateRequired = errors.New("distribution date is required")
	ErrDistributionDateLocked   = errors.New("distribution date is locked after documentation is uploaded")
	ErrEquipmentLocked          = errors.New("equipment is locked after POS Mesin documentation is uploaded")
	ErrRevisionReasonRequired   = errors.New("distribution revision reason is required")
	ErrRevisionStageInvalid     = errors.New("distribution revision stage is invalid")
	ErrRevisionNotCompleted     = errors.New("only completed distribution slots can be reopened")
	ErrRecipientNotLinked       = errors.New("distribution slot is not linked to a recipient")
)

type SlotSummary struct {
	ID                string      `json:"id,omitempty"`
	Code              string      `json:"code"`
	Label             string      `json:"label"`
	Stage             string      `json:"stage"`
	Status            string      `json:"status"`
	Required          bool        `json:"required,omitempty"`
	MinFiles          int         `json:"min_files,omitempty"`
	MaxFiles          int         `json:"max_files,omitempty"`
	Files             []MediaFile `json:"files,omitempty"`
	InputSource       string      `json:"input_source,omitempty"`
	MediaKind         string      `json:"media_kind,omitempty"`
	RequireLocation   bool        `json:"require_location,omitempty"`
	RequireCapturedAt bool        `json:"require_captured_at,omitempty"`
}

type DistributionSlot struct {
	ID                    string        `json:"id"`
	ScheduleID            string        `json:"schedule_id"`
	SlotNumber            int           `json:"slot_number"`
	DistributionDate      *string       `json:"distribution_date"`
	Status                string        `json:"status"`
	AllocationID          *string       `json:"allocation_id,omitempty"`
	FullName              string        `json:"full_name,omitempty"`
	NIK                   string        `json:"nik,omitempty"`
	SectorIdentifier      string        `json:"sector_identifier,omitempty"`
	Address               string        `json:"address,omitempty"`
	Village               string        `json:"village,omitempty"`
	District              string        `json:"district,omitempty"`
	PhoneNumber           string        `json:"phone_number,omitempty"`
	MachineOptionCode     string        `json:"machine_option_code,omitempty"`
	MachineSerialNumber   string        `json:"machine_serial_number,omitempty"`
	HoseOptionCode        string        `json:"hose_option_code,omitempty"`
	HoseSerialNumber      string        `json:"hose_serial_number,omitempty"`
	ConverterOptionCode   string        `json:"converter_option_code,omitempty"`
	ConverterSerialNumber string        `json:"converter_serial_number,omitempty"`
	Documentation         []SlotSummary `json:"documentation"`
	DistributedAt         *time.Time    `json:"distributed_at,omitempty"`
	NeedsRecompletion     bool          `json:"needs_recompletion"`
	ReopenedAt            *time.Time    `json:"reopened_at,omitempty"`
	ReopenedBy            *string       `json:"reopened_by,omitempty"`
	ReopenedStage         string        `json:"reopened_stage,omitempty"`
	RevisionReason        string        `json:"revision_reason,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

type SlotCatalogEntry struct {
	SlotNumber            int    `json:"slot_number"`
	Status                string `json:"status"`
	DocumentationComplete bool   `json:"documentation_complete"`
	NeedsRecompletion     bool   `json:"needs_recompletion"`
}

type CreateSlotInput struct {
	ScheduleID string `json:"schedule_id"`
	SlotNumber int    `json:"slot_number"`
}

type CandidateMatch struct {
	AllocationID         string `json:"allocation_id"`
	FullName             string `json:"full_name"`
	NIK                  string `json:"nik"`
	SectorIdentifier     string `json:"sector_identifier"`
	SectorIdentifierType string `json:"sector_identifier_type"`
	Address              string `json:"address"`
	Village              string `json:"village"`
	District             string `json:"district"`
	PhoneNumber          string `json:"phone_number"`
	ProgramType          string `json:"program_type"`
}

type LinkSlotInput struct {
	ScheduleID       string `json:"schedule_id"`
	SlotNumber       int    `json:"slot_number"`
	NIK              string `json:"nik"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
	SectorIdentifier string `json:"sector_identifier"`
}

type UpdateRecipientInput struct {
	ScheduleID       string `json:"schedule_id"`
	SlotNumber       int    `json:"slot_number"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
	SectorIdentifier string `json:"sector_identifier"`
}

type ReplaceRecipientInput struct {
	ScheduleID       string `json:"schedule_id"`
	SlotNumber       int    `json:"slot_number"`
	NIK              string `json:"nik"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
	SectorIdentifier string `json:"sector_identifier"`
}

type ReopenSlotInput struct {
	ScheduleID string `json:"schedule_id"`
	SlotNumber int    `json:"slot_number"`
	Stage      string `json:"stage"`
	Reason     string `json:"reason"`
}

type CompleteSlotInput struct {
	ScheduleID string `json:"schedule_id"`
	SlotNumber int    `json:"slot_number"`
}

type SetDistributionDateInput struct {
	ScheduleID       string `json:"schedule_id"`
	SlotNumber       int    `json:"slot_number"`
	DistributionDate string `json:"distribution_date"`
}

type UpdateEquipmentInput struct {
	ScheduleID            string `json:"schedule_id"`
	SlotNumber            int    `json:"slot_number"`
	MachineOptionCode     string `json:"machine_option_code"`
	MachineSerialNumber   string `json:"machine_serial_number"`
	HoseOptionCode        string `json:"hose_option_code"`
	HoseSerialNumber      string `json:"hose_serial_number"`
	ConverterOptionCode   string `json:"converter_option_code"`
	ConverterSerialNumber string `json:"converter_serial_number"`
}

type MediaSlot struct {
	ID                 string
	ScheduleID         string
	SlotNumber         int
	DistributionDate   *string
	HasRecipient       bool
	DistributionStatus string
	Label              string
	ProgramType        string
	ZoneName           string
	RegencyName        string
	InputSource        string
	MediaKind          string
	RequireLocation    bool
	RequireCapturedAt  bool
	MinFiles           int
	MaxFiles           int
	AcceptedFiles      int
}

type UploadMediaInput struct {
	SlotID           string
	OriginalFilename string
	Source           string
	Data             io.Reader
	DeclaredSize     int64
	CapturedAt       *time.Time
	Latitude         *float64
	Longitude        *float64
}

type MediaFileInput struct {
	SlotID, StorageKey, OriginalFilename, MimeType, Checksum, Source string
	ByteSize                                                         int64
	StorageState                                                     string
	CapturedAt                                                       *time.Time
	Latitude, Longitude                                              *float64
}

type MediaFile struct {
	ID               string     `json:"id"`
	SlotID           string     `json:"slot_id"`
	StorageKey       string     `json:"-"`
	OriginalFilename string     `json:"original_filename"`
	MimeType         string     `json:"mime_type"`
	ByteSize         int64      `json:"byte_size"`
	Source           string     `json:"source"`
	CapturedAt       *time.Time `json:"captured_at,omitempty"`
	Latitude         *float64   `json:"latitude,omitempty"`
	Longitude        *float64   `json:"longitude,omitempty"`
	Status           string     `json:"status"`
	StorageState     string     `json:"storage_state"`
	StorageLastError string     `json:"storage_last_error,omitempty"`
	ContentURL       string     `json:"content_url"`
	UploadedAt       time.Time  `json:"uploaded_at"`
}

type MediaContent struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}

type MediaMoveJob struct {
	MediaFileID      string
	StorageKey       string
	TargetPath       []string
	TargetGeneration int64
	Attempts         int
}
