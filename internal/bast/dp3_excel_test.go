package bast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
	"time"

	"konkit/internal/auth"

	"github.com/xuri/excelize/v2"
)

type fakeDP3ExcelRepo struct {
	info       DP3ExcelContext
	recipients []DP3Recipient
	details    map[string]DP3ExcelDetail
	export     *DP3ExcelExport
	saves      int
	checked    int
	failures   []string
	saveErr    error
}

func (f *fakeDP3ExcelRepo) ListDP3ExcelScheduleIDs(context.Context) ([]string, error) {
	return []string{f.info.ScheduleID}, nil
}
func (f *fakeDP3ExcelRepo) GetDP3ExcelContext(_ context.Context, id string, scope auth.RegencyScope) (DP3ExcelContext, error) {
	if id != f.info.ScheduleID || (!scope.Unrestricted && !slices.Contains(scope.RegencyIDs, "regency-1")) {
		return DP3ExcelContext{}, ErrNotFound
	}
	return f.info, nil
}
func (f *fakeDP3ExcelRepo) ListDP3Recipients(context.Context, string, string, auth.RegencyScope) ([]DP3Recipient, error) {
	return f.recipients, nil
}
func (f *fakeDP3ExcelRepo) ListDP3ExcelDetails(context.Context, string) (map[string]DP3ExcelDetail, error) {
	return f.details, nil
}
func (f *fakeDP3ExcelRepo) GetDP3ExcelExport(context.Context, string, auth.RegencyScope) (DP3ExcelExport, error) {
	if f.export == nil {
		return DP3ExcelExport{}, ErrNotFound
	}
	return *f.export, nil
}
func (f *fakeDP3ExcelRepo) SaveDP3ExcelExport(_ context.Context, export DP3ExcelExport) (string, error) {
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
func (f *fakeDP3ExcelRepo) MarkDP3ExcelChecked(context.Context, string) error {
	f.checked++
	return nil
}
func (f *fakeDP3ExcelRepo) RecordDP3ExcelFailure(_ context.Context, _ string, message string) error {
	f.failures = append(f.failures, message)
	return nil
}

// dp3ExcelStorage keeps uploaded files in memory under numbered keys.
type dp3ExcelStorage struct {
	files   map[string][]byte
	folders [][]string
	deleted []string
	next    int
}

func (s *dp3ExcelStorage) Put(_ context.Context, _ string, folderPath []string, source io.Reader) (string, int64, string, error) {
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
func (s *dp3ExcelStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.files[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *dp3ExcelStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.files, key)
	return nil
}
func (s *dp3ExcelStorage) EnsureFolders(context.Context, [][]string) error { return nil }

func newDP3ExcelFixture() *fakeDP3ExcelRepo {
	number := 7
	return &fakeDP3ExcelRepo{
		info: DP3ExcelContext{ScheduleID: "schedule-1", ProgramName: "Konkit 2026", ProgramType: "farmer", FiscalYear: 2026,
			RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 1"},
		recipients: []DP3Recipient{{
			RowNumber: 1, SlotID: "slot-1", FullName: "Siti Aminah", NIK: "0306014101900001", Address: "Jl. Poros", Village: "Tempe",
			District: "Sabbangparu", DistributionNumber: &number, MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "6.5 HP",
			MachineFuelType: "Bensin", HandoverLocation: "Balai Desa Tempe",
		}},
		details: map[string]DP3ExcelDetail{"slot-1": {
			DistributionDate: "2026-11-05", FarmerCardNumber: "0012345678901234", PhoneNumber: "081234567890",
			MachineSerial: "000123", ConverterBrand: "ERGAS", ConverterSerial: "C-001", HoseSerial: "H-001",
		}},
	}
}

func TestDP3ExcelSyncUploadsOnlyWhenDataChanges(t *testing.T) {
	ctx := context.Background()
	repo := newDP3ExcelFixture()
	storage := &dp3ExcelStorage{}
	service := NewDP3ExcelService(repo, storage, time.UTC)

	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 1 {
		t.Fatalf("first sync: uploaded=%d err=%v", uploaded, err)
	}
	if repo.export.Filename != "DP3 - WAJO - Data Penerima.xlsx" || repo.export.RecipientCount != 1 {
		t.Fatalf("unexpected export: %+v", repo.export)
	}
	if folder := storage.folders[0]; folder[len(folder)-1] != dp3Folder {
		t.Fatalf("workbook not stored in the DP3 folder: %v", folder)
	}

	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 0 {
		t.Fatalf("unchanged sync: uploaded=%d err=%v", uploaded, err)
	}
	if repo.saves != 1 || repo.checked != 1 || len(storage.files) != 1 {
		t.Fatalf("unchanged data was uploaded again: saves=%d checked=%d files=%d", repo.saves, repo.checked, len(storage.files))
	}

	repo.recipients[0].FullName = "Siti Aminah Rahman"
	if uploaded, err := service.SyncAll(ctx); err != nil || uploaded != 1 {
		t.Fatalf("changed sync: uploaded=%d err=%v", uploaded, err)
	}
	if !slices.Equal(storage.deleted, []string{"file-1"}) || repo.export.StorageKey != "file-2" {
		t.Fatalf("old workbook not replaced: deleted=%v key=%q", storage.deleted, repo.export.StorageKey)
	}
}

func TestDP3ExcelSyncSkipsScheduleWithoutRecipientsOrFile(t *testing.T) {
	repo := newDP3ExcelFixture()
	repo.recipients = nil
	storage := &dp3ExcelStorage{}
	if uploaded, err := NewDP3ExcelService(repo, storage, time.UTC).SyncAll(context.Background()); err != nil || uploaded != 0 {
		t.Fatalf("uploaded=%d err=%v", uploaded, err)
	}
	if len(storage.files) != 0 || repo.export != nil {
		t.Fatal("an empty workbook was uploaded")
	}
}

func TestDP3ExcelSyncRecordsFailureAndDropsUpload(t *testing.T) {
	repo := newDP3ExcelFixture()
	repo.saveErr = errors.New("db down")
	storage := &dp3ExcelStorage{}
	if _, err := NewDP3ExcelService(repo, storage, time.UTC).SyncAll(context.Background()); err != nil {
		t.Fatalf("SyncAll must keep going: %v", err)
	}
	if len(repo.failures) != 1 || len(storage.files) != 0 {
		t.Fatalf("failure not recorded or upload kept: failures=%v files=%d", repo.failures, len(storage.files))
	}
}

func TestDP3ExcelStatusAndOpenRespectScope(t *testing.T) {
	ctx := context.Background()
	repo := newDP3ExcelFixture()
	storage := &dp3ExcelStorage{}
	service := NewDP3ExcelService(repo, storage, time.UTC)
	outside := auth.RegencyScope{RegencyIDs: []string{"regency-2"}}
	if _, err := service.Status(ctx, "schedule-1", outside); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status outside scope: %v", err)
	}
	status, err := service.Sync(ctx, "schedule-1", auth.RegencyScope{RegencyIDs: []string{"regency-1"}})
	if err != nil || !status.Available || status.RecipientCount != 1 || status.SyncedAt == nil {
		t.Fatalf("status after sync: %+v err=%v", status, err)
	}
	content, err := service.Open(ctx, "schedule-1", unrestrictedScope)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Reader.Close()
	if data, _ := io.ReadAll(content.Reader); !bytes.Equal(data, storage.files["file-1"]) {
		t.Fatal("download is not the stored workbook")
	}
}

func TestBuildDP3ExcelHoldsRecipientDataWithoutStatus(t *testing.T) {
	repo := newDP3ExcelFixture()
	service := NewDP3ExcelService(repo, &dp3ExcelStorage{}, time.UTC)
	rows, err := service.rows(context.Background(), "schedule-1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := buildDP3Excel(repo.info, rows, time.Date(2026, 11, 6, 10, 20, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	sheetRows, err := file.GetRows(dp3ExcelSheet)
	if err != nil {
		t.Fatal(err)
	}
	if sheetRows[0][0] != "DAFTAR PENERIMA PAKET PERDANA (DP3)" || sheetRows[2][0] != "Kabupaten Wajo, Sulawesi Selatan" {
		t.Fatalf("unexpected title block: %q", sheetRows[:4])
	}
	if sheetRows[3][0] != "Diperbarui 6 November 2026 pukul 10.20 WIB · 1 penerima" {
		t.Fatalf("unexpected update line: %q", sheetRows[3][0])
	}
	header := sheetRows[dp3ExcelHeaderRow-1]
	for _, title := range header {
		if title == "Status" || title == "Sumber" {
			t.Fatalf("status column in DP3 excel: %q", header)
		}
	}
	if header[3] != "No. Kartu Tani" || len(header) != 19 {
		t.Fatalf("unexpected header: %q", header)
	}
	row := sheetRows[dp3ExcelHeaderRow]
	want := []string{"1", "Siti Aminah", "0306014101900001", "0012345678901234", "081234567890", "Jl. Poros", "Tempe", "Sabbangparu",
		"05-11-2026", "Balai Desa Tempe", "7", "SHARK", "SPWP", "6.5 HP", "Bensin", "000123", "ERGAS", "C-001", "H-001"}
	if !slices.Equal(row, want) {
		t.Fatalf("row mismatch:\n got %q\nwant %q", row, want)
	}
}

func TestDP3ExcelColumnsNameKusukaForFishermen(t *testing.T) {
	if title := dp3ExcelColumns("fisherman")[3].title; title != "No. KUSUKA" {
		t.Fatalf("card title = %q", title)
	}
}
