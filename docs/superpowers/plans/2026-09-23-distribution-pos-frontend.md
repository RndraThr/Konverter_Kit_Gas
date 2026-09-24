# Distribution POS Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the old single-flow distribution UI with one page that drives the 3-POS lifecycle (POS Mesin / POS Dokumen / POS Penyerahan) end to end, close the small backend gaps the 3-POS backend plan left for this follow-up, and fix a live bug in the Template Dokumentasi admin panel.

**Architecture:** One page (`/dokumentasi/pendistribusian`, `DistributionPage.tsx`) with a schedule selector, a unified lookup (slot number or NIK, any status) plus a "Buat Slot Mesin Baru" action, and — once a slot is found or created — three stacked sections (Mesin/Dokumen/Penyerahan) for that one slot: read-only once a stage is done, active for the slot's current stage, locked until its turn. This mirrors the old `RecipientWorkspace` single-page feel, restructured around the new 3-stage lifecycle. Backend gaps closed along the way: `documentation_slots` gains a `stage` column so each section can show only its own evidence; the slot-search endpoint is broadened from "linked only" to "any status" (permission relaxed from `distribution.pos_penyerahan` to `distribution.view`, since it's read-only); POS rejection errors get distinct codes/Indonesian messages; Template Dokumentasi gets a "Pos" selector (its current silent default of `'distribution'` is rejected by the DB since the earlier migration, breaking new-slot creation today).

**Tech Stack:** Go 1.26 (pgx/v5, goose), React 19 + TypeScript, TanStack Query, Vitest + Testing Library, shadcn-style UI components (`@base-ui/react`).

**Spec:** None — per this session's established pattern (see the backend plan, `docs/superpowers/plans/2026-09-21-distribution-pos-backend.md`), the user asked to skip the formal spec.md step to save time/tokens; design was worked out interactively in chat. This plan is the design's only written record.

## Global Constraints

