package dcp3

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

func ParseWorkbook(source io.Reader, limits ParseLimits, headerRowIndex int) (WorkbookPreview, error) {
	file, sheetName, rows, err := openWorkbook(source, limits)
	if err != nil {
		return WorkbookPreview{}, err
	}
	defer func() { _ = file.Close() }()

	if headerRowIndex < 0 || headerRowIndex >= len(rows) {
		return WorkbookPreview{}, ErrHeadersInvalid
	}

	headers := make([]string, len(rows[headerRowIndex]))
	seen := make(map[string]struct{}, len(headers))
	for index, header := range rows[headerRowIndex] {
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
	for rowIndex := headerRowIndex + 1; rowIndex < len(rows); rowIndex++ {
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

// RawRows returns up to maxRows raw rows of the workbook's first visible sheet,
// with no header validation — used to let the uploader pick which row is the
// real header row before ParseWorkbook is called with that index.
func RawRows(source io.Reader, limits ParseLimits, maxRows int) ([][]string, error) {
	file, _, rows, err := openWorkbook(source, limits)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if maxRows >= 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows, nil
}

func openWorkbook(source io.Reader, limits ParseLimits) (*excelize.File, string, [][]string, error) {
	if limits.MaxBytes <= 0 || limits.MaxRows <= 0 || limits.MaxColumns <= 0 {
		return nil, "", nil, ErrWorkbookInvalid
	}
	data, err := io.ReadAll(io.LimitReader(source, limits.MaxBytes+1))
	if err != nil {
		return nil, "", nil, fmt.Errorf("read DCP3 workbook: %w", err)
	}
	if int64(len(data)) > limits.MaxBytes {
		return nil, "", nil, ErrWorkbookTooLarge
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, "", nil, ErrWorkbookInvalid
	}

	sheetName := ""
	for _, candidate := range file.GetSheetList() {
		visible, visibilityErr := file.GetSheetVisible(candidate)
		if visibilityErr != nil {
			_ = file.Close()
			return nil, "", nil, ErrWorkbookInvalid
		}
		if visible {
			sheetName = candidate
			break
		}
	}
	if sheetName == "" {
		_ = file.Close()
		return nil, "", nil, ErrWorkbookInvalid
	}

	rows, err := file.GetRows(sheetName, excelize.Options{RawCellValue: true})
	if err != nil || len(rows) == 0 {
		_ = file.Close()
		return nil, "", nil, ErrWorkbookInvalid
	}
	if len(rows)-1 > limits.MaxRows {
		_ = file.Close()
		return nil, "", nil, ErrTooManyRows
	}
	for _, row := range rows {
		if len(row) > limits.MaxColumns {
			_ = file.Close()
			return nil, "", nil, ErrTooManyColumns
		}
	}
	return file, sheetName, rows, nil
}

func allBlank(values []string) bool {
	for _, value := range values {
		if value != "" {
			return false
		}
	}
	return true
}
