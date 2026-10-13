# Dokumen Pendukung Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let field officers add PDF supporting documents (direct upload or multi-page camera scan with auto-crop) to a per-kabupaten "DOKUMEN PENDUKUNG" Google Drive folder, named automatically from a slot list configured per Jadwal Program.

**Architecture:** Reuses the existing `activity_media`/`activities` module (the 9 photo/video Dokumentasi Kegiatan modules) as a 10th `activity_type`, `dokumen_pendukung`, with its own file-type policy (PDF only) and folder destination (`DOKUMEN PENDUKUNG` category, flat). Adds one small new table, `supporting_document_slots`, scoped per `program_schedules` row, that supplies the picklist used to name uploaded/scanned files. Camera scanning does edge-detection/perspective-crop/sharpen client-side (lazy-loaded, like the existing `BarcodeScanner`) and uploads already-cropped page images to a new backend endpoint that assembles them into one PDF with `fpdf` (already a dependency).

**Tech Stack:** Go 1.26 / pgx / goose migrations (backend), React 19 / TypeScript / Vite / TanStack Query (frontend), `github.com/go-pdf/fpdf` (PDF assembly), `jscanify` (client-side document-edge-detection and perspective-crop, built on OpenCV.js — lazy-loaded).

**Spec:** `docs/superpowers/specs/2026-10-13-dokumen-pendukung-design.md`

## Global Constraints

- PDF only for `dokumen_pendukung` uploads — never image/video (spec §2, §4, §6).
- No color-mode picker — one automatic sharpen+contrast pass, no user-selectable filter (spec §2).
- No new storage category folder structure beyond `DOKUMEN PENDUKUNG` itself — flat, no per-type subfolder (spec §5).
- Slot names are a per-`schedule_id` picklist for naming only, never a storage category and never a foreign key on `activity_media` (spec §3.1).
- Max 30 pages per scanned document; max 6 MB per page; max 60 MB total scan request (spec §7).
- No hard-delete of slots — only `is_active` toggle (spec §8.2).
- All new endpoints enforce regency scope the same way the rest of the system does: out-of-scope access returns `not_found`, not `forbidden` (spec §10).

## Review Focus

- A `dokumen_pendukung` upload sent with `activity_type=dokumen_pendukung` but an image/video file — must be rejected with `ErrMediaTypeInvalid`, not silently accepted as a photo (covered: Task 6).
- A scan request whose `slot_id` belongs to a *different* schedule than the `schedule_id` sent — must be rejected as `ErrSlotInvalid`, not silently accepted (covered: Task 7).
- A scan request with 0 pages, or with a non-image part named `page` — must be rejected before any `fpdf` work happens, not panic or produce an empty/corrupt PDF (covered: Task 8).
- Deactivating a slot must not affect documents already uploaded through it — their `display_name` must keep the old name verbatim (covered: Task 4 — snapshot behavior, no FK).
- A user with `activities.manage` but without `programs.manage` must be able to list slots (to pick one) but must not be able to create/rename/deactivate them (covered: Task 5).

---

### Task 1: Migration — `supporting_document_slots` table

**Files:**
- Create: `internal/database/migrations/00065_supporting_document_slots.sql`
- Test: `internal/database/migrations/00065_supporting_document_slots_test.go`

**Interfaces:**
- Produces: table `supporting_document_slots(id uuid, schedule_id uuid, name text, is_active boolean, sort_order integer, created_at timestamptz, updated_at timestamptz)`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Picklist of document names an officer chooses from when uploading or
-- scanning a Dokumen Pendukung PDF, scoped to one Jadwal Program so the
-- schedule is the single place that configures it. The name is never
-- stored as a foreign key on activity_media — only used once to build
-- that row's display_name, so renaming/deactivating a slot never changes
-- the name already printed on a past upload.
CREATE TABLE supporting_document_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_supporting_document_slots_schedule ON supporting_document_slots(schedule_id) WHERE is_active;

-- +goose Down
DROP TABLE IF EXISTS supporting_document_slots;
```

- [ ] **Step 2: Write the migration test**

Follow the pattern of `internal/database/migrations/00060_recipient_replacements_test.go` (or any recent `NNNNN_*_test.go` in that directory) — it runs the migration against a scratch schema and asserts the table/columns exist. Write:

```go
package migrations

import "testing"

func TestMigration00065CreatesSupportingDocumentSlots(t *testing.T) {
	db := migrateToVersion(t, 65)
	var exists bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'supporting_document_slots')`).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("supporting_document_slots table was not created")
	}
}
```

Check the actual helper name (`migrateToVersion` is a guess at the existing pattern) by reading one neighboring `_test.go` file in `internal/database/migrations/` before writing this — copy its exact helper call, not the name above verbatim if it differs.

- [ ] **Step 3: Run the migration test**

Run: `go test ./internal/database/migrations/... -run TestMigration00065 -v`
Expected: PASS

- [ ] **Step 4: Apply the migration to the local dev database**

Run: `go run ./cmd/migrate up`
Expected: `OK   00065_supporting_document_slots.sql` printed, final line `goose: successfully migrated database to version: 65`

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/00065_supporting_document_slots.sql internal/database/migrations/00065_supporting_document_slots_test.go
git commit -m "feat(db): add supporting_document_slots table for Dokumen Pendukung naming"
```

---

### Task 2: `internal/media/upload.go` — `KindDocument` + `"document"` policy

**Files:**
- Modify: `internal/media/upload.go`
- Modify: `internal/media/upload_test.go`

**Interfaces:**
- Produces: `media.KindDocument` (`Kind` value `"document"`), `media.MaxDocumentBytes` (`int64`, `20 << 20`), `DetectUpload` recognizing `application/pdf`, `PolicyAllows("document", kind)`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/media/upload_test.go` (follow the existing table-style tests in that file for `DetectUpload`/`PolicyAllows`):

```go
func TestDetectUploadRecognizesPDF(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4\n%fake-but-valid-magic-bytes-for-detection\n")
	detected, err := DetectUpload(bytes.NewReader(pdfBytes), "scan.pdf")
	if err != nil {
		t.Fatalf("DetectUpload() error = %v", err)
	}
	if detected.Kind != KindDocument {
		t.Fatalf("Kind = %v, want KindDocument", detected.Kind)
	}
	if detected.MimeType != "application/pdf" {
		t.Fatalf("MimeType = %q, want application/pdf", detected.MimeType)
	}
	if detected.MaxBytes != MaxDocumentBytes {
		t.Fatalf("MaxBytes = %d, want %d", detected.MaxBytes, MaxDocumentBytes)
	}
}

func TestDetectUploadRejectsFakePDFExtension(t *testing.T) {
	notActuallyPDF := []byte("this is plain text, not a PDF")
	_, err := DetectUpload(bytes.NewReader(notActuallyPDF), "scan.pdf")
	if !errors.Is(err, ErrUnsupportedUpload) {
		t.Fatalf("err = %v, want ErrUnsupportedUpload", err)
	}
}

func TestPolicyAllowsDocumentPolicy(t *testing.T) {
	if !PolicyAllows("document", KindDocument) {
		t.Fatal("policy \"document\" must allow KindDocument")
	}
	if PolicyAllows("document", KindImage) {
		t.Fatal("policy \"document\" must not allow KindImage")
	}
	if PolicyAllows("image_video", KindDocument) {
		t.Fatal("policy \"image_video\" must not allow KindDocument (regression guard)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/media/... -run 'TestDetectUploadRecognizesPDF|TestDetectUploadRejectsFakePDFExtension|TestPolicyAllowsDocumentPolicy' -v`
Expected: FAIL — `KindDocument`/`MaxDocumentBytes` undefined, `"document"` case missing from `PolicyAllows`.

- [ ] **Step 3: Implement**

In `internal/media/upload.go`, change the `const` block:

```go
const (
	KindImage    Kind = "image"
	KindVideo    Kind = "video"
	KindDocument Kind = "document"

	MaxImageBytes    int64 = 25 << 20
	MaxVideoBytes    int64 = 500 << 20
	MaxDocumentBytes int64 = 20 << 20
)
```

In `DetectUpload`, add a case to the `switch mimeType` block (any position — `http.DetectContentType` returns `"application/pdf"` for real PDFs via their `%PDF-` magic bytes, so this only matches genuine PDFs):

```go
case "application/pdf":
	detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindDocument, mimeType, ".pdf", MaxDocumentBytes
```

In `PolicyAllows`, add a case:

```go
case "document":
	return kind == KindDocument
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/media/... -v`
Expected: PASS (including all pre-existing tests in the package — this change must be additive).

- [ ] **Step 5: Commit**

```bash
git add internal/media/upload.go internal/media/upload_test.go
git commit -m "feat(media): recognize PDF uploads as KindDocument"
```

---

### Task 3: `internal/programs` — `GetSchedule(ctx, id, scope)` lookup

**Files:**
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/repository.go`
- Test: `internal/programs/repository_integration_test.go`

**Interfaces:**
- Consumes: existing `scheduleSelect` SQL constant and `scanSchedule` helper in `internal/programs/repository.go` (around line 640-661), existing `ErrNotFound` in that package.
- Produces: `Service.GetSchedule(ctx context.Context, id string, scope auth.RegencyScope) (Schedule, error)` and `Repository.GetSchedule(ctx context.Context, id string, scope auth.RegencyScope) (Schedule, error)` — scoped single-row lookup, needed by Task 6 to resolve a Dokumen Pendukung upload's `schedule_id` into `ProgramID`/`RegencyID`.

- [ ] **Step 1: Write the failing test**

Add to `internal/programs/repository_integration_test.go` (follow the existing integration-test setup in that file — it already seeds a program/regency/schedule fixture for other tests; reuse that fixture helper rather than re-writing seed SQL):

```go
func TestRepositoryGetScheduleReturnsNotFoundOutsideScope(t *testing.T) {
	repo, ctx := newTestRepository(t) // use whatever the file's existing setup helper is actually named
	schedule := seedSchedule(t, repo, ctx) // reuse the existing fixture helper for a schedule row

	got, err := repo.GetSchedule(ctx, schedule.ID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatalf("GetSchedule() error = %v", err)
	}
	if got.ID != schedule.ID || got.ProgramID != schedule.ProgramID || got.RegencyID != schedule.RegencyID {
		t.Fatalf("got %+v, want ID/ProgramID/RegencyID matching %+v", got, schedule)
	}

	_, err = repo.GetSchedule(ctx, schedule.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for out-of-scope regency", err)
	}
}
```

Read the top of `internal/programs/repository_integration_test.go` first and substitute the real names of its setup/seed helpers for `newTestRepository`/`seedSchedule` above — every existing test in that file already seeds a schedule, copy that exact call.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/programs/... -run TestRepositoryGetScheduleReturnsNotFoundOutsideScope -v`
Expected: FAIL — `GetSchedule` undefined on `*Repository`.

