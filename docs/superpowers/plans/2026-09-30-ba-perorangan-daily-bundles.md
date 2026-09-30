# BA Perorangan Daily Bundles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate immutable Petani BA Perorangan snapshots and synchronize one ordered multi-page PDF per regency and distribution date.

**Architecture:** A new focused `internal/bast` package owns numbering, snapshot creation, PDF rendering, bundle versioning, and storage synchronization. It reads completed distribution slots through scoped repository queries, renders one recipient at a time, appends those pages to a daily document, and swaps the active stored bundle only after the new upload and database update succeed. The existing Berita Acara page becomes a real date/bundle workspace while retaining strict Petani/Nelayan separation.

**Tech Stack:** Go 1.24, PostgreSQL/Goose, `pgx/v5`, `github.com/go-pdf/fpdf`, existing `media.Storage`, React 19, TypeScript, TanStack Query, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-09-30-ba-perorangan-zona-drive-design.md`

## Global Constraints

- Plan `2026-09-30-program-zones-drive-foundation.md` must be complete first.
- One BA record belongs to one completed distribution slot; one daily bundle belongs to one program, regency, local date, and document type.
- `slot_number` is the numerator; the locked final regency total is the denominator.
- Numbering resets per regency, never globally across a tender.
- Each recipient starts on a new page; one recipient may consume multiple pages.
- Drive stores only the daily combined PDF directly in `BERITA ACARA (BA)/2. BA PERORANGAN`, with no date subfolder.
- Filename format is uppercase Indonesian `{HARI}, {DD} {BULAN} {YYYY}.pdf`.
- Petani and Nelayan templates/data are isolated; Nelayan generation remains disabled until configured.
- No signature image or digital-signature workflow is implemented in this plan.

## Review Focus

- A distribution completed around midnight UTC must group by the configured application timezone, not UTC; Task 2 pins local-date conversion.
- Slot numbers `1`, `10`, and `100` must sort numerically and format with schedule padding, never lexicographically; Tasks 2 and 3 pin this.
- A component list long enough to overflow A4 must continue cleanly while the next recipient still starts on a fresh page; Task 4 pins pagination.
- A retry after successful upload but failed database swap must not make the unreferenced file active or delete the previous active bundle; Task 5 pins compensation.
- Two operators finalizing the same date concurrently must produce one active version and an auditable loser/conflict path; Tasks 1 and 5 pin locking and uniqueness.

---

### Task 1: Add BA document and daily-bundle schema

**Files:**
- Create: `internal/database/migrations/00021_bast_individual_documents.sql`
- Create: `internal/bast/schema_integration_test.go`
- Modify: `internal/database/migrations/00019_bast_permissions.sql`

**Interfaces:**
- Consumes: distribution slots, schedules, program/regency assignments, profile versions, users, and permissions.
- Produces: `program_regency_bast_settings`, `bast_individual_documents`, `bast_daily_bundles`, `bast_daily_bundle_items`, and `bast.manage`.

- [ ] **Step 1: Write failing migration tests**

Tests assert unique settings per `(program_id, regency_id)`, one current BA revision per distribution slot/document type, one bundle version per date, one active bundle per date, numeric item ordering, and `bast.manage` granted to `super_admin`.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/bast -run TestBASTSchema -v`

Expected: FAIL because package/tables do not exist.

- [ ] **Step 3: Implement migration**

`program_regency_bast_settings` stores `final_total`, `locked_at`, and `locked_by`. `bast_individual_documents` stores the full document number, local date, profile/template IDs, JSON snapshot, revision, and status. `bast_daily_bundles` stores filename, counts, checksum, storage key, version, status, last error, and sync actor/time. Use partial unique indexes for current/final rows and foreign keys with restrictive deletes for finalized artifacts.

- [ ] **Step 4: Run schema tests**

Run: `go test ./internal/bast -run TestBASTSchema -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/database/migrations/00019_bast_permissions.sql internal/database/migrations/00021_bast_individual_documents.sql internal/bast/schema_integration_test.go
git commit -m "feat(bast): add individual document and bundle schema"
```

### Task 2: Implement scoped BA queries, lock validation, and date summaries

**Files:**
- Create: `internal/bast/models.go`
- Create: `internal/bast/repository.go`
- Create: `internal/bast/repository_integration_test.go`
- Create: `internal/bast/service.go`
- Create: `internal/bast/service_test.go`

