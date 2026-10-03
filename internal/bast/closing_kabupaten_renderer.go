package bast

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
)

const (
	closingKabupatenTitle    = "BERITA ACARA SERAH TERIMA"
	closingKabupatenSubtitle = "(FORM REKAPITULASI CLOSING KABUPATEN / KOTA)"
)

// Kolom: No, Lokasi/Titik Serah, Merek Mesin, Tipe Mesin (Per-Varian), Jumlah Paket (set), Keterangan.
// Jumlah lebar harus persis = pageWidthMM-2*marginMM (182mm).
var closingKabupatenColumnWidths = []float64{12, 38, 30, 40, 28, 34}

func RenderClosingKabupaten(snapshot ClosingKabupatenSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentClosingKabupaten {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseClosingKabupatenSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, marginMM)
	pdf.SetTitle("CLOSING KABUPATEN "+snapshot.DocumentDate, false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	pdf.AddPage()
	renderClosingKabupatenHeader(pdf, snapshot, logos)
	renderClosingKabupatenTable(pdf, snapshot, logos)

	pdf.AddPage()
	renderClosingKabupatenHeader(pdf, snapshot, logos)
	renderClosingKabupatenSignatures(pdf, snapshot)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderClosingKabupatenHeader(pdf *fpdf.Fpdf, snapshot ClosingKabupatenSnapshot, logos []registeredLogo) {
	contentWidth := pageWidthMM - 2*marginMM
	gap, stripY, stripHeight := 5.8, topMarginMM, 17.5
	widths := make([]float64, len(logos))
	heights := make([]float64, len(logos))
	for i, logo := range logos {
		maxWidth := logo.maxWidthMM
		if maxWidth <= 0 {
			maxWidth = 42
		}
		maxHeight := logo.maxHeightMM
		if maxHeight <= 0 {
			maxHeight = 16
		}
		scale := minFloat(maxWidth/logo.widthMM, maxHeight/logo.heightMM)
		widths[i], heights[i] = logo.widthMM*scale, logo.heightMM*scale
	}
	totalWidth := gap * float64(maxInt(0, len(logos)-1))
	for _, width := range widths {
		totalWidth += width
	}
	x := marginMM + (contentWidth-totalWidth)/2
	for i, logo := range logos {
		pdf.ImageOptions(logo.name, x, stripY+(stripHeight-heights[i])/2, widths[i], heights[i], false, logo.options, 0, "")
		x += widths[i] + gap
	}
	pdf.SetXY(marginMM, 33.8)
	pdf.SetFont("Helvetica", "BU", 12.5)
	pdf.CellFormat(contentWidth, 5.6, closingKabupatenTitle, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BI", 9)
	pdf.CellFormat(contentWidth, 4.2, closingKabupatenSubtitle, "", 1, "C", false, 0, "")
	pdf.Ln(1.4)

	renderProcurementHeading(pdf, marginMM, contentWidth, snapshot.FiscalYear)
	pdf.Ln(1.6)

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetX(marginMM)
	pdf.CellFormat(contentWidth, 4.6, "No. "+snapshot.DocumentNumber, "", 1, "C", false, 0, "")
	pdf.Ln(1.2)

	metadata := []struct{ label, value string }{
		{"Kabupaten / Kota", snapshot.RegencyName},
		{"Provinsi", snapshot.ProvinceName},
	}
	for _, item := range metadata {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetX(marginMM)
		pdf.CellFormat(44, infoRowMM, item.label, "", 0, "L", false, 0, "")
		pdf.CellFormat(3.7, infoRowMM, ":", "", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(contentWidth-47.7, infoRowMM, item.value, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.6)

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetX(marginMM)
	pdf.MultiCell(contentWidth, 4.1, closingNarrative(snapshot.DocumentDate, snapshot.FiscalYear), "", "J", false)
	pdf.Ln(1.4)
}

func renderClosingKabupatenTable(pdf *fpdf.Fpdf, snapshot ClosingKabupatenSnapshot, logos []registeredLogo) {
	headers := []string{"No.", "Lokasi / Titik Serah", "Merek Mesin", "Tipe Mesin\n(Per-Varian)", "Jumlah Paket\n(set)", "Keterangan"}
	renderClosingKabupatenTableHeader(pdf, headers)
	for i, row := range snapshot.Rows {
		cells := closingKabupatenRowCells(i+1, row)
		height := closingKabupatenRowHeight(pdf, cells)
		if pdf.GetY()+height > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			renderClosingKabupatenHeader(pdf, snapshot, logos)
			renderClosingKabupatenTableHeader(pdf, headers)
		}
		drawClosingKabupatenRow(pdf, cells, height)
	}
	totalHeight := minRowMM
	if pdf.GetY()+totalHeight > pageHeightMM-bottomMarginMM {
		pdf.AddPage()
		renderClosingKabupatenHeader(pdf, snapshot, logos)
		renderClosingKabupatenTableHeader(pdf, headers)
	}
	drawClosingKabupatenTotalRow(pdf, snapshot.GrandTotal)
}

func renderClosingKabupatenTableHeader(pdf *fpdf.Fpdf, headers []string) {
	x, y := marginMM, pdf.GetY()
	height := minRowMM + 1.5
	for i, header := range headers {
		drawCell(pdf, x, y, closingKabupatenColumnWidths[i], height, header, "C", true)
		x += closingKabupatenColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+height)
}

func closingKabupatenRowCells(number int, row ClosingKabupatenRow) []string {
	return []string{
		fmt.Sprintf("%d.", number),
		row.Location,
		row.MachineBrand,
		row.MachineType,
		fmt.Sprintf("%d", row.Count),
		"",
	}
}

func closingKabupatenRowHeight(pdf *fpdf.Fpdf, cells []string) float64 {
	height := minRowMM
	pdf.SetFont("Helvetica", "", tableFontPt)
	for i, value := range cells {
		lines := len(pdf.SplitLines([]byte(value), closingKabupatenColumnWidths[i]-2*cellPadMM))
		height = maxFloat(height, float64(lines)*tableLineMM+2)
	}
	return height
}

func drawClosingKabupatenRow(pdf *fpdf.Fpdf, cells []string, height float64) {
	x, y := marginMM, pdf.GetY()
	aligns := []string{"C", "C", "C", "C", "C", "L"}
	for i, value := range cells {
		drawCell(pdf, x, y, closingKabupatenColumnWidths[i], height, value, aligns[i], false)
		x += closingKabupatenColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+height)
}

// drawClosingKabupatenTotalRow menggambar baris "Jumlah/Total": empat kolom
// pertama digabung menjadi satu label, kolom Jumlah Paket diisi total,
// Keterangan kosong.
func drawClosingKabupatenTotalRow(pdf *fpdf.Fpdf, total int) {
	x, y := marginMM, pdf.GetY()
	mergedWidth := closingKabupatenColumnWidths[0] + closingKabupatenColumnWidths[1] + closingKabupatenColumnWidths[2] + closingKabupatenColumnWidths[3]
	height := minRowMM
	drawCell(pdf, x, y, mergedWidth, height, "Jumlah/Total", "C", true)
	x += mergedWidth
	drawCell(pdf, x, y, closingKabupatenColumnWidths[4], height, fmt.Sprintf("%d", total), "C", true)
	x += closingKabupatenColumnWidths[4]
	drawCell(pdf, x, y, closingKabupatenColumnWidths[5], height, "", "C", true)
	pdf.SetXY(marginMM, y+height)
}

func renderClosingKabupatenSignatures(pdf *fpdf.Fpdf, snapshot ClosingKabupatenSnapshot) {
	contentWidth := pageWidthMM - 2*marginMM
	sigTop := pageHeightMM - bottomMarginMM - 52
	if pdf.GetY() < sigTop {
		pdf.SetY(sigTop)
	}
	widths := []float64{contentWidth / 4, contentWidth / 4, contentWidth / 4, contentWidth / 4}
	headers := []string{"DINAS PERTANIAN DAERAH", "PELAKSANA PEMASANGAN\nDAN PENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS", "PT PERTAMINA\nPATRA NIAGA"}
	names := []string{snapshot.Signatories.AgricultureOfficeName, snapshot.Signatories.InstallerName, snapshot.Signatories.SupervisorName, snapshot.Signatories.PertaminaRepName}
	nips := []string{snapshot.Signatories.AgricultureOfficeNIP, "", "", ""}

	x, y := marginMM, pdf.GetY()
	headerHeight := 14.0
	for i := range widths {
		drawCell(pdf, x, y, widths[i], headerHeight, headers[i], "C", true)
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight)

	x = marginMM
	spaceHeight := 24.0
	for i := range widths {
		drawCell(pdf, x, y+headerHeight, widths[i], spaceHeight, "", "C", false)
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight+spaceHeight)

	x = marginMM
	for i := range widths {
		dp3FitText(pdf, x, y+headerHeight+spaceHeight, widths[i], 6.5, "Nama : "+trimSpaceOrEmpty(names[i]))
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight+spaceHeight+6.5)

	x = marginMM
	for i := range widths {
		nipText := ""
		if trimSpaceOrEmpty(nips[i]) != "" {
			nipText = "NIP : " + trimSpaceOrEmpty(nips[i])
		}
		dp3FitText(pdf, x, y+headerHeight+spaceHeight+6.5, widths[i], 5.5, nipText)
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight+spaceHeight+12)
}
