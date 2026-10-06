package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
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
const driveUploadChunkSize = 8 << 20

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
	created, err := a.service.Files.Create(file).Media(r, googleapi.ChunkSize(driveUploadChunkSize)).Context(ctx).Do()
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

// GoogleDriveStorage implements Storage backed by Google Drive, using
// OAuth2 user-delegated authorization (not a Service Account — Service
// Accounts have no storage quota of their own on a personal Google account
// and cannot upload files anywhere, even to a folder shared with them).
// Folder IDs are cached (via folderCache) so repeated uploads to the same
// folderPath don't re-query the Drive API on every call.
type GoogleDriveStorage struct {
	api          driveFilesAPI
	cache        folderCache
	rootFolderID string
}

// NewGoogleDriveStorage authenticates using a previously-issued OAuth2
// token (see cmd/gdrive-oauth-setup for how to obtain one). The oauth2
// TokenSource built from clientID/clientSecret/token refreshes the access
// token automatically using the token's refresh token — no further human
// interaction is needed at runtime.
func NewGoogleDriveStorage(ctx context.Context, clientID, clientSecret, tokenJSONPath, rootFolderID string, cache folderCache) (*GoogleDriveStorage, error) {
	tokenBytes, err := os.ReadFile(tokenJSONPath)
	if err != nil {
		return nil, fmt.Errorf("read oauth token file: %w", err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("parse oauth token file: %w", err)
	}
	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
	}
	service, err := drive.NewService(ctx, option.WithTokenSource(oauthConfig.TokenSource(ctx, &token)))
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}
	return &GoogleDriveStorage{
		api:          &realDriveFilesAPI{service: service},
		cache:        cache,
		rootFolderID: rootFolderID,
	}, nil
}

// resolveFolder resolves (creating as needed) the Drive folder identified
// by folderPath, idempotently. It makes no assumption about what any given
// path index "means" (e.g. it does not treat any particular index as a
// regency folder) — callers that need sibling folders to exist ahead of
// time must request them explicitly via EnsureFolders.
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

// EnsureFolders resolves (creating as needed) each folder path in paths,
// idempotently, without uploading any file. Used to pre-create folder
// hierarchies — e.g. the reserved document-category folders for a new
// program zone — ahead of any upload.
func (s *GoogleDriveStorage) EnsureFolders(ctx context.Context, paths [][]string) error {
	for _, folderPath := range paths {
		if len(folderPath) == 0 {
			return ErrInvalidFolderPath
		}
		if _, err := s.resolveFolder(ctx, folderPath); err != nil {
			return fmt.Errorf("ensure drive folder %v: %w", folderPath, err)
		}
	}
	return nil
}

func (s *GoogleDriveStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	return s.PutNamed(ctx, key, key, folderPath, source)
}

func (s *GoogleDriveStorage) PutNamed(ctx context.Context, key, filename string, folderPath []string, source io.Reader) (string, int64, string, error) {
	folderID, err := s.resolveFolder(ctx, folderPath)
	if err != nil {
		return "", 0, "", fmt.Errorf("resolve drive folder: %w", err)
	}
	hashing := newHashingReader(source)
	fileID, _, err := s.api.uploadFile(ctx, filename, folderID, hashing)
	if err != nil {
		return "", 0, "", err
	}
	return fileID, hashing.size(), hashing.checksum(), nil
}

// Open and Delete take the real Drive file ID (the storageKey returned by
// Put) directly, so they need no in-memory bookkeeping and work correctly
// even after a server restart.
func (s *GoogleDriveStorage) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	return s.api.downloadFile(ctx, storageKey)
}

func (s *GoogleDriveStorage) Delete(ctx context.Context, storageKey string) error {
	return s.api.deleteFile(ctx, storageKey)
}

// hashingReader wraps an io.Reader and computes a SHA-256 checksum of
// everything read through it, mirroring what LocalStorage.Put does with
// io.MultiWriter.
type hashingReader struct {
	source io.Reader
	bytes  int64
	hasher interface {
		io.Writer
		Sum([]byte) []byte
	}
}

func newHashingReader(source io.Reader) *hashingReader {
	h := sha256.New()
	return &hashingReader{source: io.TeeReader(source, h), hasher: h}
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	r.bytes += int64(n)
	return n, err
}
func (r *hashingReader) size() int64      { return r.bytes }
func (r *hashingReader) checksum() string { return hex.EncodeToString(r.hasher.Sum(nil)) }
