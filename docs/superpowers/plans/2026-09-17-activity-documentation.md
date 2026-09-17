# Dokumentasi Kegiatan Lapangan (Activity Documentation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 10 field-activity photo/video documentation modules (Ceremony & Sosialisasi, Pelatihan Teknis, Rakor, Training 10%, Training 100%, Unloading Konkit/Mesin Pompa/Oli/Selang Hisap & Buang/Tabung Gas), backed by a new `internal/activities` package and a generic frontend gallery page, and move "Laporan" out of the sidebar's "Dokumentasi" group.

**Note on scope count:** the spec's title says "9 Modul" but its own activity-type list (section 1) and `activity_media.activity_type` CHECK constraint (section 3) enumerate exactly **10** concrete types. This plan builds all 10 listed types — the CHECK constraint is the authoritative, exact-values source; the title's "9" is a stale count from earlier discussion.

**Architecture:** `internal/activities` mirrors the just-shipped `internal/recipients` package (models/repository/service, RegencyScope-scoped SQL, tx+audit pattern). Media storage reuses the pluggable `media.Storage` interface from the media-storage-backend plan. Before building the feature, this plan first resolves that prior plan's disclosed gap: `Storage.Put` is extended to return the storage backend's real assigned key, which lets `GoogleDriveStorage` drop its in-memory-only file-ID map entirely (Open/Delete take the real Drive file ID directly, surviving process restarts).

