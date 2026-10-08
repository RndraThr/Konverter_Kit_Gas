package bast

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/go-pdf/fpdf"
)

const (
	pemeriksaanTitle         = "BERITA ACARA"
	pemeriksaanSubtitle      = "(FORM LAPORAN HASIL PEMERIKSAAN BARANG)"
	pemeriksaanRequester     = "PT. Pertamina Patra Niaga – Retail Gas Sales"
	pemeriksaanTablePt       = 11.0
	pemeriksaanChecklistPt   = 11.0
	pemeriksaanSubtitlePt    = 14.0 // sub judul pengadaan lebih besar seperti referensi
	pemeriksaanMetaRowMM     = 6.2
	pemeriksaanMetaLabelMM   = 58.0
	pemeriksaanSectionLeftMM = 18.0
	pemeriksaanTableBodyMM   = 36.0
	pemeriksaanChecklistRow  = 6.0
)

// pemeriksaanContentWidthMM: identitas, pernyataan, tabel A, dan catatan
// semuanya rata kiri dengan penanda "A."/"B." sampai margin kanan.
const pemeriksaanContentWidthMM = pageWidthMM - marginMM - pemeriksaanSectionLeftMM

// Kolom tabel "A. Berdasarkan": No, Jumlah, Unit, Deskripsi, Keterangan, TTD
// Pemeriksa. Proporsi mengikuti referensi, diskalakan ke lebar konten.
var pemeriksaanColumnWidths = scaleWidths([]float64{10.2, 20.9, 16.2, 51.5, 39.1, 28.1}, pemeriksaanContentWidthMM)

func scaleWidths(widths []float64, total float64) []float64 {
	sum := 0.0
	for _, width := range widths {
		sum += width
	}
	scaled := make([]float64, len(widths))
	for i, width := range widths {
		scaled[i] = width * total / sum
	}
	return scaled
}

var pemeriksaanSignatureHeaders = []string{"DINAS YANG MEMBIDANGI\nPERTANIAN DI DAERAH", "PELAKSANA\nPEMASANGAN &\nPENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS", "PT. PERTAMINA PATRA\nNIAGA"}

