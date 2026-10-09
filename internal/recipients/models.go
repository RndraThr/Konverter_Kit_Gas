package recipients

import (
	"errors"
	"time"
)

var (
	ErrNotFound              = errors.New("recipient not found")
	ErrScheduleNotFound      = errors.New("schedule not found")
	ErrFullNameRequired      = errors.New("full name is required")
	ErrNIKInvalid            = errors.New("nik must be exactly 16 digits")
	ErrNIKInUse              = errors.New("nik is already registered to another person")
	ErrSectorIdentifierInUse = errors.New("sector identifier is already registered to another person")
	ErrAlreadyCancelled      = errors.New("recipient is already cancelled")
	ErrNotCancelled          = errors.New("recipient is not cancelled")
	ErrCancelNotAllowed      = errors.New("recipient: cannot cancel a completed distribution")
)

type Recipient struct {
	AllocationID         string                `json:"allocation_id"`
	DistributionNumber   *int                  `json:"distribution_number"`
	AllocationStatus     string                `json:"allocation_status"`
	DistributionStatus   *string               `json:"distribution_status"`
	FullName             string                `json:"full_name"`
	NIK                  string                `json:"nik"`
	SectorIdentifierType string                `json:"sector_identifier_type"`
	SectorIdentifier     string                `json:"sector_identifier"`
	Address              string                `json:"address"`
	Village              string                `json:"village"`
	District             string                `json:"district"`
	PhoneNumber          string                `json:"phone_number"`
	ProgramID            string                `json:"program_id"`
	ProgramName          string                `json:"program_name"`
	ProgramType          string                `json:"program_type"`
	ZoneID               string                `json:"zone_id"`
	ZoneCode             string                `json:"zone_code"`
	ZoneName             string                `json:"zone_name"`
	RegencyID            string                `json:"regency_id"`
	RegencyName          string                `json:"regency_name"`
	RegencyDocumentCode  string                `json:"regency_document_code"`
	ScheduleID           string                `json:"schedule_id"`
	ScheduleName         string                `json:"schedule_name"`
	EvidenceSlots        []EvidenceSlotSummary `json:"evidence_slots"`
	ReplacedBy           *ReplacementSummary   `json:"replaced_by,omitempty"`
	Replaces             *ReplacementSummary   `json:"replaces,omitempty"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

type EvidenceSlotSummary struct {
	SlotCode      string `json:"slot_code"`
	Label         string `json:"label"`
	IsRequired    bool   `json:"is_required"`
	MinFiles      int    `json:"min_files"`
	AcceptedFiles int    `json:"accepted_files"`
	Complete      bool   `json:"complete"`
}

// ReplacementSummary is the condensed replacement counterpart attached to a recipient row, so Data
// Penerima can show "digantikan oleh" and "menggantikan" without loading the full history.
type ReplacementSummary struct {
	FullName   string    `json:"full_name"`
	NIK        string    `json:"nik,omitempty"`
	Reason     string    `json:"reason"`
	ReplacedAt time.Time `json:"replaced_at"`
}

type Filter struct {
	Page               int
	PageSize           int
	All                bool
	Search             string
	RegencyID          string
	ProgramID          string
	ZoneID             string
	ProgramType        string
	AllocationStatus   string
	DistributionStatus string
	ScheduleID         string
	District           string
	EvidenceStatus     string
	SortBy             string
	SortDirection      string
}

type Page struct {
	Items    []Recipient `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	All      bool        `json:"all"`
	Total    int64       `json:"total"`
}

type Stats struct {
	Total              int64            `json:"total"`
	ByAllocationStatus map[string]int64 `json:"by_allocation_status"`
	ByEvidenceStatus   map[string]int64 `json:"by_evidence_status"`
}

// MapRegion is one kabupaten/kota aggregate for the distribution map. Recipient and evidence
// buckets come from the same query the Data Penerima list uses, so the map cannot disagree with the
// list for the same filter. Slot counters describe distribution progress, which recipient rows
// alone cannot express because an open slot has no allocation yet.
type MapRegion struct {
	RegencyID    string `json:"regency_id"`
	RegencyName  string `json:"regency_name"`
	ProvinceName string `json:"province_name"`
	DocumentCode string `json:"document_code"`

	Recipients  int64 `json:"recipients"`
	Candidate   int64 `json:"candidate"`
	Ready       int64 `json:"ready"`
	Distributed int64 `json:"distributed"`
	NeedsReview int64 `json:"needs_review"`
	Replaced    int64 `json:"replaced"`

	EvidenceComplete      int64 `json:"evidence_complete"`
	EvidencePartial       int64 `json:"evidence_partial"`
	EvidenceEmpty         int64 `json:"evidence_empty"`
	EvidenceNotConfigured int64 `json:"evidence_not_configured"`

	SlotQuota      int64 `json:"slot_quota"`
	SlotsOpen      int64 `json:"slots_open"`
	SlotsLinked    int64 `json:"slots_linked"`
	SlotsCompleted int64 `json:"slots_completed"`
	SlotsCancelled int64 `json:"slots_cancelled"`
}

// MapData carries the per-regency rows plus the summed totals that the legend and metric scales need.
type MapData struct {
	Regions []MapRegion `json:"regions"`
	Totals  MapRegion   `json:"totals"`
}

type CreateInput struct {
	ScheduleID        string `json:"schedule_id"`
	FullName          string `json:"full_name"`
	NIK               string `json:"nik"`
	SectorIdentifier  string `json:"sector_identifier"`
	Address           string `json:"address"`
	Village           string `json:"village"`
	District          string `json:"district"`
	PhoneNumber       string `json:"phone_number"`
	MachineOptionCode string `json:"machine_option_code"`
}

type UpdateInput struct {
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	SectorIdentifier string `json:"sector_identifier"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
}
