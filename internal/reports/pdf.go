package reports

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// PDF Laporan DP3: A4 landscape, judul di halaman pertama, header tabel
// diulang di setiap halaman, baris berselang warna, nomor halaman di kaki.
// Kolom diringkas agar muat satu halaman lebar; data lengkap ada di Excel.

type pdfColumn struct {
	title string
	width float64
	align string
	value func(Row) string
}

func pdfColumns(programType string) []pdfColumn {
	return []pdfColumn{
		{"No", 9, "C", func(r Row) string { return fmt.Sprint(r.RowNumber) }},
		{"No. Bagi", 13, "C", func(r Row) string { return pdfNumber(r.DistributionNumber) }},
		{"Nama Penerima", 36, "L", func(r Row) string { return r.FullName }},
		{"NIK", 29, "L", func(r Row) string { return r.NIK }},
		{cardTitle(programType), 30, "L", func(r Row) string { return r.CardNumber }},
		{"No. HP", 24, "L", func(r Row) string { return r.PhoneNumber }},
		{"Desa/Kel.", 23, "L", func(r Row) string { return r.Village }},
		{"Kecamatan", 23, "L", func(r Row) string { return r.District }},
		{"Tanggal Distribusi", 19, "C", func(r Row) string { return formatDate(r.DistributionDate) }},
		{"Titik Serah", 27, "L", func(r Row) string { return r.HandoverLocation }},
		{"Mesin", 22, "L", func(r Row) string { return strings.TrimSpace(r.MachineBrand + " " + r.MachineType) }},
		{"No. Seri Mesin", 22, "L", func(r Row) string { return r.MachineSerial }},
	}
}

func pdfNumber(value int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprint(value)
}

const (
	pdfMargin     = 10.0
	pdfLineHeight = 3.6
	pdfCellPad    = 1.0
	pdfFontSize   = 7.5
)

func buildPDF(report Report, generatedAt time.Time) ([]byte, error) {
	doc := fpdf.New("L", "mm", "A4", "")
	doc.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	doc.SetAutoPageBreak(false, pdfMargin)
	doc.AliasNbPages("{nb}")
	doc.SetTitle("Laporan DP3", true)
	doc.SetCreator("Konkit", true)
	tr := doc.UnicodeTranslatorFromDescriptor("")
	columns := pdfColumns(report.ProgramType)
	_, pageHeight := doc.GetPageSize()
	footerTop := pageHeight - pdfMargin - 4

	footerLabel := tr("Laporan DP3 · Kabupaten " + strings.TrimSpace(report.RegencyName))
	doc.SetFooterFunc(func() {
		doc.SetY(-pdfMargin - 2)
		doc.SetFont("Helvetica", "", 7)
		doc.SetTextColor(93, 107, 97)
		doc.CellFormat(0, 4, footerLabel, "", 0, "L", false, 0, "")
		doc.SetX(pdfMargin)
		doc.CellFormat(0, 4, fmt.Sprintf("Halaman %d dari {nb}", doc.PageNo()), "", 0, "R", false, 0, "")
	})

	header := func() {
		doc.SetFont("Helvetica", "B", pdfFontSize)
		doc.SetFillColor(23, 61, 43)
		doc.SetTextColor(255, 255, 255)
		doc.SetDrawColor(201, 211, 203)
		height := 0.0
		for _, column := range columns {
			lines := len(doc.SplitText(tr(column.title), column.width-2*pdfCellPad))
			height = max(height, float64(lines)*pdfLineHeight+2*pdfCellPad)
		}
		x, y := pdfMargin, doc.GetY()
		for _, column := range columns {
			doc.Rect(x, y, column.width, height, "FD")
			lines := doc.SplitText(tr(column.title), column.width-2*pdfCellPad)
			top := y + (height-float64(len(lines))*pdfLineHeight)/2
			for index, line := range lines {
				doc.SetXY(x+pdfCellPad, top+float64(index)*pdfLineHeight)
				doc.CellFormat(column.width-2*pdfCellPad, pdfLineHeight, line, "", 0, "C", false, 0, "")
			}
			x += column.width
		}
		doc.SetXY(pdfMargin, y+height)
	}

	doc.AddPage()
	heading := reportHeading(report.ScheduleInfo, len(report.Rows), generatedAt)
	doc.SetTextColor(23, 61, 43)
	doc.SetFont("Helvetica", "B", 14)
	doc.CellFormat(0, 7, tr(heading[0]), "", 1, "L", false, 0, "")
	doc.SetTextColor(31, 43, 35)
	doc.SetFont("Helvetica", "", 10)
	doc.CellFormat(0, 5, tr(heading[1]), "", 1, "L", false, 0, "")
	doc.CellFormat(0, 5, tr(heading[2]), "", 1, "L", false, 0, "")
	doc.SetTextColor(93, 107, 97)
	doc.SetFont("Helvetica", "I", 8.5)
	doc.CellFormat(0, 5, tr(heading[3]), "", 1, "L", false, 0, "")
	doc.Ln(2)
	header()

	if len(report.Rows) == 0 {
		doc.SetFont("Helvetica", "I", 9)
		doc.SetTextColor(93, 107, 97)
		doc.CellFormat(0, 10, tr("Belum ada penerima yang distribusinya selesai."), "1", 1, "C", false, 0, "")
	}
	for index, row := range report.Rows {
		doc.SetFont("Helvetica", "", pdfFontSize)
		cells := make([][]string, len(columns))
		height := 0.0
		for columnIndex, column := range columns {
			text := strings.TrimSpace(column.value(row))
			if text == "" {
				text = "-"
			}
			cells[columnIndex] = doc.SplitText(tr(text), column.width-2*pdfCellPad)
			height = max(height, float64(len(cells[columnIndex]))*pdfLineHeight+2*pdfCellPad)
		}
		if doc.GetY()+height > footerTop {
			doc.AddPage()
			header()
			// header() leaves the bold header font set.
			doc.SetFont("Helvetica", "", pdfFontSize)
		}
		if index%2 == 1 {
			doc.SetFillColor(241, 245, 239)
		} else {
			doc.SetFillColor(255, 255, 255)
		}
		doc.SetTextColor(31, 43, 35)
		doc.SetDrawColor(201, 211, 203)
		x, y := pdfMargin, doc.GetY()
		for columnIndex, column := range columns {
			doc.Rect(x, y, column.width, height, "FD")
			for lineIndex, line := range cells[columnIndex] {
				doc.SetXY(x+pdfCellPad, y+pdfCellPad+float64(lineIndex)*pdfLineHeight)
				doc.CellFormat(column.width-2*pdfCellPad, pdfLineHeight, line, "", 0, column.align, false, 0, "")
			}
			x += column.width
		}
		doc.SetXY(pdfMargin, y+height)
	}

	var buffer bytes.Buffer
	if err := doc.Output(&buffer); err != nil {
		return nil, fmt.Errorf("encode laporan pdf: %w", err)
	}
	return buffer.Bytes(), nil
}
