package dcp3

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

func ParseWorkbook(source io.Reader, limits ParseLimits) (WorkbookPreview, error) {
	if limits.MaxBytes <= 0 || limits.MaxRows <= 0 || limits.MaxColumns <= 0 {
		return WorkbookPreview{}, ErrWorkbookInvalid
	}
	data, err := io.ReadAll(io.LimitReader(source, limits.MaxBytes+1))
	if err != nil {
		return WorkbookPreview{}, fmt.Errorf("read DCP3 workbook: %w", err)
	}
	if int64(len(data)) > limits.MaxBytes {
		return WorkbookPreview{}, ErrWorkbookTooLarge
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return WorkbookPreview{}, ErrWorkbookInvalid
	}
	defer func() { _ = file.Close() }()

	sheetName := ""
	for _, candidate := range file.GetSheetList() {
		visible, visibilityErr := file.GetSheetVisible(candidate)
		if visibilityErr != nil {
			return WorkbookPreview{}, ErrWorkbookInvalid
		}
		if visible {
			sheetName = candidate
			break
		}
	}
	if sheetName == "" {
		return WorkbookPreview{}, ErrWorkbookInvalid
	}

	rows, err := file.GetRows(sheetName, excelize.Options{RawCellValue: true})
	if err != nil || len(rows) == 0 {
		return WorkbookPreview{}, ErrWorkbookInvalid
	}
	if len(rows)-1 > limits.MaxRows {
		return WorkbookPreview{}, ErrTooManyRows
	}
	for _, row := range rows {
		if len(row) > limits.MaxColumns {
			return WorkbookPreview{}, ErrTooManyColumns
		}
	}

	headers := make([]string, len(rows[0]))
	seen := make(map[string]struct{}, len(headers))
	for index, header := range rows[0] {
		header = strings.TrimSpace(header)
		key := strings.ToLower(header)
		if header == "" {
			return WorkbookPreview{}, ErrHeadersInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return WorkbookPreview{}, ErrHeadersInvalid
		}
		seen[key] = struct{}{}
		headers[index] = header
	}
	if len(headers) == 0 {
		return WorkbookPreview{}, ErrHeadersInvalid
	}

	preview := WorkbookPreview{SheetName: sheetName, Headers: headers, Rows: []PreviewRow{}}
	for rowIndex := 1; rowIndex < len(rows); rowIndex++ {
		values := make([]string, len(headers))
		for columnIndex := range headers {
			if columnIndex < len(rows[rowIndex]) {
				values[columnIndex] = strings.TrimSpace(rows[rowIndex][columnIndex])
			}
			cell, coordinateErr := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if coordinateErr != nil {
				return WorkbookPreview{}, ErrWorkbookInvalid
			}
			formula, formulaErr := file.GetCellFormula(sheetName, cell)
			if formulaErr != nil {
				return WorkbookPreview{}, ErrWorkbookInvalid
			}
			if formula != "" {
				values[columnIndex] = "=" + formula
			}
		}
		if allBlank(values) {
			continue
		}
		preview.Rows = append(preview.Rows, PreviewRow{SourceRowNumber: rowIndex + 1, Values: values})
	}
	return preview, nil
}

func allBlank(values []string) bool {
	for _, value := range values {
		if value != "" {
			return false
		}
	}
	return true
}
