package bast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"konkit/internal/auth"
)

type fakeBrandingRepo struct {
	code       string
	codeErr    error
	saved      LogoAsset
	savedOld   string
	saveErr    error
	lastUpload LogoAsset
}

func (f *fakeBrandingRepo) GetProgramCode(_ context.Context, _ string) (string, error) {
	return f.code, f.codeErr
}
func (f *fakeBrandingRepo) ListLogos(_ context.Context, programID string) ([]LogoAsset, error) {
	return []LogoAsset{{ID: "logo-1", ProgramID: programID, SlotCode: "left"}}, nil
}
func (f *fakeBrandingRepo) SaveLogo(_ context.Context, _ auth.Principal, logo LogoAsset, _ auth.ClientMeta) (LogoAsset, string, error) {
	f.lastUpload = logo
	if f.saveErr != nil {
		return LogoAsset{}, "", f.saveErr
	}
	logo.ID = "new-logo"
	return logo, f.savedOld, nil
}
func (f *fakeBrandingRepo) UpdateLogo(_ context.Context, _ auth.Principal, input LogoPatchInput, _ auth.ClientMeta) (LogoAsset, error) {
	return LogoAsset{ID: input.ID, ProgramID: input.ProgramID, SortOrder: input.SortOrder, IsVisible: input.IsVisible}, nil
}
func (f *fakeBrandingRepo) GetLogo(_ context.Context, _, logoID string) (LogoAsset, error) {
	return LogoAsset{ID: logoID, StorageKey: "key-" + logoID, MimeType: "image/png"}, nil
}

type fakeStorage struct {
	putKey    string
	deleted   []string
	openBytes []byte
}

func (f *fakeStorage) Put(_ context.Context, _ string, _ []string, source io.Reader) (string, int64, string, error) {
	data, _ := io.ReadAll(source)
	return f.putKey, int64(len(data)), strings.Repeat("a", 64), nil
}
func (f *fakeStorage) Open(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.openBytes)), nil
}
func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func (f *fakeStorage) EnsureFolders(_ context.Context, _ [][]string) error { return nil }

// pngData returns bytes http.DetectContentType recognizes as image/png.
func pngData() []byte {
	return append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 16)...)
}

func TestBrandingServiceUploadValidatesAndStores(t *testing.T) {
	repo := &fakeBrandingRepo{code: "KONKIT-2026", savedOld: "old-key"}
	storage := &fakeStorage{putKey: "stored-key"}
	svc := NewBrandingService(repo, storage)

	logo, err := svc.UploadLogo(context.Background(), auth.Principal{}, LogoUploadInput{ProgramID: "p1", SlotCode: "left", OriginalFilename: "l.png", Data: pngData(), SortOrder: 0}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if logo.ID != "new-logo" || repo.lastUpload.MimeType != "image/png" {
		t.Fatalf("unexpected stored logo: %+v (saved mime %q)", logo, repo.lastUpload.MimeType)
	}
	if repo.lastUpload.MaxWidthMM != 35 || repo.lastUpload.MaxHeightMM != 18 {
		t.Fatalf("expected size defaults, got %v x %v", repo.lastUpload.MaxWidthMM, repo.lastUpload.MaxHeightMM)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != "old-key" {
		t.Fatalf("expected old key deleted, got %v", storage.deleted)
	}
}

func TestBrandingServiceUploadRejectsNonImage(t *testing.T) {
	svc := NewBrandingService(&fakeBrandingRepo{code: "X"}, &fakeStorage{})
	_, err := svc.UploadLogo(context.Background(), auth.Principal{}, LogoUploadInput{ProgramID: "p1", SlotCode: "left", Data: []byte("not an image at all, plain text")}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for non-image, got %v", err)
	}
}

func TestBrandingServiceUploadRejectsOversize(t *testing.T) {
	svc := NewBrandingService(&fakeBrandingRepo{code: "X"}, &fakeStorage{})
	big := append(pngData(), make([]byte, maxLogoBytes+1)...)
	_, err := svc.UploadLogo(context.Background(), auth.Principal{}, LogoUploadInput{ProgramID: "p1", SlotCode: "left", Data: big}, auth.ClientMeta{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for oversize, got %v", err)
	}
}

func TestBrandingServiceListRejectsMissingProgram(t *testing.T) {
	svc := NewBrandingService(&fakeBrandingRepo{codeErr: ErrNotFound}, &fakeStorage{})
	if _, err := svc.ListBranding(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