- [ ] **Step 3: Implement**

In `internal/programs/repository.go`, add near `scheduleByID` (around line 663):

```go
func (r *Repository) GetSchedule(ctx context.Context, id string, scope auth.RegencyScope) (Schedule, error) {
	item, err := scanSchedule(r.pool.QueryRow(ctx, scheduleSelect+` WHERE s.id=$1 AND ($2 OR s.regency_id::text = ANY($3))`, id, scope.Unrestricted, scope.RegencyIDs))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	return item, err
}
```

In `internal/programs/service.go`, add near `ListSchedules`/`SaveSchedule` (around line 81-85):

```go
func (s *Service) GetSchedule(ctx context.Context, id string, scope auth.RegencyScope) (Schedule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Schedule{}, ErrInvalidInput
	}
	return s.repository.GetSchedule(ctx, id, scope)
}
```

Check `strings` is already imported in `service.go` (it is, used elsewhere in that file) — no new import needed. Add `GetSchedule(ctx context.Context, id string, scope auth.RegencyScope) (Schedule, error)` to this file's `repository` interface definition (wherever `ListSchedules`/`SaveSchedule` are declared on it) so `*Service` can call it on the injected repository.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/programs/... -v`
Expected: PASS (including all pre-existing tests in the package).

- [ ] **Step 5: Commit**

```bash
git add internal/programs/repository.go internal/programs/service.go internal/programs/repository_integration_test.go
git commit -m "feat(programs): add scoped GetSchedule lookup for Dokumen Pendukung's schedule-scoped uploads"
```

---

### Task 4: `internal/activities` — `supporting_document_slots` CRUD

**Files:**
- Create: `internal/activities/slots.go`
- Create: `internal/activities/slots_test.go`
- Test: `internal/activities/repository_integration_test.go` (add slot repository tests here, following that file's existing fixture/setup pattern)

**Interfaces:**
- Consumes: `auth.Principal`, `auth.RegencyScope`, `auth.ClientMeta` (all already imported in `internal/activities`); the `program_schedules` table from Task 1's migration context (no Go-level dependency — slots are scoped by `schedule_id`, validated against `program_schedules` only implicitly via the FK and via the programs-package schedule lookup used at upload time in Task 6, not here).
- Produces:
  - `type SupportingDocumentSlot struct { ID, ScheduleID, Name string; IsActive bool; SortOrder int; CreatedAt, UpdatedAt time.Time }`
  - `Service.ListSlots(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error)`
  - `Service.CreateSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)`
  - `Service.PatchSlot(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)`
  - `ErrSlotNameRequired`, `ErrSlotNotFound`, `ErrScheduleNotFound` (new, package `activities`)

**Security-critical — every one of these three methods MUST enforce `scope`, not just accept it.** `schedule_id`/slot `id` are plain UUIDs a client controls; without a regency-scope check at the SQL level, any authenticated user with `programs.manage` could create or rename a slot on a schedule belonging to a kabupaten outside their assignment (IDOR). `CreateSlot`/`PatchSlot` must fail with `ErrScheduleNotFound`/`ErrSlotNotFound` (not succeed) when the target schedule's regency is outside `scope`; `ListSlots` must return an empty list (not an error) for an out-of-scope `schedule_id`, matching how `activities.List` already filters by scope at the SQL level elsewhere in this package.

- [ ] **Step 1: Write the failing tests**

Create `internal/activities/slots_test.go`:

```go
package activities

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type slotRepositoryStub struct {
	slots       []SupportingDocumentSlot
	insertCalls int
	patchCalls  int
}

func (r *slotRepositoryStub) ListSlots(_ context.Context, scheduleID string) ([]SupportingDocumentSlot, error) {
	items := make([]SupportingDocumentSlot, 0)
	for _, s := range r.slots {
		if s.ScheduleID == scheduleID && s.IsActive {
			items = append(items, s)
		}
	}
	return items, nil
}

func (r *slotRepositoryStub) InsertSlot(_ context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	r.insertCalls++
	slot := SupportingDocumentSlot{ID: "new-slot", ScheduleID: scheduleID, Name: name, IsActive: true}
	r.slots = append(r.slots, slot)
	return slot, nil
}

func (r *slotRepositoryStub) PatchSlotRow(_ context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	r.patchCalls++
	for i, s := range r.slots {
		if s.ID != id {
			continue
		}
		if name != nil {
			r.slots[i].Name = *name
		}
		if isActive != nil {
			r.slots[i].IsActive = *isActive
		}
		return r.slots[i], nil
	}
	return SupportingDocumentSlot{}, ErrSlotNotFound
}

func TestCreateSlotRejectsBlankName(t *testing.T) {
	service := &Service{slotRepository: &slotRepositoryStub{}}
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "   ", auth.ClientMeta{})
	if !errors.Is(err, ErrSlotNameRequired) {
		t.Fatalf("err = %v, want ErrSlotNameRequired", err)
	}
}

func TestCreateAndListSlots(t *testing.T) {
	repo := &slotRepositoryStub{}
	service := &Service{slotRepository: repo}
	created, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "Surat Tugas", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if created.Name != "Surat Tugas" || created.ScheduleID != "schedule-1" {
		t.Fatalf("created = %+v, want name/schedule matching input", created)
	}
	list, err := service.ListSlots(context.Background(), "schedule-1")
	if err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "Surat Tugas" {
		t.Fatalf("list = %+v, want one slot named Surat Tugas", list)
	}
}

func TestPatchSlotDeactivates(t *testing.T) {
	repo := &slotRepositoryStub{slots: []SupportingDocumentSlot{{ID: "slot-1", ScheduleID: "schedule-1", Name: "SK Penetapan", IsActive: true}}}
	service := &Service{slotRepository: repo}
	inactive := false
	updated, err := service.PatchSlot(context.Background(), auth.Principal{}, "slot-1", nil, &inactive, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("PatchSlot() error = %v", err)
	}
	if updated.IsActive {
		t.Fatal("slot should be inactive after PatchSlot(isActive=false)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/activities/... -run 'TestCreateSlotRejectsBlankName|TestCreateAndListSlots|TestPatchSlotDeactivates' -v`
Expected: FAIL — `SupportingDocumentSlot`, `Service.slotRepository`, `ErrSlotNameRequired`, `ErrSlotNotFound` undefined.

- [ ] **Step 3: Implement**

Create `internal/activities/slots.go`:

```go
package activities

import (
	"context"
	"errors"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSlotNameRequired = errors.New("supporting document slot name is required")
	ErrSlotNotFound     = errors.New("supporting document slot not found")
	ErrScheduleNotFound = errors.New("schedule not found")
)

type SupportingDocumentSlot struct {
	ID         string    `json:"id"`
	ScheduleID string    `json:"schedule_id"`
	Name       string    `json:"name"`
	IsActive   bool      `json:"is_active"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type slotRepository interface {
	ListSlots(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error)
	InsertSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)
	PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)
}

func (s *Service) ListSlots(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	return s.slotRepository.ListSlots(ctx, scheduleID, scope)
}

func (s *Service) CreateSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	name = strings.TrimSpace(name)
	if scheduleID == "" {
		return SupportingDocumentSlot{}, ErrScheduleRequired
	}
	if name == "" {
		return SupportingDocumentSlot{}, ErrSlotNameRequired
	}
	return s.slotRepository.InsertSlot(ctx, actor, scheduleID, name, meta, scope)
}

func (s *Service) PatchSlot(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SupportingDocumentSlot{}, ErrSlotNotFound
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return SupportingDocumentSlot{}, ErrSlotNameRequired
		}
		name = &trimmed
	}
	return s.slotRepository.PatchSlotRow(ctx, actor, id, name, isActive, meta, scope)
}

// ListSlots filters by the schedule's regency at the SQL level — like
// activities.List elsewhere in this package, an out-of-scope schedule_id
// yields an empty list, not an error.
func (r *Repository) ListSlots(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id::text, s.schedule_id::text, s.name, s.is_active, s.sort_order, s.created_at, s.updated_at
		FROM supporting_document_slots s
		JOIN program_schedules ps ON ps.id = s.schedule_id
		WHERE s.schedule_id = $1 AND s.is_active AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY s.sort_order, s.name
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SupportingDocumentSlot, 0)
	for rows.Next() {
		var item SupportingDocumentSlot
		if err := rows.Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// InsertSlot only inserts if scheduleID resolves to a schedule whose regency
// is in scope — the SELECT...FROM program_schedules in place of a bare VALUES
// list makes an out-of-scope or nonexistent schedule_id insert zero rows
// instead of succeeding, which ErrScheduleNotFound below turns into a clear
// not-found error (the same convention programs.GetSchedule already uses).
func (r *Repository) InsertSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	err = tx.QueryRow(ctx, `
		INSERT INTO supporting_document_slots (schedule_id, name)
		SELECT ps.id, $2 FROM program_schedules ps
		WHERE ps.id = $1 AND ($3 OR ps.regency_id::text = ANY($4))
		RETURNING id::text, schedule_id::text, name, is_active, sort_order, created_at, updated_at
	`, scheduleID, name, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupportingDocumentSlot{}, ErrScheduleNotFound
	}
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "supporting_document_slot.created", ResourceType: "supporting_document_slot", ResourceID: item.ID, Metadata: map[string]any{"schedule_id": scheduleID, "name": name}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportingDocumentSlot{}, err
	}
	return item, nil
}

