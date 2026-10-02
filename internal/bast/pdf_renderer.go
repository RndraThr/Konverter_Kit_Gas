package bast

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

const (
	pageWidthMM          = 210.0
	pageHeightMM         = 297.0
	marginMM             = 16.0
	topMarginMM          = 11.0
	bottomMarginMM       = 10.0
	baPeroranganTitle    = "BERITA ACARA SERAH TERIMA"
	baPeroranganSubtitle = "(FORM PENERIMA PAKET)"
	declarationText      = "Dengan ini kami menyatakan bahwa Seluruh Material/Produk/Barang tercantum diatas telah diterima dan dapat berfungsi dengan baik dengan jumlah yang benar serta telah diperiksa dengan seksama oleh masing-masing pihak"

	tableFontPt   = 8.5
	tableLineMM   = 3.6
	cellPadMM     = 1.6
	minRowMM      = 5.8
	infoRowMM     = 4.9
	signatureHMM  = 36.0
	checkToken    = "\u2713"
	hoseLabelPair = "Panjang Selang Hisap"
)

type BundleRenderInput struct {
	LocalDate string
	Documents []RecipientDocument
	LogoBytes map[string][]byte
}

type RenderedBundle struct {
	PDF        []byte
	PageCount  int
	Filename   string
	Events     []string
	Recipients []RenderedRecipientPages
}

type RenderedRecipientPages struct {
	SlotNumber int
	PageStart  int
	PageEnd    int
}

type registeredLogo struct {
	name        string
	options     fpdf.ImageOptions
	widthMM     float64
	heightMM    float64
	maxWidthMM  float64
	maxHeightMM float64
}

type textSegment struct {
	text  string
	style string
}

func BundleFilename(localDate string) (string, error) {
	date, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", fmt.Errorf("invalid bundle date: %w", err)
	}
	days := [...]string{"MINGGU", "SENIN", "SELASA", "RABU", "KAMIS", "JUMAT", "SABTU"}
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	return fmt.Sprintf("%s, %02d %s %d.pdf", days[date.Weekday()], date.Day(), months[date.Month()], date.Year()), nil
}

func RenderPetaniBundle(input BundleRenderInput) (RenderedBundle, error) {
	filename, err := BundleFilename(input.LocalDate)
	if err != nil {
		return RenderedBundle{}, err
	}
	if len(input.Documents) == 0 {
		return RenderedBundle{}, fmt.Errorf("%w: bundle has no recipients", ErrInvalidInput)
	}

	documents := append([]RecipientDocument(nil), input.Documents...)
	sort.SliceStable(documents, func(i, j int) bool { return documents[i].SlotNumber < documents[j].SlotNumber })
	for index, document := range documents {
		if document.LocalDate != input.LocalDate || document.Snapshot.LocalDate != input.LocalDate {
			return RenderedBundle{}, fmt.Errorf("%w: recipient date does not match bundle date", ErrInvalidInput)
		}
		if document.Snapshot.ProgramType != "farmer" {
			return RenderedBundle{}, ErrTemplateUnavailable
		}
		documents[index] = uppercaseRecipientDocument(document)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, topMarginMM, marginMM)
	pdf.SetAutoPageBreak(false, marginMM)
	pdf.SetTitle(strings.TrimSuffix(filename, ".pdf"), false)
	pdf.SetAuthor("KONKIT", false)
	creationDate, _ := time.Parse("2006-01-02", input.LocalDate)
	pdf.SetCreationDate(creationDate)
	pdf.SetCatalogSort(true)

	logosByRender := make(map[string][]registeredLogo)
	for _, document := range documents {
		key := renderLogoKey(document.Snapshot.Render)
		if _, exists := logosByRender[key]; exists {
			continue
		}
		logos, err := registerLogos(pdf, document.Snapshot.Render.Logos, input.LogoBytes, len(logosByRender))
		if err != nil {
			return RenderedBundle{}, err
		}
		logosByRender[key] = logos
	}

	events := make([]string, 0, len(documents))
	pageRanges := make([]RenderedRecipientPages, 0, len(documents))
	for _, document := range documents {
		logos := logosByRender[renderLogoKey(document.Snapshot.Render)]
		pdf.AddPage()
		pageStart := pdf.PageNo()
		events = append(events, fmt.Sprintf("PAGE %d RECIPIENT %d START", pdf.PageNo(), document.SlotNumber))
		renderRecipient(pdf, document, logos, &events)
		pageRanges = append(pageRanges, RenderedRecipientPages{SlotNumber: document.SlotNumber, PageStart: pageStart, PageEnd: pdf.PageNo()})
		if pdf.Err() {
			return RenderedBundle{}, pdf.Error()
		}
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return RenderedBundle{}, err
	}
	return RenderedBundle{PDF: output.Bytes(), PageCount: pdf.PageNo(), Filename: filename, Events: events, Recipients: pageRanges}, nil
}

