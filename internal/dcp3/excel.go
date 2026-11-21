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

	// A header cell left blank (an empty first column, or a gap between two
	// titled columns) is skipped when its column holds no data, and named after
	// its column letter ("Kolom D") when it does, so such files still import.
	var headers []string
	var columns []int
	seen := map[string]struct{}{}
	// Blank cells at the end of the header row are not returned, so a data
	// column after the last title is found through the widest row.
	width := len(rows[headerRowIndex])
	for _, row := range rows[headerRowIndex+1:] {
		width = max(width, len(row))
	}
	for index := range width {
		header := ""
		if index < len(rows[headerRowIndex]) {
			header = strings.TrimSpace(rows[headerRowIndex][index])
		}
		if header == "" {
			if !columnHasData(rows[headerRowIndex+1:], index) {
				continue
			}
			name, err := excelize.ColumnNumberToName(index + 1)
			if err != nil {
				return WorkbookPreview{}, ErrWorkbookInvalid
			}
			header = "Kolom " + name
		}
		key := strings.ToLower(header)
		if _, duplicate := seen[key]; duplicate {
			return WorkbookPreview{}, ErrHeadersInvalid
		}
		seen[key] = struct{}{}
		headers = append(headers, header)
		columns = append(columns, index)
	}
	if len(headers) == 0 {
		return WorkbookPreview{}, ErrHeadersInvalid
	}

	preview := WorkbookPreview{SheetName: sheetName, Headers: headers, Rows: []PreviewRow{}}
	for rowIndex := headerRowIndex + 1; rowIndex < len(rows); rowIndex++ {
		values := make([]string, len(headers))
		for position, columnIndex := range columns {
			if columnIndex < len(rows[rowIndex]) {
				values[position] = strings.TrimSpace(rows[rowIndex][columnIndex])
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
				if literal, ok := formulaStringLiteral(formula); ok {
					values[position] = strings.TrimSpace(literal)
				} else {
					values[position] = "=" + formula
				}
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
	// A spreadsheet row with no cells ever set comes back from excelize as a nil
	// []string, which encoding/json marshals as `null` instead of `[]` — normalize
	// it so every row is safe for frontend code to call .filter()/.map() on.
	normalized := make([][]string, len(rows))
	for index, row := range rows {
		if row == nil {
			row = []string{}
		}
		normalized[index] = row
	}
	return normalized, nil
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

// columnHasData reports whether any of rows has a value in column index.
func columnHasData(rows [][]string, index int) bool {
	for _, row := range rows {
		if index < len(row) && strings.TrimSpace(row[index]) != "" {
			return true
		}
	}
	return false
}

func allBlank(values []string) bool {
	for _, value := range values {
		if value != "" {
			return false
		}
	}
	return true
}

// formulaStringLiteral unwraps a formula that is only a quoted text, such as
// ="6032988517402660" (a common way to keep long numbers as text in Excel).
// Its value is the text itself, so it is safe to import; any other formula is
// kept as written rather than trusting the cached result.
func formulaStringLiteral(formula string) (string, bool) {
	formula = strings.TrimSpace(formula)
	if len(formula) < 2 || formula[0] != '"' || formula[len(formula)-1] != '"' {
		return "", false
	}
	inner := formula[1 : len(formula)-1]
	if strings.Count(inner, `"`) != 2*strings.Count(inner, `""`) {
		return "", false
	}
	return strings.ReplaceAll(inner, `""`, `"`), true
}