**Interfaces:**
- Consumes: completed distribution data, locked `slot_quota`, profiles, zones, and `auth.RegencyScope`.
- Produces: `LockRegencyTotal`, `ListDates`, `ListRecipients`, and `BuildDocumentNumber`.

- [ ] **Step 1: Define models and failing service tests**

Use explicit types:

```go
type DateSummary struct {
    LocalDate string `json:"local_date"`
    RecipientCount int `json:"recipient_count"`
    ValidationStatus string `json:"validation_status"`
    Bundle *DailyBundle `json:"bundle,omitempty"`
}
type RecipientDocument struct {
    DistributionSlotID string
    SlotNumber, FinalTotal, Padding int
    DocumentNumber, LocalDate string
    Snapshot Snapshot
}
```

Tests cover per-regency numbering, Roman months, `Asia/Jakarta` date grouping, numeric ordering, out-of-range slot numbers, missing final total, and `ZONA BELUM DIATUR` rejection.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/bast -run 'Test.*Number|Test.*Date|Test.*Lock' -v`

Expected: FAIL because service and repository do not exist.

- [ ] **Step 3: Implement repository queries**

Queries join distribution slots to schedules, people, identifiers, package snapshots, program, profile, assignment, zone, and regency. Every entry point includes regency scope. Convert `distributed_at` to configured timezone in Go after retrieving timestamps; store the resulting `YYYY-MM-DD` in final rows.

- [ ] **Step 4: Implement lock and formatting logic**

Locking validates positive `slot_quota`, all existing slot numbers within `1..quota`, no final BA with a different denominator, and a published Petani document profile. `BuildDocumentNumber` uses `fmt.Sprintf("%0*d/%d/%s-%s/%s/%d", padding, slot, total, series, regencyCode, romanMonth, year)`.

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/bast/... -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/bast
git commit -m "feat(bast): query and number individual handover records"
```

### Task 3: Build immutable recipient snapshots

**Files:**
- Create: `internal/bast/snapshot.go`
- Create: `internal/bast/snapshot_test.go`
- Modify: `internal/bast/repository.go`
- Modify: `internal/bast/repository_integration_test.go`
- Modify: `internal/bast/service.go`
- Modify: `internal/bast/service_test.go`

**Interfaces:**
- Consumes: raw joined distribution row and published document profile/logo metadata.
- Produces: `BuildSnapshot(SourceData) (Snapshot, error)` and persisted final `bast_individual_documents` rows.

- [ ] **Step 1: Write failing snapshot tests**

Cover required recipient fields, farmer card selection, machine/hose/converter values, component quantity/unit/check state, supervisor/executor names, logo order, phone/address wrapping inputs, and refusal to build a farmer snapshot from a fisherman program.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/bast -run 'TestBuildSnapshot|TestFinalSnapshot' -v`

Expected: FAIL because snapshot builder is missing.

- [ ] **Step 3: Implement normalized immutable snapshot types**

Use typed nested structs rather than arbitrary maps for renderer-facing fields. Marshal the typed snapshot to JSON for persistence. Finalization inserts a revision in one transaction and never overwrites a `final` row.

- [ ] **Step 4: Prove immutability in integration tests**

Finalize a BA, update the source person's name and package serial, reload the BA, and assert the snapshot still contains the original values.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/bast/... -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/bast
git commit -m "feat(bast): persist immutable recipient snapshots"
```

### Task 4: Render Petani BA pages and daily PDFs

**Files:**
- Create: `internal/bast/pdf_renderer.go`
- Create: `internal/bast/pdf_renderer_test.go`
- Create: `internal/bast/testdata/petani-short.golden.txt`
- Create: `internal/bast/testdata/petani-long.golden.txt`

**Interfaces:**
- Consumes: typed `Snapshot` values and logo byte readers.
- Produces: `RenderPetaniBundle(ctx, BundleRenderInput) (RenderedBundle, error)` with PDF bytes and page count.

- [ ] **Step 1: Write failing renderer tests**

Assert `%PDF-` output, recipient order `1, 10, 100`, each recipient's first-page marker appears after a page boundary, a long component list creates continuation pages, logo aspect ratios are bounded, and bundle filename formatting returns `SELASA, 10 DESEMBER 2024.pdf`.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/bast -run 'TestRenderPetani|TestBundleFilename' -v`

