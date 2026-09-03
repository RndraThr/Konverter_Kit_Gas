package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type failingReader struct{ sent bool }

func (r *failingReader) Read(target []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(target, []byte("partial")), nil
	}
	return 0, errors.New("source interrupted")
}

func TestLocalStorageRejectsTraversalAndWritesAtomically(t *testing.T) {
	root := t.TempDir()
	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../secret", "folder/file", `folder\file`, ""} {
		if _, _, err := storage.Put(context.Background(), key, bytes.NewBufferString("secret")); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("key=%q err=%v", key, err)
		}
	}

	size, checksum, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440000", bytes.NewBufferString("photo"))
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 || checksum != "55c64d0fcd6f9d5f7c828093857e3fdfda68478bb4e9bd24d481ef391c7804e8" {
		t.Fatalf("size=%d checksum=%s", size, checksum)
	}
	reader, err := storage.Open(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "photo" {
		t.Fatalf("content=%q", content)
	}

	if _, _, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440001", &failingReader{}); err == nil {
		t.Fatal("expected interrupted write to fail")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("partial files remain: %+v", entries)
	}
}

func TestLocalStorageDeleteIsIdempotent(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Delete(context.Background(), "550e8400-e29b-41d4-a716-446655440099"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStorageRequiresDirectoryPath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalStorage(file); err == nil {
		t.Fatal("expected file root to be rejected")
	}
}
