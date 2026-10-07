package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
)

type storageStub struct {
	putKey, deletedKey string
	putFilename        string
	content            []byte
	putErr, deleteErr  error
	folderPath         []string
}

func (s *storageStub) PutNamed(ctx context.Context, key, filename string, folderPath []string, source io.Reader) (string, int64, string, error) {
	s.putFilename = filename
	return s.Put(ctx, key, folderPath, source)
}

type operationsRepositoryStub struct {
	createInput    CreateSlotInput
	linkInput      LinkSlotInput
	equipmentInput UpdateEquipmentInput
}

type mediaStageRepositoryStub struct {
	documentationID string
	mediaID         string
	scope           auth.RegencyScope
}

func (r *mediaStageRepositoryStub) DocumentationSlotStage(_ context.Context, id string, scope auth.RegencyScope) (string, error) {
	r.documentationID, r.scope = id, scope
	return "dokumen", nil
}

func (r *mediaStageRepositoryStub) MediaStage(_ context.Context, id string, scope auth.RegencyScope) (string, error) {
	r.mediaID, r.scope = id, scope
	return "penyerahan", nil
}

func TestMediaStageLookupsTrimIDsAndForwardScope(t *testing.T) {
	repository := &mediaStageRepositoryStub{}
	service := NewService(repository)
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	stage, err := service.DocumentationSlotStage(context.Background(), " docs-1 ", scope)
	if err != nil || stage != "dokumen" || repository.documentationID != "docs-1" || len(repository.scope.RegencyIDs) != 1 {
		t.Fatalf("documentation stage=%q id=%q scope=%+v err=%v", stage, repository.documentationID, repository.scope, err)
	}
	stage, err = service.MediaStage(context.Background(), " media-1 ", scope)
	if err != nil || stage != "penyerahan" || repository.mediaID != "media-1" {
		t.Fatalf("media stage=%q id=%q err=%v", stage, repository.mediaID, err)
	}
}

func (r *operationsRepositoryStub) CreateSlot(_ context.Context, _ auth.Principal, input CreateSlotInput, _ auth.RegencyScope, _ auth.ClientMeta) (DistributionSlot, error) {
	r.createInput = input
	return DistributionSlot{}, nil
}

func (r *operationsRepositoryStub) UpdateEquipment(_ context.Context, _ auth.Principal, input UpdateEquipmentInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.equipmentInput = input
	return DistributionSlot{}, nil
}

func (r *operationsRepositoryStub) SearchCandidate(context.Context, string, string, auth.RegencyScope) (CandidateMatch, error) {
	return CandidateMatch{}, nil
}

func (r *operationsRepositoryStub) SuggestCandidates(context.Context, string, string, int, auth.RegencyScope) ([]CandidateMatch, error) {
	return nil, nil
}

func (r *operationsRepositoryStub) LinkSlot(_ context.Context, _ auth.Principal, input LinkSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.linkInput = input
	return DistributionSlot{}, nil
}

