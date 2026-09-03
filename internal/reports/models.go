package reports

import (
	"errors"
	"time"
)

var (
	ErrScheduleRequired = errors.New("report schedule is required")
	ErrFilterInvalid    = errors.New("report filter is invalid")
)

var allocationStatuses = map[string]bool{
	"candidate": true, "ready": true, "needs_review": true,
	"distributed": true, "replaced": true, "cancelled": true,
}

var distributionStatuses = map[string]bool{
	"draft": true, "completed": true, "cancelled": true,
}

var documentationStatuses = map[string]bool{
	"complete": true, "incomplete": true,
}

type Filter struct {
	AllocationStatus    string
	DistributionStatus  string
	DocumentationStatus string
}

func (f Filter) validate() error {
	if f.AllocationStatus != "" && !allocationStatuses[f.AllocationStatus] {
		return ErrFilterInvalid
	}
	if f.DistributionStatus != "" && !distributionStatuses[f.DistributionStatus] {
		return ErrFilterInvalid
	}
	if f.DocumentationStatus != "" && !documentationStatuses[f.DocumentationStatus] {
		return ErrFilterInvalid
	}
	return nil
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type Summary struct {
	TotalAllocations         int           `json:"total_allocations"`
	AllocationStatusCounts   []StatusCount `json:"allocation_status_counts"`
	DistributionStatusCounts []StatusCount `json:"distribution_status_counts"`
	DocumentationIncomplete  int           `json:"documentation_incomplete"`
}

type Row struct {
	DistributionNumber    int        `json:"distribution_number"`
	FullName              string     `json:"full_name"`
	NIK                   string     `json:"nik"`
	SectorIdentifier      string     `json:"sector_identifier"`
	Village               string     `json:"village"`
	District              string     `json:"district"`
	AllocationStatus      string     `json:"allocation_status"`
	DistributionStatus    string     `json:"distribution_status"`
	DocumentationComplete bool       `json:"documentation_complete"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
}
