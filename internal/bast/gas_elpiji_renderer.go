package bast

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	gasElpijiTitle          = "BERITA ACARA SURAT PENGANTAR PENGIRIMAN SERAH TERIMA"
	gasElpijiTitleSecond    = "TABUNG GAS ELPIJI"
	gasElpijiTitlePt        = 12.0
	gasElpijiTitleLineMM    = 5.6
	gasElpijiTextLineMM     = 5.4
	gasElpijiMetadataRowMM  = 6.6
	gasElpijiMetadataLabel  = 37.0
	gasElpijiSectionGapMM   = 9.0
	gasElpijiDetailColumnMM = 38.0
	gasElpijiDetailHeadMM   = 7.6
	gasElpijiSignColumnMM   = 55.0
	gasElpijiSignHeadMM     = 11.0
	gasElpijiSignSpaceMM    = 28.0
)

// RenderGasElpiji membuat BA Surat Pengantar Pengiriman Serah Terima Tabung Gas
// Elpiji, satu halaman per penerimaan sesuai referensi: kop logo, judul, sub
// judul pengadaan, lokasi gudang/alamat/PIC, pernyataan tanggal, tabel jumlah
// tabung, lalu tanda tangan Penerima dan PT Pertamina Patra Niaga. Satu
// penerimaan menghasilkan satu halaman; BA gabungan memuat semua halaman urut
// tanggal.
func RenderGasElpiji(snapshots []GasElpijiSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if len(snapshots) == 0 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshots[len(snapshots)-1].DocumentDate)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle(gasElpijiTitle+" "+gasElpijiTitleSecond, false)
	pdf.SetAuthor("KONKIT", false)
	registerArial(pdf)

	registerSignatoryImages(pdf, logoBytes)
	for i, snapshot := range snapshots {
		if snapshot.DocumentType != AggregateDocumentGasElpiji {
			return RenderedAggregate{}, ErrInvalidInput
		}
		// Satu halaman: bila sudah ditandatangani basah, pindaiannya menggantikan
		// seluruh halaman.
		scanKey := gasElpijiScanKey(snapshot.DocumentDate)
		if snapshot.Wet.Scanned {
			registerWetScanAs(pdf, logoBytes, scanKey)
			drawWetScanPageAs(pdf, "P", scanKey)
			continue
		}
		logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, i)
		if err != nil {
			return RenderedAggregate{}, err
		}
		pdf.AddPage()
		renderGasElpijiPage(pdf, snapshot, logos)
		drawWetFooter(pdf, snapshot.Wet, "BA Gas Elpiji", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

// gasElpijiScanKey adalah kunci gambar pindaian tanda tangan basah satu
// penerimaan (unik per tanggal agar BA gabungan dapat memuat semuanya).
func gasElpijiScanKey(date string) string { return wetScanImageKey + ":" + date }

func renderGasElpijiPage(pdf *fpdf.Fpdf, snapshot GasElpijiSnapshot, logos []registeredLogo) {
	contentWidth := pageWidthMM - 2*marginMM
	y := renderRakordaLogoStrip(pdf, logos) + 9

	pdf.SetFont(activityFont, "BU", gasElpijiTitlePt)
	for _, line := range []string{gasElpijiTitle, gasElpijiTitleSecond} {
		pdf.Text(marginMM+(contentWidth-pdf.GetStringWidth(line))/2, textBaseline(y, gasElpijiTitleLineMM, gasElpijiTitlePt), line)
		y += gasElpijiTitleLineMM
	}
	pdf.SetXY(marginMM, y+8)
	renderActivityProcurementHeading(pdf, contentWidth, snapshot.FiscalYear, activitySubtitlePt)

	// Lokasi gudang, alamat, dan PIC: label tebal, nilai dibungkus di kolom nilai.
	y = pdf.GetY() + gasElpijiSectionGapMM + 4
	valueX := marginMM + gasElpijiMetadataLabel + 4
	valueWidth := pageWidthMM - marginMM - valueX
	for _, item := range []struct{ label, value string }{
		{"Lokasi Gudang", snapshot.WarehouseLocation},
		{"Alamat", snapshot.WarehouseAddress},
		{"PIC", snapshot.PIC},
	} {
		pdf.SetFont(activityFont, "B", activityTextPt)
		baseline := textBaseline(y, gasElpijiMetadataRowMM, activityTextPt)
		pdf.Text(marginMM, baseline, item.label)
		pdf.Text(marginMM+gasElpijiMetadataLabel, baseline, ":")
		pdf.SetFont(activityFont, "", activityTextPt)
		lines := wrapWords(pdf, item.value, valueWidth)
		for j, line := range lines {
			pdf.Text(valueX, textBaseline(y+float64(j)*gasElpijiTextLineMM, gasElpijiMetadataRowMM, activityTextPt), line)
		}
		y += gasElpijiMetadataRowMM + float64(len(lines)-1)*gasElpijiTextLineMM
	}

	pdf.SetXY(marginMM, y+gasElpijiSectionGapMM)
	renderJustifiedSegments(pdf, marginMM, contentWidth, gasElpijiTextLineMM, activityTextPt, []textSegment{{text: gasElpijiStatement(snapshot.DocumentDate)}})

	pdf.SetY(pdf.GetY() + gasElpijiSectionGapMM)
	renderGasElpijiDetail(pdf, snapshot)
	pdf.SetY(pdf.GetY() + gasElpijiSectionGapMM + 4)
	renderGasElpijiSignatures(pdf, snapshot)
}

// gasElpijiStatement menulis kalimat pernyataan dengan hari dan tanggal
// penerimaan (huruf judul untuk nama hari/bulan, sesuai EYD).
func gasElpijiStatement(documentDate string) string {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		return "Pada hari ini telah dilakukan pengantaran tabung gas elpiji dan diterima, dengan rincian sebagai berikut:"
	}
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("Pada hari ini, %s, tanggal %d %s tahun %d, telah dilakukan pengantaran tabung gas elpiji dan diterima, dengan rincian sebagai berikut:", days[date.Weekday()], date.Day(), months[date.Month()], date.Year())
}

