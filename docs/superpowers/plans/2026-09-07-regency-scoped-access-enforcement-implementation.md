# Regency-Scoped Access — DCP3/Pendistribusian/Laporan Enforcement (PR2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce `auth.RegencyScope` (built in PR1) against every `schedule_id`/`allocation_id`/`slot_id`/`media_id` input in `internal/dcp3`, `internal/distribution`, and `internal/reports`, so a caller can never read or mutate a record whose `program_schedules.regency_id` falls outside their role's cakupan — closing the gap PR1 left open (PR1 only scoped the `programs` package's `ListRegencies`/`ListSchedules`).

**Architecture:** `auth.RegencyScope` is already resolved once per request in `internal/api/handler.go` via `h.regencyScope(w, r, principal)`. This plan threads that same value as an explicit parameter (never via `context.Context`, matching the existing `auth.Principal`/`auth.ClientMeta` convention) into each domain service method and its repository SQL. Two enforcement shapes are used, chosen per the record's cardinality:
- **List/single-record lookups with a direct `regency_id` join already in the query** (`dcp3.CreatePreview/GetPreview`, `distribution.Search/GetWorkspace/Complete`, and the newly-joined `distribution` media methods): fold `($n OR regency_id::text = ANY($n+1))` into the existing `WHERE`, reusing the query's current not-found path (`pgx.ErrNoRows` → the package's existing sentinel). No new sentinel errors needed for `dcp3`/`distribution`.
- **`reports`, which has no existing not-found concept** (a schedule with zero allocations legitimately returns an empty `Summary`/`[]Row`, so folding scope into the reporting query would make "no data yet" and "out of scope" indistinguishable — acceptable for `distribution.Search` because that already returns an empty list either way, but `reports` needs an explicit pre-check since the spec treats a missing/out-of-scope schedule as an error, not silent-empty for `Summary`/`Rows`): add a new `ErrScheduleNotFound` sentinel and a lightweight `ScheduleRegency(ctx, scheduleID) (string, error)` repository method, called once before `Summary`/`Rows` run.

Two calls rely on an **already-scoped upstream call instead of re-checking**, to avoid duplicating the same WHERE clause twice in one request: `dcp3.Commit` trusts the scope check already done by the `GetPreview` call `Service.Commit` makes internally before it; `distribution.SaveDraft` trusts the scope check already done by the `GetWorkspace` call `Service.SaveDraft` makes internally before it. Both of these internal calls already existed before this plan — they're just being told to enforce scope now.

No frontend changes: per the approved spec (§7), the "Jadwal" dropdowns on DCP3/Pendistribusian/Laporan already only show schedules returned by `programs.ListSchedules`, which PR1 already scopes. Backend enforcement here closes the direct-API-request gap; no additional frontend filtering logic is needed.

**Tech Stack:** Same as PR1 — Go 1.26, PostgreSQL 18 via pgx v5, goose migrations (no new migration in this plan — no schema change).

**Spec:** `docs/superpowers/specs/2026-09-07-regency-scoped-access-design.md` (§5.2 DCP3, §5.3 Pendistribusian, §5.4 Laporan, §8 Keamanan dan Audit — "not found" instead of "forbidden"). This plan is PR2 of the split the spec's own document approved; PR1 (`docs/superpowers/plans/2026-09-07-regency-scoped-access-foundation-implementation.md`) built the `auth.RegencyScope` mechanism this plan consumes and is a prerequisite — this branch must fork from `worktree-regency-scoped-access`, not from `main`, until PR1 merges.

## Global Constraints

- `auth.RegencyScope` is always passed as an explicit function parameter, appended as the **last** parameter of any changed signature — never via `context.Context`.
- Out-of-scope access always returns the same error a truly-nonexistent record would return (no new information leaks about whether the record exists elsewhere) — per spec §8.
- No audit event is added for out-of-scope access attempts (spec explicitly puts this out of scope for now).
- Every new/changed repository method keeps the existing error-wrapping style of its file (`fmt.Errorf("...: %w", err)` for unexpected errors, direct sentinel return for the expected not-found case).
- Every test file's existing stub-implementation style is preserved exactly: `internal/dcp3` currently has no repository stub in `service_test.go` (only pure-function tests) — this plan introduces one, matching the direct-implementation style used by `internal/distribution` and `internal/reports` (a stub implements every interface method directly, no embedding of the real type). `internal/api`'s test fakes embed the real interface type (`fakeDCP3Service DCP3Service`, etc.) — unchanged by this plan, only the methods they already override gain the new parameter.

---

### Task 1: DCP3 — Enforce Regency Scope on Preview/GetPreview/Commit

**Files:**
- Modify: `internal/dcp3/service.go`
- Modify: `internal/dcp3/repository.go`
- Create: `internal/dcp3/service_test.go` additions (repository stub + new tests; file already exists with pure-function tests only)
- Modify: `internal/dcp3/repository_integration_test.go`

**Interfaces:**
- Consumes: `auth.RegencyScope` (from PR1, `internal/auth/models.go`).
- Produces: `ImportService.Preview(ctx, actor, scheduleID, filename string, source io.Reader, meta auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error)`, `ImportService.GetPreview(ctx, id string, scope auth.RegencyScope) (ImportPreview, error)`, `ImportService.Commit(ctx, actor, batchID string, mapping Mapping, meta auth.ClientMeta, scope auth.RegencyScope) (ImportResult, error)` — these three new signatures are what Task 5 (API layer) will call.

- [ ] **Step 1: Write the failing unit test**

