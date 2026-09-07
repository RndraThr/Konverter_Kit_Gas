package distribution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"konkit/internal/auth"
)

type storageStub struct {
	putKey, deletedKey string
	content            []byte
	putErr, deleteErr  error
}

func (s *storageStub) Put(_ context.Context, key string, source io.Reader) (int64, string, error) {
	s.putKey = key
	s.content, _ = io.ReadAll(source)
	return int64(len(s.content)), "checksum", s.putErr
}
func (s *storageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}
func (s *storageStub) Delete(_ context.Context, key string) error {
	s.deletedKey = key
	return s.deleteErr
}

type mediaRepositoryStub struct {
	repositoryStub
	slot       MediaSlot
	media      MediaFile
	saveErr    error
	restoredID string
}

func (r *mediaRepositoryStub) GetMediaSlot(context.Context, string) (MediaSlot, error) {
	return r.slot, nil
}
func (r *mediaRepositoryStub) SaveMedia(_ context.Context, _ auth.Principal, input MediaFileInput, _ auth.ClientMeta) (MediaFile, error) {
	if r.saveErr != nil {
		return MediaFile{}, r.saveErr
	}
	r.media = MediaFile{ID: "media-1", SlotID: input.SlotID, StorageKey: input.StorageKey, MimeType: input.MimeType, ByteSize: input.ByteSize, Status: "accepted"}
	return r.media, nil
}
func (r *mediaRepositoryStub) GetMedia(context.Context, string) (MediaFile, error) {
	return r.media, nil
}
func (r *mediaRepositoryStub) DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta) (MediaFile, error) {
	return r.media, nil
}
func (r *mediaRepositoryStub) RestoreMedia(_ context.Context, id string) error {
	r.restoredID = id
	return nil
}

type repositoryStub struct {
	searchRecords []SearchRecord
	searchQuery   string
	searchLimit   int
	workspace     RecipientWorkspace
	saved         DraftInput
}

func (r *repositoryStub) Search(_ context.Context, _ string, query string, limit int) ([]SearchRecord, error) {
	r.searchQuery, r.searchLimit = query, limit
	return r.searchRecords, nil
}
func (r *repositoryStub) GetWorkspace(context.Context, string) (RecipientWorkspace, error) {
	return r.workspace, nil
}
func (r *repositoryStub) SaveDraft(_ context.Context, _ auth.Principal, _ string, input DraftInput, _ auth.ClientMeta) (RecipientWorkspace, error) {
	r.saved = input
	return r.workspace, nil
}

func TestSearchValidatesContextAndNameLength(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Search(context.Background(), "", "Siti", 20); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
	if _, err := service.Search(context.Background(), "schedule-1", "S", 20); !errors.Is(err, ErrQueryTooShort) {
		t.Fatalf("short name err=%v", err)
	}
	if _, err := service.Search(context.Background(), "schedule-1", "", 20); !errors.Is(err, ErrQueryRequired) {
		t.Fatalf("empty query err=%v", err)
	}
}

func TestSearchAllowsSingleDistributionNumberMasksNIKAndCapsResults(t *testing.T) {
	repository := &repositoryStub{searchRecords: []SearchRecord{{
		AllocationID: "allocation-1", DistributionNumber: 7, FullName: "Siti Aminah",
		NIK: "7306014101900001", Location: "Tempe, Wajo", ProgramType: "farmer", Eligibility: "eligible",
	}}}
	service := NewService(repository)

	results, err := service.Search(context.Background(), " schedule-1 ", " 7 ", 200)
	if err != nil {
		t.Fatal(err)
	}
	if repository.searchQuery != "7" || repository.searchLimit != 20 {
		t.Fatalf("query=%q limit=%d", repository.searchQuery, repository.searchLimit)
	}
	if len(results) != 1 || results[0].MaskedNIK != "7306********0001" {
		t.Fatalf("results=%+v", results)
	}
}

