package reports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"konkit/internal/auth"
	"konkit/internal/bast"
	"konkit/internal/media"

	"github.com/google/uuid"
)

// Laporan DP3: daftar penerima satu jadwal kabupaten yang distribusinya sudah
// selesai, tanpa status apa pun, bernomor sama dengan BA DP3 dan Rekap
// Harian. Halaman Laporan DP3 menampilkan dan mengekspornya (Excel, PDF).
// Server juga membangun ulang workbook-nya berkala dan menyimpannya langsung
// di folder kabupaten di Drive, hanya bila datanya berubah; file lama dihapus
// setelah file baru tersimpan.

var (
	ErrScheduleRequired = errors.New("report schedule is required")
	ErrScheduleNotFound = errors.New("report schedule not found")
)

// dp3Layout ikut masuk sidik isi: ubah nilainya saat isi atau lokasi file
// Drive berubah, agar semua workbook dibangun ulang dan dipindahkan.
const dp3Layout = "laporan-dp3-v2"

// DriveSyncInterval is how often the server rebuilds changed workbooks.
const DriveSyncInterval = 10 * time.Minute

const dp3Title = "DAFTAR PENERIMA PAKET PERDANA"

// ScheduleInfo is the kabupaten and programme a report belongs to.
type ScheduleInfo struct {
	ScheduleID   string `json:"schedule_id"`
	ProgramName  string `json:"program_name"`
	ProgramType  string `json:"program_type"`
	FiscalYear   int    `json:"fiscal_year"`
	RegencyName  string `json:"regency_name"`
	ProvinceName string `json:"province_name"`
	ZoneName     string `json:"-"`
}

// SlotDetail is the recipient data of one slot that bast.DP3Recipient lacks.
type SlotDetail struct {
	DistributionDate string
	CardNumber       string
	PhoneNumber      string
	MachineSerial    string
	ConverterBrand   string
	ConverterSerial  string
	HoseSerial       string
}

// Row is one recipient of the report.
type Row struct {
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

// Report is what the Laporan DP3 page shows.
type Report struct {
	ScheduleInfo
	Rows []Row `json:"rows"`
}

// DriveExport is the stored state of one schedule's Drive workbook.
type DriveExport struct {
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

// DriveStatus is what the page shows about the Drive workbook.
type DriveStatus struct {
	Available      bool       `json:"available"`
	Filename       string     `json:"filename"`
	RecipientCount int        `json:"recipient_count"`
	SyncedAt       *time.Time `json:"synced_at"`
	LastError      string     `json:"last_error"`
	LastAttemptAt  *time.Time `json:"last_attempt_at"`
}

// Summary keeps the schedule total the mobile app reads
// (GET reports/schedule/{id}/summary).
type Summary struct {
	TotalAllocations int `json:"total_allocations"`
}

// File is a generated export.
type File struct {
	Filename string
	Data     []byte
}

type repository interface {
	ListDriveScheduleIDs(context.Context) ([]string, error)
	GetScheduleInfo(context.Context, string, auth.RegencyScope) (ScheduleInfo, error)
	ListSlotDetails(context.Context, string) (map[string]SlotDetail, error)
	GetDriveExport(context.Context, string, auth.RegencyScope) (DriveExport, error)
	SaveDriveExport(context.Context, DriveExport) (string, error)
	MarkDriveChecked(context.Context, string) error
	RecordDriveFailure(context.Context, string, string) error
	RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, meta auth.ClientMeta) error
	CountAllocations(context.Context, string) (int, error)
}

// recipientSource is the BA DP3 recipient list, so the report keeps the same
// numbering and machine data as the BA.
type recipientSource interface {
	ListDP3Recipients(context.Context, string, string, auth.RegencyScope) ([]bast.DP3Recipient, error)
}

type Service struct {
	repository repository
	recipients recipientSource
	storage    media.Storage
	location   *time.Location
	now        func() time.Time
	// mu keeps the worker and a manual sync from uploading the same schedule twice.
	mu sync.Mutex
}

func NewService(repository repository, recipients recipientSource, storage media.Storage, location *time.Location) *Service {
	if location == nil {
		location = time.UTC
	}
	return &Service{repository: repository, recipients: recipients, storage: storage, location: location, now: time.Now}
}

var unrestricted = auth.RegencyScope{Unrestricted: true}

func (s *Service) Report(ctx context.Context, scheduleID string, scope auth.RegencyScope) (Report, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Report{}, ErrScheduleRequired
	}
	info, err := s.repository.GetScheduleInfo(ctx, scheduleID, scope)
	if err != nil {
		return Report{}, err
	}
	rows, err := s.rows(ctx, scheduleID)
	if err != nil {
		return Report{}, err
	}
	return Report{ScheduleInfo: info, Rows: rows}, nil
}

