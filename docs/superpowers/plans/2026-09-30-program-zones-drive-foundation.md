# Program Zones and Drive Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tender-scoped zones, immutable document profiles with ordered logos, and a single program-aware folder hierarchy used by new Drive uploads.

**Architecture:** Extend the existing `programs` package instead of creating a second setup subsystem. A focused `media.FolderPathBuilder` derives storage paths from program type, zone, and regency; activities and distribution supply program context through their existing repository/service chains. Document-profile versions are managed by `programs`, while binary logo content uses the existing `media.Storage` abstraction.

**Tech Stack:** Go 1.24, PostgreSQL/Goose, `pgx/v5`, Google Drive v3 storage adapter, React 19, TypeScript, TanStack Query, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-09-30-ba-perorangan-zona-drive-design.md`

## Global Constraints

- Work on `main` as requested; preserve and stage only files belonging to each task.
- One program represents one tender and exactly one `program_type`: `farmer` or `fisherman`.
- One regency belongs to exactly one zone inside a program, but may belong to a different zone in another program.
- Existing schedules and files must remain readable; no existing Drive file or folder is moved or deleted by this plan.
- New upload paths start with `PETANI` or `NELAYAN`, then zone, then regency.
- Drive files remain private and are streamed through authenticated backend endpoints.
- Published document-profile versions are immutable; edits create a new version.
- Petani and Nelayan configuration and assets must never be used as fallback for one another.

## Review Focus

- A program/regency pair still assigned to `ZONA BELUM DIATUR` must be readable but must reject new uploads with an actionable error; Task 2 and Task 6 pin this behavior.
- Zone/regency names containing `/`, `\\`, repeated whitespace, or control characters must not create an extra Drive path level; Task 4 pins sanitization.
- Two simultaneous saves assigning one regency to different zones must end with one database row, not duplicate membership; Task 1 pins the constraint.
- A failed logo upload must not leave a database asset row pointing to missing content; Task 7 pins cleanup/transaction ordering.
- Existing `activity_media` rows without determinable program context must remain listable and downloadable after migration; Task 5 pins legacy compatibility.

---

### Task 1: Add zone, assignment, and document-profile schema

**Files:**
- Create: `internal/database/migrations/00020_program_zones_drive_profiles.sql`
- Modify: `internal/programs/repository_integration_test.go`
- Create: `internal/programs/schema_integration_test.go`

**Interfaces:**
- Consumes: existing `programs`, `regencies`, `program_schedules`, `activity_media`, and `drive_folder_cache` tables.
- Produces: `program_zones`, `program_regency_assignments`, `program_document_profile_versions`, `program_document_logo_assets`, and nullable legacy-safe `activity_media.program_id`.

- [ ] **Step 1: Write failing schema tests**

Add integration tests that run migrations and assert:

```go
func TestProgramZoneSchemaEnforcesOneAssignmentPerProgramRegency(t *testing.T) {
    // Insert one program, one regency, and two zones.
    // The first assignment succeeds; the second assignment for the same
    // (program_id, regency_id) must fail with a unique violation.
}