func TestSaveDraftRequiresReasonWhenNIKChanges(t *testing.T) {
	repository := &repositoryStub{workspace: RecipientWorkspace{NIK: "7306014101900001"}}
	service := NewService(repository)
	input := DraftInput{NIK: "7306014101900002"}

	_, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{})
	if !errors.Is(err, ErrIdentityChangeReasonRequired) {
		t.Fatalf("err=%v", err)
	}
	input.IdentityChangeReason = "Perbaikan berdasarkan KTP asli"
	if _, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.saved.IdentityChangeReason == "" {
		t.Fatal("identity change reason was not persisted")
	}
}

func TestSaveDraftPassesEquipmentFieldsThrough(t *testing.T) {
	repository := &repositoryStub{workspace: RecipientWorkspace{NIK: "7306014101900001"}}
	service := NewService(repository)
	input := DraftInput{
		NIK:                   "7306014101900001",
		MachineOptionCode:     " shark-spwp8030 ",
		MachineSerialNumber:   " SP 06IABD 421291 ",
		HoseOptionCode:        " triliunhose ",
		HoseSerialNumber:      "",
		ConverterSerialNumber: " 240A005582 ",
	}

	_, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saved.MachineOptionCode != "shark-spwp8030" || repository.saved.MachineSerialNumber != "SP 06IABD 421291" {
		t.Fatalf("machine fields not trimmed/passed: %+v", repository.saved)
	}
	if repository.saved.HoseOptionCode != "triliunhose" || repository.saved.HoseSerialNumber != "" {
		t.Fatalf("hose fields not trimmed/passed: %+v", repository.saved)
	}
	if repository.saved.ConverterSerialNumber != "240A005582" {
		t.Fatalf("converter serial not trimmed/passed: %+v", repository.saved)
	}
}

func TestUploadMediaDetectsImageAndCleansStorageWhenMetadataFails(t *testing.T) {
	storage := &storageStub{}
	repository := &mediaRepositoryStub{slot: MediaSlot{ID: "slot-1", InputSource: "both", MinFiles: 1, MaxFiles: 2}}
	service := NewService(repository, storage)
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	media, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.txt", Source: "camera", Data: jpeg}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if media.MimeType != "image/jpeg" || storage.putKey == "" || repository.media.StorageKey != storage.putKey {
		t.Fatalf("media=%+v key=%q", media, storage.putKey)
	}

	repository.saveErr = errors.New("database unavailable")
	_, err = service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.jpg", Source: "camera", Data: jpeg}, auth.ClientMeta{})
	if err == nil || storage.deletedKey == "" {
		t.Fatalf("err=%v deleted=%q", err, storage.deletedKey)
	}
}

func TestUploadMediaEnforcesTypeSizeAndSlotRequirements(t *testing.T) {
	locationRequired := &mediaRepositoryStub{slot: MediaSlot{ID: "slot-1", InputSource: "camera", MinFiles: 1, MaxFiles: 1, RequireLocation: true, RequireCapturedAt: true}}
	service := NewService(locationRequired, &storageStub{})
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "gallery", Data: []byte("not an image")}, auth.ClientMeta{}); !errors.Is(err, ErrMediaSourceInvalid) {
		t.Fatalf("source err=%v", err)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}); !errors.Is(err, ErrMediaLocationRequired) {
		t.Fatalf("location err=%v", err)
	}
	lat, lng := float64(-4.1), float64(120.2)
	captured := time.Now()
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg, Latitude: &lat, Longitude: &lng, CapturedAt: &captured}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMediaRestoresMetadataWhenStorageDeleteFails(t *testing.T) {
	storage := &storageStub{deleteErr: errors.New("disk unavailable")}
	repository := &mediaRepositoryStub{media: MediaFile{ID: "media-1", StorageKey: "opaque-key", Status: "accepted"}}
	service := NewService(repository, storage)
	if err := service.DeleteMedia(context.Background(), auth.Principal{}, "media-1", auth.ClientMeta{}); err == nil {
		t.Fatal("expected delete failure")
	}
	if repository.restoredID != "media-1" {
		t.Fatalf("restored=%q", repository.restoredID)
	}
}
