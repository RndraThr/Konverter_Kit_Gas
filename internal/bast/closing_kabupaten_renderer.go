package bast

import (
	"bytes"
	"fmt"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	closingKabupatenTitle    = "BERITA ACARA SERAH TERIMA"
	closingKabupatenSubtitle = "(FORM REKAPITULASI CLOSING KABUPATEN / KOTA)"
)

// Kolom: No., Lokasi / Titik Serah, Merek Mesin, Tipe Mesin (Per-Varian),
// Jumlah Paket (set), Keterangan. Jumlahnya persis lebar konten (178 mm).
var closingKabupatenColumnWidths = []float64{12, 46, 28, 36, 26, 30}

// RenderClosingKabupaten mengikuti struktur Sosialisasi s.d. TKDN, sama
// seperti Closing Titik Serah.
func RenderClosingKabupaten(snapshot ClosingKabupatenSnapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentClosingKabupaten {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseClosingKabupatenSnapshot(snapshot)
	pdf := fpdf.New("P", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	registerArial(pdf)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("CLOSING KABUPATEN "+snapshot.DocumentDate, false)
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
			title: closingKabupatenTitle, subtitle: closingKabupatenSubtitle, number: snapshot.DocumentNumber, fiscal: snapshot.FiscalYear,
			metadata: []formalMeta{
				{"Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)},
				{"Provinsi", textnorm.DisplayTitle(snapshot.ProvinceName)},
			},
			narrative: closingNarrative(snapshot.DocumentDate, snapshot.FiscalYear),
		})
		renderClosingKabupatenTable(pdf, snapshot, logos)
	}
	// Tanda tangan basah: halaman pindaian menggantikan halaman tanda tangan.
	if snapshot.Wet.Scanned {
		drawWetScanPage(pdf, "P")
	} else {
		renderFormalSignatures(pdf, logos, closingSignatureHeaders, closingSignatoryNames(snapshot.Signatories), snapshot.Signatories.AgricultureOfficeNIP, snapshot.SignatureMarks)
		drawWetFooter(pdf, snapshot.Wet, "Closing Kabupaten", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderClosingKabupatenTable(pdf *fpdf.Fpdf, snapshot ClosingKabupatenSnapshot, logos []registeredLogo) {
	headers := []string{"No.", "Lokasi / Titik Serah", "Merek Mesin", "Tipe Mesin\n(Per-Varian)", "Jumlah Paket\n(set)", "Keterangan"}
	renderFormalTableHeader(pdf, closingKabupatenColumnWidths, headers)
	// Varian pada lokasi yang sama digabung: kolom No. dan Lokasi / Titik
	// Serah dicetak satu kali sepanjang baris-baris lokasi itu.
	for number, rows := range groupClosingKabupatenRowsByLocation(snapshot.Rows) {
		cells := make([][]string, len(rows))
		for i, row := range rows {
			cells[i] = closingKabupatenRowCells(number+1, row)
		}
		group := layoutFormalRowGroup(pdf, closingKabupatenColumnWidths, cells)
		if pdf.GetY()+group.height() > pageHeightMM-bottomMarginMM {
			addFormalPage(pdf, logos, activityTopGapMM)
			renderFormalTableHeader(pdf, closingKabupatenColumnWidths, headers)
		}
		drawFormalRowGroup(pdf, closingKabupatenColumnWidths, group, 2, closingTableAligns)
	}
	if pdf.GetY()+formalRowMinMM > pageHeightMM-bottomMarginMM {
		addFormalPage(pdf, logos, activityTopGapMM)
		renderFormalTableHeader(pdf, closingKabupatenColumnWidths, headers)
	}
	drawFormalTotalRow(pdf, closingKabupatenColumnWidths, 4, snapshot.GrandTotal)
}

// groupClosingKabupatenRowsByLocation mengelompokkan baris berurutan dengan
// lokasi sama.
func groupClosingKabupatenRowsByLocation(rows []ClosingKabupatenRow) [][]ClosingKabupatenRow {
	var groups [][]ClosingKabupatenRow
	for _, row := range rows {
		if n := len(groups); n > 0 && groups[n-1][0].Location == row.Location {
			groups[n-1] = append(groups[n-1], row)
			continue
		}
		groups = append(groups, []ClosingKabupatenRow{row})
	}
	return groups
}

func closingKabupatenRowCells(number int, row ClosingKabupatenRow) []string {
	return []string{
		fmt.Sprintf("%d.", number),
		textnorm.DisplayTitle(row.Location),
		row.MachineBrand,
		row.MachineType,
		fmt.Sprintf("%d", row.Count),
		"",
	}
}
