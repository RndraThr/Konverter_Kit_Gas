package bast

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// drawLogoStrip menggambar deretan logo tender terpusat dalam kotak setinggi
// height mulai dari top, lalu mengembalikan posisi Y tepat di bawah kotak.
func drawLogoStrip(pdf *fpdf.Fpdf, logos []registeredLogo, left, width, top, height, defaultMaxHeight float64) float64 {
	const gap = 5.8
	widths := make([]float64, len(logos))
	heights := make([]float64, len(logos))
	for i, logo := range logos {
		maxWidth := logo.maxWidthMM
		if maxWidth <= 0 {
			maxWidth = 42
		}
		maxHeight := logo.maxHeightMM
		if maxHeight <= 0 {
			maxHeight = defaultMaxHeight
		}
		scale := minFloat(maxWidth/logo.widthMM, maxHeight/logo.heightMM)
		widths[i], heights[i] = logo.widthMM*scale, logo.heightMM*scale
	}
	totalWidth := gap * float64(maxInt(0, len(logos)-1))
	for _, w := range widths {
		totalWidth += w
	}
	x := left + (width-totalWidth)/2
	for i, logo := range logos {
		pdf.ImageOptions(logo.name, x, top+(height-heights[i])/2, widths[i], heights[i], false, logo.options, 0, "")
		x += widths[i] + gap
	}
	return top + height
}

// Dokumen formal tegak (Closing Titik Serah, Closing Kabupaten, RAKORDA)
// memakai struktur dan ukuran huruf yang sama dengan Sosialisasi s.d. TKDN:
// logo, judul 14 pt, sub judul 12 pt, nomor dan keterangan 12 pt sejajar
// garis kolom No., paragraf rata kiri-kanan 12 pt, header tabel 11 pt tebal,
// isi tabel 10 pt, dan tanda tangan di halaman terakhir tersendiri.
const (
	formalMetaRowMM    = 6.0
	formalMetaLabelMM  = 40.0
	formalNarrativeMM  = 5.4
	formalHeaderMinMM  = 11.0
	formalRowMinMM     = 6.8
	formalCellPadYMM   = 1.0
	formalCellPadXMM   = 1.6
	formalSubtitlePt   = 11.0
	formalNumberGapMM  = 6.0
	formalMetadataGap  = 3.0
	formalNarrativeGap = 2.0
)

type formalMeta struct{ label, value string }

// formalHeader adalah isi kop halaman pertama dokumen formal.
type formalHeader struct {
	title     string
	subtitle  string // opsional, mis. "(FORM REKAPITULASI CLOSING ...)"
	number    string // opsional, dicetak "No. ..." di bawah sub judul pengadaan
	fiscal    int
	metadata  []formalMeta
	narrative []textSegment
	// pageWidth lebar halaman; 0 berarti A4 tegak. DP3 memakai A4 mendatar.
	pageWidth float64
}

func renderFormalHeader(pdf *fpdf.Fpdf, logos []registeredLogo, header formalHeader) {
	width := header.pageWidth
	if width == 0 {
		width = pageWidthMM
	}
	contentWidth := width - 2*marginMM
	pdf.SetXY(marginMM, formalLogoStrip(pdf, logos, width)+3)
	pdf.SetFont(activityFont, "BU", activityTitlePt)
	pdf.CellFormat(contentWidth, 7, header.title, "", 1, "C", false, 0, "")
	if header.subtitle != "" {
		pdf.SetFont(activityFont, "BI", formalSubtitlePt)
		pdf.CellFormat(contentWidth, 5.5, header.subtitle, "", 1, "C", false, 0, "")
	}
	pdf.Ln(1)
	renderActivityProcurementHeading(pdf, contentWidth, header.fiscal, activitySubtitlePt)
	y := pdf.GetY()
	if header.number != "" {
		number := "No. " + header.number
		pdf.SetFont(activityFont, "B", activityTextPt)
		pdf.Text(marginMM+(contentWidth-pdf.GetStringWidth(number))/2, textBaseline(y+1, formalNumberGapMM, activityTextPt), number)
		y += formalNumberGapMM + 1
	}
	y += formalMetadataGap

	// Keterangan dan paragraf selebar tabel: sejajar garis kiri dan kanannya
	// (sama di semua BA).
	textLeft, textWidth := marginMM, contentWidth
	pdf.SetFont(activityFont, "", activityTextPt)
	for _, item := range header.metadata {
		baseline := textBaseline(y, formalMetaRowMM, activityTextPt)
		pdf.Text(textLeft, baseline, item.label)
		pdf.Text(textLeft+formalMetaLabelMM, baseline, ":")
		pdf.Text(textLeft+formalMetaLabelMM+3.5, baseline, item.value)
		y += formalMetaRowMM
	}
	pdf.SetXY(textLeft, y+1)
	if len(header.narrative) > 0 {
		renderJustifiedSegments(pdf, textLeft, textWidth, formalNarrativeMM, activityTextPt, header.narrative)
	}
	pdf.SetXY(marginMM, pdf.GetY()+formalNarrativeGap)
}

