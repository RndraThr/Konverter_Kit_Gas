package bast

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/go-pdf/fpdf"
)

// Font BA kegiatan: Arial asli yang disematkan (lihat fonts.go).
const activityFont = arialFamily

// Ukuran huruf BA kegiatan (pt): judul 14, sub judul 12, blok teks (lokasi
// sampai "Dihadiri oleh") 12, header tabel hadir 11, isi tabel 10, dan tabel
// tanda tangan 10.
const (
	activityTitlePt     = 14.0
	activitySubtitlePt  = 12.0
	activityTextPt      = 12.0
	activityTableHeadPt = 11.0
	activityTableBodyPt = 10.0
	activitySignaturePt = 10.0

	activityHeaderRowMM    = 9.0
	activityRowMM          = 8.2
	activityTopGapMM       = 4.0
	activitySignatureGap   = 10.0
	activitySignatureSpace = 28.0

	// Tabel tanda tangan empat kolom sama lebar, persis selebar tabel utama
	// (4 × 44,5 mm). Nama/NIP 9,5 pt agar NIP 18 digit muat satu baris.
	activitySignatureTableMM = pageWidthMM - 2*marginMM
	activitySignatureNamePt  = 9.5
	activitySignaturePadXMM  = 1.2
	activitySignaturePadYMM  = 1.0
)

// Kolom: No., Nama, Pekerjaan, No. Telepon, Alamat, Ttd. Pekerjaan dan
// No. Telepon cukup selebar judul/isinya; sisa dari Pekerjaan ke Alamat,
// sisa dari No. Telepon ke Nama. Jumlahnya persis lebar
// konten (pageWidthMM-2*marginMM).
var activityColumnWidths = []float64{12, 38, 22, 28, 61, 17}

// activityDocument membedakan BA kegiatan yang berbagi tata letak daftar
// hadir: judul, kata kegiatan, dan sasaran peserta pada narasi pembuka.
type activityDocument struct {
	title    string
	activity string
	audience string
}

var (
	sosialisasiDocument = activityDocument{title: "BERITA ACARA TELAH DILAKSANAKANNYA SOSIALISASI", activity: "Sosialisasi", audience: "10% penerima"}
	training10Document  = activityDocument{title: "BERITA ACARA TELAH DILAKSANAKANNYA TRAINING", activity: "Training", audience: "10% penerima"}
	training100Document = activityDocument{title: "BERITA ACARA TELAH DILAKSANAKANNYA TRAINING", activity: "Training", audience: "penerima"}
)

func RenderSosialisasi(snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	return renderActivityAttendance(sosialisasiDocument, snapshot, logoBytes)
}

func RenderTraining10(snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	return renderActivityAttendance(training10Document, snapshot, logoBytes)
}

func RenderTraining100(snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	return renderActivityAttendance(training100Document, snapshot, logoBytes)
}

