package reports

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/bast"

	"github.com/xuri/excelize/v2"
)

type fakeRepo struct {
	info     ScheduleInfo
	details  map[string]SlotDetail
	export   *DriveExport
	saves    int
	checked  int
	failures []string
	exports  []string
	saveErr  error
}

func (f *fakeRepo) ListDriveScheduleIDs(context.Context) ([]string, error) {
	return []string{f.info.ScheduleID}, nil
}
func (f *fakeRepo) GetScheduleInfo(_ context.Context, id string, scope auth.RegencyScope) (ScheduleInfo, error) {
	if id != f.info.ScheduleID || (!scope.Unrestricted && !slices.Contains(scope.RegencyIDs, "regency-1")) {
		return ScheduleInfo{}, ErrScheduleNotFound
	}
	return f.info, nil
}
func (f *fakeRepo) ListSlotDetails(context.Context, string) (map[string]SlotDetail, error) {
	return f.details, nil
}
func (f *fakeRepo) GetDriveExport(context.Context, string, auth.RegencyScope) (DriveExport, error) {
	if f.export == nil {
		return DriveExport{}, ErrScheduleNotFound
	}
	return *f.export, nil
}
func (f *fakeRepo) SaveDriveExport(_ context.Context, export DriveExport) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	f.saves++
	old := ""
	if f.export != nil {
		old = f.export.StorageKey
	}
	now := time.Now()
	export.SyncedAt, export.LastAttemptAt = &now, now
	f.export = &export
	return old, nil
}
func (f *fakeRepo) MarkDriveChecked(context.Context, string) error {
	f.checked++
	return nil
}
func (f *fakeRepo) RecordDriveFailure(_ context.Context, _ string, message string) error {
	f.failures = append(f.failures, message)
	return nil
}
func (f *fakeRepo) CountAllocations(context.Context, string) (int, error) { return 200, nil }
func (f *fakeRepo) RecordExport(_ context.Context, _ auth.Principal, _ string, format string, _ auth.ClientMeta) error {
	f.exports = append(f.exports, format)
	return nil
}

type fakeRecipients struct{ list []bast.DP3Recipient }

func (f *fakeRecipients) ListDP3Recipients(context.Context, string, string, auth.RegencyScope) ([]bast.DP3Recipient, error) {
	return f.list, nil
}

// memoryStorage keeps uploaded files in memory under numbered keys.
type memoryStorage struct {
	files   map[string][]byte
	folders [][]string
	deleted []string
	next    int
}

func (s *memoryStorage) Put(_ context.Context, _ string, folderPath []string, source io.Reader) (string, int64, string, error) {
	data, _ := io.ReadAll(source)
	s.next++
	key := fmt.Sprintf("file-%d", s.next)
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	s.files[key] = data
	s.folders = append(s.folders, folderPath)
	return key, int64(len(data)), "", nil
}
func (s *memoryStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.files[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *memoryStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.files, key)
	return nil
}
func (s *memoryStorage) EnsureFolders(context.Context, [][]string) error { return nil }