Add to `internal/dcp3/service_test.go` (after the existing imports, before `func ValidateMapping` tests — the file currently has no repository stub, so this introduces the package's first one):

```go
type importRepositoryStub struct {
	preview       ImportPreview
	result        ImportResult
	seenScope     auth.RegencyScope
	getPreviewErr error
}

func (r *importRepositoryStub) CreatePreview(_ context.Context, _ auth.Principal, _, _, _ string, _ WorkbookPreview, _ auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error) {
	r.seenScope = scope
	return r.preview, nil
}
func (r *importRepositoryStub) GetPreview(_ context.Context, _ string, scope auth.RegencyScope) (ImportPreview, error) {
	r.seenScope = scope
	if r.getPreviewErr != nil {
		return ImportPreview{}, r.getPreviewErr
	}
	return r.preview, nil
}
func (r *importRepositoryStub) Commit(_ context.Context, _ auth.Principal, batchID string, _ Mapping, _ auth.ClientMeta) (ImportResult, error) {
	return r.result, nil
}

func TestPreviewGetPreviewAndCommitForwardRegencyScope(t *testing.T) {
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	repository := &importRepositoryStub{}

	// Preview() parses the workbook before calling the repository, so this test exercises
	// CreatePreview's scope forwarding by calling the repository method directly rather than
	// going through Preview() with a fake .xlsx payload that would fail ParseWorkbook first.
	repository.preview = ImportPreview{ID: "batch-1", Status: "draft", Headers: []string{"Nama"}}
	if _, err := repository.CreatePreview(context.Background(), auth.Principal{}, "schedule-1", "file.xlsx", "checksum", WorkbookPreview{}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("CreatePreview scope=%+v", repository.seenScope)
	}

	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	repository.seenScope = auth.RegencyScope{}
	if _, err := service.GetPreview(context.Background(), "batch-1", scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("GetPreview scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.Commit(context.Background(), auth.Principal{}, "batch-1", Mapping{SourceSequence: "No", FullName: "Nama"}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("Commit did not forward scope through its internal GetPreview call: %+v", repository.seenScope)
	}
}

func TestGetPreviewReturnsRepositoryErrorForOutOfScopeBatch(t *testing.T) {
	repository := &importRepositoryStub{getPreviewErr: ErrPreviewNotFound}
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	if _, err := service.GetPreview(context.Background(), "batch-1", auth.RegencyScope{}); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("err=%v", err)
	}
}
```

Add `"konkit/internal/auth"` to the test file's import block (it is not currently imported — the existing file only imports `context`, `errors`, `testing`, and `konkit/internal/programs`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/dcp3 -run 'TestPreviewGetPreviewAndCommitForwardRegencyScope|TestGetPreviewReturnsRepositoryErrorForOutOfScopeBatch' -count=1`

Expected: FAIL (compile error — `importRepository` interface doesn't have these signatures yet, `Preview`/`GetPreview`/`Commit` don't take `scope` yet).

- [ ] **Step 3: Update the service**

Modify `internal/dcp3/service.go`:

```go
type importRepository interface {
	CreatePreview(context.Context, auth.Principal, string, string, string, WorkbookPreview, auth.ClientMeta, auth.RegencyScope) (ImportPreview, error)
	GetPreview(context.Context, string, auth.RegencyScope) (ImportPreview, error)
	Commit(context.Context, auth.Principal, string, Mapping, auth.ClientMeta) (ImportResult, error)
}
```

```go
func (s *ImportService) Preview(ctx context.Context, actor auth.Principal, scheduleID, filename string, source io.Reader, meta auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error) {
	data, err := io.ReadAll(io.LimitReader(source, s.limits.MaxBytes+1))
	if err != nil {
		return ImportPreview{}, err
	}
	if int64(len(data)) > s.limits.MaxBytes {
		return ImportPreview{}, ErrWorkbookTooLarge
	}
	preview, err := ParseWorkbook(bytes.NewReader(data), s.limits)
	if err != nil {
		return ImportPreview{}, err
	}
	digest := sha256.Sum256(data)
	return s.repository.CreatePreview(ctx, actor, strings.TrimSpace(scheduleID), strings.TrimSpace(filename), hex.EncodeToString(digest[:]), preview, meta, scope)
}

func (s *ImportService) GetPreview(ctx context.Context, id string, scope auth.RegencyScope) (ImportPreview, error) {
	return s.repository.GetPreview(ctx, strings.TrimSpace(id), scope)
}

func (s *ImportService) Commit(ctx context.Context, actor auth.Principal, batchID string, mapping Mapping, meta auth.ClientMeta, scope auth.RegencyScope) (ImportResult, error) {
	preview, err := s.repository.GetPreview(ctx, strings.TrimSpace(batchID), scope)
	if err != nil {
		return ImportResult{}, err
	}
	if preview.Status != "draft" {
		return ImportResult{}, ErrImportState
	}
	if err := ValidateMapping(preview.ProgramType, preview.Headers, mapping); err != nil {
		return ImportResult{}, err
	}
	return s.repository.Commit(ctx, actor, preview.ID, mapping, meta)
}
```

- [ ] **Step 4: Update the repository**

Modify `internal/dcp3/repository.go`. `CreatePreview`'s signature and INSERT query:

```go
func (r *Repository) CreatePreview(ctx context.Context, actor auth.Principal, scheduleID, filename, checksum string, workbook WorkbookPreview, meta auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ImportPreview{}, fmt.Errorf("begin DCP3 preview: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var batchID string
	headerEnvelope, _ := json.Marshal(map[string]any{"headers": workbook.Headers})
	err = tx.QueryRow(ctx, `
		INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,mapping_json,total_rows)
		SELECT s.id,$2,$3,$4,$5,$6 FROM program_schedules s WHERE s.id=$1 AND ($7 OR s.regency_id::text = ANY($8))
		RETURNING id::text
	`, scheduleID, filename, checksum, workbook.SheetName, headerEnvelope, len(workbook.Rows), scope.Unrestricted, scope.RegencyIDs).Scan(&batchID)
	if isUniqueViolation(err) {
		return ImportPreview{}, ErrDuplicateImport
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, ErrPreviewNotFound
	}
	if err != nil {
		return ImportPreview{}, fmt.Errorf("insert DCP3 preview: %w", err)
	}
	for _, row := range workbook.Rows {
		values := make(map[string]string, len(workbook.Headers))
		for index, header := range workbook.Headers {
			if index < len(row.Values) {
				values[header] = row.Values[index]
			}
		}
		raw, _ := json.Marshal(values)
		if _, err := tx.Exec(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,raw_data_json) VALUES($1,$2,$3)`, batchID, row.SourceRowNumber, raw); err != nil {
			return ImportPreview{}, fmt.Errorf("insert DCP3 preview row: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "dcp3.preview_created", ResourceType: "dcp3_import_batch", ResourceID: batchID, Metadata: map[string]any{"schedule_id": scheduleID, "filename": filename, "checksum": checksum, "row_count": len(workbook.Rows)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return ImportPreview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportPreview{}, fmt.Errorf("commit DCP3 preview: %w", err)
	}
	return r.GetPreview(ctx, batchID, auth.RegencyScope{Unrestricted: true})
}
```

(Only the SQL's `WHERE` clause and the two new args gained; the final line changed from `r.GetPreview(ctx, batchID)` to pass an unrestricted scope, since the row was just created inside a scope-checked transaction — re-reading it internally needs no second check.)

`GetPreview`:

```go
func (r *Repository) GetPreview(ctx context.Context, id string, scope auth.RegencyScope) (ImportPreview, error) {
	var result ImportPreview
	var programType programs.ProgramType
	var envelopeData []byte
	err := r.pool.QueryRow(ctx, `
		SELECT b.id::text,b.schedule_id::text,p.program_type,b.original_filename,b.file_checksum,b.sheet_name,b.mapping_json,b.status,b.created_at
		FROM dcp3_import_batches b JOIN program_schedules s ON s.id=b.schedule_id JOIN programs p ON p.id=s.program_id
		WHERE b.id=$1 AND ($2 OR s.regency_id::text = ANY($3))
	`, id, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.ScheduleID, &programType, &result.OriginalFilename, &result.FileChecksum, &result.SheetName, &envelopeData, &result.Status, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, ErrPreviewNotFound
	}
	if err != nil {
		return ImportPreview{}, fmt.Errorf("get DCP3 preview: %w", err)
	}
	result.ProgramType = programType
	var envelope struct {
		Headers []string `json:"headers"`
	}
	if err := json.Unmarshal(envelopeData, &envelope); err != nil {
		return ImportPreview{}, fmt.Errorf("decode DCP3 headers: %w", err)
	}
	result.Headers = envelope.Headers
	rows, err := r.pool.Query(ctx, `SELECT id::text,source_row_number,raw_data_json FROM dcp3_import_rows WHERE batch_id=$1 ORDER BY source_row_number`, id)
	if err != nil {
		return ImportPreview{}, fmt.Errorf("list DCP3 preview rows: %w", err)
	}
	defer rows.Close()
	result.Rows = []RawImportRow{}
	for rows.Next() {
		var item RawImportRow
		var raw []byte
		if err := rows.Scan(&item.ID, &item.SourceRowNumber, &raw); err != nil {
			return ImportPreview{}, fmt.Errorf("scan DCP3 preview row: %w", err)
		}
		if err := json.Unmarshal(raw, &item.Values); err != nil {
			return ImportPreview{}, fmt.Errorf("decode DCP3 preview row: %w", err)
		}
		result.Rows = append(result.Rows, item)
	}
	return result, rows.Err()
}
```

`Commit` itself is unchanged (its signature and SQL stay exactly as they are today — the scope check already happened in `Service.Commit`'s `GetPreview` call).

- [ ] **Step 5: Run the unit tests**

Run: `go test ./internal/dcp3 -count=1`

Expected: PASS (all existing pure-function tests plus the two new ones).

- [ ] **Step 6: Add the integration test**

Modify `internal/dcp3/repository_integration_test.go`. First, change `createDCP3ScheduleFixture` to also return the regency ID (it currently only returns `(scheduleID, personID string)`):

```go
func createDCP3ScheduleFixture(t *testing.T, pool *pgxpool.Pool) (string, string, string) {
```

...and its final line changes from `return scheduleID, personID` to `return scheduleID, personID, regencyID`. Update the existing call site at the top of `TestIntegrationCommitImportsIdentitiesAllocationsAndDocumentation`:

```go
	scheduleID, conflictingPersonID, _ := createDCP3ScheduleFixture(t, pool)
```

Then append a new test after `TestIntegrationCommitImportsIdentitiesAllocationsAndDocumentation`:

```go
func TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := dcp3IntegrationPool(t)
	ctx := context.Background()
	scheduleID, _, regencyID := createDCP3ScheduleFixture(t, pool)
	repository := NewRepository(pool)
	service := NewImportService(repository, ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100})
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "dcp3-scope-integration-test"}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE user_agent='dcp3-scope-integration-test'`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
	})
	workbook := workbookBytes(t, func(file *excelize.File) {
		rows := [][]any{{"No", "Nama"}, {1, "Siti Aminah"}}
		for index, row := range rows {
			_ = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", index+1), &row)
		}
	})
	outOfScope := auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}
	if _, err := service.Preview(ctx, auth.Principal{}, scheduleID, "out-of-scope.xlsx", bytes.NewReader(workbook), meta, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound for out-of-scope schedule, got %v", err)
	}

	inScope := auth.RegencyScope{RegencyIDs: []string{regencyID}}
	preview, err := service.Preview(ctx, auth.Principal{}, scheduleID, "in-scope.xlsx", bytes.NewReader(workbook), meta, inScope)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.GetPreview(ctx, preview.ID, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound reading preview out of scope, got %v", err)
	}
	if _, err := service.GetPreview(ctx, preview.ID, inScope); err != nil {
		t.Fatalf("in-scope read should succeed: %v", err)
	}
	if _, err := service.Commit(ctx, auth.Principal{}, preview.ID, Mapping{SourceSequence: "No", FullName: "Nama"}, meta, outOfScope); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expected ErrPreviewNotFound committing out of scope, got %v", err)
	}
}
```

- [ ] **Step 7: Run the integration test**

Run (with `TEST_DATABASE_URL` set per README):

```powershell
go test ./internal/dcp3 -run TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess -count=1 -v
go test ./internal/dcp3 -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/dcp3
git commit -m "feat: enforce regency scope on DCP3 preview and import"
```

---

### Task 2: Pendistribusian — Enforce Regency Scope on Search/GetWorkspace/SaveDraft/Complete

**Files:**
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Produces: `Service.Search(ctx, scheduleID, query string, limit int, scope auth.RegencyScope) ([]SearchResult, error)`, `Service.GetWorkspace(ctx, allocationID string, scope auth.RegencyScope) (RecipientWorkspace, error)`, `Service.SaveDraft(ctx, actor, allocationID string, input DraftInput, meta auth.ClientMeta, scope auth.RegencyScope) (RecipientWorkspace, error)`, `Service.Complete(ctx, actor, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionRecord, error)`.

- [ ] **Step 1: Write the failing unit test**

Modify `internal/distribution/service_test.go`. Update `repositoryStub` to capture scope and add a `Complete` method (so `NewService` picks it up via the existing `repository.(completionRepository)` type assertion, letting `Complete` be unit-tested without a separate stub type):

