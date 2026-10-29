package bast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
)

func TestFinalizeBundleUploadsToExactPathAndDeletesOldAfterSwap(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.active = DailyBundle{ID: "old", Checksum: strings.Repeat("a", 64), StorageKey: "old-key", Status: "active"}
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)

	result, err := service.FinalizeBundle(context.Background(), auth.Principal{UserID: "actor"}, BundleRequest{ProgramID: " program ", RegencyID: " regency ", LocalDate: " 2024-12-10 "}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "PETANI/ZONA 1/WAJO/BERITA ACARA (BA)/2. BA PERORANGAN"
	if strings.Join(storage.putPath, "/") != wantPath {
		t.Fatalf("path=%q want=%q", strings.Join(storage.putPath, "/"), wantPath)
	}
	if storage.putFilename != "SELASA, 10 DESEMBER 2024.pdf" {
		t.Fatalf("drive filename=%q", storage.putFilename)
	}
	if result.Filename != "SELASA, 10 DESEMBER 2024.pdf" || result.RecipientCount != 2 {
		t.Fatalf("bundle=%+v", result)
	}
	if strings.Join(storage.operations, ",") != "open:logo-key,put,delete:old-key" {
		t.Fatalf("operations=%v", storage.operations)
	}
	if repository.activated.ProgramID != "program" || repository.activated.RegencyID != "regency" || repository.activated.LocalDate != "2024-12-10" || repository.activated.Filename != result.Filename || repository.activated.Items[0].SlotNumber != 1 || repository.activated.Items[1].SlotNumber != 2 {
		t.Fatalf("activation=%+v", repository.activated)
	}
}

func TestPreviewBundleWorksBeforeTotalLockAndWithIncompleteData(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.unlocked = true
	repository.blankSource = true
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)
	request := BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}

	preview, err := service.PreviewBundle(context.Background(), request, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.RecipientCount != 2 || len(preview.PDF) == 0 {
		t.Fatalf("preview=%+v", preview)
	}
	if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, request, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err == nil || errors.Is(err, ErrFinalTotalMissing) {
		t.Fatalf("finalize with incomplete data err=%v, want a data error rather than a lock error", err)
	}
}

// BA Perorangan is synced without locking the regency total: the number uses the slot quota.
func TestFinalizeBundleWithoutLockedTotalUsesTheSlotQuota(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.unlocked = true
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)

	bundle, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.RecipientCount != 2 || len(repository.activated.Items) != 2 {
		t.Fatalf("bundle=%+v activation=%+v", bundle, repository.activated)
	}
}

func TestFinalizeBundleDeletesTheFilesOfSupersededStaleVersions(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.staleKeys = []string{"stale-key"}
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	storage.files["stale-key"] = []byte("old pdf")
	service := NewBundleService(repository, storage, time.UTC)

	if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storage.files["stale-key"]; ok {
		t.Fatal("stale bundle file was not deleted")
	}
}

func TestRetireDateDeletesTheStaleFiles(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.staleKeys = []string{"stale-key"}
	storage := newBundleStorageStub(t)
	storage.files["stale-key"] = []byte("old pdf")
	service := NewBundleService(repository, storage, time.UTC)

	if err := service.RetireDate(context.Background(), "program", "regency", "2024-12-10", auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storage.files["stale-key"]; ok || repository.retiredDate != "2024-12-10" {
		t.Fatalf("retired=%q files=%v", repository.retiredDate, storage.files)
	}
}

func TestPreviewBundleOfOneSlotKeepsItsNumber(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)
	scope := auth.RegencyScope{Unrestricted: true}
	request := BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}

	all, err := service.PreviewBundle(context.Background(), request, scope)
	if err != nil {
		t.Fatal(err)
	}
	request.SlotID = " slot-2 "
	one, err := service.PreviewBundle(context.Background(), request, scope)
	if err != nil {
		t.Fatal(err)
	}
	if all.RecipientCount != 2 || one.RecipientCount != 1 || len(one.PDF) == 0 {
		t.Fatalf("all=%d one=%d", all.RecipientCount, one.RecipientCount)
	}
	// The slot's own completion day is used, whatever date the app sends.
	request.LocalDate = "2024-12-01"
	request.SlotID = "slot-1"
	if other, err := service.PreviewBundle(context.Background(), request, scope); err != nil || other.RecipientCount != 1 {
		t.Fatalf("slot preview with another date: %+v err=%v", other, err)
	}
	request.SlotID = "unknown"
	if _, err := service.PreviewBundle(context.Background(), request, scope); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown slot err=%v", err)
	}
}

