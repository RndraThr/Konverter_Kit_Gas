package bast

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

const (
	dailyRecapTitle    = "BERITA ACARA SERAH TERIMA"
	dailyRecapSubtitle = "(FORM REKAPITULASI PENERIMA PAKET)"
)

var dailyRecapColumnWidths = []float64{14, 44, 36, 28, 30, 26}

func RenderDailyRecap(snapshot DailyRecapSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentDailyRecap {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseDailyRecapSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, marginMM)
	pdf.SetTitle("REKAP HARIAN "+snapshot.DocumentDate, false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	pdf.AddPage()
	renderDailyRecapHeader(pdf, snapshot, logos)
	renderDailyRecapTable(pdf, snapshot, logos)
	renderDailyRecapGrandTotal(pdf, snapshot, logos)

	pdf.AddPage()
	renderDailyRecapHeader(pdf, snapshot, logos)
	renderDailyRecapSignatures(pdf, snapshot)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderDailyRecapHeader(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot, logos []registeredLogo) {
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
	pdf.CellFormat(contentWidth, 5.6, dailyRecapTitle, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BI", 9)
	pdf.CellFormat(contentWidth, 4.2, dailyRecapSubtitle, "", 1, "C", false, 0, "")
	pdf.Ln(1.4)

	renderProcurementHeading(pdf, marginMM, contentWidth, snapshot.FiscalYear)
	pdf.Ln(1.2)

	metadata := []struct{ label, value string }{
		{"Hari / Tanggal", formatIndonesianDate(snapshot.DocumentDate)},
		{"Lokasi / Titik Serah", snapshot.HandoverLocation},
		{"Konsultan Distribusi", snapshot.ConsultantCompanyName},
	}
	for _, item := range metadata {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetX(marginMM)
		pdf.CellFormat(44, infoRowMM, item.label, "", 0, "L", false, 0, "")
		pdf.CellFormat(3.7, infoRowMM, ":", "", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(contentWidth-47.7, infoRowMM, item.value, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.4)
}

var dailyRecapHeaders = []string{"No", "Nama Petani", "No. Kartu Petani", "Merek Mesin", "Tipe Mesin", "No. Seri Mesin"}

func renderDailyRecapTable(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot, logos []registeredLogo) {
	renderDailyRecapTableHeader(pdf, dailyRecapHeaders)
	for i, recipient := range snapshot.Recipients {
		height := dailyRecapRowHeight(pdf, dailyRecapRowCells(i+1, recipient))
		// Tinggi baris dihitung dan diperiksa SEBELUM baris digambar, agar
		// baris terakhir di suatu halaman tidak pernah tergambar melewati
		// margin bawah.
		if pdf.GetY()+height > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			renderDailyRecapHeader(pdf, snapshot, logos)
			renderDailyRecapTableHeader(pdf, dailyRecapHeaders)
		}
		renderDailyRecapRow(pdf, dailyRecapRowCells(i+1, recipient), height)
	}
}

func renderDailyRecapTableHeader(pdf *fpdf.Fpdf, headers []string) {
	x, y := marginMM, pdf.GetY()
	for i, header := range headers {
		drawCell(pdf, x, y, dailyRecapColumnWidths[i], minRowMM+1.5, header, "C", true)
		x += dailyRecapColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+minRowMM+1.5)
}

func dailyRecapRowCells(number int, recipient dailyRecapRecipientSnapshot) []string {
	return []string{
		fmt.Sprintf("%d", number),
		recipient.FullName,
		recipient.FarmerCardNumber,
		recipient.MachineBrand,
		recipient.MachineType,
		recipient.MachineSerial,
	}
}

func dailyRecapRowHeight(pdf *fpdf.Fpdf, cells []string) float64 {
	rowHeight := minRowMM
	pdf.SetFont("Helvetica", "", tableFontPt)
	for i, value := range cells {
		lines := len(pdf.SplitLines([]byte(value), dailyRecapColumnWidths[i]-2*cellPadMM))
		rowHeight = maxFloat(rowHeight, float64(lines)*tableLineMM+2)
	}
	return rowHeight
}

func renderDailyRecapRow(pdf *fpdf.Fpdf, cells []string, rowHeight float64) {
	x, y := marginMM, pdf.GetY()
	aligns := []string{"C", "L", "C", "C", "C", "C"}
	for i, value := range cells {
		drawCell(pdf, x, y, dailyRecapColumnWidths[i], rowHeight, value, aligns[i], false)
		x += dailyRecapColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+rowHeight)
}

// renderDailyRecapGrandTotal menggambar satu baris ringkasan per varian,
// menyambung langsung pada grid kolom tabel utama (bukan tabel terpisah),
// sesuai dokumen referensi: "Grand Total" pada kolom gabungan No/Nama
// Petani/No. Kartu Petani (hanya pada baris varian pertama), label varian
// pada kolom Merek Mesin, deskripsi merek/tipe pada kolom Tipe Mesin (spec
// §9.4), dan jumlah dalam satuan Set pada kolom No. Seri Mesin.
func renderDailyRecapGrandTotal(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot, logos []registeredLogo) {
	mergedWidth := dailyRecapColumnWidths[0] + dailyRecapColumnWidths[1] + dailyRecapColumnWidths[2]
	widths := []float64{mergedWidth, dailyRecapColumnWidths[3], dailyRecapColumnWidths[4], dailyRecapColumnWidths[5]}
	aligns := []string{"L", "C", "L", "C"}
	for i, variant := range snapshot.Variants {
		label := ""
		if i == 0 {
			label = "Grand Total"
		}
		cells := []string{label, variant.Label, joinVariantDescription(variant.MachineBrand, variant.MachineType), fmt.Sprintf("%d Set", variant.Count)}
		height := minRowMM
		pdf.SetFont("Helvetica", "B", tableFontPt)
		for j, value := range cells {
			lines := len(pdf.SplitLines([]byte(value), widths[j]-2*cellPadMM))
			height = maxFloat(height, float64(lines)*tableLineMM+2)
		}
		if pdf.GetY()+height > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			renderDailyRecapHeader(pdf, snapshot, logos)
			renderDailyRecapTableHeader(pdf, dailyRecapHeaders)
		}
		x, y := marginMM, pdf.GetY()
		for j, value := range cells {
			drawCell(pdf, x, y, widths[j], height, value, aligns[j], true)
			x += widths[j]
		}
		pdf.SetXY(marginMM, y+height)
	}
}

func joinVariantDescription(brand, typ string) string {
	brand = trimSpaceOrEmpty(brand)
	typ = trimSpaceOrEmpty(typ)
	if brand == "" {
		return typ
	}
	if typ == "" {
		return brand
	}
	return brand + " / " + typ
}

func trimSpaceOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

func renderDailyRecapSignatures(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot) {
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
		dp3FitText(pdf, x, y+headerHeight+spaceHeight, widths[i], 6.5, "Nama : "+strings.TrimSpace(names[i]))
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight+spaceHeight+6.5)

	x = marginMM
	for i := range widths {
		nipText := ""
		if strings.TrimSpace(nips[i]) != "" {
			nipText = "NIP : " + strings.TrimSpace(nips[i])
		}
		dp3FitText(pdf, x, y+headerHeight+spaceHeight+6.5, widths[i], 5.5, nipText)
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+headerHeight+spaceHeight+12)
}