```go
type repositoryStub struct {
	searchRecords []SearchRecord
	searchQuery   string
	searchLimit   int
	workspace     RecipientWorkspace
	saved         DraftInput
	seenScope     auth.RegencyScope
	completed     DistributionRecord
	completeErr   error
}

func (r *repositoryStub) Search(_ context.Context, _ string, query string, limit int, scope auth.RegencyScope) ([]SearchRecord, error) {
	r.searchQuery, r.searchLimit, r.seenScope = query, limit, scope
	return r.searchRecords, nil
}
func (r *repositoryStub) GetWorkspace(_ context.Context, _ string, scope auth.RegencyScope) (RecipientWorkspace, error) {
	r.seenScope = scope
	return r.workspace, nil
}
func (r *repositoryStub) SaveDraft(_ context.Context, _ auth.Principal, _ string, input DraftInput, _ auth.ClientMeta) (RecipientWorkspace, error) {
	r.saved = input
	return r.workspace, nil
}
func (r *repositoryStub) Complete(_ context.Context, _ auth.Principal, _ string, _ auth.ClientMeta, scope auth.RegencyScope) (DistributionRecord, error) {
	r.seenScope = scope
	if r.completeErr != nil {
		return DistributionRecord{}, r.completeErr
	}
	return r.completed, nil
}
```

Update every existing call site in this file that calls `service.Search(...)`, `service.GetWorkspace(...)` (via `SaveDraft`'s internal call — no test calls `GetWorkspace` directly today), `service.SaveDraft(...)`, to append `auth.RegencyScope{Unrestricted: true}` as the last argument (this keeps their existing assertions valid — they're not testing scope, just keeping the calls compiling):

- `TestSearchValidatesContextAndNameLength` — 3 calls to `service.Search(context.Background(), ..., 20)` each become `service.Search(context.Background(), ..., 20, auth.RegencyScope{Unrestricted: true})`.
- `TestSearchAllowsSingleDistributionNumberMasksNIKAndCapsResults` — `service.Search(context.Background(), " schedule-1 ", " 7 ", 200)` becomes `service.Search(context.Background(), " schedule-1 ", " 7 ", 200, auth.RegencyScope{Unrestricted: true})`.
- `TestSaveDraftRequiresReasonWhenNIKChanges` — both `service.SaveDraft(...)` calls gain `, auth.RegencyScope{Unrestricted: true}` before the closing paren.
- `TestSaveDraftPassesEquipmentFieldsThrough` — same.

Then append new tests at the end of the file:

```go
func TestSearchGetWorkspaceSaveDraftAndCompleteForwardRegencyScope(t *testing.T) {
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	repository := &repositoryStub{workspace: RecipientWorkspace{NIK: "7306014101900001"}}
	service := NewService(repository)

	if _, err := service.Search(context.Background(), "schedule-1", "Siti", 20, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("Search scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.GetWorkspace(context.Background(), "allocation-1", scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("GetWorkspace scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	input := DraftInput{NIK: "7306014101900001"}
	if _, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("SaveDraft did not forward scope through its internal GetWorkspace call: %+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.Complete(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("Complete scope=%+v", repository.seenScope)
	}
}

func TestGetWorkspaceReturnsRepositoryErrorForOutOfScopeAllocation(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	repository.completeErr = ErrAllocationNotFound
	if _, err := service.Complete(context.Background(), auth.Principal{}, "allocation-1", auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("err=%v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/distribution -run 'TestSearchGetWorkspaceSaveDraftAndCompleteForwardRegencyScope|TestGetWorkspaceReturnsRepositoryErrorForOutOfScopeAllocation' -count=1`