// renderActivityAttendance membuat lembar BA kegiatan kosong: halaman pertama
// memuat kop, narasi, dan awal tabel hadir; halaman lanjutan hanya logo dan
// baris bernomor; kolom tanda tangan empat pihak menyusul di bawah baris
// terakhir, atau di halaman baru bila ruangnya tidak cukup.
func renderActivityAttendance(document activityDocument, snapshot RakordaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if len(snapshot.Participants) == 0 && (snapshot.RowCount < 5 || snapshot.RowCount > 200) {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(document.title, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}

	pdf.AddPage()
	renderActivityHeader(pdf, document, snapshot, logos)
	renderActivityTableHeader(pdf)
	rows := snapshot.RowCount
	if len(snapshot.Participants) > 0 {
		rows = len(snapshot.Participants)
	}
	for number := 1; number <= rows; number++ {
		height := activityRowMM
		var lines [][]string
		if len(snapshot.Participants) > 0 {
			lines, height = layoutParticipantRow(pdf, snapshot.Participants[number-1])
		}
		if pdf.GetY()+height > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+activityTopGapMM)
		}
		if len(snapshot.Participants) > 0 {
			renderActivityParticipantRow(pdf, number, snapshot.Participants[number-1], lines, height)
		} else {
			renderActivityBlankRow(pdf, number)
		}
	}

	// Tanda tangan selalu di halaman terakhir tersendiri, terpisah dari tabel hadir.
	signature := layoutActivitySignatures(pdf, snapshot.Signatories, activitySignatureHeaders)
	pdf.AddPage()
	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+activitySignatureGap)
	renderActivitySignatures(pdf, signature)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderActivityHeader(pdf *fpdf.Fpdf, document activityDocument, snapshot RakordaSnapshot, logos []registeredLogo) {
	contentWidth := pageWidthMM - 2*marginMM
	stripBottom := renderRakordaLogoStrip(pdf, logos)

	pdf.SetXY(marginMM, stripBottom+3)
	pdf.SetFont(activityFont, "BU", activityTitlePt)
	pdf.CellFormat(contentWidth, 7, document.title, "", 1, "C", false, 0, "")
	pdf.Ln(1)
	renderActivityProcurementHeading(pdf, contentWidth, snapshot.FiscalYear, activitySubtitlePt)
	pdf.Ln(3)

	// Blok teks (metadata, pernyataan, "Dihadiri oleh") menjorok sejajar garis
	// kolom No. tabel di kiri, dengan jarak yang sama dari tepi kanan tabel.
	textLeft := marginMM + activityColumnWidths[0]
	textWidth := contentWidth - 2*activityColumnWidths[0]
	const metadataRowMM, narrativeLineMM, labelWidthMM = 6.0, 5.4, 40.0

	metadata := []struct{ label, value string }{
		{"Lokasi", snapshot.Location},
		{"Kabupaten / Kota", snapshot.RegencyName},
		{"Provinsi", snapshot.ProvinceName},
	}
	pdf.SetFont(activityFont, "", activityTextPt)
	y := pdf.GetY()
	for _, item := range metadata {
		baseline := textBaseline(y, metadataRowMM, activityTextPt)
		pdf.Text(textLeft, baseline, item.label)
		pdf.Text(textLeft+labelWidthMM, baseline, ":")
		pdf.Text(textLeft+labelWidthMM+3.5, baseline, item.value)
		y += metadataRowMM
	}
	pdf.SetXY(textLeft, y+1)

	renderJustifiedSegments(pdf, textLeft, textWidth, narrativeLineMM, activityTextPt, activityNarrative(document, snapshot.DocumentDate, snapshot.FiscalYear, snapshot.ZoneName))
	pdf.Ln(1.5)

	pdf.SetFont(activityFont, "", activityTextPt)
	y = pdf.GetY()
	pdf.Text(textLeft, textBaseline(y, metadataRowMM, activityTextPt), "Dihadiri oleh :")
	pdf.SetXY(marginMM, y+metadataRowMM+1.5)
}