- Work directly on `main`, no isolated worktree or branch — consistent with every prior plan this session. The working tree has unrelated, uncommitted, in-progress work in `internal/recipients`, `internal/api/recipients_routes.go`, and `frontend/src/features/dashboard/*` (a `page_size=all` feature) — every task must `git add` only the files its own step lists, never `-A` or `.`.
- `internal/distribution/models.go`'s `LinkSlotInput.IdentityChangeReason` and the sentinel `ErrIdentityChangeReasonRequired` are dead code: the field is trimmed in `service.go` but never validated or read by `repository.go`'s `LinkSlot` (NIK is used strictly as an exact-match search key against DCP3-imported candidates — it is never written back to `people.nik`, so there is nothing to justify a "reason for changing it"). Per this project's standing preference to keep the schema/code clean, Task 2 removes both rather than building a frontend field for a backend path that does nothing.
- Every new/changed Go sentinel error must get an explicit, Indonesian, specific `writeServiceError` mapping in `internal/api/routes.go` — no falling through to a generic message.
- Reuse existing UI primitives (`components/ui/*`, `components/FormField`, `components/DataState`) and existing patterns (`RecipientDialog.tsx` for the create-slot dialog shape, `TemplatesPanel.tsx`'s slot-row editor for the new "Pos" selector) — do not introduce a second dialog/form styling convention.
- `DocumentationSlot.tsx` (photo upload/remove) is reused unchanged across all 3 sections — sections only ever pass it a stage-filtered slice of `DistributionSlot.documentation`.
- After Task 1 (migration) lands, every later task assumes `documentation_slots.stage` exists and is populated — do not query/serialize it before Task 1 is committed.
- Build/vet/test must stay green after every task: `go build ./... && go vet ./...` for backend tasks; `npm run build` (tsc) and `npx vitest run` (from `frontend/`) for frontend tasks. Two long-documented, pre-existing, unrelated Go test flakes are not this plan's concern: `internal/recipients: TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal`, `internal/web: TestIntegrationLoginDashboardAndLogout`.
- `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable` for Go integration tests; reset via `psql` (`C:\Program Files\PostgreSQL\18\bin\psql.exe`) — terminate connections, `DROP DATABASE`, `CREATE DATABASE` — before any full-suite run, per this session's established fixture-pollution workaround.

---

### Task 1: Migration — `documentation_slots.stage`

**Files:**
- Create: `internal/database/migrations/00011_documentation_slot_stage.sql`

**Interfaces:**
- Produces: `documentation_slots.stage` (`text NOT NULL CHECK (stage IN ('mesin','dokumen','penyerahan'))`), backfilled for every existing row from its matching `documentation_template_slots.stage`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
ALTER TABLE documentation_slots ADD COLUMN stage text;

UPDATE documentation_slots ds SET stage = dts.stage
FROM distribution_slots dsl
JOIN program_schedules ps ON ps.id = dsl.schedule_id
JOIN documentation_template_slots dts ON dts.template_version_id = ps.documentation_template_version_id AND dts.slot_code = ds.slot_code
WHERE ds.distribution_slot_id = dsl.id AND ds.stage IS NULL;

UPDATE documentation_slots SET stage = 'penyerahan' WHERE stage IS NULL;

ALTER TABLE documentation_slots ALTER COLUMN stage SET NOT NULL;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_stage_check CHECK (stage IN ('mesin', 'dokumen', 'penyerahan'));

-- +goose Down
ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_stage_check;
ALTER TABLE documentation_slots DROP COLUMN stage;
```

The backfill UPDATE matches each existing snapshot row back to the template slot it was copied from (`slot_code` + the schedule's `documentation_template_version_id`) — this is the same join path `CreateSlot`'s own snapshot INSERT already uses (see Task 2). Any row that can't be matched (e.g. a template slot that was later deleted/renamed) falls back to `'penyerahan'`, the same fallback Task 1 of the backend plan used for the analogous `documentation_template_slots.stage` backfill.

- [ ] **Step 2: Apply and verify against a reset test DB**

```bash
cd "d:/KSM/Deployment/konkit"
"/c/Program Files/PostgreSQL/18/bin/psql.exe" "postgres://postgres:admin@127.0.0.1:5432/postgres" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='konkit_test';" -c "DROP DATABASE IF EXISTS konkit_test;" -c "CREATE DATABASE konkit_test;"
export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable"
go test ./internal/database/... ./internal/auth/... -count=1
```

Expected: migrations apply cleanly (goose runs 00001-00011), `internal/auth`'s foundation-migration smoke test still passes (it doesn't currently assert on `documentation_slots` columns, so no change needed there — confirm this by reading `internal/auth/repository_integration_test.go`'s `TestIntegrationOperationalMigrationCreatesFoundation` table-name list before assuming).

- [ ] **Step 3: Commit**

```bash
git add internal/database/migrations/00011_documentation_slot_stage.sql
git commit -m "feat(database): add documentation_slots.stage for per-POS evidence filtering"
```

---

### Task 2: `internal/distribution` — expose `stage`, drop dead identity-change-reason code

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository_integration_test.go` (Step 6's new test)

**Interfaces:**
- Consumes: nothing new from other tasks (Task 1's migration).
- Produces: `SlotSummary.Stage` (`json:"stage"`, one of `mesin`/`dokumen`/`penyerahan`) — Task 3 (search relaxation) and every frontend task read this.

- [ ] **Step 1: Add `Stage` to `SlotSummary`**

In `internal/distribution/models.go`, add the field (`internal/distribution/models.go:34-46`):

```go
type SlotSummary struct {
	ID                string      `json:"id,omitempty"`
	Code              string      `json:"code"`
	Label             string      `json:"label"`
	Stage             string      `json:"stage"`
	Status            string      `json:"status"`
	Required          bool        `json:"required,omitempty"`
	MinFiles          int         `json:"min_files,omitempty"`
	MaxFiles          int         `json:"max_files,omitempty"`
	Files             []MediaFile `json:"files,omitempty"`
	InputSource       string      `json:"input_source,omitempty"`
	RequireLocation   bool        `json:"require_location,omitempty"`
	RequireCapturedAt bool        `json:"require_captured_at,omitempty"`
}
```

Remove `IdentityChangeReason` from `LinkSlotInput` (`internal/distribution/models.go:89-99`):

```go
type LinkSlotInput struct {
	ScheduleID       string `json:"schedule_id"`
	SlotNumber       int    `json:"slot_number"`
	NIK              string `json:"nik"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
	SectorIdentifier string `json:"sector_identifier"`
}
```

Remove the now-unused sentinel from the `var (...)` block (`internal/distribution/models.go:13`): delete the `ErrIdentityChangeReasonRequired = errors.New("identity change reason is required")` line entirely.

- [ ] **Step 2: Update `CreateSlot`'s snapshot INSERT to carry `stage`**

In `internal/distribution/repository.go`, `CreateSlot` (around line 210-216):

```go
	if _, err := tx.Exec(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		SELECT $1,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order
		FROM documentation_template_slots WHERE template_version_id=$2
	`, slotID, documentationTemplateID); err != nil {
		return DistributionSlot{}, fmt.Errorf("snapshot documentation slots: %w", err)
	}
```

- [ ] **Step 3: Read `stage` back in `listSlotDocumentation`**

In `internal/distribution/repository.go`, `listSlotDocumentation` (around line 262-284):

```go
func (r *Repository) listSlotDocumentation(ctx context.Context, distributionSlotID string) ([]SlotSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_code, ds.label_snapshot, ds.stage, ds.status, ds.is_required, ds.min_files, ds.max_files, ds.input_source, ds.require_location, ds.require_captured_at
		FROM documentation_slots ds WHERE ds.distribution_slot_id=$1 ORDER BY ds.sort_order
	`, distributionSlotID)
	if err != nil {
		return nil, fmt.Errorf("list slot documentation: %w", err)
	}
	defer rows.Close()
	var summaries []SlotSummary
	for rows.Next() {
		var summary SlotSummary
		if err := rows.Scan(&summary.ID, &summary.Code, &summary.Label, &summary.Stage, &summary.Status, &summary.Required, &summary.MinFiles, &summary.MaxFiles, &summary.InputSource, &summary.RequireLocation, &summary.RequireCapturedAt); err != nil {
			return nil, fmt.Errorf("scan slot documentation: %w", err)
		}
		files, err := r.listMediaFiles(ctx, summary.ID)
		if err != nil {
			return nil, err
		}
		summary.Files = files
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}
```

- [ ] **Step 4: Remove `IdentityChangeReason` trimming from `service.go`**

In `internal/distribution/service.go`, `LinkSlot` (around line 94-116), delete the line `input.IdentityChangeReason = strings.TrimSpace(input.IdentityChangeReason)`.

- [ ] **Step 5: Confirm no test references the removed field/sentinel**

Run `grep -rn "IdentityChangeReason\|ErrIdentityChangeReasonRequired" internal/distribution/`. As of this plan being written, this returns zero matches outside `models.go`/`service.go` (both already handled by Steps 1 and 4) — `LinkSlotInput.IdentityChangeReason` was never actually exercised by any test. If Steps 1-4 landed exactly as written above, this grep should now return nothing at all (not even in `models.go`/`service.go`, since those references were deleted). If it finds something else, fix that reference (delete the struct field from the literal, or the dead assertion) before moving on — do not skip this check just because it was expected to be empty.

- [ ] **Step 6: Add/extend integration coverage for `stage`**

In `internal/distribution/repository_integration_test.go`, extend (or add, if none currently asserts on `Documentation` contents) a test that creates a slot via `CreateSlot` and asserts at least one returned `SlotSummary.Stage` equals `"mesin"` for a template slot seeded with that stage — confirms the snapshot INSERT and the read-back SELECT both carry the column correctly, not just that they compile.

- [ ] **Step 7: Run tests**

```bash
cd "d:/KSM/Deployment/konkit"
go build ./... && go vet ./...
"/c/Program Files/PostgreSQL/18/bin/psql.exe" "postgres://postgres:admin@127.0.0.1:5432/postgres" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='konkit_test';" -c "DROP DATABASE IF EXISTS konkit_test;" -c "CREATE DATABASE konkit_test;"
export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable"
go test ./internal/distribution/... ./internal/api/... -count=1
```

Expected: all pass, including the new stage-carrying test.

- [ ] **Step 8: Commit**

```bash
git add internal/distribution/models.go internal/distribution/repository.go internal/distribution/service.go internal/distribution/repository_integration_test.go
git commit -m "feat(distribution): expose documentation stage on SlotSummary, drop dead identity-change-reason field"
```

(If Step 5's grep found and fixed an unexpected reference in another file, add that file too.)

---

### Task 3: `internal/distribution` + `internal/api` — broaden slot search to any status

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/models.go` (only if `ErrQueryRequired`'s doc comment needs updating — check first, likely no change)
- Modify: `internal/api/handler.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `internal/distribution/pos_penyerahan_test.go` (rename references — search first)

**Interfaces:**
- Consumes: nothing new.
- Produces: `Service.SearchSlot(ctx, scheduleID, query, scope) (DistributionSlot, error)` — replaces `SearchLinkedSlot`. Every frontend task's "load a slot" call hits `GET /api/v1/distribution/slots/search`, now permitted for anyone with `distribution.view` (not just `distribution.pos_penyerahan`), returning a slot of ANY status (`open`/`linked`/`completed`/`cancelled`).

- [ ] **Step 1: Rename and broaden the repository query**

In `internal/distribution/repository.go`, rename `SearchLinkedSlot` to `SearchSlot` and drop the `status='linked'` filter (around line 419-437):

```go
func (r *Repository) SearchSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ($4 OR ps.regency_id::text = ANY($5))
			AND (ds.slot_number::text = $2 OR (p.nik IS NOT NULL AND p.nik = NULLIF($3,'')))
		LIMIT 1
	`, scheduleID, query, digits, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("search slot: %w", err)
	}
	return r.getSlotByID(ctx, id)
}
```

This is a one-line removal (`AND ds.status='linked'`) plus the rename — the business rule that only a *linked* slot may be completed stays enforced where it actually matters: `CompleteSlot`'s own `FOR UPDATE` status check (which independently returns `ErrSlotNotLinked` — unchanged by this task) already re-verifies status at the moment of the mutating action, so relaxing this read-only lookup does not weaken that guarantee.

- [ ] **Step 2: Rename the service method**

In `internal/distribution/service.go` (around line 118-130), rename `SearchLinkedSlot` to `SearchSlot`, calling `s.posPenyerahanRepository.SearchSlot(...)` (the repository field name `posPenyerahanRepository` does not need to change — it still legitimately backs the POS Penyerahan *mutation*, `CompleteSlot`; only the read method it also serves is now general-purpose):

```go
func (s *Service) SearchSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	scheduleID, query = strings.TrimSpace(scheduleID), strings.TrimSpace(query)
	if scheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if query == "" {
		return DistributionSlot{}, ErrQueryRequired
	}
	if s.posPenyerahanRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Penyerahan is unavailable")
	}
	return s.posPenyerahanRepository.SearchSlot(ctx, scheduleID, query, scope)
}
```

Also rename the `posPenyerahanRepository` interface's first method (`internal/distribution/service.go:39-42`) from `SearchLinkedSlot` to `SearchSlot`:

```go
type posPenyerahanRepository interface {
	SearchSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error)
	CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
}
```

- [ ] **Step 3: Update `internal/api/handler.go`'s `DistributionService` interface**

Change (`internal/api/handler.go:94`):

```go
	SearchSlot(context.Context, string, string, auth.RegencyScope) (distribution.DistributionSlot, error)
```

- [ ] **Step 4: Update the route handler and its permission gate**

In `internal/api/distribution_routes.go`, `handleDistributionSlotSearch` (around line 85-107):

```go
func (h *Handler) handleDistributionSlotSearch(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SearchSlot(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("q"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
```

(Only the permission string and the called method name change — method name, signature, and route wiring in `handleDistributionSlot` at line 131 stay the same.)

- [ ] **Step 5: Update `handler_test.go` and the POS Penyerahan test file**

Run `grep -rn "SearchLinkedSlot" internal/api/handler_test.go internal/distribution/` and rename every reference (fake-service method name, test names like `TestDistributionSlotSearch...`) to `SearchSlot`. Add one new test case to `handler_test.go` alongside the existing search-route tests: a caller with only `distribution.view` (no `pos_penyerahan`) hits `GET /api/v1/distribution/slots/search` and gets `200`, not `403` — this is the behavior change this task exists to make, so it needs its own explicit assertion, not just a rename of the old test.

- [ ] **Step 6: Run tests**

```bash
cd "d:/KSM/Deployment/konkit"
go build ./... && go vet ./...
"/c/Program Files/PostgreSQL/18/bin/psql.exe" "postgres://postgres:admin@127.0.0.1:5432/postgres" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='konkit_test';" -c "DROP DATABASE IF EXISTS konkit_test;" -c "CREATE DATABASE konkit_test;"
export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable"
go test ./internal/distribution/... ./internal/api/... -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/distribution/repository.go internal/distribution/service.go internal/api/handler.go internal/api/distribution_routes.go internal/api/handler_test.go internal/distribution/pos_penyerahan_test.go
git commit -m "feat(distribution): broaden slot search to any status, gate on distribution.view"
```

(Adjust the `git add` list to whatever files Step 5's grep actually touched.)

---

### Task 4: `internal/api` — distinct error codes for POS rejection errors

**Files:**
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go` (only if an existing test asserts the old shared `"operation_rejected"` code for one of these four errors — search first)

**Interfaces:**
- Produces: 4 new distinct `(code, message)` pairs the frontend (Task 8-11) matches on to show stage-specific banners.

- [ ] **Step 1: Split the shared case into 4 explicit ones**

In `internal/api/routes.go`, replace the one shared case (`internal/api/routes.go:468`):

```go
	case errors.Is(err, distribution.ErrSlotNotOpen), errors.Is(err, distribution.ErrSlotNotLinked), errors.Is(err, distribution.ErrAlreadyCompleted), errors.Is(err, distribution.ErrPreviouslyReceived):
		writeError(w, http.StatusConflict, "operation_rejected", err.Error())
```

with:

```go
	case errors.Is(err, distribution.ErrPreviouslyReceived):
		writeError(w, http.StatusConflict, "previously_received", "Penerima sudah pernah menerima paket sebelumnya")
	case errors.Is(err, distribution.ErrSlotNotOpen):
		writeError(w, http.StatusConflict, "slot_not_open", "Nomor bagi ini sudah terhubung atau tidak lagi terbuka")
	case errors.Is(err, distribution.ErrSlotNotLinked):
		writeError(w, http.StatusConflict, "slot_not_linked", "Nomor bagi ini belum terhubung ke penerima")
	case errors.Is(err, distribution.ErrAlreadyCompleted):
		writeError(w, http.StatusConflict, "already_completed", "Distribusi untuk nomor bagi ini sudah selesai")
```

- [ ] **Step 2: Check for existing test assertions on the old shared code**

Run `grep -rn '"operation_rejected"' internal/api/handler_test.go`. If any test asserts this code for a POS error (as opposed to the unrelated `administration`/`recipients` errors that still legitimately share `"operation_rejected"` — those cases are untouched by Step 1), update it to assert the new specific code instead.

- [ ] **Step 3: Run tests**

```bash
cd "d:/KSM/Deployment/konkit"
go build ./... && go vet ./...
go test ./internal/api/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/api/routes.go internal/api/handler_test.go
git commit -m "feat(api): distinct error codes and Indonesian messages for POS rejection errors"
```

---

### Task 5: `internal/programs` — validate `stage` instead of silently defaulting

**Files:**
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go` (add a test — check current file structure first for the right place)

**Interfaces:**
- Produces: `SaveDocumentationTemplate` now rejects (`ErrTemplateSlotInvalid`, already an existing sentinel mapped to 400 elsewhere in `routes.go` — confirm this mapping exists before assuming, do not add a new one if it already does) any slot whose `stage` is not one of `mesin`/`dokumen`/`penyerahan`, instead of silently rewriting an empty value to the now-invalid `'distribution'`.

**Context:** Today, `internal/programs/service.go:165-167` defaults an empty `stage` to `"distribution"` — a value the database has rejected since the 3-POS backend migration added a CHECK constraint. Since `TemplatesPanel.tsx` (Task 7) never sent a `stage` value at all until now, every new documentation-template-slot save currently fails with a database constraint violation. This task removes the silent, now-broken default; Task 7 supplies a real value from a new UI selector so the two land together.

- [ ] **Step 1: Replace the default with validation**

In `internal/programs/service.go`, `SaveDocumentationTemplate` (around line 160-176):

```go
	seen := make(map[string]struct{}, len(input.Slots))
	for index := range input.Slots {
		slot := &input.Slots[index]
		slot.SlotCode = strings.ToLower(strings.TrimSpace(slot.SlotCode))
		slot.Label = strings.TrimSpace(slot.Label)
		slot.Stage = strings.ToLower(strings.TrimSpace(slot.Stage))
		slot.InputSource = strings.ToLower(strings.TrimSpace(slot.InputSource))
		slot.Instructions = strings.TrimSpace(slot.Instructions)
		_, duplicate := seen[slot.SlotCode]
		if duplicate || !slotCodePattern.MatchString(slot.SlotCode) || slot.Label == "" || slot.MinFiles < 0 || slot.MaxFiles < slot.MinFiles || !oneOf(slot.InputSource, "camera", "gallery", "both") || !oneOf(slot.Stage, "mesin", "dokumen", "penyerahan") {
			return DocumentationTemplate{}, ErrTemplateSlotInvalid
		}
		seen[slot.SlotCode] = struct{}{}
	}
```

(The only changes: `slot.Stage` is now lowercased/trimmed like `slot.InputSource` instead of defaulted, and the validation `if` gains one more `oneOf(...)` clause. `oneOf` already exists at `internal/programs/service.go:184` — reuse it, do not write a new helper.)

- [ ] **Step 2: Confirm `ErrTemplateSlotInvalid`'s HTTP mapping exists**

Run `grep -n "ErrTemplateSlotInvalid" internal/api/routes.go`. It should already map to a 400 validation response (added when this sentinel was first introduced). If it is missing, add one case following this file's existing pattern for sibling `programs` validation errors — but expect it to already be present, since the sentinel is not new.

- [ ] **Step 3: Write the failing test, then the fix (TDD)**

`internal/programs/service_test.go` already has an almost-identical test for the sibling validation case, `TestSaveDocumentationTemplateRejectsDuplicateSlotCodes` (around line 166) — copy its exact harness (`NewService(&repositoryStub{})`, `auth.Principal{}`, `auth.ClientMeta{}`, the slot type is `DocumentationTemplateSlotInput`, not `DocumentationSlot` — that's the wire-level input type, distinct from the read-model type of the same name used elsewhere in this package). Add, right after that test:

```go
func TestSaveDocumentationTemplateRejectsInvalidStage(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, DocumentationTemplateInput{
		TemplateCode: "DOK-TEST-STAGE", Name: "Uji Stage", ProgramType: ProgramFarmer, Status: "draft",
		Slots: []DocumentationTemplateSlotInput{
			{SlotCode: "bukti", Label: "Bukti", Stage: "distribution", MinFiles: 1, MaxFiles: 1, InputSource: "both"},
		},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrTemplateSlotInvalid) {
		t.Fatalf("expected ErrTemplateSlotInvalid for stage=%q, got %v", "distribution", err)
	}
}
```

`context`, `errors`, and `auth` are already imported at the top of the file (lines 3-9) — no new imports needed.

- [ ] **Step 4: Run tests**

```bash
cd "d:/KSM/Deployment/konkit"
go build ./... && go vet ./...
go test ./internal/programs/... -count=1
```

Expected: new test passes; no existing test broke (search `internal/programs/*_test.go` for any literal that saves a slot without an explicit valid `Stage` — those now need one, since the silent default is gone).

- [ ] **Step 5: Commit**

```bash
git add internal/programs/service.go internal/programs/service_test.go
git commit -m "fix(programs): validate documentation slot stage instead of defaulting to invalid value"
```

---

### Task 6: `frontend/src/features/distribution/types.ts` — rewrite for the 3-POS shape

**Files:**
- Modify: `frontend/src/features/distribution/types.ts`

**Interfaces:**
- Produces: `DistributionSlot`, `SlotSummary` (with `stage`), `CreateSlotInput`, `CandidateMatch`, `LinkSlotInput`, `EquipmentOption`, `DataResponse`, `ScheduleResponse` — every later frontend task imports from this file.

- [ ] **Step 1: Replace the file contents**

```ts
import type { Schedule } from '../programs/types';

export type MediaFile = { id: string; slot_id: string; original_filename: string; mime_type: string; byte_size: number; source: string; status: string; content_url: string; captured_at?: string };

export type SlotSummary = {
  id?: string; code: string; label: string; stage: 'mesin' | 'dokumen' | 'penyerahan'; status: string;
  required?: boolean; min_files?: number; max_files?: number;
  input_source?: 'camera' | 'gallery' | 'both'; require_location?: boolean; require_captured_at?: boolean; files?: MediaFile[];
};

export type DistributionSlot = {
  id: string; schedule_id: string; slot_number: number; status: 'open' | 'linked' | 'completed' | 'cancelled';
  allocation_id?: string; full_name?: string; nik?: string;
  machine_option_code?: string; machine_serial_number?: string;
  hose_option_code?: string; hose_serial_number?: string; converter_serial_number?: string;
  documentation: SlotSummary[]; distributed_at?: string; created_at: string; updated_at: string;
};

export type CreateSlotInput = {
  schedule_id: string;
  machine_option_code: string; machine_serial_number: string;
  hose_option_code: string; hose_serial_number: string; converter_serial_number: string;
};

export type CandidateMatch = {
  allocation_id: string; full_name: string; nik: string;
  sector_identifier: string; sector_identifier_type: string;
  address: string; village: string; district: string; phone_number: string;
  program_type: 'farmer' | 'fisherman';
};

export type LinkSlotInput = {
  schedule_id: string; slot_number: number; nik: string;
  address: string; village: string; district: string; phone_number: string; sector_identifier: string;
};

export type EquipmentOption = { code: string; brand: string; type?: string; spec?: string };

export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
```

- [ ] **Step 2: Confirm the type-check fails everywhere else first (expected, fixed by later tasks)**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx tsc --noEmit
```

Expected: errors in `DistributionPage.tsx`, `RecipientWorkspace.tsx`, `RecipientSearch.tsx`, `DocumentationSlot.tsx`, and their test files — every one of those is rewritten or removed by Tasks 8-11. This step exists only to confirm Task 6's types compile on their own and to get a baseline list of what's left; do not attempt to fix the downstream errors here.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/distribution/types.ts
git commit -m "feat(distribution): rewrite frontend types for the 3-POS slot shape"
```

(The rest of the package will not build until Tasks 8-11 land — that is expected and matches this session's established pattern from the backend plan, where intermediate commits left `internal/api` non-building until the final task. Do not run the frontend test suite as a gate until Task 11.)

---

### Task 7: `frontend/src/features/programs` — Template Dokumentasi "Pos" selector

**Files:**
- Modify: `frontend/src/features/programs/types.ts`
- Modify: `frontend/src/features/programs/TemplatesPanel.tsx`
- Modify: `frontend/src/features/programs/ProgramSetupPage.test.tsx`

**Interfaces:**
- Produces: `DocumentationSlot.stage` narrowed to `'mesin' | 'dokumen' | 'penyerahan'`; a working "Pos" selector in the slot editor, fixing the live bug where new slots fail to save.

- [ ] **Step 1: Narrow the type**

In `frontend/src/features/programs/types.ts`, change (`frontend/src/features/programs/types.ts:8`):

```ts
export type DocumentationSlot = { slot_code: string; label: string; stage: 'mesin' | 'dokumen' | 'penyerahan'; is_required: boolean; min_files: number; max_files: number; input_source: 'camera' | 'gallery' | 'both'; require_location: boolean; require_captured_at: boolean; instructions?: string; sort_order: number };
```

- [ ] **Step 2: Fix the default and add the selector**

In `frontend/src/features/programs/TemplatesPanel.tsx`, change `newSlot`'s default (`frontend/src/features/programs/TemplatesPanel.tsx:22`):

```ts
const newSlot = (index = 0): DocumentationSlot => ({ slot_code: '', label: '', stage: 'mesin', is_required: true, min_files: 1, max_files: 1, input_source: 'both', require_location: false, require_captured_at: false, sort_order: (index + 1) * 10 });
```

In the slot-row editor (`frontend/src/features/programs/TemplatesPanel.tsx:139`), add a "Pos" `Select` right after the existing "Sumber" `Select`, following the exact same inline pattern:

```tsx
<div className="grid min-w-0 gap-2"><Label id={`slot-stage-${index}`}>Pos</Label><Select value={slot.stage} onValueChange={(value) => updateSlot(index, { stage: (value ?? 'mesin') as DocumentationSlot['stage'] })}><SelectTrigger className="w-full" aria-labelledby={`slot-stage-${index}`}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="mesin">POS Mesin</SelectItem><SelectItem value="dokumen">POS Dokumen</SelectItem><SelectItem value="penyerahan">POS Penyerahan</SelectItem></SelectContent></Select></div>
```

Insert it as a new sibling within the same `<div className="slotRow" ...>` block, immediately after the "Sumber" `Select`'s closing `</div>` and before the "Minimal" `FormField`.

- [ ] **Step 3: Add a regression test for the save-payload bug**

There is no `TemplatesPanel.test.tsx` — `TemplatesPanel` is only ever exercised indirectly through `frontend/src/features/programs/ProgramSetupPage.test.tsx` (confirmed via `find frontend/src/features/programs -iname "*.test.tsx"`). No existing test in that file opens "Tambah dokumentasi" or submits it. Add one, following the file's established `renderPage(...)` + `apiRequest` mock pattern (see `manages package template equipment options...` around line 263 for the closest existing shape — same tab, same `SetupDialog`, different underlying template type):

```tsx
test('saves a new documentation slot with a valid Pos stage', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=templates' });
  await userEvent.click(await screen.findByRole('tab', { name: 'Template' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah dokumentasi' }));

  await userEvent.type(screen.getByRole('textbox', { name: 'Kode template' }), 'dok-baru');
  await userEvent.type(screen.getByRole('textbox', { name: 'Nama template' }), 'dok baru');
  await userEvent.type(screen.getByRole('textbox', { name: 'Kode slot' }), 'bukti_mesin');
  await userEvent.type(screen.getByRole('textbox', { name: 'Judul' }), 'Bukti mesin');

  let savedBody: { slots: Array<{ stage: string }> } | undefined;
  vi.mocked(apiRequest).mockImplementation(((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/documentation-templates' && init?.method === 'POST') {
      savedBody = JSON.parse(init.body as string);
      return Promise.resolve({ data: { id: 'document-2', ...savedBody } });
    }
    return Promise.resolve(responses[path] ?? { data: [] });
  }) as typeof apiRequest);

  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
  await waitFor(() => expect(savedBody).toBeDefined());
  expect(savedBody!.slots[0].stage).toBe('mesin');
});
```

`renderPage`'s own `request` option (line 27-31) only forwards `path`, not `init`, so it cannot see the POST body — this test instead re-installs `apiRequest`'s mock implementation right before the save click, with full access to `init`, the same technique the file already applies (`vi.mocked(apiRequest).mockImplementation(...)`, called once inside `renderPage` itself at line 28) just invoked a second time, later, for this one assertion. This is the regression test for the bug this task fixes: before Tasks 5/7, the saved `stage` would have been `'distribution'` (rejected by the database) because nothing set it and no UI existed to change it.

- [ ] **Step 4: Run tests**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx vitest run src/features/programs
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/programs/types.ts frontend/src/features/programs/TemplatesPanel.tsx frontend/src/features/programs/ProgramSetupPage.test.tsx
git commit -m "fix(programs): add Pos selector to documentation template slots, fix invalid default"
```

---

### Task 8: `DistributionPage.tsx` — schedule selector, unified lookup, create-slot dialog

**Files:**
- Modify: `frontend/src/features/distribution/DistributionPage.tsx`
- Modify: `frontend/src/features/distribution/Distribution.module.css`

**Interfaces:**
- Consumes: `types.ts` (Task 6).
- Produces: `DistributionPage` renders the schedule selector, the lookup bar, the create-slot dialog, and hosts `<slot state>` for Tasks 9-11's section components (imported as placeholders in this task — Task 8 defines `DistributionPage`'s shape and state; Tasks 9-11 fill in the 3 section components it renders). To keep this task's own build green in isolation, Task 8 creates minimal stub versions of the 3 section components (a single `<p>` showing the slot's status) that Tasks 9-11 then replace — this mirrors how the backend plan's Tasks 5-8 layered POS methods onto a package that had to keep compiling at every step.

- [ ] **Step 1: Create the 3 stub section components**

`frontend/src/features/distribution/SlotMesinSection.tsx`:

```tsx
import type { DistributionSlot } from './types';

export function SlotMesinSection({ slot }: { slot: DistributionSlot }) {
  return <section aria-label="POS Mesin"><p>Slot #{slot.slot_number} — {slot.status}</p></section>;
}
```

`frontend/src/features/distribution/SlotDokumenSection.tsx` and `SlotPenyerahanSection.tsx`: identical stub shape, `aria-label="POS Dokumen"` / `aria-label="POS Penyerahan"` respectively, same props and body. Tasks 10 and 11 replace these bodies; Task 8 only needs them to exist and accept `{ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }` (add `onChanged` to all three stub signatures now, even though the stub body doesn't call it, so Task 8's `DistributionPage` can wire the prop once and Tasks 9-11 don't need to touch the call site).

- [ ] **Step 2: Rewrite `DistributionPage.tsx`**

```tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { Search } from 'lucide-react';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CreateSlotInput, DataResponse, DistributionSlot, EquipmentOption, ScheduleResponse } from './types';
import { SlotMesinSection } from './SlotMesinSection';
import { SlotDokumenSection } from './SlotDokumenSection';
import { SlotPenyerahanSection } from './SlotPenyerahanSection';
import styles from './Distribution.module.css';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { FormField } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

const emptyCreateInput = (scheduleID: string): CreateSlotInput => ({ schedule_id: scheduleID, machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_serial_number: '' });

export function DistributionPage() {
  const queryClient = useQueryClient();
  const canCreateSlot = useCan('distribution.pos_mesin');
  const [scheduleID, setScheduleID] = useState('');
  const [query, setQuery] = useState('');
  const [slot, setSlot] = useState<DistributionSlot | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [createInput, setCreateInput] = useState<CreateSlotInput>(() => emptyCreateInput(''));

  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const selectedSchedule = schedules.data?.data.find((schedule) => schedule.id === scheduleID);
  const machineOptions = ((selectedSchedule?.package_template?.values as { machine_options?: EquipmentOption[] } | undefined)?.machine_options) ?? [];
  const hoseOptions = ((selectedSchedule?.package_template?.values as { hose_options?: EquipmentOption[] } | undefined)?.hose_options) ?? [];

  const search = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/search?schedule_id=${encodeURIComponent(scheduleID)}&q=${encodeURIComponent(query)}`),
    onSuccess: ({ data }) => setSlot(data),
  });

  const create = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>('/api/v1/distribution/slots', { method: 'POST', body: JSON.stringify(createInput) }),
    onSuccess: ({ data }) => { setSlot(data); setCreateOpen(false); void queryClient.invalidateQueries({ queryKey: ['distribution'] }); },
  });

  const changeSchedule = (value: string) => { setScheduleID(value); setQuery(''); setSlot(null); setCreateInput(emptyCreateInput(value)); };
  const submitSearch = (event: FormEvent) => { event.preventDefault(); setSlot(null); search.mutate(); };
  const openCreate = () => { setCreateInput(emptyCreateInput(scheduleID)); setCreateOpen(true); };
  const submitCreate = (event: FormEvent) => { event.preventDefault(); create.mutate(); };
  const onSlotChanged = (next: DistributionSlot) => setSlot(next);

  return <div className={`page ${styles.page}`}>
    <PageHeader title="Pendistribusian" description="Tandai mesin, hubungkan penerima, dan selesaikan serah terima dalam satu halaman." context={selectedSchedule ? <span className={styles.context}><strong>{selectedSchedule.regency?.document_code}</strong>{selectedSchedule.name}</span> : undefined} />
    <section className={styles.lookup} aria-label="Cari atau buat nomor bagi">
      <div className={styles.scheduleField}>
        <Label id="distribution-schedule-label">Jadwal distribusi</Label>
        <Select value={scheduleID} onValueChange={(value) => changeSchedule(value ?? '')}>
          <SelectTrigger className="w-full" aria-labelledby="distribution-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
          <SelectContent>{schedules.data?.data.filter((schedule) => schedule.status === 'active').map((schedule) => <SelectItem key={schedule.id} value={schedule.id}>{schedule.regency?.name} / {schedule.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <form className={styles.searchArea} onSubmit={submitSearch}>
        <div className={styles.searchField}>
          <Search />
          <input aria-label="Nomor bagi atau NIK" placeholder="Nomor bagi atau NIK" disabled={!scheduleID} value={query} onChange={(event) => setQuery(event.target.value)} />
        </div>
        <div className={styles.lookupActions}>
          <Button type="submit" disabled={!scheduleID || !query.trim() || search.isPending}>{search.isPending ? 'Mencari...' : 'Cari'}</Button>
          {canCreateSlot && <Button type="button" variant="outline" disabled={!scheduleID} onClick={openCreate}>Buat Slot Mesin Baru</Button>}
        </div>
      </form>
    </section>
    {search.isError && <DataState kind="error" title="Nomor bagi tidak ditemukan" description={search.error instanceof ApiError ? search.error.message : 'Periksa nomor bagi atau NIK, lalu coba lagi.'} />}
    {slot && <div className={styles.slotSections}>
      <SlotMesinSection slot={slot} onChanged={onSlotChanged} />
      <SlotDokumenSection slot={slot} onChanged={onSlotChanged} />
      <SlotPenyerahanSection slot={slot} onChanged={onSlotChanged} />
    </div>}

    <Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogContent aria-label="Buat Slot Mesin Baru" className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto p-0">
      <form onSubmit={submitCreate}>
        <DialogHeader className="border-b p-5"><DialogTitle>Buat Slot Mesin Baru</DialogTitle><DialogDescription>Catat perlengkapan yang dipasang pada unit mesin sebelum penerima diketahui.</DialogDescription></DialogHeader>
        <div className="grid gap-4 p-5 sm:grid-cols-2">
          <div className="grid min-w-0 gap-2"><Label id="create-machine-label">Merk/Tipe Mesin</Label><Select value={createInput.machine_option_code} onValueChange={(value) => setCreateInput({ ...createInput, machine_option_code: value ?? '' })}><SelectTrigger className="w-full" aria-labelledby="create-machine-label"><SelectValue placeholder="Pilih mesin" /></SelectTrigger><SelectContent>{machineOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.type}</SelectItem>)}</SelectContent></Select></div>
          <FormField label="Serial Number Mesin" name="machine_serial_number" value={createInput.machine_serial_number} onChange={(event) => setCreateInput({ ...createInput, machine_serial_number: event.target.value })} />
          <div className="grid min-w-0 gap-2"><Label id="create-hose-label">Merk/Spesifikasi Selang</Label><Select value={createInput.hose_option_code} onValueChange={(value) => setCreateInput({ ...createInput, hose_option_code: value ?? '' })}><SelectTrigger className="w-full" aria-labelledby="create-hose-label"><SelectValue placeholder="Pilih selang" /></SelectTrigger><SelectContent>{hoseOptions.map((option) => <SelectItem key={option.code} value={option.code}>{option.brand} {option.spec}</SelectItem>)}</SelectContent></Select></div>
          <FormField label="Serial Number Selang" name="hose_serial_number" value={createInput.hose_serial_number} onChange={(event) => setCreateInput({ ...createInput, hose_serial_number: event.target.value })} />
          <FormField className="sm:col-span-2" label="Serial Number Konkit/Reducer" name="converter_serial_number" value={createInput.converter_serial_number} onChange={(event) => setCreateInput({ ...createInput, converter_serial_number: event.target.value })} />
          {create.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Slot belum dapat dibuat.'}</AlertDescription></Alert>}
        </div>
        <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={create.isPending} type="submit">{create.isPending ? 'Membuat...' : 'Buat Slot'}</Button></DialogFooter>
      </form>
    </DialogContent></Dialog>
  </div>;
}
```

- [ ] **Step 3: Add the new CSS classes**

In `frontend/src/features/distribution/Distribution.module.css`, replace the `.lookup` rule (line 4) to drop the old 2-column `RecipientSearch` layout and add a lookup-actions row and the sections stack:

```css
.lookup { display: grid; grid-template-columns: minmax(230px, .5fr) minmax(320px, 1fr); align-items: end; gap: 16px; padding: 18px 0 24px; border-top: 1px solid var(--dashboard-line); border-bottom: 1px solid var(--dashboard-line); }
.searchArea { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 10px; }
.lookupActions { display: flex; gap: 10px; }
.slotSections { display: grid; gap: 20px; margin-top: 28px; }
```

(`.searchField` and its children at lines 7-11 are unchanged and still apply — the new `.searchArea` just wraps `.searchField` plus `.lookupActions` instead of the old bare `<RecipientSearch>` markup. Leave `.results`, `.resultRow`, `.resultNumber`, `.resultIdentity`, `.eligibility`/`.eligibilityBanner`, `.slotDots` and the `.recipientWorkspace`-prefixed rules in place for now — Task 11 removes whichever of these end up genuinely unused once `RecipientSearch.tsx`/`RecipientWorkspace.tsx` are deleted, since some class names are reused by the new section components.)

- [ ] **Step 4: Type-check**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx tsc --noEmit
```

Expected: `DistributionPage.tsx` and the 3 new stub files compile clean. Remaining errors should now be confined to `RecipientWorkspace.tsx`/`RecipientSearch.tsx`/`DocumentationSlot.tsx`/their test files and `DistributionPage.test.tsx` (still referencing the old flow) — all resolved by Tasks 9-11.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/distribution/DistributionPage.tsx frontend/src/features/distribution/Distribution.module.css frontend/src/features/distribution/SlotMesinSection.tsx frontend/src/features/distribution/SlotDokumenSection.tsx frontend/src/features/distribution/SlotPenyerahanSection.tsx
git commit -m "feat(distribution): rewrite DistributionPage around unified slot lookup and create-slot dialog"
```

---

### Task 9: `SlotMesinSection.tsx` — real POS Mesin section

**Files:**
- Modify: `frontend/src/features/distribution/SlotMesinSection.tsx`
- Modify: `frontend/src/features/distribution/Distribution.module.css`

**Interfaces:**
- Consumes: `DistributionSlot`, `SlotSummary` (Task 6); `DocumentationSlot.tsx` (existing, unchanged — reused as-is for photo upload).
- Produces: nothing new consumed by later tasks — this section is self-contained (no create/edit action after `CreateSlot` already ran in Task 8's dialog; this section only displays the result and hosts the mesin-stage evidence upload).

- [ ] **Step 1: Read `DocumentationSlot.tsx` to confirm its props are unchanged**

Confirm `DocumentationSlot` still takes `{ slot: SlotSummary; onChanged: (slot: SlotSummary) => void }` and nothing about its internals depends on a specific `stage` value — it doesn't (it only reads `id`/`label`/`required`/`min_files`/`max_files`/`input_source`/`require_location`/`require_captured_at`/`files`, all stage-independent). No changes needed to that file in this task.

- [ ] **Step 2: Implement the section**

```tsx
import { CheckCircle2, Cog } from 'lucide-react';
import type { DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';

export function SlotMesinSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const documentation = slot.documentation.filter((item) => item.stage === 'mesin');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  return <section className={styles.slotSection} data-state="done" aria-label="POS Mesin">
    <header className={styles.slotSectionHeader}>
      <div><Badge variant="outline">POS Mesin</Badge><h3><Cog aria-hidden="true" />Nomor bagi #{slot.slot_number}</h3></div>
      <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />Tercatat</span>
    </header>
    <dl className={styles.slotSummaryList}>
      <div><dt>Merk/Tipe Mesin</dt><dd>{slot.machine_option_code || '-'}</dd></div>
      <div><dt>Serial Number Mesin</dt><dd>{slot.machine_serial_number || '-'}</dd></div>
      <div><dt>Merk/Spesifikasi Selang</dt><dd>{slot.hose_option_code || '-'}</dd></div>
      <div><dt>Serial Number Selang</dt><dd>{slot.hose_serial_number || '-'}</dd></div>
      <div><dt>Serial Number Konkit/Reducer</dt><dd>{slot.converter_serial_number || '-'}</dd></div>
    </dl>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
  </section>;
}
```

POS Mesin's evidence stays open for upload regardless of the slot's overall status (an operator may need to add a machine photo after the fact) — `data-state="done"` here only means "identity data recorded," styled the same as a completed section but never disables the upload UI beneath it (matching `DocumentationSlot.tsx`'s own `canManage`-gated upload controls, unchanged from Task 8's assumption).

- [ ] **Step 3: Add section CSS**

In `Distribution.module.css`, add:

```css
.slotSection { padding: 20px; border: 1px solid var(--dashboard-line); border-radius: 10px; background: white; }
.slotSection[data-state="locked"] { opacity: .55; background: #f7f8f6; }
.slotSection[data-state="active"] { border-color: var(--green); box-shadow: 0 0 0 1px var(--green); }
.slotSectionHeader { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.slotSectionHeader h3 { display: flex; align-items: center; gap: 8px; margin: 6px 0 0; font-size: 16px; }
.slotSectionStatus { display: inline-flex; align-items: center; gap: 6px; color: var(--green); font-size: 11px; font-weight: 750; }
.slotSectionStatus svg { width: 15px; }
.slotSummaryList { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 8px 15px; margin: 16px 0 0; }
.slotSummaryList div { display: grid; gap: 3px; padding: 8px 0; border-bottom: 1px solid var(--dashboard-line); }
.slotSummaryList dt { color: var(--dashboard-muted); font-size: 11px; }
.slotSummaryList dd { margin: 0; font-size: 12px; font-weight: 650; }
.sectionDocumentation { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 0 24px; margin-top: 16px; }
@media (max-width: 900px) { .slotSummaryList, .sectionDocumentation { grid-template-columns: 1fr; } }
```

- [ ] **Step 4: Type-check**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx tsc --noEmit
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/distribution/SlotMesinSection.tsx frontend/src/features/distribution/Distribution.module.css
git commit -m "feat(distribution): implement POS Mesin section with mesin-stage evidence upload"
```

---

### Task 10: `SlotDokumenSection.tsx` — real POS Dokumen section

**Files:**
- Modify: `frontend/src/features/distribution/SlotDokumenSection.tsx`

**Interfaces:**
- Consumes: `DistributionSlot`, `CandidateMatch`, `LinkSlotInput` (Task 6).

- [ ] **Step 1: Implement the section**

```tsx
import { FormEvent, useState } from 'react';
import { CheckCircle2, Lock, Search, UserCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { CandidateMatch, DataResponse, DistributionSlot, LinkSlotInput } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Alert, AlertDescription } from '@/components/ui/alert';

const emptyLinkInput = (scheduleID: string, slotNumber: number): LinkSlotInput => ({ schedule_id: scheduleID, slot_number: slotNumber, nik: '', address: '', village: '', district: '', phone_number: '', sector_identifier: '' });

export function SlotDokumenSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canLink = useCan('distribution.pos_dokumen');
  const documentation = slot.documentation.filter((item) => item.stage === 'dokumen');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });

  const [nik, setNik] = useState('');
  const [candidate, setCandidate] = useState<CandidateMatch | null>(null);
  const [linkInput, setLinkInput] = useState<LinkSlotInput>(() => emptyLinkInput(slot.schedule_id, slot.slot_number));

  const lookup = useMutation({
    mutationFn: () => apiRequest<DataResponse<CandidateMatch>>(`/api/v1/distribution/candidates?schedule_id=${encodeURIComponent(slot.schedule_id)}&nik=${encodeURIComponent(nik)}`),
    onSuccess: ({ data }) => { setCandidate(data); setLinkInput({ schedule_id: slot.schedule_id, slot_number: slot.slot_number, nik: data.nik, address: data.address, village: data.village, district: data.district, phone_number: data.phone_number, sector_identifier: data.sector_identifier }); },
  });

  const link = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.slot_number}/link?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'POST', body: JSON.stringify(linkInput) }),
    onSuccess: ({ data }) => onChanged(data),
  });

  const submitLookup = (event: FormEvent) => { event.preventDefault(); setCandidate(null); lookup.mutate(); };
  const submitLink = (event: FormEvent) => { event.preventDefault(); link.mutate(); };

  if (slot.status === 'open') {
    if (!canLink) {
      return <section className={styles.slotSection} data-state="locked" aria-label="POS Dokumen"><header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Dokumen</Badge><h3><Lock aria-hidden="true" />Menunggu penerima</h3></div></header></section>;
    }
    return <section className={styles.slotSection} data-state="active" aria-label="POS Dokumen">
      <header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Dokumen</Badge><h3><UserCheck aria-hidden="true" />Hubungkan penerima</h3></div></header>
      <form className={styles.fields} onSubmit={submitLookup}>
        <FormField className={styles.fieldWide} label="NIK Penerima" name="nik" maxLength={16} value={nik} onChange={(event) => setNik(event.target.value.replace(/\D/g, ''))} />
        <Button type="submit" disabled={nik.length !== 16 || lookup.isPending}>{lookup.isPending ? 'Mencari...' : 'Cari di DCP3'}</Button>
      </form>
      {lookup.isError && <Alert variant="destructive"><AlertDescription>{lookup.error instanceof ApiError ? lookup.error.message : 'Kandidat tidak ditemukan.'}</AlertDescription></Alert>}
      {candidate && <form className={styles.fields} onSubmit={submitLink}>
        <FormField className={styles.fieldWide} label="Nama" name="candidate_full_name" value={candidate.full_name} disabled onChange={() => {}} />
        <FormField label={candidate.program_type === 'farmer' ? 'Nomor kartu petani' : 'Nomor KUSUKA'} name="sector_identifier" value={linkInput.sector_identifier} onChange={(event) => setLinkInput({ ...linkInput, sector_identifier: event.target.value.toUpperCase() })} />
        <FormField label="Nomor telepon" name="phone_number" value={linkInput.phone_number} onChange={(event) => setLinkInput({ ...linkInput, phone_number: event.target.value })} />
        <FormField className={styles.fieldWide} label="Alamat" name="address" value={linkInput.address} onChange={(event) => setLinkInput({ ...linkInput, address: event.target.value })} />
        <FormField label="Desa/kelurahan" name="village" value={linkInput.village} onChange={(event) => setLinkInput({ ...linkInput, village: event.target.value })} />
        <FormField label="Kecamatan" name="district" value={linkInput.district} onChange={(event) => setLinkInput({ ...linkInput, district: event.target.value })} />
        {link.isError && <Alert className={styles.fieldWide} variant="destructive"><AlertDescription>{link.error instanceof ApiError ? link.error.message : 'Slot belum dapat dihubungkan.'}</AlertDescription></Alert>}
        <Button className={styles.fieldWide} disabled={link.isPending} type="submit">{link.isPending ? 'Menghubungkan...' : 'Hubungkan ke Nomor Bagi Ini'}</Button>
      </form>}
    </section>;
  }

  return <section className={styles.slotSection} data-state="done" aria-label="POS Dokumen">
    <header className={styles.slotSectionHeader}>
      <div><Badge variant="outline">POS Dokumen</Badge><h3><UserCheck aria-hidden="true" />{slot.full_name}</h3></div>
      <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />Terhubung</span>
    </header>
    <dl className={styles.slotSummaryList}><div><dt>NIK</dt><dd>{slot.nik}</dd></div></dl>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
  </section>;
}
```

`FormField`'s `Props` type is `InputHTMLAttributes<HTMLInputElement> & {...}` (`frontend/src/components/FormField.tsx:5`), so it accepts and forwards `disabled` like any native input — the read-only "Nama" field above is valid as written.

- [ ] **Step 2: Type-check**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/distribution/SlotDokumenSection.tsx
git commit -m "feat(distribution): implement POS Dokumen section with NIK lookup and link form"
```