Expected: FAIL (compile errors — signatures don't match yet).

- [ ] **Step 3: Update the service**

Modify `internal/distribution/service.go`:

```go
type repository interface {
	Search(context.Context, string, string, int, auth.RegencyScope) ([]SearchRecord, error)
	GetWorkspace(context.Context, string, auth.RegencyScope) (RecipientWorkspace, error)
	SaveDraft(context.Context, auth.Principal, string, DraftInput, auth.ClientMeta) (RecipientWorkspace, error)
}
```

```go
type completionRepository interface {
	Complete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (DistributionRecord, error)
}
```

```go
func (s *Service) Complete(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionRecord, error) {
	allocationID = strings.TrimSpace(allocationID)
	if allocationID == "" {
		return DistributionRecord{}, ErrAllocationNotFound
	}
	if s.completion == nil {
		return DistributionRecord{}, errors.New("distribution completion is unavailable")
	}
	return s.completion.Complete(ctx, actor, allocationID, meta, scope)
}

func (s *Service) Search(ctx context.Context, scheduleID, query string, limit int, scope auth.RegencyScope) ([]SearchResult, error) {
	scheduleID, query = strings.TrimSpace(scheduleID), strings.TrimSpace(query)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if query == "" {
		return nil, ErrQueryRequired
	}
	if len([]rune(query)) < 2 && !onlyDigits.MatchString(query) {
		return nil, ErrQueryTooShort
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	records, err := s.repository.Search(ctx, scheduleID, query, limit, scope)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(records))
	for _, record := range records {
		results = append(results, SearchResult{
			AllocationID: record.AllocationID, DistributionNumber: record.DistributionNumber,
			FullName: record.FullName, MaskedNIK: maskNIK(record.NIK), Location: record.Location,
			ProgramType: record.ProgramType, Eligibility: record.Eligibility,
			AllocationStatus: record.AllocationStatus, Documentation: record.Documentation,
		})
	}
	return results, nil
}

func (s *Service) GetWorkspace(ctx context.Context, allocationID string, scope auth.RegencyScope) (RecipientWorkspace, error) {
	if strings.TrimSpace(allocationID) == "" {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	return s.repository.GetWorkspace(ctx, strings.TrimSpace(allocationID), scope)
}

func (s *Service) SaveDraft(ctx context.Context, actor auth.Principal, allocationID string, input DraftInput, meta auth.ClientMeta, scope auth.RegencyScope) (RecipientWorkspace, error) {
	current, err := s.GetWorkspace(ctx, allocationID, scope)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	input.NIK = stripNonDigits.ReplaceAllString(input.NIK, "")
	input.Address = strings.TrimSpace(input.Address)
	input.Village = strings.TrimSpace(input.Village)
	input.District = strings.TrimSpace(input.District)
	input.PhoneNumber = stripNonDigits.ReplaceAllString(input.PhoneNumber, "")
	input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)
	input.IdentityChangeReason = strings.TrimSpace(input.IdentityChangeReason)
	input.MachineOptionCode = strings.TrimSpace(input.MachineOptionCode)
	input.MachineSerialNumber = strings.TrimSpace(input.MachineSerialNumber)
	input.HoseOptionCode = strings.TrimSpace(input.HoseOptionCode)
	input.HoseSerialNumber = strings.TrimSpace(input.HoseSerialNumber)
	input.ConverterSerialNumber = strings.TrimSpace(input.ConverterSerialNumber)
	if input.NIK != "" && len(input.NIK) != 16 {
		return RecipientWorkspace{}, ErrNIKInvalid
	}
	if input.NIK != current.NIK && input.IdentityChangeReason == "" {
		return RecipientWorkspace{}, ErrIdentityChangeReasonRequired
	}
	return s.repository.SaveDraft(ctx, actor, strings.TrimSpace(allocationID), input, meta)
}
```

(`repository.SaveDraft`'s own signature is unchanged — the scope check already happened via `s.GetWorkspace` above.)

- [ ] **Step 4: Update the repository**

Modify `internal/distribution/repository.go`. `Search`:

```go
func (r *Repository) Search(ctx context.Context, scheduleID, query string, limit int, scope auth.RegencyScope) ([]SearchRecord, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	identifier := normalizeIdentifier(query)
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text,a.distribution_number,
			COALESCE(p.full_name,ir.normalized_data_json->>'full_name','Data perlu ditinjau'),COALESCE(p.nik,''),
			concat_ws(', ',NULLIF(p.village,''),NULLIF(p.district,''),rg.name),pr.program_type,a.status,dr.id::text,
			CASE
				WHEN EXISTS(SELECT 1 FROM distribution_records old WHERE old.recipient_person_id=p.id AND old.status='completed' AND old.allocation_id<>a.id) THEN 'previously_received'
				WHEN p.id IS NULL OR a.status='needs_review' THEN 'incomplete'
				ELSE 'eligible'
			END eligibility
		FROM package_allocations a
		JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN dcp3_import_rows ir ON ir.id=n.import_row_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		JOIN programs pr ON pr.id=ps.program_id
		JOIN regencies rg ON rg.id=ps.regency_id
		LEFT JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,a.intended_person_id,n.person_id)
		LEFT JOIN distribution_records dr ON dr.allocation_id=a.id
		WHERE a.schedule_id=$1 AND ($6 OR ps.regency_id::text = ANY($7)) AND (
			a.distribution_number::text=$2 OR p.nik=NULLIF($3,'') OR
			EXISTS(SELECT 1 FROM person_sector_identifiers psi WHERE psi.person_id=p.id AND psi.normalized_value=NULLIF($4,'')) OR
			lower(p.full_name) LIKE lower($2)||'%' OR lower(p.full_name) LIKE '%'||lower($2)||'%'
		)
		ORDER BY CASE
			WHEN a.distribution_number::text=$2 THEN 0
			WHEN p.nik=NULLIF($3,'') OR EXISTS(SELECT 1 FROM person_sector_identifiers psi WHERE psi.person_id=p.id AND psi.normalized_value=NULLIF($4,'')) THEN 1
			WHEN lower(p.full_name) LIKE lower($2)||'%' THEN 2 ELSE 3 END,
			lower(COALESCE(p.full_name,'')),a.distribution_number
		LIMIT $5
	`, scheduleID, query, digits, identifier, limit, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("search distribution recipients: %w", err)
	}
	defer rows.Close()
	results := []SearchRecord{}
	for rows.Next() {
		var item SearchRecord
		var distributionID string
		if err := rows.Scan(&item.AllocationID, &item.DistributionNumber, &item.FullName, &item.NIK, &item.Location, &item.ProgramType, &item.AllocationStatus, &distributionID, &item.Eligibility); err != nil {
			return nil, fmt.Errorf("scan distribution recipient: %w", err)
		}
		item.Documentation, err = r.listSlots(ctx, distributionID)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
```

`GetWorkspace` — only the `WHERE` clause and args change:

```go
func (r *Repository) GetWorkspace(ctx context.Context, allocationID string, scope auth.RegencyScope) (RecipientWorkspace, error) {
	var result RecipientWorkspace
	var sourceJSON, packageJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT a.id::text,dr.id::text,a.schedule_id::text,a.distribution_number,a.status,dr.status,
			pr.program_type,pr.name,rg.name,COALESCE(p.full_name,''),COALESCE(p.nik,''),
			COALESCE(psi.display_value,''),COALESCE(psi.identifier_type,''),COALESCE(p.address,''),
			COALESCE(p.village,''),COALESCE(p.district,''),COALESCE(p.phone_number,''),n.source_snapshot_json,a.package_snapshot_json,
			COALESCE(dr.machine_option_code,''),COALESCE(dr.machine_serial_number,''),COALESCE(dr.hose_option_code,''),COALESCE(dr.hose_serial_number,''),COALESCE(dr.converter_serial_number,''),
			CASE
				WHEN EXISTS(SELECT 1 FROM distribution_records old WHERE old.recipient_person_id=p.id AND old.status='completed' AND old.allocation_id<>a.id) THEN 'previously_received'
				WHEN p.id IS NULL OR a.status='needs_review' THEN 'incomplete'
				ELSE 'eligible'
			END
		FROM package_allocations a
		JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		JOIN programs pr ON pr.id=ps.program_id
		JOIN regencies rg ON rg.id=ps.regency_id
		JOIN distribution_records dr ON dr.allocation_id=a.id
		LEFT JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,dr.recipient_person_id,a.intended_person_id,n.person_id)
		LEFT JOIN LATERAL (
			SELECT identifier_type,display_value FROM person_sector_identifiers
			WHERE person_id=p.id AND identifier_type=CASE WHEN pr.program_type='farmer' THEN 'farmer_card' ELSE 'kusuka' END LIMIT 1
		) psi ON true
		WHERE a.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, allocationID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&result.AllocationID, &result.DistributionID, &result.ScheduleID, &result.DistributionNumber,
		&result.AllocationStatus, &result.DistributionStatus, &result.ProgramType, &result.ProgramName,
		&result.RegencyName, &result.FullName, &result.NIK, &result.SectorIdentifier,
		&result.SectorIdentifierType, &result.Address, &result.Village, &result.District,
		&result.PhoneNumber, &sourceJSON, &packageJSON,
		&result.MachineOptionCode, &result.MachineSerialNumber, &result.HoseOptionCode, &result.HoseSerialNumber, &result.ConverterSerialNumber,
		&result.Eligibility,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("get recipient workspace: %w", err)
	}
	result.SourceSnapshot = map[string]any{}
	if err := json.Unmarshal(sourceJSON, &result.SourceSnapshot); err != nil {
		return RecipientWorkspace{}, fmt.Errorf("decode recipient source snapshot: %w", err)
	}
	result.PackageSnapshot = map[string]any{}
	if err := json.Unmarshal(packageJSON, &result.PackageSnapshot); err != nil {
		return RecipientWorkspace{}, fmt.Errorf("decode package snapshot: %w", err)
	}
	result.Documentation, err = r.listSlots(ctx, result.DistributionID)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	result.ReceiptHistory, err = r.listReceiptHistory(ctx, allocationID)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	result.EligibilityReasons = []string{}
	if result.Eligibility == "previously_received" {
		result.EligibilityReasons = append(result.EligibilityReasons, "Penerima tercatat sudah menerima paket pada program sebelumnya")
	} else if result.Eligibility == "incomplete" {
		result.EligibilityReasons = append(result.EligibilityReasons, "Data identitas penerima perlu dilengkapi atau ditinjau")
	}
	return result, nil
}
```

`SaveDraft`'s internal tail call changes from `return r.GetWorkspace(ctx, allocationID)` to `return r.GetWorkspace(ctx, allocationID, auth.RegencyScope{Unrestricted: true})` (the row was already scope-checked earlier in this same repository call via the lock-select — re-reading it internally needs no second check). No other change to `SaveDraft`.

`Complete` — add scope param, add the `WHERE` condition, and change the final internal call the same way `SaveDraft` does (it doesn't call `GetWorkspace` again, but it does re-query nothing else — no other internal call to change):

```go
func (r *Repository) Complete(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionRecord, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionRecord{}, fmt.Errorf("begin distribution completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var record DistributionRecord
	var allocationStatus, personID, fullName, nik, programType, sectorType, sectorIdentifier string
	var machineOptionCode, machineSerialNumber, hoseOptionCode, hoseSerialNumber, converterSerialNumber string
	var packageJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT dr.id::text,dr.status,a.status,p.id::text,p.full_name,COALESCE(p.nik,''),pr.program_type,
			COALESCE(psi.identifier_type,''),COALESCE(psi.normalized_value,''),a.package_snapshot_json,
			COALESCE(dr.machine_option_code,''),COALESCE(dr.machine_serial_number,''),COALESCE(dr.hose_option_code,''),COALESCE(dr.hose_serial_number,''),COALESCE(dr.converter_serial_number,'')
		FROM package_allocations a
		JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN distribution_records dr ON dr.allocation_id=a.id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		JOIN programs pr ON pr.id=ps.program_id
		JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,dr.recipient_person_id,a.intended_person_id,n.person_id)
		LEFT JOIN LATERAL (
			SELECT identifier_type,normalized_value FROM person_sector_identifiers
			WHERE person_id=p.id AND identifier_type=CASE WHEN pr.program_type='farmer' THEN 'farmer_card' ELSE 'kusuka' END LIMIT 1
		) psi ON true
		WHERE a.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF a,dr,p
	`, allocationID, scope.Unrestricted, scope.RegencyIDs).Scan(&record.ID, &record.Status, &allocationStatus, &personID, &fullName, &nik, &programType, &sectorType, &sectorIdentifier, &packageJSON,
		&machineOptionCode, &machineSerialNumber, &hoseOptionCode, &hoseSerialNumber, &converterSerialNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionRecord{}, ErrAllocationNotFound
	}
	if err != nil {
		return DistributionRecord{}, fmt.Errorf("lock distribution completion: %w", err)
	}
	if record.Status == "completed" || allocationStatus == "distributed" {
		return DistributionRecord{}, ErrAlreadyCompleted
	}
	if strings.TrimSpace(fullName) == "" || len(stripNonDigits.ReplaceAllString(nik, "")) != 16 || strings.TrimSpace(sectorIdentifier) == "" {
		return DistributionRecord{}, ErrIdentityIncomplete
	}
	// ... rest of the function body is unchanged from here.
```

(Everything from `var previouslyReceived bool` to the end of the function stays exactly as it is today — only the initial lock-select's `WHERE`/args and the func signature change.)

- [ ] **Step 5: Run the unit tests**

Run: `go test ./internal/distribution -count=1`

Expected: PASS.

- [ ] **Step 6: Add the integration test**

Modify `internal/distribution/repository_integration_test.go`. Add `regencyID` and `historyRegencyID` to the `distributionFixture` struct (the local variables already exist in `createDistributionFixture`, they're just not currently returned):

```go
type distributionFixture struct {
	scheduleID, allocationID, historyAllocationID, secondaryAllocationID string
	secondaryPersonID, userID, userAgent                                 string
	regencyID, historyRegencyID                                          string
}
```

Change the `return` statement at the end of `createDistributionFixture` to include the two new fields:

```go
	return distributionFixture{scheduleID: scheduleID, allocationID: allocationID, historyAllocationID: historyAllocationID, secondaryAllocationID: secondaryAllocationID, secondaryPersonID: sitiNurID, userAgent: userAgent, regencyID: regencyID, historyRegencyID: historyRegencyID}
```

Then append a new test after `TestIntegrationSearchRanksIdentifiersAndShowsCrossScheduleHistory`:

```go
func TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := createDistributionFixture(t, pool)
	repository := NewRepository(pool)
	service := NewService(repository)

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}}
	otherRegencyOnly := auth.RegencyScope{RegencyIDs: []string{fixture.historyRegencyID}}

	results, err := service.Search(ctx, fixture.scheduleID, "Siti", 20, otherRegencyOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("out-of-scope search should return no matches, got %+v", results)
	}
	results, err = service.Search(ctx, fixture.scheduleID, "Siti", 20, scoped)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("in-scope search should return matches")
	}

	if _, err := service.GetWorkspace(ctx, fixture.allocationID, otherRegencyOnly); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound reading out-of-scope allocation, got %v", err)
	}
	if _, err := service.GetWorkspace(ctx, fixture.allocationID, scoped); err != nil {
		t.Fatalf("in-scope read should succeed: %v", err)
	}

	if _, err := service.SaveDraft(ctx, auth.Principal{}, fixture.allocationID, DraftInput{}, auth.ClientMeta{UserAgent: fixture.userAgent}, otherRegencyOnly); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound saving draft out of scope, got %v", err)
	}

	if _, err := service.Complete(ctx, auth.Principal{}, fixture.historyAllocationID, auth.ClientMeta{UserAgent: fixture.userAgent}, scoped); !errors.Is(err, ErrAllocationNotFound) {
		t.Fatalf("expected ErrAllocationNotFound completing allocation from a different regency, got %v", err)
	}
}
```

(`fixture.historyAllocationID` belongs to `historyRegencyID`, so checking it against `scoped` — which only allows `fixture.regencyID` — exercises the cross-regency rejection for `Complete` without needing a third fixture.)

- [ ] **Step 7: Run the integration test**

Run:

```powershell
go test ./internal/distribution -run TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess -count=1 -v
go test ./internal/distribution -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/distribution
git commit -m "feat: enforce regency scope on distribution search, workspace, draft, and completion"
```

---

### Task 3: Pendistribusian — Enforce Regency Scope on Media Upload/Delete/Open

**Files:**
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: Task 2's `repository`/`completionRepository` changes (same file, same package — this task is sequenced after Task 2 so it can reuse the already-updated `repositoryStub`).
- Produces: `Service.UploadMedia(ctx, actor, input UploadMediaInput, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error)`, `Service.DeleteMedia(ctx, actor, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) error`, `Service.OpenMedia(ctx, mediaID string, scope auth.RegencyScope) (MediaContent, error)`.

The identifier chain for all three is `documentation_slots.distribution_id → distribution_records.id`, `distribution_records.allocation_id → package_allocations.id`, `package_allocations.schedule_id → program_schedules.id`, `program_schedules.regency_id` — three joins beyond each query's current tables, fused into the existing single-row query (no extra round trip).

- [ ] **Step 1: Write the failing unit test**

Modify `internal/distribution/service_test.go`. Update `mediaRepositoryStub` to capture scope:

```go
type mediaRepositoryStub struct {
	repositoryStub
	slot       MediaSlot
	media      MediaFile
	saveErr    error
	restoredID string
	mediaScope auth.RegencyScope
}

func (r *mediaRepositoryStub) GetMediaSlot(_ context.Context, _ string, scope auth.RegencyScope) (MediaSlot, error) {
	r.mediaScope = scope
	return r.slot, nil
}
func (r *mediaRepositoryStub) SaveMedia(_ context.Context, _ auth.Principal, input MediaFileInput, _ auth.ClientMeta) (MediaFile, error) {
	if r.saveErr != nil {
		return MediaFile{}, r.saveErr
	}
	r.media = MediaFile{ID: "media-1", SlotID: input.SlotID, StorageKey: input.StorageKey, MimeType: input.MimeType, ByteSize: input.ByteSize, Status: "accepted"}
	return r.media, nil
}
func (r *mediaRepositoryStub) GetMedia(_ context.Context, _ string, scope auth.RegencyScope) (MediaFile, error) {
	r.mediaScope = scope
	return r.media, nil
}
func (r *mediaRepositoryStub) DeleteMedia(_ context.Context, _ auth.Principal, _ string, _ auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	r.mediaScope = scope
	return r.media, nil
}
func (r *mediaRepositoryStub) RestoreMedia(_ context.Context, id string) error {
	r.restoredID = id
	return nil
}
```

Update every existing call site of `service.UploadMedia(...)` and `service.DeleteMedia(...)` in this file (in `TestUploadMediaDetectsImageAndCleansStorageWhenMetadataFails`, `TestUploadMediaEnforcesTypeSizeAndSlotRequirements`, `TestDeleteMediaRestoresMetadataWhenStorageDeleteFails`) to append `, auth.RegencyScope{Unrestricted: true}` as the final argument.

Append a new test:

```go
func TestUploadDeleteAndOpenMediaForwardRegencyScope(t *testing.T) {
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	storage := &storageStub{}
	repository := &mediaRepositoryStub{slot: MediaSlot{ID: "slot-1", InputSource: "camera", MinFiles: 1, MaxFiles: 2}, media: MediaFile{ID: "media-1", StorageKey: "opaque-key", Status: "accepted"}}
	service := NewService(repository, storage)
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)

	if _, err := service.UploadMedia(context.Background(), auth.Principal{}, UploadMediaInput{SlotID: "slot-1", Source: "camera", Data: jpeg}, auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("UploadMedia scope=%+v", repository.mediaScope)
	}

	repository.mediaScope = auth.RegencyScope{}
	if _, err := service.OpenMedia(context.Background(), "media-1", scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("OpenMedia scope=%+v", repository.mediaScope)
	}

	repository.mediaScope = auth.RegencyScope{}
	if err := service.DeleteMedia(context.Background(), auth.Principal{}, "media-1", auth.ClientMeta{}, scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.mediaScope.RegencyIDs) != 1 || repository.mediaScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("DeleteMedia scope=%+v", repository.mediaScope)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/distribution -run TestUploadDeleteAndOpenMediaForwardRegencyScope -count=1`

Expected: FAIL (compile error).

- [ ] **Step 3: Update the service**

Modify `internal/distribution/service.go`:

```go
type mediaRepository interface {
	GetMediaSlot(context.Context, string, auth.RegencyScope) (MediaSlot, error)
	SaveMedia(context.Context, auth.Principal, MediaFileInput, auth.ClientMeta) (MediaFile, error)
	GetMedia(context.Context, string, auth.RegencyScope) (MediaFile, error)
	DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (MediaFile, error)
	RestoreMedia(context.Context, string) error
}
```

```go
func (s *Service) UploadMedia(ctx context.Context, actor auth.Principal, input UploadMediaInput, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaFile{}, ErrMediaUnavailable
	}
	if len(input.Data) == 0 {
		return MediaFile{}, ErrMediaTypeInvalid
	}
	if len(input.Data) > maxMediaBytes {
		return MediaFile{}, ErrMediaTooLarge
	}
	slot, err := s.mediaRepository.GetMediaSlot(ctx, strings.TrimSpace(input.SlotID), scope)
	if err != nil {
		return MediaFile{}, err
	}
	input.Source = strings.TrimSpace(input.Source)
	if (slot.InputSource != "both" && slot.InputSource != input.Source) || (input.Source != "camera" && input.Source != "gallery") {
		return MediaFile{}, ErrMediaSourceInvalid
	}
	if slot.AcceptedFiles >= slot.MaxFiles {
		return MediaFile{}, ErrMediaLimitReached
	}
	if slot.RequireLocation && (input.Latitude == nil || input.Longitude == nil) {
		return MediaFile{}, ErrMediaLocationRequired
	}
	if slot.RequireCapturedAt && input.CapturedAt == nil {
		return MediaFile{}, ErrMediaCapturedAtRequired
	}
	mimeType := http.DetectContentType(input.Data[:min(len(input.Data), 512)])
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		return MediaFile{}, ErrMediaTypeInvalid
	}
	key, err := newStorageKey()
	if err != nil {
		return MediaFile{}, err
	}
	size, checksum, err := s.storage.Put(ctx, key, bytes.NewReader(input.Data))
	if err != nil {
		return MediaFile{}, err
	}
	stored, err := s.mediaRepository.SaveMedia(ctx, actor, MediaFileInput{SlotID: slot.ID, StorageKey: key, OriginalFilename: strings.TrimSpace(input.OriginalFilename), MimeType: mimeType, Checksum: checksum, Source: input.Source, ByteSize: size, CapturedAt: input.CapturedAt, Latitude: input.Latitude, Longitude: input.Longitude}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return MediaFile{}, err
	}
	stored.ContentURL = "/api/v1/distribution/media/" + stored.ID + "/content"
	return stored, nil
}

func (s *Service) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	if s.storage == nil || s.mediaRepository == nil {
		return ErrMediaUnavailable
	}
	item, err := s.mediaRepository.DeleteMedia(ctx, actor, strings.TrimSpace(mediaID), meta, scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, item.StorageKey); err != nil {
		_ = s.mediaRepository.RestoreMedia(context.Background(), item.ID)
		return err
	}
	return nil
}

func (s *Service) OpenMedia(ctx context.Context, mediaID string, scope auth.RegencyScope) (MediaContent, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaContent{}, ErrMediaUnavailable
	}
	item, err := s.mediaRepository.GetMedia(ctx, strings.TrimSpace(mediaID), scope)
	if err != nil {
		return MediaContent{}, err
	}
	reader, err := s.storage.Open(ctx, item.StorageKey)
	if err != nil {
		return MediaContent{}, err
	}
	return MediaContent{Reader: reader, MimeType: item.MimeType, Filename: item.OriginalFilename}, nil
}
```

- [ ] **Step 4: Update the repository**

Modify `internal/distribution/repository.go`. `GetMediaSlot`:

```go
func (r *Repository) GetMediaSlot(ctx context.Context, slotID string, scope auth.RegencyScope) (MediaSlot, error) {
	var result MediaSlot
	err := r.pool.QueryRow(ctx, `
		SELECT s.id::text,s.input_source,s.require_location,s.require_captured_at,s.min_files,s.max_files,count(m.id) FILTER(WHERE m.status='accepted')
		FROM documentation_slots s
		LEFT JOIN media_files m ON m.documentation_slot_id=s.id
		JOIN distribution_records dr ON dr.id=s.distribution_id
		JOIN package_allocations a ON a.id=dr.allocation_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		WHERE s.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY s.id
	`, slotID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.InputSource, &result.RequireLocation, &result.RequireCapturedAt, &result.MinFiles, &result.MaxFiles, &result.AcceptedFiles)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaSlot{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaSlot{}, fmt.Errorf("get documentation slot: %w", err)
	}
	return result, nil
}
```

`GetMedia`:

```go
func (r *Repository) GetMedia(ctx context.Context, mediaID string, scope auth.RegencyScope) (MediaFile, error) {
	var result MediaFile
	err := r.pool.QueryRow(ctx, `
		SELECT m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.uploaded_at
		FROM media_files m
		JOIN documentation_slots s ON s.id=m.documentation_slot_id
		JOIN distribution_records dr ON dr.id=s.distribution_id
		JOIN package_allocations a ON a.id=dr.allocation_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text = ANY($3))
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("get documentation media: %w", err)
	}
	result.ContentURL = "/api/v1/distribution/media/" + result.ID + "/content"
	return result, nil
}
```

`DeleteMedia`:

```go
func (r *Repository) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result MediaFile
	err = tx.QueryRow(ctx, `
		UPDATE media_files m
		SET status='deleted', updated_at=now()
		FROM documentation_slots s
		JOIN distribution_records dr ON dr.id=s.distribution_id
		JOIN package_allocations a ON a.id=dr.allocation_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		WHERE m.documentation_slot_id=s.id
			AND m.id=$1 AND m.status='accepted'
			AND ($2 OR ps.regency_id::text = ANY($3))
		RETURNING m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.uploaded_at
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("mark media deleted: %w", err)
	}
	if err := updateSlotStatus(ctx, tx, result.SlotID); err != nil {
		return MediaFile{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation.media_deleted", ResourceType: "media_file", ResourceID: result.ID, Metadata: map[string]any{"slot_id": result.SlotID, "mime_type": result.MimeType, "byte_size": result.ByteSize}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return MediaFile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MediaFile{}, fmt.Errorf("commit media delete: %w", err)
	}
	return result, nil
}
```

`SaveMedia` and `RestoreMedia` are unchanged.

- [ ] **Step 5: Run the unit tests**

Run: `go test ./internal/distribution -count=1`

Expected: PASS.

- [ ] **Step 6: Add the integration test**

Append to `internal/distribution/repository_integration_test.go`, after `TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess` (from Task 2):

```go
func TestIntegrationMediaScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	fixture := createDistributionFixture(t, pool)
	repository := NewRepository(pool)
	storage := &storageStub{}
	service := NewService(repository, storage)

	var slotID string
	if err := pool.QueryRow(ctx, `SELECT s.id::text FROM documentation_slots s JOIN distribution_records d ON d.id=s.distribution_id WHERE d.allocation_id=$1 AND s.slot_code='recipient_package'`, fixture.allocationID).Scan(&slotID); err != nil {
		t.Fatal(err)
	}

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}}
	otherRegencyOnly := auth.RegencyScope{RegencyIDs: []string{fixture.historyRegencyID}}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...)

	if _, err := service.UploadMedia(ctx, auth.Principal{}, UploadMediaInput{SlotID: slotID, Source: "camera", Data: jpeg}, auth.ClientMeta{UserAgent: fixture.userAgent}, otherRegencyOnly); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound uploading to out-of-scope slot, got %v", err)
	}
	uploaded, err := service.UploadMedia(ctx, auth.Principal{}, UploadMediaInput{SlotID: slotID, Source: "camera", Data: jpeg}, auth.ClientMeta{UserAgent: fixture.userAgent}, scoped)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.OpenMedia(ctx, uploaded.ID, otherRegencyOnly); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound opening out-of-scope media, got %v", err)
	}
	if _, err := service.OpenMedia(ctx, uploaded.ID, scoped); err != nil {
		t.Fatalf("in-scope open should succeed: %v", err)
	}

	if err := service.DeleteMedia(ctx, auth.Principal{}, uploaded.ID, auth.ClientMeta{UserAgent: fixture.userAgent}, otherRegencyOnly); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound deleting out-of-scope media, got %v", err)
	}
	if err := service.DeleteMedia(ctx, auth.Principal{}, uploaded.ID, auth.ClientMeta{UserAgent: fixture.userAgent}, scoped); err != nil {
		t.Fatalf("in-scope delete should succeed: %v", err)
	}
}
```

- [ ] **Step 7: Run the integration test**

Run:

```powershell
go test ./internal/distribution -run TestIntegrationMediaScopeEnforcementRejectsOutOfRegencyAccess -count=1 -v
go test ./internal/distribution -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/distribution
git commit -m "feat: enforce regency scope on distribution media upload, delete, and open"
```

---

### Task 4: Laporan — Add ErrScheduleNotFound and Enforce Regency Scope

**Files:**
- Modify: `internal/reports/models.go`
- Modify: `internal/reports/service.go`
- Modify: `internal/reports/repository.go`
- Modify: `internal/reports/service_test.go`
- Modify: `internal/reports/repository_integration_test.go`

**Interfaces:**
- Produces: `reports.ErrScheduleNotFound` (new sentinel), `Service.Summary(ctx, scheduleID string, filter Filter, scope auth.RegencyScope) (Summary, error)`, `Service.Rows(ctx, scheduleID string, filter Filter, scope auth.RegencyScope) ([]Row, error)`, `Service.ExportExcel(ctx, actor, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error)`, `Service.ExportPDF(ctx, actor, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error)`.
- Task 5 also needs `reports.ErrScheduleNotFound` added to `internal/api/routes.go`'s `writeServiceError` not-found case — that edit lives in Task 5, not here, since it touches `internal/api`.

- [ ] **Step 1: Write the failing unit test**

Modify `internal/reports/service_test.go`. Update `repositoryStub` to add `ScheduleRegency`:

```go
type repositoryStub struct {
	summary          Summary
	rows             []Row
	scheduleID       string
	filter           Filter
	recordedActor    auth.Principal
	recordedFormat   string
	regencyID        string
	scheduleRegencyErr error
}

func (r *repositoryStub) Summary(_ context.Context, scheduleID string, filter Filter) (Summary, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.summary, nil
}

func (r *repositoryStub) Rows(_ context.Context, scheduleID string, filter Filter) ([]Row, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.rows, nil
}

func (r *repositoryStub) RecordExport(_ context.Context, actor auth.Principal, scheduleID, format string, filter Filter, _ auth.ClientMeta) error {
	r.recordedActor, r.recordedFormat, r.scheduleID, r.filter = actor, format, scheduleID, filter
	return nil
}

func (r *repositoryStub) ScheduleRegency(_ context.Context, scheduleID string) (string, error) {
	r.scheduleID = scheduleID
	if r.scheduleRegencyErr != nil {
		return "", r.scheduleRegencyErr
	}
	return r.regencyID, nil
}
```

Update every existing call site in this file — `service.Summary(context.Background(), ..., Filter{...})` and `service.Rows(...)` and `service.ExportExcel(...)` and `service.ExportPDF(...)` — to append `, auth.RegencyScope{Unrestricted: true}` as the final argument. This affects: `TestSummaryRequiresScheduleAndValidatesFilter` (4 calls), `TestRowsRequiresScheduleAndTrimsInput` (1 call), `TestRowsPassesTrimmedScheduleAndFilterToRepository` (1 call), `TestSummaryPassesTrimmedScheduleAndFilterToRepository` (1 call), `TestExportExcelRecordsAuditEventAfterBuildingFile` (1 call), `TestExportPDFRecordsAuditEventAfterBuildingFile` (1 call).

Append new tests:

```go
func TestSummaryAndRowsRejectScheduleOutsideRegencyScope(t *testing.T) {
	repository := &repositoryStub{regencyID: "regency-1", summary: Summary{TotalAllocations: 3}, rows: []Row{{DistributionNumber: 7}}}
	service := NewService(repository)
	outOfScope := auth.RegencyScope{RegencyIDs: []string{"regency-2"}}
	inScope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}

	if _, err := service.Summary(context.Background(), "schedule-1", Filter{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("Summary err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{}, inScope); err != nil {
		t.Fatalf("Summary in-scope err=%v", err)
	}
	if _, err := service.Rows(context.Background(), "schedule-1", Filter{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("Rows err=%v", err)
	}
	if _, err := service.Rows(context.Background(), "schedule-1", Filter{}, inScope); err != nil {
		t.Fatalf("Rows in-scope err=%v", err)
	}
}

func TestScheduleRegencyLookupErrorPropagates(t *testing.T) {
	repository := &repositoryStub{scheduleRegencyErr: ErrScheduleNotFound}
	service := NewService(repository)
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{}, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestExportExcelAndExportPDFRejectScheduleOutsideRegencyScope(t *testing.T) {
	repository := &repositoryStub{regencyID: "regency-1", summary: Summary{TotalAllocations: 1}, rows: []Row{{DistributionNumber: 7, FullName: "Siti Aminah"}}}
	service := NewService(repository)
	outOfScope := auth.RegencyScope{RegencyIDs: []string{"regency-2"}}

	if _, err := service.ExportExcel(context.Background(), auth.Principal{}, "schedule-1", Filter{}, auth.ClientMeta{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("ExportExcel err=%v", err)
	}
	if _, err := service.ExportPDF(context.Background(), auth.Principal{}, "schedule-1", Filter{}, auth.ClientMeta{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("ExportPDF err=%v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/reports -run 'TestSummaryAndRowsRejectScheduleOutsideRegencyScope|TestScheduleRegencyLookupErrorPropagates|TestExportExcelAndExportPDFRejectScheduleOutsideRegencyScope' -count=1`

Expected: FAIL (compile error — `ErrScheduleNotFound` doesn't exist yet, signatures don't match).

- [ ] **Step 3: Add the sentinel error**

Modify `internal/reports/models.go`:

```go
var (
	ErrScheduleRequired = errors.New("report schedule is required")
	ErrFilterInvalid    = errors.New("report filter is invalid")
	ErrScheduleNotFound = errors.New("report schedule not found")
)
```

- [ ] **Step 4: Update the service**

Modify `internal/reports/service.go`:

```go
type repository interface {
	Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error)
	Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error)
	RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, filter Filter, meta auth.ClientMeta) error
	ScheduleRegency(ctx context.Context, scheduleID string) (string, error)
}
```

```go
func (s *Service) checkScheduleScope(ctx context.Context, scheduleID string, scope auth.RegencyScope) error {
	regencyID, err := s.repository.ScheduleRegency(ctx, scheduleID)
	if err != nil {
		return err
	}
	if !scope.Allows(regencyID) {
		return ErrScheduleNotFound
	}
	return nil
}

func (s *Service) Summary(ctx context.Context, scheduleID string, filter Filter, scope auth.RegencyScope) (Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Summary{}, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return Summary{}, err
	}
	if err := s.checkScheduleScope(ctx, scheduleID, scope); err != nil {
		return Summary{}, err
	}
	return s.repository.Summary(ctx, scheduleID, filter)
}

func (s *Service) Rows(ctx context.Context, scheduleID string, filter Filter, scope auth.RegencyScope) ([]Row, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return nil, err
	}
	if err := s.checkScheduleScope(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	return s.repository.Rows(ctx, scheduleID, filter)
}

func (s *Service) ExportExcel(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	rows, err := s.Rows(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	data, err := buildExcel(rows)
	if err != nil {
		return nil, err
	}
	if err := s.repository.RecordExport(ctx, actor, strings.TrimSpace(scheduleID), "xlsx", filter, meta); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) ExportPDF(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	summary, err := s.Summary(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	rows, err := s.Rows(ctx, scheduleID, filter, scope)
	if err != nil {
		return nil, err
	}
	data, err := buildPDF(summary, rows)
	if err != nil {
		return nil, err
	}
	if err := s.repository.RecordExport(ctx, actor, strings.TrimSpace(scheduleID), "pdf", filter, meta); err != nil {
		return nil, err
	}
	return data, nil
}
```

- [ ] **Step 5: Update the repository**

Modify `internal/reports/repository.go`, add after `NewRepository`:

```go
func (r *Repository) ScheduleRegency(ctx context.Context, scheduleID string) (string, error) {
	var regencyID string
	err := r.pool.QueryRow(ctx, `SELECT regency_id::text FROM program_schedules WHERE id=$1`, scheduleID).Scan(&regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrScheduleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("look up schedule regency: %w", err)
	}
	return regencyID, nil
}
```

Add `"errors"` and `"github.com/jackc/pgx/v5"` to this file's imports (neither is currently imported — `internal/reports/repository.go` only imports `context`, `fmt`, `konkit/internal/audit`, `konkit/internal/auth`, `github.com/jackc/pgx/v5/pgxpool`).

- [ ] **Step 6: Run the unit tests**

Run: `go test ./internal/reports -count=1`

Expected: PASS.

(Wiring `reports.ErrScheduleNotFound` into `internal/api/routes.go`'s not-found mapping is Task 5 Step 5 — `internal/api` is outside this task's package boundary.)

- [ ] **Step 7: Add the integration test**

Modify `internal/reports/repository_integration_test.go`. Add `regencyID` to `reportsFixture`:

```go
type reportsFixture struct {
	scheduleID string
	primaryNIK string
	regencyID  string
}
```

Change the `return` at the end of `createReportsFixture`:

```go
	return reportsFixture{scheduleID: scheduleID, primaryNIK: primaryNIK, regencyID: regencyID}
```

Append a new test after `TestIntegrationRecordExportWritesAuditEvent`:

```go
func TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess(t *testing.T) {
	pool := reportsIntegrationPool(t)
	ctx := context.Background()
	fixture := createReportsFixture(t, pool)
	repository := NewRepository(pool)
	service := NewService(repository)

	inScope := auth.RegencyScope{RegencyIDs: []string{fixture.regencyID}}
	outOfScope := auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}

	if _, err := service.Summary(ctx, fixture.scheduleID, Filter{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("Summary err=%v", err)
	}
	if _, err := service.Summary(ctx, fixture.scheduleID, Filter{}, inScope); err != nil {
		t.Fatalf("Summary in-scope err=%v", err)
	}
	if _, err := service.Rows(ctx, fixture.scheduleID, Filter{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("Rows err=%v", err)
	}
	if _, err := service.Rows(ctx, fixture.scheduleID, Filter{}, inScope); err != nil {
		t.Fatalf("Rows in-scope err=%v", err)
	}
	if _, err := service.ExportExcel(ctx, auth.Principal{}, fixture.scheduleID, Filter{}, auth.ClientMeta{}, outOfScope); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("ExportExcel err=%v", err)
	}
}
```

Add `"konkit/internal/auth"` to this file's import block if not already present (check first — `internal/reports/repository.go` already imports it, but `repository_integration_test.go` may not).

- [ ] **Step 8: Run the integration test**

Run:

```powershell
go test ./internal/reports -run TestIntegrationScopeEnforcementRejectsOutOfRegencyAccess -count=1 -v
go test ./internal/reports -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add internal/reports
git commit -m "feat: enforce regency scope on reports summary, rows, and export"
```

---

### Task 5: API Layer — Wire Regency Scope Into DCP3/Pendistribusian/Laporan Routes

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/dcp3_routes.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/reports_routes.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go`

**Interfaces:**
- Consumes: `Handler.regencyScope(w, r, principal) (auth.RegencyScope, bool)` (already exists from PR1, `internal/api/handler.go`), and every new service-layer signature from Tasks 1-4.

- [ ] **Step 1: Write the failing handler tests**

Append to `internal/api/handler_test.go`, matching the existing `TestRegenciesEndpointAppliesCallerRegencyScope` style:

```go
func TestDCP3PreviewEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	dcp3Service := &fakeDCP3Service{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dcp3/previews/batch-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(dcp3Service.seenRegencyScope.RegencyIDs) != 1 || dcp3Service.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", dcp3Service.seenRegencyScope)
	}
}

func TestDistributionSearchEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	distributionService := &fakeDistributionService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/search?schedule_id=schedule-1&q=Siti", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Distribution: distributionService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(distributionService.seenRegencyScope.RegencyIDs) != 1 || distributionService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", distributionService.seenRegencyScope)
	}
}

func TestReportsSummaryEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	reportsService := &fakeReportsService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Reports: reportsService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(reportsService.seenRegencyScope.RegencyIDs) != 1 || reportsService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", reportsService.seenRegencyScope)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api -run 'TestDCP3PreviewEndpointAppliesCallerRegencyScope|TestDistributionSearchEndpointAppliesCallerRegencyScope|TestReportsSummaryEndpointAppliesCallerRegencyScope' -count=1`

Expected: FAIL (compile errors — interfaces and fakes don't have the new parameter/fields yet).

- [ ] **Step 3: Update the service interfaces**

Modify `internal/api/handler.go`:

```go
type DCP3Service interface {
	Preview(context.Context, auth.Principal, string, string, io.Reader, auth.ClientMeta, auth.RegencyScope) (dcp3.ImportPreview, error)
	GetPreview(context.Context, string, auth.RegencyScope) (dcp3.ImportPreview, error)
	Commit(context.Context, auth.Principal, string, dcp3.Mapping, auth.ClientMeta, auth.RegencyScope) (dcp3.ImportResult, error)
}

type DistributionService interface {
	Search(context.Context, string, string, int, auth.RegencyScope) ([]distribution.SearchResult, error)
	GetWorkspace(context.Context, string, auth.RegencyScope) (distribution.RecipientWorkspace, error)
	SaveDraft(context.Context, auth.Principal, string, distribution.DraftInput, auth.ClientMeta, auth.RegencyScope) (distribution.RecipientWorkspace, error)
	Complete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (distribution.DistributionRecord, error)
	UploadMedia(context.Context, auth.Principal, distribution.UploadMediaInput, auth.ClientMeta, auth.RegencyScope) (distribution.MediaFile, error)
	DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	OpenMedia(context.Context, string, auth.RegencyScope) (distribution.MediaContent, error)
}

type ReportsService interface {
	Summary(context.Context, string, reports.Filter, auth.RegencyScope) (reports.Summary, error)
	Rows(context.Context, string, reports.Filter, auth.RegencyScope) ([]reports.Row, error)
	ExportExcel(context.Context, auth.Principal, string, reports.Filter, auth.ClientMeta, auth.RegencyScope) ([]byte, error)
	ExportPDF(context.Context, auth.Principal, string, reports.Filter, auth.ClientMeta, auth.RegencyScope) ([]byte, error)
}
```

- [ ] **Step 4: Wire the route handlers**

Modify `internal/api/dcp3_routes.go`. In `handleDCP3PreviewCreate`, after the existing `if !h.authorize(...)` block and before the multipart-parsing (or right before the `h.deps.DCP3.Preview(...)` call — insert right before that call):

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.Preview(r.Context(), rc.principal, scheduleID, filepath.Base(header.Filename), file, clientMeta(r), scope)
```

In `handleDCP3Preview`:

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.GetPreview(r.Context(), strings.TrimSpace(id), scope)
```

In `handleDCP3Import`:

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.Commit(r.Context(), rc.principal, strings.TrimSpace(input.BatchID), input.Mapping, clientMeta(r), scope)
```

Modify `internal/api/distribution_routes.go`. In `handleDistributionSearch`:

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.Search(r.Context(), scheduleID, query, limit, scope)
```

In `handleDistributionAllocation`, all three branches gain the same two-line insert before their `h.deps.Distribution.*` call:

```go
	// GET branch:
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.GetWorkspace(r.Context(), parts[0], scope)

	// PATCH .../draft branch:
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SaveDraft(r.Context(), rc.principal, parts[0], input, clientMeta(r), scope)

	// POST .../complete branch:
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.Complete(r.Context(), rc.principal, parts[0], clientMeta(r), scope)
```

In `handleDistributionSlot`, before the `h.deps.Distribution.UploadMedia(...)` call:

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.UploadMedia(r.Context(), rc.principal, input, clientMeta(r), scope)
```

In `handleDistributionMedia`, both branches:

```go
	// GET .../content branch:
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	content, err := h.deps.Distribution.OpenMedia(r.Context(), parts[0], scope)

	// DELETE branch:
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	if err := h.deps.Distribution.DeleteMedia(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
```

Modify `internal/api/reports_routes.go`. Insert right after the `if !h.authorize(...)` block, before `parts := strings.Split(...)`:

```go
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
```

Then update the four calls inside the `switch parts[1]` block to append `, scope`:

```go
	case "summary":
		result, err := h.deps.Reports.Summary(r.Context(), scheduleID, filter, scope)
	case "rows":
		result, err := h.deps.Reports.Rows(r.Context(), scheduleID, filter, scope)
	case "export.xlsx":
		data, err := h.deps.Reports.ExportExcel(r.Context(), rc.principal, scheduleID, filter, clientMeta(r), scope)
	case "export.pdf":
		data, err := h.deps.Reports.ExportPDF(r.Context(), rc.principal, scheduleID, filter, clientMeta(r), scope)
```

- [ ] **Step 5: Wire the not-found mapping**

Modify `internal/api/routes.go` line 448 — add `reports.ErrScheduleNotFound` to the existing not-found case:

```go
	case errors.Is(err, profile.ErrNotFound), errors.Is(err, administration.ErrNotFound), errors.Is(err, programs.ErrNotFound), errors.Is(err, dcp3.ErrPreviewNotFound), errors.Is(err, distribution.ErrAllocationNotFound), errors.Is(err, distribution.ErrMediaNotFound), errors.Is(err, reports.ErrScheduleNotFound):
```

(`internal/api/routes.go` already imports `konkit/internal/reports` — confirm and skip adding the import if so; it's used lower in the same switch for `reports.ErrScheduleRequired`/`reports.ErrFilterInvalid`.)

- [ ] **Step 6: Update the test fakes**

Modify `internal/api/handler_test.go`. Add `seenRegencyScope auth.RegencyScope` to `fakeDCP3Service`, `fakeDistributionService`, `fakeReportsService`:

```go
type fakeDCP3Service struct {
	DCP3Service
	preview          dcp3.ImportPreview
	result           dcp3.ImportResult
	scheduleID       string
	filename         string
	batchID          string
	mapping          dcp3.Mapping
	seenRegencyScope auth.RegencyScope
}
```

```go
type fakeDistributionService struct {
	DistributionService
	results          []distribution.SearchResult
	workspace        distribution.RecipientWorkspace
	scheduleID       string
	query            string
	limit            int
	allocationID     string
	media            distribution.MediaFile
	mediaContent     []byte
	slotID           string
	upload           distribution.UploadMediaInput
	completed        distribution.DistributionRecord
	completeErr      error
	seenRegencyScope auth.RegencyScope
}
```

```go
type fakeReportsService struct {
	ReportsService
	scheduleID       string
	filter           reports.Filter
	summary          reports.Summary
	rows             []reports.Row
	exportData       []byte
	exportFormat     string
	seenRegencyScope auth.RegencyScope
}
```

Update every existing method on these three fakes to capture `scope auth.RegencyScope` into `seenRegencyScope` and accept the new parameter:

```go
func (f *fakeDistributionService) Search(_ context.Context, scheduleID, query string, limit int, scope auth.RegencyScope) ([]distribution.SearchResult, error) {
	f.scheduleID, f.query, f.limit, f.seenRegencyScope = scheduleID, query, limit, scope
	return f.results, nil
}
func (f *fakeDistributionService) GetWorkspace(_ context.Context, allocationID string, scope auth.RegencyScope) (distribution.RecipientWorkspace, error) {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.workspace, nil
}
func (f *fakeDistributionService) SaveDraft(_ context.Context, _ auth.Principal, allocationID string, _ distribution.DraftInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.RecipientWorkspace, error) {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.workspace, nil
}
func (f *fakeDistributionService) Complete(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionRecord, error) {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.completed, f.completeErr
}
func (f *fakeDistributionService) UploadMedia(_ context.Context, _ auth.Principal, input distribution.UploadMediaInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.MediaFile, error) {
	f.slotID, f.upload, f.seenRegencyScope = input.SlotID, input, scope
	return f.media, nil
}
func (f *fakeDistributionService) DeleteMedia(_ context.Context, _ auth.Principal, _ string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.seenRegencyScope = scope
	return nil
}
func (f *fakeDistributionService) OpenMedia(_ context.Context, _ string, scope auth.RegencyScope) (distribution.MediaContent, error) {
	f.seenRegencyScope = scope
	return distribution.MediaContent{Reader: io.NopCloser(bytes.NewReader(f.mediaContent)), MimeType: f.media.MimeType, Filename: f.media.OriginalFilename}, nil
}
```

```go
func (f *fakeDCP3Service) Preview(_ context.Context, _ auth.Principal, scheduleID, filename string, _ io.Reader, _ auth.ClientMeta, scope auth.RegencyScope) (dcp3.ImportPreview, error) {
	f.scheduleID, f.filename, f.seenRegencyScope = scheduleID, filename, scope
	return f.preview, nil
}
func (f *fakeDCP3Service) GetPreview(_ context.Context, _ string, scope auth.RegencyScope) (dcp3.ImportPreview, error) {
	f.seenRegencyScope = scope
	return f.preview, nil
}
func (f *fakeDCP3Service) Commit(_ context.Context, _ auth.Principal, batchID string, mapping dcp3.Mapping, _ auth.ClientMeta, scope auth.RegencyScope) (dcp3.ImportResult, error) {
	f.batchID, f.mapping, f.seenRegencyScope = batchID, mapping, scope
	return f.result, nil
}
```

```go
func (s *fakeReportsService) Summary(_ context.Context, scheduleID string, filter reports.Filter, scope auth.RegencyScope) (reports.Summary, error) {
	s.scheduleID, s.filter, s.seenRegencyScope = scheduleID, filter, scope
	return s.summary, nil
}
func (s *fakeReportsService) Rows(_ context.Context, scheduleID string, filter reports.Filter, scope auth.RegencyScope) ([]reports.Row, error) {
	s.scheduleID, s.filter, s.seenRegencyScope = scheduleID, filter, scope
	return s.rows, nil
}
func (s *fakeReportsService) ExportExcel(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat, s.seenRegencyScope = scheduleID, filter, "xlsx", scope
	return s.exportData, nil
}
func (s *fakeReportsService) ExportPDF(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat, s.seenRegencyScope = scheduleID, filter, "pdf", scope
	return s.exportData, nil
}
```

- [ ] **Step 7: Run the API tests**

Run: `go test ./internal/api -count=1`

Expected: PASS. This also re-verifies every pre-existing DCP3/distribution/reports handler test (upload, search, workspace, draft, completion, export) still passes with the zero-value `auth.RegencyScope{}` the `fakeAuthService` returns by default when a test doesn't set `regencyScope` explicitly — since none of those existing tests assert on `seenRegencyScope`, this is safe, but double check: a zero-value scope is `{Unrestricted: false, RegencyIDs: nil}`, which the real services would treat as "no access" — the *fakes* don't enforce scope themselves (they're fakes, not the real repository SQL), so this has no effect on those tests' outcomes.

- [ ] **Step 8: Run the full backend build and unit suite**

Run: `go build ./... && go test ./... -count=1` (integration tests skip without `TEST_DATABASE_URL` — full run happens in Task 6).

- [ ] **Step 9: Commit**

```powershell
git add internal/api
git commit -m "feat: apply regency scope to DCP3, distribution, and reports endpoints"
```

---

### Task 6: Full Verification and README Note

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes the complete Tasks 1-5 vertical slice.

- [ ] **Step 1: Document the change**

Modify `README.md` — the sentence added in PR1 currently reads:

```
Role kini dapat dibatasi ke satu atau beberapa kabupaten, atau ditandai memiliki akses tanpa batas (mis. untuk peran pengawasan lintas kabupaten); Super Admin selalu tanpa batas. Pembatasan ini sudah diterapkan pada listing kabupaten dan jadwal di Persiapan Program — penerapan pada DCP3, Pendistribusian, dan Laporan menyusul pada perubahan terpisah.
```

Replace the second sentence (only) with:

```
Pembatasan ini diterapkan pada listing kabupaten dan jadwal di Persiapan Program, serta pada seluruh endpoint DCP3, Pendistribusian, dan Laporan yang menerima `schedule_id`/`allocation_id`/`slot_id`/`media_id` — percobaan mengakses data di luar cakupan kabupaten dikembalikan sebagai galat "tidak ditemukan", konsisten dengan resource yang benar-benar tidak ada.
```

- [ ] **Step 2: Run the full suite**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test -p 1 ./... -count=1
go vet ./...
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: all commands exit 0. No migration status check needed — this plan adds no migration. No frontend build artifact changes are expected (no `.tsx`/`.ts` files touched by this plan) — if `npm run build` still regenerates `web/static/app` bytes due to a toolchain hash, include that directory in the commit; otherwise `README.md` alone is the diff.

- [ ] **Step 3: Commit**

```powershell
git add README.md
git commit -m "docs: document DCP3, distribution, and reports regency scope enforcement"
```