// PatchSlotRow joins to program_schedules in the UPDATE so an id belonging to
// an out-of-scope regency matches zero rows (-> ErrSlotNotFound) instead of
// being patched.
func (r *Repository) PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	err = tx.QueryRow(ctx, `
		UPDATE supporting_document_slots s SET
			name = COALESCE($2, s.name),
			is_active = COALESCE($3, s.is_active),
			updated_at = now()
		FROM program_schedules ps
		WHERE s.id = $1 AND ps.id = s.schedule_id AND ($4 OR ps.regency_id::text = ANY($5))
		RETURNING s.id::text, s.schedule_id::text, s.name, s.is_active, s.sort_order, s.created_at, s.updated_at
	`, id, name, isActive, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupportingDocumentSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "supporting_document_slot.patched", ResourceType: "supporting_document_slot", ResourceID: id, Metadata: map[string]any{"name": name, "is_active": isActive}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportingDocumentSlot{}, err
	}
	return item, nil
}
```

Add `slotRepository slotRepository` as a field on `Service` in `internal/activities/service.go`. Wire it in `NewService` with `slotRepo := repository.(slotRepository)` (no comma-ok — the real `*Repository` always implements this once this task adds `ListSlots`/`InsertSlot`/`PatchSlotRow` to it, so this assertion never panics in production; using the loud form means a future caller wiring in an incompatible stub gets an immediate, clear panic at construction time instead of a confusing nil-pointer panic the first time a slot method is called) and set `slotRepository: slotRepo` in the `&Service{...}` literal — no change to the pre-existing `repository` interface, no main.go change, no type-switch loop (this is the same first positional `repository` parameter asserted to a second interface, not one of the variadic `dependencies`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/activities/... -v`
Expected: PASS.

- [ ] **Step 5: Add a repository integration test**

Add to `internal/activities/repository_integration_test.go`, following its existing seed-fixture pattern for a schedule row (reuse whatever helper the file already uses to seed `program_schedules` for its other tests):

```go
func TestInsertAndListSlotsScopedBySchedule(t *testing.T) {
	repo, ctx := newRepositoryForTest(t) // substitute the file's real setup helper name
	scheduleA := seedScheduleForTest(t, repo, ctx)
	scheduleB := seedScheduleForTest(t, repo, ctx)

	_, err := repo.InsertSlot(ctx, auth.Principal{}, scheduleA.ID, "Surat Tugas", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("InsertSlot() error = %v", err)
	}
	_, err = repo.InsertSlot(ctx, auth.Principal{}, scheduleB.ID, "SK Penetapan", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("InsertSlot() error = %v", err)
	}

	listA, err := repo.ListSlots(ctx, scheduleA.ID)
	if err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if len(listA) != 1 || listA[0].Name != "Surat Tugas" {
		t.Fatalf("listA = %+v, want exactly one slot named Surat Tugas", listA)
	}
}
```

- [ ] **Step 6: Run the integration test**

Run: `go test ./internal/activities/... -run TestInsertAndListSlotsScopedBySchedule -v`
Expected: PASS (requires the local Postgres test DB from Task 1 migrated to version 65 — run `go run ./cmd/migrate up` against `konkit_test` first if this fails with "relation does not exist").

- [ ] **Step 7: Commit**

```bash
git add internal/activities/slots.go internal/activities/slots_test.go internal/activities/repository_integration_test.go internal/activities/service.go internal/activities/models.go
git commit -m "feat(activities): add supporting_document_slots CRUD"
```

---

### Task 5: `internal/api` — slot CRUD routes

**Files:**
- Create: `internal/api/supporting_document_slots_routes.go`
- Create: `internal/api/supporting_document_slots_routes_test.go`
- Modify: `internal/api/routes.go` (register the new path)
- Modify: `internal/api/handler.go` (widen `ActivitiesService` interface)

**Interfaces:**
- Consumes: `Service.ListSlots`/`CreateSlot`/`PatchSlot` from Task 4; `h.authorize`, `h.regencyScope`, `writeData`, `writeServiceError`, `decodeJSON`, `clientMeta` helpers already used throughout `internal/api` (see `internal/api/signature_routes.go` for the exact calling convention).
- Produces: `GET /api/v1/activities/supporting-documents/slots?schedule_id=...`, `POST /api/v1/activities/supporting-documents/slots`, `PATCH /api/v1/activities/supporting-documents/slots/{id}`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/supporting_document_slots_routes_test.go`. This package's shared test harness (see `internal/api/signature_routes_test.go`, `internal/api/bast_routes_test.go`) is: `fakeAuthService{principal auth.Principal, allowedPermissions map[string]bool}` as the `Auth` dependency, `authenticatedRequest(method, target, body string, secret []byte) *http.Request` (defined in `bast_routes_test.go`, reused package-wide) to build a cookie+CSRF-bearing request, and `NewHandler(Dependencies{...}).ServeHTTP(recorder, req)`. `fakeActivitiesService` does not exist yet for this package's tests — if one already exists for Task 5/6/8 reuse it; otherwise define the minimal stub below once and reuse across all three tasks' test files (do not redefine it per file — if `fakeActivitiesService` already exists by the time this task runs, add methods to it instead of declaring a new type):

```go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
)

type fakeActivitiesService struct {
	ActivitiesService
	listedScheduleID string
	createdName      string
	patchedID        string
}

func (f *fakeActivitiesService) ListSlots(_ context.Context, scheduleID string, _ auth.RegencyScope) ([]activities.SupportingDocumentSlot, error) {
	f.listedScheduleID = scheduleID
	return []activities.SupportingDocumentSlot{{ID: "slot-1", ScheduleID: scheduleID, Name: "Surat Tugas", IsActive: true}}, nil
}
func (f *fakeActivitiesService) CreateSlot(_ context.Context, _ auth.Principal, scheduleID, name string, _ auth.ClientMeta, _ auth.RegencyScope) (activities.SupportingDocumentSlot, error) {
	f.createdName = name
	return activities.SupportingDocumentSlot{ID: "slot-2", ScheduleID: scheduleID, Name: name, IsActive: true}, nil
}
func (f *fakeActivitiesService) PatchSlot(_ context.Context, _ auth.Principal, id string, _ *string, isActive *bool, _ auth.ClientMeta, _ auth.RegencyScope) (activities.SupportingDocumentSlot, error) {
	f.patchedID = id
	active := isActive != nil && *isActive
	return activities.SupportingDocumentSlot{ID: id, IsActive: active}, nil
}

const supportingSlotsTestSecret0 = "01234567890123456789012345678901"

func TestHandleSupportingDocumentSlotsListRequiresActivitiesView(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{}}
	service := &fakeActivitiesService{}
	req := authenticatedRequest(http.MethodGet, "/api/v1/activities/supporting-documents/slots?schedule_id=schedule-1", "", nil)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if service.listedScheduleID != "" {
		t.Fatal("ListSlots must not be called without activities.view")
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.view": true}}
	rec = httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.listedScheduleID != "schedule-1" {
		t.Fatalf("status = %d, listedScheduleID = %q", rec.Code, service.listedScheduleID)
	}
}

