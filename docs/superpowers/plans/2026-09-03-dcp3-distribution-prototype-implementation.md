# DCP3 Distribution Prototype Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an end-to-end prototype where a Super Admin configures a Petani/Nelayan program and regency schedule, imports DCP3 Excel data, searches recipients in Pendistribusian, verifies prior-receipt warnings, and captures required documentation photos.

**Architecture:** Extend the existing Go modular monolith with three bounded domains: `programs` owns regencies, programs, schedules, and versioned templates; `dcp3` owns Excel ingestion, people, identifiers, nominations, and allocations; `distribution` owns recipient lookup, eligibility summaries, distribution drafts, and documentation media. React consumes permission-protected JSON APIs and stores no authoritative eligibility decision client-side.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgx v5, goose, Excelize v2.11.0, React 19, TypeScript, TanStack Query, Base UI, Lucide, Vitest, Playwright, local filesystem storage for development.

**Spec:** `docs/superpowers/specs/2026-09-03-dcp3-distribution-documentation-design.md`

## Global Constraints

- Keep one PostgreSQL database for all regencies; every operational record carries program/schedule context.
- Preserve the original DCP3 row number and source sequence separately from the unique operational distribution number.
- NIK is the strongest identity; Kartu Petani and KUSUKA are sector-specific exact identifiers; names and phone numbers never auto-merge people.
- Import, lookup, and final distribution each run eligibility checks; final confirmation is authoritative and transactional.
- Default repeat-receipt policy is blocked and requires central approval; this prototype displays the block but does not implement approval issuance.
- Published template versions and completed distribution snapshots are immutable.
- NIK is masked in search results and API summaries unless a detail permission explicitly permits full identity data.
- Offline mode, replacement approval, BAST/DP3 rendering, and central override issuance are outside this prototype plan and receive separate plans after workflow evaluation.
- Uploaded files are limited to JPEG, PNG, or WebP, 10 MiB each; Excel files are limited to 10 MiB, 5,000 rows, and 100 columns.
- Use backend permission checks for every endpoint; frontend permission checks only control presentation.
- All mutations use the existing session-cookie authentication, CSRF validation, and audit recorder.

## File Structure

### Backend

- `internal/database/migrations/00004_program_dcp3_distribution.sql`: domain tables, indexes, permissions, and seed template.
- `internal/programs/models.go`: program setup records and request types.
- `internal/programs/service.go`: validation, template version rules, and setup orchestration.
- `internal/programs/repository.go`: PostgreSQL persistence for setup records.
- `internal/dcp3/models.go`: preview, mapping, import, person, identifier, nomination, and allocation types.
- `internal/dcp3/excel.go`: bounded Excel parsing only.
- `internal/dcp3/service.go`: preview/commit validation and identity matching orchestration.
- `internal/dcp3/repository.go`: transactional import persistence and identity matching.
- `internal/distribution/models.go`: lookup, eligibility, detail, draft, slot, and media DTOs.
- `internal/distribution/service.go`: masked search, eligibility rules, draft updates, and completion checks.
- `internal/distribution/repository.go`: lookup and transactional distribution persistence.
- `internal/media/storage.go`: filesystem storage interface and safe local implementation.
- `internal/api/program_routes.go`: program-setup HTTP handlers.
- `internal/api/dcp3_routes.go`: multipart preview and import handlers.
- `internal/api/distribution_routes.go`: search, detail, draft, media, and completion handlers.
- `cmd/server/main.go`: dependency wiring.
- `internal/config/config.go`: `STORAGE_PATH` configuration.

### Frontend

- `frontend/src/features/programs/ProgramSetupPage.tsx`: tabbed operational setup workspace.
- `frontend/src/features/programs/RegenciesPanel.tsx`: regency CRUD.
- `frontend/src/features/programs/ProgramsPanel.tsx`: program CRUD.
- `frontend/src/features/programs/SchedulesPanel.tsx`: schedule CRUD and template assignment.
- `frontend/src/features/programs/TemplatesPanel.tsx`: package fields and documentation slots.
- `frontend/src/features/dcp3/DCP3ImportPage.tsx`: upload, mapping, preview, validation, and commit flow.
- `frontend/src/features/distribution/DistributionPage.tsx`: schedule context and realtime recipient search.
- `frontend/src/features/distribution/RecipientWorkspace.tsx`: verification, history, missing fields, and slot completion.
- `frontend/src/features/distribution/DocumentationSlot.tsx`: camera/gallery capture and upload state.
- `frontend/src/app/AppShell.tsx`: new navigation groups.
- `frontend/src/app/routes.tsx`: protected routes.

---

### Task 1: Operational Schema and Permissions

**Files:**
- Create: `internal/database/migrations/00004_program_dcp3_distribution.sql`
- Modify: `internal/auth/repository_integration_test.go`
- Test: `internal/database/postgres_test.go`

