package dcp3

import "errors"

var (
	ErrWorkbookTooLarge = errors.New("DCP3 workbook exceeds the file size limit")
	ErrTooManyRows      = errors.New("DCP3 workbook exceeds the row limit")
	ErrTooManyColumns   = errors.New("DCP3 workbook exceeds the column limit")
	ErrHeadersInvalid   = errors.New("DCP3 workbook headers are invalid")
	ErrWorkbookInvalid  = errors.New("DCP3 workbook is invalid")
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