// RenderPemeriksaan merender satu atau lebih form BA Pemeriksaan ke satu PDF.
// Finalisasi memanggilnya per form (satu file per form); preview memanggilnya
// dengan semua form sekaligus, masing-masing diberi bookmark.
func RenderPemeriksaan(snapshots []PemeriksaanSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if len(snapshots) == 0 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(pemeriksaanTitle+" "+pemeriksaanSubtitle, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	logos, err := registerLogos(pdf, snapshots[0].Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	for _, snapshot := range snapshots {
		if !strings.HasPrefix(snapshot.DocumentType, pemeriksaanDocumentPrefix) || len(snapshot.Form.Rows) == 0 {
			return RenderedAggregate{}, ErrInvalidInput
		}
		renderPemeriksaanForm(pdf, snapshot, logos, len(snapshots) > 1)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderPemeriksaanForm(pdf *fpdf.Fpdf, snapshot PemeriksaanSnapshot, logos []registeredLogo, bookmark bool) {
	contentWidth := pageWidthMM - 2*marginMM
	textLeft := pemeriksaanSectionLeftMM
	tableWidth := pemeriksaanContentWidthMM

	pdf.AddPage()
	if bookmark {
		pdf.Bookmark(snapshot.Form.Title, 0, -1)
	}
	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+3)
	pdf.SetFont(activityFont, "BU", activityTitlePt)
	pdf.CellFormat(contentWidth, 6.5, pemeriksaanTitle, "", 1, "C", false, 0, "")
	pdf.SetFont(activityFont, "BI", 11)
	pdf.CellFormat(contentWidth, 5, pemeriksaanSubtitle, "", 1, "C", false, 0, "")
	pdf.Ln(1)
	renderActivityProcurementHeading(pdf, contentWidth, snapshot.FiscalYear, pemeriksaanSubtitlePt)

	y := pdf.GetY() + 4
	pdf.SetFont(activityFont, "", activityTextPt)
	for _, item := range []struct{ label, value string }{
		{"No. Purchase Order (PO)", snapshot.PONumber},
		{"Kabupaten / Kota", snapshot.RegencyName},
		{"Provinsi", snapshot.ProvinceName},
		{"Pemasok", snapshot.SupplierName},
		{"Fungsi Peminta Pengadaan", pemeriksaanRequester},
	} {
		baseline := textBaseline(y, pemeriksaanMetaRowMM, activityTextPt)
		pdf.Text(textLeft, baseline, item.label)
		pdf.Text(textLeft+pemeriksaanMetaLabelMM, baseline, ":")
		pdf.Text(textLeft+pemeriksaanMetaLabelMM+3.5, baseline, item.value)
		y += pemeriksaanMetaRowMM
	}

	pdf.SetXY(textLeft, y+4)
	renderJustifiedSegments(pdf, textLeft, tableWidth, 5.4, activityTextPt, []textSegment{{text: pemeriksaanNarrative(snapshot.DocumentDate, snapshot.FiscalYear)}})

	y = pdf.GetY() + 6
	renderPemeriksaanSectionTitle(pdf, y, "A.", "Berdasarkan")
	pdf.SetY(y + 8)
	renderPemeriksaanTable(pdf, snapshot, textLeft)

	y = pdf.GetY() + 7
	renderPemeriksaanSectionTitle(pdf, y, "B.", "Hasil Pemeriksaan Barang")
	pdf.SetY(y + 9)
	renderPemeriksaanChecklist(pdf, snapshot.Form.Checklist)

	renderPemeriksaanNote(pdf, pdf.GetY()+5, textLeft, tableWidth, snapshot.Form.Note)

	signature := layoutActivitySignatures(pdf, snapshot.Signatories, pemeriksaanSignatureHeaders)
	pdf.AddPage()
	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+activitySignatureGap)
	renderActivitySignatures(pdf, signature)
}

// renderPemeriksaanSectionTitle menulis "A.  Berdasarkan" dengan judul tebal
// bergaris bawah seperti referensi.
func renderPemeriksaanSectionTitle(pdf *fpdf.Fpdf, y float64, marker, title string) {
	pdf.SetFont(activityFont, "B", activityTextPt)
	baseline := textBaseline(y, 6, activityTextPt)
	pdf.Text(pemeriksaanSectionLeftMM, baseline, marker)
	titleX := pemeriksaanSectionLeftMM + 6
	pdf.Text(titleX, baseline, title)
	pdf.SetLineWidth(0.25)
	pdf.Line(titleX, baseline+0.8, titleX+pdf.GetStringWidth(title), baseline+0.8)
	pdf.SetLineWidth(0.2)
}

func renderPemeriksaanTable(pdf *fpdf.Fpdf, snapshot PemeriksaanSnapshot, left float64) {
	lineHeight := pemeriksaanTablePt * 0.3528 * 1.2
	y := pdf.GetY()
	headerHeight := 2*lineHeight + 2
	x := left
	for i, header := range []string{"NO", "JUMLAH", "UNIT", "DESKRIPSI", "KETERANGAN", "TTD\nPEMERIKSA"} {
		pdf.Rect(x, y, pemeriksaanColumnWidths[i], headerHeight, "D")
		pdf.SetFont(activityFont, "B", pemeriksaanTablePt)
		lines := strings.Split(header, "\n")
		top := y + 1
		if len(lines) == 1 {
			top = y + (headerHeight-lineHeight)/2
		}
		for j, line := range lines {
			pdf.Text(x+(pemeriksaanColumnWidths[i]-pdf.GetStringWidth(line))/2, textBaseline(top+float64(j)*lineHeight, lineHeight, pemeriksaanTablePt), line)
		}
		x += pemeriksaanColumnWidths[i]
	}

	// Isi tabel tanpa garis antar baris, seperti referensi (selang hisap &
	// buang dalam satu blok); tinggi minimum agar tetap lega untuk paraf.
	pdf.SetFont(activityFont, "", pemeriksaanTablePt)
	type cellLines [6][]string
	rows := make([]cellLines, len(snapshot.Form.Rows))
	bodyLines := 0
	for i, row := range snapshot.Form.Rows {
		values := []string{fmt.Sprintf("%d", i+1), formatTKDNQuantity(row.QuantityPerPackage, snapshot.TotalPackages), row.Unit, row.Description, row.Notes, ""}
		height := 1
		for col, value := range values {
			for _, part := range strings.Split(value, "\n") {
				if part == "" && value == "" {
					continue
				}
				rows[i][col] = append(rows[i][col], wrapWords(pdf, part, pemeriksaanColumnWidths[col]-2*activitySignaturePadXMM)...)
			}
			height = maxInt(height, len(rows[i][col]))
		}
		bodyLines += height
	}
	bodyTop := y + headerHeight
	bodyHeight := maxFloat(pemeriksaanTableBodyMM, float64(bodyLines)*lineHeight+4)
	x = left
	for _, width := range pemeriksaanColumnWidths {
		pdf.Rect(x, bodyTop, width, bodyHeight, "D")
		x += width
	}
	lineTop := bodyTop + 1.5
	centered := []bool{false, true, true, false, false, false}
	for _, row := range rows {
		height := 1
		x = left
		for col, lines := range row {
			for j, text := range lines {
				textX := x + activitySignaturePadXMM
				if centered[col] {
					textX = x + (pemeriksaanColumnWidths[col]-pdf.GetStringWidth(text))/2
				}
				pdf.Text(textX, textBaseline(lineTop+float64(j)*lineHeight, lineHeight, pemeriksaanTablePt), text)
			}
			height = maxInt(height, len(lines))
			x += pemeriksaanColumnWidths[col]
		}
		lineTop += float64(height) * lineHeight
	}
	pdf.SetY(bodyTop + bodyHeight)
}

// renderPemeriksaanChecklist menulis tujuh butir bagian B dengan kotak
// centang; pilihan pada checklist form dicetak tercentang.
func renderPemeriksaanChecklist(pdf *fpdf.Fpdf, checklist PemeriksaanChecklist) {
	type option struct{ value, label string }
	columns := []float64{71, 112, 153}
	rows := []struct {
		label    string
		options  []option
		selected func(string) bool
	}{
		{"Kemasan", []option{{"baik", "Baik"}, {"rusak", "Rusak"}}, func(v string) bool { return checklist.Packaging == v }},
		{"Jumlah Barang", []option{{"lengkap", "Lengkap"}, {"kurang", "Kurang"}}, func(v string) bool { return checklist.Quantity == v }},
		{"Jenis Dan Spesifikasi", []option{{"sesuai", "Sesuai"}, {"tidak_sesuai", "Tidak Sesuai"}}, func(v string) bool { return checklist.Specification == v }},
		{"Kondisi Barang", []option{{"baru_baik", "Baru & Baik"}, {"tidak_baik", "Tidak Baik"}, {"bekas", "Bekas"}}, func(v string) bool { return checklist.Condition == v }},
		{"Dokumen Pendukung", []option{{"sertifikat", "Sertifikat"}, {"manual_book", "Manual Book"}, {"garansi", "Garansi"}}, func(v string) bool { return slices.Contains(checklist.Documents, v) }},
		{"", []option{{"lainnya", "Dokumen Lainnya " + otherDocumentText(checklist.OtherDocument)}}, func(v string) bool { return slices.Contains(checklist.Documents, v) }},
		{"Dilakukan Uji Fungsi", []option{{"ya", "Ya"}, {"tidak", "Tidak"}}, func(v string) bool { return checklist.FunctionTest == v }},
		{"Kesimpulan", []option{{"diterima", "Diterima"}, {"tidak_diterima", "Tidak Di Terima"}}, func(v string) bool { return checklist.Conclusion == v }},
	}
	y := pdf.GetY()
	number := 0
	for _, row := range rows {
		baseline := textBaseline(y, pemeriksaanChecklistRow, pemeriksaanChecklistPt)
		pdf.SetFont(activityFont, "", pemeriksaanChecklistPt)
		if row.label != "" {
			number++
			pdf.Text(pemeriksaanSectionLeftMM, baseline, fmt.Sprintf("%d.", number))
			pdf.Text(pemeriksaanSectionLeftMM+6, baseline, row.label)
		}
		for i, opt := range row.options {
			drawPemeriksaanCheckbox(pdf, columns[i], y+(pemeriksaanChecklistRow-3.4)/2, row.selected(opt.value))
			pdf.SetFont(activityFont, "", pemeriksaanChecklistPt)
			pdf.Text(columns[i]+5, baseline, opt.label)
		}
		y += pemeriksaanChecklistRow
	}
	pdf.SetY(y)
}

func otherDocumentText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "……………."
	}
	return strings.TrimSpace(value)
}