**Tech Stack:** Go 1.26, PostgreSQL via pgx/v5, goose migrations, React 19 + TypeScript, Vite, Shadcn/Base UI, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-16-dokumentasi-kegiatan-gdrive-design.md`

## Global Constraints

- No task requires real Google Drive credentials to pass its tests — `GoogleDriveStorage` is exercised only through its existing fake-based test seams (Plan A already built these).
- `go build ./...`, `go vet ./...`, and `go test ./... -count=1 -p 1` (with `TEST_DATABASE_URL` set) must stay green at the end of every backend task. Two pre-existing, unrelated failures — `internal/recipients: TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal` and `internal/web: TestIntegrationLoginDashboardAndLogout` — are known flakiness from before this plan; do not try to fix them here, and verify via `git diff <task-base>..<task-head> -- internal/recipients internal/web` (expect empty) that this plan's tasks didn't touch either package before treating a failure there as pre-existing.
- `internal/distribution`'s existing local-disk photo upload behavior (production Pendistribusian feature) must remain byte-for-byte unaffected by Task 1's interface change — `LocalStorage.Put` continues to return the same `key` it was given as its storage key.
- Max activity-media file size is 100 MiB (`104857600` bytes) — larger than `media_files`' existing 10 MiB image-only cap, which is untouched.
- Allowed mime types: `image/jpeg`, `image/png`, `image/webp`, `video/mp4`, `video/webm`, `video/quicktime`.
- One shared permission pair for all 10 modules: `activities.view` / `activities.manage` (not per-module), granted to `super_admin` in the migration.
- `activity_media` has no `schedule_id`/`program_id` column — purely kabupaten + activity type + free timestamp, per the spec's explicit decision.
- Frontend: 10 new routes under `/dokumentasi/...`, each gated by `ProtectedPage permission="activities.view"`, sharing one generic `ActivityDocumentationPage` component. "Laporan" moves out of the "Dokumentasi" sidebar group into its own new group.

---

### Task 1: Extend `Storage.Put` to return the backend-assigned storage key

This resolves the disclosed gap from the prior media-storage-backend plan: `GoogleDriveStorage` currently tracks Drive file IDs in an in-memory-only map, which is lost on every restart. After this task, the real Drive file ID is returned directly from `Put` and persisted by the caller as `storage_key`, so `Open`/`Delete` take that real ID and need no map at all.

**Files:**
- Modify: `internal/media/storage.go`
- Modify: `internal/media/storage_test.go`
- Modify: `internal/media/gdrive.go`
- Modify: `internal/media/gdrive_test.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`

**Interfaces:**
- Produces: `media.Storage.Put(ctx, key string, folderPath []string, source io.Reader) (storageKey string, size int64, checksum string, err error)` — consumed by Task 6 (`internal/activities` service).
- `LocalStorage.Put` returns `storageKey == key` always (zero behavior change for the existing Pendistribusian caller).
- `GoogleDriveStorage.Put` returns the real Drive file ID as `storageKey`; `GoogleDriveStorage.Open`/`Delete` now take that real ID directly (the `fileIDs`/`fileIDsMu` fields are removed).

- [ ] **Step 1: Update `Storage` interface and `LocalStorage.Put`**

In `internal/media/storage.go`, replace:

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

with:

```go
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
}
```

Update `LocalStorage.Put`'s signature and every `return` in it to add `key`/`""` as the new first value:

```go
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
```

(Only the signature, every `return` statement's arity, and the final success `return` changed — `Open`/`Delete`/`path` are untouched.)

- [ ] **Step 2: Update `internal/media/storage_test.go` call sites**

In `TestLocalStorageRejectsTraversalAndWritesAtomically`, change:

```go
if _, _, err := storage.Put(context.Background(), key, nil, bytes.NewBufferString("secret")); !errors.Is(err, ErrInvalidKey) {
```

to:

```go
if _, _, _, err := storage.Put(context.Background(), key, nil, bytes.NewBufferString("secret")); !errors.Is(err, ErrInvalidKey) {
```

Change:

```go
size, checksum, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440000", nil, bytes.NewBufferString("photo"))
if err != nil {
	t.Fatal(err)
}
if size != 5 || checksum != "55c64d0fcd6f9d5f7c828093857e3fdfda68478bb4e9bd24d481ef391c7804e8" {
	t.Fatalf("size=%d checksum=%s", size, checksum)
}
```

to:

```go
storageKey, size, checksum, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440000", nil, bytes.NewBufferString("photo"))
if err != nil {
	t.Fatal(err)
}
if storageKey != "550e8400-e29b-41d4-a716-446655440000" || size != 5 || checksum != "55c64d0fcd6f9d5f7c828093857e3fdfda68478bb4e9bd24d481ef391c7804e8" {
	t.Fatalf("storageKey=%q size=%d checksum=%s", storageKey, size, checksum)
}
```

Change the interrupted-write assertion:

```go
if _, _, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440001", nil, &failingReader{}); err == nil {
```

to:

```go
if _, _, _, err := storage.Put(context.Background(), "550e8400-e29b-41d4-a716-446655440001", nil, &failingReader{}); err == nil {
```

- [ ] **Step 3: Run `internal/media` tests to see the compile failures the interface change causes**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... 2>&1 | head -50`
Expected: compile errors in `internal/media/gdrive.go` (wrong arity for `Put`) and `internal/distribution/service.go`/`service_test.go` (wrong arity at call sites) — this confirms every affected call site before you fix them in the next steps.

- [ ] **Step 4: Update `GoogleDriveStorage` — drop the in-memory `fileIDs` map**

In `internal/media/gdrive.go`, remove the `sync` import (no longer needed) and replace the struct + constructor + `Put`/`Open`/`Delete`:

Replace:

```go
type GoogleDriveStorage struct {
	api          driveFilesAPI
	cache        folderCache
	rootFolderID string
	// key -> Drive file ID, populated by Put, consumed by Open/Delete.
	// Guarded by fileIDsMu since Put/Open/Delete are called concurrently
	// from per-request goroutines once this backend is wired into the
	// server (Task 6).
	//
	// KNOWN GAP (accepted for this task): this map is in-memory only, so it
	// only remembers file IDs uploaded during the current process's
	// lifetime. Open/Delete for a key uploaded in a different process (e.g.
	// after a server restart) will fail to resolve. See task-4-report.md /
	// the media-storage-backend plan for the required follow-up: the caller
	// must persist the real Drive file ID and the Storage interface likely
	// needs to grow a way to report it back from Put.
	fileIDsMu sync.Mutex
	fileIDs   map[string]string
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
```

with:

```go
type GoogleDriveStorage struct {
	api          driveFilesAPI
	cache        folderCache
	rootFolderID string
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
	}, nil
}
```

Replace:

```go
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
	s.fileIDsMu.Lock()
	if s.fileIDs == nil {
		s.fileIDs = map[string]string{}
	}
	s.fileIDs[key] = fileID
	s.fileIDsMu.Unlock()
	return size, hashing.checksum(), nil
}

func (s *GoogleDriveStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	s.fileIDsMu.Lock()
	fileID, ok := s.fileIDs[key]
	s.fileIDsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("open drive file: unknown key %q", key)
	}
	return s.api.downloadFile(ctx, fileID)
}

func (s *GoogleDriveStorage) Delete(ctx context.Context, key string) error {
	s.fileIDsMu.Lock()
	fileID, ok := s.fileIDs[key]
	s.fileIDsMu.Unlock()
	if !ok {
		return nil
	}
	if err := s.api.deleteFile(ctx, fileID); err != nil {
		return err
	}
	s.fileIDsMu.Lock()
	delete(s.fileIDs, key)
	s.fileIDsMu.Unlock()
	return nil
}
```

with:

```go
func (s *GoogleDriveStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	folderID, err := s.resolveFolder(ctx, folderPath)
	if err != nil {
		return "", 0, "", fmt.Errorf("resolve drive folder: %w", err)
	}
	hashing := newHashingReader(source)
	fileID, size, err := s.api.uploadFile(ctx, key, folderID, hashing)
	if err != nil {
		return "", 0, "", err
	}
	return fileID, size, hashing.checksum(), nil
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
```

- [ ] **Step 5: Update `internal/media/gdrive_test.go`**

Replace `TestGoogleDriveStoragePutCreatesNestedFoldersAndCachesThem`'s body:

```go
func TestGoogleDriveStoragePutCreatesNestedFoldersAndCachesThem(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	_, size, checksum, err := storage.Put(context.Background(), "file-key-1", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("photo bytes"))
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
	if _, _, _, err := storage.Put(context.Background(), "file-key-2", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("more bytes")); err != nil {
		t.Fatal(err)
	}
	if len(api.createdFolders) != 4 {
		t.Fatalf("expected still 4 folders created after reusing cached path, got %v", api.createdFolders)
	}
}
```

Replace `TestGoogleDriveStorageOpenAndDelete`'s body:

```go
func TestGoogleDriveStorageOpenAndDelete(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	storageKey, _, _, err := storage.Put(context.Background(), "file-key-3", []string{"Konkit 2026", "Wajo"}, bytes.NewBufferString("hello drive"))
	if err != nil {
		t.Fatal(err)
	}
	if storageKey == "file-key-3" {
		t.Fatal("expected storageKey to be the fake API's assigned file ID, not the caller-supplied key")
	}

	reader, err := storage.Open(context.Background(), storageKey)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "hello drive" {
		t.Fatalf("content=%q", content)
	}

	if err := storage.Delete(context.Background(), storageKey); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Open(context.Background(), storageKey); err == nil {
		t.Fatal("expected Open after Delete to fail")
	}
}
```

- [ ] **Step 6: Update `internal/distribution/service.go`**

Change:

```go
	key, err := newStorageKey()
	if err != nil {
		return MediaFile{}, err
	}
	size, checksum, err := s.storage.Put(ctx, key, nil, bytes.NewReader(input.Data))
	if err != nil {
		return MediaFile{}, err
	}
	stored, err := s.mediaRepository.SaveMedia(ctx, actor, MediaFileInput{SlotID: slot.ID, StorageKey: key, OriginalFilename: strings.TrimSpace(input.OriginalFilename), MimeType: mimeType, Checksum: checksum, Source: input.Source, ByteSize: size, CapturedAt: input.CapturedAt, Latitude: input.Latitude, Longitude: input.Longitude}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return MediaFile{}, err
	}
```

to:

```go
	key, err := newStorageKey()
	if err != nil {
		return MediaFile{}, err
	}
	storageKey, size, checksum, err := s.storage.Put(ctx, key, nil, bytes.NewReader(input.Data))
	if err != nil {
		return MediaFile{}, err
	}
	stored, err := s.mediaRepository.SaveMedia(ctx, actor, MediaFileInput{SlotID: slot.ID, StorageKey: storageKey, OriginalFilename: strings.TrimSpace(input.OriginalFilename), MimeType: mimeType, Checksum: checksum, Source: input.Source, ByteSize: size, CapturedAt: input.CapturedAt, Latitude: input.Latitude, Longitude: input.Longitude}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return MediaFile{}, err
	}
```

(`LocalStorage.Put` guarantees `storageKey == key`, so this is behavior-identical — `internal/distribution` still stores a UUID-shaped key exactly as before.)

- [ ] **Step 7: Update `internal/distribution/service_test.go`'s `storageStub`**

Change:

```go
func (s *storageStub) Put(_ context.Context, key string, _ []string, source io.Reader) (int64, string, error) {
	s.putKey = key
	s.content, _ = io.ReadAll(source)
	return int64(len(s.content)), "checksum", s.putErr
}
```

to:

```go
func (s *storageStub) Put(_ context.Context, key string, _ []string, source io.Reader) (string, int64, string, error) {
	s.putKey = key
	s.content, _ = io.ReadAll(source)
	return key, int64(len(s.content)), "checksum", s.putErr
}
```

- [ ] **Step 8: Verify everything builds and tests pass**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... ./internal/distribution/... -v`
Expected: all pass, including the two rewritten `gdrive_test.go` tests.

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./... -count=1 -p 1`
Expected: all pass except the two pre-existing, already-documented failures named in Global Constraints.

- [ ] **Step 9: Commit**

```bash
git add internal/media/storage.go internal/media/storage_test.go internal/media/gdrive.go internal/media/gdrive_test.go internal/distribution/service.go internal/distribution/service_test.go
git commit -m "feat(media): return the backend-assigned storage key from Put, resolving GoogleDriveStorage's cross-process persistence gap"
```

---

### Task 2: `GoogleDriveStorage` — create reserved BA/Dokumen Pendukung sibling folders per kabupaten

Per spec section 2 (non-goals) and 4.3, the two future-feature folders "BERITA ACARA (BA)" and "DOKUMEN PENDUKUNG" must be created empty, once, the first time a kabupaten's folder is created — as siblings of "DOKUMENTASI FOTO & VIDEO", not descendants of it.

**Files:**
- Modify: `internal/media/gdrive.go`
- Modify: `internal/media/gdrive_test.go`

**Interfaces:**
- Consumes: `resolveFolder` (Task 4 of the prior plan), unchanged `driveFilesAPI`/`folderCache` interfaces.
- Produces: no new exported symbol — this only changes `resolveFolder`'s side effects. `internal/activities` (Task 6) relies on this happening automatically whenever it calls `Put` with a folder path shaped `["Konkit {tahun}", "{Nama Kabupaten}", "DOKUMENTASI FOTO & VIDEO", "{Jenis Kegiatan}"]` — index 1 is always the kabupaten level.

- [ ] **Step 1: Write the failing test**

Add to `internal/media/gdrive_test.go`:

```go
func TestGoogleDriveStoragePutCreatesReservedRegencyFoldersOnce(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	if _, _, _, err := storage.Put(context.Background(), "file-key-1", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("photo")); err != nil {
		t.Fatal(err)
	}
	wantReserved := map[string]bool{"BERITA ACARA (BA)": false, "DOKUMEN PENDUKUNG": false}
	for _, name := range api.createdFolders {
		if _, ok := wantReserved[name]; ok {
			wantReserved[name] = true
		}
	}
	for name, created := range wantReserved {
		if !created {
			t.Fatalf("expected reserved folder %q to be created alongside the regency folder, created folders: %v", name, api.createdFolders)
		}
	}
	countBefore := len(api.createdFolders)

	// A second Put for a DIFFERENT activity type under the SAME regency must
	// reuse the cached regency folder and must NOT recreate the reserved
	// siblings.
	if _, _, _, err := storage.Put(context.Background(), "file-key-2", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Pelatihan Teknis"}, bytes.NewBufferString("photo 2")); err != nil {
		t.Fatal(err)
	}
	if len(api.createdFolders) != countBefore+1 { // +1 for the new "Pelatihan Teknis" activity folder only
		t.Fatalf("expected no reserved-folder recreation, createdFolders after second Put: %v", api.createdFolders)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -run TestGoogleDriveStoragePutCreatesReservedRegencyFoldersOnce -v`
Expected: FAIL — `api.createdFolders` will not contain `"BERITA ACARA (BA)"`/`"DOKUMEN PENDUKUNG"` yet.

- [ ] **Step 3: Implement**

In `internal/media/gdrive.go`, replace `resolveFolder`:

```go
func (s *GoogleDriveStorage) resolveFolder(ctx context.Context, folderPath []string) (string, error) {
	parentID := s.rootFolderID
	pathKeyParts := make([]string, 0, len(folderPath))
	for i, name := range folderPath {
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
		isNew := found == ""
		if isNew {
			found, err = s.api.createFolder(ctx, name, parentID)
			if err != nil {
				return "", err
			}
		}
		if err := s.cache.Set(ctx, pathKey, found); err != nil {
			return "", err
		}
		parentID = found

		// Index 0 is always "Konkit {tahun}"; index 1 is always the
		// regency-level folder. The first time it's created, also create
		// its two reserved-for-future-features sibling folders (empty).
		if isNew && i == 1 {
			if err := s.ensureRegencyReservedFolders(ctx, pathKey, parentID); err != nil {
				return "", err
			}
		}
	}
	return parentID, nil
}

// ensureRegencyReservedFolders creates the "BERITA ACARA (BA)" and
// "DOKUMEN PENDUKUNG" folders as empty siblings of "DOKUMENTASI FOTO &
// VIDEO" under the regency folder, once. Both are reserved for future
// features (see spec section 2) and are never written to by this plan.
func (s *GoogleDriveStorage) ensureRegencyReservedFolders(ctx context.Context, regencyPathKey, regencyFolderID string) error {
	for _, name := range []string{"BERITA ACARA (BA)", "DOKUMEN PENDUKUNG"} {
		pathKey := regencyPathKey + "/" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		if _, ok, err := s.cache.Get(ctx, pathKey); err != nil {
			return err
		} else if ok {
			continue
		}
		found, err := s.api.findFolder(ctx, name, regencyFolderID)
		if err != nil {
			return err
		}
		if found == "" {
			found, err = s.api.createFolder(ctx, name, regencyFolderID)
			if err != nil {
				return err
			}
		}
		if err := s.cache.Set(ctx, pathKey, found); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/media/... -v`
Expected: all `internal/media` tests pass, including the new one and the two from Task 1.

- [ ] **Step 5: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/media/gdrive.go internal/media/gdrive_test.go
git commit -m "feat(media): create reserved BA/Dokumen Pendukung folders once per regency in GoogleDriveStorage"
```

---

### Task 3: Migration — `activity_media` table + `activities.view`/`activities.manage` permissions

**Files:**
- Create: `internal/database/migrations/00009_activities.sql`
- Create: `internal/activities/schema_integration_test.go`

**Interfaces:**
- Produces: `activity_media` table (columns per spec section 3), `activities.view`/`activities.manage` permissions granted to `super_admin` — consumed by Task 5 (repository).

- [ ] **Step 1: Write the failing test**

Create `internal/activities/schema_integration_test.go`:

```go
package activities

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

func activitiesIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func TestMigrationCreatesActivityMediaTableAndSeedsPermissions(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	ctx := context.Background()

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'activity_media')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected activity_media table to exist")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('activities.view','activities.manage')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 activities permissions seeded, got %d", count)
	}

	var superAdminGrants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp
		JOIN roles ON roles.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE roles.code = 'super_admin' AND p.code IN ('activities.view','activities.manage')
	`).Scan(&superAdminGrants); err != nil {
		t.Fatal(err)
	}
	if superAdminGrants != 2 {
		t.Fatalf("expected super_admin granted both new permissions, got %d", superAdminGrants)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/activities/... -v`
Expected: FAIL — `activity_media` table doesn't exist yet, `internal/activities` package doesn't otherwise compile-fail since this is its first file.

- [ ] **Step 3: Write the migration**

Create `internal/database/migrations/00009_activities.sql`:

```sql
-- +goose Up
CREATE TABLE activity_media (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    regency_id uuid NOT NULL REFERENCES regencies(id),
    activity_type text NOT NULL CHECK (activity_type IN (
        'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
        'training_10', 'training_100',
        'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
        'unloading_selang', 'unloading_tabung_gas'
    )),
    storage_key text NOT NULL UNIQUE,
    display_name text NOT NULL,
    original_filename text NOT NULL,
    media_type text NOT NULL CHECK (media_type IN ('image', 'video')),
    mime_type text NOT NULL CHECK (mime_type IN (
        'image/jpeg', 'image/png', 'image/webp',
        'video/mp4', 'video/webm', 'video/quicktime'
    )),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 104857600),
    checksum char(64) NOT NULL,
    source text NOT NULL CHECK (source IN ('camera', 'gallery')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleted')),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_media_regency_type_idx ON activity_media (regency_id, activity_type, status, uploaded_at DESC);

INSERT INTO permissions (code, name, description) VALUES
    ('activities.view', 'Lihat Dokumentasi Kegiatan', 'Melihat dokumentasi foto/video kegiatan lapangan lintas kabupaten'),
    ('activities.manage', 'Kelola Dokumentasi Kegiatan', 'Mengunggah dan menghapus dokumentasi foto/video kegiatan lapangan')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('activities.view', 'activities.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('activities.view', 'activities.manage');
DROP TABLE activity_media;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/activities/... -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/database/migrations/00009_activities.sql internal/activities/schema_integration_test.go
git commit -m "feat(activities): add activity_media table migration and activities.view/manage permissions"
```

---

### Task 4: `internal/activities` — models.go

**Files:**
- Create: `internal/activities/models.go`

**Interfaces:**
- Produces: `ActivityMedia`, `Filter`, `Page`, `UploadInput`, `MediaContent` types; sentinel errors; `activityTypeCodes`/`activityTypeFolderNames`/`allowedMimeTypes` maps; `isValidActivityType` — all consumed by Tasks 5-6.

- [ ] **Step 1: Write the file**

Create `internal/activities/models.go`:

```go
package activities

import (
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound            = errors.New("activity media not found")
	ErrRegencyRequired     = errors.New("regency_id is required")
	ErrRegencyNotFound     = errors.New("regency not found")
	ErrActivityTypeInvalid = errors.New("activity_type is not a recognized activity type")
	ErrMediaTypeInvalid    = errors.New("file must be a supported image or video format")
	ErrFileTooLarge        = errors.New("file exceeds the 100 MiB size limit")
	ErrSourceInvalid       = errors.New(`source must be "camera" or "gallery"`)
)

// maxFileBytes mirrors activity_media.byte_size's CHECK constraint (100 MiB).
const maxFileBytes = 100 << 20

var activityTypeCodes = map[string]string{
	"ceremony_sosialisasi":  "CEREMONY",
	"pelatihan_teknis":      "PELATIHAN",
	"rakor":                 "RAKOR",
	"training_10":           "TRAINING10",
	"training_100":          "TRAINING100",
	"unloading_konkit":      "UNLOADKONKIT",
	"unloading_mesin_pompa": "UNLOADPOMPA",
	"unloading_oli":         "UNLOADOLI",
	"unloading_selang":      "UNLOADSELANG",
	"unloading_tabung_gas":  "UNLOADTABUNG",
}

// activityTypeFolderNames are the exact Google Drive folder names under
// "DOKUMENTASI FOTO & VIDEO" for each activity type (spec section 4.3).
var activityTypeFolderNames = map[string]string{
	"ceremony_sosialisasi":  "Ceremony & Sosialisasi",
	"pelatihan_teknis":      "Pelatihan Teknis",
	"rakor":                 "Rakor",
	"training_10":           "Training 10%",
	"training_100":          "Training 100%",
	"unloading_konkit":      "Unloading Konkit",
	"unloading_mesin_pompa": "Unloading Mesin Pompa",
	"unloading_oli":         "Unloading Oli",
	"unloading_selang":      "Unloading Selang Hisap & Buang",
	"unloading_tabung_gas":  "Unloading Tabung Gas",
}

var allowedMimeTypes = map[string]string{
	"image/jpeg":      "image",
	"image/png":       "image",
	"image/webp":      "image",
	"video/mp4":       "video",
	"video/webm":      "video",
	"video/quicktime": "video",
}

func isValidActivityType(activityType string) bool {
	_, ok := activityTypeCodes[activityType]
	return ok
}

type ActivityMedia struct {
	ID                  string    `json:"id"`
	RegencyID           string    `json:"regency_id"`
	RegencyName         string    `json:"regency_name"`
	RegencyDocumentCode string    `json:"regency_document_code"`
	ActivityType        string    `json:"activity_type"`
	StorageKey          string    `json:"-"`
	DisplayName         string    `json:"display_name"`
	OriginalFilename    string    `json:"original_filename"`
	MediaType           string    `json:"media_type"`
	MimeType            string    `json:"mime_type"`
	ByteSize            int64     `json:"byte_size"`
	Checksum            string    `json:"checksum"`
	Source              string    `json:"source"`
	Status              string    `json:"status"`
	UploadedBy          *string   `json:"uploaded_by,omitempty"`
	UploadedAt          time.Time `json:"uploaded_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ContentURL          string    `json:"content_url"`
}

