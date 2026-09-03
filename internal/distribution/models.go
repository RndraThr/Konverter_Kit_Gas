package distribution

import (
	"errors"
	"io"
	"time"
)

var (
	ErrScheduleRequired             = errors.New("distribution schedule is required")
	ErrQueryRequired                = errors.New("recipient search query is required")
	ErrQueryTooShort                = errors.New("recipient name search requires at least two characters")
	ErrAllocationNotFound           = errors.New("distribution allocation not found")
	ErrNIKInvalid                   = errors.New("NIK must contain 16 digits")
	ErrIdentityChangeReasonRequired = errors.New("identity change reason is required")
	ErrIdentifierConflict           = errors.New("recipient identifier is already in use")
	ErrMediaUnavailable             = errors.New("media storage is unavailable")
	ErrMediaNotFound                = errors.New("documentation media not found")
	ErrMediaTypeInvalid             = errors.New("documentation file must be JPEG, PNG, or WebP")
	ErrMediaTooLarge                = errors.New("documentation file exceeds 10 MiB")
	ErrMediaSourceInvalid           = errors.New("documentation source is not allowed for this slot")
	ErrMediaLocationRequired        = errors.New("documentation location is required")
	ErrMediaCapturedAtRequired      = errors.New("documentation capture time is required")
	ErrMediaLimitReached            = errors.New("documentation slot has reached its file limit")
)

type SlotSummary struct {
	ID                string      `json:"id,omitempty"`
	Code              string      `json:"code"`
	Label             string      `json:"label"`
	Status            string      `json:"status"`
	Required          bool        `json:"required,omitempty"`
	MinFiles          int         `json:"min_files,omitempty"`
	MaxFiles          int         `json:"max_files,omitempty"`
	Files             []MediaFile `json:"files,omitempty"`
	InputSource       string      `json:"input_source,omitempty"`
	RequireLocation   bool        `json:"require_location,omitempty"`
	RequireCapturedAt bool        `json:"require_captured_at,omitempty"`
}

type SearchRecord struct {
	AllocationID       string
	DistributionNumber int
	FullName           string
	NIK                string
	Location           string
	ProgramType        string
	Eligibility        string
	AllocationStatus   string
	Documentation      []SlotSummary
}

type SearchResult struct {
	AllocationID       string        `json:"allocation_id"`
	DistributionNumber int           `json:"distribution_number"`
	FullName           string        `json:"full_name"`
	MaskedNIK          string        `json:"masked_nik"`
	Location           string        `json:"location"`
	ProgramType        string        `json:"program_type"`
	Eligibility        string        `json:"eligibility"`
	AllocationStatus   string        `json:"allocation_status"`
	Documentation      []SlotSummary `json:"documentation"`
}

type ReceiptHistory struct {
	CompletedAt time.Time `json:"completed_at"`
	Regency     string    `json:"regency"`
	Program     string    `json:"program"`
	BASTNumber  string    `json:"bast_number,omitempty"`
}

type RecipientWorkspace struct {
	AllocationID         string           `json:"allocation_id"`
	DistributionID       string           `json:"distribution_id"`
	ScheduleID           string           `json:"schedule_id"`
	DistributionNumber   int              `json:"distribution_number"`
	AllocationStatus     string           `json:"allocation_status"`
	DistributionStatus   string           `json:"distribution_status"`
	ProgramType          string           `json:"program_type"`
	ProgramName          string           `json:"program_name"`
	RegencyName          string           `json:"regency_name"`
	FullName             string           `json:"full_name"`
	NIK                  string           `json:"nik,omitempty"`
	SectorIdentifier     string           `json:"sector_identifier,omitempty"`
	SectorIdentifierType string           `json:"sector_identifier_type,omitempty"`
	Address              string           `json:"address,omitempty"`
	Village              string           `json:"village,omitempty"`
	District             string           `json:"district,omitempty"`
	PhoneNumber          string           `json:"phone_number,omitempty"`
	Eligibility          string           `json:"eligibility"`
	EligibilityReasons   []string         `json:"eligibility_reasons"`
	SourceSnapshot       map[string]any   `json:"source_snapshot"`
	ReceiptHistory       []ReceiptHistory `json:"receipt_history"`
	Documentation        []SlotSummary    `json:"documentation"`
}

type DraftInput struct {
	NIK                  string `json:"nik"`
	Address              string `json:"address"`
	Village              string `json:"village"`
	District             string `json:"district"`
	PhoneNumber          string `json:"phone_number"`
	SectorIdentifier     string `json:"sector_identifier"`
	IdentityChangeReason string `json:"identity_change_reason"`
}

type MediaSlot struct {
	ID                string
	InputSource       string
	RequireLocation   bool
	RequireCapturedAt bool
	MinFiles          int
	MaxFiles          int
	AcceptedFiles     int
}

type UploadMediaInput struct {
	SlotID           string
	OriginalFilename string
	Source           string
	Data             []byte
	CapturedAt       *time.Time
	Latitude         *float64
	Longitude        *float64
}

type MediaFileInput struct {
	SlotID, StorageKey, OriginalFilename, MimeType, Checksum, Source string
	ByteSize                                                         int64
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
	ContentURL       string     `json:"content_url"`
	UploadedAt       time.Time  `json:"uploaded_at"`
}

type MediaContent struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}