**Interfaces:**
- Produces tables consumed by every later task.
- Produces permissions `programs.view`, `programs.manage`, `dcp3.view`, `dcp3.import`, `distribution.view`, `distribution.manage`, and `documentation.manage`.

- [ ] **Step 1: Write the failing migration integration test**

Add assertions that migration version 4 exists and these constraints work:

```go
var permissionCount int
err := pool.QueryRow(ctx, `
  SELECT count(*) FROM permissions
  WHERE code = ANY($1)
`, []string{"programs.view", "programs.manage", "dcp3.view", "dcp3.import", "distribution.view", "distribution.manage", "documentation.manage"}).Scan(&permissionCount)
if err != nil || permissionCount != 7 {
    t.Fatalf("operational permissions: count=%d err=%v", permissionCount, err)
}
```

- [ ] **Step 2: Run the migration test to verify it fails**

Run: `go test ./internal/database ./internal/auth -count=1`

Expected: FAIL because migration 4 and the operational permissions do not exist.

- [ ] **Step 3: Add the complete migration**

Create the following tables with UUID primary keys, `created_at`, and `updated_at` where applicable:

```sql
regencies(id, province_name, name, document_code, is_active, notes)
programs(id, code, name, program_type, fiscal_year, status, notes)
package_template_versions(id, template_code, version, name, program_type, values_json, status, published_at)
documentation_template_versions(id, template_code, version, name, program_type, status, published_at)
documentation_template_slots(id, template_version_id, slot_code, label, stage, is_required, min_files, max_files, input_source, require_location, require_captured_at, instructions, sort_order)
program_schedules(id, program_id, regency_id, package_template_version_id, documentation_template_version_id, name, start_date, end_date, status, distribution_number_padding, receipt_policy_json, notes)
dcp3_import_batches(id, schedule_id, original_filename, file_checksum, source_name, received_at, sheet_name, mapping_json, status, total_rows, valid_rows, warning_rows, invalid_rows, imported_by, imported_at)
dcp3_import_rows(id, batch_id, source_row_number, source_sequence_number, raw_data_json, normalized_data_json, validation_status, validation_messages_json)
people(id, full_name, nik, date_of_birth, address, village, district, phone_number, verification_status)
person_sector_identifiers(id, person_id, identifier_type, normalized_value, display_value, verified_at)
candidate_nominations(id, batch_id, import_row_id, person_id, program_type, source_snapshot_json, status)
package_allocations(id, schedule_id, nomination_id, intended_person_id, actual_recipient_person_id, distribution_number, status, package_snapshot_json)
distribution_records(id, allocation_id, recipient_person_id, status, verification_snapshot_json, distributed_at, distributed_by, completed_at)
eligibility_checks(id, person_id, schedule_id, result, reasons_json, checked_at, checked_by)
documentation_slots(id, distribution_id, slot_code, label_snapshot, is_required, min_files, max_files, status, sort_order)
media_files(id, documentation_slot_id, storage_key, original_filename, mime_type, byte_size, checksum, source, captured_at, latitude, longitude, status, uploaded_by, uploaded_at)
```

Add exact uniqueness indexes:

```sql
CREATE UNIQUE INDEX regencies_name_province_uq
    ON regencies (lower(name), lower(province_name));
CREATE UNIQUE INDEX regencies_document_code_uq ON regencies (document_code);
CREATE UNIQUE INDEX programs_code_uq ON programs (code);
CREATE UNIQUE INDEX package_templates_code_version_uq
    ON package_template_versions (template_code, version);
CREATE UNIQUE INDEX documentation_templates_code_version_uq
    ON documentation_template_versions (template_code, version);
CREATE UNIQUE INDEX documentation_template_slots_code_uq
    ON documentation_template_slots (template_version_id, slot_code);
CREATE UNIQUE INDEX allocations_distribution_number_uq
    ON package_allocations (schedule_id, distribution_number);
CREATE UNIQUE INDEX people_nik_uq ON people (nik) WHERE nik IS NOT NULL;
CREATE UNIQUE INDEX people_sector_identifier_uq
    ON person_sector_identifiers (identifier_type, normalized_value);
CREATE UNIQUE INDEX distribution_allocation_uq ON distribution_records (allocation_id);
CREATE UNIQUE INDEX distribution_slots_code_uq
    ON documentation_slots (distribution_id, slot_code);
```

Seed one Petani package template, one Nelayan package template, and documentation slots `recipient_package`, `machine_serial`, `package_completeness`, and `signed_bast`. Grant all seven permissions to `super_admin`.

- [ ] **Step 4: Apply and verify migration 4**

Run:

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate status
go test ./internal/database ./internal/auth -count=1
```

Expected: migration `00004_program_dcp3_distribution.sql` is applied and tests PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/database/migrations/00004_program_dcp3_distribution.sql internal/database internal/auth
git commit -m "feat: add program and DCP3 schema"
```

