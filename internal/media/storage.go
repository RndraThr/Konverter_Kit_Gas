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
)

var ErrInvalidKey = errors.New("media storage key is invalid")

var validKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Storage interface {
	Put(context.Context, string, io.Reader) (int64, string, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
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

func (s *LocalStorage) Put(ctx context.Context, key string, source io.Reader) (size int64, checksum string, resultErr error) {
	path, err := s.path(key)
	if err != nil {
		return 0, "", err
	}
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}
	temporary, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return 0, "", fmt.Errorf("create temporary media file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if resultErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o640); err != nil {
		return 0, "", fmt.Errorf("secure temporary media file: %w", err)
	}
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(temporary, hash), source)
	if err != nil {
		return 0, "", fmt.Errorf("write media file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return 0, "", fmt.Errorf("sync media file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return 0, "", fmt.Errorf("close media file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return 0, "", fmt.Errorf("commit media file: %w", err)
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
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

func (s *LocalStorage) path(key string) (string, error) {
	if !validKey.MatchString(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(s.root, key), nil
}
