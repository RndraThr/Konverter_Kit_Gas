package bast

import (
	"bytes"
	"fmt"
	"strings"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	tkdnTitle         = "REALISASI TINGKAT KOMPONEN DALAM NEGERI"
	tkdnTablePt       = 10.0
	tkdnHeaderRowMM   = 11.0
	tkdnRowMM         = 6.8
	tkdnMetaRowMM     = 6.4
	tkdnMetaLabelMM   = 36.0
	tkdnParagraphLine = 6.0
)

// Kolom: No., Nama Barang, Merek, Jumlah (Unit), TKDN. Proporsi mengikuti
// referensi; lebar total sama dengan blok teks (pageWidthMM-2*marginMM =
// 178 mm), sehingga tabel rata kiri-kanan dengan teks dan tidak condong.
var tkdnColumnWidths = []float64{12, 67, 33, 33, 33}

var tkdnSignatureHeaders = []string{"DINAS YANG\nMEMBIDANGI\nPERTANIAN DI DAERAH", "PELAKSANA\nPEMASANGAN &\nPENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS", "PT PERTAMINA\nPATRA NIAGA"}

// tkdnRowGroup adalah satu nomor pada tabel: barang tunggal, atau judul
// kelompok beserta sub-barangnya.
type tkdnRowGroup struct {
	title string
	items []TKDNItem
}

func groupTKDNItems(items []TKDNItem) []tkdnRowGroup {
	var groups []tkdnRowGroup
	for _, item := range items {
		if item.Group != "" && len(groups) > 0 && groups[len(groups)-1].title == item.Group {
			groups[len(groups)-1].items = append(groups[len(groups)-1].items, item)
			continue
		}
		groups = append(groups, tkdnRowGroup{title: item.Group, items: []TKDNItem{item}})
	}
	return groups
}

