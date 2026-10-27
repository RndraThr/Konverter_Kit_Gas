package bast

import (
	"bytes"
	"fmt"
	"strings"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	dailyRecapTitle    = "BERITA ACARA SERAH TERIMA"
	dailyRecapSubtitle = "(FORM REKAPITULASI PENERIMA PAKET)"
)

// Kolom: No., Nama Petani, No. Kartu Petani, Merek Mesin, Tipe Mesin, No. Seri
// Mesin. Jumlahnya persis lebar konten (pageWidthMM-2*marginMM = 178 mm).
var dailyRecapColumnWidths = []float64{12, 44, 36, 24, 32, 30}

var dailyRecapHeaders = []string{"No.", "Nama Petani", "No. Kartu Petani", "Merek Mesin", "Tipe Mesin", "No. Seri Mesin"}

var dailyRecapAligns = []string{"C", "L", "C", "C", "C", "C"}

// RenderDailyRecap mengikuti struktur dan ukuran huruf Sosialisasi s.d. TKDN
// (lihat renderFormalHeader): kop dan keterangan hanya di halaman pertama,
// tabel berlanjut di halaman yang hanya memuat logo, tanda tangan di halaman
// terakhir tersendiri.
func RenderDailyRecap(snapshot DailyRecapSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentDailyRecap {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseDailyRecapSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	registerArial(pdf)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("REKAP HARIAN "+snapshot.DocumentDate, false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}
	registerSignatoryImages(pdf, logoBytes)
	registerWetScan(pdf, logoBytes)
	// Cetak halaman tanda tangan saja: isi dokumen dilewati.
	if !snapshot.Wet.PageOnly {
		pdf.AddPage()
		renderFormalHeader(pdf, logos, formalHeader{
			title: dailyRecapTitle, subtitle: dailyRecapSubtitle, fiscal: snapshot.FiscalYear,
			metadata: []formalMeta{
				{"Hari / Tanggal", formatIndonesianDate(snapshot.DocumentDate)},
				{"Lokasi / Titik Serah", textnorm.DisplayTitle(snapshot.HandoverLocation)},
				{"Konsultan Distribusi", textnorm.DisplayTitle(snapshot.ConsultantCompanyName)},
			},
		})
		renderDailyRecapTable(pdf, snapshot, logos)
		renderDailyRecapGrandTotal(pdf, snapshot, logos)
	}
	signatories := snapshot.Signatories
	// Tanda tangan basah: halaman pindaian menggantikan halaman tanda tangan.
	if snapshot.Wet.Scanned {
		drawWetScanPage(pdf, "P")
	} else {
		renderFormalSignatures(pdf, logos, closingSignatureHeaders, []string{
			textnorm.DisplayTitle(signatories.AgricultureOfficeName), textnorm.DisplayTitle(signatories.InstallerName),
			textnorm.DisplayTitle(signatories.SupervisorName), textnorm.DisplayTitle(signatories.PertaminaRepName),
		}, signatories.AgricultureOfficeNIP, snapshot.SignatureMarks)
		drawWetFooter(pdf, snapshot.Wet, "Rekap Harian", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderDailyRecapTable(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot, logos []registeredLogo) {
	renderFormalTableHeader(pdf, dailyRecapColumnWidths, dailyRecapHeaders)
	for i, recipient := range snapshot.Recipients {
		// Tinggi baris dihitung dan diperiksa SEBELUM baris digambar, agar
		// baris terakhir di suatu halaman tidak pernah melewati margin bawah.
		row := layoutFormalRowGroup(pdf, dailyRecapColumnWidths, [][]string{dailyRecapRowCells(rowNumber(recipient.RowNumber, i), recipient)})
		if pdf.GetY()+row.height() > pageHeightMM-bottomMarginMM {
			addFormalPage(pdf, logos, activityTopGapMM)
			renderFormalTableHeader(pdf, dailyRecapColumnWidths, dailyRecapHeaders)
		}
		drawFormalRowGroup(pdf, dailyRecapColumnWidths, row, 0, dailyRecapAligns)
	}
}

func dailyRecapRowCells(number int, recipient dailyRecapRecipientSnapshot) []string {
	return []string{
		fmt.Sprintf("%d", number),
		textnorm.DisplayTitle(recipient.FullName),
		recipient.FarmerCardNumber,
		recipient.MachineBrand,
		recipient.MachineType,
		recipient.MachineSerial,
	}
}

// renderDailyRecapGrandTotal menggambar satu baris ringkasan per varian,
// menyambung langsung pada grid kolom tabel utama, sesuai dokumen referensi:
// "Grand Total" digabung vertikal pada kolom gabungan No./Nama Petani/No.
// Kartu Petani, label varian pada kolom Merek Mesin, deskripsi merek/tipe pada
// kolom Tipe Mesin, dan jumlah dalam satuan Set pada kolom No. Seri Mesin.
func renderDailyRecapGrandTotal(pdf *fpdf.Fpdf, snapshot DailyRecapSnapshot, logos []registeredLogo) {
	if len(snapshot.Variants) == 0 {
		return
	}
	mergedWidth := dailyRecapColumnWidths[0] + dailyRecapColumnWidths[1] + dailyRecapColumnWidths[2]
	widths := []float64{dailyRecapColumnWidths[3], dailyRecapColumnWidths[4], dailyRecapColumnWidths[5]}
	aligns := []string{"C", "C", "C"}
	rows := make([][]string, len(snapshot.Variants))
	heights := make([]float64, len(snapshot.Variants))
	total := 0.0
	for i, variant := range snapshot.Variants {
		rows[i] = []string{variant.Label, joinVariantDescription(variant.MachineBrand, variant.MachineType), fmt.Sprintf("%d Set", variant.Count)}
		heights[i] = formalCellHeight(pdf, widths, rows[i], "B", activityTableBodyPt, formalRowMinMM)
		total += heights[i]
	}
	// Seluruh baris varian dijaga dalam satu halaman agar sel "Grand Total"
	// dapat digabung vertikal sepanjang baris-baris itu.
	if pdf.GetY()+total > pageHeightMM-bottomMarginMM {
		addFormalPage(pdf, logos, activityTopGapMM)
		renderFormalTableHeader(pdf, dailyRecapColumnWidths, dailyRecapHeaders)
	}
	top := pdf.GetY()
	drawFormalCell(pdf, marginMM, top, mergedWidth, total, "Grand Total", "C", "B", activityTableBodyPt)
	y := top
	for i, cells := range rows {
		x := marginMM + mergedWidth
		for j, value := range cells {
			drawFormalCell(pdf, x, y, widths[j], heights[i], value, aligns[j], "B", activityTableBodyPt)
			x += widths[j]
		}
		y += heights[i]
	}
	pdf.SetXY(marginMM, y)
}

func joinVariantDescription(brand, typ string) string {
	brand = strings.TrimSpace(brand)
	typ = strings.TrimSpace(typ)
	if brand == "" {
		return typ
	}
	if typ == "" {
		return brand
	}
	return brand + " / " + typ
}

// rowNumber memakai nomor urut berjalan dari snapshot; snapshot lama tanpa
// nomor itu tetap bernomor 1, 2, 3, ... seperti sebelumnya.
func rowNumber(running, index int) int {
	if running > 0 {
		return running
	}
	return index + 1
}
