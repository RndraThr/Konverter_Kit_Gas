# Media Storage Backend (Local + Google Drive) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `internal/media.Storage` folder-aware and add a Google Drive-backed implementation, selectable via config, without changing the behavior of the existing local-disk-backed Pendistribusian photo uploads.

**Architecture:** Extend `Storage.Put` with a `folderPath []string` parameter (list of folder names from a logical root). `LocalStorage` ignores it (stays flat, zero behavior change for existing callers). A new `GoogleDriveStorage` resolves/creates the nested Drive folder chain (cached in a new `drive_folder_cache` table to avoid repeated Drive API calls) and uploads into it. Config gains a `STORAGE_BACKEND` switch (`local` default, `gdrive`) plus Drive credentials/root-folder settings; `cmd/server/main.go` picks the concrete implementation at startup.

**Tech Stack:** Go 1.26, PostgreSQL (pgx/v5), `google.golang.org/api/drive/v3` + `golang.org/x/oauth2/google` (new dependencies), goose migrations.

**Spec:** `docs/superpowers/specs/2026-09-16-dokumentasi-kegiatan-gdrive-design.md` (sections 4.1–4.4 specifically — this plan implements only the storage-infrastructure portion of that spec; the `activity_media` feature itself is a separate, later plan).

## Global Constraints

- `LocalStorage`'s on-disk behavior must not change for existing callers — `folderPath` is accepted but ignored.
- The one production caller of `Storage.Put` (`internal/distribution/service.go:185`) must keep passing a value that preserves today's flat layout — pass `nil` for `folderPath`.
- Google Drive authentication is via a Service Account JSON credentials file, read from `GDRIVE_SERVICE_ACCOUNT_JSON` (a file path). No OAuth user-consent flow.
- Drive files are never made public/shared-by-link; content is always read back through `GoogleDriveStorage.Open`, never a direct Drive URL.
- `go test ./... -count=1`, `go vet ./...` must stay green after every task. A live Postgres test database is available at `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable` (must be named exactly `konkit_test`) for integration tests — do not accept a skipped integration test if this is reachable; run it for real.
- No task in this plan requires real Google Drive credentials to pass its tests — `GoogleDriveStorage`'s own unit tests fake the Drive API via a narrow interface. Real end-to-end verification against actual Drive happens manually, later, once the user has created a Service Account and shared a root folder with it (out of scope for this plan's automated tests).

---

### Task 1: Migration — `drive_folder_cache` table

**Files:**
- Create: `internal/database/migrations/00008_media_storage_backend.sql`
- Test: `internal/media/schema_integration_test.go`

**Interfaces:**
- Produces: table `drive_folder_cache(id uuid, path_key text UNIQUE, drive_folder_id text, created_at timestamptz)` — consumed by Task 3's cache repository.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE drive_folder_cache (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    path_key text NOT NULL UNIQUE,
    drive_folder_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE drive_folder_cache;
