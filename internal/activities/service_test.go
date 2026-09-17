package activities

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/media"
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
}

func (r *repositoryStub) GetRegency(context.Context, string, auth.RegencyScope) (regencyInfo, error) {
	return r.regency, r.regencyErr
}
func (r *repositoryStub) List(context.Context, Filter, auth.RegencyScope) (Page, error) {
	return r.listResult, r.listErr
}
func (r *repositoryStub) Insert(context.Context, auth.Principal, insertInput, auth.ClientMeta) (ActivityMedia, error) {
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
	putErr, deleteErr  error
}

func (s *storageStub) Put(_ context.Context, key string, _ []string, source io.Reader) (string, int64, string, error) {
	s.putKey = key
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

var _ media.Storage = (*storageStub)(nil)

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

func TestUploadGeneratesAKeyAndPersistsTheReturnedStorageKey(t *testing.T) {
	repository := &repositoryStub{regency: regencyInfo{ID: "regency-1", Name: "Wajo", DocumentCode: "WJO"}}
	storage := &storageStub{}
	service := NewService(repository, storage)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}

	if _, err := service.Upload(context.Background(), auth.Principal{}, UploadInput{
		RegencyID: "regency-1", ActivityType: "rakor", Source: "camera", OriginalFilename: "a.jpg", Data: jpeg,
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