// renderGasElpijiDetail menggambar tabel kecil di tengah: jumlah tabung dan
// keterangan. Keterangan panjang dibungkus dan menambah tinggi baris.
func renderGasElpijiDetail(pdf *fpdf.Fpdf, snapshot GasElpijiSnapshot) {
	width := gasElpijiDetailColumnMM
	pad := activitySignaturePadXMM + 0.6
	pdf.SetFont(activityFont, "", activitySignaturePt+1)
	count := ""
	if snapshot.CylinderCount > 0 {
		count = formatThousands(snapshot.CylinderCount) + " Tabung"
	}
	remarks := wrapWords(pdf, snapshot.Remarks, width-2*pad)
	lineHeight := signatureLineHeight(activitySignaturePt + 1)
	rowHeight := maxFloat(gasElpijiDetailHeadMM, float64(len(remarks))*lineHeight+2*activitySignaturePadYMM+1)
	x := (pageWidthMM - 2*width) / 2
	y := pdf.GetY()

	pdf.SetFont(activityFont, "B", activitySignaturePt+1)
	for i, header := range []string{"Tabung Gas Elpiji", "Keterangan"} {
		left := x + float64(i)*width
		pdf.Rect(left, y, width, gasElpijiDetailHeadMM, "")
		pdf.Text(left+(width-pdf.GetStringWidth(header))/2, textBaseline(y, gasElpijiDetailHeadMM, activitySignaturePt+1), header)
	}
	y += gasElpijiDetailHeadMM
	pdf.SetFont(activityFont, "", activitySignaturePt+1)
	pdf.Rect(x, y, width, rowHeight, "")
	pdf.Rect(x+width, y, width, rowHeight, "")
	pdf.Text(x+(width-pdf.GetStringWidth(count))/2, textBaseline(y, rowHeight, activitySignaturePt+1), count)
	top := y + (rowHeight-float64(len(remarks))*lineHeight)/2
	for j, line := range remarks {
		pdf.Text(x+width+(width-pdf.GetStringWidth(line))/2, textBaseline(top+float64(j)*lineHeight, lineHeight, activitySignaturePt+1), line)
	}
	pdf.SetXY(marginMM, y+rowHeight)
}

// renderGasElpijiSignatures menggambar tabel tanda tangan dua kolom: Penerima
// (nama dan perusahaan pelaksana) dan PT Pertamina Patra Niaga (nama).
func renderGasElpijiSignatures(pdf *fpdf.Fpdf, snapshot GasElpijiSnapshot) {
	pdf.SetFont(activityFont, "", activitySignaturePt)
	company := textnorm.DisplayTitle(snapshot.ReceiverCompany)
	width := minFloat(maxFloat(gasElpijiSignColumnMM, pdf.GetStringWidth(company)+2*activitySignaturePadXMM+0.5), (pageWidthMM-2*marginMM)/2)
	usable := width - 2*activitySignaturePadXMM
	receiver := wrapWords(pdf, strings.TrimSpace("Nama : "+textnorm.DisplayTitle(snapshot.ReceiverName)), usable)
	if company != "" {
		receiver = append(append(receiver, ""), wrapWords(pdf, company, usable)...)
	}
	names := [][]string{receiver, wrapWords(pdf, strings.TrimSpace("Nama : "+textnorm.DisplayTitle(snapshot.PertaminaName)), usable)}
	lineHeight := activitySignatureLineHeight()
	nameHeight := tightCellHeight(names) + 1
	// Isian yang sangat panjang dapat mendorong tabel tanda tangan melewati
	// batas bawah; tabel itu lalu dipindah utuh ke halaman berikutnya.
	if pdf.GetY()+gasElpijiSignHeadMM+gasElpijiSignSpaceMM+nameHeight > pageHeightMM-bottomMarginMM {
		pdf.AddPage()
		pdf.SetY(topMarginMM)
	}
	x := (pageWidthMM - 2*width) / 2
	y := pdf.GetY()
	for i, header := range []string{"Penerima", "PT Pertamina Patra Niaga"} {
		left := x + float64(i)*width
		pdf.Rect(left, y, width, gasElpijiSignHeadMM, "")
		pdf.Text(left+(width-pdf.GetStringWidth(header))/2, textBaseline(y, gasElpijiSignHeadMM, activitySignaturePt), header)
		pdf.Rect(left, y+gasElpijiSignHeadMM, width, gasElpijiSignSpaceMM, "")
		role := signatureDocuments[gasElpijiDocumentKey][i]
		drawSignatoryMark(pdf, role, snapshot.SignatureMarks[role], left, y+gasElpijiSignHeadMM, width, gasElpijiSignSpaceMM)
		nameTop := y + gasElpijiSignHeadMM + gasElpijiSignSpaceMM
		pdf.Rect(left, nameTop, width, nameHeight, "")
		for j, text := range names[i] {
			pdf.Text(left+activitySignaturePadXMM, textBaseline(nameTop+activitySignaturePadYMM+0.5+float64(j)*lineHeight, lineHeight, activitySignaturePt), text)
		}
	}
	pdf.SetXY(marginMM, y+gasElpijiSignHeadMM+gasElpijiSignSpaceMM+nameHeight)
}
