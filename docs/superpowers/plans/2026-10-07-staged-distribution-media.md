# Staged Distribution Media Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow POS Mesin to capture durable media before a distribution date exists, then reliably move that media from Google Drive staging to the date/slot folder after POS Dokumen supplies both the date and recipient.

**Architecture:** Distribution media uploads use a persistent `_PENDING` Drive path until a slot has both `distribution_date` and `allocation_id`. Database-backed move jobs drive an idempotent worker that changes the Google Drive parent without re-uploading; job generations make date changes safe during concurrent moves. Existing revision, recompletion, and stale-BA behavior remains intact while frontend POS ownership is updated.

**Tech Stack:** Go 1.x, PostgreSQL/pgx, Goose SQL migrations, Google Drive API v3, React 19, TypeScript, TanStack Query, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-07-staged-distribution-media-design.md`

## Global Constraints

- Production staging is Google Drive; never use RAM or local VPS disk as the durable staging source.
- Never delete media or empty Drive folders automatically.
- Preserve existing commits `25561bd` through `a59c40a`: revision metadata, POS authorization, edit/replace recipient, reopen, and stale BA history remain valid.
- The interrupted Task 7 files currently staged in the worktree must be preserved, unstaged before backend commits, adapted in Task 7, and never discarded.
- Uploads remain streaming with image limit 25 MiB and video limit 500 MiB.
- The final Drive path remains `{PROGRAM}/{ZONE}/{REGENCY}/DOKUMENTASI (FOTO)/PENDISTRIBUSIAN/{DATE}/{SLOT_NUMBER}` and existing folders are reused.
- `distribution_date` is owned by `distribution.pos_dokumen`; machine media remains owned by `distribution.pos_mesin`.
- Media must remain previewable from every non-deleted storage state.
- A slot cannot complete until every accepted media file belonging to a required documentation slot has `storage_state='final'`.
- All new mutation paths must preserve regency scope, audit metadata, and completed-slot revision rules.

## Existing Worktree Reconciliation

Before Task 1, preserve but unstage the interrupted frontend work so backend commits cannot accidentally include it:

```powershell
git restore --staged frontend/src/features/distribution/types.ts frontend/src/features/distribution/RevisionDialog.tsx frontend/src/features/distribution/RevisionDialog.test.tsx frontend/src/features/distribution/DocumentationSlot.tsx frontend/src/features/distribution/DocumentationSlot.test.tsx
git status --short
```

Expected: those five paths remain modified/untracked in the working tree; no product file is deleted. Record the baseline commit (`be5b1ce`) in the execution ledger.

## Review Focus

- A 500 MiB video uploaded before date/recipient exists must remain streaming and land in staging without whole-file buffering; Task 3 adds the test.
- A date change while a move is in flight must end at only the newest target folder; Task 4 adds a generation-race integration test.
- A Drive move that succeeds before the process crashes must be safe to repeat and must not duplicate the file; Task 5 adds an idempotent storage/worker test.
- A worker process that dies after claiming a job must leave a reclaimable job for another instance after its lease expires; Task 5 adds the lease recovery test.
- Deleting media while a move is queued must cancel/ignore the job without restoring or moving the deleted file; Task 4 adds the transaction test.

---

### Task 1: Add nullable dates and durable media-move schema

**Files:**
- Create: `internal/database/migrations/00044_staged_distribution_media.sql`
- Create: `internal/database/migrations/00044_staged_distribution_media_test.go`
- Modify: `internal/distribution/schema_integration_test.go`

**Interfaces:**
- Produces: nullable `distribution_slots.distribution_date`; `media_files.storage_state`, `storage_last_error`, `storage_target_generation`; table `distribution_media_move_jobs` keyed by `media_file_id`.
- Consumes: migration `00043_distribution_slot_revisions.sql` and existing `media_files.status` content lifecycle.

- [ ] **Step 1: Write failing migration tests**

Add tests that migrate through `00044`, create a slot with `distribution_date=NULL`, verify existing media backfills to `storage_state='final'`, reject invalid storage/job states, and enforce one job per media file. Use the exact states below:

```sql
storage_state IN ('staging','moving','final','move_failed')
job status IN ('queued','processing','retry')
```

- [ ] **Step 2: Run migration tests and verify RED**

Run: `go test ./internal/database/migrations ./internal/distribution -run 'TestStagedDistributionMediaSchema|TestDistributionSchema' -count=1`

Expected: FAIL because migration `00044` and the new columns/table do not exist.

- [ ] **Step 3: Implement migration `00044`**

The Up migration must:

```sql
ALTER TABLE distribution_slots ALTER COLUMN distribution_date DROP NOT NULL;
ALTER TABLE distribution_slots ALTER COLUMN distribution_date DROP DEFAULT;

