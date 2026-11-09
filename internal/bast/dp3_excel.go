package bast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// Excel DP3: data penerima satu jadwal kabupaten dalam satu workbook, tanpa
// status apa pun. Isinya penerima yang distribusinya selesai, bernomor sama
// dengan DP3 dan Rekap Harian. Server membangunnya ulang berkala dan
// mengunggahnya ke folder BA/1. DP3 kabupaten hanya bila datanya berubah;
// file lama di Drive dihapus setelah file baru tersimpan.

// dp3ExcelLayout ikut masuk sidik isi: ubah nilainya saat tata letak
// workbook berubah, agar semua file dibangun ulang dengan tata letak baru.
const dp3ExcelLayout = "dp3-excel-v1"

// DP3ExcelSyncInterval is how often the server rebuilds changed workbooks.
const DP3ExcelSyncInterval = 10 * time.Minute

type DP3ExcelContext struct {
	ScheduleID   string
	ProgramName  string
	ProgramType  string
	FiscalYear   int
	RegencyName  string
	ProvinceName string
	ZoneName     string
}

// DP3ExcelDetail is the recipient data of one slot that DP3Recipient lacks.
type DP3ExcelDetail struct {
	DistributionDate string
	FarmerCardNumber string
	PhoneNumber      string
	MachineSerial    string
	ConverterBrand   string
	ConverterSerial  string
	HoseSerial       string
}

// DP3ExcelRow is one recipient line of the workbook.
type DP3ExcelRow struct {
	RowNumber          int    `json:"row_number"`
	FullName           string `json:"full_name"`
	NIK                string `json:"nik"`
	CardNumber         string `json:"card_number"`
	PhoneNumber        string `json:"phone_number"`
	Address            string `json:"address"`
	Village            string `json:"village"`
	District           string `json:"district"`
	DistributionDate   string `json:"distribution_date"`
	HandoverLocation   string `json:"handover_location"`
	DistributionNumber int    `json:"distribution_number"`
	MachineBrand       string `json:"machine_brand"`
	MachineType        string `json:"machine_type"`
	MachinePower       string `json:"machine_power"`
	MachineFuelType    string `json:"machine_fuel_type"`
	MachineSerial      string `json:"machine_serial"`
	ConverterBrand     string `json:"converter_brand"`
	ConverterSerial    string `json:"converter_serial"`
	HoseSerial         string `json:"hose_serial"`
}

// DP3ExcelExport is the stored state of one schedule's workbook.
type DP3ExcelExport struct {
	ScheduleID      string
	Filename        string
	StorageKey      string
	ContentChecksum string
	RecipientCount  int
	ByteSize        int64
	SyncedAt        *time.Time
	LastError       string
	LastAttemptAt   time.Time
}

// DP3ExcelStatus is what the DP3 page shows about the workbook.
type DP3ExcelStatus struct {
	Available      bool       `json:"available"`
	Filename       string     `json:"filename"`
	RecipientCount int        `json:"recipient_count"`
	SyncedAt       *time.Time `json:"synced_at"`
	LastError      string     `json:"last_error"`
	LastAttemptAt  *time.Time `json:"last_attempt_at"`
}

type DP3ExcelContent struct {
	Filename string
	Reader   io.ReadCloser
}

type dp3ExcelRepository interface {
	ListDP3ExcelScheduleIDs(context.Context) ([]string, error)
	GetDP3ExcelContext(context.Context, string, auth.RegencyScope) (DP3ExcelContext, error)
	ListDP3Recipients(context.Context, string, string, auth.RegencyScope) ([]DP3Recipient, error)
	ListDP3ExcelDetails(context.Context, string) (map[string]DP3ExcelDetail, error)
	GetDP3ExcelExport(context.Context, string, auth.RegencyScope) (DP3ExcelExport, error)
	SaveDP3ExcelExport(context.Context, DP3ExcelExport) (string, error)
	MarkDP3ExcelChecked(context.Context, string) error
	RecordDP3ExcelFailure(context.Context, string, string) error
}

type DP3ExcelService struct {
	repository dp3ExcelRepository
	storage    media.Storage
	location   *time.Location
	now        func() time.Time
	// mu keeps the worker and a manual sync from uploading the same schedule twice.
	mu sync.Mutex
}

func NewDP3ExcelService(repository dp3ExcelRepository, storage media.Storage, location *time.Location) *DP3ExcelService {
	if location == nil {
		location = time.UTC
	}
	return &DP3ExcelService{repository: repository, storage: storage, location: location, now: time.Now}
}

