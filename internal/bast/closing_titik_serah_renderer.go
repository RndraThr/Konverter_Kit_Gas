package bast

import (
	"bytes"
	"fmt"
	"time"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	closingTitle    = "BERITA ACARA SERAH TERIMA"
	closingSubtitle = "(FORM REKAPITULASI CLOSING LOKASI / TITIK SERAH)"
)

// Kolom: No., Hari & Tanggal (Closing Harian), Merek Mesin, Tipe Mesin
// (Per-Varian), Jumlah Paket (set), Keterangan. Jumlahnya persis lebar konten
// (pageWidthMM-2*marginMM = 178 mm).
var closingColumnWidths = []float64{12, 46, 28, 36, 26, 30}

var closingSignatureHeaders = []string{"DINAS PERTANIAN DAERAH", "PELAKSANA PEMASANGAN\nDAN PENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS", "PT PERTAMINA\nPATRA NIAGA"}

// RenderClosingTitikSerah mengikuti struktur Sosialisasi s.d. TKDN: kop,
// nomor, keterangan, dan paragraf pembuka hanya di halaman pertama; tabel
// berlanjut di halaman yang hanya memuat logo; tanda tangan di halaman
// terakhir tersendiri.
func RenderClosingTitikSerah(snapshot ClosingTitikSerahSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentClosingTitikSerah {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseClosingSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	registerArial(pdf)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("CLOSING TITIK SERAH "+snapshot.DocumentDate, false)
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
			title: closingTitle, subtitle: closingSubtitle, number: snapshot.DocumentNumber, fiscal: snapshot.FiscalYear,
			metadata: []formalMeta{
				{"Lokasi / Titik Serah", textnorm.DisplayTitle(snapshot.HandoverLocation)},
				{"Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)},
				{"Provinsi", textnorm.DisplayTitle(snapshot.ProvinceName)},
			},
			narrative: closingNarrative(snapshot.DocumentDate, snapshot.FiscalYear),
		})
		renderClosingTable(pdf, snapshot, logos)
	}
	// Tanda tangan basah: halaman pindaian menggantikan halaman tanda tangan.
	if snapshot.Wet.Scanned {
		drawWetScanPage(pdf, "P")
	} else {
		renderFormalSignatures(pdf, logos, closingSignatureHeaders, closingSignatoryNames(snapshot.Signatories), snapshot.Signatories.AgricultureOfficeNIP, snapshot.SignatureMarks)
		drawWetFooter(pdf, snapshot.Wet, "Closing Titik Serah", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func closingSignatoryNames(signatories closingSignatories) []string {
	return []string{textnorm.DisplayTitle(signatories.AgricultureOfficeName), textnorm.DisplayTitle(signatories.InstallerName), textnorm.DisplayTitle(signatories.SupervisorName), textnorm.DisplayTitle(signatories.PertaminaRepName)}
}

// closingNarrative menyusun paragraf pembuka Closing sesuai EYD: huruf
// kapital hanya di awal kalimat, nama hari/bulan, dan nama kegiatan
// pengadaan (kata tugas seperti "untuk", "bagi", "di" tetap kecil);
// "Liquefied Petroleum Gas" dimiringkan sebagai istilah asing. Tahun
// anggaran dari programs.fiscal_year.
func closingNarrative(documentDate string, fiscalYear int) []textSegment {
	date, err := time.Parse("2006-01-02", documentDate)
	if err != nil {
		date = time.Now()
	}
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return []textSegment{
		{text: fmt.Sprintf("Pada hari ini, %s, tanggal %d %s tahun %d, telah dilakukan pemasangan dan pendistribusian Paket Perdana ", days[date.Weekday()], date.Day(), months[date.Month()], date.Year())},
		{text: "Liquefied Petroleum Gas", style: "I"},
		{text: fmt.Sprintf(" (LPG) untuk Mesin Pompa Air bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga, dengan rincian sebagai berikut:", fiscalYear)},
	}
}

var closingTableAligns = []string{"C", "C", "C", "C", "C", "L"}

func renderClosingTable(pdf *fpdf.Fpdf, snapshot ClosingTitikSerahSnapshot, logos []registeredLogo) {
	headers := []string{"No.", "Hari & Tanggal\n(Closing Harian)", "Merek Mesin", "Tipe Mesin\n(Per-Varian)", "Jumlah Paket\n(set)", "Keterangan"}
	renderFormalTableHeader(pdf, closingColumnWidths, headers)
	// Varian pada tanggal yang sama digabung: kolom No. dan Hari & Tanggal
	// dicetak satu kali sepanjang baris-baris tanggal itu.
	for number, rows := range groupClosingRowsByDate(snapshot.Rows) {
		cells := make([][]string, len(rows))
		for i, row := range rows {
			cells[i] = closingRowCells(number+1, row)
		}
		group := layoutFormalRowGroup(pdf, closingColumnWidths, cells)
		if pdf.GetY()+group.height() > pageHeightMM-bottomMarginMM {
			addFormalPage(pdf, logos, activityTopGapMM)
			renderFormalTableHeader(pdf, closingColumnWidths, headers)
		}
		drawFormalRowGroup(pdf, closingColumnWidths, group, 2, closingTableAligns)
	}
	if pdf.GetY()+formalRowMinMM > pageHeightMM-bottomMarginMM {
		addFormalPage(pdf, logos, activityTopGapMM)
		renderFormalTableHeader(pdf, closingColumnWidths, headers)
	}
	drawFormalTotalRow(pdf, closingColumnWidths, 4, snapshot.GrandTotal)
}

// groupClosingRowsByDate mengelompokkan baris berurutan dengan tanggal sama.
func groupClosingRowsByDate(rows []ClosingRow) [][]ClosingRow {
	var groups [][]ClosingRow
	for _, row := range rows {
		if n := len(groups); n > 0 && groups[n-1][0].LocalDate == row.LocalDate {
			groups[n-1] = append(groups[n-1], row)
			continue
		}
		groups = append(groups, []ClosingRow{row})
	}
	return groups
}

func closingRowCells(number int, row ClosingRow) []string {
	return []string{
		fmt.Sprintf("%d.", number),
		formatIndonesianDate(row.LocalDate),
		row.MachineBrand,
		row.MachineType,
		fmt.Sprintf("%d", row.Count),
		"",
	}
}