ALTER TABLE media_files
  ADD COLUMN storage_state text NOT NULL DEFAULT 'final'
    CHECK (storage_state IN ('staging','moving','final','move_failed')),
  ADD COLUMN storage_last_error text NOT NULL DEFAULT '',
  ADD COLUMN storage_target_generation bigint NOT NULL DEFAULT 0 CHECK (storage_target_generation >= 0);

CREATE TABLE distribution_media_move_jobs (
  media_file_id uuid PRIMARY KEY REFERENCES media_files(id) ON DELETE CASCADE,
  target_path text[] NOT NULL CHECK (cardinality(target_path) > 0),
  target_generation bigint NOT NULL CHECK (target_generation > 0),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','processing','retry')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  locked_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX distribution_media_move_jobs_ready_idx
  ON distribution_media_move_jobs (next_attempt_at, updated_at)
  WHERE status IN ('queued','retry','processing');
CREATE INDEX media_files_storage_state_idx ON media_files (storage_state, documentation_slot_id)
  WHERE status='accepted';
```

The Down migration must drop the job table/columns and restore a non-null date only after filling null dates with the Jakarta current date. It must never touch Drive files.

- [ ] **Step 4: Run schema tests and verify GREEN**

Run: `go test ./internal/database/migrations ./internal/distribution -run 'TestStagedDistributionMediaSchema|TestDistributionSchema' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/database/migrations/00044_staged_distribution_media.sql internal/database/migrations/00044_staged_distribution_media_test.go internal/distribution/schema_integration_test.go
git commit -m "feat: add staged distribution media schema"
```

### Task 2: Add idempotent storage move support

**Files:**
- Modify: `internal/media/storage.go`
- Modify: `internal/media/storage_test.go`
- Modify: `internal/media/gdrive.go`
- Modify: `internal/media/gdrive_test.go`

**Interfaces:**
- Produces: `type MovableStorage interface { Storage; Move(context.Context, string, []string) error }`.
- Produces: Google Drive `moveFile(ctx, fileID, targetParentID string) error` adapter behavior.
- Consumes: existing folder resolver/cache and stable `storage_key` file identity.

- [ ] **Step 1: Write failing storage tests**

Add tests for:

```go
func TestGoogleDriveMoveResolvesTargetAndChangesParent(t *testing.T)
func TestGoogleDriveMoveIsIdempotentWhenAlreadyInTarget(t *testing.T)
func TestLocalStorageMoveValidatesTargetAndKeepsStableKey(t *testing.T)
```

The fake Drive API must expose current parents and count update calls. The second test expects zero duplicate/copy calls when the target parent is already present.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/media -run 'Test.*Move' -count=1`

Expected: FAIL because `MovableStorage` and Drive move operations do not exist.

- [ ] **Step 3: Implement move contracts**

Add:

```go
type MovableStorage interface {
    Storage
    Move(ctx context.Context, storageKey string, targetPath []string) error
}
```

Extend `driveFilesAPI` with parent lookup and parent update. `GoogleDriveStorage.Move` resolves `targetPath`, returns success if that folder is already a parent, otherwise updates parents in one Drive API operation. `LocalStorage.Move` validates `targetPath` and returns success without changing the flat stable key.

- [ ] **Step 4: Run media tests and verify GREEN**

Run: `go test ./internal/media -count=1`

Expected: PASS, including existing streaming upload tests.

- [ ] **Step 5: Commit**

```powershell
git add internal/media/storage.go internal/media/storage_test.go internal/media/gdrive.go internal/media/gdrive_test.go
git commit -m "feat: support idempotent media moves"
```

### Task 3: Create slots without dates and upload to staging safely

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/pos_mesin_test.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository_integration_test.go`
- Modify: `internal/media/folder_path.go`
- Modify: `internal/media/folder_path_test.go`

**Interfaces:**
- Produces: `DistributionSlot.DistributionDate *string`; `MediaSlot.DistributionDate *string`, `ScheduleID string`, `HasRecipient bool`; `MediaFile.StorageState string`, `StorageLastError string`.
- Produces: `BuildDistributionStagingPath(base []string, scheduleID string, slotNumber int) ([]string,error)` and `BuildDistributionFinalPath(base []string, date time.Time, slotNumber int) ([]string,error)`.
- Consumes: schema Task 1 and `media.MovableStorage` Task 2.

- [ ] **Step 1: Write failing domain/repository tests**

Replace the old “create requires date” assertion with:

```go
func TestCreateSlotDoesNotAcceptOrRequireDistributionDate(t *testing.T)
func TestUploadMachineMediaWithoutDateUsesPersistentStagingPath(t *testing.T)
func TestUploadReadySlotUsesFinalPathAndFinalState(t *testing.T)
func TestUploadLargeVideoToStagingRemainsStreaming(t *testing.T)
```

The staging path assertion must end with `PENDISTRIBUSIAN/_PENDING/<schedule-id>/<slot-number>`. The large-video test uses a counting reader and must prove service code does not call `io.ReadAll` or allocate the declared 500 MiB.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/distribution ./internal/media -run 'TestCreateSlotDoesNot|TestUpload.*Staging|TestUploadReady|TestUploadLarge' -count=1`

Expected: FAIL because create validates dates and upload parses a mandatory date.

- [ ] **Step 3: Implement nullable date and path selection**

Change the model contract:

```go
type CreateSlotInput struct {
    ScheduleID string `json:"schedule_id"`
    SlotNumber int    `json:"slot_number"`
}
type DistributionSlot struct {
    // existing fields
    DistributionDate *string `json:"distribution_date"`
}
```

`CreateSlot` inserts NULL date. Upload computes the common base path once; if both date and recipient exist, upload directly to the final path with `storage_state='final'`, otherwise upload to staging with `storage_state='staging'`. Keep `io.LimitReader`, video concurrency limiting, orphan cleanup on DB failure, and content URLs unchanged.

- [ ] **Step 4: Run distribution/media tests and verify GREEN**

Run: `go test ./internal/distribution ./internal/media -count=1 -p 1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/distribution internal/media/folder_path.go internal/media/folder_path_test.go
git commit -m "feat: stage distribution media before dating"
```

### Task 4: Queue moves atomically from date, recipient, upload, and delete flows

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/repository_integration_test.go`
- Modify: `internal/distribution/distribution_date_test.go`
- Modify: `internal/distribution/pos_dokumen_test.go`

**Interfaces:**
- Produces: repository helper `queueMediaMovesForSlot(ctx, tx, distributionSlotID string) error`.
- Produces: `RetryMediaMove(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (MediaFile,error)` repository/service contract, where the string is `mediaID`.
- Consumes: final path builder from Task 3 and job schema from Task 1.

- [ ] **Step 1: Write failing transaction tests**

Add integration tests proving:

- date-first leaves no job until linking, then queues every accepted staging file;
- recipient-first leaves no job until date update, then queues the same files;
- an upload to an already-ready slot is final and creates no move job;
- changing a date increments `storage_target_generation`, sets media `moving`, and upserts the latest target;
- changing a date while a generation-N job is processing leaves generation N+1 queued;
- deleting media cascades/removes its queued job and no retry can resurrect it.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/distribution -run 'TestIntegrationMediaMoveQueue|TestIntegrationDateChangeDuringMove|TestIntegrationDeleteQueuedMedia' -count=1 -p 1`

Expected: FAIL because mutations do not enqueue media moves.

- [ ] **Step 3: Implement queue helper and mutation hooks**

Within the same DB transaction as date/link/replace:

```sql
UPDATE media_files m
SET storage_state='moving', storage_last_error='',
    storage_target_generation=storage_target_generation+1, updated_at=now()
FROM documentation_slots ds
WHERE m.documentation_slot_id=ds.id
  AND ds.distribution_slot_id=$1 AND m.status='accepted';
```

Upsert each job with the generated final target and newest generation. Call the helper after `SetDistributionDate`, `LinkSlot`, and `ReplaceRecipient`; `SaveMedia` inserts a job only when a race made the slot ready between path selection and metadata commit. `DeleteMedia` relies on the FK cascade and never requeues deleted content.

- [ ] **Step 4: Run integration tests and verify GREEN**

Run: `go test ./internal/distribution -count=1 -p 1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/distribution
git commit -m "feat: queue distribution media relocation"
```

### Task 5: Process move jobs with retry, leases, and restart safety

**Files:**
- Create: `internal/distribution/media_move_worker.go`
- Create: `internal/distribution/media_move_worker_test.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/models.go`
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`

**Interfaces:**
- Produces: `NewMediaMoveWorker(repository MediaMoveRepository, storage media.MovableStorage, options MediaMoveWorkerOptions) *MediaMoveWorker`.
- Produces: `Run(context.Context)` and `ProcessOne(context.Context) (bool,error)`.
- Consumes: durable jobs from Task 4 and move storage from Task 2.

Define the worker boundary exactly as:

```go
type MediaMoveJob struct {
    MediaFileID string
    StorageKey string
    TargetPath []string
    TargetGeneration int64
    Attempts int
}
type MediaMoveRepository interface {
    ClaimMediaMove(context.Context, time.Time, time.Duration) (MediaMoveJob, bool, error)
    CompleteMediaMove(context.Context, string, int64) error
    FailMediaMove(context.Context, string, int64, string, time.Time) error
}
type MediaMoveWorkerOptions struct {
    PollInterval time.Duration
    LeaseDuration time.Duration
    MaxBackoff time.Duration
}
```

- [ ] **Step 1: Write failing worker tests**

Use fake repository/storage clocks to add:

```go
func TestMediaMoveWorkerMarksNewestGenerationFinal(t *testing.T)
func TestMediaMoveWorkerRetriesDriveFailureWithoutLosingFile(t *testing.T)
func TestMediaMoveWorkerRepeatsAlreadyCompletedDriveMoveAfterCrash(t *testing.T)
func TestMediaMoveWorkerReclaimsExpiredProcessingLease(t *testing.T)
func TestMediaMoveWorkerLeavesNewerTargetQueued(t *testing.T)
```

The crash test makes `Move` succeed and repository completion fail once; the second run must call an idempotent move and finish without creating another file.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/distribution -run 'TestMediaMoveWorker' -count=1`

Expected: FAIL because the worker does not exist.

- [ ] **Step 3: Implement repository leases and worker**

Use `FOR UPDATE SKIP LOCKED` to claim one queued/retry job or a processing job whose `locked_at` is older than the lease. Process Drive outside the claim transaction. Completion updates media to `final` and deletes the job only when its target generation still matches; otherwise it returns the job to `queued`. Failure sets media `move_failed`, job `retry`, increments attempts, and calculates capped exponential backoff.

Use defaults:

```go
MediaMoveWorkerOptions{
    PollInterval: 5 * time.Second,
    LeaseDuration: 5 * time.Minute,
    MaxBackoff: 15 * time.Minute,
}
```

Start the worker in `run` only when configured storage satisfies `media.MovableStorage`. Cancel it through the server root context and do not block HTTP shutdown waiting on a poll sleep.

- [ ] **Step 4: Run worker/server tests and verify GREEN**

Run: `go test ./internal/distribution ./cmd/server -count=1 -p 1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/distribution/media_move_worker.go internal/distribution/media_move_worker_test.go internal/distribution/repository.go internal/distribution/models.go cmd/server/main.go cmd/server/main_test.go
git commit -m "feat: process durable media move jobs"
```

### Task 6: Enforce POS Dokumen dates, retry API, and final-media completion

**Files:**
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/pos_penyerahan_test.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `internal/api/routes.go`

**Interfaces:**
- Produces: `POST /api/v1/distribution/media/{id}/retry-move` guarded by `distribution.pos_dokumen`.
- Produces: date PATCH guarded by `distribution.pos_dokumen`.
- Produces: `ErrMediaMovePending` and `ErrMediaMoveFailed` mapped to HTTP 409.
- Consumes: `RetryMediaMove` Task 4 and media states Task 1.

- [ ] **Step 1: Write failing API/service tests**

Add tests that POS Mesin receives 403 for date PATCH, POS Dokumen succeeds, retry rejects cross-regency/deleted media, and complete returns distinct 409 messages for `moving` and `move_failed`. Preserve the existing completed-slot requirement to reopen before changing dates.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/api ./internal/distribution -run 'Test.*DistributionDate|Test.*RetryMediaMove|TestComplete.*MediaMove' -count=1 -p 1`

Expected: FAIL on old permission and absent retry/completion rules.

- [ ] **Step 3: Implement routes and completion validation**

Change `handleDistributionDateUpdate` authorization to `distribution.pos_dokumen`. Add retry routing after stage lookup/scope validation. In `CompleteSlot`, reject required accepted media whose `storage_state <> 'final'`; return failed before pending so users see the actionable Drive error.

- [ ] **Step 4: Run API/distribution suites and verify GREEN**

Run: `go test ./internal/api ./internal/distribution -count=1 -p 1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/api internal/distribution
git commit -m "feat: enforce staged media completion rules"
```

### Task 7: Reconcile frontend revision primitives and media states

**Files:**
- Modify: `frontend/src/features/distribution/types.ts`
- Modify: `frontend/src/features/distribution/RevisionDialog.tsx`
- Modify: `frontend/src/features/distribution/RevisionDialog.test.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.test.tsx`

**Interfaces:**
- Produces: `RevisionDialog({slot,stage,open,onOpenChange,onReopened})` from the interrupted work, adapted to nullable dates.
- Produces: `DocumentationSlot({slot,canManage,onChanged,onRetryMove})` and visible storage-state copy.
- Consumes: API media state/retry Task 6.

- [ ] **Step 1: Audit and adapt the preserved interrupted files**

Do not recreate or discard them. Change TypeScript contracts to:

```ts
type MediaStorageState = 'staging' | 'moving' | 'final' | 'move_failed'
type MediaFile = {
  id: string; slot_id: string; original_filename: string; mime_type: string;
  byte_size: number; source: string; status: string; content_url: string;
  captured_at?: string; storage_state: MediaStorageState; storage_last_error?: string;
}
type DistributionSlot = {
  id: string; schedule_id: string; slot_number: number;
  status: 'open' | 'linked' | 'completed' | 'cancelled';
  distribution_date: string | null; allocation_id?: string; full_name?: string; nik?: string;
  sector_identifier?: string; address?: string; village?: string; district?: string; phone_number?: string;
  machine_option_code?: string; machine_serial_number?: string;
  hose_option_code?: string; hose_serial_number?: string;
  converter_option_code?: string; converter_serial_number?: string;
  documentation: SlotSummary[]; distributed_at?: string; needs_recompletion: boolean;
  reopened_at?: string; reopened_by?: string; reopened_stage?: RevisionStage; revision_reason?: string;
  created_at: string; updated_at: string;
}
type CreateSlotInput = { schedule_id: string; slot_number?: number }
```

- [ ] **Step 2: Write failing media-state tests**

Extend `DocumentationSlot.test.tsx` to assert exact copy for all four states, preview availability in each state, retry only for `move_failed`, and no upload/delete controls when `canManage=false` even if global documentation permission exists. Keep the RevisionDialog whitespace, payload, callback, and API-error tests.

- [ ] **Step 3: Run focused tests and verify RED**

Run: `npm.cmd test -- --run src/features/distribution/RevisionDialog.test.tsx src/features/distribution/DocumentationSlot.test.tsx` (working directory `frontend`)

Expected: FAIL until storage-state presentation/retry is implemented.

- [ ] **Step 4: Implement storage-state UI**

Use the approved labels: `Tersimpan sementara`, `Menunggu tanggal dan penerima`, `Sedang dipindahkan`, `Tersimpan di folder final`, and `Pemindahan gagal — akan dicoba kembali`. Keep upload streaming/progress behavior. Retry calls `/api/v1/distribution/media/${id}/retry-move` and surfaces `ApiError.message` in `role="alert"`.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run: `npm.cmd test -- --run src/features/distribution/RevisionDialog.test.tsx src/features/distribution/DocumentationSlot.test.tsx` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/features/distribution/types.ts frontend/src/features/distribution/RevisionDialog.tsx frontend/src/features/distribution/RevisionDialog.test.tsx frontend/src/features/distribution/DocumentationSlot.tsx frontend/src/features/distribution/DocumentationSlot.test.tsx
git commit -m "feat: show staged distribution media status"
```

### Task 8: Make POS Mesin documentation-only

**Files:**
- Modify: `frontend/src/features/distribution/SlotMesinCreate.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinCreate.test.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinSection.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinSection.test.tsx`

**Interfaces:**
- Consumes: nullable slot/types and `DocumentationSlot.canManage` Task 7.
- Produces: POS Mesin create payload `{schedule_id,slot_number?}` with no date/equipment.

- [ ] **Step 1: Write failing POS Mesin tests**

Assert the create form sends only schedule/optional slot number; neither create nor existing section renders date/equipment/penerima fields; machine documentation remains uploadable with `distribution.pos_mesin`; completed slots are read-only until `RevisionDialog` stage `mesin` succeeds.

- [ ] **Step 2: Run tests and verify RED**

Run: `npm.cmd test -- --run src/features/distribution/SlotMesinCreate.test.tsx src/features/distribution/SlotMesinSection.test.tsx` (working directory `frontend`)

Expected: FAIL against the old date/equipment UI.

- [ ] **Step 3: Simplify POS Mesin**

Remove date picker, equipment selectors, serial fields, and scanner state. Pass `canManage={canManage && slot.status !== 'completed'}` to stage-machine documentation. Render the revision button for completed slots and update local slot state through `onReopened`.

- [ ] **Step 4: Run POS Mesin tests and verify GREEN**

Run: `npm.cmd test -- --run src/features/distribution/SlotMesinCreate.test.tsx src/features/distribution/SlotMesinSection.test.tsx` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add frontend/src/features/distribution/SlotMesinCreate.tsx frontend/src/features/distribution/SlotMesinCreate.test.tsx frontend/src/features/distribution/SlotMesinSection.tsx frontend/src/features/distribution/SlotMesinSection.test.tsx
git commit -m "feat: make machine POS documentation only"
```

### Task 9: Move date, recipient, and equipment workflow to POS Dokumen

**Files:**
- Modify: `frontend/src/features/distribution/SlotDokumenSection.tsx`
- Modify: `frontend/src/features/distribution/SlotDokumenSection.test.tsx`

**Interfaces:**
- Consumes: date PATCH/retry APIs Task 6, revision dialog and documentation slot Task 7.
- Produces: POS Dokumen ownership of date, recipient edit/replace, equipment, document media, and relocation status.

- [ ] **Step 1: Write failing POS Dokumen tests**

Cover both orders: save date then mount recipient, and mount then save date. Assert date PATCH, recipient edit/replace, equipment PATCH, document media `canManage`, relocation banner, retry action, completed-slot revision lock, and automatic UI refresh after reopen.

- [ ] **Step 2: Run tests and verify RED**

Run: `npm.cmd test -- --run src/features/distribution/SlotDokumenSection.test.tsx` (working directory `frontend`)

Expected: FAIL because date/equipment/edit/replace are not all owned here.

- [ ] **Step 3: Implement POS Dokumen workflow**

Order the UI as date, recipient, equipment, documentation. Keep NIK immutable in edit and use replacement candidate search for NIK changes. Pass `canManage={canManage && slot.status !== 'completed'}`. Summarize media states across the slot and display pending/failed copy without hiding previews.

- [ ] **Step 4: Run POS Dokumen tests and verify GREEN**

Run: `npm.cmd test -- --run src/features/distribution/SlotDokumenSection.test.tsx` (working directory `frontend`)

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add frontend/src/features/distribution/SlotDokumenSection.tsx frontend/src/features/distribution/SlotDokumenSection.test.tsx
git commit -m "feat: move distribution date to document POS"
```

### Task 10: Finish POS Penyerahan, catalog state, documentation, and full verification

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/repository.go`
- Modify: `frontend/src/features/distribution/SlotPenyerahanSection.tsx`
- Modify: `frontend/src/features/distribution/SlotPenyerahanSection.test.tsx`
- Modify: `frontend/src/features/distribution/SlotCatalogGrid.tsx`
- Modify: `frontend/src/features/distribution/SlotCatalogGrid.test.tsx`
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx`
- Modify: `.env.example`
- Modify: `.env.staging.example`

**Interfaces:**
- Consumes: final-media errors Task 6, revision/recompletion Task 7, POS workflows Tasks 8-9.
- Produces: complete end-to-end staged-media UX and operational worker configuration.

- [ ] **Step 1: Write failing handover/catalog tests**

Assert POS Penyerahan explains moving/failed media and cannot submit, succeeds once all required media is final, passes `canManage` only for stage-penyerahan media, supports completed-slot reopen, and shows `Perlu diselesaikan ulang`. Catalog prioritizes recompletion attention above ordinary linked state and exposes an accessible label.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `npm.cmd test -- --run src/features/distribution/SlotPenyerahanSection.test.tsx src/features/distribution/SlotCatalogGrid.test.tsx src/features/distribution/DistributionPage.test.tsx` (working directory `frontend`)

Expected: FAIL on relocation/recompletion presentation.

- [ ] **Step 3: Implement handover/catalog and environment docs**

Expose aggregate media move state needed by the UI without weakening backend validation. Document worker poll/lease/backoff defaults in both env examples only if made configurable; otherwise document them in comments beside worker options and omit unused environment variables.

- [ ] **Step 4: Run complete verification**

Run: `go test ./... -count=1 -p 1`

Run: `npm.cmd test -- --run` (working directory `frontend`)

Run: `npm.cmd run typecheck` (working directory `frontend`)

Run: `npm.cmd run build` (working directory `frontend`)

Expected: all commands exit 0. Existing chunk-size warnings are acceptable; new errors are not.

- [ ] **Step 5: Inspect migration/order/diff**

Run:

```powershell
git status --short
git diff --check
git log --oneline -15
```

Expected: migration `00044` is latest; no accidental staged remnants from the superseded plan; task commits remain ordered; no whitespace errors.

- [ ] **Step 6: Commit final integration**

```powershell
git add internal/distribution/models.go internal/distribution/repository.go frontend/src/features/distribution/SlotPenyerahanSection.tsx frontend/src/features/distribution/SlotPenyerahanSection.test.tsx frontend/src/features/distribution/SlotCatalogGrid.tsx frontend/src/features/distribution/SlotCatalogGrid.test.tsx frontend/src/features/distribution/DistributionPage.test.tsx .env.example .env.staging.example
git commit -m "feat: complete staged distribution media flow"
```

## Deployment and Manual Smoke Test

Deployment remains a separate, explicitly authorized action after automated verification.

- Apply migration `00044` on staging and verify health.
- Create a slot in POS Mesin without a date and upload both an image and a representative large video.
- Restart the application; verify both media and pending work remain visible.
- In POS Dokumen, test date-first and recipient-first on separate slots.
- Verify the existing final folder is reused and each Drive file ID remains unchanged after move.
- Force a Drive error, verify `move_failed`, preview, retry, and recovery.
- Change a date after final movement and verify automatic relocation to the newest date folder.
- Verify POS Penyerahan cannot complete during moving/failed states and can complete after final.
- Reopen completed slots from each POS, complete again, and regenerate stale BA versions.
- Confirm no staging/final media or empty folder was automatically deleted.
