package distribution

import (
	"errors"
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
)

type SlotSummary struct {
	ID       string `json:"id,omitempty"`
	Code     string `json:"code"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required,omitempty"`
	MinFiles int    `json:"min_files,omitempty"`
	MaxFiles int    `json:"max_files,omitempty"`
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