// Summary returns the schedule's recipient (allocation) total for the
// mobile app.
func (s *Service) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Summary{}, ErrScheduleRequired
	}
	if _, err := s.repository.GetScheduleInfo(ctx, scheduleID, scope); err != nil {
		return Summary{}, err
	}
	total, err := s.repository.CountAllocations(ctx, scheduleID)
	if err != nil {
		return Summary{}, err
	}
	return Summary{TotalAllocations: total}, nil
}

func (s *Service) ExportExcel(ctx context.Context, actor auth.Principal, scheduleID string, meta auth.ClientMeta, scope auth.RegencyScope) (File, error) {
	report, err := s.Report(ctx, scheduleID, scope)
	if err != nil {
		return File{}, err
	}
	data, err := buildExcel(report, s.now().In(s.location))
	if err != nil {
		return File{}, err
	}
	if err := s.repository.RecordExport(ctx, actor, report.ScheduleID, "xlsx", meta); err != nil {
		return File{}, err
	}
	return File{Filename: reportFilename(report.RegencyName, "xlsx"), Data: data}, nil
}

func (s *Service) ExportPDF(ctx context.Context, actor auth.Principal, scheduleID string, meta auth.ClientMeta, scope auth.RegencyScope) (File, error) {
	report, err := s.Report(ctx, scheduleID, scope)
	if err != nil {
		return File{}, err
	}
	data, err := buildPDF(report, s.now().In(s.location))
	if err != nil {
		return File{}, err
	}
	if err := s.repository.RecordExport(ctx, actor, report.ScheduleID, "pdf", meta); err != nil {
		return File{}, err
	}
	return File{Filename: reportFilename(report.RegencyName, "pdf"), Data: data}, nil
}

func (s *Service) DriveStatus(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DriveStatus, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetScheduleInfo(ctx, scheduleID, scope); err != nil {
		return DriveStatus{}, err
	}
	export, err := s.repository.GetDriveExport(ctx, scheduleID, scope)
	if errors.Is(err, ErrScheduleNotFound) {
		return DriveStatus{}, nil
	}
	if err != nil {
		return DriveStatus{}, err
	}
	attempt := export.LastAttemptAt
	return DriveStatus{
		Available: export.StorageKey != "", Filename: export.Filename, RecipientCount: export.RecipientCount,
		SyncedAt: export.SyncedAt, LastError: export.LastError, LastAttemptAt: &attempt,
	}, nil
}

// SyncDrive rebuilds one schedule's Drive workbook now, for a user in scope.
func (s *Service) SyncDrive(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DriveStatus, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetScheduleInfo(ctx, scheduleID, scope); err != nil {
		return DriveStatus{}, err
	}
	if _, err := s.syncSchedule(ctx, scheduleID); err != nil {
		return DriveStatus{}, err
	}
	return s.DriveStatus(ctx, scheduleID, scope)
}

// SyncAll rebuilds every changed Drive workbook. A failing schedule is
// recorded and does not stop the others.
func (s *Service) SyncAll(ctx context.Context) (uploaded int, err error) {
	ids, err := s.repository.ListDriveScheduleIDs(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return uploaded, ctx.Err()
		}
		changed, syncErr := s.syncSchedule(ctx, id)
		if syncErr != nil {
			log.Printf("laporan dp3: schedule %s: %v", id, syncErr)
			continue
		}
		if changed {
			uploaded++
		}
	}
	return uploaded, nil
}

