package reports

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

const excelSheet = "Laporan DP3"

// excelHeaderRow is the table header; rows above it hold the title block.
const excelHeaderRow = 6

type excelColumn struct {
	title string
	width float64
	kind  excelCellKind
	value func(Row) any
}

type excelCellKind int

const (
	cellPlain excelCellKind = iota
	cellText                // stored as text so leading zeros and long digits survive
	cellNumber
	cellDate
)

func excelColumns(programType string) []excelColumn {
	return []excelColumn{
		{"No", 6, cellNumber, func(r Row) any { return r.RowNumber }},
		{"Nama Penerima", 28, cellPlain, func(r Row) any { return r.FullName }},
		{"NIK", 20, cellText, func(r Row) any { return r.NIK }},
		{cardTitle(programType), 22, cellText, func(r Row) any { return r.CardNumber }},
		{"No. HP", 16, cellText, func(r Row) any { return r.PhoneNumber }},
		{"Alamat", 32, cellPlain, func(r Row) any { return r.Address }},
		{"Desa/Kelurahan", 18, cellPlain, func(r Row) any { return r.Village }},
		{"Kecamatan", 18, cellPlain, func(r Row) any { return r.District }},
		{"Tanggal Distribusi", 14, cellDate, func(r Row) any { return excelDate(r.DistributionDate) }},
		{"Titik Serah", 22, cellPlain, func(r Row) any { return r.HandoverLocation }},
		{"No. Bagi", 9, cellNumber, func(r Row) any { return excelNumber(r.DistributionNumber) }},
		{"Merek Mesin", 14, cellPlain, func(r Row) any { return r.MachineBrand }},
		{"Tipe Mesin", 16, cellPlain, func(r Row) any { return r.MachineType }},
		{"Daya", 10, cellPlain, func(r Row) any { return r.MachinePower }},
		{"BBM", 12, cellPlain, func(r Row) any { return r.MachineFuelType }},
		{"No. Seri Mesin", 22, cellText, func(r Row) any { return r.MachineSerial }},
		{"Merek Converter", 16, cellPlain, func(r Row) any { return r.ConverterBrand }},
		{"No. Seri Converter", 22, cellText, func(r Row) any { return r.ConverterSerial }},
		{"No. Seri Selang", 22, cellText, func(r Row) any { return r.HoseSerial }},
	}
}

func excelDate(value string) any {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return date
}

func excelNumber(value int) any {
	if value <= 0 {
		return ""
	}
	return value
}

