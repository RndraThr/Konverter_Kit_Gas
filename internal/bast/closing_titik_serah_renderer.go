package bast

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

const (
	closingTitle    = "BERITA ACARA SERAH TERIMA"
	closingSubtitle = "(FORM REKAPITULASI CLOSING LOKASI / TITIK SERAH)"
)

// Kolom: No, Hari & Tanggal (Closing Harian), Merek Mesin, Tipe Mesin (Per-Varian), Jumlah Paket (set), Keterangan.
// Jumlah lebar harus persis = pageWidthMM-2*marginMM (182mm) agar tabel tidak
// melebihi atau menyisakan celah di margin kanan.
var closingColumnWidths = []float64{12, 38, 30, 40, 28, 34}

func RenderClosingTitikSerah(snapshot ClosingTitikSerahSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentClosingTitikSerah {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseClosingSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, marginMM)
	pdf.SetTitle("CLOSING TITIK SERAH "+snapshot.DocumentDate, false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	pdf.AddPage()
	renderClosingHeader(pdf, snapshot, logos)
	renderClosingTable(pdf, snapshot, logos)

	pdf.AddPage()
	renderClosingHeader(pdf, snapshot, logos)
	renderClosingSignatures(pdf, snapshot)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderClosingHeader(pdf *fpdf.Fpdf, snapshot ClosingTitikSerahSnapshot, logos []registeredLogo) {
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
	pdf.CellFormat(contentWidth, 5.6, closingTitle, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BI", 9)
	pdf.CellFormat(contentWidth, 4.2, closingSubtitle, "", 1, "C", false, 0, "")
	pdf.Ln(1.4)

	renderProcurementHeading(pdf, marginMM, contentWidth, snapshot.FiscalYear)
	pdf.Ln(1.6)

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetX(marginMM)
	pdf.CellFormat(contentWidth, 4.6, "No. "+snapshot.DocumentNumber, "", 1, "C", false, 0, "")
	pdf.Ln(1.2)

	metadata := []struct{ label, value string }{
		{"Lokasi / Titik Serah", snapshot.HandoverLocation},
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

// closingNarrative menyusun paragraf "Pada Hari ini, ... Tanggal ... Tahun ..."
// sesuai dokumen referensi, dengan tahun anggaran dari programs.fiscal_year.
func closingNarrative(documentDate string, fiscalYear int) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"MINGGU", "SENIN", "SELASA", "RABU", "KAMIS", "JUMAT", "SABTU"}
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	return fmt.Sprintf(
		"PADA HARI INI, %s, TANGGAL %d %s TAHUN %d, TELAH DILAKUKAN PEMASANGAN DAN PENDISTRIBUSIAN PAKET PERDANA LIQUEFIED PETROLEUM GAS (LPG) UNTUK MESIN POMPA AIR BAGI PETANI SASARAN TAHUN ANGGARAN %d DI PT PERTAMINA PATRA NIAGA, DENGAN RINCIAN SEBAGAI BERIKUT:",
		days[date.Weekday()], date.Day(), months[date.Month()], date.Year(), fiscalYear,
	)
}

func renderClosingTable(pdf *fpdf.Fpdf, snapshot ClosingTitikSerahSnapshot, logos []registeredLogo) {
	headers := []string{"No.", "Hari & Tanggal\n(Closing Harian)", "Merek Mesin", "Tipe Mesin\n(Per-Varian)", "Jumlah Paket\n(set)", "Keterangan"}
	renderClosingTableHeader(pdf, headers)
	for i, row := range snapshot.Rows {
		cells := closingRowCells(i+1, row)
		height := closingRowHeight(pdf, cells)
		if pdf.GetY()+height > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			renderClosingHeader(pdf, snapshot, logos)
			renderClosingTableHeader(pdf, headers)
		}
		drawClosingRow(pdf, cells, height, false)
	}
	totalHeight := minRowMM
	if pdf.GetY()+totalHeight > pageHeightMM-bottomMarginMM {
		pdf.AddPage()
		renderClosingHeader(pdf, snapshot, logos)
		renderClosingTableHeader(pdf, headers)
	}
	drawClosingTotalRow(pdf, snapshot.GrandTotal)
}

func renderClosingTableHeader(pdf *fpdf.Fpdf, headers []string) {
	x, y := marginMM, pdf.GetY()
	height := minRowMM + 1.5
	for i, header := range headers {
		drawCell(pdf, x, y, closingColumnWidths[i], height, header, "C", true)
		x += closingColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+height)
}

func closingRowCells(number int, row ClosingRow) []string {
	return []string{
		fmt.Sprintf("%d.", number),
		formatIndonesianDate(row.LocalDate),
		row.MachineBrand,
		row.MachineType,
		fmt.Sprintf("%d", row.Count),
		"",
	}
}

func closingRowHeight(pdf *fpdf.Fpdf, cells []string) float64 {
	height := minRowMM
	pdf.SetFont("Helvetica", "", tableFontPt)
	for i, value := range cells {
		lines := len(pdf.SplitLines([]byte(value), closingColumnWidths[i]-2*cellPadMM))
		height = maxFloat(height, float64(lines)*tableLineMM+2)
	}
	return height
}

func drawClosingRow(pdf *fpdf.Fpdf, cells []string, height float64, bold bool) {
	x, y := marginMM, pdf.GetY()
	aligns := []string{"C", "C", "C", "C", "C", "L"}
	for i, value := range cells {
		drawCell(pdf, x, y, closingColumnWidths[i], height, value, aligns[i], bold)
		x += closingColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+height)
}

// drawClosingTotalRow menggambar baris "Jumlah/Total": empat kolom pertama
// digabung menjadi satu label, kolom Jumlah Paket diisi total, Keterangan kosong.
func drawClosingTotalRow(pdf *fpdf.Fpdf, total int) {
	x, y := marginMM, pdf.GetY()
	mergedWidth := closingColumnWidths[0] + closingColumnWidths[1] + closingColumnWidths[2] + closingColumnWidths[3]
	height := minRowMM
	drawCell(pdf, x, y, mergedWidth, height, "Jumlah/Total", "C", true)
	x += mergedWidth
	drawCell(pdf, x, y, closingColumnWidths[4], height, fmt.Sprintf("%d", total), "C", true)
	x += closingColumnWidths[4]
	drawCell(pdf, x, y, closingColumnWidths[5], height, "", "C", true)
	pdf.SetXY(marginMM, y+height)
}

func renderClosingSignatures(pdf *fpdf.Fpdf, snapshot ClosingTitikSerahSnapshot) {
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