---

### Task 11: `SlotPenyerahanSection.tsx`, remove old files, final wiring and tests

**Files:**
- Modify: `frontend/src/features/distribution/SlotPenyerahanSection.tsx`
- Delete: `frontend/src/features/distribution/RecipientSearch.tsx`
- Delete: `frontend/src/features/distribution/RecipientWorkspace.tsx`
- Delete: `frontend/src/features/distribution/RecipientWorkspace.test.tsx`
- Modify: `frontend/src/features/distribution/Distribution.module.css` (remove now-dead rules)
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx` (full rewrite)
- Create: `frontend/src/features/distribution/SlotMesinSection.test.tsx`
- Create: `frontend/src/features/distribution/SlotDokumenSection.test.tsx`
- Create: `frontend/src/features/distribution/SlotPenyerahanSection.test.tsx`

**Interfaces:**
- Consumes: everything from Tasks 6-10.
- Produces: a fully building, fully tested `frontend/src/features/distribution` package — the last task in this plan.

- [ ] **Step 1: Implement `SlotPenyerahanSection.tsx`**

```tsx
import { useState } from 'react';
import { CheckCircle2, Lock, PackageCheck } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { formatDate } from '../programs/types';
import type { DataResponse, DistributionSlot } from './types';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

export function SlotPenyerahanSection({ slot, onChanged }: { slot: DistributionSlot; onChanged: (slot: DistributionSlot) => void }) {
  const canComplete = useCan('distribution.pos_penyerahan');
  const documentation = slot.documentation.filter((item) => item.stage === 'penyerahan');
  const updateDocumentation = (next: DistributionSlot['documentation'][number]) => onChanged({ ...slot, documentation: slot.documentation.map((item) => item.code === next.code ? next : item) });
  const [confirmOpen, setConfirmOpen] = useState(false);

  const complete = useMutation({
    mutationFn: () => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/${slot.slot_number}/complete?schedule_id=${encodeURIComponent(slot.schedule_id)}`, { method: 'POST' }),
    onSuccess: ({ data }) => { setConfirmOpen(false); onChanged(data); },
  });

  if (slot.status === 'completed') {
    return <section className={styles.slotSection} data-state="done" aria-label="POS Penyerahan">
      <header className={styles.slotSectionHeader}>
        <div><Badge variant="outline">POS Penyerahan</Badge><h3><PackageCheck aria-hidden="true" />Distribusi selesai</h3></div>
        <span className={styles.slotSectionStatus}><CheckCircle2 aria-hidden="true" />{slot.distributed_at ? formatDate(slot.distributed_at) : 'Selesai'}</span>
      </header>
      <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
    </section>;
  }

  if (slot.status !== 'linked') {
    return <section className={styles.slotSection} data-state="locked" aria-label="POS Penyerahan"><header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Penyerahan</Badge><h3><Lock aria-hidden="true" />Menunggu dokumen selesai</h3></div></header></section>;
  }

  const required = documentation.filter((item) => item.required);
  const blocked = required.some((item) => item.status !== 'complete');

  return <section className={styles.slotSection} data-state="active" aria-label="POS Penyerahan">
    <header className={styles.slotSectionHeader}><div><Badge variant="outline">POS Penyerahan</Badge><h3><PackageCheck aria-hidden="true" />Siap diserahkan</h3></div></header>
    <div className={styles.sectionDocumentation}>{documentation.map((item) => <DocumentationSlot key={item.code} slot={item} onChanged={updateDocumentation} />)}</div>
    {canComplete && <footer className={styles.completion}>
      <div><strong>Konfirmasi penyerahan</strong><span>{blocked ? 'Lengkapi seluruh bukti wajib sebelum konfirmasi.' : 'Semua bukti wajib telah terpenuhi.'}</span></div>
      <Button disabled={blocked} onClick={() => setConfirmOpen(true)}>Selesaikan Distribusi</Button>
    </footer>}
    <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}><AlertDialogContent aria-label="Konfirmasi distribusi">
      <AlertDialogHeader><AlertDialogTitle>Konfirmasi distribusi</AlertDialogTitle><AlertDialogDescription>Pastikan penerima dan bukti penyerahan sudah benar. Aksi ini tidak dapat dibatalkan dari halaman ini.</AlertDialogDescription></AlertDialogHeader>
      {complete.isError && <p role="alert">{complete.error instanceof ApiError ? complete.error.message : 'Distribusi belum dapat diselesaikan.'}</p>}
      <AlertDialogFooter><AlertDialogCancel>Periksa lagi</AlertDialogCancel><Button disabled={complete.isPending} onClick={() => complete.mutate()}>{complete.isPending ? 'Menyelesaikan...' : 'Konfirmasi Penyerahan'}</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </section>;
}
```

Confirm `formatDate` is exported from `../programs/types` (it is — `RecipientWorkspace.tsx:6` already imports it from there today).

- [ ] **Step 2: Delete the old files**

```bash
cd "d:/KSM/Deployment/konkit"
git rm frontend/src/features/distribution/RecipientSearch.tsx frontend/src/features/distribution/RecipientWorkspace.tsx frontend/src/features/distribution/RecipientWorkspace.test.tsx
```

- [ ] **Step 3: Remove now-dead CSS**

Read `Distribution.module.css` in full and delete every rule whose only user was `RecipientSearch.tsx`/`RecipientWorkspace.tsx` and is not reused by the new section components — specifically `.results`, `.resultRow`, `.resultNumber`, `.resultIdentity`, `.eligibility`, `.eligibilityBanner`, `.eligible`/`.incomplete`/`.approval_required`/`.previously_received`/`.identity_conflict`, `.slotDots`, `.slotComplete`/`.slotMissing`, `.searchNote`, `.recipientWorkspace`, `.recipientHeader`, `.distributionNumber`, `.reasons`, `.workspaceColumns`, `.verificationSection`, `.sourcePanel`, `.history`, `.confirmSummary`, `.confirmPackage`, `.confirmSlots`, `.confirmSlotsError`, `.documentationSection`. Keep `.documentationList` → already superseded by `.sectionDocumentation` from Task 9 (delete `.documentationList` too, it has no remaining user), and keep `.documentationSlot`/`.documentationComplete`/`.documentationMissing`/`.mediaGrid`/`.pendingMedia`/`.uploadingIcon`/`.captureActions`/`.formGroup`/`.fields`/`.fieldWide`/`.formActions`/`.completion`/`.completionError`/`.srOnly` — all still used by `DocumentationSlot.tsx` and the new sections. Update the two `@media` blocks at the bottom to drop selectors referencing removed classes.

- [ ] **Step 4: Rewrite `DistributionPage.test.tsx`**

Follow the existing file's established mocking pattern (`vi.mock('../../lib/api', ...)`, `PermissionsProvider`, `QueryClientProvider`, path-matching `apiRequest` mock implementation — copy the harness structure from the current file, replace the fixture data and assertions):

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DistributionPage } from './DistributionPage';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

async function chooseSchedule(name: string | RegExp) {
  await userEvent.click(screen.getByRole('combobox', { name: 'Jadwal distribusi' }));
  await userEvent.click(await screen.findByRole('option', { name }));
}

const slot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'open',
  machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_serial_number: '',
  documentation: [], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

function renderPage(permissions = ['distribution.view', 'distribution.pos_mesin']) {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }, regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
      package_template: { id: 'pkg-1', template_code: 'PETANI-LPG', version: 1, name: 'Paket Petani', program_type: 'farmer', status: 'published', values: { machine_options: [], hose_options: [] } },
    }] });
    if (path.startsWith('/api/v1/distribution/slots/search')) return Promise.resolve({ data: slot });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DistributionPage /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => vi.clearAllMocks());