```

- [ ] **Step 2: Write the failing schema test**

Create `internal/media/schema_integration_test.go`:

```go
package media

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func mediaIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require konkit_test, got %q", config.ConnConfig.Database)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrationCreatesDriveFolderCacheTable(t *testing.T) {
	pool := mediaIntegrationPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ('wajo/dokumentasi-foto-video/rakor', 'fake-drive-id-1')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor'`); err != nil {
			t.Logf("cleanup: delete drive_folder_cache failed: %v", err)
		}
	})

	var driveFolderID string
	if err := pool.QueryRow(ctx, `SELECT drive_folder_id FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor'`).Scan(&driveFolderID); err != nil {
		t.Fatal(err)
	}
	if driveFolderID != "fake-drive-id-1" {
		t.Fatalf("drive_folder_id = %q", driveFolderID)
	}

	// UNIQUE(path_key) enforced.
	if _, err := pool.Exec(ctx, `INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ('wajo/dokumentasi-foto-video/rakor', 'fake-drive-id-2')`); err == nil {
		t.Fatal("expected unique violation on duplicate path_key")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -run TestMigrationCreatesDriveFolderCacheTable -v`
Expected: FAIL — `relation "drive_folder_cache" does not exist` (migration file doesn't exist yet, so goose has nothing to apply for it).

- [ ] **Step 4: Confirm the migration exists (Step 1), then run for real**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/media/... -run TestMigrationCreatesDriveFolderCacheTable -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/00008_media_storage_backend.sql internal/media/schema_integration_test.go
git commit -m "feat(media): add drive_folder_cache table migration"
```

---

### Task 2: Extend `Storage` interface with `folderPath`, fix all call sites

**Files:**
- Modify: `internal/media/storage.go`
- Modify: `internal/media/storage_test.go`
- Modify: `internal/distribution/service.go:185`
- Modify: `internal/distribution/service_test.go` (the `storageStub` fake, lines 1-31 per research)

**Interfaces:**
- Produces: `Storage.Put(ctx context.Context, key string, folderPath []string, source io.Reader) (int64, string, error)` — this is the new signature every later task (3, 4, and the future activities plan) must match.
- Consumes: nothing from earlier tasks.

This task touches the interface used by the already-shipped Pendistribusian upload flow. Change the signature, update every implementer and every caller in the same commit, and prove nothing broke.

- [ ] **Step 1: Update the interface and `LocalStorage.Put`**

In `internal/media/storage.go`, change:

```go
type Storage interface {
	Put(context.Context, string, io.Reader) (int64, string, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
```

to:

```go
type Storage interface {
	// Put stores source under key. folderPath is a hint for backends that
	// organize content into folders (e.g. Google Drive); LocalStorage
	// ignores it and always stores flat.
	Put(ctx context.Context, key string, folderPath []string, source io.Reader) (int64, string, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
```

Change the `LocalStorage.Put` signature:

```go
func (s *LocalStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (size int64, checksum string, resultErr error) {
```

(`folderPath` is accepted but never referenced in the body — the rest of the function is unchanged. Go does not require you to use a parameter, so this compiles as-is; do not add a `_ = folderPath` line, it's unnecessary noise.)

- [ ] **Step 2: Update `internal/media/storage_test.go`'s three `Put` call sites**

Each call gains `nil` as the third argument:

```go
if _, _, err := storage.Put(context.Background(), key, nil, bytes.NewBufferString("secret")); !errors.Is(err, ErrInvalidKey) {
```

```go
size, checksum, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440000", nil, bytes.NewBufferString("photo"))
```

```go
if _, _, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440001", nil, &failingReader{}); err == nil {
```

- [ ] **Step 3: Run the media package tests to confirm the signature change alone compiles and passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -v`
Expected: PASS (all `TestLocalStorage*` tests, plus Task 1's `TestMigrationCreatesDriveFolderCacheTable` if `TEST_DATABASE_URL` is set).

- [ ] **Step 4: Fix the production caller**

In `internal/distribution/service.go`, line 185, change:

```go
	size, checksum, err := s.storage.Put(ctx, key, bytes.NewReader(input.Data))
```

to:

```go
	size, checksum, err := s.storage.Put(ctx, key, nil, bytes.NewReader(input.Data))
```

- [ ] **Step 5: Fix the test fake**

In `internal/distribution/service_test.go`, change the `storageStub.Put` method:

```go
func (s *storageStub) Put(_ context.Context, key string, source io.Reader) (int64, string, error) {
	s.putKey = key
	s.content, _ = io.ReadAll(source)
	return int64(len(s.content)), "checksum", s.putErr
}
```

to:

```go
func (s *storageStub) Put(_ context.Context, key string, _ []string, source io.Reader) (int64, string, error) {
	s.putKey = key
	s.content, _ = io.ReadAll(source)
	return int64(len(s.content)), "checksum", s.putErr
}
```

- [ ] **Step 6: Run the full distribution package tests**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/distribution/... -v -count=2`
Expected: PASS, both iterations — this is the existing, shipped Pendistribusian test suite; it must be 100% unaffected by this change (same assertions, same behavior, only the call signature changed).

- [ ] **Step 7: Run the whole module to catch any other reference you missed**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean. A build failure here means there's a fourth call site (e.g. in a `.claude/worktrees/...` copy — ignore those, they're separate checkouts; but if `go build ./...` from the repo root finds one inside `internal/` or `cmd/`, fix it the same way as Step 4/5).

- [ ] **Step 8: Commit**

```bash
git add internal/media/storage.go internal/media/storage_test.go internal/distribution/service.go internal/distribution/service_test.go
git commit -m "feat(media): make Storage.Put folder-aware, update all callers"
```

---

### Task 3: Drive folder-ID cache repository

**Files:**
- Create: `internal/media/gdrive_cache.go`
- Test: `internal/media/gdrive_cache_integration_test.go`

**Interfaces:**
- Consumes: `drive_folder_cache` table from Task 1.
- Produces: `folderCache` interface and `*postgresFolderCache` — consumed by Task 4's `GoogleDriveStorage`.
  ```go
  type folderCache interface {
      Get(ctx context.Context, pathKey string) (driveFolderID string, ok bool, err error)
      Set(ctx context.Context, pathKey, driveFolderID string) error
  }
  func newPostgresFolderCache(pool *pgxpool.Pool) *postgresFolderCache
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/media/gdrive_cache_integration_test.go`:

```go
package media

import (
	"context"
	"testing"
)

func TestPostgresFolderCacheGetSetRoundTrip(t *testing.T) {
	pool := mediaIntegrationPool(t)
	cache := newPostgresFolderCache(pool)
	ctx := context.Background()

	_, ok, err := cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected cache miss before Set")
	}

	if err := cache.Set(ctx, "wajo/dokumentasi-foto-video/rakor-test", "drive-folder-abc"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor-test'`); err != nil {
			t.Logf("cleanup: delete drive_folder_cache failed: %v", err)
		}
	})

	id, ok, err := cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || id != "drive-folder-abc" {
		t.Fatalf("id=%q ok=%v", id, ok)
	}

	// Set again with a different ID must overwrite (idempotent upsert), not error.
	if err := cache.Set(ctx, "wajo/dokumentasi-foto-video/rakor-test", "drive-folder-xyz"); err != nil {
		t.Fatal(err)
	}
	id, ok, err = cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || id != "drive-folder-xyz" {
		t.Fatalf("after overwrite: id=%q ok=%v", id, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/media/... -run TestPostgresFolderCacheGetSetRoundTrip -v`
Expected: FAIL — `undefined: newPostgresFolderCache`.

- [ ] **Step 3: Implement**

Create `internal/media/gdrive_cache.go`:

```go
package media

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type folderCache interface {
	Get(ctx context.Context, pathKey string) (driveFolderID string, ok bool, err error)
	Set(ctx context.Context, pathKey, driveFolderID string) error
}

type postgresFolderCache struct {
	pool *pgxpool.Pool
}

func newPostgresFolderCache(pool *pgxpool.Pool) *postgresFolderCache {
	return &postgresFolderCache{pool: pool}
}

func (c *postgresFolderCache) Get(ctx context.Context, pathKey string) (string, bool, error) {
	var driveFolderID string
	err := c.pool.QueryRow(ctx, `SELECT drive_folder_id FROM drive_folder_cache WHERE path_key = $1`, pathKey).Scan(&driveFolderID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get folder cache entry: %w", err)
	}
	return driveFolderID, true, nil
}

func (c *postgresFolderCache) Set(ctx context.Context, pathKey, driveFolderID string) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ($1, $2)
		ON CONFLICT (path_key) DO UPDATE SET drive_folder_id = EXCLUDED.drive_folder_id
	`, pathKey, driveFolderID)
	if err != nil {
		return fmt.Errorf("set folder cache entry: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/media/... -run TestPostgresFolderCacheGetSetRoundTrip -v -count=2`
Expected: PASS, both iterations.

- [ ] **Step 5: Commit**

```bash
git add internal/media/gdrive_cache.go internal/media/gdrive_cache_integration_test.go
git commit -m "feat(media): add Postgres-backed Drive folder-ID cache"
```

---

### Task 4: `GoogleDriveStorage` implementation

**Files:**
- Create: `internal/media/gdrive.go`
- Test: `internal/media/gdrive_test.go`
- Modify: `go.mod`, `go.sum` (new dependencies)

**Interfaces:**
- Consumes: `Storage` interface (Task 2), `folderCache` (Task 3).
- Produces:
  ```go
  func NewGoogleDriveStorage(ctx context.Context, credentialsPath, rootFolderID string, cache folderCache) (*GoogleDriveStorage, error)
  ```
  satisfying `media.Storage` — consumed by Task 6 (`cmd/server/main.go` wiring).

**Design:** `GoogleDriveStorage` depends on a narrow `driveFilesAPI` interface (not the raw `*drive.Service`) so tests can fake Drive entirely — no network calls in this task's tests.

- [ ] **Step 1: Add the real Drive API dependencies**

Run:
```bash
cd "d:/KSM/Deployment/konkit"
go get google.golang.org/api/drive/v3
go get golang.org/x/oauth2/google
go mod tidy
```

Then verify the exact method surface this plan assumes actually matches what got installed — the Drive v3 client's shape has been stable for years, but confirm before writing code that depends on it:

```bash
go doc google.golang.org/api/drive/v3.FilesService.List
go doc google.golang.org/api/drive/v3.FilesService.Create
go doc google.golang.org/api/drive/v3.FilesService.Get
go doc google.golang.org/api/drive/v3.FilesService.Delete
go doc google.golang.org/api/drive/v3.File
```

If any signature below differs from what `go doc` shows (method names, return types), adjust Step 3's code to match the real installed library — the interface this task exposes to the rest of the codebase (`driveFilesAPI`, `GoogleDriveStorage`) is what matters; the internal call shapes against `drive.Service` can flex to match reality.

- [ ] **Step 2: Write the failing test (fakes the Drive API entirely)**

Create `internal/media/gdrive_test.go`:

```go
package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type fakeDriveFilesAPI struct {
	foldersByParentAndName map[string]string // key: parentID+"/"+name -> folder ID
	createdFolders         []string          // names, in creation order
	uploaded               map[string][]byte // key: file ID -> content
	nextID                 int
	deleteErr              error
	deletedIDs             []string
}

func newFakeDriveFilesAPI() *fakeDriveFilesAPI {
	return &fakeDriveFilesAPI{
		foldersByParentAndName: map[string]string{},
		uploaded:               map[string][]byte{},
	}
}

func (f *fakeDriveFilesAPI) newID() string {
	f.nextID++
	return "fake-id-" + string(rune('a'+f.nextID))
}

func (f *fakeDriveFilesAPI) findFolder(_ context.Context, name, parentID string) (string, error) {
	id, ok := f.foldersByParentAndName[parentID+"/"+name]
	if !ok {
		return "", nil
	}
	return id, nil
}

func (f *fakeDriveFilesAPI) createFolder(_ context.Context, name, parentID string) (string, error) {
	id := f.newID()
	f.foldersByParentAndName[parentID+"/"+name] = id
	f.createdFolders = append(f.createdFolders, name)
	return id, nil
}

func (f *fakeDriveFilesAPI) uploadFile(_ context.Context, _ string, _ string, r io.Reader) (string, int64, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	id := f.newID()
	f.uploaded[id] = content
	return id, int64(len(content)), nil
}

func (f *fakeDriveFilesAPI) downloadFile(_ context.Context, id string) (io.ReadCloser, error) {
	content, ok := f.uploaded[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (f *fakeDriveFilesAPI) deleteFile(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedIDs = append(f.deletedIDs, id)
	delete(f.uploaded, id)
	return nil
}

type fakeFolderCache struct {
	entries map[string]string
	sets    int
}

func newFakeFolderCache() *fakeFolderCache {
	return &fakeFolderCache{entries: map[string]string{}}
}

func (c *fakeFolderCache) Get(_ context.Context, pathKey string) (string, bool, error) {
	id, ok := c.entries[pathKey]
	return id, ok, nil
}

func (c *fakeFolderCache) Set(_ context.Context, pathKey, driveFolderID string) error {
	c.entries[pathKey] = driveFolderID
	c.sets++
	return nil
}

func TestGoogleDriveStoragePutCreatesNestedFoldersAndCachesThem(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	size, checksum, err := storage.Put(context.Background(), "file-key-1", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("photo bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len("photo bytes")) || checksum == "" {
		t.Fatalf("size=%d checksum=%q", size, checksum)
	}
	if len(api.createdFolders) != 4 {
		t.Fatalf("expected 4 folders created, got %v", api.createdFolders)
	}
	if cache.sets != 4 {
		t.Fatalf("expected 4 cache writes (one per folder level), got %d", cache.sets)
	}

	// Second Put with the SAME folderPath must reuse cached folder IDs, not create new ones.
	if _, _, err := storage.Put(context.Background(), "file-key-2", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("more bytes")); err != nil {
		t.Fatal(err)
	}
	if len(api.createdFolders) != 4 {
		t.Fatalf("expected still 4 folders created after reusing cached path, got %v", api.createdFolders)
	}
}

func TestGoogleDriveStorageOpenAndDelete(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	_, _, err := storage.Put(context.Background(), "file-key-3", []string{"Konkit 2026", "Wajo"}, bytes.NewBufferString("hello drive"))
	if err != nil {
		t.Fatal(err)
	}

	reader, err := storage.Open(context.Background(), "file-key-3")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "hello drive" {
		t.Fatalf("content=%q", content)
	}

	if err := storage.Delete(context.Background(), "file-key-3"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Open(context.Background(), "file-key-3"); err == nil {
		t.Fatal("expected Open after Delete to fail")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -run TestGoogleDriveStorage -v`
Expected: FAIL — `undefined: GoogleDriveStorage`.

- [ ] **Step 4: Implement**

Create `internal/media/gdrive.go`. This defines the narrow `driveFilesAPI` interface, a real implementation wrapping `*drive.Service` (`realDriveFilesAPI`), and `GoogleDriveStorage` itself (which depends only on the interface, matching the test's fake):

```go
package media

import (
	"context"
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
```

You'll also need a small `hashingReader` helper (SHA-256, mirroring what `LocalStorage.Put` does with `io.MultiWriter`) — add this to `gdrive.go` too:

```go
import "crypto/sha256"
import "encoding/hex"

type hashingReader struct {
	source io.Reader
	hash   [32]byte
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
func (r *hashingReader) checksum() string            { return hex.EncodeToString(r.hasher.Sum(nil)) }
```

(Merge the two import blocks in the real file into one `import (...)` block — they're split here only for readability in this plan.)

**⚠️ Known gap, acceptable for this task:** `GoogleDriveStorage.fileIDs` is an in-memory `map[string]string` — it only remembers file IDs uploaded during the current process lifetime. This works for `Put` immediately followed by `Open`/`Delete` in the same request (which is how `internal/distribution` and the future `internal/activities` use `Storage` — see the `Put` → persist metadata → later separate `Open`/`Delete` calls pattern in `distribution/service.go`), but a real `Open`/`Delete` call in a **different process** (e.g. after a server restart) would fail to find the file ID for a `key` it doesn't remember. **This must be fixed before this backend is usable in production** — the caller (the future `internal/activities` repository) needs to persist the Drive file ID itself (e.g. as `activity_media.storage_key` = the Drive file ID directly, not a locally-generated UUID) and pass that same value as `key` on every `Open`/`Delete` call, and `GoogleDriveStorage.Open`/`Delete` should treat `key` as *being* the Drive file ID directly rather than looking it up in `fileIDs`. Flag this explicitly in this task's commit message and self-review; the later activities plan must account for it (`storage_key` stores the real Drive file ID that `Put` returns — but `Put`'s current signature returns `(size, checksum, error)`, not the file ID, so **the `Storage` interface has no way to report the Drive file ID back to the caller today**. Do not silently work around this — report it as a concern in this task's report so the controller can decide whether to extend `Storage.Put`'s return signature (e.g. add a returned key/ID) as part of this task or explicitly defer it with a tracked follow-up.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -run TestGoogleDriveStorage -v`
Expected: PASS.

- [ ] **Step 6: Run the whole module**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./... && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./... -count=1 -p 1`
Expected: all clean/green.

- [ ] **Step 7: Commit**

```bash
git add internal/media/gdrive.go internal/media/gdrive_test.go go.mod go.sum
git commit -m "feat(media): add GoogleDriveStorage implementation"
```

---

### Task 5: Config — `STORAGE_BACKEND` and Drive settings

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (check whether this file already exists before writing — if it does, add test functions to it following its existing style; if not, this task's implementer must create it following the `loadFrom(lookupFunc)` testing seam already used by `Load`)

**Interfaces:**
- Produces: `Config.StorageBackend string`, `Config.GDriveServiceAccountJSON string`, `Config.GDriveRootFolderID string` — consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

Add to (or create) `internal/config/config_test.go` — if the file already exists, match its existing test style; otherwise use this minimal form:

```go
package config

import "testing"

func TestLoadFromDefaultsStorageBackendToLocal(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"DATABASE_URL":   "postgres://u:p@localhost/db",
			"SESSION_SECRET": "01234567890123456789012345678901",
		}
		v, ok := values[key]
		return v, ok
	}
	cfg, err := loadFrom(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorageBackend != "local" {
		t.Fatalf("StorageBackend = %q", cfg.StorageBackend)
	}
}

func TestLoadFromRequiresDriveSettingsWhenBackendIsGDrive(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":   "postgres://u:p@localhost/db",
		"SESSION_SECRET": "01234567890123456789012345678901",
		"STORAGE_BACKEND": "gdrive",
	}
	lookup := func(key string) (string, bool) { v, ok := base[key]; return v, ok }

	if _, err := loadFrom(lookup); err == nil {
		t.Fatal("expected error when gdrive backend configured without credentials/root folder")
	}

	base["GDRIVE_SERVICE_ACCOUNT_JSON"] = "/etc/konkit/gdrive-credentials.json"
	base["GDRIVE_ROOT_FOLDER_ID"] = "1AbCdEfGhIjKlMnOpQrStUvWxYz"
	cfg, err := loadFrom(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorageBackend != "gdrive" || cfg.GDriveServiceAccountJSON != "/etc/konkit/gdrive-credentials.json" || cfg.GDriveRootFolderID != "1AbCdEfGhIjKlMnOpQrStUvWxYz" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadFromRejectsUnknownStorageBackend(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":    "postgres://u:p@localhost/db",
		"SESSION_SECRET":  "01234567890123456789012345678901",
		"STORAGE_BACKEND": "s3",
	}
	lookup := func(key string) (string, bool) { v, ok := base[key]; return v, ok }
	if _, err := loadFrom(lookup); err == nil {
		t.Fatal("expected error for unknown STORAGE_BACKEND value")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/config/... -v`
Expected: FAIL — `cfg.StorageBackend` doesn't exist (compile error) or defaults differ.

- [ ] **Step 3: Implement**

In `internal/config/config.go`:

Add to the `var (...)` error block:
```go
	ErrStorageBackendInvalid     = errors.New("STORAGE_BACKEND must be 'local' or 'gdrive'")
	ErrGDriveSettingsIncomplete  = errors.New("GDRIVE_SERVICE_ACCOUNT_JSON and GDRIVE_ROOT_FOLDER_ID are required when STORAGE_BACKEND=gdrive")
```

Add fields to `Config`:
```go
	StorageBackend           string
	GDriveServiceAccountJSON string
	GDriveRootFolderID       string
```

In `loadFrom`, after the existing `cfg := Config{...}` block sets `StoragePath`, add `StorageBackend: valueOrDefault(lookup, "STORAGE_BACKEND", "local"),` as one more field in that same struct literal.

After the existing `ErrStoragePathAbsolute` check (and before the `SESSION_SECRET` block), add:

```go
	if cfg.StorageBackend != "local" && cfg.StorageBackend != "gdrive" {
		return Config{}, ErrStorageBackendInvalid
	}
	if cfg.StorageBackend == "gdrive" {
		cfg.GDriveServiceAccountJSON, _ = lookup("GDRIVE_SERVICE_ACCOUNT_JSON")
		cfg.GDriveRootFolderID, _ = lookup("GDRIVE_ROOT_FOLDER_ID")
		if cfg.GDriveServiceAccountJSON == "" || cfg.GDriveRootFolderID == "" {
			return Config{}, ErrGDriveSettingsIncomplete
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/config/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add STORAGE_BACKEND and Google Drive settings"
```

---

### Task 6: Wire backend selection into `cmd/server/main.go`

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`

**Interfaces:**
- Consumes: `Config.StorageBackend`/`GDriveServiceAccountJSON`/`GDriveRootFolderID` (Task 5), `media.NewGoogleDriveStorage` (Task 4), `media.NewLocalStorage` (existing).

- [ ] **Step 1: Write the failing test**

`cmd/server/main_test.go` currently contains exactly this (verbatim, as of this plan being written — confirm it still matches before editing; if it has drifted, adapt but keep the same `run(ctx, cfg)`-calling pattern):

```go
package main

import (
	"context"
	"testing"

	"konkit/internal/config"
)

func TestRunReturnsDatabaseConfigurationError(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:   "://invalid",
		SessionSecret: []byte("01234567890123456789012345678901"),
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected invalid database URL error")
	}
}
```

`run()` opens (and pings) the database *before* constructing media storage — so testing the `gdrive` branch specifically requires a real, reachable `DATABASE_URL` to get past that point; this makes it an integration-style test gated on `TEST_DATABASE_URL`, same as elsewhere in this codebase. Add:

```go
func TestRunFailsFastWhenGoogleDriveCredentialsAreInvalid(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	cfg := config.Config{
		DatabaseURL:              databaseURL,
		SessionSecret:            []byte("01234567890123456789012345678901"),
		StorageBackend:           "gdrive",
		GDriveServiceAccountJSON: "/nonexistent/path/credentials.json",
		GDriveRootFolderID:       "irrelevant-for-this-test",
	}
	err := run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error when Google Drive credentials file does not exist")
	}
}
```

(Add `"os"` to the file's import block.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./cmd/server/... -run TestRunFailsFastWhenGoogleDriveCredentialsAreInvalid -v -timeout 15s`
Expected: FAIL — before Step 3's implementation, `main.go` doesn't branch on `StorageBackend` yet, so a `gdrive` config still silently uses `NewLocalStorage` and `run()` proceeds to start a real HTTP server and block waiting for a shutdown signal instead of returning an error. This means the test **times out** rather than failing with a clean assertion message — that timeout (govern by `-timeout 15s` so it fails fast instead of waiting the default 10 minutes) is the expected RED-phase signal here; proceed to Step 3 once you've seen it.

- [ ] **Step 3: Export the folder cache constructor**

Task 3 created `newPostgresFolderCache` (unexported, package-private, since it was only consumed inside `internal/media` by Task 4's tests). `cmd/server/main.go` is in a different package (`main`), so it needs an exported constructor. In `internal/media/gdrive_cache.go`, rename `newPostgresFolderCache` to `NewPostgresFolderCache` (exported), and update its constructor call in `internal/media/gdrive_cache_integration_test.go` (Task 3's test) to match. `internal/media/gdrive_test.go` (Task 4's test) constructs `GoogleDriveStorage` directly with a `fakeFolderCache`, not via this constructor, so it needs no change.

- [ ] **Step 4: Implement the branch**

In `cmd/server/main.go`, replace:

```go
	mediaStorage, err := media.NewLocalStorage(cfg.StoragePath)
	if err != nil {
		return err
	}
```

with:

```go
	var mediaStorage media.Storage
	switch cfg.StorageBackend {
	case "gdrive":
		driveCache := media.NewPostgresFolderCache(pool)
		mediaStorage, err = media.NewGoogleDriveStorage(ctx, cfg.GDriveServiceAccountJSON, cfg.GDriveRootFolderID, driveCache)
		if err != nil {
			return fmt.Errorf("initialize google drive storage: %w", err)
		}
	default:
		mediaStorage, err = media.NewLocalStorage(cfg.StoragePath)
		if err != nil {
			return err
		}
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./cmd/server/... -v`
Expected: PASS (both `TestRunReturnsDatabaseConfigurationError` and the new `TestRunFailsFastWhenGoogleDriveCredentialsAreInvalid`).

- [ ] **Step 6: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./... -count=1 -p 1`
Expected: all packages pass.

- [ ] **Step 7: Manual smoke test (local backend, default — no Drive credentials needed)**

Run: `cd "d:/KSM/Deployment/konkit" && go run ./cmd/server` — confirm it starts without error (default `STORAGE_BACKEND=local` per Task 5, so this must behave exactly as it did before this whole plan). Stop it with Ctrl+C once you see the "listening on" log line.

- [ ] **Step 8: Commit**

```bash
git add cmd/server/main.go internal/media/gdrive_cache.go internal/media/gdrive_cache_integration_test.go cmd/server/main_test.go
git commit -m "feat(server): select media storage backend from STORAGE_BACKEND config"
```

---

## After This Plan

Two things are true once this plan is merged:

1. Pendistribusian's photo uploads work exactly as before (local disk, verified by the existing `internal/distribution` test suite passing unchanged).
2. `STORAGE_BACKEND=gdrive` is available but **not yet usable end-to-end** — Task 4's known gap (`GoogleDriveStorage.fileIDs` is in-memory only, and `Storage.Put`'s return signature has no way to report the Drive file ID back to a caller) must be resolved by the plan that actually builds the `activity_media` feature, since that plan controls what `storage_key` means and how `Open`/`Delete` are invoked. Carry this forward explicitly into that plan's design — do not let it get silently dropped.

Real end-to-end testing against actual Google Drive requires the user to: create a Google Cloud project, enable the Drive API, create a Service Account, download its credentials JSON, and share the intended root Drive folder with the Service Account's email (Editor access) — then set `STORAGE_BACKEND=gdrive`, `GDRIVE_SERVICE_ACCOUNT_JSON`, `GDRIVE_ROOT_FOLDER_ID` in `.env`. This is a manual, out-of-band step; nothing in this plan's automated tests depends on it.