### Task 2: Program Setup Domain and API

**Files:**
- Create: `internal/programs/models.go`
- Create: `internal/programs/service.go`
- Create: `internal/programs/service_test.go`
- Create: `internal/programs/repository.go`
- Create: `internal/programs/repository_integration_test.go`
- Create: `internal/api/program_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Produces `programs.Service` with `ListRegencies`, `SaveRegency`, `ListPrograms`, `SaveProgram`, `ListSchedules`, `SaveSchedule`, `ListPackageTemplates`, `SavePackageTemplate`, `ListDocumentationTemplates`, and `SaveDocumentationTemplate`.
- Produces REST resources under `/api/v1/program-setup/*`.

- [ ] **Step 1: Write service tests for validation and immutable published versions**

Cover these exact cases:

```go
func TestSaveRegencyNormalizesDocumentCode(t *testing.T) {
    repo := &programRepositoryStub{}
    service := NewService(repo, fixedClock)

    saved, err := service.SaveRegency(context.Background(), RegencyInput{
        ProvinceName: "Sulawesi Selatan",
        Name: "Wajo",
        DocumentCode: " wjo ",
        IsActive: true,
    })
    if err != nil { t.Fatal(err) }
    if saved.DocumentCode != "WJO" { t.Fatalf("code=%q", saved.DocumentCode) }

    for _, code := range []string{"WJ", "WAJO"} {
        _, err := service.SaveRegency(context.Background(), RegencyInput{
            ProvinceName: "Sulawesi Selatan", Name: "Wajo", DocumentCode: code,
        })
        if !errors.Is(err, ErrDocumentCodeInvalid) { t.Fatalf("code=%q err=%v", code, err) }
    }
}

func TestSaveProgramRequiresFarmerOrFisherman(t *testing.T) {
    service := NewService(&programRepositoryStub{}, fixedClock)
    _, err := service.SaveProgram(context.Background(), ProgramInput{
        Code: "TEST-2026", Name: "Test", ProgramType: "vehicle", FiscalYear: 2026,
    })
    if !errors.Is(err, ErrProgramTypeInvalid) { t.Fatalf("err=%v", err) }
}

func TestSaveScheduleRejectsEndBeforeStart(t *testing.T) {
    service := NewService(&programRepositoryStub{}, fixedClock)
    _, err := service.SaveSchedule(context.Background(), ScheduleInput{
        ProgramID: testProgramID, RegencyID: testRegencyID,
        PackageTemplateVersionID: testPackageTemplateID,
        DocumentationTemplateVersionID: testDocumentationTemplateID,
        Name: "Wajo Tahap 1", StartDate: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
        EndDate: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
    })
    if !errors.Is(err, ErrScheduleDatesInvalid) { t.Fatalf("err=%v", err) }
}

func TestPublishedTemplateCreatesNextVersion(t *testing.T) {
    repo := newProgramRepositoryStubWithPublishedTemplate(1)
    service := NewService(repo, fixedClock)
    next, err := service.SavePackageTemplate(context.Background(), PackageTemplateInput{
        TemplateCode: "PETANI-LPG", Name: "Petani LPG Revisi", ProgramType: ProgramFarmer,
        Values: map[string]any{"converter_brand": "ERGAS"},
    })
    if err != nil { t.Fatal(err) }
    if next.Version != 2 || next.Status != "draft" { t.Fatalf("next=%+v", next) }
    if repo.templates[0].Version != 1 || repo.templates[0].Status != "published" {
        t.Fatalf("published version mutated: %+v", repo.templates[0])
    }
}
```

- [ ] **Step 2: Run the service tests to verify they fail**

Run: `go test ./internal/programs -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement models and service**

Define stable enums and inputs:

```go
type ProgramType string
const (
    ProgramFarmer ProgramType = "farmer"
    ProgramFisherman ProgramType = "fisherman"
)

type RegencyInput struct { ProvinceName, Name, DocumentCode, Notes string; IsActive bool }
type ProgramInput struct { Code, Name string; ProgramType ProgramType; FiscalYear int; Status, Notes string }
type ScheduleInput struct {
    ProgramID, RegencyID, PackageTemplateVersionID, DocumentationTemplateVersionID string
    Name string
    StartDate, EndDate time.Time
    Status string
    DistributionNumberPadding int
    Notes string
}
```

Validate three-letter uppercase regency codes, fiscal years from 2000 through current year plus five, padding from 1 through 8, required references, and allowed statuses.

- [ ] **Step 4: Implement repository and integration tests**

Test creating Wajo, a Petani program, and a schedule linked to the seeded templates. Verify duplicate document code returns `ErrDocumentCodeInUse` and published template version 1 remains unchanged after version 2 is created.

- [ ] **Step 5: Add protected API routes**

Implement:

```text
GET,POST       /api/v1/program-setup/regencies
PATCH          /api/v1/program-setup/regencies/{id}
GET,POST       /api/v1/program-setup/programs
PATCH          /api/v1/program-setup/programs/{id}
GET,POST       /api/v1/program-setup/schedules
PATCH          /api/v1/program-setup/schedules/{id}
GET,POST       /api/v1/program-setup/package-templates
PATCH          /api/v1/program-setup/package-templates/{id}
GET,POST       /api/v1/program-setup/documentation-templates
PATCH          /api/v1/program-setup/documentation-templates/{id}
```

Use `programs.view` for GET and `programs.manage` for POST/PATCH. Template mutations may update an existing draft, but mutating a published version must create the next draft version under the same stable `template_code`; published rows are never updated. Validate documentation slot codes, ordering, required counts, accepted input source, and unique slot codes before persistence. Return `400` with field errors, `404` for missing references, and `409` for stable-code or version conflicts.

- [ ] **Step 6: Wire dependencies and run tests**

Run: `go test ./internal/programs ./internal/api ./cmd/server -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/programs internal/api cmd/server
git commit -m "feat: add program setup API"
```

### Task 3: Program Setup Workspace

**Files:**
- Create: `frontend/src/features/programs/ProgramSetupPage.tsx`
- Create: `frontend/src/features/programs/ProgramSetupPage.test.tsx`
- Create: `frontend/src/features/programs/RegenciesPanel.tsx`
- Create: `frontend/src/features/programs/ProgramsPanel.tsx`
- Create: `frontend/src/features/programs/SchedulesPanel.tsx`
- Create: `frontend/src/features/programs/TemplatesPanel.tsx`
- Create: `frontend/src/features/programs/ProgramSetup.module.css`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/routes.tsx`

**Interfaces:**
- Consumes `/api/v1/program-setup/*` from Task 2.
- Produces route `/persiapan-program` and query keys prefixed with `program-setup`.

- [ ] **Step 1: Write failing UI tests**

Verify that:

```tsx
expect(screen.getByRole('tab', { name: 'Kabupaten' })).toBeVisible();
expect(screen.getByRole('tab', { name: 'Program' })).toBeVisible();
expect(screen.getByRole('tab', { name: 'Jadwal' })).toBeVisible();
expect(screen.getByRole('tab', { name: 'Template' })).toBeVisible();
```

Also test that a user with only `programs.view` sees data but no create/edit/save controls.

- [ ] **Step 2: Run the UI test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/programs/ProgramSetupPage.test.tsx`

Expected: FAIL because the page does not exist.

- [ ] **Step 3: Build the setup page**

Use Base UI tabs. Keep tables unframed, use dialogs for create/edit, use date inputs for schedules, and use segmented controls for Petani/Nelayan. The Jadwal panel must show program, kabupaten, period, template package, photo template, and status in one scannable table.

- [ ] **Step 4: Add permission-aware navigation and route**

Add `Persiapan Program` under a new `Operasional` navigation group with a `CalendarRange` Lucide icon. Protect the route with `programs.view`.

- [ ] **Step 5: Run frontend verification**

Run:

```powershell
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: all frontend tests and build PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src web/static/app
git commit -m "feat: add program setup workspace"
```

### Task 4: Bounded Excel Preview Parser

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/dcp3/models.go`
- Create: `internal/dcp3/excel.go`
- Create: `internal/dcp3/excel_test.go`

**Interfaces:**
- Produces `ParseWorkbook(io.Reader, ParseLimits) (WorkbookPreview, error)`.
- Produces normalized headers and raw rows without writing to PostgreSQL.

- [ ] **Step 1: Add Excelize**

Run: `go get github.com/xuri/excelize/v2@v2.11.0`

Expected: `go.mod` and `go.sum` include Excelize v2.11.0.

- [ ] **Step 2: Write parser tests**

Define and test:

```go
type ParseLimits struct { MaxBytes int64; MaxRows, MaxColumns int }
type WorkbookPreview struct {
    SheetName string `json:"sheet_name"`
    Headers []string `json:"headers"`
    Rows []PreviewRow `json:"rows"`
}

preview, err := ParseWorkbook(file, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
```

Use an Excelize-backed test helper that writes workbooks into `t.TempDir()`; its normal fixture contains `No`, `Nama`, `NIK`, `No Kartu Petani`, `Alamat`, and `No HP`. Test malformed XLSX, no sheets, duplicate headers, row overflow, column overflow, and blank rows without committing generated binary fixtures.

- [ ] **Step 3: Run parser tests to verify they fail**

Run: `go test ./internal/dcp3 -run TestParseWorkbook -count=1`

Expected: FAIL because `ParseWorkbook` does not exist.

- [ ] **Step 4: Implement the parser**

Use `excelize.OpenReader`, close the workbook with `defer`, read only the selected first visible worksheet, stop at configured bounds, trim headers, preserve every source cell as text, and never interpret formulas. Return typed errors `ErrWorkbookTooLarge`, `ErrTooManyRows`, `ErrTooManyColumns`, `ErrHeadersInvalid`, and `ErrWorkbookInvalid`.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/dcp3 -count=1`

Expected: PASS.

```powershell
git add go.mod go.sum internal/dcp3
git commit -m "feat: parse bounded DCP3 workbooks"
```

### Task 5: DCP3 Preview, Mapping, and Transactional Import

**Files:**
- Create: `internal/dcp3/service.go`
- Create: `internal/dcp3/service_test.go`
- Create: `internal/dcp3/repository.go`
- Create: `internal/dcp3/repository_integration_test.go`
- Create: `internal/api/dcp3_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Produces `Preview(ctx, actor, scheduleID, filename, reader) (ImportPreview, error)`.
- Produces `Commit(ctx, actor, batchID, Mapping, auth.ClientMeta) (ImportResult, error)`.
- Produces `/api/v1/dcp3/previews`, `/api/v1/dcp3/previews/{id}`, and `/api/v1/dcp3/imports`.

- [ ] **Step 1: Write mapping and identity tests**

Use this mapping contract:

```go
type Mapping struct {
    SourceSequence string `json:"source_sequence"`
    FullName string `json:"full_name"`
    NIK string `json:"nik"`
    FarmerCardNumber string `json:"farmer_card_number"`
    KUSUKANumber string `json:"kusuka_number"`
    Address string `json:"address"`
    Village string `json:"village"`
    District string `json:"district"`
    PhoneNumber string `json:"phone_number"`
}
```

Test Petani requires a name and accepts Kartu Petani; Nelayan accepts KUSUKA; NIK is normalized to digits; duplicate NIK in one batch is `needs_review`; conflicting card ownership is `identity_conflict`; missing optional fields remains importable with warnings.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/dcp3 -run 'Test(ValidateMapping|NormalizeRow|Commit)' -count=1`

Expected: FAIL because service and repository are absent.

- [ ] **Step 3: Implement preview persistence**

`POST /api/v1/dcp3/previews` accepts multipart fields `schedule_id` and `file`. Stream through a 10 MiB limit, calculate SHA-256, parse with Task 4, and store a draft batch plus raw rows. A repeated checksum for the same schedule returns `409 duplicate_import`.

- [ ] **Step 4: Implement transactional commit**

Within one transaction:

1. Lock the draft batch.
2. Validate mapping against actual headers.
3. Normalize every row.
4. Match exact NIK, then exact sector identifier.
5. Create a person only when no exact identity exists and no conflict is present.
6. Save nomination with the immutable source snapshot.
7. Allocate a unique distribution number; prefer a non-conflicting positive source sequence, otherwise use the next schedule sequence, and copy the selected package template into `package_snapshot_json`.
8. Create the allocation's draft `distribution_record` for the intended person and instantiate `documentation_slots` from the schedule's published documentation template. Copy slot labels and requirements so later template versions cannot alter this distribution.
9. Save row status and aggregate batch counts.
10. Mark the batch `imported`.
11. Record `dcp3.imported` audit metadata without full NIK values.

- [ ] **Step 5: Add API permission and error tests**

Use `dcp3.view` for GET and `dcp3.import` for preview/commit. Verify `413` for oversized bodies, `415` for non-XLSX uploads, `400` for invalid mapping, `409` for duplicate checksum, and that audit/error responses never contain full NIK.

- [ ] **Step 6: Run integration tests and commit**

Run: `go test ./internal/dcp3 ./internal/api ./cmd/server -count=1`

Expected: PASS.

```powershell
git add internal/dcp3 internal/api cmd/server
git commit -m "feat: import mapped DCP3 batches"
```

### Task 6: DCP3 Import Wizard UI

**Files:**
- Create: `frontend/src/features/dcp3/DCP3ImportPage.tsx`
- Create: `frontend/src/features/dcp3/DCP3ImportPage.test.tsx`
- Create: `frontend/src/features/dcp3/DCP3Import.module.css`
- Create: `frontend/src/features/dcp3/ColumnMappingStep.tsx`
- Create: `frontend/src/features/dcp3/ImportPreviewTable.tsx`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/routes.tsx`
- Modify: `frontend/src/lib/api.ts`

**Interfaces:**
- Consumes Task 5 DCP3 endpoints.
- Produces route `/dcp3` with a four-step upload workflow.

- [ ] **Step 1: Add a multipart API helper test**

Verify `apiRequest` does not force `Content-Type: application/json` when the body is `FormData`; the browser must generate the multipart boundary.

- [ ] **Step 2: Write the wizard tests**

Test these steps and labels:

```text
1 Pilih jadwal
2 Upload DCP3
3 Cocokkan kolom
4 Periksa dan import
```

Verify preview rows display `Valid`, `Peringatan`, or `Konflik`; the import button is disabled when required mappings are empty; import success shows valid/warning/conflict totals.

- [ ] **Step 3: Run tests to verify they fail**

Run: `npm.cmd --prefix frontend test -- --run src/features/dcp3/DCP3ImportPage.test.tsx`

Expected: FAIL because the wizard does not exist.

- [ ] **Step 4: Implement the wizard**

Use one primary workspace with a compact step indicator, schedule selector, drag/drop plus file input, mapping selects, horizontally scrollable preview table, and persistent validation summary. Do not place panels inside cards. Preserve selected mapping while moving backward between steps.

- [ ] **Step 5: Add navigation and verify**

Add `DCP3` under `Operasional` with `FileSpreadsheet`; protect with `dcp3.view`, and hide upload controls without `dcp3.import`.

Run:

```powershell
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src web/static/app
git commit -m "feat: add DCP3 import wizard"
```

### Task 7: Distribution Search and Eligibility API

**Files:**
- Create: `internal/distribution/models.go`
- Create: `internal/distribution/service.go`
- Create: `internal/distribution/service_test.go`
- Create: `internal/distribution/repository.go`
- Create: `internal/distribution/repository_integration_test.go`
- Create: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Produces `Search(ctx, scheduleID, query, limit) ([]SearchResult, error)`.
- Produces `GetWorkspace(ctx, allocationID) (RecipientWorkspace, error)`.
- Produces `SaveDraft(ctx, actor, allocationID, DraftInput, meta) (RecipientWorkspace, error)`.
- Produces `/api/v1/distribution/search`, `/api/v1/distribution/allocations/{id}`, and `/api/v1/distribution/allocations/{id}/draft`.

- [ ] **Step 1: Write search and eligibility tests**

Define:

```go
type SearchResult struct {
    AllocationID string `json:"allocation_id"`
    DistributionNumber int `json:"distribution_number"`
    FullName string `json:"full_name"`
    MaskedNIK string `json:"masked_nik"`
    Location string `json:"location"`
    ProgramType string `json:"program_type"`
    Eligibility string `json:"eligibility"`
    AllocationStatus string `json:"allocation_status"`
    Documentation []SlotSummary `json:"documentation"`
}

type SlotSummary struct {
    Code string `json:"code"`
    Label string `json:"label"`
    Status string `json:"status"`
}
```

Test exact distribution number first, exact NIK/card second, and name prefix/fuzzy text last. Require a schedule, two characters for names, maximum 20 results, masked NIK, and no phone/address in search DTOs. Return each configured slot as `missing` or `complete` so the UI can render a compact red/green documentation indicator without opening the recipient.

Test eligibility returns `previously_received` when a completed distribution exists for the person in any schedule. Include previous date, regency, program, and BAST number when available in the expandable history DTO.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/distribution -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement search and workspace queries**

Use parameterized PostgreSQL queries. Search only inside the selected schedule for candidate allocations, but compute receipt history across all schedules. `GetWorkspace` returns full identity only under `distribution.view` and includes source snapshot, editable missing fields, eligibility reasons, receipt history, and required documentation slots.

- [ ] **Step 4: Implement draft updates**

Allow completion of address, village, district, phone, and sector identifier. NIK changes require `identity_change_reason`. Save before/after values to `distribution.draft_updated` audit metadata with NIK masked.

- [ ] **Step 5: Add API routes and permission tests**

Implement:

```text
GET   /api/v1/distribution/search?schedule_id=&q=&limit=
GET   /api/v1/distribution/allocations/{id}
PATCH /api/v1/distribution/allocations/{id}/draft
```

Use `distribution.view` for GET and `distribution.manage` for PATCH. Verify cross-schedule allocation IDs return only their own record, search results never leak full NIK, and missing schedule/query returns `400`.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/distribution ./internal/api ./cmd/server -count=1`

Expected: PASS.

```powershell
git add internal/distribution internal/api cmd/server
git commit -m "feat: add distribution recipient lookup"
```

### Task 8: Pendistribusian Search and Verification UI

**Files:**
- Create: `frontend/src/features/distribution/DistributionPage.tsx`
- Create: `frontend/src/features/distribution/DistributionPage.test.tsx`
- Create: `frontend/src/features/distribution/RecipientSearch.tsx`
- Create: `frontend/src/features/distribution/RecipientWorkspace.tsx`
- Create: `frontend/src/features/distribution/Distribution.module.css`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/routes.tsx`

**Interfaces:**
- Consumes Task 7 endpoints.
- Produces route `/dokumentasi/pendistribusian`.

- [ ] **Step 1: Write UI behavior tests**

Use fake timers to verify a 300 ms debounce. Assert results show distribution number, name, masked NIK, location, eligibility, and compact red/green indicators for every configured documentation slot. Selecting a result must load the workspace. A `previously_received` result must render an expandable history and disable final completion while still allowing draft edits.

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm.cmd --prefix frontend test -- --run src/features/distribution/DistributionPage.test.tsx`

Expected: FAIL because the feature does not exist.

- [ ] **Step 3: Implement schedule context and search**

Require the user to choose an active schedule. Search supports distribution number, name, NIK, Kartu Petani, and KUSUKA in one field. Use a combobox/listbox interaction with keyboard navigation and a stable result height; do not expose full NIK in the dropdown.

- [ ] **Step 4: Implement the recipient workspace**

Display DCP3 source values, missing editable fields, changed-value markers, eligibility summary, and receipt history. Use green/yellow/red/blue/gray badges defined in the spec. Save edits as a draft; do not expose replacement or override buttons in this prototype because their authoritative workflows are not implemented yet.

- [ ] **Step 5: Add navigation and responsive tests**

Add `Dokumentasi` group and `Pendistribusian` item with `Camera`; protect with `distribution.view`. At 375 px, the search, identity summary, and edit form stack vertically with no horizontal page overflow.

- [ ] **Step 6: Run tests and commit**

Run:

```powershell
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: PASS.

```powershell
git add frontend/src web/static/app
git commit -m "feat: add distribution verification workspace"
```

### Task 9: Local Media Storage and Documentation Slots

**Files:**
- Create: `internal/media/storage.go`
- Create: `internal/media/storage_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`
- Modify: `.gitignore`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`
- Create: `frontend/src/features/distribution/DocumentationSlot.tsx`
- Create: `frontend/src/features/distribution/DocumentationSlot.test.tsx`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.tsx`

**Interfaces:**
- Produces `media.Storage` with `Put`, `Open`, and `Delete` using opaque storage keys.
- Produces upload/delete/content endpoints for distribution documentation slots.

- [ ] **Step 1: Write filesystem storage tests**

Define:

```go
type Storage interface {
    Put(ctx context.Context, key string, src io.Reader) (int64, string, error)
    Open(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}
```

Test traversal keys such as `../secret` are rejected, writes are atomic, SHA-256 is returned, failed writes leave no partial file, and deleting a missing key is idempotent.

- [ ] **Step 2: Add configuration**

Load `STORAGE_PATH` with local default `./storage`; require an absolute container path outside local environment. Add `/storage/` to `.gitignore` and `STORAGE_PATH=./storage` to `.env.example`.

- [ ] **Step 3: Implement secure media endpoints**

```text
POST   /api/v1/distribution/slots/{id}/media
DELETE /api/v1/distribution/media/{id}
GET    /api/v1/distribution/media/{id}/content
```

Upload accepts multipart `file`, `source`, `captured_at`, `latitude`, and `longitude`. Detect MIME from file bytes, allow only JPEG/PNG/WebP, enforce 10 MiB, generate an opaque UUID storage key, and save metadata only after storage succeeds. Content access requires an authenticated `distribution.view` check and uses `Content-Disposition: inline` plus `X-Content-Type-Options: nosniff`.

- [ ] **Step 4: Implement slot status calculation**

Set a slot to `complete` when accepted media count meets `min_files`; otherwise set `missing`. Roll back database metadata and delete the stored file if either side fails. Audit upload/delete without filesystem paths.

- [ ] **Step 5: Build camera/gallery controls**

Render two explicit controls:

```tsx
<input type="file" accept="image/jpeg,image/png,image/webp" capture="environment" />
<input type="file" accept="image/jpeg,image/png,image/webp" />
```

Wrap them with icon buttons labeled `Buka kamera` and `Pilih galeri`. Show image preview, upload progress, retry, delete, required count, and green/red slot status. Revoke object URLs during cleanup.

- [ ] **Step 6: Run backend and frontend tests**

Run:

```powershell
go test ./internal/media ./internal/distribution ./internal/api ./internal/config ./cmd/server -count=1
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/media internal/distribution internal/api internal/config cmd/server frontend/src .env.example .gitignore web/static/app
git commit -m "feat: add distribution photo documentation"
```

### Task 10: Transactional Distribution Completion

**Files:**
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository_integration_test.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.tsx`
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx`

**Interfaces:**
- Produces `Complete(ctx, actor, allocationID, meta) (DistributionRecord, error)`.
- Produces `POST /api/v1/distribution/allocations/{id}/complete`.

- [ ] **Step 1: Write race and rule tests**

Test completion rejects missing required identity, incomplete photo slots, `previously_received`, an already completed allocation, and simultaneous completion attempts. Exactly one concurrent attempt may commit.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/distribution -run TestComplete -count=1`

Expected: FAIL because `Complete` does not exist.

- [ ] **Step 3: Implement one transaction with row locks**

Lock the allocation, recipient person, and distribution record using `FOR UPDATE`. Recompute prior completed receipts and slot counts inside the transaction. On success set allocation `distributed`, distribution `completed`, copy identity/package/documentation summaries into `verification_snapshot_json`, set timestamps, and write `distribution.completed` audit data.

Return typed errors `ErrIdentityIncomplete`, `ErrDocumentationIncomplete`, `ErrPreviouslyReceived`, and `ErrAlreadyCompleted`; map each to `409` with a stable code.

- [ ] **Step 4: Add the final UI confirmation**

Show `Konfirmasi distribusi` only to `distribution.manage`. Require a confirmation dialog summarizing recipient, schedule, package, and required slot status. Keep the button disabled for incomplete or blocked workspaces and refresh search/detail after success.

- [ ] **Step 5: Run tests and commit**

Run:

```powershell
go test ./internal/distribution ./internal/api -count=1
npm.cmd --prefix frontend test -- --run
```

Expected: PASS.

```powershell
git add internal/distribution internal/api frontend/src
git commit -m "feat: complete package distributions safely"
```

### Task 11: End-to-End Prototype Verification

**Files:**
- Modify: `cmd/e2eseed/main.go`
- Modify: `cmd/e2eseed/main_test.go`
- Modify: `frontend/playwright.config.ts`
- Create: `frontend/e2e/dcp3-distribution.spec.ts`
- Modify: `frontend/e2e/global-teardown.ts`
- Modify: `README.md`

**Interfaces:**
- Consumes the complete Tasks 1-10 vertical slice.
- Produces repeatable desktop/mobile workflow coverage and documented local commands.

- [ ] **Step 1: Extend the E2E seed command**

Seed an E2E Super Admin, Wajo regency, Petani program, published templates, two active schedules suffixed `desktop` and `mobile`, and one person with a prior completed receipt. Extend `cmd/e2eseed` with `-fixture-dir`; use Excelize to generate one DCP3 XLSX per project under ignored `.cache/e2e`, each containing a unique clean candidate and the prior recipient. Update Playwright's web-server command to pass that fixture directory. Add cleanup that deletes only records carrying deterministic `e2e-` codes.

- [ ] **Step 2: Write the Playwright journey**

The test must:

1. Log in.
2. Open Persiapan Program and verify seeded setup.
3. Select the schedule and upload the DCP3 fixture matching `testInfo.project.name`, then map its columns.
4. Commit import and verify totals.
5. Open Pendistribusian and select the schedule.
6. Search by distribution number, name, and NIK.
7. Verify full NIK is absent from dropdown markup.
8. Open the blocked candidate and expand prior receipt history.
9. Open the clean candidate, complete missing data, upload the existing `web/static/images/konkit-aceh-recipient.jpg` into each required slot, and save draft.
10. Confirm distribution and verify status `Selesai`.
11. Run at desktop 1366x768 and mobile 375x812 with no document-level horizontal overflow.

- [ ] **Step 3: Document local usage**

Add commands for migration, frontend build, E2E seed, server start, DCP3 fixture location, and `STORAGE_PATH`. Explain that camera capture depends on browser/device support and gallery upload works on desktop.

- [ ] **Step 4: Run the full verification suite**

Run:

```powershell
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
$env:GOCACHE=(Join-Path (Get-Location) '.cache\go-build')
$env:npm_config_cache=(Join-Path (Get-Location) '.cache\npm')
go test -p 1 ./... -count=1
go vet ./...
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
npm.cmd --prefix frontend run e2e
go run ./cmd/migrate status
git diff --check
```

Expected: all commands exit 0; migration status lists versions 1 through 4; Playwright passes desktop and mobile.

- [ ] **Step 5: Inspect browser screenshots**

Inspect Persiapan Program, DCP3 mapping/preview, search dropdown, blocked history, recipient workspace, documentation slots, and completion state at desktop and mobile sizes. Fix clipped controls, unreadable labels, accidental nested cards, or horizontal page overflow before committing.

- [ ] **Step 6: Commit**

```powershell
git add cmd/e2eseed frontend/e2e frontend/playwright.config.ts README.md web/static/app
git commit -m "test: verify DCP3 distribution prototype"
```

## Follow-Up Plans

After the prototype is tested with real workflow feedback, create separate implementation plans for:

1. Recipient replacement proposal and central approval.
2. Repeat-receipt override issuance and expiry policy.
3. BAST template editor, snapshot, PDF generation, and numbering.
4. DP3 dataset mapping and document/export generation after a sample DP3 is available.
5. PWA offline assignment packs, IndexedDB drafts, upload queue, and conflict resolution.
6. Map visualization and regency progress reporting.
