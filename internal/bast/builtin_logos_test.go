package bast

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/database/migrations"
)

// The migration seeds byte_size and checksum literals; they must describe the embedded files.
func TestBuiltinLogosMatchMigrationSeed(t *testing.T) {
	seed, err := migrations.FS.ReadFile("00072_builtin_ba_logos.sql")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := builtinLogoFS.ReadDir("builtin_logos")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("embedded logos = %d, want 4", len(entries))
	}
	for _, entry := range entries {
		data, err := builtinLogoFS.ReadFile("builtin_logos/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		row := fmt.Sprintf("'%s', %d, '%s'", entry.Name(), len(data), hex.EncodeToString(sum[:]))
		if !strings.Contains(string(seed), row) {
			t.Errorf("migration 00072 does not seed %s", row)
		}
	}
}

func TestReadStoredLogoServesBuiltinWithoutStorage(t *testing.T) {
	data, err := readStoredLogo(context.Background(), nil, "builtin:8a3c0e0e-0000-4000-8000-000000000001/04-pertamina-transparan.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 3365 {
		t.Fatalf("bytes = %d, want 3365", len(data))
	}
	for _, key := range []string{"builtin:p1/missing.png", "builtin:p1/../logo_loader.go", "builtin:"} {
		if _, err := readStoredLogo(context.Background(), nil, key); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: err = %v, want ErrNotFound", key, err)
		}
	}
	if _, err := readStoredLogo(context.Background(), nil, "drive-file-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("stored key without storage: err = %v, want ErrNotFound", err)
	}
}

type builtinBrandingRepo struct {
	fakeBrandingRepo
	key string
}

func (f *builtinBrandingRepo) GetLogo(_ context.Context, _, logoID string) (LogoAsset, error) {
	return LogoAsset{ID: logoID, StorageKey: f.key, MimeType: "image/png", OriginalFilename: "04-pertamina-transparan.png"}, nil
}

func TestBrandingServiceOpensBuiltinLogo(t *testing.T) {
	repo := &builtinBrandingRepo{key: "builtin:p1/04-pertamina-transparan.png"}
	content, err := NewBrandingService(repo, nil).OpenLogo(context.Background(), "p1", "logo-1")
	if err != nil {
		t.Fatal(err)
	}
	defer content.Reader.Close()
	data, _ := io.ReadAll(content.Reader)
	if len(data) != 3365 || content.MimeType != "image/png" {
		t.Fatalf("got %d bytes %q", len(data), content.MimeType)
	}
}

func TestBrandingServiceUploadKeepsReplacedBuiltinLogo(t *testing.T) {
	repo := &fakeBrandingRepo{code: "KONKIT-2026", savedOld: "builtin:p1/04-pertamina-transparan.png"}
	storage := &fakeStorage{putKey: "stored-key"}
	_, err := NewBrandingService(repo, storage).UploadLogo(context.Background(), auth.Principal{}, LogoUploadInput{ProgramID: "p1", SlotCode: "logo_1", OriginalFilename: "baru.png", Data: pngData()}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("a built-in logo has nothing in storage to delete, deleted %v", storage.deleted)
	}
}