// RenderTKDN membuat Realisasi TKDN sesuai referensi: halaman pertama memuat
// kop, nomor dokumen, identitas penyedia/pengadaan/wilayah, tabel barang
// dengan jumlah unit dari distribusi, dan pernyataan; tanda tangan empat
// pihak di halaman tersendiri.
func RenderTKDN(snapshot TKDNSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentTKDN || len(snapshot.Items) == 0 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(tkdnTitle, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	registerSignatoryImages(pdf, logoBytes)
	registerWetScan(pdf, logoBytes)
	contentWidth := pageWidthMM - 2*marginMM
	// Blok teks selebar tabel, seperti BA lain.
	textLeft := marginMM
	textWidth := contentWidth

	// Cetak halaman tanda tangan saja: isi dokumen dilewati.
	if !snapshot.Wet.PageOnly {
		pdf.AddPage()
		pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+3)
		pdf.SetFont(activityFont, "BU", activityTitlePt)
		pdf.CellFormat(contentWidth, 7, tkdnTitle, "", 1, "C", false, 0, "")
		pdf.SetFont(activityFont, "", activityTextPt)
		y := pdf.GetY()
		number := "No. " + snapshot.DocumentNumber
		pdf.Text(marginMM+(contentWidth-pdf.GetStringWidth(number))/2, textBaseline(y, 6, activityTextPt), number)
		y += 6 + 8

		valueLeft := textLeft + tkdnMetaLabelMM + 3.5
		writeMeta := func(label string) float64 {
			baseline := textBaseline(y, tkdnMetaRowMM, activityTextPt)
			pdf.SetFont(activityFont, "", activityTextPt)
			pdf.Text(textLeft, baseline, label)
			pdf.Text(textLeft+tkdnMetaLabelMM, baseline, ":")
			return baseline
		}
		pdf.SetFont(activityFont, "", activityTextPt)
		y = drawMetaRow(pdf, textLeft, tkdnMetaLabelMM, textLeft+textWidth, y, tkdnMetaRowMM, activityTextPt, "Nama Penyedia", textnorm.DisplayTitle(snapshot.ProviderName)) + 1
		writeMeta("Nama Pengadaan")
		pdf.SetXY(valueLeft, y+0.5)
		renderJustifiedSegments(pdf, valueLeft, textLeft+textWidth-valueLeft, tkdnMetaRowMM, activityTextPt, []textSegment{
			{text: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana ", style: ""},
			{text: "Liquefied Petroleum Gas", style: "I"},
			{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga", snapshot.FiscalYear), style: ""},
		})
		y = pdf.GetY() + 1
		pdf.SetFont(activityFont, "", activityTextPt)
		y = drawMetaRow(pdf, textLeft, tkdnMetaLabelMM, textLeft+textWidth, y, tkdnMetaRowMM, activityTextPt, "Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)) + 1
		y = drawMetaRow(pdf, textLeft, tkdnMetaLabelMM, textLeft+textWidth, y, tkdnMetaRowMM, activityTextPt, "Provinsi", textnorm.DisplayTitle(snapshot.ProvinceName)) + 4

		pdf.SetY(y)
		renderTKDNTable(pdf, snapshot, logos, textLeft)

		pdf.SetY(pdf.GetY() + 7)
		for _, paragraph := range []string{
			"Bahwa informasi yang kami sampaikan adalah benar, sehingga apabila dikemudian hari ditemukan adanya ketidaksesuaian atas informasi komitmen TKDN, maka Perusahaan bersedia menerima sanksi sesuai ketentuan yang berlaku.",
			"Demikian pernyataan ini kami sampaikan dengan penuh tanggung jawab serta mengikat sesuai dengan ketentuan hukum/peraturan yang berlaku.",
		} {
			lines := float64(len(wrapWords(pdf, paragraph, textWidth)))
			if pdf.GetY()+lines*tkdnParagraphLine > pageHeightMM-bottomMarginMM {
				pdf.AddPage()
				pdf.SetY(renderRakordaLogoStrip(pdf, logos) + activityTopGapMM)
			}
			pdf.SetX(textLeft)
			renderJustifiedSegments(pdf, textLeft, textWidth, tkdnParagraphLine, activityTextPt, []textSegment{{text: paragraph}})
			pdf.SetY(pdf.GetY() + 3)
		}

	}
	signature := layoutActivitySignatures(pdf, snapshot.Signatories, tkdnSignatureHeaders).withMarks(fourSignatories, snapshot.SignatureMarks)
	// Tanda tangan basah: halaman pindaian menggantikan halaman tanda tangan.
	if snapshot.Wet.Scanned {
		drawWetScanPage(pdf, "P")
	} else {
		pdf.AddPage()
		pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+activitySignatureGap)
		renderActivitySignatures(pdf, signature)
		drawWetFooter(pdf, snapshot.Wet, "TKDN", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderTKDNTable(pdf *fpdf.Fpdf, snapshot TKDNSnapshot, logos []registeredLogo, left float64) {
	renderTKDNTableHeader(pdf, left)
	for index, group := range groupTKDNItems(snapshot.Items) {
		titleHeight := 0.0
		if group.title != "" {
			titleHeight = tkdnRowHeight(pdf, []string{"", group.title})
		}
		rowHeights := make([]float64, len(group.items))
		height := titleHeight
		for i, item := range group.items {
			rowHeights[i] = tkdnRowHeight(pdf, tkdnItemCells(group, item, snapshot.TotalPackages))
			height += rowHeights[i]
		}
		// Satu kelompok tidak dipecah antar halaman agar sel nomor tetap utuh.
		if pdf.GetY()+height+tkdnRowMM > pageHeightMM-bottomMarginMM {
			pdf.AddPage()
			pdf.SetY(renderRakordaLogoStrip(pdf, logos) + activityTopGapMM)
			renderTKDNTableHeader(pdf, left)
		}
		y := pdf.GetY()
		drawTKDNCell(pdf, left, y, tkdnColumnWidths[0], height, fmt.Sprintf("%d", index+1), "", "C", false)
		row := y
		x := left + tkdnColumnWidths[0]
		if group.title != "" {
			drawTKDNCell(pdf, x, row, tkdnColumnWidths[1], titleHeight, group.title, "", "L", false)
			for col := 2; col < len(tkdnColumnWidths); col++ {
				drawTKDNCell(pdf, tkdnColumnX(left, col), row, tkdnColumnWidths[col], titleHeight, "", "", "C", false)
			}
			row += titleHeight
		}
		for i, item := range group.items {
			cells := tkdnItemCells(group, item, snapshot.TotalPackages)
			aligns := []string{"", "L", "C", "C", "C"}
			for col := 1; col < len(cells); col++ {
				drawTKDNCell(pdf, tkdnColumnX(left, col), row, tkdnColumnWidths[col], rowHeights[i], cells[col], "", aligns[col], false)
			}
			row += rowHeights[i]
		}
		pdf.SetY(row)
	}
	y := pdf.GetY()
	merged := tkdnColumnWidths[0] + tkdnColumnWidths[1] + tkdnColumnWidths[2] + tkdnColumnWidths[3]
	drawTKDNCell(pdf, left, y, merged, tkdnRowMM, "JUMLAH", "B", "C", true)
	drawTKDNCell(pdf, left+merged, y, tkdnColumnWidths[4], tkdnRowMM, formatTKDNTotal(snapshot.TotalTKDN), "B", "C", true)
	pdf.SetY(y + tkdnRowMM)
}

// tkdnItemCells adalah isi kolom satu barang (kolom nomor dikosongkan karena
// digabung per kelompok).
func tkdnItemCells(group tkdnRowGroup, item TKDNItem, totalPackages int) []string {
	name := item.Name
	if group.title != "" {
		name = "- " + name
	}
	return []string{"", name, item.Brand, formatTKDNQuantity(item.QuantityPerPackage, totalPackages), formatTKDNPercent(item.TKDNPercent)}
}

// tkdnRowHeight adalah tinggi baris yang memuat seluruh isi selnya; minimal
// tkdnRowMM seperti tata letak acuan.
func tkdnRowHeight(pdf *fpdf.Fpdf, cells []string) float64 {
	height := tkdnRowMM
	for col, text := range cells {
		if text == "" {
			continue
		}
		size, lines := tkdnCellLines(pdf, text, "", tkdnColumnWidths[col])
		height = maxFloat(height, float64(len(lines))*tkdnLineHeight(size)+2)
	}
	return height
}

func tkdnLineHeight(size float64) float64 { return size * 0.3528 * 1.2 }

// tkdnCellLines memilih ukuran huruf dan baris isi sel: teks yang sedikit
// lebih lebar diperkecil (minimal 7 pt) agar tetap satu baris, mis. merek
// "Pertamina Enduro"; yang masih terlalu lebar dibungkus pada 7 pt.
func tkdnCellLines(pdf *fpdf.Fpdf, text, style string, w float64) (float64, []string) {
	parts := strings.Split(text, "\n")
	width := w - 2*activitySignaturePadXMM
	size := tkdnTablePt
	for ; size > 7; size -= 0.5 {
		pdf.SetFont(activityFont, style, size)
		fits := true
		for _, line := range parts {
			if pdf.GetStringWidth(line) > width {
				fits = false
			}
		}
		if fits {
			return size, parts
		}
	}
	pdf.SetFont(activityFont, style, size)
	var lines []string
	for _, part := range parts {
		lines = append(lines, wrapWords(pdf, part, width)...)
	}
	return size, lines
}

func renderTKDNTableHeader(pdf *fpdf.Fpdf, left float64) {
	y := pdf.GetY()
	for i, header := range []string{"NO.", "NAMA BARANG", "MEREK", "JUMLAH\n(UNIT)", "TKDN"} {
		drawTKDNCell(pdf, tkdnColumnX(left, i), y, tkdnColumnWidths[i], tkdnHeaderRowMM, header, "B", "C", true)
	}
	pdf.SetY(y + tkdnHeaderRowMM)
}

func tkdnColumnX(left float64, column int) float64 {
	x := left
	for i := 0; i < column; i++ {
		x += tkdnColumnWidths[i]
	}
	return x
}

// drawTKDNCell menggambar sel bergaris (abu-abu bila shaded) dengan teks
// satu atau beberapa baris ("\n") yang dipusatkan secara vertikal.
func drawTKDNCell(pdf *fpdf.Fpdf, x, y, w, h float64, text, style, align string, shaded bool) {
	if shaded {
		pdf.SetFillColor(217, 217, 217)
		pdf.Rect(x, y, w, h, "FD")
	} else {
		pdf.Rect(x, y, w, h, "D")
	}
	if text == "" {
		return
	}
	size, lines := tkdnCellLines(pdf, text, style, w)
	pdf.SetFont(activityFont, style, size)
	lineHeight := tkdnLineHeight(size)
	top := y + (h-float64(len(lines))*lineHeight)/2
	for i, line := range lines {
		lineX := x + activitySignaturePadXMM
		if align == "C" {
			lineX = x + (w-pdf.GetStringWidth(line))/2
		}
		pdf.Text(lineX, textBaseline(top+float64(i)*lineHeight, lineHeight, size), line)
	}
}