// An identical PDF is not uploaded again, but the sync still records the content it checked so
// the date reads as synced.
func TestFinalizeBundleChecksumNoOpDoesNotUpload(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)

	preview, err := service.PreviewBundle(context.Background(), BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	repository.active = DailyBundle{ID: "same", Checksum: preview.Checksum, StorageKey: "same-key", Filename: preview.Filename, RecipientCount: 2, PageCount: preview.PageCount, Status: "active"}
	storage.operations = nil

	result, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "same" || storage.putCalls != 0 || repository.activateCalls != 1 || repository.activated.StorageKey != "" || repository.activated.SourceFingerprint == "" {
		t.Fatalf("result=%+v puts=%d activates=%d activation=%+v", result, storage.putCalls, repository.activateCalls, repository.activated)
	}
}

func TestFinalizeBundleUploadOrSwapFailurePreservesOldActive(t *testing.T) {
	t.Run("upload failure", func(t *testing.T) {
		repository := newBundleRepositoryStub(t)
		repository.active = DailyBundle{ID: "old", StorageKey: "old-key", Checksum: strings.Repeat("a", 64)}
		storage := newBundleStorageStub(t)
		storage.files["logo-key"] = testPNG(t)
		storage.putErr = errors.New("drive unavailable")
		service := NewBundleService(repository, storage, time.UTC)
		if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err == nil {
			t.Fatal("expected upload error")
		}
		if repository.activateCalls != 0 || contains(storage.operations, "delete:old-key") {
			t.Fatalf("old bundle was changed: ops=%v", storage.operations)
		}
	})

	t.Run("swap failure", func(t *testing.T) {
		repository := newBundleRepositoryStub(t)
		repository.active = DailyBundle{ID: "old", StorageKey: "old-key", Checksum: strings.Repeat("a", 64)}
		repository.activateErr = errors.New("database unavailable")
		storage := newBundleStorageStub(t)
		storage.files["logo-key"] = testPNG(t)
		service := NewBundleService(repository, storage, time.UTC)
		if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err == nil {
			t.Fatal("expected swap error")
		}
		if !containsPrefix(storage.operations, "delete:") || contains(storage.operations, "delete:old-key") {
			t.Fatalf("only new orphan should be deleted: ops=%v", storage.operations)
		}
	})
}

func TestFinalizeBundleRecordsOldFileCleanupFailure(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.active = DailyBundle{ID: "old", StorageKey: "old-key", Checksum: strings.Repeat("a", 64)}
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	storage.deleteErr["old-key"] = errors.New("delete failed")
	service := NewBundleService(repository, storage, time.UTC)

	result, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.cleanupBundleID != result.ID || !strings.Contains(repository.cleanupMessage, "old-key") {
		t.Fatalf("cleanup state id=%q message=%q", repository.cleanupBundleID, repository.cleanupMessage)
	}
}

func TestOpenBundleStreamsOnlyScopedActiveBundle(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	repository.byID = DailyBundle{ID: "bundle", StorageKey: "bundle-key", Filename: "SELASA.pdf", Status: "active"}
	storage := newBundleStorageStub(t)
	storage.files["bundle-key"] = []byte("pdf")
	service := NewBundleService(repository, storage, time.UTC)
	content, err := service.OpenBundle(context.Background(), "bundle", auth.RegencyScope{RegencyIDs: []string{"regency"}})
	if err != nil {
		t.Fatal(err)
	}
	defer content.Reader.Close()
	data, _ := io.ReadAll(content.Reader)
	if string(data) != "pdf" || content.Filename != "SELASA.pdf" {
		t.Fatalf("content=%q filename=%q", data, content.Filename)
	}
}

type bundleRepositoryStub struct {
	t               *testing.T
	active          DailyBundle
	byID            DailyBundle
	activated       BundleActivation
	activateCalls   int
	activateErr     error
	cleanupBundleID string
	cleanupMessage  string
	unlocked        bool
	blankSource     bool
	staleKeys       []string
	retiredDate     string
	bundles         []DailyBundle
	address         string
}

func newBundleRepositoryStub(t *testing.T) *bundleRepositoryStub { return &bundleRepositoryStub{t: t} }