func registerLogos(pdf *fpdf.Fpdf, snapshots []LogoSnapshot, logoBytes map[string][]byte, profileIndex int) ([]registeredLogo, error) {
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%w: render identity has no logos", ErrBrandingNotConfigured)
	}
	ordered := append([]LogoSnapshot(nil), snapshots...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].SortOrder < ordered[j].SortOrder })
	registered := make([]registeredLogo, 0, len(ordered))
	for i, logo := range ordered {
		data := logoBytes[logo.AssetID]
		if len(data) == 0 {
			data = logoBytes[logo.StorageKey]
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("%w: logo %s is unavailable", ErrInvalidInput, logo.AssetID)
		}
		imageType := "PNG"
		if logo.MimeType == "image/jpeg" || logo.MimeType == "image/jpg" {
			imageType = "JPG"
		}
		options := fpdf.ImageOptions{ImageType: imageType, ReadDpi: true}
		name := fmt.Sprintf("bast_logo_%d_%d", profileIndex, i)
		info := pdf.RegisterImageOptionsReader(name, options, bytes.NewReader(data))
		if pdf.Err() || info == nil {
			return nil, fmt.Errorf("invalid logo %s: %w", logo.AssetID, pdf.Error())
		}
		width, height := info.Extent()
		if width <= 0 || height <= 0 {
			return nil, fmt.Errorf("%w: logo %s has invalid dimensions", ErrInvalidInput, logo.AssetID)
		}
		maxWidth := logo.MaxWidthMM
		if maxWidth <= 0 {
			maxWidth = 42
		}
		maxHeight := logo.MaxHeightMM
		if maxHeight <= 0 {
			maxHeight = 16
		}
		registered = append(registered, registeredLogo{name: name, options: options, widthMM: width, heightMM: height, maxWidthMM: maxWidth, maxHeightMM: maxHeight})
	}
	return registered, nil
}

func renderLogoKey(render RenderIdentity) string {
	var key strings.Builder
	for _, logo := range render.Logos {
		fmt.Fprintf(&key, "%s:%s:%d|", logo.AssetID, logo.StorageKey, logo.SortOrder)
	}
	return key.String()
}

func renderRecipient(pdf *fpdf.Fpdf, document RecipientDocument, logos []registeredLogo, events *[]string) {
	snapshot := document.Snapshot
	renderHeader(pdf, logos)

	renderProcurementHeading(pdf, marginMM, pageWidthMM-2*marginMM, snapshot.Render.FiscalYear)
	pdf.Ln(2.8)

	labelValue(pdf, "No. BAST", snapshot.DocumentNumber, false)
	labelValue(pdf, "Hari / Tanggal", formatIndonesianDate(snapshot.LocalDate), false)
	pdf.Ln(2)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetX(marginMM)
	pdf.CellFormat(0, 4.8, "A.  Data Penerima", "", 1, "L", false, 0, "")
	details := []struct {
		label string
		value string
		bold  bool
	}{
		{"Nama", snapshot.Recipient.FullName, true},
		{"Alamat", snapshot.Recipient.Address, false},
		{"Desa", snapshot.Recipient.Village, false},
		{"Kecamatan", snapshot.Recipient.District, false},
		{"Kota/Kabupaten", snapshot.Recipient.Regency, false},
		{"No. KTP", snapshot.Recipient.NIK, false},
		{"No. Kartu Petani", snapshot.Recipient.SectorIdentifier, false},
		{"No. HP", snapshot.Recipient.PhoneNumber, false},
	}
	for _, detail := range details {
		labelValue(pdf, detail.label, detail.value, detail.bold)
	}
	pdf.Ln(3)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetX(marginMM)
	pdf.CellFormat(0, 4.8, "B.  Data Paket Perdana yang akan diterima", "", 1, "L", false, 0, "")
	pdf.Ln(0.8)
	renderEquipmentTables(pdf, snapshot.Equipment)
	pdf.Ln(1.5)
	renderComponentHeader(pdf)
	for _, component := range snapshot.Components {
		rowHeight := componentRowHeight(pdf, component)
		if pdf.GetY()+rowHeight > 271 {
			addContinuationPage(pdf, document.SlotNumber, logos, events)
			renderComponentHeader(pdf)
		}
		renderComponentRow(pdf, component, rowHeight)
	}

	pdf.SetFont("Helvetica", "", 9)
	declLines := len(pdf.SplitLines([]byte(declarationText), pageWidthMM-2*marginMM))
	needed := 3 + float64(declLines)*4.1 + signatureHMM
	if pdf.GetY()+needed > pageHeightMM-bottomMarginMM {
		addContinuationPage(pdf, document.SlotNumber, logos, events)
	}
	renderDeclarationAndSignatures(pdf, snapshot.Signatures)
}

