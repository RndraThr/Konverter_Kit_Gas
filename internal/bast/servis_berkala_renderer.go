package bast

import (
	"bytes"
	"fmt"
	"strings"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	servisBerkalaTitle      = "BERITA ACARA AGENDA SERVIS BERKALA"
	servisSignatureTableMM  = 122.0
	servisSignatureSpaceMM  = 29.0
	servisListNumberIndent  = 7.0
	servisListTextIndent    = 13.0
	servisParagraphGapMM    = 2.0
	servisSectionGapMM      = 10.0
	servisSignatureGapMM    = 12.0
	servisTextLineMM        = 5.4
	servisMetadataRowMM     = 6.0
	servisMetadataLabelMM   = 40.0
	servisDocumentNumberRow = 6.0
)

// RenderServisBerkala membuat BA Agenda Servis Berkala satu halaman sesuai
// referensi: kop logo, judul, sub judul pengadaan, nomor dokumen, kabupaten dan
// provinsi, pernyataan, jadwal servis ke-1 dan ke-2, lalu tanda tangan
// Pelaksana dan Dinas. Tipografi mengikuti BA kegiatan (Arial).
func RenderServisBerkala(snapshot ServisBerkalaSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentServisBerkala {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(servisBerkalaTitle, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	pdf.AddPage()
	contentWidth := pageWidthMM - 2*marginMM
	textLeft := marginMM + activityColumnWidths[0]
	textWidth := contentWidth - 2*activityColumnWidths[0]

	pdf.SetXY(marginMM, renderRakordaLogoStrip(pdf, logos)+3)
	pdf.SetFont(activityFont, "BU", activityTitlePt)
	pdf.CellFormat(contentWidth, 7, servisBerkalaTitle, "", 1, "C", false, 0, "")
	pdf.Ln(4)
	renderActivityProcurementHeading(pdf, contentWidth, snapshot.FiscalYear, activitySubtitlePt)

	pdf.SetFont(activityFont, "", activityTextPt)
	y := pdf.GetY()
	number := "No. " + snapshot.DocumentNumber
	pdf.Text(marginMM+(contentWidth-pdf.GetStringWidth(number))/2, textBaseline(y, servisDocumentNumberRow, activityTextPt), number)
	y += servisDocumentNumberRow + servisSectionGapMM

	for _, item := range []struct{ label, value string }{
		{"Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)},
		{"Provinsi", textnorm.DisplayTitle(snapshot.ProvinceName)},
	} {
		baseline := textBaseline(y, servisMetadataRowMM, activityTextPt)
		pdf.Text(textLeft, baseline, item.label)
		pdf.Text(textLeft+servisMetadataLabelMM, baseline, ":")
		pdf.Text(textLeft+servisMetadataLabelMM+3.5, baseline, item.value)
		y += servisMetadataRowMM
	}
	pdf.SetXY(textLeft, y+servisSectionGapMM)

	renderJustifiedSegments(pdf, textLeft, textWidth, servisTextLineMM, activityTextPt, servisBerkalaNarrative(snapshot.FiscalYear, snapshot.ZoneName))
	pdf.SetFont(activityFont, "", activityTextPt)
	y = pdf.GetY()
	pdf.Text(textLeft, textBaseline(y, servisTextLineMM, activityTextPt), "Adapun Jadwal Servis Berkala akan dilaksanakan:")
	y += servisTextLineMM + servisParagraphGapMM

	for i, period := range snapshot.Services {
		pdf.Text(textLeft+servisListNumberIndent, textBaseline(y, servisTextLineMM, activityTextPt), fmt.Sprintf("%d.", i+1))
		pdf.Text(textLeft+servisListTextIndent, textBaseline(y, servisTextLineMM, activityTextPt), fmt.Sprintf("Servis ke-%d", i+1))
		y += servisTextLineMM
		pdf.Text(textLeft+servisListTextIndent, textBaseline(y, servisTextLineMM, activityTextPt), formatServisRange(period))
		y += servisTextLineMM + servisParagraphGapMM
	}

	pdf.SetY(y + servisSignatureGapMM)
	renderServisSignatures(pdf, snapshot)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

// servisBerkalaNarrative menyusun paragraf "Sehubungan dengan ..." sesuai
// referensi: uraian pengadaan sampai nama zona ditebalkan, "Liquefied
// Petroleum Gas" tebal-miring, sisanya huruf biasa.
func servisBerkalaNarrative(fiscalYear int, zoneName string) []textSegment {
	return []textSegment{
		{text: "Sehubungan dengan ", style: ""},
		{text: "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana ", style: "B"},
		{text: "Liquefied Petroleum Gas", style: "BI"},
		{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga (Termasuk Pendistribusian Dan Pemasangan) %s.", fiscalYear, zoneWithWords(titleCaseWords(zoneName))), style: "B"},
		{text: " Pelaksana Pekerjaan memberikan Servis Berkala sebanyak 2 (dua) kali dalam 1 (satu) tahun terhadap komponen mesin pompa air dan konverter kit sejak didistribusikan.", style: ""},
	}
}

// renderServisSignatures menggambar tabel tanda tangan dua pihak (Pelaksana
// dan Dinas) di tengah halaman dengan jarak teks tipis ke garis, memakai
// ukuran dan pembungkusan yang sama dengan BA kegiatan.
func renderServisSignatures(pdf *fpdf.Fpdf, snapshot ServisBerkalaSnapshot) {
	// Nama perusahaan pelaksana selalu satu baris pada 10 pt: kolom (keduanya
	// sama lebar) dilebarkan secukupnya bila nama perusahaan lebih panjang dari
	// lebar standar, dibatasi lebar konten halaman.
	pdf.SetFont(activityFont, "", activitySignaturePt)
	company := textnorm.DisplayTitle(snapshot.ConsultantCompanyName)
	width := minFloat(maxFloat(servisSignatureTableMM/2, pdf.GetStringWidth(company)+2*activitySignaturePadXMM+0.5), (pageWidthMM-2*marginMM)/2)
	usable := width - 2*activitySignaturePadXMM
	headers := [][]string{
		splitCellLines(pdf, "PELAKSANA PEMASANGAN &\nPENDISTRIBUSIAN", usable),
		splitCellLines(pdf, "DINAS YANG MEMBIDANGI\nPERTANIAN DAERAH", usable),
	}
	pdf.SetFont(activityFont, "", activitySignaturePt)
	installer := wrapWords(pdf, strings.TrimSpace("Nama : "+textnorm.DisplayTitle(snapshot.Signatories.InstallerName)), usable)
	// Baris nama perusahaan pelaksana (di bawah nama) ditulis rata tengah seperti
	// referensi; baris sebelum indeks ini rata kiri.
	centerFrom := []int{len(installer), -1}
	if company != "" {
		installer = append(installer, company)
	}
	dinas := append(wrapWords(pdf, strings.TrimSpace("Nama : "+textnorm.DisplayTitle(snapshot.Signatories.AgricultureOfficeName)), usable), strings.TrimSpace("NIP : "+snapshot.Signatories.AgricultureOfficeNIP))
	names := [][]string{installer, dinas}

	headHeight, nameHeight := tightCellHeight(headers), tightCellHeight(names)
	lineHeight := activitySignatureLineHeight()
	y := pdf.GetY()
	x := (pageWidthMM - 2*width) / 2
	pdf.SetFont(activityFont, "", activitySignaturePt)
	for i := range headers {
		pdf.Rect(x, y, width, headHeight, "")
		headTop := y + (headHeight-float64(len(headers[i]))*lineHeight)/2
		for j, text := range headers[i] {
			pdf.Text(x+(width-pdf.GetStringWidth(text))/2, textBaseline(headTop+float64(j)*lineHeight, lineHeight, activitySignaturePt), text)
		}
		pdf.Rect(x, y+headHeight, width, servisSignatureSpaceMM, "")
		nameTop := y + headHeight + servisSignatureSpaceMM
		pdf.Rect(x, nameTop, width, nameHeight, "")
		for j, text := range names[i] {
			textX := x + activitySignaturePadXMM
			if centerFrom[i] >= 0 && j >= centerFrom[i] {
				textX = x + (width-pdf.GetStringWidth(text))/2
			}
			pdf.Text(textX, textBaseline(nameTop+activitySignaturePadYMM+float64(j)*lineHeight, lineHeight, activitySignaturePt), text)
		}
		x += width
	}
	pdf.SetXY(marginMM, y+headHeight+servisSignatureSpaceMM+nameHeight)
}