func (r *bundleRepositoryStub) GetSourceContext(context.Context, string, string, auth.RegencyScope) (SourceContext, error) {
	return SourceContext{ProgramID: "program", RegencyID: "regency", ProgramType: "farmer", RegencyCode: "WJO", RegencyName: "Wajo", ZoneName: "Zona 1", SlotQuota: 2, DocumentSeries: "KSM-KKT", HasActiveLogo: true}, nil
}
func (r *bundleRepositoryStub) ListCompletedSlots(context.Context, string, string, auth.RegencyScope) ([]CompletedSlot, error) {
	return []CompletedSlot{{ID: "slot-2", SlotNumber: 2, DistributedAt: time.Date(2024, 12, 10, 2, 0, 0, 0, time.UTC)}, {ID: "slot-1", SlotNumber: 1, DistributedAt: time.Date(2024, 12, 10, 1, 0, 0, 0, time.UTC)}}, nil
}
func (r *bundleRepositoryStub) GetLockedTotal(context.Context, string, string) (LockResult, error) {
	if r.unlocked {
		return LockResult{}, ErrNotFound
	}
	return LockResult{ProgramID: "program", RegencyID: "regency", FinalTotal: 2}, nil
}
func (r *bundleRepositoryStub) LockRegencyTotal(context.Context, auth.Principal, SourceContext, auth.ClientMeta) (LockResult, error) {
	panic("unexpected")
}
func (r *bundleRepositoryStub) ListActiveBundles(context.Context, string, string, auth.RegencyScope) ([]DailyBundle, error) {
	return r.bundles, nil
}
func (r *bundleRepositoryStub) LoadSourceData(_ context.Context, recipient RecipientDocument, _ SourceContext, _ auth.RegencyScope) (SourceData, error) {
	source := renderSource(recipient)
	if r.address != "" {
		source.Recipient.Address = r.address
	}
	if r.blankSource {
		source.Recipient = RecipientSnapshot{}
		source.Equipment = EquipmentSnapshot{}
	}
	return source, nil
}
func (r *bundleRepositoryStub) SaveFinalDocument(_ context.Context, _ auth.Principal, recipient RecipientDocument, source SourceData, snapshot Snapshot, _ auth.ClientMeta) (IndividualDocument, error) {
	return IndividualDocument{ID: "doc-" + recipient.DistributionSlotID, DistributionSlotID: recipient.DistributionSlotID, ProgramID: source.ProgramID, RegencyID: source.RegencyID, DocumentNumber: recipient.DocumentNumber, LocalDate: recipient.LocalDate, SlotNumber: recipient.SlotNumber, FinalTotal: recipient.FinalTotal, Snapshot: snapshot, Status: "final"}, nil
}
func (r *bundleRepositoryStub) GetActiveBundle(context.Context, string, string, string, auth.RegencyScope) (DailyBundle, error) {
	if r.active.ID == "" {
		return DailyBundle{}, ErrNotFound
	}
	return r.active, nil
}
func (r *bundleRepositoryStub) ActivateBundle(_ context.Context, _ auth.Principal, input BundleActivation, _ auth.ClientMeta) (BundleActivationResult, error) {
	r.activateCalls++
	r.activated = input
	if r.activateErr != nil {
		return BundleActivationResult{}, r.activateErr
	}
	if r.active.ID != "" && r.active.Checksum == input.Checksum {
		unchanged := r.active
		unchanged.SourceFingerprint = input.SourceFingerprint
		return BundleActivationResult{Bundle: unchanged, Unchanged: true, StaleStorageKeys: r.staleKeys}, nil
	}
	return BundleActivationResult{Bundle: DailyBundle{ID: "new", ProgramID: input.ProgramID, RegencyID: input.RegencyID, LocalDate: input.LocalDate, Filename: input.Filename, RecipientCount: len(input.Items), PageCount: input.PageCount, Checksum: input.Checksum, StorageKey: input.StorageKey, Version: 2, Status: "active"}, OldStorageKey: r.active.StorageKey, StaleStorageKeys: r.staleKeys}, nil
}
func (r *bundleRepositoryStub) RetireDateBundles(_ context.Context, _, _, localDate string, _ auth.RegencyScope) ([]string, error) {
	r.retiredDate = localDate
	return r.staleKeys, nil
}
func (r *bundleRepositoryStub) RecordCleanupFailure(_ context.Context, bundleID, message string) error {
	r.cleanupBundleID, r.cleanupMessage = bundleID, message
	return nil
}
func (r *bundleRepositoryStub) GetActiveBundleByID(context.Context, string, auth.RegencyScope) (DailyBundle, error) {
	if r.byID.ID == "" {
		return DailyBundle{}, ErrNotFound
	}
	return r.byID, nil
}

