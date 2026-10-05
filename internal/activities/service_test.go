package activities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0},
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
			ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg", Data: jpeg,
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
			ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "gallery", OriginalFilename: "a.jpg", Data: jpeg,
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
		Data: []byte("data"),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrActivityTypeInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsInvalidSource(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "email", OriginalFilename: "a.jpg",
		Data: []byte("data"),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSourceInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsFileTooLarge(t *testing.T) {
	service := NewService(&repositoryStub{}, &storageStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg",
		Data: make([]byte, maxFileBytes+1),
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRejectsUnrecognizedFileType(t *testing.T) {
	service := NewService(&repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}, &storageStub{})
	_, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.txt",
		Data: []byte("plain text, not an image or video"),
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
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0},
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
		ProgramID: "program-1", RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg", Data: jpeg,
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