Expected: FAIL because renderer functions do not exist.

- [ ] **Step 3: Implement focused renderer helpers**

Split responsibilities into header/logo, recipient identity, equipment tables, component table, declaration, and signature blocks. Register an embedded Unicode-capable font asset if the current core font cannot render Indonesian text reliably. Always call `AddPage()` before each recipient; allow table helpers to add continuation pages and repeat a compact document header.

- [ ] **Step 4: Add render-inspection assertions**

Extract text from test PDFs using a test-only reader or deterministic renderer event log and compare structural golden files. The golden files contain section/order/page markers, not binary PDF bytes or timestamps.

- [ ] **Step 5: Run renderer tests**

Run: `go test ./internal/bast -run 'TestRenderPetani|TestBundleFilename' -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/bast/pdf_renderer.go internal/bast/pdf_renderer_test.go internal/bast/testdata
git commit -m "feat(bast): render Petani daily handover PDFs"
```

### Task 5: Finalize and synchronize daily bundles safely

**Files:**
- Create: `internal/bast/bundle_service.go`
- Create: `internal/bast/bundle_service_test.go`
- Modify: `internal/bast/repository.go`
- Modify: `internal/bast/repository_integration_test.go`

**Interfaces:**
- Consumes: snapshot service, renderer, `media.Storage`, and `media.BuildFolderPath`.
- Produces: `PreviewBundle`, `FinalizeBundle`, `OpenBundle`, and idempotent active-version swapping.

- [ ] **Step 1: Write failing orchestration tests**

Tests cover no completed recipients, checksum no-op, exact destination path, successful upload/swap/delete-old order, upload failure preserving old active bundle, database swap failure deleting only the new orphan when safe, old-file deletion failure recording cleanup state, and concurrent finalization conflict.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/bast -run 'Test.*Bundle' -v`

Expected: FAIL because orchestration is missing.

- [ ] **Step 3: Implement preview and finalize flow**

Preview renders without writing storage/database final rows. Finalize obtains an advisory transaction lock keyed by program/regency/date/type, creates final recipient revisions as needed, renders, compares checksum, uploads using the Indonesian date filename, swaps the active bundle row, commits, then deletes the prior storage key. Persist retryable cleanup information when deletion fails.

- [ ] **Step 4: Implement content streaming**

`OpenBundle` resolves only an active scoped bundle and calls `Storage.Open(storageKey)`. Never expose Drive links or file IDs in public JSON.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/bast/... -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/bast
git commit -m "feat(bast): finalize and sync daily handover bundles"
```

### Task 6: Expose BA Perorangan APIs and wire dependencies