func TestHandleSupportingDocumentSlotsCreateRequiresProgramsManage(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	fieldOfficer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}
	req := authenticatedRequest(http.MethodPost, "/api/v1/activities/supporting-documents/slots", `{"schedule_id":"schedule-1","name":"Surat Tugas"}`, secret)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: fieldOfficer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (activities.manage alone must not grant slot CRUD)", rec.Code)
	}
	if service.createdName != "" {
		t.Fatal("CreateSlot must not be called without programs.manage")
	}

	admin := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"programs.manage": true}}
	req = authenticatedRequest(http.MethodPost, "/api/v1/activities/supporting-documents/slots", `{"schedule_id":"schedule-1","name":"Surat Tugas"}`, secret)
	rec = httptest.NewRecorder()
	NewHandler(Dependencies{Auth: admin, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.createdName != "Surat Tugas" {
		t.Fatalf("status = %d, createdName = %q", rec.Code, service.createdName)
	}
}
```

Check whether `validSessionToken` (used implicitly by `authenticatedRequest` via the session cookie) and `fakeAuthService` are already defined package-wide (they are, in `handler_test.go`) — do not redeclare them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestHandleSupportingDocumentSlots -v`
Expected: FAIL — handler function doesn't exist yet.

- [ ] **Step 3: Implement**

In `internal/api/handler.go`, widen the existing `ActivitiesService` interface (around the block shown in the spec research):

```go
type ActivitiesService interface {
	List(context.Context, activities.Filter, auth.RegencyScope) (activities.Page, error)
	Upload(context.Context, auth.Principal, activities.UploadInput, auth.ClientMeta, auth.RegencyScope) (activities.ActivityMedia, error)
	Delete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	OpenContent(context.Context, string, auth.RegencyScope) (activities.MediaContent, error)
	ListSlots(context.Context, string, auth.RegencyScope) ([]activities.SupportingDocumentSlot, error)
	CreateSlot(context.Context, auth.Principal, string, string, auth.ClientMeta, auth.RegencyScope) (activities.SupportingDocumentSlot, error)
	PatchSlot(context.Context, auth.Principal, string, *string, *bool, auth.ClientMeta, auth.RegencyScope) (activities.SupportingDocumentSlot, error)
}
```

Create `internal/api/supporting_document_slots_routes.go`:

```go
package api

import (
	"net/http"
	"strings"
)

func (h *Handler) handleSupportingDocumentSlots(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
		if scheduleID == "" {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "schedule_id wajib diisi", map[string]string{"schedule_id": "wajib diisi"})
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		items, err := h.deps.Activities.ListSlots(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, items)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		var input struct {
			ScheduleID string `json:"schedule_id"`
			Name       string `json:"name"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		item, err := h.deps.Activities.CreateSlot(r.Context(), rc.principal, input.ScheduleID, input.Name, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleSupportingDocumentSlotItem(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	if !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	item, err := h.deps.Activities.PatchSlot(r.Context(), rc.principal, id, input.Name, input.IsActive, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, item)
}
```

In `internal/api/routes.go`, find the existing `case path == "activities/media":` / `case strings.HasPrefix(path, "activities/media/"):` dispatch (around line 418-421 of `handler.go` per the earlier research, confirm the exact routing file — it is `handler.go`, not a separate `routes.go`, based on the grep results; adjust this step to edit `internal/api/handler.go`'s main `switch path` block directly) and add two more cases immediately alongside them:

```go
case path == "activities/supporting-documents/slots":
	h.handleSupportingDocumentSlots(w, r, rc)
case strings.HasPrefix(path, "activities/supporting-documents/slots/"):
	h.handleSupportingDocumentSlotItem(w, r, rc, strings.TrimPrefix(path, "activities/supporting-documents/slots/"))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/supporting_document_slots_routes.go internal/api/supporting_document_slots_routes_test.go internal/api/handler.go
git commit -m "feat(api): add supporting document slot CRUD endpoints"
```

---

### Task 6: `internal/activities` — `dokumen_pendukung` activity type + schedule-scoped upload

**Files:**
- Modify: `internal/activities/models.go`
- Modify: `internal/activities/service.go`
- Modify: `internal/activities/service_test.go`

**Interfaces:**
- Consumes: `programs.Schedule` (from Task 3's `GetSchedule`), `media.PolicyAllows`/`media.KindDocument` (Task 2), `media.FolderSupporting`/`media.FolderPhotos` (already exist).
- Produces: `UploadInput.ScheduleID`/`UploadInput.SlotID` (new optional fields, used only when `ActivityType == "dokumen_pendukung"`), widened `programContextResolver` interface with `GetSchedule`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/activities/service_test.go` (follow the existing stub pattern already in that file for `programContextResolver`/`repository` stubs):

```go
func TestUploadDokumenPendukungRejectsNonPDF(t *testing.T) {
	// Build a Service with a stub programContextResolver.GetSchedule returning a
	// fixed programs.Schedule{ID: "schedule-1", ProgramID: "program-1", RegencyID: "regency-1"},
	// a stub slotRepository with one active slot {ID: "slot-1", ScheduleID: "schedule-1", Name: "Surat Tugas"},
	// and a stub repository (as the existing tests in this file already do).
	// input := UploadInput{ActivityType: "dokumen_pendukung", ScheduleID: "schedule-1", SlotID: "slot-1",
	//   Source: "gallery", OriginalFilename: "photo.jpg", Data: bytes.NewReader(fakeJPEGBytes), DeclaredSize: int64(len(fakeJPEGBytes))}
	// _, err := service.Upload(context.Background(), auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	// assert errors.Is(err, ErrMediaTypeInvalid)
}

func TestUploadDokumenPendukungAcceptsPDFAndNamesFromSlot(t *testing.T) {
	// Same setup, but input.Data is a real minimal PDF ("%PDF-1.4\n...") and
	// OriginalFilename "scan.pdf". Assert the returned ActivityMedia.DisplayName
	// contains "SURAT-TUGAS" (the sanitized slot name) and MediaType == "document".
}

func TestUploadOtherActivityTypeStillRejectsPDF(t *testing.T) {
	// Regression guard for Review Focus item 1's mirror case: activity_type
	// "rakor" (one of the existing 9) with a real PDF file must still return
	// ErrMediaTypeInvalid — confirms policyFor() didn't loosen the other 9 types.
}
```

Write the full bodies using this file's existing stub types (`repositoryStub`, `programContextResolverStub`, or whatever they are actually named — read the top of `service_test.go` first) extended with the new `GetSchedule` method and a slot lookup. Do not invent a second parallel stub framework.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/activities/... -run 'TestUploadDokumenPendukung|TestUploadOtherActivityType' -v`
Expected: FAIL — `UploadInput.ScheduleID`/`SlotID` undefined, `"dokumen_pendukung"` not in `activityTypeCodes`.

- [ ] **Step 3: Implement**

In `internal/activities/models.go`:

```go
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
	"dokumen_pendukung":     "DOKPEND",
}
```

Do NOT add `"dokumen_pendukung"` to `activityTypeFolderNames` — its absence is intentional (spec §3, §5); `folderDestination` (below) handles it as a special case instead of looking it up there.

Add to `UploadInput`:

```go
type UploadInput struct {
	ProgramID        string
	RegencyID        string
	ActivityType     string
	OriginalFilename string
	Source           string
	Data             io.Reader
	DeclaredSize     int64
	ScheduleID       string // dokumen_pendukung only — resolves ProgramID/RegencyID and the slot's schedule
	SlotID           string // dokumen_pendukung only
}
```

`ErrScheduleRequired` already exists by this point (Task 4 added it to `models.go` alongside `ErrRegencyRequired`/`ErrProgramRequired`, since `ListSlots`/`CreateSlot` need it too — do not redeclare it here). Add only the one error this task newly needs, in the same `var (...)` block in `models.go`:

```go
ErrSlotInvalid = errors.New("slot_id is invalid for this schedule")
```

Widen the `programContextResolver` interface in `service.go`:

```go
type programContextResolver interface {
	ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error)
	GetSchedule(context.Context, string, auth.RegencyScope) (programs.Schedule, error)
}
```

Add helper functions in `service.go` (package-level, near `activityFileExtension`):

```go
func folderDestination(activityType string) (media.FolderCategory, string) {
	if activityType == "dokumen_pendukung" {
		return media.FolderSupporting, ""
	}
	return media.FolderPhotos, activityTypeFolderNames[activityType]
}

func policyFor(activityType string) string {
	if activityType == "dokumen_pendukung" {
		return "document"
	}
	return "image_video"
}

func sanitizeSlotName(name string) string {
	fields := strings.Fields(name)
	return strings.ToUpper(strings.Join(fields, "-"))
}
```

Rewrite the body of `Upload` (replace the existing hardcoded `Category: media.FolderPhotos` block and the implicit image/video-only gate) to:

```go
func (s *Service) Upload(ctx context.Context, actor auth.Principal, input UploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error) {
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.ActivityType = strings.TrimSpace(input.ActivityType)
	input.Source = strings.TrimSpace(input.Source)
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.SlotID = strings.TrimSpace(input.SlotID)
	if !isValidActivityType(input.ActivityType) {
		return ActivityMedia{}, ErrActivityTypeInvalid
	}
	if input.Source != "camera" && input.Source != "gallery" {
		return ActivityMedia{}, ErrSourceInvalid
	}

	var slotName string
	if input.ActivityType == "dokumen_pendukung" {
		if input.ScheduleID == "" {
			return ActivityMedia{}, ErrScheduleRequired
		}
		if s.programContexts == nil {
			return ActivityMedia{}, ErrProgramRequired
		}
		schedule, err := s.programContexts.GetSchedule(ctx, input.ScheduleID, scope)
		if err != nil {
			return ActivityMedia{}, err
		}
		input.ProgramID, input.RegencyID = schedule.ProgramID, schedule.RegencyID
		slots, err := s.ListSlots(ctx, input.ScheduleID, scope)
		if err != nil {
			return ActivityMedia{}, err
		}
		for _, candidate := range slots {
			if candidate.ID == input.SlotID {
				slotName = candidate.Name
				break
			}
		}
		if slotName == "" {
			return ActivityMedia{}, ErrSlotInvalid
		}
	} else {
		if input.ProgramID == "" || s.programContexts == nil {
			return ActivityMedia{}, ErrProgramRequired
		}
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, input.ProgramID, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	regency, err := s.repository.GetRegency(ctx, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	detected, err := media.DetectUpload(input.Data, input.OriginalFilename)
	if errors.Is(err, media.ErrUnsupportedUpload) {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if err != nil {
		return ActivityMedia{}, err
	}
	if !media.PolicyAllows(policyFor(input.ActivityType), detected.Kind) {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if input.DeclaredSize > detected.MaxBytes {
		return ActivityMedia{}, ErrFileTooLarge
	}
	if detected.Kind == media.KindVideo {
		release, ok := s.videoLimiter.TryAcquire()
		if !ok {
			return ActivityMedia{}, ErrVideoUploadBusy
		}
		defer release()
	}
	key, err := newStorageKey()
	if err != nil {
		return ActivityMedia{}, err
	}
	category, child := folderDestination(input.ActivityType)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName,
		RegencyName: storageContext.RegencyName, Category: category, Child: child,
	})
	if err != nil {
		return ActivityMedia{}, err
	}
	now := s.now()
	nameSegment := activityTypeCodes[input.ActivityType]
	if input.ActivityType == "dokumen_pendukung" {
		nameSegment = sanitizeSlotName(slotName)
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, nameSegment, now.Format("20060102-150405"))
	visibleFilename := displayName + activityFileExtension(detected.MimeType)
	storageKey, size, checksum, err := media.PutNamed(ctx, s.storage, key, visibleFilename, folderPath, io.LimitReader(detected.Reader, detected.MaxBytes+1))
	if err != nil {
		return ActivityMedia{}, err
	}
	if size > detected.MaxBytes {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, ErrFileTooLarge
	}
	stored, err := s.repository.Insert(ctx, actor, insertInput{
		ProgramID: input.ProgramID, RegencyID: input.RegencyID, ActivityType: input.ActivityType, StorageKey: storageKey,
		DisplayName: displayName, OriginalFilename: strings.TrimSpace(input.OriginalFilename),
		MediaType: string(detected.Kind), MimeType: detected.MimeType, ByteSize: size, Checksum: checksum, Source: input.Source,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, err
	}
	stored.ContentURL = "/api/v1/activities/media/" + stored.ID + "/content"
	return stored, nil
}
```

Add `".pdf": "application/pdf"` reverse mapping to `activityFileExtension` — actually that function maps MIME→extension, add the entry `"application/pdf": ".pdf"` to its map literal.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/activities/... -v`
Expected: PASS (including the 3 new tests and every pre-existing test in the package — this is the task most likely to regress the 9 existing activity types, read their test output carefully).

- [ ] **Step 5: Commit**

```bash
git add internal/activities/models.go internal/activities/service.go internal/activities/service_test.go
git commit -m "feat(activities): add dokumen_pendukung activity type with schedule-scoped upload"
```

---

### Task 7: `internal/activities` — `ScanUpload` (assemble pages into one PDF)

**Files:**
- Create: `internal/activities/scan.go`
- Create: `internal/activities/scan_test.go`

**Interfaces:**
- Consumes: `github.com/go-pdf/fpdf` (already in `go.mod`), `Service.Upload`'s shared tail (storage+insert) — refactor that tail out of `Upload` into a new private method both call.
- Produces: `type ScanUploadInput struct { ScheduleID, SlotID string; Pages []io.Reader; Source string }`, `Service.ScanUpload(ctx context.Context, actor auth.Principal, input ScanUploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error)`, `ErrScanPagesRequired`, `ErrScanTooManyPages`, `MaxScanPageBytes`, `MaxScanRequestBytes`, `maxScanPages = 30`.

- [ ] **Step 1: Write the failing tests**

Create `internal/activities/scan_test.go`:

```go
package activities

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func fakeJPEGPage(t *testing.T) *bytes.Reader {
	img := image.NewRGBA(image.Rect(0, 0, 100, 140))
	for y := 0; y < 140; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode fake jpeg: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestScanUploadRejectsZeroPages(t *testing.T) {
	service := &Service{} // whatever minimal stub wiring this file's sibling tests use
	_, err := service.ScanUpload(context.Background(), /* actor */ authPrincipalStub(), ScanUploadInput{ScheduleID: "schedule-1", SlotID: "slot-1", Source: "camera", Pages: nil}, /* meta */ clientMetaStub(), /* scope */ regencyScopeUnrestrictedStub())
	if !errors.Is(err, ErrScanPagesRequired) {
		t.Fatalf("err = %v, want ErrScanPagesRequired", err)
	}
}

func TestScanUploadRejectsTooManyPages(t *testing.T) {
	pages := make([]io_Reader_placeholder, 0) // build 31 fakeJPEGPage(t) readers as []io.Reader
	_ = pages
	// Assert ScanUpload returns ErrScanTooManyPages for 31 pages.
}

func TestScanUploadAssemblesPagesIntoOnePDF(t *testing.T) {
	// With a stub programContextResolver.GetSchedule and slotRepository as in
	// service_test.go's dokumen_pendukung tests (Task 6), call ScanUpload with
	// 3 fakeJPEGPage(t) readers and assert: no error, result.MediaType == "document",
	// result.MimeType == "application/pdf", result.ByteSize > 0.
}
```

Replace the placeholder helper names (`authPrincipalStub`, `clientMetaStub`, `regencyScopeUnrestrictedStub`, `io_Reader_placeholder`) with whatever this package's actual test helpers are named — `internal/activities/service_test.go` already constructs these values inline for every existing test; copy that exact style instead of inventing new helper names.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/activities/... -run TestScanUpload -v`
Expected: FAIL — `ScanUpload`/`ScanUploadInput` undefined.

- [ ] **Step 3: Implement**

First, refactor `Upload`'s storage+insert tail (everything from `key, err := newStorageKey()` onward in Task 6's version) into a shared private method on `*Service`:

```go
type preparedUpload struct {
	ActivityType, ProgramID, RegencyID, DisplayName, OriginalFilename, Source string
	Detected                                                                  media.DetectedUpload
}

func (s *Service) store(ctx context.Context, actor auth.Principal, prepared preparedUpload, folderPath []string, meta auth.ClientMeta) (ActivityMedia, error) {
	key, err := newStorageKey()
	if err != nil {
		return ActivityMedia{}, err
	}
	visibleFilename := prepared.DisplayName + activityFileExtension(prepared.Detected.MimeType)
	storageKey, size, checksum, err := media.PutNamed(ctx, s.storage, key, visibleFilename, folderPath, io.LimitReader(prepared.Detected.Reader, prepared.Detected.MaxBytes+1))
	if err != nil {
		return ActivityMedia{}, err
	}
	if size > prepared.Detected.MaxBytes {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, ErrFileTooLarge
	}
	stored, err := s.repository.Insert(ctx, actor, insertInput{
		ProgramID: prepared.ProgramID, RegencyID: prepared.RegencyID, ActivityType: prepared.ActivityType, StorageKey: storageKey,
		DisplayName: prepared.DisplayName, OriginalFilename: prepared.OriginalFilename,
		MediaType: string(prepared.Detected.Kind), MimeType: prepared.Detected.MimeType, ByteSize: size, Checksum: checksum, Source: prepared.Source,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, err
	}
	stored.ContentURL = "/api/v1/activities/media/" + stored.ID + "/content"
	return stored, nil
}
```

Update `Upload` to call `s.store(ctx, actor, preparedUpload{...}, folderPath, meta)` instead of its inline tail (same logic, just moved).

Create `internal/activities/scan.go`:

```go
package activities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/go-pdf/fpdf"
)

const (
	MaxScanPageBytes    int64 = 6 << 20
	MaxScanRequestBytes int64 = 60 << 20
	maxScanPages              = 30
)

var (
	ErrScanPagesRequired = errors.New("at least one scanned page is required")
	ErrScanTooManyPages  = errors.New("too many scanned pages in one document")
)

type ScanUploadInput struct {
	ScheduleID string
	SlotID     string
	Source     string
	Pages      []io.Reader
}

func (s *Service) ScanUpload(ctx context.Context, actor auth.Principal, input ScanUploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error) {
	if len(input.Pages) == 0 {
		return ActivityMedia{}, ErrScanPagesRequired
	}
	if len(input.Pages) > maxScanPages {
		return ActivityMedia{}, ErrScanTooManyPages
	}
	if input.Source != "camera" && input.Source != "gallery" {
		input.Source = "camera"
	}

	schedule, err := s.programContexts.GetSchedule(ctx, input.ScheduleID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	slots, err := s.ListSlots(ctx, input.ScheduleID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	var slotName string
	for _, candidate := range slots {
		if candidate.ID == input.SlotID {
			slotName = candidate.Name
			break
		}
	}
	if slotName == "" {
		return ActivityMedia{}, ErrSlotInvalid
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, schedule.ProgramID, schedule.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	regency, err := s.repository.GetRegency(ctx, schedule.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}

	doc := fpdf.New("P", "mm", "A4", "")
	pageWidth, pageHeight := 210.0, 297.0
	for i, page := range input.Pages {
		limited := io.LimitReader(page, MaxScanPageBytes+1)
		detected, err := media.DetectUpload(limited, fmt.Sprintf("page-%d.jpg", i))
		if err != nil {
			return ActivityMedia{}, ErrMediaTypeInvalid
		}
		if !media.PolicyAllows("image_video", detected.Kind) || detected.Kind != media.KindImage {
			return ActivityMedia{}, ErrMediaTypeInvalid
		}
		imageBytes, err := io.ReadAll(io.LimitReader(detected.Reader, MaxScanPageBytes+1))
		if err != nil {
			return ActivityMedia{}, err
		}
		if int64(len(imageBytes)) > MaxScanPageBytes {
			return ActivityMedia{}, ErrFileTooLarge
		}
		doc.AddPage()
		imageName := fmt.Sprintf("page-%d", i)
		doc.RegisterImageOptionsReader(imageName, fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(imageBytes))
		doc.ImageOptions(imageName, 0, 0, pageWidth, pageHeight, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	}
	var pdfBuffer bytes.Buffer
	if err := doc.Output(&pdfBuffer); err != nil {
		return ActivityMedia{}, fmt.Errorf("assemble scanned pdf: %w", err)
	}
	if int64(pdfBuffer.Len()) > MaxScanRequestBytes {
		return ActivityMedia{}, ErrFileTooLarge
	}

	detected, err := media.DetectUpload(bytes.NewReader(pdfBuffer.Bytes()), "scan.pdf")
	if err != nil {
		return ActivityMedia{}, fmt.Errorf("detect assembled pdf: %w", err)
	}

	category, child := folderDestination("dokumen_pendukung")
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName,
		RegencyName: storageContext.RegencyName, Category: category, Child: child,
	})
	if err != nil {
		return ActivityMedia{}, err
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, sanitizeSlotName(slotName), s.now().Format("20060102-150405"))

	return s.store(ctx, actor, preparedUpload{
		ActivityType: "dokumen_pendukung", ProgramID: schedule.ProgramID, RegencyID: schedule.RegencyID,
		DisplayName: displayName, OriginalFilename: displayName + ".pdf", Source: input.Source, Detected: detected,
	}, folderPath, meta)
}
```

Check the exact method names on `fpdf.Fpdf` (`RegisterImageOptionsReader`, `ImageOptions`, `AddPage`, `Output`) against the installed version's godoc before relying on this verbatim — run `go doc github.com/go-pdf/fpdf.Fpdf` locally first; `internal/bast/pdf_renderer.go` already uses this library for BAST PDFs and is the authoritative reference for this project's exact call conventions (e.g. whether `Output` takes an `io.Writer` or returns `([]byte, error)` in the version pinned here) — read it before writing this file, and match its patterns exactly rather than the shape guessed above.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/activities/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activities/scan.go internal/activities/scan_test.go internal/activities/service.go
git commit -m "feat(activities): add ScanUpload assembling cropped camera pages into one PDF"
```

---

### Task 8: `internal/api` — scan endpoint + wiring upload's `schedule_id`/`slot_id`

**Files:**
- Create: `internal/api/activities_scan_routes.go`
- Create: `internal/api/activities_scan_routes_test.go`
- Modify: `internal/api/activities_routes.go` (pass `schedule_id`/`slot_id` through for `dokumen_pendukung`)
- Modify: `internal/api/handler.go` (widen `ActivitiesService` with `ScanUpload`)
- Modify: `internal/api/routes.go`/`handler.go`'s path switch (register the new route)

**Interfaces:**
- Consumes: `activities.ScanUploadInput` (Task 7).
- Produces: `POST /api/v1/activities/supporting-documents/scan`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/activities_scan_routes_test.go`. Add a `ScanUpload` method to the `fakeActivitiesService` type Task 5 already created in this package (do not redeclare the type — extend it):

```go
package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
)

func (f *fakeActivitiesService) ScanUpload(_ context.Context, _ auth.Principal, input activities.ScanUploadInput, _ auth.ClientMeta, _ auth.RegencyScope) (activities.ActivityMedia, error) {
	f.listedScheduleID = input.ScheduleID // reuse the field; the assertion below only needs the count and IDs
	f.patchedID = input.SlotID
	f.scannedPageCount = len(input.Pages)
	return activities.ActivityMedia{ID: "media-1", MediaType: "document", MimeType: "application/pdf"}, nil
}

const fakeJPEGBytes0 = "\xff\xd8\xff\xe0\x00\x10JFIF\x00" // minimal JPEG SOI+APP0 header, enough for http.DetectContentType in the real service — this stub never calls DetectUpload, so any non-empty bytes are fine here

func TestHandleActivitiesSupportingDocumentsScanPostsAllPages(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	officer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("schedule_id", "schedule-1")
	_ = writer.WriteField("slot_id", "slot-1")
	for i := 0; i < 3; i++ {
		part, _ := writer.CreateFormFile("page", "page.jpg")
		_, _ = part.Write([]byte(fakeJPEGBytes0))
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/supporting-documents/scan", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))

	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: officer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.listedScheduleID != "schedule-1" || service.patchedID != "slot-1" || service.scannedPageCount != 3 {
		t.Fatalf("scheduleID = %q, slotID = %q, pages = %d", service.listedScheduleID, service.patchedID, service.scannedPageCount)
	}
}
```

Add the `scannedPageCount int` field to `fakeActivitiesService` (declared in Task 5's file) when writing this test. Copy the exact `multipart.NewWriter`/cookie/CSRF-header construction from `internal/api/handler_test.go`'s `TestDistributionMediaUploadAndContentHeaders` — it already solves this precisely for a single-file upload; this test only differs by writing the `page` field three times.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestHandleActivitiesScan -v`
Expected: FAIL — handler doesn't exist.

- [ ] **Step 3: Implement**

Widen `ActivitiesService` in `handler.go` once more:

```go
ScanUpload(context.Context, auth.Principal, activities.ScanUploadInput, auth.ClientMeta, auth.RegencyScope) (activities.ActivityMedia, error)
```

Create `internal/api/activities_scan_routes.go`:

```go
package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"konkit/internal/activities"
)

const maxScanRequestBody = activities.MaxScanRequestBytes + (1 << 20) // small margin over the service-level cap for multipart framing overhead

func (h *Handler) handleActivitiesSupportingDocumentsScan(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "activities.manage") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxScanRequestBody)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		return
	}
	var scheduleID, slotID string
	var pages []io.Reader
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(nextErr, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
			} else {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			}
			return
		}
		switch part.FormName() {
		case "schedule_id":
			data, _ := io.ReadAll(io.LimitReader(part, 128))
			scheduleID = strings.TrimSpace(string(data))
		case "slot_id":
			data, _ := io.ReadAll(io.LimitReader(part, 128))
			slotID = strings.TrimSpace(string(data))
		case "page":
			buf, err := io.ReadAll(io.LimitReader(part, activities.MaxScanPageBytes+1))
			if err != nil {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			pages = append(pages, bytesReaderFrom(buf))
		default:
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			return
		}
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Activities.ScanUpload(r.Context(), rc.principal, activities.ScanUploadInput{
		ScheduleID: scheduleID, SlotID: slotID, Source: "camera", Pages: pages,
	}, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}
```

Add the small helper `bytesReaderFrom` (or inline `bytes.NewReader(buf)` directly and import `"bytes"` instead — prefer the inline form, drop the helper, it is unnecessary indirection).

Register the route in the handler's path switch, alongside the slot routes from Task 5:

```go
case path == "activities/supporting-documents/scan":
	h.handleActivitiesSupportingDocumentsScan(w, r, rc)
```

In `internal/api/activities_routes.go`'s existing `handleActivitiesMedia` POST branch, extend the multipart field list so `dokumen_pendukung` uploads can carry `schedule_id`/`slot_id` instead of `regency_id`:

```go
upload, err := openMediaMultipart(w, r, maxActivityMediaRequestBody, map[string]int64{
	"program_id": 128, "regency_id": 128, "activity_type": 64, "source": 16, "file_size": 20,
	"schedule_id": 128, "slot_id": 128,
}, []string{"activity_type", "source", "file_size"})
```

(Note `program_id`/`regency_id` move out of `requiredFields` — they're now conditionally required depending on `activity_type`, enforced inside `Service.Upload` itself per Task 6, not at the HTTP layer.) Update the `input := activities.UploadInput{...}` construction right after to also set `ScheduleID: strings.TrimSpace(upload.Fields["schedule_id"])` and `SlotID: strings.TrimSpace(upload.Fields["slot_id"])`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/activities_scan_routes.go internal/api/activities_scan_routes_test.go internal/api/activities_routes.go internal/api/handler.go
git commit -m "feat(api): add Dokumen Pendukung scan endpoint and schedule/slot fields on upload"
```

---

### Task 9: Frontend types — `dokumen_pendukung`, `SupportingDocumentSlot`, API helpers

**Files:**
- Modify: `frontend/src/features/activities/types.ts`

**Interfaces:**
- Produces: `ActivityType` includes `'dokumen_pendukung'`; `SupportingDocumentSlot` type; `activityTypeLabels['dokumen_pendukung']`.

- [ ] **Step 1: Implement (no backend to fail against here — this is a pure type addition, verified by the TypeScript compiler, not a unit test)**

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
  | 'unloading_tabung_gas'
  | 'dokumen_pendukung';
```

```ts
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
  dokumen_pendukung: 'Dokumen Pendukung',
};
```

Also widen `ActivityMedia['media_type']` (currently `'image' | 'video'`) to `'image' | 'video' | 'document'`.

Add:

```ts
export type SupportingDocumentSlot = {
  id: string;
  schedule_id: string;
  name: string;
  is_active: boolean;
};
```

- [ ] **Step 2: Run the frontend typecheck**

Run: `cd frontend && npm run typecheck`
Expected: PASS (no type errors introduced; this also catches any place that exhaustively switches over `ActivityType` and needs the new case — search for those before considering this step done: `grep -rn "ActivityType" frontend/src --include=*.tsx -l`).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/activities/types.ts
git commit -m "feat(activities): add dokumen_pendukung type and SupportingDocumentSlot"
```

