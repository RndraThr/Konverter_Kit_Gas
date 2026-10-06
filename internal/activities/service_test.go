package activities

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
	"konkit/internal/programs"
)

type repositoryStub struct {
	regency       regencyInfo
	regencyErr    error
	listResult    Page
	listErr       error
	insertResult  ActivityMedia
	insertErr     error
	getByIDResult ActivityMedia
	getByIDErr    error
	softDeleteKey string
	softDeleteErr error
	restoreCalled bool
	insertInput   insertInput
}

func (r *repositoryStub) GetRegency(context.Context, string, auth.RegencyScope) (regencyInfo, error) {
	return r.regency, r.regencyErr
}
func (r *repositoryStub) List(context.Context, Filter, auth.RegencyScope) (Page, error) {
	return r.listResult, r.listErr
}

func (r *repositoryStub) Insert(_ context.Context, _ auth.Principal, input insertInput, _ auth.ClientMeta) (ActivityMedia, error) {
	r.insertInput = input
	return r.insertResult, r.insertErr
}
func (r *repositoryStub) GetByID(context.Context, string, auth.RegencyScope) (ActivityMedia, error) {
	return r.getByIDResult, r.getByIDErr
}
func (r *repositoryStub) SoftDelete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (string, error) {
	return r.softDeleteKey, r.softDeleteErr
}
func (r *repositoryStub) restoreAfterFailedStorageDelete(context.Context, string) error {
	r.restoreCalled = true
	return nil
}

type storageStub struct {
	putKey, deletedKey string
	putFilename        string
	putErr, deleteErr  error
	folderPath         []string
}

func (s *storageStub) PutNamed(ctx context.Context, key, filename string, folderPath []string, source io.Reader) (string, int64, string, error) {
	s.putFilename = filename
	return s.Put(ctx, key, folderPath, source)
}

func (s *storageStub) Put(_ context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	s.putKey = key
	s.folderPath = append([]string(nil), folderPath...)
	data, _ := io.ReadAll(source)
	return "storage-" + key, int64(len(data)), "checksum", s.putErr
}
func (s *storageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s *storageStub) Delete(_ context.Context, key string) error {
	s.deletedKey = key
	return s.deleteErr
}
func (s *storageStub) EnsureFolders(context.Context, [][]string) error {
	return nil
}

var _ media.Storage = (*storageStub)(nil)

type programContextResolverStub struct {
	context programs.StorageContext
	err     error
}

func (r *programContextResolverStub) ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error) {
	return r.context, r.err
}

func TestUploadRequiresProgramBeforeStorage(t *testing.T) {
	storage := &storageStub{}
	service := NewService(&repositoryStub{}, storage, &programContextResolverStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg",
		Data: bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0}),
	}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrProgramRequired) {
		t.Fatalf("err=%v, want ErrProgramRequired", err)
	}
	if storage.putKey != "" {
		t.Fatal("storage.Put called without program context")
	}
}

func TestUploadRejectsPlaceholderAndBuildsProgramZoneFolder(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0}
	t.Run("placeholder", func(t *testing.T) {
		storage := &storageStub{}
		service := NewService(&repositoryStub{}, storage, &programContextResolverStub{err: programs.ErrZoneNotConfigured})
		_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
			ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg", Data: bytes.NewReader(jpeg),
		}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
		if !errors.Is(err, programs.ErrZoneNotConfigured) || storage.putKey != "" {
			t.Fatalf("err=%v putKey=%q", err, storage.putKey)
		}
	})

	t.Run("resolved", func(t *testing.T) {
		repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Kabupaten Wajo", DocumentCode: "WJO"}}
		storage := &storageStub{}
		resolver := &programContextResolverStub{context: programs.StorageContext{
			ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Kabupaten Wajo",
		}}
		service := NewService(repository, storage, resolver)
		service.now = func() time.Time { return time.Date(2026, time.October, 20, 15, 30, 45, 0, time.Local) }
		_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
			ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "a.jpg", Data: bytes.NewReader(jpeg),
		}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "RAKOR"}
		if fmt.Sprint(storage.folderPath) != fmt.Sprint(want) {
			t.Fatalf("folderPath=%v, want %v", storage.folderPath, want)
		}
		if repository.insertInput.ProgramID != "program-1" {
			t.Fatalf("persisted program_id=%q", repository.insertInput.ProgramID)
		}
		if storage.putFilename != "WJO-RAKOR-20261020-153045.jpg" {
			t.Fatalf("filename=%q", storage.putFilename)
		}
	})
}