// renderProcurementHeading menggambar uraian pengadaan baku (tiga baris,
// dengan "LIQUEFIED PETROLEUM GAS" dicetak miring) terpusat dalam contentWidth
// yang dimulai pada marginX. Dipakai oleh BA Perorangan (portrait) dan DP3
// (landscape) agar uraiannya tetap identik di seluruh jenis BA.
func renderProcurementHeading(pdf *fpdf.Fpdf, marginX, contentWidth float64, fiscalYear int) {
	lines := [][]textSegment{
		{{text: "PENGADAAN BARANG PENYEDIAAN DAN PENDISTRIBUSIAN PAKET PERDANA ", style: "B"}, {text: "LIQUEFIED", style: "BI"}},
		{{text: "PETROLEUM GAS", style: "BI"}, {text: " (LPG) UNTUK MESIN POMPA AIR BAGI PETANI SASARAN", style: "B"}},
		{{text: fmt.Sprintf("TAHUN ANGGARAN %d DI PT PERTAMINA PATRA NIAGA", fiscalYear), style: "B"}},
	}
	const fontSize = 9.5
	const lineHeight = 4.4
	for _, line := range lines {
		width := 0.0
		for _, segment := range line {
			pdf.SetFont("Helvetica", segment.style, fontSize)
			width += pdf.GetStringWidth(segment.text)
		}
		y := pdf.GetY()
		pdf.SetXY(marginX+(contentWidth-width)/2, y)
		for _, segment := range line {
			pdf.SetFont("Helvetica", segment.style, fontSize)
			segmentWidth := pdf.GetStringWidth(segment.text)
			pdf.CellFormat(segmentWidth, lineHeight, segment.text, "", 0, "L", false, 0, "")
		}
		pdf.SetXY(marginX, y+lineHeight)
	}
}

func renderHeader(pdf *fpdf.Fpdf, logos []registeredLogo) {
	contentWidth, gap, stripY, stripHeight := pageWidthMM-2*marginMM, 5.8, topMarginMM, 17.5
	widths := make([]float64, len(logos))
	heights := make([]float64, len(logos))
	referenceHeights := []float64{10.6, 15.9, 13.8, 14.3}
	for i, logo := range logos {
		maxHeight, maxWidth := logo.maxHeightMM, logo.maxWidthMM
		if len(logos) == len(referenceHeights) {
			maxHeight, maxWidth = referenceHeights[i], 45
		}
		scale := minFloat(maxWidth/logo.widthMM, maxHeight/logo.heightMM)
		widths[i], heights[i] = logo.widthMM*scale, logo.heightMM*scale
	}
	totalWidth := gap * float64(maxInt(0, len(logos)-1))
	for _, width := range widths {
		totalWidth += width
	}
	x := marginMM + (contentWidth-totalWidth)/2
	for i, logo := range logos {
		pdf.ImageOptions(logo.name, x, stripY+(stripHeight-heights[i])/2, widths[i], heights[i], false, logo.options, 0, "")
		x += widths[i] + gap
	}
	pdf.SetXY(marginMM, 33.8)
	pdf.SetFont("Helvetica", "B", 12.75)
	pdf.CellFormat(contentWidth, 5.6, baPeroranganTitle, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "BI", 9)
	pdf.CellFormat(contentWidth, 4.2, baPeroranganSubtitle, "", 1, "C", false, 0, "")
	pdf.Ln(2.2)
}

