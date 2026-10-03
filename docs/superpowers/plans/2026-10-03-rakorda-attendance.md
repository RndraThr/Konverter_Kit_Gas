# RAKORDA Attendance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the RAKORDA blank attendance PDF, adjustable settings, print/download experience, result uploads to Drive, and browser-based scan enhancement workflow.

**Architecture:** A dedicated `internal/bast` RAKORDA service renders ephemeral blank PDFs and owns uploaded results. Schedule-level RAKORDA settings extend the existing BA settings table, while uploaded artifacts live in a dedicated scoped table. The React panel composes settings, preview actions, upload history, and a dependency-free canvas scanner that emits a multi-page PDF.

**Tech Stack:** Go, pgx/PostgreSQL, gofpdf, existing media.Storage, React 19, TanStack Query, Canvas 2D, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-03-rakorda-attendance-design.md`

## Global Constraints

- Work directly in the current shared worktree because the user explicitly requested immediate inline implementation and it contains dependent uncommitted BA work.
- Do not modify or discard unrelated user changes.
- Preview is ephemeral; only uploaded completed artifacts go to Drive.
- Accept PDF, JPEG, and PNG up to 20 MiB per file.
- Scan processing stays local in the browser and adds no large computer-vision dependency.
- Use TDD for every behavior change.

## Review Focus

- A forged MIME header must not allow an unsupported upload; tests assert byte-sniffed rejection.
- A caller outside the schedule regency must not list, download, delete, or configure RAKORDA artifacts.
- Row counts 4 and 201 must be rejected; 5 and 200 must render without broken numbering.
- Light handwriting must remain visible under enhanced-color processing.
- A failed database insert or storage deletion must not orphan or silently lose a Drive object.

---

### Task 1: Schema and settings contract

**Files:**
- Create: `internal/database/migrations/00036_bast_rakorda.sql`
- Modify: `internal/bast/settings.go`
- Modify: `internal/bast/settings_repository.go`
- Test: `internal/bast/settings_test.go`

**Interfaces:**
- Produces: `RakordaSettings{ScheduleID, Location, RowCount}` and repository get/save methods.

- [ ] Write failing tests for default row count 45, uppercase/trimmed location, and 5-200 validation.
- [ ] Run `go test ./internal/bast -run RakordaSettings -count=1` and confirm missing contract failures.
- [ ] Add migration columns/table/indexes and implement the settings service/repository methods.
- [ ] Re-run the focused tests and confirm they pass.

### Task 2: PDF context and renderer

**Files:**
- Create: `internal/bast/rakorda.go`
- Create: `internal/bast/rakorda_renderer.go`
- Create: `internal/bast/rakorda_test.go`
- Modify: `internal/bast/repository.go`

**Interfaces:**
- Consumes: `RakordaSettings` and existing `DP3Context`, logo snapshots, font helpers.
- Produces: `RakordaService.Preview(ctx,scheduleID,date,scope) (AggregatePreview,error)`.

- [ ] Write renderer tests asserting A4 portrait, five headers, exact sequential numbers, page continuation, configured location, date narrative, and absence of attendee data.
- [ ] Run `go test ./internal/bast -run Rakorda -count=1` and confirm missing renderer/service failures.
- [ ] Implement context assembly, validation, pagination, logos, title/narrative, and blank table rendering.
- [ ] Render a representative 45-row PDF, rasterize all pages with Ghostscript, and visually verify it against the reference.
- [ ] Re-run focused tests.

### Task 3: Upload persistence and Drive lifecycle

**Files:**
- Create: `internal/bast/rakorda_upload.go`
- Create: `internal/bast/rakorda_upload_repository.go`
- Create: `internal/bast/rakorda_upload_test.go`
- Modify: `internal/media/folder_path_test.go`

**Interfaces:**
- Produces: list/upload/open/delete methods and `RakordaUpload` JSON shape.
- Uses: `media.Storage.PutNamed/Open/Delete` and folder child `6. RAKORDA`.

- [ ] Write failing service tests for PDF/JPEG/PNG sniffing, unsupported content, 20 MiB limit, deterministic display filename, Drive path, cleanup, and scoped deletion recovery.
- [ ] Run the focused tests and confirm failures.
- [ ] Implement upload models, repository, service, audit events, content URLs, and storage cleanup.
- [ ] Re-run focused tests.

### Task 4: API routes and server wiring

**Files:**
- Create: `internal/api/rakorda_routes.go`
- Create: `internal/api/rakorda_routes_test.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/bast_aggregate_routes.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: preview/settings/upload services from Tasks 1-3.
- Produces: the seven endpoints in the spec with stable status/error mappings.