// formalLogoStrip menggambar deretan logo terpusat pada lebar halaman width
// dengan ukuran yang sama seperti Sosialisasi, lalu mengembalikan Y di bawahnya.
func formalLogoStrip(pdf *fpdf.Fpdf, logos []registeredLogo, width float64) float64 {
	if width == pageWidthMM {
		return renderRakordaLogoStrip(pdf, logos)
	}
	return drawLogoStrip(pdf, logos, marginMM, width-2*marginMM, topMarginMM, 16, 15)
}

// addFormalPage membuka halaman lanjutan yang hanya memuat logo; gap adalah
// jarak dari logo ke isi (activityTopGapMM untuk tabel lanjutan,
// activitySignatureGap untuk halaman tanda tangan).
func addFormalPage(pdf *fpdf.Fpdf, logos []registeredLogo, gap float64) {
	pdf.AddPage()
	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+gap)
}

func formalLineHeight(sizePt float64) float64 { return signatureLineHeight(sizePt) }

// formalCellHeight menghitung tinggi sel agar seluruh baris teks muat.
func formalCellHeight(pdf *fpdf.Fpdf, widths []float64, cells []string, style string, sizePt, minHeight float64) float64 {
	pdf.SetFont(activityFont, style, sizePt)
	height := minHeight
	for i, value := range cells {
		lines := 0
		for _, part := range strings.Split(value, "\n") {
			lines += len(wrapWords(pdf, part, widths[i]-2*formalCellPadXMM))
		}
		height = maxFloat(height, float64(lines)*formalLineHeight(sizePt)+2*formalCellPadYMM)
	}
	return height
}

// drawFormalCell menggambar sel bergaris dengan teks terbungkus per kata,
// terpusat vertikal, pada ukuran dan gaya huruf tertentu.
func drawFormalCell(pdf *fpdf.Fpdf, x, y, w, h float64, text, align, style string, sizePt float64) {
	pdf.Rect(x, y, w, h, "")
	if text == "" {
		return
	}
	pdf.SetFont(activityFont, style, sizePt)
	var lines []string
	for _, part := range strings.Split(text, "\n") {
		lines = append(lines, wrapWords(pdf, part, w-2*formalCellPadXMM)...)
	}
	lineHeight := formalLineHeight(sizePt)
	top := y + (h-float64(len(lines))*lineHeight)/2
	for i, line := range lines {
		lx := x + formalCellPadXMM
		switch align {
		case "C":
			lx = x + (w-pdf.GetStringWidth(line))/2
		case "R":
			lx = x + w - formalCellPadXMM - pdf.GetStringWidth(line)
		}
		pdf.Text(lx, textBaseline(top+float64(i)*lineHeight, lineHeight, sizePt), line)
	}
}

