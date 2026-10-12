package bast

import (
	"bytes"
	"fmt"
	"strings"

	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

const (
	dp3LandscapeWidthMM  = 297.0
	dp3LandscapeHeightMM = 210.0

	dp3Title    = "DAFTAR PENERIMA PAKET PERDANA"
	dp3Subtitle = "(FORM DP3)"

	dp3HeaderRowMM = 7.0
)

type RenderedAggregate struct {
	PDF       []byte
	PageCount int
}

// dp3ColumnWidths: No., Nama Petani, Alamat, No. KTP, Merek, Tipe, Daya,
// Jenis BBM, Paraf. Jumlahnya persis lebar konten A4 mendatar
// (dp3LandscapeWidthMM-2*marginMM = 265 mm).
var dp3ColumnWidths = []float64{12, 44, 56, 38, 24, 30, 17, 24, 20}

var dp3Aligns = []string{"C", "L", "L", "C", "C", "C", "C", "C", "C"}

// RenderDP3 memakai ukuran huruf dan struktur yang sama dengan Sosialisasi
// s.d. TKDN pada A4 mendatar: kop dan keterangan hanya di halaman pertama,
// tabel berlanjut di halaman yang hanya memuat logo dan judul kolom, tanda
// tangan di halaman terakhir tersendiri.
func RenderDP3(snapshot DP3Snapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentDP3 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseDP3Snapshot(snapshot)
	pdf := fpdf.New("L", "mm", "A4", "")
	fixPDFDates(pdf, snapshot.DocumentDate)
	registerArial(pdf)
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, bottomMarginMM)
	pdf.SetTitle("DP3 "+snapshot.DocumentDate, false)
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
			title: dp3Title, subtitle: dp3Subtitle, fiscal: snapshot.FiscalYear, pageWidth: dp3LandscapeWidthMM, flushLeft: true,
			metadata: []formalMeta{
				{"Hari / Tanggal", formatIndonesianDate(snapshot.DocumentDate)},
				{"Lokasi / Titik Serah", textnorm.DisplayTitle(snapshot.HandoverLocation)},
				{"Kabupaten / Kota", textnorm.DisplayTitle(snapshot.RegencyName)},
				{"Konsultan Distribusi", textnorm.DisplayTitle(snapshot.ConsultantCompanyName)},
			},
		})
		renderDP3Table(pdf, snapshot, logos)
	}

	// Tanda tangan di halaman terakhir tersendiri, paling atas, seperti
	// Sosialisasi; kop dan keterangan dokumen tidak diulang.
	signature := layoutSignatures(pdf, dp3LandscapeWidthMM-2*marginMM, []string{"DINAS PERTANIAN DAERAH", "PELAKSANA PEMASANGAN\nDAN PENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS"}, []string{
		textnorm.DisplayTitle(snapshot.Signatories.AgricultureOfficeName), textnorm.DisplayTitle(snapshot.Signatories.InstallerName), textnorm.DisplayTitle(snapshot.Signatories.SupervisorName),
	}, snapshot.Signatories.AgricultureOfficeNIP).withMarks(signatureDocuments["dp3"], snapshot.SignatureMarks)
	// Tanda tangan basah: halaman pindaian menggantikan halaman tanda tangan.
	if snapshot.Wet.Scanned {
		drawWetScanPage(pdf, "L")
	} else {
		addDP3ContinuationPage(pdf, logos, activitySignatureGap)
		renderSignaturesAt(pdf, signature, marginMM)
		drawWetFooter(pdf, snapshot.Wet, "DP3", snapshot.DocumentDate)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

// addDP3ContinuationPage membuka halaman mendatar yang hanya memuat logo.
func addDP3ContinuationPage(pdf *fpdf.Fpdf, logos []registeredLogo, gap float64) {
	pdf.AddPage()
	pdf.SetXY(marginMM, formalLogoStrip(pdf, logos, dp3LandscapeWidthMM)+gap)
}

func renderDP3Table(pdf *fpdf.Fpdf, snapshot DP3Snapshot, logos []registeredLogo) {
	renderDP3TableHeader(pdf)
	rows := snapshot.Recipients
	// Minimal lima baris visual; baris kosong hanya menjaga layout.
	visualRows := maxInt(5, len(rows))
	for i := 0; i < visualRows; i++ {
		cells := []string{fmt.Sprintf("%d", i+1), "", "", "", "", "", "", "", ""}
		if i < len(rows) {
			cells = dp3RowCells(i+1, rows[i])
		}
		row := layoutFormalRowGroup(pdf, dp3ColumnWidths, [][]string{cells})
		// Tinggi baris dihitung dan diperiksa SEBELUM baris digambar, agar
		// baris terakhir di suatu halaman tidak pernah melewati margin bawah.
		if pdf.GetY()+row.height() > dp3LandscapeHeightMM-bottomMarginMM {
			addDP3ContinuationPage(pdf, logos, activityTopGapMM)
			renderDP3TableHeader(pdf)
		}
		drawFormalRowGroup(pdf, dp3ColumnWidths, row, 0, dp3Aligns)
	}
}

func renderDP3TableHeader(pdf *fpdf.Fpdf) {
	x, y := marginMM, pdf.GetY()
	cell := func(x, y, w, h float64, label string) {
		drawFormalCell(pdf, x, y, w, h, label, "C", "B", activityTableHeadPt)
	}
	// No., Nama Petani, Alamat, dan No. KTP digabung dua baris; Data Mesin
	// membawahi empat sub-kolom; Paraf juga digabung dua baris. Tinggi baris
	// sub-kolom mengikuti isinya agar judul tidak pernah meluber dari sel.
	subLabels := []string{"Merek", "Tipe", "Daya", "Jenis BBM"}
	subHeight := formalCellHeight(pdf, dp3ColumnWidths[4:8], subLabels, "B", activityTableHeadPt, dp3HeaderRowMM)
	total := dp3HeaderRowMM + subHeight
	for i, label := range []string{"No.", "Nama Petani", "Alamat", "No. KTP"} {
		cell(x, y, dp3ColumnWidths[i], total, label)
		x += dp3ColumnWidths[i]
	}
	machineW := dp3ColumnWidths[4] + dp3ColumnWidths[5] + dp3ColumnWidths[6] + dp3ColumnWidths[7]
	cell(x, y, machineW, dp3HeaderRowMM, "Data Mesin")
	for i, label := range subLabels {
		cell(x, y+dp3HeaderRowMM, dp3ColumnWidths[4+i], subHeight, label)
		x += dp3ColumnWidths[4+i]
	}
	cell(x, y, dp3ColumnWidths[8], total, "Paraf")
	pdf.SetXY(marginMM, y+total)
}

// dp3RowCells membangun nilai sel satu baris penerima, dipakai bersama oleh
// penghitung tinggi (dp3RowHeight) dan penggambar (renderDP3Row) agar kedua
// langkah itu selalu konsisten terhadap konten yang sama.
func dp3RowCells(number int, recipient dp3RecipientSnapshot) []string {
	address := textnorm.DisplayTitle(joinAddress(recipient.Address, recipient.Village, recipient.District, recipient.Regency))
	return []string{
		fmt.Sprintf("%d", number),
		textnorm.DisplayTitle(recipient.FullName),
		address,
		recipient.NIK,
		recipient.MachineBrand,
		recipient.MachineType,
		recipient.MachinePower,
		recipient.MachineFuelType,
		"",
	}
}

func joinAddress(address, village, district, regency string) string {
	parts := []string{}
	for _, value := range []string{address, village, district, regency} {
		value = strings.TrimSpace(value)
		if value != "" && value != "null" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ", ")
}