var unrestrictedScope = auth.RegencyScope{Unrestricted: true}

func (s *DP3ExcelService) Status(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3ExcelStatus, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3ExcelContext(ctx, scheduleID, scope); err != nil {
		return DP3ExcelStatus{}, err
	}
	export, err := s.repository.GetDP3ExcelExport(ctx, scheduleID, scope)
	if errors.Is(err, ErrNotFound) {
		return DP3ExcelStatus{}, nil
	}
	if err != nil {
		return DP3ExcelStatus{}, err
	}
	attempt := export.LastAttemptAt
	return DP3ExcelStatus{
		Available: export.StorageKey != "", Filename: export.Filename, RecipientCount: export.RecipientCount,
		SyncedAt: export.SyncedAt, LastError: export.LastError, LastAttemptAt: &attempt,
	}, nil
}

func (s *DP3ExcelService) Open(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3ExcelContent, error) {
	export, err := s.repository.GetDP3ExcelExport(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return DP3ExcelContent{}, err
	}
	if export.StorageKey == "" {
		return DP3ExcelContent{}, ErrNotFound
	}
	reader, err := s.storage.Open(ctx, export.StorageKey)
	if err != nil {
		return DP3ExcelContent{}, fmt.Errorf("open DP3 excel: %w", err)
	}
	return DP3ExcelContent{Filename: export.Filename, Reader: reader}, nil
}

// Sync rebuilds one schedule's workbook now, for a user in scope.
func (s *DP3ExcelService) Sync(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3ExcelStatus, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3ExcelContext(ctx, scheduleID, scope); err != nil {
		return DP3ExcelStatus{}, err
	}
	if _, err := s.syncSchedule(ctx, scheduleID); err != nil {
		return DP3ExcelStatus{}, err
	}
	return s.Status(ctx, scheduleID, scope)
}

// SyncAll rebuilds every changed workbook. A failing schedule is recorded
// and does not stop the others.
func (s *DP3ExcelService) SyncAll(ctx context.Context) (uploaded int, err error) {
	ids, err := s.repository.ListDP3ExcelScheduleIDs(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return uploaded, ctx.Err()
		}
		changed, syncErr := s.syncSchedule(ctx, id)
		if syncErr != nil {
			log.Printf("dp3 excel: schedule %s: %v", id, syncErr)
			continue
		}
		if changed {
			uploaded++
		}
	}
	return uploaded, nil
}

// Run syncs every interval until ctx ends.
func (s *DP3ExcelService) Run(ctx context.Context, interval time.Duration) {
	for {
		if uploaded, err := s.SyncAll(ctx); err != nil && ctx.Err() == nil {
			log.Printf("dp3 excel: %v", err)
		} else if uploaded > 0 {
			log.Printf("dp3 excel: %d workbook(s) updated", uploaded)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// syncSchedule uploads the workbook when its data differs from the stored
// one. changed is true when a new file was uploaded.
func (s *DP3ExcelService) syncSchedule(ctx context.Context, scheduleID string) (changed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil && ctx.Err() == nil {
			_ = s.repository.RecordDP3ExcelFailure(context.Background(), scheduleID, err.Error())
		}
	}()
	info, err := s.repository.GetDP3ExcelContext(ctx, scheduleID, unrestrictedScope)
	if err != nil {
		return false, err
	}
	rows, err := s.rows(ctx, scheduleID)
	if err != nil {
		return false, err
	}
	checksum, err := dp3ExcelChecksum(info, rows)
	if err != nil {
		return false, err
	}
	existing, err := s.repository.GetDP3ExcelExport(ctx, scheduleID, unrestrictedScope)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	hasFile := err == nil && existing.StorageKey != ""
	if (hasFile && existing.ContentChecksum == checksum) || (!hasFile && len(rows) == 0) {
		if err == nil {
			return false, s.repository.MarkDP3ExcelChecked(ctx, scheduleID)
		}
		return false, nil
	}
	data, err := buildDP3Excel(info, rows, s.now().In(s.location))
	if err != nil {
		return false, err
	}
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: info.ProgramType, ZoneName: info.ZoneName, RegencyName: info.RegencyName,
		Category: media.FolderBA, Child: dp3Folder,
	})
	if err != nil {
		return false, err
	}
	filename := formatDP3ExcelFilename(info.RegencyName)
	storageKey, size, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), filename, folderPath, bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("upload DP3 excel: %w", err)
	}
	oldKey, err := s.repository.SaveDP3ExcelExport(ctx, DP3ExcelExport{
		ScheduleID: scheduleID, Filename: filename, StorageKey: storageKey, ContentChecksum: checksum,
		RecipientCount: len(rows), ByteSize: size,
	})
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return false, err
	}
	if oldKey != "" && oldKey != storageKey {
		if deleteErr := s.storage.Delete(ctx, oldKey); deleteErr != nil {
			log.Printf("dp3 excel: schedule %s: remove old workbook %s: %v", scheduleID, oldKey, deleteErr)
		}
	}
	return true, nil
}