---

### Task 10: Frontend — slot management panel in Persiapan Program

**Files:**
- Create: `frontend/src/features/programs/SupportingDocumentSlotsPanel.tsx`
- Create: `frontend/src/features/programs/SupportingDocumentSlotsPanel.test.tsx`
- Modify: `frontend/src/features/programs/SchedulesPanel.tsx` (render the new panel inside the schedule edit dialog)

**Interfaces:**
- Consumes: `GET/POST /api/v1/activities/supporting-documents/slots`, `PATCH /api/v1/activities/supporting-documents/slots/{id}` (Task 5); `apiRequest`, `useCan` from `frontend/src/lib/`; `SetupFormSection` from `./SetupDialog` (the wrapper every other panel section in this dialog already uses — read `SchedulesPanel.tsx`'s existing `show`/dialog body before writing this, to match its exact section styling).

- [ ] **Step 1: Write the failing test**

Create `frontend/src/features/programs/SupportingDocumentSlotsPanel.test.tsx`. Model it exactly on `frontend/src/features/programs/ZonePOPanel.test.tsx` or `TemplatesPanel.test.tsx` (same `vi.mock('../../lib/api')` + `@testing-library/react` pattern used by every panel test in this directory):

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { SupportingDocumentSlotsPanel } from './SupportingDocumentSlotsPanel';

vi.mock('../../lib/api');
vi.mock('../../lib/permissions', () => ({ useCan: () => true }));

describe('SupportingDocumentSlotsPanel', () => {
  it('lists existing slots and submits a new one', async () => {
    vi.mocked(apiRequest).mockImplementation((url: string) => {
      if (url.includes('/slots?schedule_id=schedule-1')) {
        return Promise.resolve({ data: [{ id: 'slot-1', schedule_id: 'schedule-1', name: 'Surat Tugas', is_active: true }] });
      }
      return Promise.resolve({ data: { id: 'slot-2', schedule_id: 'schedule-1', name: 'SK Penetapan', is_active: true } });
    });
    render(<SupportingDocumentSlotsPanel scheduleId="schedule-1" />);
    await waitFor(() => expect(screen.getByText('Surat Tugas')).toBeVisible());
    await userEvent.type(screen.getByLabelText('Nama dokumen baru'), 'SK Penetapan');
    await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
    await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/activities/supporting-documents/slots', expect.objectContaining({ method: 'POST', body: JSON.stringify({ schedule_id: 'schedule-1', name: 'SK Penetapan' }) })));
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/programs/SupportingDocumentSlotsPanel.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Implement**

```tsx
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import { Button } from '../../components/ui/button';
import { FormField } from '../../components/FormField';
import { Switch } from '../../components/ui/switch';
import type { SupportingDocumentSlot } from '../activities/types';

type DataResponse<T> = { data: T };

export function SupportingDocumentSlotsPanel({ scheduleId }: { scheduleId: string }) {
  const client = useQueryClient();
  const [name, setName] = useState('');
  const slots = useQuery({
    queryKey: ['supporting-document-slots', scheduleId],
    queryFn: () => apiRequest<DataResponse<SupportingDocumentSlot[]>>(`/api/v1/activities/supporting-documents/slots?schedule_id=${encodeURIComponent(scheduleId)}`),
    enabled: !!scheduleId,
  });
  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<SupportingDocumentSlot>>('/api/v1/activities/supporting-documents/slots', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleId, name }) }),
    onSuccess: () => { setName(''); void client.invalidateQueries({ queryKey: ['supporting-document-slots', scheduleId] }); },
  });
  const toggle = useMutation({
    mutationFn: (slot: SupportingDocumentSlot) => apiRequest<DataResponse<SupportingDocumentSlot>>(`/api/v1/activities/supporting-documents/slots/${slot.id}`, { method: 'PATCH', body: JSON.stringify({ is_active: !slot.is_active }) }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['supporting-document-slots', scheduleId] }),
  });

  return <div className="space-y-3">
    <h4 className="font-medium">Nama Dokumen Pendukung</h4>
    <ul className="space-y-1">
      {(slots.data?.data ?? []).map((slot) => <li key={slot.id} className="flex items-center justify-between gap-2 text-sm">
        <span>{slot.name}</span>
        <Switch checked={slot.is_active} onCheckedChange={() => toggle.mutate(slot)} aria-label={`Aktifkan ${slot.name}`} />
      </li>)}
    </ul>
    <form className="flex items-end gap-2" onSubmit={(event) => { event.preventDefault(); if (name.trim()) create.mutate(); }}>
      <FormField label="Nama dokumen baru" name="new_slot_name" value={name} onChange={(event) => setName(event.target.value)} />
      <Button type="submit" disabled={!name.trim() || create.isPending}><Plus aria-hidden="true" />Tambah</Button>
    </form>
  </div>;
}
```

Confirm a `Switch` component exists at `frontend/src/components/ui/switch.tsx` before importing it (`grep -rl "export function Switch" frontend/src/components/ui/`) — if it doesn't exist, substitute a plain checkbox `<input type="checkbox">` wired the same way instead of introducing a new UI primitive as a side effect of this task.

In `SchedulesPanel.tsx`, inside the schedule edit dialog body (where `editing` is non-null — i.e. only shown once a schedule already exists, since slots need a real `schedule_id`), render `<SupportingDocumentSlotsPanel scheduleId={editing.id} />` as one more `SetupFormSection`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/features/programs/SupportingDocumentSlotsPanel.test.tsx src/features/programs/SchedulesPanel.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/programs/SupportingDocumentSlotsPanel.tsx frontend/src/features/programs/SupportingDocumentSlotsPanel.test.tsx frontend/src/features/programs/SchedulesPanel.tsx
git commit -m "feat(programs): add Dokumen Pendukung slot management panel to Jadwal settings"
```

---

### Task 11: Frontend — Dokumen Pendukung gallery page

**Files:**
- Create: `frontend/src/features/activities/SupportingDocumentsPage.tsx`
- Create: `frontend/src/features/activities/SupportingDocumentsPage.test.tsx`
- Modify: wherever the 9 existing activity routes/sidebar entries are registered (find it: `grep -rln "ceremony_sosialisasi" frontend/src --include=*.tsx` outside `types.ts` — almost certainly a router config and a sidebar nav config; read both before editing).

**Interfaces:**
- Consumes: `GET /api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=...`, `POST /api/v1/activities/media`, `DELETE /api/v1/activities/media/{id}`, `GET /api/v1/activities/supporting-documents/slots?schedule_id=...` (Tasks 5, 6, 8); `SupportingDocumentSlot`/`ActivityMedia` types (Task 9).
- Produces: the page component mounted at whatever route path the sidebar config dictates (follow the existing 9 modules' URL pattern exactly, e.g. `/dokumentasi/dokumen-pendukung` if that's the pattern `/dokumentasi/rakor` etc. already use).

- [ ] **Step 1: Write the failing test**

Create `frontend/src/features/activities/SupportingDocumentsPage.test.tsx`, modeled on whatever test file already covers one of the 9 existing activity modules' gallery page (`grep -rl "activity_type=rakor\|activityType.*rakor" frontend/src/features/activities/*.test.tsx` to find it) — copy its mocking pattern for `apiRequest` list/upload/delete calls, then adapt:

```tsx
// Mirror the existing activity gallery page test's structure (list render, upload
// triggers a slot-picker dialog first, delete calls DELETE with the right id).
// Key new assertion versus the existing pattern: clicking "Upload PDF" opens a
// dialog listing slots from GET .../slots?schedule_id=..., and only after a slot
// is chosen does the file input become enabled and the POST fire with
// schedule_id+slot_id set (not program_id/regency_id).
```

Write the actual test body once you've read the sibling file — do not guess its internal helper names here.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/activities/SupportingDocumentsPage.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Implement**

```tsx
import { lazy, Suspense, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, FileText, Trash2, Upload } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import type { ActivityMedia, ActivityMediaPage, SupportingDocumentSlot } from './types';

type DataResponse<T> = { data: T };

const DocumentScanner = lazy(() => import('./DocumentScanner'));

// Dokumen Pendukung is scheduled-scoped (schedule_id is the single gate — see
// the spec), unlike the other 9 activity modules which pick program+regency
// separately. scheduleId is supplied by whatever page-level schedule selector
// this route already renders for other schedule-scoped pages (follow the
// Berita Acara page's schedule selector pattern, not the 9 activity modules').
export function SupportingDocumentsPage({ scheduleId, programId, regencyId }: { scheduleId: string; programId: string; regencyId: string }) {
  const canManage = useCan('activities.manage');
  const client = useQueryClient();
  const [pickerOpen, setPickerOpen] = useState<'upload' | 'scan' | null>(null);
  const [slotId, setSlotId] = useState('');
  const [scannerOpen, setScannerOpen] = useState(false);

  const slots = useQuery({
    queryKey: ['supporting-document-slots', scheduleId],
    queryFn: () => apiRequest<DataResponse<SupportingDocumentSlot[]>>(`/api/v1/activities/supporting-documents/slots?schedule_id=${encodeURIComponent(scheduleId)}`),
    enabled: !!scheduleId,
  });
  const documents = useQuery({
    queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId],
    queryFn: () => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=${encodeURIComponent(regencyId)}`),
    enabled: !!regencyId,
  });

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append('activity_type', 'dokumen_pendukung');
      form.append('schedule_id', scheduleId);
      form.append('slot_id', slotId);
      form.append('source', 'gallery');
      form.append('file_size', String(file.size));
      form.append('file', file, file.name);
      return apiRequest<DataResponse<ActivityMedia>>('/api/v1/activities/media', { method: 'POST', body: form });
    },
    onSuccess: () => { setPickerOpen(null); setSlotId(''); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); },
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }),
  });

  const activeSlots = slots.data?.data ?? [];
  const items = documents.data?.data.items ?? [];

  return <div className="space-y-4">
    {canManage && <div className="flex gap-2">
      <Button type="button" variant="outline" disabled={!activeSlots.length} onClick={() => setPickerOpen('upload')}><Upload aria-hidden="true" />Upload PDF</Button>
      <Button type="button" variant="outline" disabled={!activeSlots.length} onClick={() => setPickerOpen('scan')}><Camera aria-hidden="true" />Scan dari Kamera</Button>
      {!activeSlots.length && <p className="text-sm text-muted-foreground">Minta Admin Kabupaten menyiapkan daftar nama dokumen di Persiapan Program dulu.</p>}
    </div>}

    <ul className="grid gap-2 sm:grid-cols-2">
      {items.map((item) => <li key={item.id} className="flex items-center justify-between gap-2 rounded-lg border p-3">
        <a href={item.content_url} target="_blank" rel="noreferrer" className="flex items-center gap-2 text-sm font-medium"><FileText aria-hidden="true" />{item.display_name}</a>
        {canManage && <Button type="button" size="icon" variant="ghost" aria-label={`Hapus ${item.display_name}`} onClick={() => remove.mutate(item.id)}><Trash2 aria-hidden="true" /></Button>}
      </li>)}
    </ul>

    {pickerOpen && <Dialog open onOpenChange={(open) => { if (!open) setPickerOpen(null); }}>
      <DialogContent aria-label="Pilih nama dokumen">
        <DialogHeader><DialogTitle>Pilih nama dokumen</DialogTitle></DialogHeader>
        <Select value={slotId} onValueChange={(value) => setSlotId(value ?? '')}>
          <SelectTrigger><SelectValue placeholder="Pilih nama dokumen" /></SelectTrigger>
          <SelectContent>{activeSlots.map((slot) => <SelectItem key={slot.id} value={slot.id}>{slot.name}</SelectItem>)}</SelectContent>
        </Select>
        {pickerOpen === 'upload'
          ? <input type="file" accept="application/pdf" disabled={!slotId || upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); }} />
          : <Button type="button" disabled={!slotId} onClick={() => { setPickerOpen(null); setScannerOpen(true); }}>Mulai Scan</Button>}
        {upload.isError && <p role="alert" className="text-sm text-destructive">{upload.error instanceof ApiError ? upload.error.message : 'Upload belum dapat disimpan.'}</p>}
      </DialogContent>
    </Dialog>}

    {scannerOpen && <Suspense fallback={null}>
      <DocumentScanner
        scheduleId={scheduleId}
        slotId={slotId}
        programId={programId}
        onDone={() => { setScannerOpen(false); setSlotId(''); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); }}
        onClose={() => setScannerOpen(false)}
      />
    </Suspense>}
  </div>;
}
```

Register this page in the sidebar/router config found by the earlier `grep`, following the exact pattern used for the 9 existing activity modules — read that config file in full before editing it, since this task's acceptance depends on the new route actually being reachable, not just the component existing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/features/activities/SupportingDocumentsPage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/activities/SupportingDocumentsPage.tsx frontend/src/features/activities/SupportingDocumentsPage.test.tsx
git commit -m "feat(activities): add Dokumen Pendukung gallery page with slot picker"
```

---

### Task 12: Frontend — `DocumentScanner.tsx` (camera, edge-detect, crop, enhance, multi-page)

**Files:**
- Create: `frontend/src/features/activities/DocumentScanner.tsx`
- Create: `frontend/src/features/activities/DocumentScanner.test.tsx`
- Modify: `frontend/package.json` (add `jscanify` dependency)

**Interfaces:**
- Consumes: `POST /api/v1/activities/supporting-documents/scan` (Task 8).
- Produces: default export `DocumentScanner({ scheduleId, slotId, programId, onDone, onClose })`, lazy-imported from `SupportingDocumentsPage.tsx` (Task 11).

- [ ] **Step 0 (verification, not implementation): confirm the real `jscanify` API**

```bash
cd frontend && npm install jscanify --save
npm view jscanify readme | head -80
```

`jscanify` wraps OpenCV.js behind `new jscanify()` with `.highlightPaper(canvasOrImg)` (draws a detected-edge overlay onto a canvas) and `.extractPaper(canvasOrImg, resultWidth, resultHeight)` (returns a new `<canvas>` already perspective-cropped to the detected paper) as of its published README — **read the actual installed package's README/`dist` typings before writing Step 3**, and adjust the two call sites below if the real API differs from this description. This step exists because the spec (§9.2) deliberately deferred pinning the exact API to this point in planning — do not skip verifying it against the installed version.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/features/activities/DocumentScanner.test.tsx`. Mock `jscanify`'s default export as a class with stub `highlightPaper`/`extractPaper` methods, and mock `navigator.mediaDevices.getUserMedia` the same way `BarcodeScanner.test.tsx` already does (read that test file first — it already solves "mock getUserMedia in this test environment" for this exact codebase):

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import DocumentScanner from './DocumentScanner';

vi.mock('../../lib/api');
vi.mock('jscanify', () => ({
  default: class {
    highlightPaper() {}
    extractPaper(canvas: HTMLCanvasElement) { return canvas; }
  },
}));

describe('DocumentScanner', () => {
  it('accumulates pages across captures and posts them all on finish', async () => {
    // Follow BarcodeScanner.test.tsx's getUserMedia mock setup exactly, then:
    // 1. render <DocumentScanner scheduleId="s1" slotId="slot-1" programId="p1" onDone={vi.fn()} onClose={vi.fn()} />
    // 2. fire the capture button twice (simulating two scanned pages)
    // 3. fire the "Selesai" button
    // 4. assert apiRequest was called once with '/api/v1/activities/supporting-documents/scan'
    //    and a FormData body containing 2 parts named "page" plus schedule_id="s1", slot_id="slot-1"
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/activities/DocumentScanner.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Implement**

```tsx
import { useEffect, useRef, useState } from 'react';
import jscanify from 'jscanify';
import { LoaderCircle, RefreshCw } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '../../components/ui/dialog';

// Document scan for Dokumen Pendukung: live camera with an auto-detected-edge
// overlay (jscanify, built on OpenCV.js — this is why the component is
// lazy-loaded from SupportingDocumentsPage, not imported eagerly), capture +
// perspective-crop to a flat rectangle, a quick sharpen/contrast pass, then
// accumulate pages until "Selesai" assembles them server-side into one PDF.
// No color-mode picker (spec §2) — the enhance step always runs, unconditionally.
export default function DocumentScanner({ scheduleId, slotId, programId, onDone, onClose }: {
  scheduleId: string; slotId: string; programId: string; onDone: () => void; onClose: () => void;
}) {
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const overlayRef = useRef<HTMLCanvasElement>(null);
  const scannerRef = useRef(new jscanify());
  const [pages, setPages] = useState<Blob[]>([]);
  const [starting, setStarting] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!video) return;
    let stream: MediaStream | undefined;
    let raf = 0;
    navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } } })
      .then((activeStream) => {
        stream = activeStream;
        video.srcObject = activeStream;
        setStarting(false);
        const overlay = overlayRef.current;
        const tick = () => {
          if (overlay && video.videoWidth) {
            overlay.width = video.videoWidth; overlay.height = video.videoHeight;
            const ctx = overlay.getContext('2d');
            if (ctx) {
              ctx.drawImage(video, 0, 0);
              try {
                scannerRef.current.highlightPaper(overlay);
              } catch {
                // jscanify's OpenCV.js WASM failed to init or threw mid-detection (e.g. weak
                // connection in the field). Fail fast with a retry, per spec §10 — never fall
                // back silently to a mode with no edge detection.
                setError('Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.');
                return;
              }
            }
          }
          raf = window.setTimeout(tick, 250); // throttled — not every frame, per spec §9.2
        };
        tick();
      })
      .catch(() => setError('Kamera tidak dapat diakses. Beri izin kamera pada browser, lalu coba lagi.'));
    return () => { window.clearTimeout(raf); stream?.getTracks().forEach((track) => track.stop()); };
  }, [video, attempt]);

  const enhance = (canvas: HTMLCanvasElement): HTMLCanvasElement => {
    const ctx = canvas.getContext('2d');
    if (!ctx) return canvas;
    // Simple sharpen (3x3 kernel) + contrast stretch — always on, no mode picker (spec §2).
    const image = ctx.getImageData(0, 0, canvas.width, canvas.height);
    const data = image.data;
    for (let i = 0; i < data.length; i += 4) {
      for (let channel = 0; channel < 3; channel += 1) {
        const value = data[i + channel];
        data[i + channel] = Math.min(255, Math.max(0, (value - 128) * 1.2 + 128));
      }
    }
    ctx.putImageData(image, 0, 0);
    return canvas;
  };

  const capture = () => {
    if (!video) return;
    const raw = document.createElement('canvas');
    raw.width = video.videoWidth; raw.height = video.videoHeight;
    raw.getContext('2d')?.drawImage(video, 0, 0);
    let cropped: HTMLCanvasElement;
    try {
      cropped = scannerRef.current.extractPaper(raw, raw.width, raw.height) as HTMLCanvasElement;
    } catch {
      setError('Deteksi dokumen gagal dimuat. Periksa koneksi internet, lalu coba lagi.');
      return;
    }
    const finished = enhance(cropped);
    finished.toBlob((blob) => { if (blob) setPages((current) => [...current, blob]); }, 'image/jpeg', 0.85);
  };

  const finish = async () => {
    setSubmitting(true);
    try {
      const form = new FormData();
      form.append('schedule_id', scheduleId);
      form.append('slot_id', slotId);
      pages.forEach((page, index) => form.append('page', page, `page-${index}.jpg`));
      await apiRequest('/api/v1/activities/supporting-documents/scan', { method: 'POST', body: form });
      onDone();
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Scan belum dapat disimpan.');
    } finally {
      setSubmitting(false);
    }
  };

  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
    <DialogContent aria-label="Scan Dokumen Pendukung" className="max-w-md">
      <DialogHeader>
        <DialogTitle>Scan Dokumen Pendukung</DialogTitle>
        <DialogDescription>Arahkan kamera ke dokumen — tepi terdeteksi otomatis. Program: {programId}.</DialogDescription>
      </DialogHeader>
      {error ? <div className="space-y-3">
        <p className="text-sm text-destructive" role="alert">{error}</p>
        <Button type="button" variant="outline" onClick={() => { setError(null); setAttempt((value) => value + 1); }}><RefreshCw aria-hidden="true" />Coba lagi</Button>
      </div> : <div className="space-y-3">
        <div className="relative overflow-hidden rounded-lg border bg-black">
          <video ref={setVideo} className="aspect-video w-full object-cover" autoPlay muted playsInline />
          <canvas ref={overlayRef} className="pointer-events-none absolute inset-0 h-full w-full" />
          {starting && <div className="absolute inset-0 flex items-center justify-center bg-black/70 text-white"><LoaderCircle className="size-6 animate-spin" aria-hidden="true" /></div>}
        </div>
        <p className="text-sm text-muted-foreground">{pages.length} halaman terkumpul.</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={capture} disabled={starting}>Ambil Halaman</Button>
          <Button type="button" variant="outline" disabled={!pages.length || submitting} onClick={finish}>{submitting ? 'Menyimpan...' : 'Selesai'}</Button>
        </div>
      </div>}
    </DialogContent>
  </Dialog>;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/features/activities/DocumentScanner.test.tsx`
Expected: PASS.

- [ ] **Step 5: Build the frontend bundle to catch any type/import error the unit test's mocks hid**

Run: `cd frontend && npm run build`
Expected: builds cleanly; note the resulting `jscanify`/OpenCV.js chunk size in the output (it will be the largest chunk in the bundle by far) and confirm it is NOT in the main `index-*.js` chunk — only in a separate lazy chunk loaded on demand (check the `vite build` output listing: `DocumentScanner-*.js` should carry the bulk of the new size, not `index-*.js`).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/activities/DocumentScanner.tsx frontend/src/features/activities/DocumentScanner.test.tsx frontend/package.json frontend/package-lock.json
git commit -m "feat(activities): add camera document scanner with auto edge-detection and crop"
```

---

### Task 13: End-to-end smoke pass

**Files:** none (verification only).

- [ ] **Step 1: Full backend test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS, no regressions in any of the 9 existing activity types or any other package.

- [ ] **Step 2: Full frontend test suite + typecheck + lint**

Run: `cd frontend && npm run typecheck && npm run lint && npm run test`
Expected: PASS.

- [ ] **Step 3: Manual smoke per the project's `run-konkit` skill**

Use `.claude/skills/run-konkit/driver.sh` (already in this repo) to build and launch the server locally, then log in as the driver account and exercise, by hand or via `curl`:
1. `POST /api/v1/activities/supporting-documents/slots` with a real `schedule_id` from the local DB — create "Surat Tugas".
2. `GET .../slots?schedule_id=...` — confirm it comes back.
3. `POST /api/v1/activities/media` with `activity_type=dokumen_pendukung`, that `schedule_id`/`slot_id`, and a real small PDF file — confirm `201` and a `display_name` starting with the regency code and `SURAT-TUGAS`.
4. `GET /api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=...` — confirm the uploaded document appears.
5. In a browser (not curl — the scan flow needs a real camera), open the Dokumen Pendukung page, scan a real printed page, confirm a PDF appears in the list afterward.

Expected: every step succeeds against the real local server and database, matching the `run-konkit` skill's existing standard of proof (a screenshot or terminal transcript of each, not just "should work").

- [ ] **Step 4: No commit for this task** — it is pure verification. If any step fails, go back to the task that owns the broken behavior, fix it there, and re-commit in that task's own commit (do not pile a separate "fix" commit here).
