package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// driveFilesAPI is the narrow slice of the Google Drive API that
// GoogleDriveStorage needs. Kept separate from *drive.Service so tests can
// fake it without any network access.
type driveFilesAPI interface {
	findFolder(ctx context.Context, name, parentID string) (id string, err error) // "" if not found
	createFolder(ctx context.Context, name, parentID string) (id string, err error)
	uploadFile(ctx context.Context, name, parentID string, r io.Reader) (id string, size int64, err error)
	downloadFile(ctx context.Context, id string) (io.ReadCloser, error)
	deleteFile(ctx context.Context, id string) error
}

type realDriveFilesAPI struct {
	service *drive.Service
}

const driveFolderMimeType = "application/vnd.google-apps.folder"

func (a *realDriveFilesAPI) findFolder(ctx context.Context, name, parentID string) (string, error) {
	escaped := strings.ReplaceAll(name, `'`, `\'`)
	query := fmt.Sprintf("name = '%s' and mimeType = '%s' and '%s' in parents and trashed = false", escaped, driveFolderMimeType, parentID)
	result, err := a.service.Files.List().Q(query).Fields("files(id, name)").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("list drive folders: %w", err)
	}
	if len(result.Files) == 0 {
		return "", nil
	}
	return result.Files[0].Id, nil
}

func (a *realDriveFilesAPI) createFolder(ctx context.Context, name, parentID string) (string, error) {
	folder := &drive.File{Name: name, MimeType: driveFolderMimeType, Parents: []string{parentID}}
	created, err := a.service.Files.Create(folder).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create drive folder %q: %w", name, err)
	}
	return created.Id, nil
}

func (a *realDriveFilesAPI) uploadFile(ctx context.Context, name, parentID string, r io.Reader) (string, int64, error) {
	file := &drive.File{Name: name, Parents: []string{parentID}}
	created, err := a.service.Files.Create(file).Media(r).Context(ctx).Do()
	if err != nil {
		return "", 0, fmt.Errorf("upload drive file %q: %w", name, err)
	}
	return created.Id, created.Size, nil
}

func (a *realDriveFilesAPI) downloadFile(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := a.service.Files.Get(id).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("download drive file %q: %w", id, err)
	}
	return resp.Body, nil
}

func (a *realDriveFilesAPI) deleteFile(ctx context.Context, id string) error {
	if err := a.service.Files.Delete(id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete drive file %q: %w", id, err)
	}
	return nil
}

// GoogleDriveStorage implements Storage backed by Google Drive, using a
// Service Account for authentication. Folder IDs are cached (via
// folderCache) so repeated uploads to the same folderPath don't re-query
// the Drive API on every call.
type GoogleDriveStorage struct {
	api          driveFilesAPI
	cache        folderCache
	rootFolderID string
	// key -> Drive file ID, populated by Put, consumed by Open/Delete.
	//
	// KNOWN GAP (accepted for this task): this map is in-memory only, so it
	// only remembers file IDs uploaded during the current process's
	// lifetime. Open/Delete for a key uploaded in a different process (e.g.
	// after a server restart) will fail to resolve. See task-4-report.md /
	// the media-storage-backend plan for the required follow-up: the caller
	// must persist the real Drive file ID and the Storage interface likely
	// needs to grow a way to report it back from Put.
	fileIDs map[string]string
}

func NewGoogleDriveStorage(ctx context.Context, credentialsPath, rootFolderID string, cache folderCache) (*GoogleDriveStorage, error) {
	service, err := drive.NewService(ctx, option.WithCredentialsFile(credentialsPath))
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}
	return &GoogleDriveStorage{
		api:          &realDriveFilesAPI{service: service},
		cache:        cache,
		rootFolderID: rootFolderID,
		fileIDs:      map[string]string{},
	}, nil
}

func (s *GoogleDriveStorage) resolveFolder(ctx context.Context, folderPath []string) (string, error) {
	parentID := s.rootFolderID
	pathKeyParts := make([]string, 0, len(folderPath))
	for _, name := range folderPath {
		pathKeyParts = append(pathKeyParts, strings.ToLower(strings.ReplaceAll(name, " ", "-")))
		pathKey := strings.Join(pathKeyParts, "/")

		if cached, ok, err := s.cache.Get(ctx, pathKey); err != nil {
			return "", err
		} else if ok {
			parentID = cached
			continue
		}

		found, err := s.api.findFolder(ctx, name, parentID)
		if err != nil {
			return "", err
		}
		if found == "" {
			found, err = s.api.createFolder(ctx, name, parentID)
			if err != nil {
				return "", err
			}
		}
		if err := s.cache.Set(ctx, pathKey, found); err != nil {
			return "", err
		}
		parentID = found
	}
	return parentID, nil
}

func (s *GoogleDriveStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (int64, string, error) {
	folderID, err := s.resolveFolder(ctx, folderPath)
	if err != nil {
		return 0, "", fmt.Errorf("resolve drive folder: %w", err)
	}
	hashing := newHashingReader(source)
	fileID, size, err := s.api.uploadFile(ctx, key, folderID, hashing)
	if err != nil {
		return 0, "", err
	}
	if s.fileIDs == nil {
		s.fileIDs = map[string]string{}
	}
	s.fileIDs[key] = fileID
	return size, hashing.checksum(), nil
}

func (s *GoogleDriveStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	fileID, ok := s.fileIDs[key]
	if !ok {
		return nil, fmt.Errorf("open drive file: unknown key %q", key)
	}
	return s.api.downloadFile(ctx, fileID)
}

func (s *GoogleDriveStorage) Delete(ctx context.Context, key string) error {
	fileID, ok := s.fileIDs[key]
	if !ok {
		return nil
	}
	if err := s.api.deleteFile(ctx, fileID); err != nil {
		return err
	}
	delete(s.fileIDs, key)
	return nil
}

// hashingReader wraps an io.Reader and computes a SHA-256 checksum of
// everything read through it, mirroring what LocalStorage.Put does with
// io.MultiWriter.
type hashingReader struct {
	source io.Reader
	hasher interface {
		io.Writer
		Sum([]byte) []byte
	}
}

func newHashingReader(source io.Reader) *hashingReader {
	h := sha256.New()
	return &hashingReader{source: io.TeeReader(source, h), hasher: h}
}

func (r *hashingReader) Read(p []byte) (int, error) { return r.source.Read(p) }
func (r *hashingReader) checksum() string           { return hex.EncodeToString(r.hasher.Sum(nil)) }
