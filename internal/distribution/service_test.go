package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"konkit/internal/auth"
)

type storageStub struct {
	putKey, deletedKey string
	content            []byte
	putErr, deleteErr  error
	folderPath         []string
}

type operationsRepositoryStub struct {
	createInput CreateSlotInput
	linkInput   LinkSlotInput
}

func (r *operationsRepositoryStub) CreateSlot(_ context.Context, _ auth.Principal, input CreateSlotInput, _ auth.ClientMeta) (DistributionSlot, error) {
	r.createInput = input
	return DistributionSlot{}, nil
}

func (r *operationsRepositoryStub) SearchCandidate(context.Context, string, string, auth.RegencyScope) (CandidateMatch, error) {
	return CandidateMatch{}, nil
}

func (r *operationsRepositoryStub) LinkSlot(_ context.Context, _ auth.Principal, input LinkSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.linkInput = input
	return DistributionSlot{}, nil
}

func TestDistributionUppercasesSerialsAndRecipientBusinessText(t *testing.T) {
	repository := &operationsRepositoryStub{}
	service := NewService(repository)
	if _, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", SlotNumber: 1,
		MachineOptionCode: "shark-spwp8030", MachineSerialNumber: " ms-a1 ",
		HoseOptionCode: "hose-set", HoseSerialNumber: " hs-b2 ",
		ConverterOptionCode: "ergas-kit", ConverterSerialNumber: " cv-c3 ",
	}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.createInput.MachineSerialNumber != "MS-A1" || repository.createInput.HoseSerialNumber != "HS-B2" || repository.createInput.ConverterSerialNumber != "CV-C3" {
		t.Fatalf("serials=%+v", repository.createInput)
	}
	if repository.createInput.MachineOptionCode != "shark-spwp8030" || repository.createInput.HoseOptionCode != "hose-set" || repository.createInput.ConverterOptionCode != "ergas-kit" {
		t.Fatalf("option codes changed: %+v", repository.createInput)
	}

	if _, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{
		ScheduleID: "schedule-1", SlotNumber: 1, NIK: "9171031707010004", SectorIdentifier: " kp-01 ",
		Address: " jl. tani ", Village: " desa baru ", District: " wajo ", PhoneNumber: "0812-3456",
	}, auth.ClientMeta{}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.linkInput.SectorIdentifier != "KP01" || repository.linkInput.Address != "JL. TANI" || repository.linkInput.Village != "DESA BARU" || repository.linkInput.District != "WAJO" {
		t.Fatalf("link input=%+v", repository.linkInput)
	}
	if repository.linkInput.NIK != "9171031707010004" || repository.linkInput.PhoneNumber != "08123456" {
		t.Fatalf("numeric identifiers=%+v", repository.linkInput)
	}
}

func (s *storageStub) Put(_ context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	s.putKey = key
	s.folderPath = append([]string(nil), folderPath...)
	s.content, _ = io.ReadAll(source)
	return key, int64(len(s.content)), "checksum", s.putErr
}

func configuredMediaSlot() MediaSlot {
	return MediaSlot{ID: "slot-1", InputSource: "both", MinFiles: 1, MaxFiles: 2, ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Wajo"}
}

func TestUploadMediaUsesProgramZoneFolderAndRejectsPlaceholder(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	storage := &storageStub{}
	repository := &mediaRepositoryStub{slot: configuredMediaSlot()}
	service := NewService(repository, storage)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "PENDISTRIBUSIAN"}
	if fmt.Sprint(storage.folderPath) != fmt.Sprint(want) {
		t.Fatalf("folderPath=%v, want %v", storage.folderPath, want)
	}

	storage = &storageStub{}
	repository.slot.ZoneName = ""
	service = NewService(repository, storage)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err == nil || storage.putKey != "" {
		t.Fatalf("placeholder err=%v putKey=%q", err, storage.putKey)
	}
}
func (s *storageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}
func (s *storageStub) Delete(_ context.Context, key string) error {
	s.deletedKey = key
	return s.deleteErr
}
func (s *storageStub) EnsureFolders(context.Context, [][]string) error {
	return nil
}

type mediaRepositoryStub struct {
	slot       MediaSlot
	media      MediaFile
	saveErr    error
	restoredID string
	mediaScope auth.RegencyScope
}