func TestDistributionUppercasesSerialsAndRecipientBusinessText(t *testing.T) {
	repository := &operationsRepositoryStub{}
	service := NewService(repository)
	if _, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", SlotNumber: 1, DistributionDate: "2026-10-20",
		MachineOptionCode: "shark-spwp8030", MachineSerialNumber: " ms-a1 ",
		HoseOptionCode: "hose-set", HoseSerialNumber: " hs-b2 ",
		ConverterOptionCode: "ergas-kit", ConverterSerialNumber: " cv-c3 ",
	}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.createInput.MachineSerialNumber != "" || repository.createInput.HoseSerialNumber != "" || repository.createInput.ConverterSerialNumber != "" || repository.createInput.MachineOptionCode != "" || repository.createInput.HoseOptionCode != "" || repository.createInput.ConverterOptionCode != "" {
		t.Fatalf("create retained equipment: %+v", repository.createInput)
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

func TestUpdateEquipmentNormalizesInputAndDiscardsHoseSerial(t *testing.T) {
	repository := &operationsRepositoryStub{}
	service := NewService(repository)
	if _, err := service.UpdateEquipment(context.Background(), auth.Principal{}, UpdateEquipmentInput{
		ScheduleID: "schedule-1", SlotNumber: 1,
		MachineOptionCode: "shark-spwp8030", MachineSerialNumber: " ms-a1 ",
		HoseOptionCode: "hose-set", HoseSerialNumber: " hs-b2 ",
		ConverterOptionCode: "ergas-kit", ConverterSerialNumber: " cv-c3 ",
	}, auth.ClientMeta{}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.equipmentInput.MachineSerialNumber != "MS-A1" || repository.equipmentInput.HoseSerialNumber != "" || repository.equipmentInput.ConverterSerialNumber != "CV-C3" {
		t.Fatalf("serials=%+v", repository.equipmentInput)
	}
	if repository.equipmentInput.MachineOptionCode != "shark-spwp8030" || repository.equipmentInput.HoseOptionCode != "hose-set" || repository.equipmentInput.ConverterOptionCode != "ergas-kit" {
		t.Fatalf("option codes changed: %+v", repository.equipmentInput)
	}

	if _, err := service.UpdateEquipment(context.Background(), auth.Principal{}, UpdateEquipmentInput{ScheduleID: "", SlotNumber: 1}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("empty schedule err=%v", err)
	}
	if _, err := service.UpdateEquipment(context.Background(), auth.Principal{}, UpdateEquipmentInput{ScheduleID: "schedule-1", SlotNumber: 0}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrSlotNumberRequired) {
		t.Fatalf("missing slot number err=%v", err)
	}
}

func (s *storageStub) Put(_ context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	s.putKey = key
	s.folderPath = append([]string(nil), folderPath...)
	s.content, _ = io.ReadAll(source)
	return key, int64(len(s.content)), "checksum", s.putErr
}

func configuredMediaSlot() MediaSlot {
	return MediaSlot{ID: "slot-1", SlotNumber: 25, DistributionDate: "2026-10-20", Label: "Foto KTP dan Nomor Urut", InputSource: "both", MediaKind: "image", MinFiles: 1, MaxFiles: 1, ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Wajo"}
}

func TestUploadMediaUsesProgramZoneFolderAndRejectsPlaceholder(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	storage := &storageStub{}
	repository := &mediaRepositoryStub{slot: configuredMediaSlot()}
	service := NewService(repository, storage)
	captured := time.Date(2026, time.April, 2, 9, 30, 0, 0, time.FixedZone("WITA", 8*60*60))
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "camera", Data: bytes.NewReader(jpeg), CapturedAt: &captured}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "PENDISTRIBUSIAN", "20 Oktober 2026", "25"}
	if fmt.Sprint(storage.folderPath) != fmt.Sprint(want) {
		t.Fatalf("folderPath=%v, want %v", storage.folderPath, want)
	}
	if storage.putFilename != "FOTO KTP DAN NOMOR URUT.jpg" {
		t.Fatalf("filename=%q", storage.putFilename)
	}
	if repository.saveInput.CapturedAt == nil || !repository.saveInput.CapturedAt.Equal(captured) {
		t.Fatalf("captured_at=%v, want %v", repository.saveInput.CapturedAt, captured)
	}

	storage = &storageStub{}
	repository.slot.ZoneName = ""
	service = NewService(repository, storage)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "camera", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err == nil || storage.putKey != "" {
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
	saveInput  MediaFileInput
	saveErr    error
	restoredID string
	mediaScope auth.RegencyScope
}

func (r *mediaRepositoryStub) GetMediaSlot(_ context.Context, _ string, scope auth.RegencyScope) (MediaSlot, error) {
	r.mediaScope = scope
	return r.slot, nil
}
func (r *mediaRepositoryStub) SaveMedia(_ context.Context, _ auth.Principal, input MediaFileInput, _ auth.ClientMeta) (MediaFile, error) {
	r.saveInput = input
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
	media, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.txt", Source: "camera", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if media.MimeType != "image/jpeg" || storage.putKey == "" || repository.media.StorageKey != storage.putKey {
		t.Fatalf("media=%+v key=%q", media, storage.putKey)
	}

	repository.saveErr = errors.New("database unavailable")
	_, err = service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "foto.jpg", Source: "camera", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err == nil || storage.deletedKey == "" {
		t.Fatalf("err=%v deleted=%q", err, storage.deletedKey)
	}
}

func TestUploadMediaEnforcesTypeSizeAndSlotRequirements(t *testing.T) {
	slot := configuredMediaSlot()
	slot.InputSource, slot.MaxFiles, slot.RequireLocation, slot.RequireCapturedAt = "camera", 1, true, true
	locationRequired := &mediaRepositoryStub{slot: slot}
	service := NewService(locationRequired, &storageStub{})
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "fake.jpg", Source: "gallery", Data: strings.NewReader("not an image")}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaSourceInvalid) {
		t.Fatalf("source err=%v", err)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "camera", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrMediaLocationRequired) {
		t.Fatalf("location err=%v", err)
	}
	lat, lng := float64(-4.1), float64(120.2)
	captured := time.Now()
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "camera", Data: bytes.NewReader(jpeg), Latitude: &lat, Longitude: &lng, CapturedAt: &captured}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
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

	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "camera", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, scope); err != nil {
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

func TestUploadMediaEnforcesMediaPolicy(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 32)...)
	mp4 := append([]byte("\x00\x00\x00\x18ftypisom"), bytes.Repeat([]byte{0}, 32)...)
	for _, tt := range []struct {
		name, policy string
		data         []byte
		wantErr      error
	}{
		{name: "image accepts image", policy: "image", data: jpeg},
		{name: "image rejects video", policy: "image", data: mp4, wantErr: ErrMediaPolicyInvalid},
		{name: "video accepts video", policy: "video", data: mp4},
		{name: "video rejects image", policy: "video", data: jpeg, wantErr: ErrMediaPolicyInvalid},
		{name: "mixed accepts video", policy: "image_video", data: mp4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			slot := configuredMediaSlot()
			slot.MediaKind = tt.policy
			service := NewService(&mediaRepositoryStub{slot: slot}, &storageStub{}, media.NewVideoLimiter(3))
			_, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.mp4", Source: "gallery", DeclaredSize: int64(len(tt.data)), Data: bytes.NewReader(tt.data)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err=%v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUploadMediaRejectsDeclaredAndActualOversizeAndCleansStorage(t *testing.T) {
	slot := configuredMediaSlot()
	slot.MediaKind = "video"
	mp4 := []byte("\x00\x00\x00\x18ftypisom")

	storage := &countingStorageStub{}
	service := NewService(&mediaRepositoryStub{slot: slot}, storage, media.NewVideoLimiter(3))
	_, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.mp4", Source: "gallery", DeclaredSize: media.MaxVideoBytes + 1, Data: bytes.NewReader(mp4)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrMediaTooLarge) || storage.putCalls != 0 {
		t.Fatalf("declared oversize err=%v putCalls=%d", err, storage.putCalls)
	}

	storage = &countingStorageStub{}
	service = NewService(&mediaRepositoryStub{slot: slot}, storage, media.NewVideoLimiter(3))
	large := io.MultiReader(bytes.NewReader(mp4), io.LimitReader(repeatingByteReader{}, media.MaxVideoBytes+1-int64(len(mp4))))
	_, err = service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.mp4", Source: "gallery", DeclaredSize: media.MaxVideoBytes, Data: large}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrMediaTooLarge) || storage.deletedKey == "" {
		t.Fatalf("actual oversize err=%v deleted=%q", err, storage.deletedKey)
	}
}