func TestProgramDocumentProfileVersionIsUniquePerProgramVersion(t *testing.T) {
    // Two profile rows with the same (program_id, version) must conflict.
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `go test ./internal/programs/... -run 'TestProgramZoneSchema|TestProgramDocumentProfileVersion' -v`

Expected: FAIL because the new relations do not exist.

- [ ] **Step 3: Add migration `00020`**

Create the tables with database constraints matching the spec. Backfill one `ZONA BELUM DIATUR` per existing program and insert one assignment for every distinct `(program_id, regency_id)` already referenced by `program_schedules`. Add `program_id uuid REFERENCES programs(id)` to `activity_media` as nullable for legacy rows. Add indexes for `(program_id, sort_order)`, `(zone_id, regency_id)`, published profiles, and logo ordering.

Use an immutable-profile trigger:

```sql
CREATE FUNCTION reject_published_program_document_profile_change() RETURNS trigger AS $$
BEGIN
  IF OLD.status = 'published' THEN
    RAISE EXCEPTION 'published document profile is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

The down migration removes the trigger/function, new columns, and new tables in dependency order without touching the original tables.

- [ ] **Step 4: Run schema tests**

Run: `go test ./internal/programs/... -run 'TestProgramZoneSchema|TestProgramDocumentProfileVersion' -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/database/migrations/00020_program_zones_drive_profiles.sql internal/programs/repository_integration_test.go internal/programs/schema_integration_test.go
git commit -m "feat(programs): add zones and document profile schema"
```

### Task 2: Implement zone domain validation and persistence

**Files:**
- Modify: `internal/programs/models.go`
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`
- Modify: `internal/programs/repository.go`
- Modify: `internal/programs/repository_integration_test.go`

**Interfaces:**
- Consumes: tables from Task 1 and existing `auth.Principal`, `auth.ClientMeta`, `auth.RegencyScope`.
- Produces: `ListZones(ctx, programID, scope)`, `SaveZone(...)`, `AssignRegency(...)`, and `ResolveStorageContext(ctx, programID, regencyID, scope)`.

- [ ] **Step 1: Add failing service tests**

Define and test these contracts:

```go
type ProgramZone struct {
    ID, ProgramID, Code, Name string
    SortOrder int
    IsPlaceholder bool
    Regencies []Regency
}

type ZoneInput struct { ID, ProgramID, Code, Name string; SortOrder int }
type RegencyAssignmentInput struct { ProgramID, RegencyID, ZoneID string }
type StorageContext struct { ProgramID string; ProgramType ProgramType; ZoneID, ZoneName, RegencyID, RegencyName string }
```

Tests must reject blank names, unstable codes, a zone from another program, an out-of-scope regency, and storage resolution through `ZONA BELUM DIATUR` with `ErrZoneNotConfigured`.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/programs -run 'Test.*Zone|TestResolveStorageContext' -v`

Expected: FAIL because the types and methods are missing.

- [ ] **Step 3: Implement models and service validation**

Add `ErrZoneNotConfigured`, normalize zone code to uppercase, trim names, accept `sort_order >= 0`, and extend the private repository interface with the four methods. Keep scope checking in repository queries, not only handlers.

- [ ] **Step 4: Implement repository transactions and audit events**

Use `INSERT ... ON CONFLICT (program_id, code) DO UPDATE` for new zones only while the target is not a placeholder. Assignment uses one transaction and records `program.zone_saved` or `program.regency_assigned`. `ResolveStorageContext` joins programs, assignments, zones, and regencies and returns `ErrZoneNotConfigured` for the placeholder.

- [ ] **Step 5: Run unit and integration tests**

Run: `go test ./internal/programs/... -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/programs
git commit -m "feat(programs): manage zones and regency assignments"
```

### Task 3: Expose zone APIs with permissions and scope

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/program_routes.go`
- Modify: `internal/api/handler_test.go`
- Create: `internal/api/program_routes_test.go`

**Interfaces:**
- Consumes: Task 2 service methods.
- Produces: REST endpoints under `/api/v1/program-setup/programs/{programID}/zones` and `/assignments`.

- [ ] **Step 1: Add failing route tests**

Cover:

```text
GET   /api/v1/program-setup/programs/{programID}/zones
POST  /api/v1/program-setup/programs/{programID}/zones
PATCH /api/v1/program-setup/programs/{programID}/zones/{zoneID}
PUT   /api/v1/program-setup/programs/{programID}/assignments/{regencyID}
```

GET requires `programs.view`; mutations require `programs.manage`. Tests must prove an out-of-scope assignment returns 404/forbidden semantics without disclosing the regency.

- [ ] **Step 2: Run route tests and confirm failure**

Run: `go test ./internal/api -run 'Test.*ProgramZone|Test.*RegencyAssignment' -v`

Expected: FAIL with endpoint not found or missing fake-service methods.

- [ ] **Step 3: Extend the handler dependency interface and route parser**

Route nested program resources explicitly rather than stretching the existing maximum-two-parts parser. Decode JSON with existing helpers, pass `clientMeta`, and map `ErrZoneNotConfigured`/`ErrInvalidInput` through `writeServiceError`.

- [ ] **Step 4: Run API tests**

Run: `go test ./internal/api -run 'Test.*ProgramZone|Test.*RegencyAssignment' -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/api/handler.go internal/api/program_routes.go internal/api/handler_test.go internal/api/program_routes_test.go
git commit -m "feat(api): expose program zone management"
```

### Task 4: Centralize and sanitize storage folder paths

**Files:**
- Create: `internal/media/folder_path.go`
- Create: `internal/media/folder_path_test.go`
- Modify: `internal/media/storage.go`
- Modify: `internal/media/storage_test.go`
- Modify: `internal/media/gdrive.go`
- Modify: `internal/media/gdrive_test.go`

**Interfaces:**
- Consumes: plain program type, zone name, regency name, category, and optional child folder.
- Produces: `BuildFolderPath(input FolderPathInput) ([]string, error)`, `Storage.EnsureFolders(ctx, paths)`, and exact reserved folder names.

- [ ] **Step 1: Write failing path tests**

Define:

```go
type FolderCategory string
const (
    FolderBA FolderCategory = "BERITA ACARA (BA)"
    FolderSupporting FolderCategory = "DOKUMEN PENDUKUNG"
    FolderPhotos FolderCategory = "DOKUMENTASI (FOTO)"
)
type FolderPathInput struct {
    ProgramType, ZoneName, RegencyName string
    Category FolderCategory
    Child string
}
```

Assert `farmer` maps to `PETANI`, `fisherman` to `NELAYAN`, and names such as `Zona / Timur\\A` are rejected rather than split into extra path levels. Whitespace is trimmed/collapsed. Empty required fields return `ErrInvalidFolderPath`.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/media -run 'TestBuildFolderPath|TestGoogleDriveStoragePutCreatesReserved' -v`

Expected: FAIL because the builder and new hierarchy do not exist.

- [ ] **Step 3: Implement the builder and revise reserved-folder creation**

The builder returns:

```go
[]string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "RAKOR"}
```

Remove positional assumptions that path index 1 is a regency. Extend `Storage` with `EnsureFolders(ctx context.Context, paths [][]string) error`; local storage validates/no-ops and Google Drive resolves each supplied path idempotently. Do not auto-create siblings by guessing path depth.

- [ ] **Step 4: Test local and Drive adapters**

Run: `go test ./internal/media/... -v`

Expected: PASS, including stale-cache behavior already covered by existing tests.

- [ ] **Step 5: Commit**

```powershell
git add internal/media/folder_path.go internal/media/folder_path_test.go internal/media/storage.go internal/media/storage_test.go internal/media/gdrive.go internal/media/gdrive_test.go
git commit -m "refactor(media): centralize program storage paths"
```

### Task 5: Attach program context to activity documentation

**Files:**
- Modify: `internal/activities/models.go`
- Modify: `internal/activities/repository.go`
- Modify: `internal/activities/repository_integration_test.go`
- Modify: `internal/activities/service.go`
- Modify: `internal/activities/service_test.go`
- Modify: `internal/api/activities_routes_test.go`
- Modify: `cmd/server/main.go`
- Modify: `frontend/src/features/activities/types.ts`
- Modify: `frontend/src/features/activities/ActivityDocumentationPage.tsx`
- Modify: `frontend/src/features/activities/ActivityDocumentationPage.test.tsx`

**Interfaces:**
- Consumes: `programs.ResolveStorageContext` data through an activities-local narrow resolver interface and `media.BuildFolderPath`.
- Produces: activity uploads carrying `program_id`; legacy rows remain readable.

- [ ] **Step 1: Write failing backend tests**

Change upload input to include `ProgramID string`. Assert upload rejects missing program, rejects placeholder zone before calling `Storage.Put`, and produces the exact path `PETANI/ZONA 1/KABUPATEN WAJO/DOKUMENTASI (FOTO)/RAKOR`. Add a repository test proving a legacy row with `program_id IS NULL` still lists and opens.

- [ ] **Step 2: Run failing tests**

Run: `go test ./internal/activities ./internal/api -run 'Test.*Activity.*Program|Test.*LegacyActivity' -v`

Expected: FAIL because activity input/repository has no program context.

- [ ] **Step 3: Implement backend changes**

Persist `program_id` for new rows, join program context for listings, and replace the current `Konkit {year}/{regency}/DOKUMENTASI FOTO & VIDEO` path with the builder output. Inject the narrow program-context resolver when wiring `activities.Service` in `cmd/server/main.go`. Keep `ContentURL` unchanged.

- [ ] **Step 4: Write and run failing frontend tests**

Assert the page requires a program selection before upload and sends `{ program_id, regency_id, activity_type, ... }`. Run: `npm test -- --run frontend/src/features/activities/ActivityDocumentationPage.test.tsx` from `frontend` using the repository's established test command syntax.

- [ ] **Step 5: Implement frontend program context**

Reuse the schedules/program query pattern; derive available regencies from the selected program and preserve current gallery behavior.

- [ ] **Step 6: Run backend and frontend tests**

Run: `go test ./internal/activities ./internal/api -v`

Run from `frontend`: `npm test -- --run src/features/activities/ActivityDocumentationPage.test.tsx`

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/activities internal/api/activities_routes_test.go frontend/src/features/activities cmd/server/main.go
git commit -m "feat(activities): organize uploads by program zone"
```

