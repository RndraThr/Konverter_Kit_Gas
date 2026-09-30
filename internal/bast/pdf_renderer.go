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
	pageWidthMM  = 210.0
	pageHeightMM = 297.0
	marginMM     = 15.0
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
	for _, document := range documents {
		if document.LocalDate != input.LocalDate || document.Snapshot.LocalDate != input.LocalDate {
			return RenderedBundle{}, fmt.Errorf("%w: recipient date does not match bundle date", ErrInvalidInput)
		}
		if document.Snapshot.ProgramType != "farmer" {
			return RenderedBundle{}, ErrTemplateUnavailable
		}
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, 10, marginMM)
	pdf.SetAutoPageBreak(false, marginMM)
	pdf.SetTitle(strings.TrimSuffix(filename, ".pdf"), false)
	pdf.SetAuthor("KONKIT", false)
	creationDate, _ := time.Parse("2006-01-02", input.LocalDate)
	pdf.SetCreationDate(creationDate)
	pdf.SetCatalogSort(true)

	logos, err := registerLogos(pdf, documents[0].Snapshot.Profile.Logos, input.LogoBytes)
	if err != nil {
		return RenderedBundle{}, err
	}

	events := make([]string, 0, len(documents))
	pageRanges := make([]RenderedRecipientPages, 0, len(documents))
	for _, document := range documents {
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

func registerLogos(pdf *fpdf.Fpdf, snapshots []LogoSnapshot, logoBytes map[string][]byte) ([]registeredLogo, error) {
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%w: published profile has no logos", ErrInvalidInput)
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
		name := fmt.Sprintf("bast_logo_%d", i)
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

func renderRecipient(pdf *fpdf.Fpdf, document RecipientDocument, logos []registeredLogo, events *[]string) {
	snapshot := document.Snapshot
	renderHeader(pdf, snapshot.Profile, logos)

	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetX(marginMM)
	pdf.MultiCell(pageWidthMM-2*marginMM, 4, snapshot.Profile.ProcurementDescription, "", "C", false)
	pdf.Ln(2)

	labelValue(pdf, "No. BAST", snapshot.DocumentNumber)
	labelValue(pdf, "Tanggal", formatIndonesianDate(snapshot.LocalDate))
	pdf.Ln(1)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(0, 4.5, "Data Penerima:", "", 1, "L", false, 0, "")
	details := [][2]string{
		{"Nama", snapshot.Recipient.FullName},
		{"Alamat", snapshot.Recipient.Address},
		{"Desa/Kelurahan", snapshot.Recipient.Village},
		{"Kecamatan", snapshot.Recipient.District},
		{"Kota/Kabupaten", snapshot.Recipient.Regency},
		{"No. KTP", snapshot.Recipient.NIK},
		{"No. Kartu Petani", snapshot.Recipient.SectorIdentifier},
		{"No. HP", snapshot.Recipient.PhoneNumber},
	}
	for _, detail := range details {
		labelValue(pdf, detail[0], detail[1])
	}
	pdf.Ln(1)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(0, 4.5, "A. Data Paket Perdana yang akan diterima:", "", 1, "L", false, 0, "")
	renderEquipmentTables(pdf, snapshot.Equipment)
	pdf.Ln(1.5)
	renderComponentHeader(pdf)
	for _, component := range snapshot.Components {
		rowHeight := componentRowHeight(pdf, component)
		if pdf.GetY()+rowHeight > 271 {
			addContinuationPage(pdf, document.SlotNumber, snapshot.Profile, logos, events)
			renderComponentHeader(pdf)
		}
		renderComponentRow(pdf, component, rowHeight)
	}

	if pdf.GetY()+55 > pageHeightMM-marginMM {
		addContinuationPage(pdf, document.SlotNumber, snapshot.Profile, logos, events)
	}
	renderDeclarationAndSignatures(pdf, snapshot.Signatures)
}

func renderHeader(pdf *fpdf.Fpdf, profile ProfileSnapshot, logos []registeredLogo) {
	contentWidth := pageWidthMM - 2*marginMM
	gap := 5.0
	available := contentWidth - gap*float64(len(logos)-1)
	boxWidth := available / float64(len(logos))
	y := 9.0
	for i, logo := range logos {
		maxWidth := minFloat(logo.maxWidthMM, boxWidth)
		maxHeight := logo.maxHeightMM
		scale := minFloat(maxWidth/logo.widthMM, maxHeight/logo.heightMM)
		w := logo.widthMM * scale
		h := logo.heightMM * scale
		x := marginMM + float64(i)*(boxWidth+gap) + (boxWidth-w)/2
		pdf.ImageOptions(logo.name, x, y+(maxHeight-h)/2, w, h, false, logo.options, 0, "")
	}
	pdf.SetY(28)
	pdf.SetFont("Helvetica", "BU", 10)
	pdf.CellFormat(contentWidth, 4.5, profile.Title, "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.CellFormat(contentWidth, 4, profile.Subtitle, "", 1, "C", false, 0, "")
}

func addContinuationPage(pdf *fpdf.Fpdf, slot int, profile ProfileSnapshot, logos []registeredLogo, events *[]string) {
	pdf.AddPage()
	*events = append(*events, fmt.Sprintf("PAGE %d RECIPIENT %d CONTINUATION", pdf.PageNo(), slot))
	renderHeader(pdf, profile, logos)
	pdf.SetFont("Helvetica", "BI", 8)
	pdf.CellFormat(pageWidthMM-2*marginMM, 5, fmt.Sprintf("LANJUTAN - PENERIMA NOMOR %d", slot), "B", 1, "C", false, 0, "")
	pdf.Ln(2)
}

func labelValue(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetX(marginMM)
	pdf.CellFormat(31, 4, label, "", 0, "L", false, 0, "")
	pdf.CellFormat(4, 4, ":", "", 0, "C", false, 0, "")
	pdf.CellFormat(pageWidthMM-2*marginMM-35, 4, value, "B", 1, "L", false, 0, "")
}

func renderEquipmentTables(pdf *fpdf.Fpdf, equipment EquipmentSnapshot) {
	renderTable(pdf, []float64{55, 75, 35, 15}, []string{"Merk Mesin", "Tipe Mesin", "Serial Number", "Checklist"}, [][]string{{equipment.MachineBrand, equipment.MachineType, equipment.MachineSerial, checkMark(true)}})
	pdf.Ln(1.5)
	renderTable(pdf, []float64{48, 82, 35, 15}, []string{"Merk Selang Hisap dan Selang Buang", "Spesifikasi Selang Hisap dan Selang Buang", "Serial Number", "Checklist"}, [][]string{{equipment.HoseBrand, equipment.HoseSpec, equipment.HoseSerial, checkMark(true)}})
	pdf.Ln(1.5)
	renderTable(pdf, []float64{75, 75, 30}, []string{"Merk Konkit / Reducer", "Serial Number", "Checklist"}, [][]string{{equipment.ConverterBrand, equipment.ConverterSerial, checkMark(true)}})
}

func renderTable(pdf *fpdf.Fpdf, widths []float64, headers []string, rows [][]string) {
	renderFixedRow(pdf, widths, headers, 6, true)
	for _, row := range rows {
		height := 6.0
		for i, value := range row {
			lines := len(pdf.SplitLines([]byte(value), widths[i]-2))
			height = maxFloat(height, float64(lines)*3.5+1.5)
		}
		renderFixedRow(pdf, widths, row, height, false)
	}
}

func renderFixedRow(pdf *fpdf.Fpdf, widths []float64, values []string, height float64, bold bool) {
	x, y := pdf.GetXY()
	style := ""
	if bold {
		style = "B"
	}
	pdf.SetFont("Helvetica", style, 6.5)
	for i, width := range widths {
		pdf.Rect(x, y, width, height, "")
		pdf.SetXY(x+1, y+0.8)
		pdf.MultiCell(width-2, 3.2, values[i], "", "C", false)
		x += width
	}
	pdf.SetXY(marginMM, y+height)
}

func renderComponentHeader(pdf *fpdf.Fpdf) {
	renderFixedRow(pdf, []float64{105, 20, 25, 30}, []string{"Komponen Paket, Aksesoris & Kelengkapan", "Jumlah", "Satuan", "Checklist"}, 6, true)
}

func componentRowHeight(pdf *fpdf.Fpdf, component ComponentSnapshot) float64 {
	pdf.SetFont("Helvetica", "", 7)
	lines := len(pdf.SplitLines([]byte(component.Label), 103))
	return maxFloat(5, float64(lines)*3.5+1.5)
}

func renderComponentRow(pdf *fpdf.Fpdf, component ComponentSnapshot, height float64) {
	renderFixedRow(pdf, []float64{105, 20, 25, 30}, []string{component.Label, fmt.Sprintf("%d", component.Quantity), component.Unit, checkMark(component.Checked)}, height, false)
}

func renderDeclarationAndSignatures(pdf *fpdf.Fpdf, signatures SignatureSnapshot) {
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "", 7.5)
	declaration := "Dengan ini kami menyatakan bahwa seluruh material/produk/barang tercantum di atas telah diterima dan dapat berfungsi dengan baik dengan jumlah yang benar serta telah diperiksa dengan seksama oleh masing-masing pihak."
	pdf.MultiCell(pageWidthMM-2*marginMM, 4, declaration, "", "J", false)
	pdf.Ln(3)

	widths := []float64{60, 60, 60}
	renderFixedRow(pdf, widths, []string{"PENERIMA PAKET / PETANI", "PELAKSANA PEMASANGAN & PENDISTRIBUSIAN", "KONSULTAN PENGAWAS"}, 9, true)
	renderFixedRow(pdf, widths, []string{"", "", ""}, 22, false)
	renderFixedRow(pdf, widths, []string{"Nama: " + signatures.ReceiverName, "Nama: " + signatures.ExecutorName, "Nama: " + signatures.SupervisorName}, 7, false)
}

func formatIndonesianDate(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%02d %s %d", date.Day(), months[date.Month()], date.Year())
}

func checkMark(checked bool) string {
	if checked {
		return "V"
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
