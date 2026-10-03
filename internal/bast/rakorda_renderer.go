package bast

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

var rakordaColumnWidths = []float64{12, 55, 43, 35, 35}

func RenderRakorda(snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.RowCount < 5 || snapshot.RowCount > 200 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("DAFTAR HADIR RAPAT KOORDINASI", false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	for first := 1; first <= snapshot.RowCount; first += rakordaRowsPerPage {
		pdf.AddPage()
		renderRakordaHeader(pdf, snapshot, logos, first == 1)
		renderRakordaTableHeader(pdf)
		last := minInt(first+rakordaRowsPerPage-1, snapshot.RowCount)
		for number := first; number <= last; number++ {
			renderRakordaBlankRow(pdf, number)
		}
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderRakordaHeader(pdf *fpdf.Fpdf, snapshot RakordaSnapshot, logos []registeredLogo, full bool) {
	contentWidth := pageWidthMM - 2*marginMM
	gap, stripY, stripHeight := 5.5, topMarginMM, 16.0
	widths := make([]float64, len(logos))
	heights := make([]float64, len(logos))
	for i, logo := range logos {
		maxWidth, maxHeight := logo.maxWidthMM, logo.maxHeightMM
		if maxWidth <= 0 {
			maxWidth = 42
		}
		if maxHeight <= 0 {
			maxHeight = 15
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

	pdf.SetXY(marginMM, stripY+stripHeight+1.5)
	pdf.SetFont("Helvetica", "BU", 12.5)
	pdf.CellFormat(contentWidth, 6, "DAFTAR HADIR RAPAT KOORDINASI", "", 1, "C", false, 0, "")
	if !full {
		pdf.SetFont("Helvetica", "I", 8)
		pdf.CellFormat(contentWidth, 4.2, "LANJUTAN", "", 1, "C", false, 0, "")
		pdf.Ln(1)
		return
	}
	pdf.Ln(1)
	renderProcurementHeading(pdf, marginMM, contentWidth, snapshot.FiscalYear)
	pdf.Ln(1.3)
	metadata := []struct{ label, value string }{
		{"Lokasi", snapshot.Location},
		{"Kabupaten / Kota", snapshot.RegencyName},
		{"Provinsi", snapshot.ProvinceName},
	}
	for _, item := range metadata {
		pdf.SetFont("Helvetica", "", 8.5)
		pdf.SetX(marginMM)
		pdf.CellFormat(36, 4.7, item.label, "", 0, "L", false, 0, "")
		pdf.CellFormat(3.5, 4.7, ":", "", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.CellFormat(contentWidth-39.5, 4.7, item.value, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.6)

	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetX(marginMM)
	pdf.MultiCell(contentWidth, 4.3, rakordaNarrative(snapshot.DocumentDate, snapshot.FiscalYear, snapshot.ZoneName), "", "J", false)
	pdf.Ln(1.2)

	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetX(marginMM)
	pdf.CellFormat(contentWidth, 4.7, "Dihadiri oleh :", "", 1, "L", false, 0, "")
	pdf.Ln(0.8)
}

// rakordaNarrative menyusun paragraf "Pada Hari ini, ... Rapat Koordinasi
// (RAKOR) ... Zona ..." sesuai dokumen referensi, dengan tahun anggaran dari
// programs.fiscal_year dan nama zona dari program_zones.name.
func rakordaNarrative(documentDate string, fiscalYear int, zoneName string) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"MINGGU", "SENIN", "SELASA", "RABU", "KAMIS", "JUMAT", "SABTU"}
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	return fmt.Sprintf(
		"PADA HARI INI, %s, TANGGAL %d %s TAHUN %d, TELAH DILAKUKAN RAPAT KOORDINASI (RAKOR) PENGADAAN BARANG PENYEDIAAN DAN PENDISTRIBUSIAN PAKET PERDANA LIQUEFIED PETROLEUM GAS (LPG) UNTUK MESIN POMPA AIR BAGI PETANI SASARAN TAHUN ANGGARAN %d DI PT PERTAMINA PATRA NIAGA (TERMASUK PENDISTRIBUSIAN DAN PEMASANGAN) %s.",
		days[date.Weekday()], date.Day(), months[date.Month()], date.Year(), fiscalYear, zoneName,
	)
}

func renderRakordaTableHeader(pdf *fpdf.Fpdf) {
	headers := []string{"NO.", "NAMA", "PEKERJAAN", "NO. TELEPON", "TTD"}
	x, y := marginMM, pdf.GetY()
	for i, header := range headers {
		drawCell(pdf, x, y, rakordaColumnWidths[i], 9, header, "C", true)
		x += rakordaColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+9)
}

func renderRakordaBlankRow(pdf *fpdf.Fpdf, number int) {
	x, y := marginMM, pdf.GetY()
	values := []string{fmt.Sprintf("%d", number), "", "", "", ""}
	for i, value := range values {
		drawCell(pdf, x, y, rakordaColumnWidths[i], 7.2, value, "C", false)
		x += rakordaColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+7.2)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