test('searches for a slot by number and shows the 3 POS sections', async () => {
  renderPage();
  await chooseSchedule(/Wajo Tahap 1/);
  fireEvent.change(screen.getByLabelText('Nomor bagi atau NIK'), { target: { value: '7' } });
  fireEvent.click(screen.getByRole('button', { name: 'Cari' }));
  await waitFor(() => expect(screen.getByLabelText('POS Mesin')).toBeVisible());
  expect(screen.getByLabelText('POS Dokumen')).toBeVisible();
  expect(screen.getByLabelText('POS Penyerahan')).toBeVisible();
});

test('hides the create-slot button without distribution.pos_mesin', async () => {
  renderPage(['distribution.view']);
  await chooseSchedule(/Wajo Tahap 1/);
  expect(screen.queryByRole('button', { name: 'Buat Slot Mesin Baru' })).not.toBeInTheDocument();
});
```

- [ ] **Step 5: Write section tests**

`SlotMesinSection.test.tsx`, `SlotDokumenSection.test.tsx`, `SlotPenyerahanSection.test.tsx` — for each, follow the same `vi.mock('../../lib/api', ...)` + `PermissionsProvider` harness. Minimum coverage per file:
- **Mesin:** renders equipment summary fields from a fixture `DistributionSlot`.
- **Dokumen:** `status: 'open'` + `distribution.pos_dokumen` permission shows the NIK lookup form; submitting a valid NIK calls `GET /api/v1/distribution/candidates` and, on success, prefills and shows the link form; submitting that calls `POST .../link`; `status: 'linked'` renders the read-only recipient summary instead.
- **Penyerahan:** `status: 'linked'` with all required documentation `status: 'complete'` enables "Selesaikan Distribusi"; any required item not `'complete'` disables it; `status: 'completed'` renders the read-only completed view; `status: 'open'` renders the locked placeholder.

Write these against the actual component props/behavior implemented in Steps 1, and Tasks 9-10 — read each section component's final code before writing its test, rather than guessing at markup.

- [ ] **Step 6: Full frontend verification**

```bash
cd "d:/KSM/Deployment/konkit/frontend"
npx tsc --noEmit
npx vitest run src/features/distribution src/features/programs
npm run build
```

Expected: type-check clean, all distribution + programs tests pass, production build succeeds.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/distribution
git commit -m "feat(distribution): implement POS Penyerahan section, remove old single-flow UI, full test coverage"
```