type Filter struct {
	RegencyID    string
	ActivityType string
	Page         int
	PageSize     int
}

type Page struct {
	Items    []ActivityMedia `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total"`
}

type UploadInput struct {
	RegencyID        string
	ActivityType     string
	OriginalFilename string
	Source           string
	Data             []byte
}

type MediaContent struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./internal/activities/...`
Expected: clean (this file has no behavior to test standalone — Tasks 5-6 exercise it).

- [ ] **Step 3: Commit**

```bash
git add internal/activities/models.go
git commit -m "feat(activities): add models.go (types, sentinel errors, activity-type metadata)"
```

---

### Task 5: `internal/activities` — repository.go + integration tests

**Files:**
- Create: `internal/activities/repository.go`
- Create: `internal/activities/repository_integration_test.go`

**Interfaces:**
- Consumes: `Filter`, `ActivityMedia`, `ErrNotFound`, `ErrRegencyNotFound` (Task 4); `auth.Principal`/`ClientMeta`/`RegencyScope` (existing); `audit.Record` (existing).
- Produces: `Repository`, `NewRepository(pool)`, `regencyInfo{ID, Name, DocumentCode}`, `insertInput`, `(*Repository).GetRegency`, `.List`, `.Insert`, `.GetByID`, `.SoftDelete`, `.restoreAfterFailedStorageDelete` — all consumed by Task 6 (service.go).

- [ ] **Step 1: Write the failing tests**

Create `internal/activities/repository_integration_test.go`:

```go
package activities

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type activityFixture struct {
	inScopeRegencyID    string
	outOfScopeRegencyID string
	actorUserID         string
}