func drawPemeriksaanCheckbox(pdf *fpdf.Fpdf, x, y float64, checked bool) {
	const size = 3.4
	pdf.Rect(x, y, size, size, "D")
	if !checked {
		return
	}
	pdf.SetFont("ZapfDingbats", "", 9)
	pdf.Text(x+0.45, y+size-0.5, "4")
}

func renderPemeriksaanNote(pdf *fpdf.Fpdf, y, left, width float64, note string) {
	if strings.TrimSpace(note) == "" {
		note = defaultPemeriksaanNote
	}
	pdf.SetFont(activityFont, "", activityTextPt)
	lines := wrapWords(pdf, "Note : "+note, width-2*activitySignaturePadXMM)
	lineHeight := 5.2
	height := 2 + lineHeight + float64(len(lines))*lineHeight + 3
	pdf.Rect(left, y, width, height, "D")
	pdf.SetFont(activityFont, "B", activityTextPt)
	baseline := textBaseline(y+1, lineHeight, activityTextPt)
	pdf.Text(left+activitySignaturePadXMM, baseline, "CATATAN :")
	pdf.SetLineWidth(0.25)
	pdf.Line(left+activitySignaturePadXMM, baseline+0.8, left+activitySignaturePadXMM+pdf.GetStringWidth("CATATAN :"), baseline+0.8)
	pdf.SetLineWidth(0.2)
	pdf.SetFont(activityFont, "", activityTextPt)
	for i, line := range lines {
		pdf.Text(left+activitySignaturePadXMM, textBaseline(y+1+lineHeight*float64(i+1), lineHeight, activityTextPt), line)
	}
	pdf.SetY(y + height)
}