(`git rm` in Step 2 already stages the deletions; this final `git add` picks up everything else Task 11 touched.)

---

## After This Plan

- The backend plan's own follow-up warning is now resolved: the frontend consumes the new POS routes end to end, so the combined result (this plan + the backend plan) is safe to deploy to a shared environment.
- Not addressed by this plan, left for a future one if it becomes a real need: editing a recipient's identity fields *after* `LinkSlot` has already run (today, identity is fixed at the moment of linking — matches the fast, one-shot nature of the physical POS Dokumen process the user described, but if petugas report a frequent need to correct data after linking but before penyerahan, that would need a new backend method).
- Not addressed: a "list all open slots for this schedule" browse view (today, POS Dokumen and POS Penyerahan both require typing the exact slot number or NIK from the physical tag/DCP3 record — matches how the physical process actually works, per the user's description, but a browse/list view could be added later if petugas ask for one).
- **Role-provisioning assumption, found by this plan's final review:** the `/dokumentasi/pendistribusian` route itself is gated on `distribution.view` (unchanged by this plan). A petugas granted ONLY a single station permission (e.g. `distribution.pos_dokumen`, without `distribution.view`) cannot load the page at all and so cannot do their job. This is not a code defect — it's an assumption future role administration must honor: every POS station role must also grant `distribution.view`. Worth a note wherever roles get provisioned/documented for this program.