**Files:**
- Create: `internal/api/bast_routes.go`
- Create: `internal/api/bast_routes_test.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: Task 5 BA service.
- Produces: scoped date, recipient, preview, finalize/sync, metadata, and content endpoints.

- [ ] **Step 1: Write failing route tests**

Cover:

```text
GET  /api/v1/bast/individual/dates?program_id=&regency_id=
GET  /api/v1/bast/individual/recipients?program_id=&regency_id=&date=
POST /api/v1/bast/individual/lock-total
POST /api/v1/bast/individual/bundles/preview
POST /api/v1/bast/individual/bundles/finalize
GET  /api/v1/bast/individual/bundles/{id}/content
```

Reads require `bast.view`; lock/finalize require `bast.manage`. Tests cover malformed dates, missing IDs, fisherman template unavailable, out-of-scope regency, PDF headers, and Indonesian attachment filenames.

- [ ] **Step 2: Run API tests and confirm failure**

Run: `go test ./internal/api -run 'Test.*BASTIndividual' -v`

Expected: FAIL because routes/dependency are absent.

- [ ] **Step 3: Implement routes and error mapping**

Parse dates strictly as `2006-01-02`, call `regencyScope`, stream PDF with `Content-Type: application/pdf`, and use RFC 5987-safe filename handling in `Content-Disposition`.

- [ ] **Step 4: Wire repository, renderer, storage, and service in server startup**

Construct the BA service only after storage and programs dependencies exist. Startup must fail clearly if a required dependency cannot initialize; local storage remains supported.

- [ ] **Step 5: Run API tests**

Run: `go test ./internal/api ./cmd/server -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/api/bast_routes.go internal/api/bast_routes_test.go internal/api/handler.go internal/api/routes.go internal/api/handler_test.go cmd/server/main.go
git commit -m "feat(api): expose individual handover bundles"
```

### Task 7: Build the BA Perorangan workspace

**Files:**
- Modify: `frontend/src/features/berita-acara/BeritaAcaraPage.tsx`
- Modify: `frontend/src/features/berita-acara/BeritaAcaraPage.test.tsx`
- Create: `frontend/src/features/berita-acara/types.ts`
- Create: `frontend/src/features/berita-acara/BAIndividualPanel.tsx`
- Create: `frontend/src/features/berita-acara/BAIndividualPanel.test.tsx`
- Create: `frontend/src/features/berita-acara/BAIndividualDetail.tsx`
- Modify or create: `frontend/src/features/berita-acara/BeritaAcara.module.css`

**Interfaces:**
- Consumes: Task 6 endpoints and the existing URL-backed schedule/tab selection.
- Produces: date-based Petani generation/sync UI and explicit Nelayan unavailable state.

- [ ] **Step 1: Write failing UI tests**

Cover program/schedule-derived Petani badge, zone/regency/date summaries, numeric recipient order, locked-total call, preview opening, finalize/sync mutation, download content URL, retry state, no-distribution empty state, validation-error details, and Nelayan template-disabled state. Assert the UI never claims that Drive contains per-recipient PDFs or date subfolders.

- [ ] **Step 2: Run tests and confirm failure**

Run from `frontend`: `npm test -- --run src/features/berita-acara`

Expected: FAIL because the BA Perorangan panel is still a static stub.

- [ ] **Step 3: Implement query types and the date workspace**

Keep the existing 12-document tabs. Replace only the `individual` tab content with filters, summary cards, status badges, accessible tables, and actions. Use TanStack Query keys containing program, regency, and date; invalidate date/recipient/bundle queries after finalization.

- [ ] **Step 4: Implement preview and download behavior**

Preview requests a PDF blob and opens/revokes an object URL safely. Download uses the server-provided attachment. Disable mutation actions while pending and retain visible errors with retry buttons.

- [ ] **Step 5: Run focused tests, typecheck, and lint**

Run from `frontend`:

```powershell
npm test -- --run src/features/berita-acara
npm run typecheck
npm run lint
```

Expected: PASS with no new lint findings.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/features/berita-acara
git commit -m "feat(frontend): manage daily BA Perorangan bundles"
```

### Task 8: Perform visual PDF and end-to-end verification

**Files:**
- Modify only files required by verified defects.
- Update generated `web/static/app` assets using the repository build/sync workflow.

**Interfaces:**
- Consumes: the completed feature.
- Produces: release-ready assets and verification evidence.

- [ ] **Step 1: Run all backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run all frontend gates**

Run from `frontend`:

```powershell
npm test -- --run
npm run typecheck
npm run lint
npm run build
```

Expected: tests/typecheck/build PASS and no new lint findings.

- [ ] **Step 3: Render representative PDFs for inspection**

Generate fixtures for one short BA, one long multi-page BA, and a 50-recipient daily bundle. Render PDF pages to images with Poppler (or the repository PDF verification tooling) and inspect logo proportions, A4 margins, table wrapping, continuation headers, signature blocks, page boundaries, and recipient order.

- [ ] **Step 4: Exercise storage failure paths**

Run focused fake-Drive tests proving stale cache recovery, upload failure preservation, checksum no-op, safe version swap, and cleanup retry.

- [ ] **Step 5: Verify production bundle synchronization**

Run the repository's static bundle sync/check command after `npm run build`; confirm `web/static/app/.vite/manifest.json`, `index.html`, and hashed assets agree and stale generated assets are removed only when confirmed unreferenced.

- [ ] **Step 6: Commit generated assets and verification fixes**

```powershell
git add web/static/app frontend internal cmd
git commit -m "feat: complete BA Perorangan daily bundle workflow"
```

- [ ] **Step 7: Request final code review**

Use `superpowers:requesting-code-review` against the complete diff. Resolve Critical and Important findings, rerun the affected gates, and only then report completion.
