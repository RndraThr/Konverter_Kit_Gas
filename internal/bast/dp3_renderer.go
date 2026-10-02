package bast

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

const (
	dp3LandscapeWidthMM  = 297.0
	dp3LandscapeHeightMM = 210.0
	dp3MarginMM          = 14.0
	dp3TopMarginMM       = 9.0
	dp3BottomMarginMM    = 12.0

	dp3Title      = "DAFTAR PENERIMA PAKET PERDANA"
	dp3Subtitle   = "(FORM DP3)"
	dp3MetaLabelW = 46.0
	dp3InfoRowMM  = 5.2
)

type RenderedAggregate struct {
	PDF       []byte
	PageCount int
}

// dp3ColumnWidths: No, Nama Petani, Alamat, No. KTP, Merek, Tipe, Daya, Jenis BBM, Paraf.
var dp3ColumnWidths = []float64{12, 42, 55, 34, 30, 30, 22, 22, 22}

func RenderDP3(snapshot DP3Snapshot, logoBytes map[string][]byte) (RenderedAggregate, error) {
	if snapshot.DocumentType != AggregateDocumentDP3 {
		return RenderedAggregate{}, ErrInvalidInput
	}
	snapshot = uppercaseDP3Snapshot(snapshot)
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(dp3MarginMM, dp3TopMarginMM, dp3MarginMM)
	pdf.SetAutoPageBreak(false, dp3MarginMM)
	pdf.SetTitle("DP3 "+snapshot.DocumentDate, false)
	pdf.SetAuthor("KONKIT", false)

	logos, err := registerLogos(pdf, snapshot.Logos, logoBytes, 0)
	if err != nil {
		return RenderedAggregate{}, err
	}

	contentWidth := dp3LandscapeWidthMM - 2*dp3MarginMM
	pdf.AddPage()
	renderDP3Header(pdf, snapshot, logos)
	renderDP3Table(pdf, snapshot, logos, contentWidth)

	// Halaman tanda tangan terpisah.
	pdf.AddPage()
	renderDP3Header(pdf, snapshot, logos)
	renderDP3Signatures(pdf, snapshot)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedAggregate{}, err
	}
	return RenderedAggregate{PDF: output.Bytes(), PageCount: pdf.PageNo()}, nil
}