- [ ] Write failing route tests for authorization, JSON validation, multipart limits, inline PDF headers, list/content/delete, and scope-safe not-found behavior.
- [ ] Run `go test ./internal/api -run Rakorda -count=1` and confirm missing endpoints.
- [ ] Implement handlers, dependency interfaces, route dispatch, and server wiring.
- [ ] Re-run API tests.

### Task 5: Scanner processing primitives

**Files:**
- Create: `frontend/src/features/berita-acara/scanProcessing.ts`
- Create: `frontend/src/features/berita-acara/scanProcessing.test.ts`
- Create: `frontend/src/features/berita-acara/scanPdf.ts`
- Create: `frontend/src/features/berita-acara/scanPdf.test.ts`

**Interfaces:**
- Produces: four-corner perspective correction, 90-degree rotation, four color modes, JPEG page output, and `imagesToPdf(pages): Blob`.

- [ ] Write failing pixel-fixture tests for identity crop, corner mapping, rotation, grayscale, threshold, and enhanced-color preservation.
- [ ] Write a failing PDF structure test for two JPEG pages and A4 page objects.
- [ ] Run the focused Vitest files and confirm failures.
- [ ] Implement dependency-free Canvas/ImageData transformations and JPEG-PDF serialization.
- [ ] Re-run focused tests.

### Task 6: RAKORDA React workspace

**Files:**
- Create: `frontend/src/features/berita-acara/RakordaPanel.tsx`
- Create: `frontend/src/features/berita-acara/RakordaPanel.test.tsx`
- Create: `frontend/src/features/berita-acara/DocumentScanDialog.tsx`
- Create: `frontend/src/features/berita-acara/DocumentScanDialog.test.tsx`
- Modify: `frontend/src/features/berita-acara/BeritaAcaraPage.tsx`
- Modify: `frontend/src/features/berita-acara/types.ts`

**Interfaces:**
- Consumes: Task 4 endpoints and Task 5 scanner helpers.
- Produces: complete tab UI for settings, date, preview/download/print, direct multi-upload, scan-assisted PDF creation, file history, download, and delete.

- [ ] Write failing component tests for panel selection, settings validation/save, row bounds, preview actions, accepted file types, multi-upload refresh, scanner modes including `Warna Ditingkatkan`, reorder/delete, and final PDF upload.
- [ ] Run focused Vitest files and confirm failures.
- [ ] Implement the panel and accessible scanner dialog with touch-sized controls and responsive layouts.
- [ ] Re-run focused tests.

### Task 7: Migration, visual QA, and regression verification

**Files:**
- Modify generated frontend bundle under `web/static/app/` via the existing build.
- Update: `docs/dp3-rekap-harian-catatan.md` with RAKORDA endpoint/lifecycle notes.

**Interfaces:**
- Consumes all prior tasks.
- Produces an applied local database migration and verified production bundle.

- [ ] Run Go formatting, focused Go tests, frontend tests, typecheck, lint, and production build.
- [ ] Apply `go run ./cmd/migrate up` and verify database version 36.
- [ ] Open the local RAKORDA panel, render a 45-row preview, rasterize every page, and verify no clipping/overlap.
- [ ] Exercise direct PDF upload and a two-page enhanced-color scan against local storage; verify list/download/delete behavior.
- [ ] Run whole-repository test suites and report any unrelated pre-existing failures by name.