func (s *DP3ExcelService) rows(ctx context.Context, scheduleID string) ([]DP3ExcelRow, error) {
	recipients, err := s.repository.ListDP3Recipients(ctx, scheduleID, "", unrestrictedScope)
	if err != nil {
		return nil, err
	}
	details, err := s.repository.ListDP3ExcelDetails(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	rows := make([]DP3ExcelRow, 0, len(recipients))
	for _, recipient := range recipients {
		detail := details[recipient.SlotID]
		row := DP3ExcelRow{
			RowNumber: recipient.RowNumber, FullName: recipient.FullName, NIK: recipient.NIK, CardNumber: detail.FarmerCardNumber,
			PhoneNumber: detail.PhoneNumber, Address: recipient.Address, Village: recipient.Village, District: recipient.District,
			DistributionDate: detail.DistributionDate, HandoverLocation: recipient.HandoverLocation,
			MachineBrand: recipient.MachineBrand, MachineType: recipient.MachineType, MachinePower: recipient.MachinePower,
			MachineFuelType: recipient.MachineFuelType, MachineSerial: detail.MachineSerial,
			ConverterBrand: detail.ConverterBrand, ConverterSerial: detail.ConverterSerial, HoseSerial: detail.HoseSerial,
		}
		if recipient.DistributionNumber != nil {
			row.DistributionNumber = *recipient.DistributionNumber
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func dp3ExcelChecksum(info DP3ExcelContext, rows []DP3ExcelRow) (string, error) {
	payload, err := json.Marshal(struct {
		Layout       string        `json:"layout"`
		ProgramName  string        `json:"program_name"`
		ProgramType  string        `json:"program_type"`
		FiscalYear   int           `json:"fiscal_year"`
		RegencyName  string        `json:"regency_name"`
		ProvinceName string        `json:"province_name"`
		ZoneName     string        `json:"zone_name"`
		Rows         []DP3ExcelRow `json:"rows"`
	}{dp3ExcelLayout, info.ProgramName, info.ProgramType, info.FiscalYear, info.RegencyName, info.ProvinceName, info.ZoneName, rows})
	if err != nil {
		return "", fmt.Errorf("encode DP3 excel checksum: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func formatDP3ExcelFilename(regencyName string) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("DP3 - %s - Data Penerima.xlsx", name)
}

const dp3ExcelSheet = "DP3"

// dp3ExcelHeaderRow is the table header; rows above it hold the title block.
const dp3ExcelHeaderRow = 6

type dp3ExcelColumn struct {
	title string
	width float64
	text  bool // stored as text so leading zeros and long digits survive
	value func(DP3ExcelRow) any
}

func dp3ExcelColumns(programType string) []dp3ExcelColumn {
	cardTitle := "No. Kartu Tani"
	if programType == "fisherman" {
		cardTitle = "No. KUSUKA"
	}
	return []dp3ExcelColumn{
		{"No", 6, false, func(r DP3ExcelRow) any { return r.RowNumber }},
		{"Nama Penerima", 28, false, func(r DP3ExcelRow) any { return r.FullName }},
		{"NIK", 20, true, func(r DP3ExcelRow) any { return r.NIK }},
		{cardTitle, 20, true, func(r DP3ExcelRow) any { return r.CardNumber }},
		{"No. HP", 16, true, func(r DP3ExcelRow) any { return r.PhoneNumber }},
		{"Alamat", 32, false, func(r DP3ExcelRow) any { return r.Address }},
		{"Desa/Kelurahan", 18, false, func(r DP3ExcelRow) any { return r.Village }},
		{"Kecamatan", 18, false, func(r DP3ExcelRow) any { return r.District }},
		{"Tanggal Distribusi", 14, false, func(r DP3ExcelRow) any { return dp3ExcelDate(r.DistributionDate) }},
		{"Titik Serah", 22, false, func(r DP3ExcelRow) any { return r.HandoverLocation }},
		{"No. Bagi", 9, false, func(r DP3ExcelRow) any { return dp3ExcelNumber(r.DistributionNumber) }},
		{"Merek Mesin", 14, false, func(r DP3ExcelRow) any { return r.MachineBrand }},
		{"Tipe Mesin", 16, false, func(r DP3ExcelRow) any { return r.MachineType }},
		{"Daya", 10, false, func(r DP3ExcelRow) any { return r.MachinePower }},
		{"BBM", 12, false, func(r DP3ExcelRow) any { return r.MachineFuelType }},
		{"No. Seri Mesin", 22, true, func(r DP3ExcelRow) any { return r.MachineSerial }},
		{"Merek Converter", 16, false, func(r DP3ExcelRow) any { return r.ConverterBrand }},
		{"No. Seri Converter", 22, true, func(r DP3ExcelRow) any { return r.ConverterSerial }},
		{"No. Seri Selang", 22, true, func(r DP3ExcelRow) any { return r.HoseSerial }},
	}
}

func dp3ExcelDate(value string) any {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return date
}

func dp3ExcelNumber(value int) any {
	if value <= 0 {
		return ""
	}
	return value
}

var indonesianMonths = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func buildDP3Excel(info DP3ExcelContext, rows []DP3ExcelRow, generatedAt time.Time) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", dp3ExcelSheet); err != nil {
		return nil, fmt.Errorf("name DP3 excel sheet: %w", err)
	}
	columns := dp3ExcelColumns(info.ProgramType)
	lastColumn, _ := excelize.ColumnNumberToName(len(columns))
	lastRow := dp3ExcelHeaderRow + max(len(rows), 1)
	styles, err := newDP3ExcelStyles(file)
	if err != nil {
		return nil, err
	}

	place := strings.TrimSpace(info.RegencyName)
	if province := strings.TrimSpace(info.ProvinceName); province != "" {
		place += ", " + province
	}
	updated := fmt.Sprintf("Diperbarui %d %s %d pukul %s WIB · %d penerima",
		generatedAt.Day(), indonesianMonths[generatedAt.Month()-1], generatedAt.Year(), generatedAt.Format("15.04"), len(rows))
	program := strings.TrimSpace(info.ProgramName)
	if info.FiscalYear > 0 {
		program += fmt.Sprintf(" · Tahun Anggaran %d", info.FiscalYear)
	}
	title := []struct {
		text  string
		style int
	}{
		{dp3Title + " (DP3)", styles.title},
		{program, styles.subtitle},
		{"Kabupaten " + place, styles.subtitle},
		{updated, styles.note},
	}
	for index, line := range title {
		cell := fmt.Sprintf("A%d", index+1)
		if err := file.SetCellValue(dp3ExcelSheet, cell, line.text); err != nil {
			return nil, fmt.Errorf("write DP3 excel title: %w", err)
		}
		if err := file.MergeCell(dp3ExcelSheet, cell, fmt.Sprintf("%s%d", lastColumn, index+1)); err != nil {
			return nil, fmt.Errorf("merge DP3 excel title: %w", err)
		}
		_ = file.SetCellStyle(dp3ExcelSheet, cell, cell, line.style)
	}
	_ = file.SetRowHeight(dp3ExcelSheet, 1, 24)

	for index, column := range columns {
		name, _ := excelize.ColumnNumberToName(index + 1)
		if err := file.SetCellValue(dp3ExcelSheet, fmt.Sprintf("%s%d", name, dp3ExcelHeaderRow), column.title); err != nil {
			return nil, fmt.Errorf("write DP3 excel header: %w", err)
		}
		_ = file.SetColWidth(dp3ExcelSheet, name, name, column.width)
	}
	_ = file.SetCellStyle(dp3ExcelSheet, fmt.Sprintf("A%d", dp3ExcelHeaderRow), fmt.Sprintf("%s%d", lastColumn, dp3ExcelHeaderRow), styles.header)
	_ = file.SetRowHeight(dp3ExcelSheet, dp3ExcelHeaderRow, 32)

	if len(rows) == 0 {
		cell := fmt.Sprintf("A%d", dp3ExcelHeaderRow+1)
		_ = file.SetCellValue(dp3ExcelSheet, cell, "Belum ada penerima yang distribusinya selesai.")
		_ = file.MergeCell(dp3ExcelSheet, cell, fmt.Sprintf("%s%d", lastColumn, dp3ExcelHeaderRow+1))
		_ = file.SetCellStyle(dp3ExcelSheet, cell, cell, styles.empty)
	}
	for rowIndex, row := range rows {
		excelRow := dp3ExcelHeaderRow + 1 + rowIndex
		band := rowIndex % 2
		for columnIndex, column := range columns {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, excelRow)
			value := column.value(row)
			if column.text {
				value = fmt.Sprint(value)
			}
			if err := file.SetCellValue(dp3ExcelSheet, cell, value); err != nil {
				return nil, fmt.Errorf("write DP3 excel row: %w", err)
			}
			style := styles.body[band]
			switch {
			case column.text:
				style = styles.text[band]
			case columnIndex == 0 || column.title == "No. Bagi":
				style = styles.number[band]
			case column.title == "Tanggal Distribusi":
				style = styles.date[band]
			}
			_ = file.SetCellStyle(dp3ExcelSheet, cell, cell, style)
		}
	}

	if len(rows) > 0 {
		if err := file.AutoFilter(dp3ExcelSheet, fmt.Sprintf("A%d:%s%d", dp3ExcelHeaderRow, lastColumn, lastRow), nil); err != nil {
			return nil, fmt.Errorf("filter DP3 excel: %w", err)
		}
	}
	_ = file.SetPanes(dp3ExcelSheet, &excelize.Panes{
		Freeze: true, XSplit: 2, YSplit: dp3ExcelHeaderRow,
		TopLeftCell: fmt.Sprintf("C%d", dp3ExcelHeaderRow+1), ActivePane: "bottomRight",
	})
	landscape, a4, oneWide, anyTall, fit := "landscape", 9, 1, 0, true
	_ = file.SetPageLayout(dp3ExcelSheet, &excelize.PageLayoutOptions{Orientation: &landscape, Size: &a4, FitToWidth: &oneWide, FitToHeight: &anyTall})
	_ = file.SetSheetProps(dp3ExcelSheet, &excelize.SheetPropsOptions{FitToPage: &fit})
	_ = file.SetDefinedName(&excelize.DefinedName{
		Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$%d:$%d", dp3ExcelSheet, dp3ExcelHeaderRow, dp3ExcelHeaderRow), Scope: dp3ExcelSheet,
	})
	_ = file.SetDocProps(&excelize.DocProperties{Title: dp3Title, Creator: "Konkit"})

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode DP3 excel: %w", err)
	}
	return buffer.Bytes(), nil
}

type dp3ExcelStyles struct {
	title, subtitle, note, header, empty int
	// Each body style has a plain and a banded variant.
	body, text, number, date [2]int
}

func newDP3ExcelStyles(file *excelize.File) (dp3ExcelStyles, error) {
	const font = "Calibri"
	border := []excelize.Border{
		{Type: "left", Color: "C9D3CB", Style: 1}, {Type: "right", Color: "C9D3CB", Style: 1},
		{Type: "top", Color: "C9D3CB", Style: 1}, {Type: "bottom", Color: "C9D3CB", Style: 1},
	}
	var styles dp3ExcelStyles
	var err error
	add := func(style *excelize.Style) int {
		if err != nil {
			return 0
		}
		var id int
		id, err = file.NewStyle(style)
		return id
	}
	styles.title = add(&excelize.Style{Font: &excelize.Font{Family: font, Bold: true, Size: 14, Color: "173D2B"}})
	styles.subtitle = add(&excelize.Style{Font: &excelize.Font{Family: font, Size: 11, Color: "1F2B23"}})
	styles.note = add(&excelize.Style{Font: &excelize.Font{Family: font, Italic: true, Size: 10, Color: "5D6B61"}})
	styles.header = add(&excelize.Style{
		Font:      &excelize.Font{Family: font, Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"173D2B"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    border,
	})
	styles.empty = add(&excelize.Style{Font: &excelize.Font{Family: font, Italic: true, Color: "5D6B61"}, Alignment: &excelize.Alignment{Horizontal: "center"}})
	textFormat, dateFormat := "@", "dd-mm-yyyy"
	for band, fill := range []string{"FFFFFF", "F1F5EF"} {
		base := func(alignment string, format *string) *excelize.Style {
			return &excelize.Style{
				Font:         &excelize.Font{Family: font, Size: 11, Color: "1F2B23"},
				Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}},
				Alignment:    &excelize.Alignment{Horizontal: alignment, Vertical: "top", WrapText: true},
				Border:       border,
				CustomNumFmt: format,
			}
		}
		styles.body[band] = add(base("left", nil))
		styles.text[band] = add(base("left", &textFormat))
		styles.number[band] = add(base("center", nil))
		styles.date[band] = add(base("center", &dateFormat))
	}
	if err != nil {
		return dp3ExcelStyles{}, fmt.Errorf("style DP3 excel: %w", err)
	}
	return styles, nil
}
