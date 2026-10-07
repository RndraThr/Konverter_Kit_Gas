package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrInvalidKey = errors.New("media storage key is invalid")
var ErrInvalidFilename = errors.New("media storage filename is invalid")

var validKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Storage interface {
	// Put stores source under key. folderPath is a hint for backends that
	// organize content into folders (e.g. Google Drive); LocalStorage
	// ignores it and always stores flat. The returned storageKey is what
	// callers must persist and pass to Open/Delete afterwards — for
	// LocalStorage this is always the same as key; for backends with their
	// own identity scheme (e.g. Google Drive file IDs) it is not.
	Put(ctx context.Context, key string, folderPath []string, source io.Reader) (storageKey string, size int64, checksum string, err error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error

	// EnsureFolders pre-creates (or, for backends that don't need it,
	// validates) each folder path in paths, idempotently. Callers use this
	// to guarantee a folder hierarchy exists ahead of time (e.g. reserved
	// document-category folders for a new zone) without uploading a file.
	EnsureFolders(ctx context.Context, paths [][]string) error
}

type MovableStorage interface {
	Storage
	Move(ctx context.Context, storageKey string, targetPath []string) error
}

type namedStorage interface {
	PutNamed(ctx context.Context, key, filename string, folderPath []string, source io.Reader) (storageKey string, size int64, checksum string, err error)
}

// PutNamed stores an object using a stable internal key while allowing
// backends with user-visible filenames (such as Google Drive) to display a
// separate filename. Backends without named-object support safely fall back
// to Put and continue using the internal key.
func PutNamed(ctx context.Context, storage Storage, key, filename string, folderPath []string, source io.Reader) (string, int64, string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsAny(filename, `/\`) {
		return "", 0, "", ErrInvalidFilename
	}
	for _, r := range filename {
		if r < 32 || r == 127 {
			return "", 0, "", ErrInvalidFilename
		}
	}
	if named, ok := storage.(namedStorage); ok {
		return named.PutNamed(ctx, key, filename, folderPath, source)
	}
	return storage.Put(ctx, key, folderPath, source)
}

type LocalStorage struct{ root string }

func NewLocalStorage(root string) (*LocalStorage, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve media storage root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("create media storage root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect media storage root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("media storage root is not a directory")
	}
	return &LocalStorage{root: absolute}, nil
}

func (s *LocalStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (storageKey string, size int64, checksum string, resultErr error) {
	path, err := s.path(key)
	if err != nil {
		return "", 0, "", err
	}
	if err := ctx.Err(); err != nil {
		return "", 0, "", err
	}
	temporary, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return "", 0, "", fmt.Errorf("create temporary media file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if resultErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o640); err != nil {
		return "", 0, "", fmt.Errorf("secure temporary media file: %w", err)
	}
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(temporary, hash), source)
	if err != nil {
		return "", 0, "", fmt.Errorf("write media file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", 0, "", fmt.Errorf("sync media file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", 0, "", fmt.Errorf("close media file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", 0, "", fmt.Errorf("commit media file: %w", err)
	}
	return key, size, hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete media file: %w", err)
	}
	return nil
}

// EnsureFolders validates each folder path's shape. LocalStorage stores
// files flat (see Put) and has no real folder hierarchy to pre-create, so
// this is otherwise a no-op — it exists so LocalStorage satisfies Storage
// and so callers get the same validation errors regardless of backend.
func (s *LocalStorage) EnsureFolders(ctx context.Context, paths [][]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, folderPath := range paths {
		if err := validateFolderPath(folderPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *LocalStorage) Move(ctx context.Context, storageKey string, targetPath []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.path(storageKey); err != nil {
		return err
	}
	return validateFolderPath(targetPath)
}

func validateFolderPath(folderPath []string) error {
	if len(folderPath) == 0 {
		return ErrInvalidFolderPath
	}
	for _, segment := range folderPath {
		if strings.TrimSpace(segment) == "" || strings.ContainsAny(segment, `/\`) {
			return ErrInvalidFolderPath
		}
	}
	return nil
}

func (s *LocalStorage) path(key string) (string, error) {
	if !validKey.MatchString(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(s.root, key), nil
}