func seedActivityFixture(t *testing.T, pool *pgxpool.Pool) activityFixture {
	t.Helper()
	ctx := context.Background()
	var fixture activityFixture

	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Wajo Activities Test','WAT',true) RETURNING id::text`).Scan(&fixture.inScopeRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Bone Activities Test','BAT',true) RETURNING id::text`).Scan(&fixture.outOfScopeRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES('activities-test-actor','activities-test-actor@example.test','x') RETURNING id::text`).Scan(&fixture.actorUserID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM activity_media WHERE regency_id IN ($1,$2)`, fixture.inScopeRegencyID, fixture.outOfScopeRegencyID); err != nil {
			t.Logf("cleanup: delete activity_media failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, fixture.actorUserID); err != nil {
			t.Logf("cleanup: delete users failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id IN ($1,$2)`, fixture.inScopeRegencyID, fixture.outOfScopeRegencyID); err != nil {
			t.Logf("cleanup: delete regencies failed: %v", err)
		}
	})
	return fixture
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetRegencyEnforcesScope(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}

	info, err := repository.GetRegency(ctx, fixture.inScopeRegencyID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if info.DocumentCode != "WAT" || info.Name != "Wajo Activities Test" {
		t.Fatalf("info = %+v", info)
	}

	if _, err := repository.GetRegency(ctx, fixture.outOfScopeRegencyID, scope); !errors.Is(err, ErrRegencyNotFound) {
		t.Fatalf("err = %v, want ErrRegencyNotFound", err)
	}
}

func TestInsertListGetByIDAndSoftDeleteRoundTrip(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.actorUserID}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "test"}
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}

	created, err := repository.Insert(ctx, actor, insertInput{
		RegencyID: fixture.inScopeRegencyID, ActivityType: "rakor", StorageKey: "storage-key-1",
		DisplayName: "WAT-RAKOR-20260916-154500", OriginalFilename: "foto.jpg",
		MediaType: "image", MimeType: "image/jpeg", ByteSize: 100, Checksum: "checksum-1", Source: "gallery",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if created.RegencyName != "Wajo Activities Test" || created.Status != "active" || created.DisplayName != "WAT-RAKOR-20260916-154500" {
		t.Fatalf("created = %+v", created)
	}

	page, err := repository.List(ctx, Filter{RegencyID: fixture.inScopeRegencyID, ActivityType: "rakor", Page: 1, PageSize: 20}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("page = %+v", page)
	}

	outOfScope := auth.RegencyScope{RegencyIDs: []string{fixture.outOfScopeRegencyID}}
	if _, err := repository.GetByID(ctx, created.ID, outOfScope); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for out-of-scope access", err)
	}

	storageKey, err := repository.SoftDelete(ctx, actor, created.ID, meta, scope)
	if err != nil {
		t.Fatal(err)
	}
	if storageKey != "storage-key-1" {
		t.Fatalf("storageKey = %q", storageKey)
	}

	if _, err := repository.GetByID(ctx, created.ID, scope); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound after soft delete", err)
	}

	if err := repository.restoreAfterFailedStorageDelete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetByID(ctx, created.ID, scope); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/activities/... -v`
Expected: FAIL — compile error, `Repository`/`NewRepository`/`insertInput` don't exist yet.

- [ ] **Step 3: Implement**

Create `internal/activities/repository.go`:

```go
package activities

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

type regencyInfo struct {
	ID           string
	Name         string
	DocumentCode string
}