### Task 6: Route distribution documentation through the new hierarchy

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: schedule-to-program/zone/regency context and `media.BuildFolderPath`.
- Produces: new distribution media uploads under `DOKUMENTASI (FOTO)/PENDISTRIBUSIAN`.

- [ ] **Step 1: Add failing service tests**

Extend the media-slot repository result with program type, zone, and regency names. Assert `Storage.Put` receives:

```go
[]string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "PENDISTRIBUSIAN"}
```

Also assert placeholder-zone upload fails before storage is called.

- [ ] **Step 2: Run focused tests**

Run: `go test ./internal/distribution -run 'Test.*Upload.*Folder|Test.*ZoneNotConfigured' -v`

Expected: FAIL because `folderPath` is currently `nil`.

- [ ] **Step 3: Implement the joined context and folder path**

Join through `program_schedules`, `program_regency_assignments`, `program_zones`, `programs`, and `regencies`. Keep accepted MIME/size/checksum behavior unchanged.

- [ ] **Step 4: Run distribution tests**

Run: `go test ./internal/distribution/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/distribution
git commit -m "feat(distribution): store documentation in zone hierarchy"
```

### Task 7: Implement versioned document profiles and logo assets

**Files:**
- Modify: `internal/programs/models.go`
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`
- Modify: `internal/programs/repository.go`
- Modify: `internal/programs/repository_integration_test.go`
- Modify: `internal/api/program_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: profile/logo tables from Task 1 and `media.Storage`.
- Produces: draft profile CRUD, logo upload/reorder/hide, publish, and asset streaming endpoints.

