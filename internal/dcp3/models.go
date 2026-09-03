package dcp3

import (
	"errors"
	"time"

	"konkit/internal/programs"
)

var (
	ErrWorkbookTooLarge = errors.New("DCP3 workbook exceeds the file size limit")
	ErrTooManyRows      = errors.New("DCP3 workbook exceeds the row limit")
	ErrTooManyColumns   = errors.New("DCP3 workbook exceeds the column limit")
	ErrHeadersInvalid   = errors.New("DCP3 workbook headers are invalid")
	ErrWorkbookInvalid  = errors.New("DCP3 workbook is invalid")
	ErrMappingInvalid   = errors.New("DCP3 column mapping is invalid")
	ErrPreviewNotFound  = errors.New("DCP3 preview not found")
	ErrDuplicateImport  = errors.New("DCP3 file has already been uploaded for this schedule")
	ErrImportState      = errors.New("DCP3 batch cannot be imported in its current state")
)

type RowStatus string

const (
	RowPending     RowStatus = "pending"
	RowValid       RowStatus = "valid"
	RowWarning     RowStatus = "warning"
	RowNeedsReview RowStatus = "needs_review"
	RowInvalid     RowStatus = "invalid"
)

const (
	IdentifierFarmerCard = "farmer_card"
	IdentifierKUSUKA     = "kusuka"
)

type ParseLimits struct {
	MaxBytes   int64
	MaxRows    int
	MaxColumns int
}

type PreviewRow struct {
	SourceRowNumber int      `json:"source_row_number"`
	Values          []string `json:"values"`
}

type WorkbookPreview struct {
	SheetName string       `json:"sheet_name"`
	Headers   []string     `json:"headers"`
	Rows      []PreviewRow `json:"rows"`
}

type Mapping struct {
	SourceSequence   string `json:"source_sequence"`
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	FarmerCardNumber string `json:"farmer_card_number"`
	KUSUKANumber     string `json:"kusuka_number"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
}

type RawImportRow struct {
	ID              string            `json:"id,omitempty"`
	SourceRowNumber int               `json:"source_row_number"`
	Values          map[string]string `json:"values"`
}

type NormalizedRow struct {
	SourceRowNumber         int               `json:"source_row_number"`
	SourceSequenceNumber    *int              `json:"source_sequence_number,omitempty"`
	FullName                string            `json:"full_name"`
	NIK                     string            `json:"nik,omitempty"`
	IdentifierType          string            `json:"identifier_type,omitempty"`
	SectorIdentifier        string            `json:"sector_identifier,omitempty"`
	SectorIdentifierDisplay string            `json:"sector_identifier_display,omitempty"`
	Address                 string            `json:"address,omitempty"`
	Village                 string            `json:"village,omitempty"`
	District                string            `json:"district,omitempty"`
	PhoneNumber             string            `json:"phone_number,omitempty"`
	ValidationStatus        RowStatus         `json:"validation_status"`
	ValidationMessages      []string          `json:"validation_messages"`
	SourceValues            map[string]string `json:"source_values"`
}

type ImportPreview struct {
	ID               string               `json:"id"`
	ScheduleID       string               `json:"schedule_id"`
	ProgramType      programs.ProgramType `json:"program_type"`
	OriginalFilename string               `json:"original_filename"`
	FileChecksum     string               `json:"file_checksum"`
	SheetName        string               `json:"sheet_name"`
	Headers          []string             `json:"headers"`
	Rows             []RawImportRow       `json:"rows"`
	Status           string               `json:"status"`
	CreatedAt        time.Time            `json:"created_at"`
}

type ImportResult struct {
	BatchID     string `json:"batch_id"`
	TotalRows   int    `json:"total_rows"`
	ValidRows   int    `json:"valid_rows"`
	WarningRows int    `json:"warning_rows"`
	InvalidRows int    `json:"invalid_rows"`
}