func (r *Repository) GetRegency(ctx context.Context, regencyID string, scope auth.RegencyScope) (regencyInfo, error) {
	var info regencyInfo
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, name, document_code FROM regencies
		WHERE id = $1 AND ($2 OR id::text = ANY($3))
	`, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(&info.ID, &info.Name, &info.DocumentCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return regencyInfo{}, ErrRegencyNotFound
	}
	if err != nil {
		return regencyInfo{}, fmt.Errorf("get regency: %w", err)
	}
	return info, nil
}

const activityMediaSelect = `
SELECT m.id::text, m.regency_id::text, r.name, r.document_code, m.activity_type, m.storage_key,
	m.display_name, m.original_filename, m.media_type, m.mime_type, m.byte_size, m.checksum,
	m.source, m.status, m.uploaded_by, m.uploaded_at, m.created_at, m.updated_at
FROM activity_media m
JOIN regencies r ON r.id = m.regency_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanActivityMedia(row rowScanner) (ActivityMedia, error) {
	var item ActivityMedia
	var uploadedBy *string
	if err := row.Scan(&item.ID, &item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode, &item.ActivityType,
		&item.StorageKey, &item.DisplayName, &item.OriginalFilename, &item.MediaType, &item.MimeType,
		&item.ByteSize, &item.Checksum, &item.Source, &item.Status, &uploadedBy, &item.UploadedAt,
		&item.CreatedAt, &item.UpdatedAt); err != nil {
		return ActivityMedia{}, fmt.Errorf("scan activity media: %w", err)
	}
	item.UploadedBy = uploadedBy
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}
	rows, err := r.pool.Query(ctx, activityMediaSelect+`
		WHERE m.status = 'active' AND m.regency_id = $1 AND m.activity_type = $2
		AND ($3 OR r.id::text = ANY($4))
		ORDER BY m.uploaded_at DESC
		LIMIT $5 OFFSET $6
	`, filter.RegencyID, filter.ActivityType, scope.Unrestricted, scope.RegencyIDs, pageSize, (page-1)*pageSize)
	if err != nil {
		return Page{}, fmt.Errorf("list activity media: %w", err)
	}
	defer rows.Close()

	items := make([]ActivityMedia, 0, pageSize)
	for rows.Next() {
		item, err := scanActivityMedia(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate activity media: %w", err)
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM activity_media m JOIN regencies r ON r.id = m.regency_id
		WHERE m.status = 'active' AND m.regency_id = $1 AND m.activity_type = $2
		AND ($3 OR r.id::text = ANY($4))
	`, filter.RegencyID, filter.ActivityType, scope.Unrestricted, scope.RegencyIDs).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count activity media: %w", err)
	}

	return Page{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

type insertInput struct {
	RegencyID        string
	ActivityType     string
	StorageKey       string
	DisplayName      string
	OriginalFilename string
	MediaType        string
	MimeType         string
	ByteSize         int64
	Checksum         string
	Source           string
}

func (r *Repository) Insert(ctx context.Context, actor auth.Principal, input insertInput, meta auth.ClientMeta) (ActivityMedia, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ActivityMedia{}, fmt.Errorf("begin insert activity media: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO activity_media (regency_id, activity_type, storage_key, display_name, original_filename,
			media_type, mime_type, byte_size, checksum, source, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id::text
	`, input.RegencyID, input.ActivityType, input.StorageKey, input.DisplayName, input.OriginalFilename,
		input.MediaType, input.MimeType, input.ByteSize, input.Checksum, input.Source, actor.UserID).Scan(&id); err != nil {
		return ActivityMedia{}, fmt.Errorf("insert activity media: %w", err)
	}
	if err := recordActivityMediaAudit(ctx, tx, actor, meta, "uploaded", id, map[string]any{
		"regency_id": input.RegencyID, "activity_type": input.ActivityType,
		"mime_type": input.MimeType, "byte_size": input.ByteSize, "source": input.Source,
	}); err != nil {
		return ActivityMedia{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ActivityMedia{}, fmt.Errorf("commit insert activity media: %w", err)
	}
	return r.GetByID(ctx, id, auth.RegencyScope{Unrestricted: true})
}

func (r *Repository) GetByID(ctx context.Context, id string, scope auth.RegencyScope) (ActivityMedia, error) {
	row := r.pool.QueryRow(ctx, activityMediaSelect+`
		WHERE m.id = $1 AND m.status = 'active' AND ($2 OR r.id::text = ANY($3))
	`, id, scope.Unrestricted, scope.RegencyIDs)
	item, err := scanActivityMedia(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityMedia{}, ErrNotFound
	}
	if err != nil {
		return ActivityMedia{}, err
	}
	return item, nil
}

func (r *Repository) SoftDelete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin delete activity media: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storageKey, activityType, regencyID string
	err = tx.QueryRow(ctx, `
		SELECT m.storage_key, m.activity_type, m.regency_id::text
		FROM activity_media m
		JOIN regencies r ON r.id = m.regency_id
		WHERE m.id = $1 AND m.status = 'active' AND ($2 OR r.id::text = ANY($3))
		FOR UPDATE OF m
	`, id, scope.Unrestricted, scope.RegencyIDs).Scan(&storageKey, &activityType, &regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock activity media: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_media SET status='deleted', updated_at=now() WHERE id=$1`, id); err != nil {
		return "", fmt.Errorf("delete activity media: %w", err)
	}
	if err := recordActivityMediaAudit(ctx, tx, actor, meta, "deleted", id, map[string]any{
		"activity_type": activityType, "regency_id": regencyID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit delete activity media: %w", err)
	}
	return storageKey, nil
}

// restoreAfterFailedStorageDelete undoes a soft delete when the storage
// backend delete that must follow it fails, mirroring
// internal/distribution's mediaRepository.RestoreMedia. Internal-use only —
// not exposed via any API route.
func (r *Repository) restoreAfterFailedStorageDelete(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, `UPDATE activity_media SET status='active', updated_at=now() WHERE id=$1`, id); err != nil {
		return fmt.Errorf("restore activity media: %w", err)
	}
	return nil
}

func recordActivityMediaAudit(ctx context.Context, tx pgx.Tx, actor auth.Principal, meta auth.ClientMeta, action, resourceID string, metadata map[string]any) error {
	return audit.Record(ctx, tx, audit.Event{
		ActorUserID: actor.UserID, Action: "activity_media." + action, ResourceType: "activity_media",
		ResourceID: resourceID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/activities/... -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/activities/repository.go internal/activities/repository_integration_test.go
git commit -m "feat(activities): add repository.go (List, GetRegency, Insert, GetByID, SoftDelete)"
```

---

### Task 6: `internal/activities` — service.go + unit tests

**Files:**
- Create: `internal/activities/service.go`
- Create: `internal/activities/service_test.go`

**Interfaces:**
- Consumes: `Repository`'s methods as an unexported `repository` interface (Task 5); `media.Storage` (Task 1's new signature) directly, following `internal/distribution`'s exact pattern of importing `konkit/internal/media` rather than redeclaring a local copy.
- Produces: `Service`, `NewService(repository, storage media.Storage) *Service`, `.List`, `.Upload`, `.Delete`, `.OpenContent` — consumed by Task 7 (API routes) and `cmd/server/main.go`.

- [ ] **Step 1: Write the failing tests**

Create `internal/activities/service_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/activities/... -run TestUpload -v`
Expected: FAIL — compile error, `Service`/`NewService` don't exist yet.

- [ ] **Step 3: Implement**

Create `internal/activities/service.go`:

```go
package activities

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
)

type repository interface {
	GetRegency(ctx context.Context, regencyID string, scope auth.RegencyScope) (regencyInfo, error)
	List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error)
	Insert(ctx context.Context, actor auth.Principal, input insertInput, meta auth.ClientMeta) (ActivityMedia, error)
	GetByID(ctx context.Context, id string, scope auth.RegencyScope) (ActivityMedia, error)
	SoftDelete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) (string, error)
	restoreAfterFailedStorageDelete(ctx context.Context, id string) error
}

type Service struct {
	repository repository
	storage    media.Storage
}

func NewService(repository repository, storage media.Storage) *Service {
	return &Service{repository: repository, storage: storage}
}

func (s *Service) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	filter.RegencyID = strings.TrimSpace(filter.RegencyID)
	filter.ActivityType = strings.TrimSpace(filter.ActivityType)
	if !isValidActivityType(filter.ActivityType) {
		return Page{}, ErrActivityTypeInvalid
	}
	if filter.RegencyID == "" {
		return Page{}, ErrRegencyRequired
	}
	page, err := s.repository.List(ctx, filter, scope)
	if err != nil {
		return Page{}, err
	}
	for i := range page.Items {
		page.Items[i].ContentURL = "/api/v1/activities/media/" + page.Items[i].ID + "/content"
	}
	return page, nil
}

func (s *Service) Upload(ctx context.Context, actor auth.Principal, input UploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error) {
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.ActivityType = strings.TrimSpace(input.ActivityType)
	input.Source = strings.TrimSpace(input.Source)
	if !isValidActivityType(input.ActivityType) {
		return ActivityMedia{}, ErrActivityTypeInvalid
	}
	if input.Source != "camera" && input.Source != "gallery" {
		return ActivityMedia{}, ErrSourceInvalid
	}
	if len(input.Data) == 0 {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if len(input.Data) > maxFileBytes {
		return ActivityMedia{}, ErrFileTooLarge
	}
	regency, err := s.repository.GetRegency(ctx, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	mimeType, mediaType, ok := detectMediaType(input.Data, input.OriginalFilename)
	if !ok {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	key, err := newStorageKey()
	if err != nil {
		return ActivityMedia{}, err
	}
	folderPath := []string{
		fmt.Sprintf("Konkit %d", time.Now().Year()),
		regency.Name,
		"DOKUMENTASI FOTO & VIDEO",
		activityTypeFolderNames[input.ActivityType],
	}
	storageKey, size, checksum, err := s.storage.Put(ctx, key, folderPath, bytes.NewReader(input.Data))
	if err != nil {
		return ActivityMedia{}, err
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, activityTypeCodes[input.ActivityType], time.Now().Format("20060102-150405"))
	stored, err := s.repository.Insert(ctx, actor, insertInput{
		RegencyID: input.RegencyID, ActivityType: input.ActivityType, StorageKey: storageKey,
		DisplayName: displayName, OriginalFilename: strings.TrimSpace(input.OriginalFilename),
		MediaType: mediaType, MimeType: mimeType, ByteSize: size, Checksum: checksum, Source: input.Source,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, err
	}
	stored.ContentURL = "/api/v1/activities/media/" + stored.ID + "/content"
	return stored, nil
}

func (s *Service) Delete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	storageKey, err := s.repository.SoftDelete(ctx, actor, id, meta, scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, storageKey); err != nil {
		_ = s.repository.restoreAfterFailedStorageDelete(context.Background(), id)
		return err
	}
	return nil
}

func (s *Service) OpenContent(ctx context.Context, id string, scope auth.RegencyScope) (MediaContent, error) {
	item, err := s.repository.GetByID(ctx, id, scope)
	if err != nil {
		return MediaContent{}, err
	}
	reader, err := s.storage.Open(ctx, item.StorageKey)
	if err != nil {
		return MediaContent{}, err
	}
	return MediaContent{Reader: reader, MimeType: item.MimeType, Filename: item.OriginalFilename}, nil
}

// detectMediaType sniffs the upload's mime type. http.DetectContentType
// alone is unreliable for some video containers (e.g. .mov often sniffs as
// application/octet-stream), so an allow-listed extension is used as a
// fallback — never trusting the client-supplied header, only sniffed bytes
// or the upload's own filename extension.
func detectMediaType(data []byte, filename string) (mimeType, mediaType string, ok bool) {
	sniffed := http.DetectContentType(data[:min(len(data), 512)])
	if mt, found := allowedMimeTypes[sniffed]; found {
		return sniffed, mt, true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		return "video/mp4", "video", true
	case ".webm":
		return "video/webm", "video", true
	case ".mov":
		return "video/quicktime", "video", true
	}
	return "", "", false
}

func newStorageKey() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", fmt.Errorf("generate media key: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/activities/... -v`
Expected: all pass (unit tests run without `TEST_DATABASE_URL`; the Task 3/5 integration tests still skip cleanly if it's unset).

- [ ] **Step 5: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/activities/service.go internal/activities/service_test.go
git commit -m "feat(activities): add service.go (Upload, Delete, List, OpenContent)"
```

---

### Task 7: `internal/api` — activities routes + wiring

**Files:**
- Create: `internal/api/activities_routes.go`
- Create: `internal/api/activities_routes_test.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/routes.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `activities.Service` (Task 6) via a new `ActivitiesService` interface in `handler.go`, matching the exact pattern of `RecipientsService`.
- Produces: `GET/POST /api/v1/activities/media`, `DELETE /api/v1/activities/media/{id}`, `GET /api/v1/activities/media/{id}/content` — consumed by the frontend (Tasks 8-10).

- [ ] **Step 1: Write the failing tests**

Create `internal/api/activities_routes_test.go`:

```go
package api

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
)

type fakeActivitiesService struct {
	page             activities.Page
	uploaded         activities.ActivityMedia
	uploadErr        error
	deleteErr        error
	content          activities.MediaContent
	contentErr       error
	seenFilter       activities.Filter
	seenUploadInput  activities.UploadInput
	seenRegencyScope auth.RegencyScope
	seenDeleteID     string
}

func (f *fakeActivitiesService) List(_ context.Context, filter activities.Filter, scope auth.RegencyScope) (activities.Page, error) {
	f.seenFilter, f.seenRegencyScope = filter, scope
	return f.page, nil
}
func (f *fakeActivitiesService) Upload(_ context.Context, _ auth.Principal, input activities.UploadInput, _ auth.ClientMeta, scope auth.RegencyScope) (activities.ActivityMedia, error) {
	f.seenUploadInput, f.seenRegencyScope = input, scope
	return f.uploaded, f.uploadErr
}
func (f *fakeActivitiesService) Delete(_ context.Context, _ auth.Principal, id string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.seenDeleteID, f.seenRegencyScope = id, scope
	return f.deleteErr
}
func (f *fakeActivitiesService) OpenContent(_ context.Context, id string, scope auth.RegencyScope) (activities.MediaContent, error) {
	f.seenDeleteID, f.seenRegencyScope = id, scope
	return f.content, f.contentErr
}

func TestActivitiesListRequiresViewPermissionAndForwardsFilters(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.view": true}}
	service := &fakeActivitiesService{page: activities.Page{Page: 1, PageSize: 24, Total: 1, Items: []activities.ActivityMedia{{ID: "media-1", DisplayName: "WJO-RAKOR-20260916-154500"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media?regency_id=regency-1&activity_type=rakor&page=2&page_size=48", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "WJO-RAKOR-20260916-154500") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenFilter.RegencyID != "regency-1" || service.seenFilter.ActivityType != "rakor" || service.seenFilter.Page != 2 || service.seenFilter.PageSize != 48 {
		t.Fatalf("filters were not forwarded: %+v", service.seenFilter)
	}

	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{}}
	denied := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media", nil)
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Activities: service}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without activities.view, got %d", deniedRecorder.Code)
	}
}

func TestActivitiesUploadRequiresManagePermissionAndParsesMultipart(t *testing.T) {
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{uploaded: activities.ActivityMedia{ID: "media-1", DisplayName: "WJO-RAKOR-20260916-154500"}}

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "foto.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake jpeg bytes"))
	_ = writer.WriteField("source", "gallery")
	_ = writer.WriteField("activity_type", "rakor")
	_ = writer.WriteField("regency_id", "regency-1")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/media", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Activities: service}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenUploadInput.RegencyID != "regency-1" || service.seenUploadInput.ActivityType != "rakor" || service.seenUploadInput.Source != "gallery" || service.seenUploadInput.OriginalFilename != "foto.jpg" {
		t.Fatalf("upload input not forwarded: %+v", service.seenUploadInput)
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"activities.view": true}}
	deniedRecorder := httptest.NewRecorder()
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/activities/media", strings.NewReader(body.String()))
	deniedReq.Header.Set("Content-Type", writer.FormDataContentType())
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	NewHandler(Dependencies{Auth: viewer, Activities: service}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without activities.manage, got %d", deniedRecorder.Code)
	}
}

func TestActivityMediaDeleteRequiresManagePermission(t *testing.T) {
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/activities/media/media-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Activities: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || service.seenDeleteID != "media-1" {
		t.Fatalf("status=%d seenDeleteID=%q", rec.Code, service.seenDeleteID)
	}
}

func TestActivityMediaContentStreamsWithMimeType(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.view": true}}
	service := &fakeActivitiesService{content: activities.MediaContent{Reader: io.NopCloser(strings.NewReader("bytes")), MimeType: "image/jpeg", Filename: "foto.jpg"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media/media-1/content", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.String() != "bytes" {
		t.Fatalf("status=%d content-type=%s body=%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/api/... -run TestActivit -v`
Expected: FAIL — compile error, `Dependencies.Activities` field doesn't exist yet.

- [ ] **Step 3: Add `ActivitiesService` interface and `Dependencies` field**

In `internal/api/handler.go`, add `"konkit/internal/activities"` to the import block, then add this interface next to `RecipientsService` (around handler.go:106-113):

```go
type ActivitiesService interface {
	List(context.Context, activities.Filter, auth.RegencyScope) (activities.Page, error)
	Upload(context.Context, auth.Principal, activities.UploadInput, auth.ClientMeta, auth.RegencyScope) (activities.ActivityMedia, error)
	Delete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	OpenContent(context.Context, string, auth.RegencyScope) (activities.MediaContent, error)
}
```

Add `Activities ActivitiesService` to the `Dependencies` struct, alongside the existing `Recipients RecipientsService` field.

Add these two cases to `routeProtected`'s switch (`internal/api/handler.go`, alongside the existing `path == "recipients"` cases around handler.go:240-245):

```go
case path == "activities/media":
	h.handleActivitiesMedia(w, r, rc)
case strings.HasPrefix(path, "activities/media/"):
	h.handleActivityMediaItem(w, r, rc, strings.TrimPrefix(path, "activities/media/"))
```

- [ ] **Step 4: Write `internal/api/activities_routes.go`**

```go
package api

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"konkit/internal/activities"
)

const maxActivityMediaRequestBody = 101 << 20 // 100 MiB + multipart overhead margin

func activityFilterFromRequest(r *http.Request) activities.Filter {
	return activities.Filter{
		RegencyID: r.URL.Query().Get("regency_id"), ActivityType: r.URL.Query().Get("activity_type"),
		Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 24),
	}
}

func (h *Handler) handleActivitiesMedia(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		page, err := h.deps.Activities.List(r.Context(), activityFilterFromRequest(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, page)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "activities.manage") {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxActivityMediaRequestBody)
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "File melebihi batas 100 MiB")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "File wajib dipilih", map[string]string{"file": "File wajib dipilih"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (100<<20)+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "media_invalid", "File tidak dapat dibaca")
			return
		}
		if len(data) > 100<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "File melebihi batas 100 MiB")
			return
		}
		input := activities.UploadInput{
			RegencyID: strings.TrimSpace(r.FormValue("regency_id")), ActivityType: strings.TrimSpace(r.FormValue("activity_type")),
			OriginalFilename: header.Filename, Source: strings.TrimSpace(r.FormValue("source")), Data: data,
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Activities.Upload(r.Context(), rc.principal, input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleActivityMediaItem(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		content, err := h.deps.Activities.OpenContent(r.Context(), parts[0], scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", content.MimeType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if !h.authorize(w, r, rc.principal, "activities.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Activities.Delete(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}
```

- [ ] **Step 5: Wire error mapping in `internal/api/routes.go`**

Add `"konkit/internal/activities"` to the import block. In `writeServiceError`, add `activities.ErrNotFound` and `activities.ErrRegencyNotFound` to the existing combined not-found case (around routes.go:449):

```go
case errors.Is(err, profile.ErrNotFound), errors.Is(err, administration.ErrNotFound), errors.Is(err, programs.ErrNotFound), errors.Is(err, dcp3.ErrPreviewNotFound), errors.Is(err, distribution.ErrAllocationNotFound), errors.Is(err, distribution.ErrMediaNotFound), errors.Is(err, reports.ErrScheduleNotFound), errors.Is(err, recipients.ErrNotFound), errors.Is(err, recipients.ErrScheduleNotFound), errors.Is(err, activities.ErrNotFound), errors.Is(err, activities.ErrRegencyNotFound):
```

Add three new cases anywhere after that in the same `switch` (before the `default:` case):

```go
case errors.Is(err, activities.ErrActivityTypeInvalid), errors.Is(err, activities.ErrSourceInvalid), errors.Is(err, activities.ErrRegencyRequired):
	writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), map[string]string{"request": err.Error()})
case errors.Is(err, activities.ErrMediaTypeInvalid):
	writeFieldError(w, http.StatusBadRequest, "media_invalid", err.Error(), map[string]string{"file": err.Error()})
case errors.Is(err, activities.ErrFileTooLarge):
	writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", err.Error())
```

- [ ] **Step 6: Wire `cmd/server/main.go`**

Add `"konkit/internal/activities"` to the import block. Add `Activities: activities.NewService(activities.NewRepository(pool), mediaStorage),` to the `apihttp.Dependencies{...}` literal, alongside the existing `Recipients: recipients.NewService(recipients.NewRepository(pool)),` line.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/api/... -v`
Expected: all pass, including the four new tests.

- [ ] **Step 8: Verify the whole build**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: both clean.

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./... -count=1 -p 1`
Expected: all pass except the two pre-existing, already-documented failures named in Global Constraints.

- [ ] **Step 9: Manual smoke test**

Run: `cd "d:/KSM/Deployment/konkit" && go run ./cmd/server` — confirm it starts without error (default `STORAGE_BACKEND=local`). Stop it with Ctrl+C once you see the "listening on" log line.

- [ ] **Step 10: Commit**

```bash
git add internal/api/activities_routes.go internal/api/activities_routes_test.go internal/api/handler.go internal/api/routes.go cmd/server/main.go
git commit -m "feat(api): wire activities routes (list/upload/delete/content) with activities.view/manage permission gating"
```

---

### Task 8: Frontend — `types.ts` + pagination helper

**Files:**
- Create: `frontend/src/features/activities/types.ts`
- Create: `frontend/src/features/activities/pagination.ts`

**Interfaces:**
- Produces: `ActivityType`, `ActivityMedia`, `ActivityMediaPage`, `RegencyOption`, `activityTypeLabels` — consumed by Task 9. `buildPageItems`/`PageItem` (duplicated from `frontend/src/features/dashboard/recipientTable.ts` — a small pure function, kept local to this feature rather than importing across unrelated feature folders) — consumed by Task 9.

- [ ] **Step 1: Write `frontend/src/features/activities/types.ts`**

```ts
export type ActivityType =
  | 'ceremony_sosialisasi'
  | 'pelatihan_teknis'
  | 'rakor'
  | 'training_10'
  | 'training_100'
  | 'unloading_konkit'
  | 'unloading_mesin_pompa'
  | 'unloading_oli'
  | 'unloading_selang'
  | 'unloading_tabung_gas';

export type ActivityMedia = {
  id: string;
  regency_id: string;
  regency_name: string;
  regency_document_code: string;
  activity_type: ActivityType;
  display_name: string;
  original_filename: string;
  media_type: 'image' | 'video';
  mime_type: string;
  byte_size: number;
  checksum: string;
  source: 'camera' | 'gallery';
  status: 'active' | 'deleted';
  uploaded_by?: string;
  uploaded_at: string;
  created_at: string;
  updated_at: string;
  content_url: string;
};

export type ActivityMediaPage = {
  items: ActivityMedia[];
  page: number;
  page_size: number;
  total: number;
};

export type RegencyOption = { id: string; name: string; document_code: string };

export const activityTypeLabels: Record<ActivityType, string> = {
  ceremony_sosialisasi: 'Ceremony & Sosialisasi',
  pelatihan_teknis: 'Pelatihan Teknis',
  rakor: 'Rakor',
  training_10: 'Training 10%',
  training_100: 'Training 100%',
  unloading_konkit: 'Unloading Konkit',
  unloading_mesin_pompa: 'Unloading Mesin Pompa',
  unloading_oli: 'Unloading Oli',
  unloading_selang: 'Unloading Selang Hisap & Buang',
  unloading_tabung_gas: 'Unloading Tabung Gas',
};
```

- [ ] **Step 2: Write `frontend/src/features/activities/pagination.ts`**

```ts
export type PageItem = number | 'ellipsis-start' | 'ellipsis-end';

export function buildPageItems(currentPage: number, totalPages: number): PageItem[] {
  if (totalPages < 1) return [];
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, index) => index + 1);

  const current = Math.min(Math.max(currentPage, 1), totalPages);
  if (current <= 4) return [1, 2, 3, 4, 5, 'ellipsis-end', totalPages];
  if (current >= totalPages - 3) {
    return [1, 'ellipsis-start', totalPages - 4, totalPages - 3, totalPages - 2, totalPages - 1, totalPages];
  }
  return [1, 'ellipsis-start', current - 1, current, current + 1, 'ellipsis-end', totalPages];
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npm.cmd run build`
Expected: clean (these are pure type/utility files with no runtime behavior to test standalone yet — Task 9 exercises `buildPageItems`).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/activities/types.ts frontend/src/features/activities/pagination.ts
git commit -m "feat(activities-ui): add frontend types and pagination helper"
```

---

### Task 9: Frontend — `ActivityDocumentationPage.tsx` generic component + test

**Files:**
- Create: `frontend/src/features/activities/ActivityDocumentationPage.tsx`
- Create: `frontend/src/features/activities/ActivityDocumentationPage.test.tsx`

**Interfaces:**
- Consumes: `ActivityType`, `ActivityMedia`, `ActivityMediaPage`, `RegencyOption`, `activityTypeLabels` (Task 8); `apiRequest` (`lib/api`), `useCan` (`lib/permissions`) — both existing.
- Produces: `ActivityDocumentationPage({ activityType, label })` — consumed by Task 10 (routes.tsx, one instance per activity type).

- [ ] **Step 1: Write `frontend/src/features/activities/ActivityDocumentationPage.tsx`**

```tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, ImagePlus, PlayCircle, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { DataState } from '@/components/DataState';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { PageHeader } from '@/components/PageHeader';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { buildPageItems } from './pagination';
import type { ActivityMedia, ActivityMediaPage, ActivityType, RegencyOption } from './types';

type PendingFile = { file: File; source: 'camera' | 'gallery'; previewURL: string };
const acceptedTypes = 'image/jpeg,image/png,image/webp,video/mp4,video/webm,video/quicktime';
const uploadButtonClass = 'inline-flex cursor-pointer items-center gap-2 rounded-md border bg-secondary px-4 py-2 text-sm font-medium text-secondary-foreground hover:bg-secondary/80';

export function ActivityDocumentationPage({ activityType, label }: { activityType: ActivityType; label: string }) {
  const canManage = useCan('activities.manage');
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const regencyID = params.get('regency_id') ?? '';
  const page = Number(params.get('page') ?? '1') || 1;
  const [pending, setPending] = useState<PendingFile | null>(null);
  const [preview, setPreview] = useState<ActivityMedia | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ActivityMedia | null>(null);

  useEffect(() => () => { if (pending?.previewURL) URL.revokeObjectURL(pending.previewURL); }, [pending]);

  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const gallery = useQuery({
    queryKey: ['activities', activityType, regencyID, page],
    queryFn: () => apiRequest<{ data: ActivityMediaPage }>(`/api/v1/activities/media?activity_type=${activityType}&regency_id=${regencyID}&page=${page}&page_size=24`),
    enabled: regencyID !== '',
  });

  const setRegency = (value: string) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); next.delete('page'); return next; });
  const setPage = (value: number) => setParams((prev) => { const next = new URLSearchParams(prev); next.set('page', String(value)); return next; });

  const upload = useMutation({
    mutationFn: ({ file, source }: PendingFile) => {
      const body = new FormData();
      body.set('file', file); body.set('source', source); body.set('activity_type', activityType); body.set('regency_id', regencyID);
      return apiRequest<{ data: ActivityMedia }>('/api/v1/activities/media', { method: 'POST', body });
    },
    onSuccess: () => { setPending(null); client.invalidateQueries({ queryKey: ['activities', activityType, regencyID] }); toast.success('Dokumentasi berhasil diunggah.'); },
    onError: () => toast.error('Gagal mengunggah dokumentasi.'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => { setPendingDelete(null); setPreview(null); client.invalidateQueries({ queryKey: ['activities', activityType, regencyID] }); toast.success('Dokumentasi dihapus.'); },
  });

  const choose = (file: File | undefined, source: 'camera' | 'gallery') => {
    if (!file || !regencyID) return;
    const selected = { file, source, previewURL: URL.createObjectURL(file) };
    setPending(selected); upload.mutate(selected);
  };

  const items = gallery.data?.data.items ?? [];
  const total = gallery.data?.data.total ?? 0;
  const pageSize = gallery.data?.data.page_size ?? 24;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pageItems = buildPageItems(page, totalPages);

  return <div className="space-y-6">
    <PageHeader title={label} description="Dokumentasi foto/video kegiatan lapangan, tidak terikat jadwal." />

    <div className="grid max-w-xs gap-2 rounded-xl border bg-card p-4"><Label id="filter-regency-label">Kabupaten</Label><Select value={regencyID} onValueChange={setRegency}><SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder="Pilih kabupaten" /></SelectTrigger><SelectContent>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>

    {!regencyID ? <DataState kind="empty" title="Pilih kabupaten" description="Pilih kabupaten untuk melihat dan mengunggah dokumentasi." /> : <>
      {canManage && <div className="flex flex-wrap gap-2">
        <label className={uploadButtonClass}><Camera />Buka kamera<input aria-label="Buka kamera" type="file" accept={acceptedTypes} capture="environment" className="sr-only" onChange={(event) => choose(event.target.files?.[0], 'camera')} /></label>
        <label className={uploadButtonClass}><ImagePlus />Pilih galeri<input aria-label="Pilih galeri" type="file" accept={acceptedTypes} className="sr-only" onChange={(event) => choose(event.target.files?.[0], 'gallery')} /></label>
      </div>}

      {gallery.isError ? <DataState kind="error" title="Dokumentasi belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => gallery.refetch() }} />
        : gallery.isPending ? <DataState kind="loading" title="Memuat dokumentasi" description="Mengambil data dari kabupaten terpilih." />
        : items.length === 0 && !pending ? <DataState kind="empty" title="Belum ada dokumentasi" description="Unggah foto atau video pertama untuk kegiatan ini." />
        : <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
          {pending && <figure className="relative aspect-square overflow-hidden rounded-lg border bg-muted"><img src={pending.previewURL} alt="Preview unggahan" className="size-full object-cover" /><figcaption className="absolute inset-x-0 bottom-0 bg-black/60 px-2 py-1 text-xs text-white">{upload.isError ? 'Gagal' : 'Mengunggah...'}</figcaption></figure>}
          {items.map((item) => <button key={item.id} type="button" className="group relative aspect-square overflow-hidden rounded-lg border bg-muted" onClick={() => setPreview(item)}>
            {item.media_type === 'video'
              ? <><video src={item.content_url} className="size-full object-cover" muted /><PlayCircle aria-hidden="true" className="absolute inset-0 m-auto size-8 text-white drop-shadow" /></>
              : <img src={item.content_url} alt={item.display_name} className="size-full object-cover" />}
            <span className="absolute inset-x-0 bottom-0 truncate bg-black/60 px-2 py-1 text-left text-xs text-white">{item.display_name}</span>
          </button>)}
        </div>}

      {totalPages > 1 && <nav className="flex flex-wrap items-center justify-center gap-1" aria-label={`Pagination ${label}`}>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman pertama" disabled={page <= 1} onClick={() => setPage(1)}><ChevronsLeft /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman sebelumnya" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
        {pageItems.map((item) => typeof item === 'number'
          ? <Button type="button" size="icon-sm" variant={item === page ? 'default' : 'outline'} aria-label={`Halaman ${item}`} aria-current={item === page ? 'page' : undefined} key={item} onClick={() => setPage(item)}>{item}</Button>
          : <span key={item} aria-hidden="true" className="flex size-11 items-center justify-center">…</span>)}
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman berikutnya" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman terakhir" disabled={page >= totalPages} onClick={() => setPage(totalPages)}><ChevronsRight /></Button>
      </nav>}
    </>}

    <Dialog open={Boolean(preview)} onOpenChange={(open) => { if (!open) setPreview(null); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader><DialogTitle>{preview?.display_name}</DialogTitle></DialogHeader>
        {preview && (preview.media_type === 'video'
          ? <video src={preview.content_url} controls className="max-h-[70vh] w-full rounded-lg" />
          : <img src={preview.content_url} alt={preview.display_name} className="max-h-[70vh] w-full rounded-lg object-contain" />)}
        {canManage && preview && <Button type="button" variant="destructive" onClick={() => setPendingDelete(preview)}><Trash2 />Hapus</Button>}
      </DialogContent>
    </Dialog>

    <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Hapus {pendingDelete?.display_name}?</AlertDialogTitle><AlertDialogDescription>Dokumentasi ini akan dihapus dan tidak lagi tampil di galeri.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingDelete) remove.mutate(pendingDelete.id); }}>Hapus</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
```

- [ ] **Step 2: Write `frontend/src/features/activities/ActivityDocumentationPage.test.tsx`**

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { ActivityDocumentationPage } from './ActivityDocumentationPage';
import type { ActivityMediaPage } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPage(permissions: string[], initialEntries: string[] = ['/dokumentasi/rakor']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={initialEntries}><ActivityDocumentationPage activityType="rakor" label="Rakor" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function mockApi(gallery: ActivityMediaPage = { items: [], page: 1, page_size: 24, total: 0 }) {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: gallery });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

test('prompts to pick a kabupaten before loading the gallery', async () => {
  mockApi();
  renderPage(['activities.view']);
  expect(await screen.findByText('Pilih kabupaten')).toBeVisible();
});

test('shows the gallery once a kabupaten is selected', async () => {
  mockApi({
    items: [{
      id: 'media-1', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', activity_type: 'rakor',
      display_name: 'WJO-RAKOR-20260916-154500', original_filename: 'foto.jpg', media_type: 'image', mime_type: 'image/jpeg',
      byte_size: 100, checksum: 'abc', source: 'gallery', status: 'active',
      uploaded_at: '2026-09-16T15:45:00Z', created_at: '2026-09-16T15:45:00Z', updated_at: '2026-09-16T15:45:00Z',
      content_url: '/api/v1/activities/media/media-1/content',
    }], page: 1, page_size: 24, total: 1,
  });
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  expect(await screen.findByAltText('WJO-RAKOR-20260916-154500')).toBeVisible();
});

test('hides upload controls without activities.manage', async () => {
  mockApi();
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  await screen.findByText('Belum ada dokumentasi');
  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Pilih galeri')).not.toBeInTheDocument();
});

test('uploads a photo via the gallery picker', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') {
      return Promise.resolve({ data: { id: 'media-2', display_name: 'WJO-RAKOR-20260916-160000', media_type: 'image', content_url: '/api/v1/activities/media/media-2/content' } });
    }
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);
  const file = new File(['photo'], 'foto.jpg', { type: 'image/jpeg' });
  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [file] } });
  expect(await screen.findByAltText('Preview unggahan')).toHaveAttribute('src', 'blob:preview');
});
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npx vitest run src/features/activities`
Expected: all 4 tests pass.

- [ ] **Step 4: Verify the whole frontend build**

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npm.cmd run build`
Expected: clean (this component isn't referenced by any route yet — Task 10 wires it in).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/activities/ActivityDocumentationPage.tsx frontend/src/features/activities/ActivityDocumentationPage.test.tsx
git commit -m "feat(activities-ui): add generic ActivityDocumentationPage component"
```

---

### Task 10: Frontend — routes + sidebar navigation

**Files:**
- Modify: `frontend/src/app/routes.tsx`
- Modify: `frontend/src/app/AppShell.tsx`

**Interfaces:**
- Consumes: `ActivityDocumentationPage` (Task 9).
- Produces: 10 new routes at `/dokumentasi/{slug}`, each `ProtectedPage permission="activities.view"`; "Dokumentasi" sidebar group gains 10 items; "Laporan" moves into its own new sidebar group.

- [ ] **Step 1: Update `frontend/src/app/routes.tsx`**

Add this import alongside the existing feature imports:

```tsx
import { ActivityDocumentationPage } from '../features/activities/ActivityDocumentationPage';
```

Insert these 10 route entries into the `children` array, right after the existing `dokumentasi/pendistribusian` entry:

```tsx
    { path: 'dokumentasi/ceremony-sosialisasi', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="ceremony_sosialisasi" label="Ceremony & Sosialisasi" /></ProtectedPage> },
    { path: 'dokumentasi/pelatihan-teknis', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="pelatihan_teknis" label="Pelatihan Teknis" /></ProtectedPage> },
    { path: 'dokumentasi/rakor', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="rakor" label="Rakor" /></ProtectedPage> },
    { path: 'dokumentasi/training-10', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="training_10" label="Training 10%" /></ProtectedPage> },
    { path: 'dokumentasi/training-100', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="training_100" label="Training 100%" /></ProtectedPage> },
    { path: 'dokumentasi/unloading-konkit', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="unloading_konkit" label="Unloading Konkit" /></ProtectedPage> },
    { path: 'dokumentasi/unloading-mesin-pompa', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="unloading_mesin_pompa" label="Unloading Mesin Pompa" /></ProtectedPage> },
    { path: 'dokumentasi/unloading-oli', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="unloading_oli" label="Unloading Oli" /></ProtectedPage> },
    { path: 'dokumentasi/unloading-selang', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="unloading_selang" label="Unloading Selang Hisap & Buang" /></ProtectedPage> },
    { path: 'dokumentasi/unloading-tabung-gas', element: <ProtectedPage permission="activities.view"><ActivityDocumentationPage activityType="unloading_tabung_gas" label="Unloading Tabung Gas" /></ProtectedPage> },
```

- [ ] **Step 2: Update `frontend/src/app/AppShell.tsx`**

Add these icon imports to the existing `lucide-react` import block (alongside `Activity, CalendarClock, ...`): `BookOpen, BookOpenCheck, Cog, Cylinder, Droplet, GraduationCap, PackageOpen, PartyPopper, Users, Waves`. (If any of these names don't exist in the installed `lucide-react` version, substitute the closest available icon with the same visual intent and note the substitution — this is a cosmetic choice, not a functional one.)

Replace the "Dokumentasi" group and add a new "Laporan" group right after it — change:

```tsx
  { label: 'Dokumentasi', items: [
    { label: 'Pendistribusian', to: '/dokumentasi/pendistribusian', permission: 'distribution.view', icon: <Camera /> },
    { label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
  ] },
```

to:

```tsx
  { label: 'Dokumentasi', items: [
    { label: 'Pendistribusian', to: '/dokumentasi/pendistribusian', permission: 'distribution.view', icon: <Camera /> },
    { label: 'Ceremony & Sosialisasi', to: '/dokumentasi/ceremony-sosialisasi', permission: 'activities.view', icon: <PartyPopper /> },
    { label: 'Pelatihan Teknis', to: '/dokumentasi/pelatihan-teknis', permission: 'activities.view', icon: <GraduationCap /> },
    { label: 'Rakor', to: '/dokumentasi/rakor', permission: 'activities.view', icon: <Users /> },
    { label: 'Training 10%', to: '/dokumentasi/training-10', permission: 'activities.view', icon: <BookOpen /> },
    { label: 'Training 100%', to: '/dokumentasi/training-100', permission: 'activities.view', icon: <BookOpenCheck /> },
    { label: 'Unloading Konkit', to: '/dokumentasi/unloading-konkit', permission: 'activities.view', icon: <PackageOpen /> },
    { label: 'Unloading Mesin Pompa', to: '/dokumentasi/unloading-mesin-pompa', permission: 'activities.view', icon: <Cog /> },
    { label: 'Unloading Oli', to: '/dokumentasi/unloading-oli', permission: 'activities.view', icon: <Droplet /> },
    { label: 'Unloading Selang Hisap & Buang', to: '/dokumentasi/unloading-selang', permission: 'activities.view', icon: <Waves /> },
    { label: 'Unloading Tabung Gas', to: '/dokumentasi/unloading-tabung-gas', permission: 'activities.view', icon: <Cylinder /> },
  ] },
  { label: 'Laporan', items: [
    { label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
  ] },
```

(`FileText` and `Camera` are already imported — no change needed for those.)

- [ ] **Step 3: Verify the whole frontend build**

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npm.cmd run build`
Expected: clean.

- [ ] **Step 4: Run the whole frontend test suite**

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npx vitest run`
Expected: all pass, including the pre-existing `AppShell`/`routes` tests if any exist (check for and update any test that snapshots the `groups` array or route list, since this task changes both).

- [ ] **Step 5: Manual smoke test**

Start the backend (`go run ./cmd/server`) and frontend dev server, log in as a `super_admin` user, and confirm: the "Dokumentasi" sidebar group shows 11 items (Pendistribusian + 10 new), a new "Laporan" group appears separately, each of the 10 new pages loads, the kabupaten picker populates, and an upload completes and appears in the gallery. Stop both servers when done.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/routes.tsx frontend/src/app/AppShell.tsx
git commit -m "feat(activities-ui): add 10 sidebar routes, move Laporan out of the Dokumentasi group"
```

---

## After This Plan

- All 10 activity-documentation modules are live, backed by whichever `STORAGE_BACKEND` is configured (`local` by default; `gdrive` per the prior plan's documented readiness caveat in `.env.example`/`README.md`).
- The `Storage.Put` persistence gap flagged at the end of the media-storage-backend plan is now fully resolved (Task 1) — `GoogleDriveStorage` no longer depends on any in-memory state, so `STORAGE_BACKEND=gdrive` is now genuinely production-viable for this feature, not just for Pendistribusian's pre-existing risk.
- Not built in this phase (per spec non-goals): the "BERITA ACARA (BA)"/"DOKUMEN PENDUKUNG" folders exist but stay empty (Task 2 only creates them); no Drive file-manager UI; no migration of existing local files to Drive.