// renderActivityProcurementHeading menulis sub judul pengadaan dengan huruf
// besar-kecil seperti dokumen referensi BA kegiatan (berbeda dari BA lain
// yang memakai huruf kapital seluruhnya).
func renderActivityProcurementHeading(pdf *fpdf.Fpdf, contentWidth float64, fiscalYear int, fontSize float64) {
	lines := [][]textSegment{
		{{text: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana", style: "B"}},
		{{text: "Liquefied Petroleum Gas", style: "BI"}, {text: " (LPG) untuk Mesin Pompa Air Bagi Petani", style: "B"}},
		{{text: fmt.Sprintf("Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga", fiscalYear), style: "B"}},
	}
	lineHeight := fontSize * 0.3528 * 1.37
	y := pdf.GetY()
	for _, line := range lines {
		width := 0.0
		for _, segment := range line {
			pdf.SetFont(activityFont, segment.style, fontSize)
			width += pdf.GetStringWidth(segment.text)
		}
		x := marginMM + (contentWidth-width)/2
		for _, segment := range line {
			pdf.SetFont(activityFont, segment.style, fontSize)
			pdf.Text(x, textBaseline(y, lineHeight, fontSize), segment.text)
			x += pdf.GetStringWidth(segment.text)
		}
		y += lineHeight
	}
	pdf.SetXY(marginMM, y)
}

// activityNarrative menyusun paragraf pembuka sesuai dokumen referensi:
// bagian pembuka huruf biasa, uraian pengadaan sampai nama zona ditebalkan,
// dan "Liquefied Petroleum Gas" dimiringkan. Tahun anggaran dari
// programs.fiscal_year dan nama zona dari program_zones.name.
func activityNarrative(document activityDocument, documentDate string, fiscalYear int, zoneName string) []textSegment {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return []textSegment{
		{text: fmt.Sprintf("Pada Hari ini, %s, Tanggal %d %s Tahun %d, Telah dilakukan %s kepada %s ", days[date.Weekday()], date.Day(), months[date.Month()], date.Year(), document.activity, document.audience), style: ""},
		{text: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana ", style: "B"},
		{text: "Liquefied Petroleum Gas", style: "BI"},
		{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga (Termasuk Pendistribusian Dan Pemasangan) %s", fiscalYear, titleCaseWords(zoneName)), style: "B"},
	}
}

// renderJustifiedSegments menulis paragraf rata kiri-kanan yang gaya hurufnya
// berganti per segmen (biasa/tebal/miring). Batas segmen harus jatuh pada
// spasi; baris terakhir rata kiri.
func renderJustifiedSegments(pdf *fpdf.Fpdf, x, width, lineHeight, fontSize float64, segments []textSegment) {
	type styledWord struct {
		text, style string
		width       float64
	}
	var words []styledWord
	for _, segment := range segments {
		pdf.SetFont(activityFont, segment.style, fontSize)
		for _, word := range strings.Fields(segment.text) {
			words = append(words, styledWord{text: word, style: segment.style, width: pdf.GetStringWidth(word)})
		}
	}
	pdf.SetFont(activityFont, "", fontSize)
	space := pdf.GetStringWidth(" ")
	y := pdf.GetY()
	for start := 0; start < len(words); {
		end, lineWidth := start, 0.0
		for end < len(words) {
			add := words[end].width
			if end > start {
				add += space
			}
			if end > start && lineWidth+add > width {
				break
			}
			lineWidth += add
			end++
		}
		gap := space
		if end < len(words) && end-start > 1 {
			gap += (width - lineWidth) / float64(end-start-1)
		}
		cursor := x
		for i := start; i < end; i++ {
			pdf.SetFont(activityFont, words[i].style, fontSize)
			pdf.Text(cursor, textBaseline(y, lineHeight, fontSize), words[i].text)
			cursor += words[i].width + gap
		}
		y += lineHeight
		start = end
	}
	pdf.SetXY(x, y)
}

// textBaseline memusatkan teks setinggi fontSize (pt) secara vertikal dalam
// baris setinggi lineHeight (mm) untuk dipakai pdf.Text.
func textBaseline(top, lineHeight, fontSize float64) float64 {
	capHeight := fontSize * 0.3528 * 0.72
	return top + (lineHeight+capHeight)/2
}

func titleCaseWords(value string) string {
	words := strings.Fields(strings.ToLower(value))
	for i, word := range words {
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

func renderActivityTableHeader(pdf *fpdf.Fpdf) {
	headers := []string{"No.", "Nama", "Pekerjaan", "No. Telepon", "Alamat", "Ttd"}
	x, y := marginMM, pdf.GetY()
	for i, header := range headers {
		drawActivityCell(pdf, x, y, activityColumnWidths[i], activityHeaderRowMM, header, "B", activityTableHeadPt)
		x += activityColumnWidths[i]
	}
	pdf.SetXY(marginMM, y+activityHeaderRowMM)
}

// renderActivityBlankRow menulis nomor urut rata kiri di bagian atas sel,
// dimulai sejajar huruf "N" pada header "No."; sel lain dibiarkan kosong
// untuk tulisan tangan.
func renderActivityBlankRow(pdf *fpdf.Fpdf, number int) {
	x, y := marginMM, pdf.GetY()
	for _, width := range activityColumnWidths {
		pdf.Rect(x, y, width, activityRowMM, "")
		x += width
	}
	pdf.SetFont(activityFont, "B", activityTableHeadPt)
	numberLeft := marginMM + (activityColumnWidths[0]-pdf.GetStringWidth("No."))/2
	pdf.SetFont(activityFont, "", activityTableBodyPt)
	pdf.Text(numberLeft, textBaseline(y+0.4, 4.6, activityTableBodyPt), fmt.Sprintf("%d", number))
	pdf.SetXY(marginMM, y+activityRowMM)
}

var activitySignatureHeaders = []string{"DINAS YANG\nMEMBIDANGI\nPERTANIAN DAERAH", "PELAKSANA\nPEMASANGAN &\nPENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS", "PT PERTAMINA\nPATRA NIAGA"}

// activitySignatureLayout menyimpan lebar kolom serta baris teks header dan
// Nama/NIP yang sudah dibungkus, sehingga tinggi sel mengikuti isinya dengan
// jarak tipis ke garis seperti referensi.
type activitySignatureLayout struct {
	widths  []float64
	headers [][]string
	names   [][]string
	// nipPt adalah ukuran huruf baris NIP (baris terakhir kolom Dinas); sama
	// dengan nama kecuali NIP luar biasa panjang sehingga perlu diperkecil.
	nipPt float64
}

// layoutActivitySignatures menyusun empat kolom tanda tangan sama lebar,
// persis selebar tabel utama. Header 10 pt; Nama/NIP 9,5 pt. Nama panjang
// dibungkus per kata dan baris lanjutannya dimulai dari tepi kiri kolom. NIP
// selalu satu baris: bila tetap tidak muat, hanya baris NIP yang diperkecil.
func layoutActivitySignatures(pdf *fpdf.Fpdf, signatories closingSignatories, headers []string) activitySignatureLayout {
	column := activitySignatureTableMM / 4
	usable := column - 2*activitySignaturePadXMM
	nip := strings.TrimSpace("NIP : " + strings.TrimSpace(signatories.AgricultureOfficeNIP))
	layout := activitySignatureLayout{widths: []float64{column, column, column, column}, nipPt: fitFontSize(pdf, nip, usable, activitySignatureNamePt, 7)}

	names := []string{signatories.AgricultureOfficeName, signatories.InstallerName, signatories.SupervisorName, signatories.PertaminaRepName}
	for i, name := range names {
		layout.headers = append(layout.headers, splitCellLines(pdf, headers[i], usable))
		pdf.SetFont(activityFont, "", activitySignatureNamePt)
		lines := wrapWords(pdf, strings.TrimSpace("Nama : "+strings.TrimSpace(name)), usable)
		if i == 0 {
			lines = append(lines, nip)
		}
		layout.names = append(layout.names, lines)
	}
	return layout
}

// fitFontSize mengembalikan ukuran terbesar (dari maxPt turun 0,5 pt, minimal
// minPt) agar text muat dalam width.
func fitFontSize(pdf *fpdf.Fpdf, text string, width, maxPt, minPt float64) float64 {
	size := maxPt
	for ; size > minPt; size -= 0.5 {
		pdf.SetFont(activityFont, "", size)
		if pdf.GetStringWidth(text) <= width {
			break
		}
	}
	return size
}

// wrapWords membungkus teks per kata pada lebar tertentu tanpa memotong kata.
func wrapWords(pdf *fpdf.Fpdf, text string, width float64) []string {
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		candidate := strings.TrimSpace(current + " " + word)
		if current != "" && pdf.GetStringWidth(candidate) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	return append(lines, current)
}

// splitCellLines memecah teks header di "\n" lalu membungkusnya per kata bila
// sebuah baris masih lebih lebar dari sel.
func splitCellLines(pdf *fpdf.Fpdf, text string, width float64) []string {
	pdf.SetFont(activityFont, "", activitySignaturePt)
	var lines []string
	for _, part := range strings.Split(text, "\n") {
		lines = append(lines, wrapWords(pdf, part, width)...)
	}
	return lines
}

func activitySignatureLineHeight() float64 {
	return signatureLineHeight(activitySignaturePt)
}

func signatureLineHeight(sizePt float64) float64 {
	return sizePt * 0.3528 * 1.15
}

func tightCellHeight(columns [][]string) float64 {
	return tightCellHeightAt(columns, activitySignatureLineHeight())
}

func tightCellHeightAt(columns [][]string, lineHeight float64) float64 {
	lines := 0
	for _, column := range columns {
		lines = maxInt(lines, len(column))
	}
	return float64(lines)*lineHeight + 2*activitySignaturePadYMM
}

func (layout activitySignatureLayout) height() float64 {
	return tightCellHeight(layout.headers) + activitySignatureSpace + tightCellHeightAt(layout.names, signatureLineHeight(activitySignatureNamePt))
}

func renderActivitySignatures(pdf *fpdf.Fpdf, layout activitySignatureLayout) {
	nameLine := signatureLineHeight(activitySignatureNamePt)
	headHeight, nameHeight := tightCellHeight(layout.headers), tightCellHeightAt(layout.names, nameLine)
	lineHeight := activitySignatureLineHeight()

	tableWidth := 0.0
	for _, width := range layout.widths {
		tableWidth += width
	}
	y := pdf.GetY()
	x := (pageWidthMM - tableWidth) / 2
	pdf.SetFont(activityFont, "", activitySignaturePt)
	for i, width := range layout.widths {
		pdf.Rect(x, y, width, headHeight, "")
		headTop := y + (headHeight-float64(len(layout.headers[i]))*lineHeight)/2
		for j, text := range layout.headers[i] {
			pdf.Text(x+(width-pdf.GetStringWidth(text))/2, textBaseline(headTop+float64(j)*lineHeight, lineHeight, activitySignaturePt), text)
		}
		pdf.Rect(x, y+headHeight, width, activitySignatureSpace, "")
		nameTop := y + headHeight + activitySignatureSpace
		pdf.Rect(x, nameTop, width, nameHeight, "")
		for j, text := range layout.names[i] {
			size := activitySignatureNamePt
			if i == 0 && j == len(layout.names[i])-1 {
				size = layout.nipPt
			}
			pdf.SetFont(activityFont, "", size)
			pdf.Text(x+activitySignaturePadXMM, textBaseline(nameTop+activitySignaturePadYMM+float64(j)*nameLine, nameLine, size), text)
		}
		pdf.SetFont(activityFont, "", activitySignaturePt)
		x += width
	}
	pdf.SetXY(marginMM, y+headHeight+activitySignatureSpace+nameHeight)
}

// drawActivityCell menggambar sel bergaris dengan teks satu baris terpusat
// horizontal dan vertikal pada ukuran huruf yang ditentukan.
func drawActivityCell(pdf *fpdf.Fpdf, x, y, w, h float64, text, style string, sizePt float64) {
	pdf.Rect(x, y, w, h, "")
	pdf.SetFont(activityFont, style, sizePt)
	pdf.Text(x+(w-pdf.GetStringWidth(text))/2, textBaseline(y, h, sizePt), text)
}
