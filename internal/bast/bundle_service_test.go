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
	if result.ID != "same" || storage.putCalls != 0 || repository.activateCalls != 0 {
		t.Fatalf("result=%+v puts=%d activates=%d", result, storage.putCalls, repository.activateCalls)
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
}

func newBundleRepositoryStub(t *testing.T) *bundleRepositoryStub { return &bundleRepositoryStub{t: t} }

func (r *bundleRepositoryStub) GetSourceContext(context.Context, string, string, auth.RegencyScope) (SourceContext, error) {
	return SourceContext{ProgramID: "program", RegencyID: "regency", ProgramType: "farmer", RegencyCode: "WJO", RegencyName: "Wajo", ZoneName: "Zona 1", Padding: 4, SlotQuota: 2, DocumentSeries: "KSM-KKT", HasActiveLogo: true}, nil
}
func (r *bundleRepositoryStub) ListCompletedSlots(context.Context, string, string, auth.RegencyScope) ([]CompletedSlot, error) {
	return []CompletedSlot{{ID: "slot-2", SlotNumber: 2, DistributedAt: time.Date(2024, 12, 10, 2, 0, 0, 0, time.UTC)}, {ID: "slot-1", SlotNumber: 1, DistributedAt: time.Date(2024, 12, 10, 1, 0, 0, 0, time.UTC)}}, nil
}
func (r *bundleRepositoryStub) GetLockedTotal(context.Context, string, string) (LockResult, error) {
	return LockResult{ProgramID: "program", RegencyID: "regency", FinalTotal: 2}, nil
}
func (r *bundleRepositoryStub) LockRegencyTotal(context.Context, auth.Principal, SourceContext, auth.ClientMeta) (LockResult, error) {
	panic("unexpected")
}
func (r *bundleRepositoryStub) ListActiveBundles(context.Context, string, string, auth.RegencyScope) ([]DailyBundle, error) {
	return nil, nil
}
func (r *bundleRepositoryStub) LoadSourceData(_ context.Context, recipient RecipientDocument, _ SourceContext, _ auth.RegencyScope) (SourceData, error) {
	return renderSource(recipient), nil
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
	return BundleActivationResult{Bundle: DailyBundle{ID: "new", ProgramID: input.ProgramID, RegencyID: input.RegencyID, LocalDate: input.LocalDate, Filename: input.Filename, RecipientCount: len(input.Items), PageCount: input.PageCount, Checksum: input.Checksum, StorageKey: input.StorageKey, Version: 2, Status: "active"}, OldStorageKey: r.active.StorageKey}, nil
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