// renderFormalTableHeader menggambar baris header tabel (11 pt tebal).
func renderFormalTableHeader(pdf *fpdf.Fpdf, widths []float64, headers []string) {
	height := formalCellHeight(pdf, widths, headers, "B", activityTableHeadPt, formalHeaderMinMM)
	x, y := marginMM, pdf.GetY()
	for i, header := range headers {
		drawFormalCell(pdf, x, y, widths[i], height, header, "C", "B", activityTableHeadPt)
		x += widths[i]
	}
	pdf.SetXY(marginMM, y+height)
}

// formalRowGroup adalah sekelompok baris tabel yang kolom-kolom awalnya
// (sebanyak lead) digabung vertikal, mis. varian pada tanggal yang sama.
type formalRowGroup struct {
	cells   [][]string
	heights []float64
}

func layoutFormalRowGroup(pdf *fpdf.Fpdf, widths []float64, rows [][]string) formalRowGroup {
	group := formalRowGroup{cells: rows}
	for _, row := range rows {
		group.heights = append(group.heights, formalCellHeight(pdf, widths, row, "", activityTableBodyPt, formalRowMinMM))
	}
	return group
}

func (g formalRowGroup) height() float64 {
	total := 0.0
	for _, h := range g.heights {
		total += h
	}
	return total
}

func drawFormalRowGroup(pdf *fpdf.Fpdf, widths []float64, group formalRowGroup, lead int, aligns []string) {
	top := pdf.GetY()
	x := marginMM
	for c := 0; c < lead; c++ {
		drawFormalCell(pdf, x, top, widths[c], group.height(), group.cells[0][c], aligns[c], "", activityTableBodyPt)
		x += widths[c]
	}
	y := top
	for i, row := range group.cells {
		cx := x
		for c := lead; c < len(row); c++ {
			drawFormalCell(pdf, cx, y, widths[c], group.heights[i], row[c], aligns[c], "", activityTableBodyPt)
			cx += widths[c]
		}
		y += group.heights[i]
	}
	pdf.SetXY(marginMM, y)
}

// drawFormalTotalRow menggambar baris "Jumlah/Total": kolom sebelum kolom
// jumlah digabung menjadi satu label tebal, lalu total, lalu kolom sisanya kosong.
func drawFormalTotalRow(pdf *fpdf.Fpdf, widths []float64, totalColumn, total int) {
	x, y := marginMM, pdf.GetY()
	merged := 0.0
	for _, w := range widths[:totalColumn] {
		merged += w
	}
	drawFormalCell(pdf, x, y, merged, formalRowMinMM, "Jumlah/Total", "C", "B", activityTableBodyPt)
	x += merged
	drawFormalCell(pdf, x, y, widths[totalColumn], formalRowMinMM, fmt.Sprintf("%d", total), "C", "B", activityTableBodyPt)
	x += widths[totalColumn]
	for _, w := range widths[totalColumn+1:] {
		drawFormalCell(pdf, x, y, w, formalRowMinMM, "", "C", "B", activityTableBodyPt)
		x += w
	}
	pdf.SetXY(marginMM, y+formalRowMinMM)
}

// renderFormalSignatures menaruh tabel tanda tangan empat pihak di halaman
// terakhir tersendiri, paling atas, seperti Sosialisasi.
func renderFormalSignatures(pdf *fpdf.Fpdf, logos []registeredLogo, headers, names []string, agricultureNIP string, marks SignatureMarks) {
	layout := layoutSignatures(pdf, activitySignatureTableMM, headers, names, agricultureNIP).withMarks(fourSignatories, marks)
	addFormalPage(pdf, logos, activitySignatureGap)
	renderActivitySignatures(pdf, layout)
}

// fixPDFDates mengisi tanggal pembuatan dan perubahan PDF dari tanggal
// dokumen, dan mengurutkan katalog. Tanpa ini fpdf mengisi jam saat render,
// sehingga isi yang sama menghasilkan checksum berbeda dan finalisasi ulang
// selalu membuat versi baru alih-alih dianggap tidak berubah.
func fixPDFDates(pdf *fpdf.Fpdf, documentDate string) {
	date, _ := time.Parse("2006-01-02", documentDate)
	pdf.SetCreationDate(date)
	pdf.SetModificationDate(date)
	pdf.SetCatalogSort(true)
}
