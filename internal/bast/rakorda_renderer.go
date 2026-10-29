package bast

import (
	"bytes"
	"fmt"
	"time"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

// Kolom: No., Nama, Pekerjaan, No. Telepon, Ttd. Jumlahnya persis lebar konten
// (pageWidthMM-2*marginMM = 178 mm).
var rakordaColumnWidths = []float64{12, 55, 43, 35, 33}

// RenderRakorda membuat daftar hadir RAKORDA kosong dengan struktur dan ukuran
// huruf Sosialisasi: halaman pertama memuat kop, keterangan, paragraf pembuka,
// "Dihadiri oleh :", dan awal tabel; halaman lanjutan hanya logo dan baris
// bernomor.
func RenderRakorda(snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.RowCount < 5 || snapshot.RowCount > 200 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	registerArial(pdf)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("DAFTAR HADIR RAPAT KOORDINASI", false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	pdf.AddPage()
	renderFormalHeader(pdf, logos, formalHeader{
		title: "DAFTAR HADIR RAPAT KOORDINASI", fiscal: snapshot.FiscalYear,
		metadata: []formalMeta{
			{"Lokasi", textnorm.DisplayTitle(snapshot.Location)},
			{"Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)},
			{"Provinsi", textnorm.DisplayTitle(snapshot.ProvinceName)},
		},
		narrative: rakordaNarrative(snapshot.DocumentDate, snapshot.FiscalYear, snapshot.ZoneName),
	})
	pdf.SetFont(activityFont, "", activityTextPt)
	y := pdf.GetY() - formalNarrativeGap + 1.5
	pdf.Text(marginMM, textBaseline(y, formalMetaRowMM, activityTextPt), "Dihadiri oleh :")
	pdf.SetXY(marginMM, y+formalMetaRowMM+1.5)

	renderRakordaTableHeader(pdf)
	for number := 1; number <= snapshot.RowCount; number++ {
		if pdf.GetY()+activityRowMM > pageHeightMM-bottomMarginMM {
			addFormalPage(pdf, logos, activityTopGapMM)
		}
		renderRakordaBlankRow(pdf, number)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

// renderRakordaLogoStrip menggambar deretan logo tender terpusat di atas
// halaman dan mengembalikan posisi Y tepat di bawahnya.
func renderRakordaLogoStrip(pdf *fpdf.Fpdf, logos []registeredLogo) float64 {
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
	return stripY + stripHeight
}

// rakordaNarrative menyusun paragraf pembuka RAKORDA sesuai EYD: huruf
// kapital hanya di awal kalimat, nama hari/bulan, nama rapat, dan nama
// kegiatan pengadaan (kata tugas tetap kecil); "Liquefied Petroleum Gas"
// dimiringkan. Tahun anggaran dari programs.fiscal_year dan nama zona dari
// program_zones.name.
func rakordaNarrative(documentDate string, fiscalYear int, zoneName string) []textSegment {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return []textSegment{
		{text: fmt.Sprintf("Pada hari ini, %s, tanggal %d %s %d, telah dilakukan Rapat Koordinasi (Rakor) Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana ", days[date.Weekday()], date.Day(), months[date.Month()], date.Year())},
		{text: "Liquefied Petroleum Gas", style: "I"},
		{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga (Termasuk Pendistribusian dan Pemasangan) %s.", fiscalYear, zoneWithWords(titleCaseWords(zoneName)))},
	}
}

func renderRakordaTableHeader(pdf *fpdf.Fpdf) {
	headers := []string{"No.", "Nama", "Pekerjaan", "No. Telepon", "Ttd"}
	x, y := marginMM, pdf.GetY()
	for i, header := range headers {
		drawActivityCell(pdf, x, y, rakordaColumnWidths[i], activityHeaderRowMM, header, "B", activityTableHeadPt)
		x += rakordaColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+activityHeaderRowMM)
}

// renderRakordaBlankRow menulis nomor urut seperti Sosialisasi: rata kiri di
// bagian atas sel, sejajar huruf "N" pada header "No."; sel lain kosong
// untuk tulisan tangan.
func renderRakordaBlankRow(pdf *fpdf.Fpdf, number int) {
	x, y := marginMM, pdf.GetY()
	for _, width := range rakordaColumnWidths {
		pdf.Rect(x, y, width, activityRowMM, "")
		x += width
	}
	pdf.SetFont(activityFont, "B", activityTableHeadPt)
	numberLeft := marginMM + (rakordaColumnWidths[0]-pdf.GetStringWidth("No."))/2
	pdf.SetFont(activityFont, "", activityTableBodyPt)
	pdf.Text(numberLeft, textBaseline(y+0.4, 4.6, activityTableBodyPt), fmt.Sprintf("%d", number))
	pdf.SetXY(marginMM, y+activityRowMM)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