func renderDP3Header(pdf *fpdf.Fpdf, snapshot DP3Snapshot, logos []registeredLogo) {
	contentWidth := dp3LandscapeWidthMM - 2*dp3MarginMM
	gap, stripY, stripHeight := 5.8, dp3TopMarginMM, 13.0
	widths := make([]float64, len(logos))
	heights := make([]float64, len(logos))
	for i, logo := range logos {
		maxWidth := logo.maxWidthMM
		if maxWidth <= 0 {
			maxWidth = 42
		}
		maxHeight := logo.maxHeightMM
		if maxHeight <= 0 {
			maxHeight = 13
		}
		scale := minFloat(maxWidth/logo.widthMM, maxHeight/logo.heightMM)
		widths[i], heights[i] = logo.widthMM*scale, logo.heightMM*scale
	}
	totalWidth := gap * float64(maxInt(0, len(logos)-1))
	for _, width := range widths {
		totalWidth += width
	}
	x := dp3MarginMM + (contentWidth-totalWidth)/2
	for i, logo := range logos {
		pdf.ImageOptions(logo.name, x, stripY+(stripHeight-heights[i])/2, widths[i], heights[i], false, logo.options, 0, "")
		x += widths[i] + gap
	}
	pdf.SetXY(dp3MarginMM, stripY+stripHeight+2)
	pdf.SetFont("Helvetica", "BU", 13)
	pdf.CellFormat(contentWidth, 6, dp3Title, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BI", 9.5)
	pdf.CellFormat(contentWidth, 4.4, dp3Subtitle, "", 1, "C", false, 0, "")
	pdf.Ln(1.4)

	renderProcurementHeading(pdf, dp3MarginMM, contentWidth, snapshot.FiscalYear)
	pdf.Ln(1.6)

	metadata := []struct{ label, value string }{
		{"Hari / Tanggal", formatIndonesianDate(snapshot.DocumentDate)},
		{"Lokasi / Titik Serah", snapshot.HandoverLocation},
		{"Kabupaten / Kota", snapshot.RegencyName},
		{"Konsultan Distribusi", snapshot.ConsultantCompanyName},
	}
	for _, item := range metadata {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetX(dp3MarginMM)
		pdf.CellFormat(dp3MetaLabelW, dp3InfoRowMM, item.label, "", 0, "L", false, 0, "")
		pdf.CellFormat(3, dp3InfoRowMM, ":", "", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(contentWidth-dp3MetaLabelW-3, dp3InfoRowMM, item.value, "", 1, "L", false, 0, "")
	}
	pdf.Ln(1.4)
}

func renderDP3Table(pdf *fpdf.Fpdf, snapshot DP3Snapshot, logos []registeredLogo, contentWidth float64) {
	renderDP3TableHeader(pdf)
	rows := snapshot.Recipients
	// Minimal lima baris visual; baris kosong hanya menjaga layout.
	visualRows := maxInt(5, len(rows))
	for i := 0; i < visualRows; i++ {
		var height float64
		if i < len(rows) {
			height = dp3RowHeight(pdf, rows[i])
		} else {
			height = minRowMM
		}
		// Tinggi baris dihitung dan diperiksa SEBELUM baris digambar, agar
		// baris terakhir di suatu halaman tidak pernah tergambar melewati
		// margin bawah.
		if pdf.GetY()+height > dp3LandscapeHeightMM-dp3BottomMarginMM {
			pdf.AddPage()
			renderDP3Header(pdf, snapshot, logos)
			renderDP3TableHeader(pdf)
		}
		if i < len(rows) {
			renderDP3Row(pdf, i+1, rows[i], height)
		} else {
			renderDP3EmptyRow(pdf, i+1)
		}
	}
}

func renderDP3TableHeader(pdf *fpdf.Fpdf) {
	x, y := dp3MarginMM, pdf.GetY()
	// Baris header atas: No, Nama Petani, Alamat, No. KTP, Data Mesin (span 4), Paraf.
	noW, nameW, addrW, nikW := dp3ColumnWidths[0], dp3ColumnWidths[1], dp3ColumnWidths[2], dp3ColumnWidths[3]
	machineW := dp3ColumnWidths[4] + dp3ColumnWidths[5] + dp3ColumnWidths[6] + dp3ColumnWidths[7]
	parafW := dp3ColumnWidths[8]
	drawCell(pdf, x, y, noW, 6, "No", "C", true)
	x += noW
	drawCell(pdf, x, y, nameW, 6, "Nama Petani", "C", true)
	x += nameW
	drawCell(pdf, x, y, addrW, 6, "Alamat", "C", true)
	x += addrW
	drawCell(pdf, x, y, nikW, 6, "No. KTP", "C", true)
	x += nikW
	drawCell(pdf, x, y, machineW, 6, "Data Mesin", "C", true)
	x += machineW
	drawCell(pdf, x, y, parafW, 6, "Paraf", "C", true)
	pdf.SetXY(dp3MarginMM, y+6)

	// Baris header bawah untuk sub-kolom Data Mesin.
	x = dp3MarginMM
	for i := 0; i < 4; i++ {
		drawCell(pdf, x, y+6, dp3ColumnWidths[i], 6, "", "C", true)
		x += dp3ColumnWidths[i]
	}
	subLabels := []string{"Merek", "Tipe", "Daya", "Jenis BBM"}
	for i, label := range subLabels {
		drawCell(pdf, x, y+6, dp3ColumnWidths[4+i], 6, label, "C", true)
		x += dp3ColumnWidths[4+i]
	}
	drawCell(pdf, x, y+6, parafW, 6, "", "C", true)
	pdf.SetXY(dp3MarginMM, y+12)
}

// dp3RowCells membangun nilai sel satu baris penerima, dipakai bersama oleh
// penghitung tinggi (dp3RowHeight) dan penggambar (renderDP3Row) agar kedua
// langkah itu selalu konsisten terhadap konten yang sama.
func dp3RowCells(number int, recipient dp3RecipientSnapshot) []string {
	address := joinAddress(recipient.Address, recipient.Village, recipient.District, recipient.Regency)
	return []string{
		fmt.Sprintf("%d", number),
		recipient.FullName,
		address,
		recipient.NIK,
		recipient.MachineBrand,
		recipient.MachineType,
		recipient.MachinePower,
		recipient.MachineFuelType,
		"",
	}
}

// dp3RowHeight menghitung tinggi baris sebelum digambar, sehingga pemeriksaan
// batas halaman di renderDP3Table selalu akurat.
func dp3RowHeight(pdf *fpdf.Fpdf, recipient dp3RecipientSnapshot) float64 {
	cells := dp3RowCells(0, recipient)
	rowHeight := minRowMM
	pdf.SetFont("Helvetica", "", tableFontPt)
	for i, value := range cells {
		lines := len(pdf.SplitLines([]byte(value), dp3ColumnWidths[i]-2*cellPadMM))
		rowHeight = maxFloat(rowHeight, float64(lines)*tableLineMM+2)
	}
	return rowHeight
}

func renderDP3Row(pdf *fpdf.Fpdf, number int, recipient dp3RecipientSnapshot, rowHeight float64) {
	x, y := dp3MarginMM, pdf.GetY()
	cells := dp3RowCells(number, recipient)
	aligns := []string{"C", "L", "L", "C", "C", "C", "C", "C", "C"}
	for i, value := range cells {
		drawCell(pdf, x, y, dp3ColumnWidths[i], rowHeight, value, aligns[i], false)
		x += dp3ColumnWidths[i]
	}
	pdf.SetXY(dp3MarginMM, y+rowHeight)
}

func renderDP3EmptyRow(pdf *fpdf.Fpdf, number int) {
	x, y := dp3MarginMM, pdf.GetY()
	for i := 0; i < len(dp3ColumnWidths); i++ {
		value := ""
		if i == 0 {
			value = fmt.Sprintf("%d", number)
		}
		drawCell(pdf, x, y, dp3ColumnWidths[i], minRowMM, value, "C", false)
		x += dp3ColumnWidths[i]
	}
	pdf.SetXY(dp3MarginMM, y+minRowMM)
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

func renderDP3Signatures(pdf *fpdf.Fpdf, snapshot DP3Snapshot) {
	contentWidth := dp3LandscapeWidthMM - 2*dp3MarginMM
	sigTop := dp3LandscapeHeightMM - dp3BottomMarginMM - 52
	if pdf.GetY() < sigTop {
		pdf.SetY(sigTop)
	}
	widths := []float64{contentWidth / 3, contentWidth / 3, contentWidth / 3}
	headers := []string{"DINAS PERTANIAN DAERAH", "PELAKSANA PEMASANGAN\nDAN PENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS"}
	names := []string{snapshot.Signatories.AgricultureOfficeName, snapshot.Signatories.InstallerName, snapshot.Signatories.SupervisorName}
	nips := []string{snapshot.Signatories.AgricultureOfficeNIP, "", ""}

	x, y := dp3MarginMM, pdf.GetY()
	headerHeight := 12.0
	for i := range widths {
		drawCell(pdf, x, y, widths[i], headerHeight, headers[i], "C", true)
		x += widths[i]
	}
	pdf.SetXY(dp3MarginMM, y+headerHeight)

	x = dp3MarginMM
	spaceHeight := 24.0
	for i := range widths {
		drawCell(pdf, x, y+headerHeight, widths[i], spaceHeight, "", "C", false)
		x += widths[i]
	}
	pdf.SetXY(dp3MarginMM, y+headerHeight+spaceHeight)

	x = dp3MarginMM
	for i := range widths {
		dp3FitText(pdf, x, y+headerHeight+spaceHeight, widths[i], 6.5, "Nama : "+strings.TrimSpace(names[i]))
		x += widths[i]
	}
	pdf.SetXY(dp3MarginMM, y+headerHeight+spaceHeight+6.5)

	x = dp3MarginMM
	for i := range widths {
		nipText := ""
		if strings.TrimSpace(nips[i]) != "" {
			nipText = "NIP : " + strings.TrimSpace(nips[i])
		}
		dp3FitText(pdf, x, y+headerHeight+spaceHeight+6.5, widths[i], 5.5, nipText)
		x += widths[i]
	}
	pdf.SetXY(dp3MarginMM, y+headerHeight+spaceHeight+12)
}

// dp3FitText menggambar kotak dan teks rata kiri untuk Nama/NIP. Pertama
// mencoba mengecilkan ukuran huruf (sampai minimum 6,5pt) agar tetap satu
// baris; jika pada ukuran minimum teks masih lebih lebar dari kotak, teks
// dibungkus ke beberapa baris (bukan dibiarkan meluber ke kolom sebelah).
// Ini mencegah nama/NIP panjang terpotong atau keluar kotak, sesuai dokumen
// referensi DP3 dan Rekap Harian.
func dp3FitText(pdf *fpdf.Fpdf, x, y, w, h float64, text string) {
	pdf.Rect(x, y, w, h, "")
	if text == "" {
		return
	}
	usableWidth := w - 2*cellPadMM
	size := tableFontPt
	for ; size > 6.5; size -= 0.5 {
		pdf.SetFont("Helvetica", "", size)
		if pdf.GetStringWidth(text) <= usableWidth {
			break
		}
	}
	pdf.SetFont("Helvetica", "", size)
	if pdf.GetStringWidth(text) <= usableWidth {
		pdf.SetXY(x+cellPadMM, y+maxFloat(0.3, (h-tableLineMM)/2))
		pdf.CellFormat(usableWidth, tableLineMM, text, "", 0, "L", false, 0, "")
		return
	}
	lines := pdf.SplitLines([]byte(text), usableWidth)
	top := y + maxFloat(0.3, (h-float64(len(lines))*tableLineMM)/2)
	pdf.SetXY(x+cellPadMM, top)
	pdf.MultiCell(usableWidth, tableLineMM, text, "", "L", false)
}