func TestUploadMediaVideoLimiterDoesNotBlockImages(t *testing.T) {
	limiter := media.NewVideoLimiter(3)
	for range 3 {
		if _, ok := limiter.TryAcquire(); !ok {
			t.Fatal("failed to occupy video permit")
		}
	}
	slot := configuredMediaSlot()
	slot.MediaKind = "image_video"
	storage := &storageStub{}
	service := NewService(&mediaRepositoryStub{slot: slot}, storage, limiter)
	mp4 := append([]byte("\x00\x00\x00\x18ftypisom"), bytes.Repeat([]byte{0}, 32)...)
	_, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.mp4", Source: "gallery", Data: bytes.NewReader(mp4)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrVideoUploadBusy) || storage.putKey != "" {
		t.Fatalf("video err=%v put=%q", err, storage.putKey)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 32)...)
	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "gallery", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatalf("image should proceed while video permits are full: %v", err)
	}
}

func TestUploadMediaPropagatesCancellationWithoutSavingMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	storage := &countingStorageStub{}
	repository := &mediaRepositoryStub{slot: configuredMediaSlot()}
	service := NewService(repository, storage, media.NewVideoLimiter(3))
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 32)...)
	_, err := service.UploadMedia(ctx, auth.Principal{}, UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.jpg", Source: "gallery", Data: bytes.NewReader(jpeg)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, context.Canceled) || repository.saveInput.StorageKey != "" {
		t.Fatalf("err=%v saveInput=%+v", err, repository.saveInput)
	}
}

type repeatingByteReader struct{}

func (repeatingByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0xff
	}
	return len(p), nil
}

type countingStorageStub struct {
	putCalls   int
	deletedKey string
}

func (s *countingStorageStub) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	return s.PutNamed(ctx, key, key, folderPath, source)
}
func (s *countingStorageStub) PutNamed(ctx context.Context, key, _ string, _ []string, source io.Reader) (string, int64, string, error) {
	s.putCalls++
	if err := ctx.Err(); err != nil {
		return "", 0, "", err
	}
	size, err := io.Copy(io.Discard, source)
	return key, size, "checksum", err
}
func (s *countingStorageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *countingStorageStub) Delete(_ context.Context, key string) error {
	s.deletedKey = key
	return nil
}
func (s *countingStorageStub) EnsureFolders(context.Context, [][]string) error { return nil }