- [ ] **Step 1: Write failing profile tests**

Define `DocumentProfile`, `DocumentLogo`, and `DocumentProfileInput`. Tests must prove version 1 creation, publishing, immutable published updates, version 2 creation from version 1, PNG/JPEG-only logo validation, stable order, and cleanup of uploaded storage content if repository insertion fails.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/programs ./internal/api -run 'Test.*DocumentProfile|Test.*DocumentLogo' -v`

Expected: FAIL because profile services and routes do not exist.

- [ ] **Step 3: Implement service/repository methods**

Use a narrow asset storage dependency on `programs.Service` and wire it from the existing storage instance in `cmd/server/main.go`. Logo files are stored under a system path derived from `PROGRAM ASSETS/{program code}/DOCUMENT PROFILE V{version}` rather than the regency hierarchy. On publish, validate at least one visible logo and non-empty title, procurement description, and document series.

- [ ] **Step 4: Implement routes**

Add list/create/update/publish endpoints plus multipart logo upload and authenticated content streaming. Mutations require `programs.manage`; reads require `programs.view` or `bast.view` where needed by the BA screen.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/programs ./internal/api -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/programs internal/api/program_routes.go internal/api/handler.go internal/api/handler_test.go cmd/server/main.go
git commit -m "feat(programs): manage versioned document profiles"
```