func newFixture() (*fakeRepo, *fakeRecipients) {
	number := 7
	repo := &fakeRepo{
		info: ScheduleInfo{ScheduleID: "schedule-1", ProgramName: "Konkit 2026", ProgramType: "farmer", FiscalYear: 2026,
			RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 1"},
		details: map[string]SlotDetail{"slot-1": {
			DistributionDate: "2026-11-05", CardNumber: "0012345678901234", PhoneNumber: "081234567890",
			MachineSerial: "000123", ConverterBrand: "ERGAS", ConverterSerial: "C-001", HoseSerial: "H-001",
		}},
	}
	recipients := &fakeRecipients{list: []bast.DP3Recipient{{
		RowNumber: 1, SlotID: "slot-1", FullName: "Siti Aminah", NIK: "0306014101900001", Address: "Jl. Poros", Village: "Tempe",
		District: "Sabbangparu", DistributionNumber: &number, MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "6.5 HP",
		MachineFuelType: "Bensin", HandoverLocation: "Balai Desa Tempe",
	}}}
	return repo, recipients
}

var fixedNow = func() time.Time { return time.Date(2026, 11, 6, 10, 20, 0, 0, time.UTC) }

func TestDriveSyncUploadsToRegencyFolderOnlyWhenDataChanges(t *testing.T) {
	ctx := context.Background()
	repo, recipients := newFixture()
	storage := &memoryStorage{}
	service := NewService(repo, recipients, storage, time.UTC)

	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 1 {
		t.Fatalf("first sync: uploaded=%d err=%v", uploaded, err)
	}
	if repo.export.Filename != "LAPORAN DP3 - WAJO.xlsx" || repo.export.RecipientCount != 1 {
		t.Fatalf("unexpected export: %+v", repo.export)
	}
	if folder := storage.folders[0]; len(folder) != 3 || folder[2] != "WAJO" {
		t.Fatalf("workbook not stored in the regency folder: %v", folder)
	}

	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 0 {
		t.Fatalf("unchanged sync: uploaded=%d err=%v", uploaded, err)
	}
	if repo.saves != 1 || repo.checked != 1 || len(storage.files) != 1 {
		t.Fatalf("unchanged data was uploaded again: saves=%d checked=%d files=%d", repo.saves, repo.checked, len(storage.files))
	}

	recipients.list[0].FullName = "Siti Aminah Rahman"
	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 1 {
		t.Fatalf("changed sync: uploaded=%d err=%v", uploaded, err)
	}
	if !slices.Equal(storage.deleted, []string{"file-1"}) || repo.export.StorageKey != "file-2" {
		t.Fatalf("old workbook not replaced: deleted=%v key=%q", storage.deleted, repo.export.StorageKey)
	}
}

func TestDriveSyncReplacesWorkbookFromAnOlderLayout(t *testing.T) {
	repo, recipients := newFixture()
	repo.export = &DriveExport{ScheduleID: "schedule-1", StorageKey: "old-ba-folder-file", ContentChecksum: "checksum-of-v1"}
	storage := &memoryStorage{}
	if uploaded, err := NewService(repo, recipients, storage, time.UTC).SyncAll(context.Background()); err != nil || uploaded != 1 {
		t.Fatalf("uploaded=%d err=%v", uploaded, err)
	}
	if !slices.Equal(storage.deleted, []string{"old-ba-folder-file"}) {
		t.Fatalf("workbook in the old folder not removed: %v", storage.deleted)
	}
}

func TestDriveSyncSkipsScheduleWithoutRecipientsOrFile(t *testing.T) {
	repo, recipients := newFixture()
	recipients.list = nil
	storage := &memoryStorage{}
	if uploaded, err := NewService(repo, recipients, storage, time.UTC).SyncAll(context.Background()); err != nil || uploaded != 0 {
		t.Fatalf("uploaded=%d err=%v", uploaded, err)
	}
	if len(storage.files) != 0 || repo.export != nil {
		t.Fatal("an empty workbook was uploaded")
	}
}

func TestDriveSyncRecordsFailureAndDropsUpload(t *testing.T) {
	repo, recipients := newFixture()
	repo.saveErr = errors.New("db down")
	storage := &memoryStorage{}
	if _, err := NewService(repo, recipients, storage, time.UTC).SyncAll(context.Background()); err != nil {
		t.Fatalf("SyncAll must keep going: %v", err)
	}
	if len(repo.failures) != 1 || len(storage.files) != 0 {
		t.Fatalf("failure not recorded or upload kept: failures=%v files=%d", repo.failures, len(storage.files))
	}
}

func TestReportAndDriveStatusRespectScope(t *testing.T) {
	ctx := context.Background()
	repo, recipients := newFixture()
	service := NewService(repo, recipients, &memoryStorage{}, time.UTC)
	outside := auth.RegencyScope{RegencyIDs: []string{"regency-2"}}
	if _, err := service.Report(ctx, "schedule-1", outside); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("report outside scope: %v", err)
	}
	if _, err := service.DriveStatus(ctx, "schedule-1", outside); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("drive status outside scope: %v", err)
	}
	if _, err := service.Report(ctx, " ", unrestricted); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("empty schedule: %v", err)
	}
	inside := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	report, err := service.Report(ctx, "schedule-1", inside)
	if err != nil || len(report.Rows) != 1 || report.Rows[0].CardNumber != "0012345678901234" || report.RegencyName != "Wajo" {
		t.Fatalf("report: %+v err=%v", report, err)
	}
	status, err := service.SyncDrive(ctx, "schedule-1", inside)
	if err != nil || !status.Available || status.RecipientCount != 1 || status.SyncedAt == nil {
		t.Fatalf("status after sync: %+v err=%v", status, err)
	}
}