func renderSource(recipient RecipientDocument) SourceData {
	return SourceData{ProgramID: "program", RegencyID: "regency", ProgramType: "farmer", DocumentNumber: recipient.DocumentNumber, LocalDate: recipient.LocalDate, PackageTemplateVersionID: "package", Render: RenderIdentity{FiscalYear: 2024, Logos: []LogoSnapshot{{AssetID: "logo", StorageKey: "logo-key", MimeType: "image/png", SortOrder: 1}}}, Recipient: RecipientSnapshot{FullName: "Penerima", NIK: "123", SectorIdentifier: "KARTU", Address: "Alamat", Regency: "Wajo"}, Equipment: EquipmentSnapshot{MachineBrand: "SHARK", MachineType: "SPWP", HoseBrand: "TRILLIUNHOSE", HoseSpec: "6 M / 10 M", ConverterBrand: "ERGAS"}, Components: []ComponentSnapshot{{Label: "Tabung LPG", Quantity: 1, Unit: "Tabung"}}}
}

type bundleStorageStub struct {
	t           *testing.T
	files       map[string][]byte
	operations  []string
	putPath     []string
	putFilename string
	putCalls    int
	putErr      error
	deleteErr   map[string]error
}

func newBundleStorageStub(t *testing.T) *bundleStorageStub {
	return &bundleStorageStub{t: t, files: map[string][]byte{}, deleteErr: map[string]error{}}
}
func (s *bundleStorageStub) Put(_ context.Context, key string, path []string, source io.Reader) (string, int64, string, error) {
	s.operations = append(s.operations, "put")
	s.putCalls++
	s.putPath = append([]string(nil), path...)
	if s.putErr != nil {
		return "", 0, "", s.putErr
	}
	data, _ := io.ReadAll(source)
	s.files[key] = data
	return key, int64(len(data)), "", nil
}
func (s *bundleStorageStub) PutNamed(ctx context.Context, key, filename string, path []string, source io.Reader) (string, int64, string, error) {
	s.putFilename = filename
	return s.Put(ctx, key, path, source)
}
func (s *bundleStorageStub) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.operations = append(s.operations, "open:"+key)
	data, ok := s.files[key]
	if !ok {
		return nil, errors.New("missing file")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *bundleStorageStub) Delete(_ context.Context, key string) error {
	s.operations = append(s.operations, "delete:"+key)
	if err := s.deleteErr[key]; err != nil {
		return err
	}
	delete(s.files, key)
	return nil
}
func (s *bundleStorageStub) EnsureFolders(context.Context, [][]string) error { return nil }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func containsPrefix(values []string, wanted string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, wanted) {
			return true
		}
	}
	return false
}

// A date is synced only while its content matches what was last written to Drive; any change
// afterwards labels it out of sync, and a date whose recipients all left is offered for removal.
func TestListDatesReportsSyncStatusFromTheContent(t *testing.T) {
	repository := newBundleRepositoryStub(t)
	storage := newBundleStorageStub(t)
	storage.files["logo-key"] = testPNG(t)
	service := NewBundleService(repository, storage, time.UTC)
	scope := auth.RegencyScope{Unrestricted: true}
	status := func() map[string]DateSummary {
		t.Helper()
		dates, err := service.ListDates(context.Background(), "program", "regency", scope)
		if err != nil {
			t.Fatal(err)
		}
		byDate := map[string]DateSummary{}
		for _, date := range dates {
			byDate[date.LocalDate] = date
		}
		return byDate
	}

	if got := status()["2024-12-10"].SyncStatus; got != BundleNotCreated {
		t.Fatalf("before sync = %q", got)
	}
	if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-10"}, scope, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	repository.bundles = []DailyBundle{{ID: "new", LocalDate: "2024-12-10", Status: "active", SourceFingerprint: repository.activated.SourceFingerprint}}
	if got := status()["2024-12-10"].SyncStatus; got != BundleSynced {
		t.Fatalf("after sync = %q", got)
	}

	repository.address = "Alamat yang diperbaiki"
	if got := status()["2024-12-10"].SyncStatus; got != BundleOutOfSync {
		t.Fatalf("after a data change = %q", got)
	}

	repository.bundles = append(repository.bundles, DailyBundle{ID: "old", LocalDate: "2024-12-01", Status: "stale", StorageKey: "old-key"})
	orphan := status()["2024-12-01"]
	if orphan.RecipientCount != 0 || orphan.SyncStatus != BundleOutOfSync {
		t.Fatalf("emptied date = %+v", orphan)
	}
	if _, err := service.FinalizeBundle(context.Background(), auth.Principal{}, BundleRequest{ProgramID: "program", RegencyID: "regency", LocalDate: "2024-12-01"}, scope, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.retiredDate != "2024-12-01" {
		t.Fatalf("emptied date was not retired: %q", repository.retiredDate)
	}
}
