package bast

import (
	"errors"
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
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
}

type DateSummary struct {
	LocalDate        string       `json:"local_date"`
	RecipientCount   int          `json:"recipient_count"`
	ValidationStatus string       `json:"validation_status"`
	Bundle           *DailyBundle `json:"bundle,omitempty"`
}

type Snapshot struct{}

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