func addContinuationPage(pdf *fpdf.Fpdf, slot int, logos []registeredLogo, events *[]string) {
	pdf.AddPage()
	*events = append(*events, fmt.Sprintf("PAGE %d RECIPIENT %d CONTINUATION", pdf.PageNo(), slot))
	renderHeader(pdf, logos)
	pdf.SetFont("Helvetica", "BI", 8)
	pdf.CellFormat(pageWidthMM-2*marginMM, 5, fmt.Sprintf("LANJUTAN - PENERIMA NOMOR %d", slot), "B", 1, "C", false, 0, "")
	pdf.Ln(2)
}

func labelValue(pdf *fpdf.Fpdf, label, value string, bold bool) {
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetX(marginMM)
	pdf.CellFormat(29.6, infoRowMM, label, "", 0, "L", false, 0, "")
	pdf.CellFormat(3.7, infoRowMM, ":", "", 0, "C", false, 0, "")
	style := ""
	if bold {
		style = "B"
	}
	pdf.SetFont("Helvetica", style, 9)
	pdf.CellFormat(pageWidthMM-2*marginMM-33.3, infoRowMM, value, "", 1, "L", false, 0, "")
}

func renderEquipmentTables(pdf *fpdf.Fpdf, equipment EquipmentSnapshot) {
	renderTable(pdf, []float64{53.4, 53.4, 49.8, 21.4}, []string{"Merek Mesin", "Tipe Mesin", "Serial Number", "Checklist"}, [][]string{{equipment.MachineBrand, equipment.MachineType, equipment.MachineSerial, checkMark(true)}})
	pdf.Ln(1.5)
	renderHoseTable(pdf, equipment)
	pdf.Ln(1.5)
	renderTable(pdf, []float64{53.4, 103.2, 21.4}, []string{"Merek Konkit / Reducer", "Serial Number", "Checklist"}, [][]string{{equipment.ConverterBrand, equipment.ConverterSerial, checkMark(true)}})
}

func renderHoseTable(pdf *fpdf.Fpdf, equipment EquipmentSnapshot) {
	widths := []float64{53.4, 53.4, 49.8, 21.4}
	renderFixedRow(pdf, widths, []string{"Merek Selang Hisap Dan Selang Buang", "Spesifikasi Selang Hisap dan Selang Buang", "Serial Number", "Checklist"}, nil, 10.5, true)

	brands, specs := splitHosePair(equipment.HoseBrand), splitHosePair(equipment.HoseSpec)
	x, y, rowHeight := marginMM, pdf.GetY(), minRowMM

	pdf.SetFont("Helvetica", "", tableFontPt)
	labelWidth := pdf.GetStringWidth(hoseLabelPair) + 1.2
	labels := [2]string{"Panjang Selang Hisap", "Panjang Selang Buang"}

	for row := 0; row < 2; row++ {
		rowY := y + float64(row)*rowHeight
		drawCell(pdf, x, rowY, widths[0], rowHeight, brands[row], "C", false)
		specX := x + widths[0]
		pdf.Rect(specX, rowY, widths[1], rowHeight, "")
		pdf.SetFont("Helvetica", "", tableFontPt)
		pdf.SetXY(specX+cellPadMM, rowY+(rowHeight-tableLineMM)/2)
		pdf.CellFormat(labelWidth, tableLineMM, labels[row], "", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1]-2*cellPadMM-labelWidth, tableLineMM, ": "+specs[row], "", 0, "L", false, 0, "")
	}
	spanX := x + widths[0] + widths[1]
	drawCell(pdf, spanX, y, widths[2], rowHeight*2, equipment.HoseSerial, "C", false)
	drawCell(pdf, spanX+widths[2], y, widths[3], rowHeight*2, checkMark(true), "C", false)
	pdf.SetXY(marginMM, y+rowHeight*2)
}

func splitHosePair(value string) [2]string {
	for _, separator := range []string{"\n", ";", " / "} {
		parts := strings.SplitN(value, separator, 2)
		if len(parts) == 2 {
			return [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])}
		}
	}
	value = strings.TrimSpace(value)
	return [2]string{value, value}
}