// Run syncs every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	for {
		if uploaded, err := s.SyncAll(ctx); err != nil && ctx.Err() == nil {
			log.Printf("laporan dp3: %v", err)
		} else if uploaded > 0 {
			log.Printf("laporan dp3: %d workbook(s) updated in Drive", uploaded)
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
func (s *Service) syncSchedule(ctx context.Context, scheduleID string) (changed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil && ctx.Err() == nil {
			_ = s.repository.RecordDriveFailure(context.Background(), scheduleID, err.Error())
		}
	}()
	info, err := s.repository.GetScheduleInfo(ctx, scheduleID, unrestricted)
	if err != nil {
		return false, err
	}
	rows, err := s.rows(ctx, scheduleID)
	if err != nil {
		return false, err
	}
	checksum, err := reportChecksum(info, rows)
	if err != nil {
		return false, err
	}
	existing, err := s.repository.GetDriveExport(ctx, scheduleID, unrestricted)
	if err != nil && !errors.Is(err, ErrScheduleNotFound) {
		return false, err
	}
	hasFile := err == nil && existing.StorageKey != ""
	if (hasFile && existing.ContentChecksum == checksum) || (!hasFile && len(rows) == 0) {
		if err == nil {
			return false, s.repository.MarkDriveChecked(ctx, scheduleID)
		}
		return false, nil
	}
	data, err := buildExcel(Report{ScheduleInfo: info, Rows: rows}, s.now().In(s.location))
	if err != nil {
		return false, err
	}
	folderPath, err := media.BuildRegencyFolderPath(info.ProgramType, info.ZoneName, info.RegencyName)
	if err != nil {
		return false, err
	}
	filename := reportFilename(info.RegencyName, "xlsx")
	storageKey, size, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), filename, folderPath, bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("upload laporan DP3: %w", err)
	}
	oldKey, err := s.repository.SaveDriveExport(ctx, DriveExport{
		ScheduleID: scheduleID, Filename: filename, StorageKey: storageKey, ContentChecksum: checksum,
		RecipientCount: len(rows), ByteSize: size,
	})
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return false, err
	}
	if oldKey != "" && oldKey != storageKey {
		if deleteErr := s.storage.Delete(ctx, oldKey); deleteErr != nil {
			log.Printf("laporan dp3: schedule %s: remove old workbook %s: %v", scheduleID, oldKey, deleteErr)
		}
	}
	return true, nil
}

func (s *Service) rows(ctx context.Context, scheduleID string) ([]Row, error) {
	recipients, err := s.recipients.ListDP3Recipients(ctx, scheduleID, "", unrestricted)
	if err != nil {
		return nil, err
	}
	details, err := s.repository.ListSlotDetails(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(recipients))
	for _, recipient := range recipients {
		detail := details[recipient.SlotID]
		row := Row{
			RowNumber: recipient.RowNumber, FullName: recipient.FullName, NIK: recipient.NIK, CardNumber: detail.CardNumber,
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

func reportChecksum(info ScheduleInfo, rows []Row) (string, error) {
	payload, err := json.Marshal(struct {
		Layout string       `json:"layout"`
		Info   ScheduleInfo `json:"info"`
		Zone   string       `json:"zone"`
		Rows   []Row        `json:"rows"`
	}{dp3Layout, info, info.ZoneName, rows})
	if err != nil {
		return "", fmt.Errorf("encode laporan DP3 checksum: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func reportFilename(regencyName, extension string) string {
	name := strings.ToUpper(strings.Join(strings.Fields(regencyName), " "))
	return fmt.Sprintf("LAPORAN DP3 - %s.%s", name, extension)
}

var indonesianMonths = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// reportHeading is the title block shared by the Excel and PDF exports.
func reportHeading(info ScheduleInfo, rowCount int, generatedAt time.Time) []string {
	place := strings.TrimSpace(info.RegencyName)
	if province := strings.TrimSpace(info.ProvinceName); province != "" {
		place += ", " + province
	}
	program := strings.TrimSpace(info.ProgramName)
	if info.FiscalYear > 0 {
		program += fmt.Sprintf(" · Tahun Anggaran %d", info.FiscalYear)
	}
	updated := fmt.Sprintf("Diperbarui %d %s %d pukul %s WIB · %d penerima",
		generatedAt.Day(), indonesianMonths[generatedAt.Month()-1], generatedAt.Year(), generatedAt.Format("15.04"), rowCount)
	return []string{"LAPORAN DP3 · " + dp3Title, program, "Kabupaten " + place, updated}
}

func cardTitle(programType string) string {
	if programType == "fisherman" {
		return "No. KUSUKA"
	}
	return "No. Kartu Tani"
}

// formatDate turns YYYY-MM-DD into DD-MM-YYYY.
func formatDate(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return date.Format("02-01-2006")
}