func TestUploadRejectsInvalidActivityType(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "not_a_real_type", Source: "camera", OriginalFilename: "a.jpg",
		Data: strings.NewReader("data"),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrActivityTypeInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsInvalidSource(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "email", OriginalFilename: "a.jpg",
		Data: strings.NewReader("data"),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSourceInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsFileTooLarge(t *testing.T) {
	service := configuredActivityService(&repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}}, &storageStub{}, media.NewVideoLimiter(3))
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg",
		DeclaredSize: media.MaxImageBytes + 1, Data: bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0}),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsUnrecognizedFileType(t *testing.T) {
	service := configuredActivityService(&repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}, &storageStub{}, media.NewVideoLimiter(3))
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.txt",
		Data: strings.NewReader("plain text, not an image or video"),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrMediaTypeInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadShortCircuitsOnRegencyScopeFailure(t *testing.T) {
	repository := &repositoryStub{regencyErr: ErrRegencyNotFound}
	storage := &storageStub{}
	service := NewService(repository, storage, &programContextResolverStub{context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"}})

	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg",
		Data: bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}),
	}, auth.ClientMeta{}, auth.RegencyScope{RegencyIDs: []string{"other-regency"}})

	if !errors.Is(err, ErrRegencyNotFound) {
		t.Fatalf("err = %v", err)
	}
	if storage.putKey != "" {
		t.Fatal("expected storage.Put to never be called when GetRegency fails")
	}
}

func TestUploadGeneratesAKeyAndPersistsTheReturnedStorageKey(t *testing.T) {
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}
	storage := &storageStub{}
	service := NewService(repository, storage, &programContextResolverStub{context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"}})
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}

	if _, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg", Data: bytes.NewReader(jpeg),
	}, auth.ClientMeta{}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if storage.putKey == "" {
		t.Fatal("expected Put to be called with a generated key")
	}
}

func TestDeleteRestoresOnFailedStorageDelete(t *testing.T) {
	repository := &repositoryStub{softDeleteKey: "storage-key-1"}
	storage := &storageStub{deleteErr: errors.New("drive unavailable")}
	service := NewService(repository, storage)

	err := service.Delete(context.Background(), auth.Principal{}, "media-1", auth.ClientMeta{}, auth.RegencyScope{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !repository.restoreCalled {
		t.Fatal("expected restoreAfterFailedStorageDelete to be called")
	}
}

func TestOpenContentReturnsStorageReaderAndMetadata(t *testing.T) {
	repository := &repositoryStub{getByIDResult: ActivityMedia{StorageKey: "storage-key-1", MimeType: "image/jpeg", OriginalFilename: "a.jpg"}}
	service := NewService(repository, &storageStub{})

	content, err := service.OpenContent(context.Background(), "media-1", auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if content.MimeType != "image/jpeg" || content.Filename != "a.jpg" {
		t.Fatalf("content = %+v", content)
	}
	_ = content.Reader.Close()
}

func TestActivityUploadAcceptsExactImageAndVideoLimits(t *testing.T) {
	for _, tt := range []struct {
		name, filename string
		prefix         []byte
		limit          int64
		wantMIME       string
	}{
		{name: "image 25 MiB", filename: "proof.jpg", prefix: []byte{0xff, 0xd8, 0xff, 0xe0}, limit: media.MaxImageBytes, wantMIME: "image/jpeg"},
		{name: "video 500 MiB", filename: "proof.mp4", prefix: []byte("\x00\x00\x00\x18ftypisom"), limit: media.MaxVideoBytes, wantMIME: "video/mp4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}}
			storage := &countingActivityStorage{}
			service := configuredActivityService(repository, storage, media.NewVideoLimiter(3))
			data := io.MultiReader(bytes.NewReader(tt.prefix), io.LimitReader(activityRepeatingReader{}, tt.limit-int64(len(tt.prefix))))
			_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: tt.filename, DeclaredSize: tt.limit, Data: data}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
			if err != nil {
				t.Fatal(err)
			}
			if repository.insertInput.ByteSize != tt.limit || repository.insertInput.MimeType != tt.wantMIME {
				t.Fatalf("insert=%+v", repository.insertInput)
			}
		})
	}
}

func TestActivityUploadMOVFallbackAndOversizeCleanup(t *testing.T) {
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}}
	storage := &countingActivityStorage{}
	service := configuredActivityService(repository, storage, media.NewVideoLimiter(3))
	mov := append([]byte("\x00\x00\x00\x14ftypqt  "), bytes.Repeat([]byte{0}, 32)...)
	if _, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.mov", Data: bytes.NewReader(mov)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true}); err != nil {
		t.Fatal(err)
	}
	if repository.insertInput.MimeType != "video/quicktime" {
		t.Fatalf("mime=%q", repository.insertInput.MimeType)
	}

	repository = &repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}}
	storage = &countingActivityStorage{}
	service = configuredActivityService(repository, storage, media.NewVideoLimiter(3))
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0}
	data := io.MultiReader(bytes.NewReader(jpeg), io.LimitReader(activityRepeatingReader{}, media.MaxImageBytes+1-int64(len(jpeg))))
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.jpg", DeclaredSize: media.MaxImageBytes, Data: data}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrFileTooLarge) || storage.deletedKey == "" || repository.insertInput.StorageKey != "" {
		t.Fatalf("err=%v deleted=%q insert=%+v", err, storage.deletedKey, repository.insertInput)
	}
}