func renderTable(pdf *fpdf.Fpdf, widths []float64, headers []string, rows [][]string) {
	renderFixedRow(pdf, widths, headers, nil, minRowMM, true)
	for _, row := range rows {
		pdf.SetFont("Helvetica", "", tableFontPt)
		height := minRowMM
		for i, value := range row {
			lines := len(pdf.SplitLines([]byte(value), widths[i]-2*cellPadMM))
			height = maxFloat(height, float64(lines)*tableLineMM+2)
		}
		renderFixedRow(pdf, widths, row, nil, height, false)
	}
}

func renderFixedRow(pdf *fpdf.Fpdf, widths []float64, values []string, aligns []string, height float64, bold bool) {
	x, y := marginMM, pdf.GetY()
	for i, width := range widths {
		align := "C"
		if i < len(aligns) && aligns[i] != "" {
			align = aligns[i]
		}
		drawCell(pdf, x, y, width, height, values[i], align, bold)
		x += width
	}
	pdf.SetXY(marginMM, y+height)
}

func drawCell(pdf *fpdf.Fpdf, x, y, w, h float64, text, align string, bold bool) {
	pdf.Rect(x, y, w, h, "")
	if text == "" {
		return
	}
	if text == checkToken {
		pdf.SetFont("ZapfDingbats", "", 8)
		pdf.SetXY(x, y+(h-tableLineMM)/2)
		pdf.CellFormat(w, tableLineMM, "3", "", 0, "C", false, 0, "")
		return
	}
	style := ""
	if bold {
		style = "B"
	}
	pdf.SetFont("Helvetica", style, tableFontPt)
	lines := pdf.SplitLines([]byte(text), w-2*cellPadMM)
	textHeight := float64(len(lines)) * tableLineMM
	pdf.SetXY(x+cellPadMM, y+maxFloat(0.3, (h-textHeight)/2))
	pdf.MultiCell(w-2*cellPadMM, tableLineMM, text, "", align, false)
}

var componentWidths = []float64{106.8, 21.4, 28.5, 21.3}

func renderComponentHeader(pdf *fpdf.Fpdf) {
	renderFixedRow(pdf, componentWidths, []string{"Komponen Paket, Aksesoris & Kelengkapan", "Jumlah", "Satuan", "Checklist"}, nil, minRowMM, true)
}

func componentRowHeight(pdf *fpdf.Fpdf, component ComponentSnapshot) float64 {
	pdf.SetFont("Helvetica", "", tableFontPt)
	lines := len(pdf.SplitLines([]byte(component.Label), componentWidths[0]-2*cellPadMM))
	return maxFloat(minRowMM, float64(lines)*tableLineMM+2)
}

func renderComponentRow(pdf *fpdf.Fpdf, component ComponentSnapshot, height float64) {
	renderFixedRow(pdf, componentWidths,
		[]string{component.Label, fmt.Sprintf("%d", component.Quantity), component.Unit, checkMark(component.Checked)},
		[]string{"L", "C", "C", "C"}, height, false)
}

func renderDeclarationAndSignatures(pdf *fpdf.Fpdf, signatures SignatureSnapshot) {
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetX(marginMM)
	pdf.MultiCell(pageWidthMM-2*marginMM, 4.1, declarationText, "", "J", false)

	sigTop := pageHeightMM - bottomMarginMM - signatureHMM
	if pdf.GetY() < sigTop {
		pdf.SetY(sigTop)
	}

	widths := []float64{57, 64, 57}
	renderFixedRow(pdf, widths, []string{"PENERIMA PAKET / PETANI", "PELAKSANA PEMASANGAN\nDAN PENDISTRIBUSIAN", "KONSULTAN\nPENGAWAS"}, nil, 11, true)
	renderFixedRow(pdf, widths, []string{"", "", ""}, nil, 15.5, false)
	renderFixedRow(pdf, widths, []string{signatures.ReceiverName, signatures.ExecutorName, signatures.SupervisorName}, nil, 9.5, true)
}

func formatIndonesianDate(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	days := [...]string{"MINGGU", "SENIN", "SELASA", "RABU", "KAMIS", "JUMAT", "SABTU"}
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	return fmt.Sprintf("%s, %d %s %d", days[date.Weekday()], date.Day(), months[date.Month()], date.Year())
}

func checkMark(checked bool) string {
	if checked {
		return checkToken
	}
	return ""
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
