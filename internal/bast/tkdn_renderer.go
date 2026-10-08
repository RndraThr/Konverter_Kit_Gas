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
// referensi; lebar total sama dengan blok teks (154 mm).
var tkdnColumnWidths = []float64{10.5, 57.7, 28.6, 28.6, 28.6}

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

func (g tkdnRowGroup) rowCount() int {
	if g.title == "" {
		return len(g.items)
	}
	return len(g.items) + 1
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
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(tkdnTitle, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	contentWidth := pageWidthMM - 2*marginMM
	textLeft := marginMM + activityColumnWidths[0]
	textWidth := contentWidth - 2*activityColumnWidths[0]

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
	writeMeta("Nama Penyedia")
	pdf.Text(valueLeft, textBaseline(y, tkdnMetaRowMM, activityTextPt), textnorm.DisplayTitle(snapshot.ProviderName))
	y += tkdnMetaRowMM + 1
	writeMeta("Nama Pengadaan")
	pdf.SetXY(valueLeft, y+0.5)
	renderJustifiedSegments(pdf, valueLeft, textLeft+textWidth-valueLeft, tkdnMetaRowMM, activityTextPt, []textSegment{
		{text: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana ", style: ""},
		{text: "Liquefied Petroleum Gas", style: "I"},
		{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga", snapshot.FiscalYear), style: ""},
	})
	y = pdf.GetY() + 1
	writeMeta("Kabupaten / Kota")
	pdf.Text(valueLeft, textBaseline(y, tkdnMetaRowMM, activityTextPt), textnorm.DisplayTitle(snapshot.RegencyName))
	y += tkdnMetaRowMM + 1
	writeMeta("Provinsi")
	pdf.Text(valueLeft, textBaseline(y, tkdnMetaRowMM, activityTextPt), textnorm.DisplayTitle(snapshot.ProvinceName))
	y += tkdnMetaRowMM + 4

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

	signature := layoutActivitySignatures(pdf, snapshot.Signatories, tkdnSignatureHeaders)
	pdf.AddPage()
	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+activitySignatureGap)
	renderActivitySignatures(pdf, signature)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderTKDNTable(pdf *fpdf.Fpdf, snapshot TKDNSnapshot, logos []registeredLogo, left float64) {
	renderTKDNTableHeader(pdf, left)
	for index, group := range groupTKDNItems(snapshot.Items) {
		height := float64(group.rowCount()) * tkdnRowMM
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
			drawTKDNCell(pdf, x, row, tkdnColumnWidths[1], tkdnRowMM, group.title, "", "L", false)
			for col := 2; col < len(tkdnColumnWidths); col++ {
				drawTKDNCell(pdf, tkdnColumnX(left, col), row, tkdnColumnWidths[col], tkdnRowMM, "", "", "C", false)
			}
			row += tkdnRowMM
		}
		for _, item := range group.items {
			name := item.Name
			if group.title != "" {
				name = "- " + name
			}
			drawTKDNCell(pdf, x, row, tkdnColumnWidths[1], tkdnRowMM, name, "", "L", false)
			drawTKDNCell(pdf, tkdnColumnX(left, 2), row, tkdnColumnWidths[2], tkdnRowMM, item.Brand, "", "C", false)
			drawTKDNCell(pdf, tkdnColumnX(left, 3), row, tkdnColumnWidths[3], tkdnRowMM, formatTKDNQuantity(item.QuantityPerPackage, snapshot.TotalPackages), "", "C", false)
			drawTKDNCell(pdf, tkdnColumnX(left, 4), row, tkdnColumnWidths[4], tkdnRowMM, formatTKDNPercent(item.TKDNPercent), "", "C", false)
			row += tkdnRowMM
		}
		pdf.SetY(row)
	}
	y := pdf.GetY()
	merged := tkdnColumnWidths[0] + tkdnColumnWidths[1] + tkdnColumnWidths[2] + tkdnColumnWidths[3]
	drawTKDNCell(pdf, left, y, merged, tkdnRowMM, "JUMLAH", "B", "C", true)
	drawTKDNCell(pdf, left+merged, y, tkdnColumnWidths[4], tkdnRowMM, formatTKDNPercent(snapshot.TotalTKDN), "B", "C", true)
	pdf.SetY(y + tkdnRowMM)
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
	lines := strings.Split(text, "\n")
	// Teks yang lebih lebar dari sel diperkecil (minimal 7 pt) agar tidak
	// meluber ke kolom sebelah, mis. merek "Pertamina Enduro".
	size := tkdnTablePt
	for ; size > 7; size -= 0.5 {
		pdf.SetFont(activityFont, style, size)
		fits := true
		for _, line := range lines {
			if pdf.GetStringWidth(line) > w-2*activitySignaturePadXMM {
				fits = false
			}
		}
		if fits {
			break
		}
	}
	pdf.SetFont(activityFont, style, size)
	lineHeight := size * 0.3528 * 1.2
	top := y + (h-float64(len(lines))*lineHeight)/2
	for i, line := range lines {
		lineX := x + activitySignaturePadXMM
		if align == "C" {
			lineX = x + (w-pdf.GetStringWidth(line))/2
		}
		pdf.Text(lineX, textBaseline(top+float64(i)*lineHeight, lineHeight, size), line)
	}
}