func (r *mediaRepositoryStub) GetMediaSlot(_ context.Context, _ string, scope auth.RegencyScope) (MediaSlot, error) {
	r.mediaScope = scope
	return r.slot, nil
}
func (r *mediaRepositoryStub) SaveMedia(_ context.Context, _ auth.Principal, input MediaFileInput, _ auth.ClientMeta) (MediaFile, error) {
	if r.saveErr != nil {
		return MediaFile{}, r.saveErr
	}
	r.media = MediaFile{ID: "media-1", SlotID: input.SlotID, StorageKey: input.StorageKey, MimeType: input.MimeType, ByteSize: input.ByteSize, Status: "accepted"}
	return r.media, nil
}
func (r *mediaRepositoryStub) GetMedia(_ context.Context, _ string, scope auth.RegencyScope) (MediaFile, error) {
	r.mediaScope = scope
	return r.media, nil
}
func (r *mediaRepositoryStub) DeleteMedia(_ context.Context, _ auth.Principal, _ string, _ auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	r.mediaScope = scope
	return r.media, nil
}
func (r *mediaRepositoryStub) RestoreMedia(_ context.Context, id string) error {
	r.restoredID = id
	return nil
}

func TestUploadMediaDetectsImageAndCleansStorageWhenMetadataFails(t *testing.T) {
	storage := &storageStub{}
	repository := &mediaRepositoryStub{slot: configuredMediaSlot()}
	service := NewService(repository, storage)
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	media, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.txt", Source: "camera", Data: jpeg}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if media.MimeType != "image/jpeg" || storage.putKey == "" || repository.media.StorageKey != storage.putKey {
		t.Fatalf("media=%+v key=%q", media, storage.putKey)
	}

	repository.saveErr = errors.New("database unavailable")
	_, err = service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.jpg", Source: "camera", Data: jpeg}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err == nil || storage.deletedKey == "" {
		t.Fatalf("err=%v deleted=%q", err, storage.deletedKey)
	}
}

func TestUploadMediaEnforcesTypeSizeAndSlotRequirements(t *testing.T) {
	slot := configuredMediaSlot()
	slot.InputSource, slot.MaxFiles, slot.RequireLocation, slot.RequireCapturedAt = "camera", 1, true, true
	locationRequired := &mediaRepositoryStub{slot: slot}
	service := NewService(locationRequired, &storageStub{})
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "gallery", Data: []byte("not an image")}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaSourceInvalid) {
		t.Fatalf("source err=%v", err)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaLocationRequired) {
		t.Fatalf("location err=%v", err)
	}
	lat, lng := float64(-4.1), float64(120.2)
	captured := time.Now()
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg, Latitude: &lat, Longitude: &lng, CapturedAt: &captured}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMediaRestoresMetadataWhenStorageDeleteFails(t *testing.T) {
	storage := &storageStub{deleteErr: errors.New("disk unavailable")}
	repository := &mediaRepositoryStub{media: MediaFile{ID: "media-1", StorageKey: "opaque-key", Status: "accepted"}}
	service := NewService(repository, storage)
	if err := service.DeleteMedia(context.Background(), auth.Principal{}, "media-1", auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err == nil {
		t.Fatal("expected delete failure")
	}
	if repository.restoredID != "media-1" {
		t.Fatalf("restored=%q", repository.restoredID)
	}
}

func TestUploadDeleteAndOpenMediaForwardRegencyScope(t *testing.T) {
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	storage := &storageStub{}
	slot := configuredMediaSlot()
	slot.InputSource = "camera"
	repository := &mediaRepositoryStub{slot: slot, media: MediaFile{ID: "media-1", StorageKey: "opaque-key", Status: "accepted"}}
	service := NewService(repository, storage)
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)

	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("UploadMedia scope=%+v", repository.mediaScope)
	}

	repository.mediaScope = auth.RegencyScope{}
	if _, err := service.OpenMedia(context.Background(), "media-1", scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("OpenMedia scope=%+v", repository.mediaScope)
	}

	repository.mediaScope = auth.RegencyScope{}
	if err := service.DeleteMedia(context.Background(), auth.Principal{}, "media-1", auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("DeleteMedia scope=%+v", repository.mediaScope)
	}
}