func buildExcel(report Report, generatedAt time.Time) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", excelSheet); err != nil {
		return nil, fmt.Errorf("name laporan sheet: %w", err)
	}
	columns := excelColumns(report.ProgramType)
	lastColumn, _ := excelize.ColumnNumberToName(len(columns))
	lastRow := excelHeaderRow + max(len(report.Rows), 1)
	styles, err := newExcelStyles(file)
	if err != nil {
		return nil, err
	}

	heading := reportHeading(report.ScheduleInfo, len(report.Rows), generatedAt)
	headingStyles := []int{styles.title, styles.subtitle, styles.subtitle, styles.note}
	for index, line := range heading {
		cell := fmt.Sprintf("A%d", index+1)
		if err := file.SetCellValue(excelSheet, cell, line); err != nil {
			return nil, fmt.Errorf("write laporan title: %w", err)
		}
		if err := file.MergeCell(excelSheet, cell, fmt.Sprintf("%s%d", lastColumn, index+1)); err != nil {
			return nil, fmt.Errorf("merge laporan title: %w", err)
		}
		_ = file.SetCellStyle(excelSheet, cell, cell, headingStyles[index])
	}
	_ = file.SetRowHeight(excelSheet, 1, 24)

	for index, column := range columns {
		name, _ := excelize.ColumnNumberToName(index + 1)
		if err := file.SetCellValue(excelSheet, fmt.Sprintf("%s%d", name, excelHeaderRow), column.title); err != nil {
			return nil, fmt.Errorf("write laporan header: %w", err)
		}
		_ = file.SetColWidth(excelSheet, name, name, column.width)
	}
	_ = file.SetCellStyle(excelSheet, fmt.Sprintf("A%d", excelHeaderRow), fmt.Sprintf("%s%d", lastColumn, excelHeaderRow), styles.header)
	_ = file.SetRowHeight(excelSheet, excelHeaderRow, 32)

	if len(report.Rows) == 0 {
		cell := fmt.Sprintf("A%d", excelHeaderRow+1)
		_ = file.SetCellValue(excelSheet, cell, "Belum ada penerima yang distribusinya selesai.")
		_ = file.MergeCell(excelSheet, cell, fmt.Sprintf("%s%d", lastColumn, excelHeaderRow+1))
		_ = file.SetCellStyle(excelSheet, cell, cell, styles.empty)
	}
	for rowIndex, row := range report.Rows {
		excelRow := excelHeaderRow + 1 + rowIndex
		band := rowIndex % 2
		for columnIndex, column := range columns {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, excelRow)
			value := column.value(row)
			style := styles.body[band]
			switch column.kind {
			case cellText:
				value, style = fmt.Sprint(value), styles.text[band]
			case cellNumber:
				style = styles.number[band]
			case cellDate:
				style = styles.date[band]
			}
			if err := file.SetCellValue(excelSheet, cell, value); err != nil {
				return nil, fmt.Errorf("write laporan row: %w", err)
			}
			_ = file.SetCellStyle(excelSheet, cell, cell, style)
		}
	}

	if len(report.Rows) > 0 {
		if err := file.AutoFilter(excelSheet, fmt.Sprintf("A%d:%s%d", excelHeaderRow, lastColumn, lastRow), nil); err != nil {
			return nil, fmt.Errorf("filter laporan: %w", err)
		}
	}
	_ = file.SetPanes(excelSheet, &excelize.Panes{
		Freeze: true, XSplit: 2, YSplit: excelHeaderRow,
		TopLeftCell: fmt.Sprintf("C%d", excelHeaderRow+1), ActivePane: "bottomRight",
	})
	landscape, a4, oneWide, anyTall, fit := "landscape", 9, 1, 0, true
	_ = file.SetPageLayout(excelSheet, &excelize.PageLayoutOptions{Orientation: &landscape, Size: &a4, FitToWidth: &oneWide, FitToHeight: &anyTall})
	_ = file.SetSheetProps(excelSheet, &excelize.SheetPropsOptions{FitToPage: &fit})
	_ = file.SetDefinedName(&excelize.DefinedName{
		Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$%d:$%d", excelSheet, excelHeaderRow, excelHeaderRow), Scope: excelSheet,
	})
	_ = file.SetDocProps(&excelize.DocProperties{Title: "Laporan DP3", Creator: "Konkit"})

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode laporan excel: %w", err)
	}
	return buffer.Bytes(), nil
}

type excelStyles struct {
	title, subtitle, note, header, empty int
	// Each body style has a plain and a banded variant.
	body, text, number, date [2]int
}

func newExcelStyles(file *excelize.File) (excelStyles, error) {
	const font = "Calibri"
	border := []excelize.Border{
		{Type: "left", Color: "C9D3CB", Style: 1}, {Type: "right", Color: "C9D3CB", Style: 1},
		{Type: "top", Color: "C9D3CB", Style: 1}, {Type: "bottom", Color: "C9D3CB", Style: 1},
	}
	var styles excelStyles
	var err error
	add := func(style *excelize.Style) int {
		if err != nil {
			return 0
		}
		var id int
		id, err = file.NewStyle(style)
		return id
	}
	styles.title = add(&excelize.Style{Font: &excelize.Font{Family: font, Bold: true, Size: 14, Color: "173D2B"}})
	styles.subtitle = add(&excelize.Style{Font: &excelize.Font{Family: font, Size: 11, Color: "1F2B23"}})
	styles.note = add(&excelize.Style{Font: &excelize.Font{Family: font, Italic: true, Size: 10, Color: "5D6B61"}})
	styles.header = add(&excelize.Style{
		Font:      &excelize.Font{Family: font, Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"173D2B"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    border,
	})
	styles.empty = add(&excelize.Style{Font: &excelize.Font{Family: font, Italic: true, Color: "5D6B61"}, Alignment: &excelize.Alignment{Horizontal: "center"}})
	textFormat, dateFormat := "@", "dd-mm-yyyy"
	for band, fill := range []string{"FFFFFF", "F1F5EF"} {
		base := func(alignment string, format *string) *excelize.Style {
			return &excelize.Style{
				Font:         &excelize.Font{Family: font, Size: 11, Color: "1F2B23"},
				Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}},
				Alignment:    &excelize.Alignment{Horizontal: alignment, Vertical: "top", WrapText: true},
				Border:       border,
				CustomNumFmt: format,
			}
		}
		styles.body[band] = add(base("left", nil))
		styles.text[band] = add(base("left", &textFormat))
		styles.number[band] = add(base("center", nil))
		styles.date[band] = add(base("center", &dateFormat))
	}
	if err != nil {
		return excelStyles{}, fmt.Errorf("style laporan excel: %w", err)
	}
	return styles, nil
}