func TestExportExcelHoldsRecipientDataWithoutStatus(t *testing.T) {
	repo, recipients := newFixture()
	service := NewService(repo, recipients, &memoryStorage{}, time.UTC)
	service.now = fixedNow
	file, err := service.ExportExcel(context.Background(), auth.Principal{}, "schedule-1", auth.ClientMeta{}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if file.Filename != "LAPORAN DP3 - WAJO.xlsx" || !slices.Equal(repo.exports, []string{"xlsx"}) {
		t.Fatalf("filename=%q audit=%v", file.Filename, repo.exports)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	sheetRows, err := workbook.GetRows(excelSheet)
	if err != nil {
		t.Fatal(err)
	}
	if sheetRows[0][0] != "LAPORAN DP3 · DAFTAR PENERIMA PAKET PERDANA" || sheetRows[2][0] != "Kabupaten Wajo, Sulawesi Selatan" {
		t.Fatalf("unexpected title block: %q", sheetRows[:4])
	}
	if sheetRows[3][0] != "Diperbarui 6 November 2026 pukul 10.20 WIB · 1 penerima" {
		t.Fatalf("unexpected update line: %q", sheetRows[3][0])
	}
	header := sheetRows[excelHeaderRow-1]
	for _, title := range header {
		if strings.Contains(title, "Status") || strings.Contains(title, "Dokumentasi") {
			t.Fatalf("status column in laporan DP3: %q", header)
		}
	}
	if header[3] != "No. Kartu Tani" || len(header) != 19 {
		t.Fatalf("unexpected header: %q", header)
	}
	row := sheetRows[excelHeaderRow]
	want := []string{"1", "Siti Aminah", "0306014101900001", "0012345678901234", "081234567890", "Jl. Poros", "Tempe", "Sabbangparu",
		"05-11-2026", "Balai Desa Tempe", "7", "SHARK", "SPWP", "6.5 HP", "Bensin", "000123", "ERGAS", "C-001", "H-001"}
	if !slices.Equal(row, want) {
		t.Fatalf("row mismatch:\n got %q\nwant %q", row, want)
	}
}

func TestExportPDFIsAPDFAndAudited(t *testing.T) {
	repo, recipients := newFixture()
	// Enough recipients to span pages, so the repeated header path runs.
	for index := 2; index <= 60; index++ {
		recipients.list = append(recipients.list, bast.DP3Recipient{RowNumber: index, SlotID: fmt.Sprintf("slot-%d", index), FullName: "Penerima Dengan Nama Yang Cukup Panjang Sekali"})
	}
	service := NewService(repo, recipients, &memoryStorage{}, time.UTC)
	service.now = fixedNow
	file, err := service.ExportPDF(context.Background(), auth.Principal{}, "schedule-1", auth.ClientMeta{}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if file.Filename != "LAPORAN DP3 - WAJO.pdf" || !bytes.HasPrefix(file.Data, []byte("%PDF")) || !slices.Equal(repo.exports, []string{"pdf"}) {
		t.Fatalf("filename=%q prefix=%q audit=%v", file.Filename, file.Data[:4], repo.exports)
	}
	if pages := bytes.Count(file.Data, []byte("/Type /Page\n")); pages < 2 {
		t.Fatalf("expected several pages, got %d", pages)
	}
}

func TestCardTitleNamesKusukaForFishermen(t *testing.T) {
	if title := excelColumns("fisherman")[3].title; title != "No. KUSUKA" {
		t.Fatalf("card title = %q", title)
	}
}

func TestSummaryKeepsTheScheduleTotalForTheMobileApp(t *testing.T) {
	repo, recipients := newFixture()
	service := NewService(repo, recipients, &memoryStorage{}, time.UTC)
	summary, err := service.Summary(context.Background(), "schedule-1", auth.RegencyScope{RegencyIDs: []string{"regency-1"}})
	if err != nil || summary.TotalAllocations != 200 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", auth.RegencyScope{RegencyIDs: []string{"regency-2"}}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("summary outside scope: %v", err)
	}
}
