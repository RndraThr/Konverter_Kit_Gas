package bast

import (
	"errors"
	"io"
	"time"
)

var (
	ErrAggregateConflict = errors.New("aggregate document was changed by another operator")
	ErrAggregateNotFound = errors.New("aggregate document not found")
)

const (
	AggregateDocumentDP3        = "dp3"
	AggregateDocumentDailyRecap = "daily_recap"
)

// AggregateDocument adalah satu versi dokumen agregat (DP3 atau Rekap Harian).
type AggregateDocument struct {
	ID             string    `json:"id"`
	ScheduleID     string    `json:"schedule_id"`
	ProgramID      string    `json:"program_id"`
	RegencyID      string    `json:"regency_id"`
	DocumentType   string    `json:"document_type"`
	DocumentDate   string    `json:"document_date"`
	Filename       string    `json:"filename"`
	RecipientCount int       `json:"recipient_count"`
	PageCount      int       `json:"page_count"`
	Version        int       `json:"version"`
	Status         string    `json:"status"`
	Checksum       string    `json:"checksum"`
	StorageKey     string    `json:"-"`
	LastError      string    `json:"last_error,omitempty"`
	FinalizedAt    time.Time `json:"finalized_at"`
}

// AggregateActivation membawa hasil render + snapshot siap aktivasi.
type AggregateActivation struct {
	ScheduleID       string
	ProgramID        string
	RegencyID        string
	DocumentType     string
	DocumentDate     string
	Filename         string
	RecipientCount   int
	PageCount        int
	Checksum         string
	StorageKey       string
	Snapshot         []byte
	ExpectedVersion  int
	ExpectedActiveID string
}

type AggregateActivationResult struct {
	Document     AggregateDocument
	OldStorageKey string
	Unchanged    bool
}

type AggregateContent struct {
	Reader   io.ReadCloser
	Filename string
}

// AggregatePreview adalah hasil render preview (belum tersimpan ke storage/DB).
type AggregatePreview struct {
	PDF            []byte `json:"-"`
	Filename       string `json:"filename"`
	RecipientCount int    `json:"recipient_count"`
	PageCount      int    `json:"page_count"`
	Checksum       string `json:"checksum"`
}