### Task 8: Add Zona and Document Profile panels to Persiapan Program

**Files:**
- Modify: `frontend/src/features/programs/types.ts`
- Modify: `frontend/src/features/programs/ProgramSetupPage.tsx`
- Modify: `frontend/src/features/programs/ProgramSetupPage.test.tsx`
- Create: `frontend/src/features/programs/ZonesPanel.tsx`
- Create: `frontend/src/features/programs/ZonesPanel.test.tsx`
- Create: `frontend/src/features/programs/DocumentProfilePanel.tsx`
- Create: `frontend/src/features/programs/DocumentProfilePanel.test.tsx`
- Modify: `frontend/src/features/programs/ProgramSetup.module.css`

**Interfaces:**
- Consumes: Tasks 3 and 7 APIs.
- Produces: accessible UI for zone membership and tender document configuration.

- [ ] **Step 1: Add failing page and panel tests**

Tests cover URL-backed tabs `zones` and `document-profile`, placeholder-zone warnings, assignment mutation payloads, logo ordering, hide/show, publish confirmation, retry states, and a visible Petani/Nelayan program badge.

- [ ] **Step 2: Run failing frontend tests**

Run from `frontend`: `npm test -- --run src/features/programs/ProgramSetupPage.test.tsx src/features/programs/ZonesPanel.test.tsx src/features/programs/DocumentProfilePanel.test.tsx`

Expected: FAIL because panels and types do not exist.

- [ ] **Step 3: Implement types and panels**

Follow existing `ProgramsPanel`/`SchedulesPanel` patterns, use TanStack Query invalidation after mutations, keep one modal/dialog responsibility per component, and expose loading/empty/error/retry states. Logo controls use buttons with accessible labels and do not rely on drag-only interaction.

- [ ] **Step 4: Run frontend quality gates**

Run from `frontend`:

```powershell
npm test -- --run src/features/programs
npm run typecheck
npm run lint
```

Expected: tests and typecheck PASS; lint has no new errors or warnings.

- [ ] **Step 5: Commit**

```powershell
git add frontend/src/features/programs
git commit -m "feat(frontend): configure zones and document profiles"
```

### Task 9: Verify the foundation end to end

**Files:**
- Modify only files required by failures found during verification.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: a stable foundation for the BA Perorangan plan.

- [ ] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run frontend tests and build**

Run from `frontend`:

```powershell
npm test -- --run
npm run typecheck
npm run lint
npm run build
```

Expected: all tests/typecheck/build PASS and no new lint findings.

- [ ] **Step 3: Verify migrations against a test PostgreSQL database**

Run the repository's integration-test command with `TEST_DATABASE_URL` configured. Confirm migration up/down/up works and old schedules are assigned to `ZONA BELUM DIATUR`.

- [ ] **Step 4: Verify folder behavior with the fake Drive API**

Run: `go test ./internal/media ./internal/activities ./internal/distribution -v`

Expected: exact new paths PASS, legacy content remains readable, and no test expects the old `Konkit {year}` path.

- [ ] **Step 5: Commit verification fixes if any**

Stage only verified fixes and commit with `fix: stabilize program zone and drive foundation`. If no files changed, do not create an empty commit.
