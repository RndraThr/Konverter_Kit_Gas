package recipients

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("recipient not found")
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrFullNameRequired = errors.New("full name is required")
	ErrNIKInvalid       = errors.New("nik must be exactly 16 digits")
	ErrAlreadyCancelled = errors.New("recipient is already cancelled")
	ErrNotCancelled     = errors.New("recipient is not cancelled")
)

type Recipient struct {
	AllocationID         string    `json:"allocation_id"`
	DistributionNumber   int       `json:"distribution_number"`
	AllocationStatus     string    `json:"allocation_status"`
	DistributionStatus   *string   `json:"distribution_status"`
	FullName             string    `json:"full_name"`
	NIK                  string    `json:"nik"`
	SectorIdentifierType string    `json:"sector_identifier_type"`
	SectorIdentifier     string    `json:"sector_identifier"`
	Address              string    `json:"address"`
	Village              string    `json:"village"`
	District             string    `json:"district"`
	PhoneNumber          string    `json:"phone_number"`
	ProgramID            string    `json:"program_id"`
	ProgramName          string    `json:"program_name"`
	ProgramType          string    `json:"program_type"`
	RegencyID            string    `json:"regency_id"`
	RegencyName          string    `json:"regency_name"`
	RegencyDocumentCode  string    `json:"regency_document_code"`
	ScheduleID           string    `json:"schedule_id"`
	ScheduleName         string    `json:"schedule_name"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Filter struct {
	Page               int
	PageSize           int
	Search             string
	RegencyID          string
	ProgramID          string
	ProgramType        string
	AllocationStatus   string
	DistributionStatus string
}

type Page struct {
	Items    []Recipient `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}

type Stats struct {
	Total              int64            `json:"total"`
	ByAllocationStatus map[string]int64 `json:"by_allocation_status"`
}

type CreateInput struct {
	ScheduleID       string `json:"schedule_id"`
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	SectorIdentifier string `json:"sector_identifier"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
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