func TestActivityUploadLimiterCancellationAndFailureCleanup(t *testing.T) {
	video := append([]byte("\x00\x00\x00\x18ftypisom"), bytes.Repeat([]byte{0}, 32)...)
	limiter := media.NewVideoLimiter(3)
	for range 3 {
		_, _ = limiter.TryAcquire()
	}
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}}
	storage := &countingActivityStorage{}
	service := configuredActivityService(repository, storage, limiter)
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.mp4", Data: bytes.NewReader(video)}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrVideoUploadBusy) || storage.putCalls != 0 {
		t.Fatalf("busy err=%v calls=%d", err, storage.putCalls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	storage = &countingActivityStorage{}
	service = configuredActivityService(repository, storage, media.NewVideoLimiter(3))
	_, err = service.Upload(ctx, auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.jpg", Data: bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0})}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}

	storage = &countingActivityStorage{putErr: errors.New("drive failed")}
	service = configuredActivityService(repository, storage, media.NewVideoLimiter(3))
	_, err = service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.jpg", Data: bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0})}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err == nil {
		t.Fatal("expected storage failure")
	}

	repository = &repositoryStub{regency: regencyInfo{ID: "regency-1", DocumentCode: "WJO"}, insertErr: errors.New("database failed")}
	storage = &countingActivityStorage{}
	service = configuredActivityService(repository, storage, media.NewVideoLimiter(3))
	_, err = service.Upload(context.Background(), auth.Principal{}, UploadInput{ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "proof.jpg", Data: bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0})}, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err == nil || storage.deletedKey == "" {
		t.Fatalf("repository err=%v deleted=%q", err, storage.deletedKey)
	}
}

func configuredActivityService(repository *repositoryStub, storage media.Storage, limiter *media.VideoLimiter) *Service {
	return NewService(repository, storage, &programContextResolverStub{context: programs.StorageContext{ProgramID: "program-1", ProgramType: programs.ProgramFarmer, ZoneName: "Zona 1", RegencyID: "regency-1", RegencyName: "Wajo"}}, limiter)
}

type activityRepeatingReader struct{}

func (activityRepeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0xff
	}
	return len(p), nil
}

type countingActivityStorage struct {
	putCalls   int
	putErr     error
	deletedKey string
}

func (s *countingActivityStorage) Put(ctx context.Context, key string, folder []string, source io.Reader) (string, int64, string, error) {
	return s.PutNamed(ctx, key, key, folder, source)
}
func (s *countingActivityStorage) PutNamed(ctx context.Context, key, _ string, _ []string, source io.Reader) (string, int64, string, error) {
	s.putCalls++
	if err := ctx.Err(); err != nil {
		return "", 0, "", err
	}
	size, err := io.Copy(io.Discard, source)
	if err == nil {
		err = s.putErr
	}
	return "storage-" + key, size, "checksum", err
}
func (s *countingActivityStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *countingActivityStorage) Delete(_ context.Context, key string) error {
	s.deletedKey = key
	return nil
}
func (s *countingActivityStorage) EnsureFolders(context.Context, [][]string) error { return nil }
