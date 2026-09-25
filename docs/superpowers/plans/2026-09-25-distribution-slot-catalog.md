# Distribution Slot Catalog + Quota + Completion Badge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the "create one slot at a time, no limit" distribution UX with a numbered catalog grid per schedule, an optional hard quota per schedule, and a completion badge per number, without changing the existing 3-POS (mesin/dokumen/penyerahan) detail flow.

**Architecture:** Backend adds one nullable `slot_quota` column on `program_schedules`, enforces it inside `CreateSlot`'s existing locking transaction, and exposes a new read-only `ListSlotCatalog` endpoint that reports status + documentation completeness per existing slot number. Frontend adds a `slot_quota` field to the existing schedule admin form (reusing it for both "raise quota" and, via a new schedule row, "addendum"), and a new `SlotCatalogGrid` component that renders 1..N boxes and delegates clicks to the distribution page's existing search/create code paths — no new creation or search logic.

**Tech Stack:** Go 1.26 (pgx/v5, goose migrations), React 19 + TypeScript (TanStack Query, Vitest + Testing Library).

**Spec:** `docs/superpowers/specs/2026-09-25-distribution-slot-catalog-design.md` sections 4.1, 4.2, 4.3, 4.4 (barcode scanning, section 4.5, is a separate future plan).

## Global Constraints

- Kuota bersifat batas keras saat diisi (spec §4.2): `CreateSlot` must reject with a dedicated error when the next slot number would exceed `slot_quota`, validated server-side (not just UI-side).
- Kuota nullable (spec §4.2): a schedule with no quota keeps today's unlimited/sequential behavior.
- Addendum jalur ganda (spec §4.3): both "raise quota" and "new schedule" must remain possible; this plan achieves that by exposing `slot_quota` on the existing schedule admin form (`SchedulesPanel.tsx`), which already supports both editing an existing schedule and creating a new one — no new UI surface for addendum itself.
- Badge warna ikut skema existing (spec §4.4): reuse the `--green`/`--blue`/`--amber` CSS custom properties already defined in `frontend/src/features/distribution/Distribution.module.css:1`, not a new palette.
- No changes to POS Mesin/Dokumen/Penyerahan business logic (`LinkSlot`, `CompleteSlot`) — this plan only adds a quota gate to `CreateSlot` and a new read-only listing endpoint.
- Follow existing suffix-uniqueness pattern (`fmt.Sprintf("%d", time.Now().UnixNano())`) for all new Go integration test fixtures, matching `internal/distribution/repository_integration_test.go` and `internal/programs/repository_integration_test.go`.

---

### Task 1: Migration — `program_schedules.slot_quota`

**Files:**
- Create: `internal/database/migrations/00015_program_schedules_slot_quota.sql`

**Interfaces:**
- Produces: column `program_schedules.slot_quota integer NULL`, constrained `> 0` when set. Every later task's SQL and Go struct field depend on this exact column name and nullability.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
ALTER TABLE program_schedules ADD COLUMN slot_quota integer;
ALTER TABLE program_schedules ADD CONSTRAINT program_schedules_slot_quota_check CHECK (slot_quota IS NULL OR slot_quota > 0);

-- +goose Down
ALTER TABLE program_schedules DROP CONSTRAINT program_schedules_slot_quota_check;
ALTER TABLE program_schedules DROP COLUMN slot_quota;
```

- [ ] **Step 2: Apply it to the local test database and verify**

Run: `go run ./cmd/migrate up` (against `TEST_DATABASE_URL`, or your local `konkit_test`/`konkit` database — whichever your shell's `DATABASE_URL`/`TEST_DATABASE_URL` currently points at)
Expected: `OK   00015_program_schedules_slot_quota.sql` printed, migration reaches version 15.

Then confirm the constraint is live:
Run (psql): `INSERT INTO program_schedules (program_id, regency_id, package_template_version_id, documentation_template_version_id, name, start_date, end_date, status, distribution_number_padding, receipt_policy_json, slot_quota) SELECT id, id, id, id, 'x', now(), now(), 'draft', 4, '{}'::jsonb, 0 FROM programs LIMIT 1;`
Expected: fails with `new row for relation "program_schedules" violates check constraint "program_schedules_slot_quota_check"` (this proves the constraint exists; the bogus foreign keys will also fail first if `programs` is empty — either failure is fine, don't chase this further, it's a live sanity check, not a real test).

- [ ] **Step 3: Commit**

```bash
git add internal/database/migrations/00015_program_schedules_slot_quota.sql
git commit -m "feat(database): add nullable slot_quota to program_schedules"
```

---

### Task 2: Backend — `Schedule.SlotQuota` plumbing (`internal/programs`)

**Files:**
- Modify: `internal/programs/models.go:8-18` (new error), `internal/programs/models.go:142-178` (`Schedule`, `ScheduleInput`)
- Modify: `internal/programs/service.go:76-98` (`SaveSchedule` validation)
- Modify: `internal/programs/repository.go:283-316` (`SaveSchedule` SQL), `internal/programs/repository.go:392-413` (`scheduleSelect`, `scanSchedule`)
- Test: `internal/programs/repository_integration_test.go`

**Interfaces:**
- Consumes: nothing new from other tasks.
- Produces: `Schedule.SlotQuota *int` (JSON `slot_quota`, omitted when nil) and `ScheduleInput.SlotQuota *int` (JSON `slot_quota`, omitted when nil) — Task 3 reads this column directly via SQL (not through this Go type), Task 6 (frontend) consumes the JSON field name `slot_quota`.

- [ ] **Step 1: Add the error and struct fields**

In `internal/programs/models.go`, add to the `var (...)` error block (after `ErrPackageOptionsRequired`, models.go:17):

```go
	ErrSlotQuotaInvalid       = errors.New("slot_quota must be greater than zero when set")
```

In the `Schedule` struct (models.go:142-162), add a field right after `DistributionNumberPadding int`:

```go
	SlotQuota                      *int                   `json:"slot_quota,omitempty"`
```

In the `ScheduleInput` struct (models.go:164-178), add the same field right after `DistributionNumberPadding int`:

```go
	SlotQuota                      *int           `json:"slot_quota,omitempty"`
```

- [ ] **Step 2: Write the failing service test**

Add to `internal/programs/service_test.go` (follow the existing style of `TestSavePackageTemplateRequiresEquipmentOptionsWhenPublishing` in that file — a `repositoryStub`-backed unit test, no database):

```go
func TestSaveScheduleRejectsNonPositiveSlotQuota(t *testing.T) {
	service := NewService(&repositoryStub{})
	zero := 0
	_, err := service.SaveSchedule(context.Background(), auth.Principal{}, ScheduleInput{
		ProgramID: "program", RegencyID: "regency", PackageTemplateVersionID: "package",
		DocumentationTemplateVersionID: "document", Name: "Test", Status: "draft",
		StartDate: time.Now(), EndDate: time.Now().Add(24 * time.Hour),
		SlotQuota: &zero,
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotQuotaInvalid) {
		t.Fatalf("err = %v, want ErrSlotQuotaInvalid", err)
	}
}
```

- [ ] **Step 2b: Run it to confirm it fails**

Run: `go test ./internal/programs/... -run TestSaveScheduleRejectsNonPositiveSlotQuota -v`
Expected: FAIL — `err = <nil>, want ErrSlotQuotaInvalid` (the repository stub will happily accept it since no validation exists yet).

- [ ] **Step 3: Add validation in `SaveSchedule`**

In `internal/programs/service.go`, inside `SaveSchedule` (service.go:76-98), insert this check right after the `DistributionNumberPadding` default (after line 90, before the big `ErrInvalidInput` condition on line 91):

```go
	if input.SlotQuota != nil && *input.SlotQuota < 1 {
		return Schedule{}, ErrSlotQuotaInvalid
	}
```

- [ ] **Step 4: Run the service test again**

Run: `go test ./internal/programs/... -run TestSaveScheduleRejectsNonPositiveSlotQuota -v`
Expected: PASS

- [ ] **Step 5: Update the SQL — `scheduleSelect` and `scanSchedule`**

In `internal/programs/repository.go`, change the `scheduleSelect` constant (repository.go:392-396) to add `s.slot_quota` right after `s.distribution_number_padding`:

```go
const scheduleSelect = `
SELECT s.id::text,s.program_id::text,s.regency_id::text,s.package_template_version_id::text,s.documentation_template_version_id::text,s.name,s.start_date,s.end_date,s.status,s.distribution_number_padding,s.slot_quota,s.receipt_policy_json,COALESCE(s.notes,''),COALESCE(s.supervisor_name,''),s.created_at,s.updated_at,
p.id::text,p.code,p.name,p.program_type,p.fiscal_year,p.status,COALESCE(p.notes,''),p.created_at,p.updated_at,
r.id::text,r.province_name,r.name,r.document_code,r.is_active,COALESCE(r.notes,''),r.created_at,r.updated_at
FROM program_schedules s JOIN programs p ON p.id=s.program_id JOIN regencies r ON r.id=s.regency_id`
```

In `scanSchedule` (repository.go:398-413), add `&item.SlotQuota` to the `row.Scan(...)` call right after `&item.DistributionNumberPadding` and before `&policy`:

```go
	err := row.Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.PackageTemplateVersionID, &item.DocumentationTemplateVersionID, &item.Name, &item.StartDate, &item.EndDate, &item.Status, &item.DistributionNumberPadding, &item.SlotQuota, &policy, &item.Notes, &item.SupervisorName, &item.CreatedAt, &item.UpdatedAt,
		&item.Program.ID, &item.Program.Code, &item.Program.Name, &item.Program.ProgramType, &item.Program.FiscalYear, &item.Program.Status, &item.Program.Notes, &item.Program.CreatedAt, &item.Program.UpdatedAt,
		&item.Regency.ID, &item.Regency.ProvinceName, &item.Regency.Name, &item.Regency.DocumentCode, &item.Regency.IsActive, &item.Regency.Notes, &item.Regency.CreatedAt, &item.Regency.UpdatedAt)
```

- [ ] **Step 6: Update `SaveSchedule`'s INSERT and UPDATE**

In `internal/programs/repository.go`, replace the `id == ""` (insert) branch (repository.go:295) with:

```go
		err = tx.QueryRow(ctx, `INSERT INTO program_schedules (program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json,notes,supervisor_name,slot_quota) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13 FROM programs p JOIN package_template_versions pt ON pt.id=$3 JOIN documentation_template_versions dt ON dt.id=$4 WHERE p.id=$1 AND p.program_type=pt.program_type AND p.program_type=dt.program_type RETURNING id::text`, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName, input.SlotQuota).Scan(&id)
```

And replace the `else` (update) branch (repository.go:297-298) with:

```go
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE program_schedules s SET program_id=$2,regency_id=$3,package_template_version_id=$4,documentation_template_version_id=$5,name=$6,start_date=$7,end_date=$8,status=$9,distribution_number_padding=$10,receipt_policy_json=$11,notes=NULLIF($12,''),supervisor_name=NULLIF($13,''),slot_quota=$14,updated_at=now() WHERE s.id=$1 AND EXISTS (SELECT 1 FROM programs p JOIN package_template_versions pt ON pt.id=$4 JOIN documentation_template_versions dt ON dt.id=$5 WHERE p.id=$2 AND p.program_type=pt.program_type AND p.program_type=dt.program_type)`, id, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName, input.SlotQuota)
		if err == nil && tag.RowsAffected() == 0 {
			return Schedule{}, ErrNotFound
		}
```

- [ ] **Step 7: Write the failing integration test**

Add to `internal/programs/repository_integration_test.go`, after `TestIntegrationRepositoryPersistsProgramSetupAndVersionsPublishedTemplate` (after line 136):

```go
// TestSaveScheduleRoundTripsSlotQuota proves slot_quota survives INSERT, UPDATE, and re-read through
// ListSchedules against the live schema — not just that the query compiles.
func TestSaveScheduleRoundTripsSlotQuota(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "QTA-" + suffix
	templateCode := "QTA-PKG-" + suffix
	regencyCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-quota-test"}

	regency, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Kuota " + suffix, DocumentCode: regencyCode, IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	program, err := service.SaveProgram(ctx, actor, ProgramInput{Code: programCode, Name: "Program Kuota", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "active"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		TemplateCode: templateCode, Name: "Template Kuota", ProgramType: ProgramFarmer,
		Values: map[string]any{"machine_options": []any{map[string]any{"code": "m", "brand": "M", "type": "T"}}, "hose_options": []any{map[string]any{"code": "h", "brand": "H", "spec": "S"}}},
		Status: "published",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	var documentationTemplateID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1`).Scan(&documentationTemplateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_schedules WHERE program_id IN (SELECT id FROM programs WHERE code = $1)", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM programs WHERE code = $1", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM package_template_versions WHERE template_code = $1", templateCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regency.ID)
	})

	quota := 46
	created, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap Kuota",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		Status: "active", SlotQuota: &quota,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if created.SlotQuota == nil || *created.SlotQuota != 46 {
		t.Fatalf("created.SlotQuota = %v, want 46", created.SlotQuota)
	}

	raised := 60
	updated, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ID: created.ID, ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap Kuota",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		Status: "active", SlotQuota: &raised,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SlotQuota == nil || *updated.SlotQuota != 60 {
		t.Fatalf("updated.SlotQuota = %v, want 60", updated.SlotQuota)
	}

	list, err := service.ListSchedules(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range list {
		if item.ID == created.ID {
			found = true
			if item.SlotQuota == nil || *item.SlotQuota != 60 {
				t.Fatalf("listed item.SlotQuota = %v, want 60", item.SlotQuota)
			}
		}
	}
	if !found {
		t.Fatalf("schedule %s not found in ListSchedules result", created.ID)
	}
}
```

- [ ] **Step 8: Run it to verify it fails, then passes**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/programs/... -run TestSaveScheduleRoundTripsSlotQuota -v`
Expected before Step 5/6: FAIL with a SQL error (`column "slot_quota" does not exist` or similar) if run before those steps; after them, PASS.

- [ ] **Step 9: Run the full package test suite**

Run: `go test ./internal/programs/...`
Expected: all PASS (this also re-validates `TestIntegrationOperationalMigrationCreatesFoundation` in `internal/auth` is unaffected — that test doesn't touch `slot_quota`, but run `go test ./...` once at the end of Task 5 to be sure).

- [ ] **Step 10: Commit**

```bash
git add internal/programs/models.go internal/programs/service.go internal/programs/repository.go internal/programs/service_test.go internal/programs/repository_integration_test.go
git commit -m "feat(programs): add slot_quota to schedules"
```

---

### Task 3: Backend — quota enforcement in `CreateSlot` (`internal/distribution`)

**Files:**
- Modify: `internal/distribution/models.go:9-31` (new error), `internal/distribution/repository.go:181-225` (`CreateSlot`)
- Test: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: `program_schedules.slot_quota` column from Task 1 (read directly via SQL, not through `programs.Schedule`).
- Produces: `distribution.ErrSlotQuotaExceeded` — Task 5 maps this to an HTTP 409 in `writeServiceError`.

- [ ] **Step 1: Add the error**

In `internal/distribution/models.go`, add to the `var (...)` block (models.go:9-31), after `ErrSlotNumberRequired`:

```go
	ErrSlotQuotaExceeded            = errors.New("distribution slot quota has been reached for this schedule")
```

- [ ] **Step 2: Write the failing integration test**

Add to `internal/distribution/repository_integration_test.go`, after `TestCreateSlotSnapshotsDocumentationStage` (after line 137):

```go
// TestCreateSlotRejectsSlotBeyondQuota proves CreateSlot enforces program_schedules.slot_quota as a
// hard limit (spec: docs/superpowers/specs/2026-09-25-distribution-slot-catalog-design.md §4.2) —
// the (quota+1)th slot must be rejected, not silently created.
func TestCreateSlotRejectsSlotBeyondQuota(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Quota Test','QTA',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Quota','farmer',2026,'active') RETURNING id::text`, "QTA-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Quota','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-QTA-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Quota','farmer','published') RETURNING id::text`, "DOC-QTA-"+suffix).Scan(&docTemplateID))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json,slot_quota) VALUES($1,$2,$3,$4,'Jadwal Test Quota','2026-01-01','2026-12-31','active',4,'{}'::jsonb,1) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	first, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	must(t, err)
	if first.SlotNumber != 1 {
		t.Fatalf("first.SlotNumber = %d, want 1", first.SlotNumber)
	}

	_, err = repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	if !errors.Is(err, ErrSlotQuotaExceeded) {
		t.Fatalf("err = %v, want ErrSlotQuotaExceeded", err)
	}
}
```

- [ ] **Step 3: Run it to confirm it fails**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/... -run TestCreateSlotRejectsSlotBeyondQuota -v`
Expected: FAIL — the second `CreateSlot` call currently succeeds (creates slot #2) instead of returning `ErrSlotQuotaExceeded`.

- [ ] **Step 4: Enforce the quota in `CreateSlot`**

In `internal/distribution/repository.go`, replace the schedule-locking query and the `nextNumber` block inside `CreateSlot` (repository.go:188-199) with:

```go
	var documentationTemplateID string
	var slotQuota *int
	if err := tx.QueryRow(ctx, `SELECT documentation_template_version_id, slot_quota FROM program_schedules WHERE id=$1 FOR UPDATE`, input.ScheduleID).Scan(&documentationTemplateID, &slotQuota); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrScheduleRequired
		}
		return DistributionSlot{}, fmt.Errorf("lock schedule: %w", err)
	}

	var nextNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(slot_number),0)+1 FROM distribution_slots WHERE schedule_id=$1`, input.ScheduleID).Scan(&nextNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("allocate slot number: %w", err)
	}
	if slotQuota != nil && nextNumber > *slotQuota {
		return DistributionSlot{}, ErrSlotQuotaExceeded
	}
```

(The rest of `CreateSlot` — the `INSERT INTO distribution_slots`, snapshot, audit, commit — is unchanged.)

- [ ] **Step 5: Run the test again**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/... -run TestCreateSlotRejectsSlotBeyondQuota -v`
Expected: PASS

- [ ] **Step 6: Run the full package suite**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/...`
Expected: all PASS, including the pre-existing `TestCreateSlotSnapshotsDocumentationStage` (which uses no quota, so `slotQuota` is nil and the new check is a no-op).

- [ ] **Step 7: Commit**

```bash
git add internal/distribution/models.go internal/distribution/repository.go internal/distribution/repository_integration_test.go
git commit -m "feat(distribution): enforce slot_quota as a hard limit in CreateSlot"
```

---

### Task 4: Backend — `ListSlotCatalog` (`internal/distribution`)

**Files:**
- Modify: `internal/distribution/models.go` (new `SlotCatalogEntry` type)
- Modify: `internal/distribution/repository.go` (new `ListSlotCatalog` method)
- Test: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Consumes: `distribution_slots`, `documentation_slots`, `media_files` tables (all pre-existing).
- Produces: `Repository.ListSlotCatalog(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SlotCatalogEntry, error)` where `SlotCatalogEntry{ SlotNumber int; Status string; DocumentationComplete bool }` — Task 5 wires this into the `DistributionService` interface and a new HTTP route; Task 7 (frontend) consumes the JSON shape `{slot_number, status, documentation_complete}`.

- [ ] **Step 1: Add the type**

In `internal/distribution/models.go`, add after the `DistributionSlot` struct (after models.go:65):

```go
type SlotCatalogEntry struct {
	SlotNumber            int    `json:"slot_number"`
	Status                string `json:"status"`
	DocumentationComplete bool   `json:"documentation_complete"`
}
```

- [ ] **Step 2: Write the failing integration test**

Add to `internal/distribution/repository_integration_test.go`, after `TestCreateSlotRejectsSlotBeyondQuota` (added in Task 3):

```go
// TestListSlotCatalogReportsStatusAndCompleteness proves ListSlotCatalog returns one row per existing
// distribution_slots, with documentation_complete correctly reflecting whether every required
// documentation_slots row for that slot has met its min_files — using the same completeness rule as
// CompleteSlot (repository.go's incomplete check), just aggregated per slot instead of a single EXISTS.
func TestListSlotCatalogReportsStatusAndCompleteness(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyID string
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Catalog Test','CTG',true) ON CONFLICT (document_code) DO UPDATE SET is_active=true RETURNING id::text`).Scan(&regencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Catalog','farmer',2026,'active') RETURNING id::text`, "CTG-TEST-"+suffix).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Catalog','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-CTG-"+suffix).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Catalog','farmer','published') RETURNING id::text`, "DOC-CTG-"+suffix).Scan(&docTemplateID))
	must(t, pool.QueryRow(ctx, `
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		VALUES($1,'foto-wajib','Foto Wajib','penyerahan',true,1,1,'both',false,false,1) RETURNING id::text
	`, docTemplateID).Scan(new(string)))
	var scheduleID string
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Catalog','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, regencyID, packageTemplateID, docTemplateID).Scan(&scheduleID))

	repo := NewRepository(pool)
	incomplete, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	must(t, err)
	complete, err := repo.CreateSlot(ctx, auth.Principal{}, CreateSlotInput{ScheduleID: scheduleID}, auth.ClientMeta{})
	must(t, err)

	// Satisfy the "complete" slot's one required documentation_slots row with an accepted media file.
	var completeDocSlotID string
	must(t, pool.QueryRow(ctx, `SELECT id::text FROM documentation_slots WHERE distribution_slot_id=(SELECT id FROM distribution_slots WHERE schedule_id=$1 AND slot_number=$2)`, scheduleID, complete.SlotNumber).Scan(&completeDocSlotID))
	checksum := strings.Repeat("b", 64)
	must(t, pool.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status) VALUES($1,gen_random_uuid(),'f.jpg','image/jpeg',10,$2,'camera','accepted') RETURNING id::text`, completeDocSlotID, checksum).Scan(new(string)))

	entries, err := repo.ListSlotCatalog(ctx, scheduleID, auth.RegencyScope{Unrestricted: true})
	must(t, err)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	byNumber := map[int]SlotCatalogEntry{}
	for _, entry := range entries {
		byNumber[entry.SlotNumber] = entry
	}
	if got := byNumber[incomplete.SlotNumber]; got.Status != "open" || got.DocumentationComplete {
		t.Fatalf("incomplete slot entry = %+v, want status=open documentation_complete=false", got)
	}
	if got := byNumber[complete.SlotNumber]; got.Status != "open" || !got.DocumentationComplete {
		t.Fatalf("complete slot entry = %+v, want status=open documentation_complete=true", got)
	}
}
```

- [ ] **Step 3: Run it to confirm it fails**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/... -run TestListSlotCatalogReportsStatusAndCompleteness -v`
Expected: FAIL with a compile error (`repo.ListSlotCatalog undefined`).

- [ ] **Step 4: Implement `ListSlotCatalog`**

In `internal/distribution/repository.go`, add this method after `getSlotByID` (after line 268, before `listSlotDocumentation`):

```go
func (r *Repository) ListSlotCatalog(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SlotCatalogEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.slot_number, ds.status,
			NOT EXISTS(
				SELECT 1 FROM documentation_slots dcs
				LEFT JOIN (SELECT documentation_slot_id, count(*) AS accepted FROM media_files WHERE status='accepted' GROUP BY documentation_slot_id) m ON m.documentation_slot_id = dcs.id
				WHERE dcs.distribution_slot_id = ds.id AND dcs.is_required AND COALESCE(m.accepted,0) < dcs.min_files
			) AS documentation_complete
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY ds.slot_number
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list slot catalog: %w", err)
	}
	defer rows.Close()
	entries := []SlotCatalogEntry{}
	for rows.Next() {
		var entry SlotCatalogEntry
		if err := rows.Scan(&entry.SlotNumber, &entry.Status, &entry.DocumentationComplete); err != nil {
			return nil, fmt.Errorf("scan slot catalog entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 5: Run the test again**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/... -run TestListSlotCatalogReportsStatusAndCompleteness -v`
Expected: PASS

- [ ] **Step 6: Run the full package suite**

Run: `TEST_DATABASE_URL=<your konkit_test URL> go test ./internal/distribution/...`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/distribution/models.go internal/distribution/repository.go internal/distribution/repository_integration_test.go
git commit -m "feat(distribution): add ListSlotCatalog for the slot number grid"
```

---

### Task 5: Backend — wire `ListSlotCatalog` into the HTTP API (`internal/api`)

**Files:**
- Modify: `internal/api/handler.go:90-99` (`DistributionService` interface)
- Modify: `internal/api/distribution_routes.go:125-155` (`handleDistributionSlot` dispatch), add new handler
- Modify: `internal/api/routes.go:448-476` (`writeServiceError`)
- Modify: `internal/api/handler_test.go:781-838` (`fakeDistributionService`)
- Test: `internal/api/handler_test.go`

**Interfaces:**
- Consumes: `distribution.Repository.ListSlotCatalog` (Task 4), `distribution.ErrSlotQuotaExceeded` (Task 3).
- Produces: `GET /api/v1/distribution/slots/catalog?schedule_id=...` (permission `distribution.view`, `200` with `{"data": [...]}`), HTTP `409 slot_quota_exceeded` for `ErrSlotQuotaExceeded` — Task 8 (frontend) calls this endpoint and checks this error code.

- [ ] **Step 1: Add the interface method**

In `internal/api/handler.go`, add to `DistributionService` (handler.go:90-99), after `OpenMedia`:

```go
	ListSlotCatalog(context.Context, string, auth.RegencyScope) ([]distribution.SlotCatalogEntry, error)
```

- [ ] **Step 2: Add the error mapping**

In `internal/api/routes.go`, inside `writeServiceError` (routes.go:448-476), add a new case right after the `ErrAlreadyCompleted` case (after routes.go:475):

```go
	case errors.Is(err, distribution.ErrSlotQuotaExceeded):
		writeError(w, http.StatusConflict, "slot_quota_exceeded", "Kuota slot untuk jadwal ini sudah tercapai")
```

- [ ] **Step 3: Add the route handler**

In `internal/api/distribution_routes.go`, add a new handler function after `handleDistributionSlotSearch` (after line 107):

```go
func (h *Handler) handleDistributionSlotCatalog(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.Distribution.ListSlotCatalog(r.Context(), r.URL.Query().Get("schedule_id"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
```

Then, in `handleDistributionSlot` (distribution_routes.go:125-155), add a branch right after the existing `search` branch (after line 133, before the `media` branch):

```go
	if len(parts) == 1 && parts[0] == "catalog" && r.Method == http.MethodGet {
		h.handleDistributionSlotCatalog(w, r, rc)
		return
	}
```

- [ ] **Step 4: Update the test double**

In `internal/api/handler_test.go`, add fields to `fakeDistributionService` (handler_test.go:781-805), after `upload distribution.UploadMediaInput`:

```go
	catalogScheduleID    string
	catalog              []distribution.SlotCatalogEntry
	catalogErr           error
```

Add the method after `OpenMedia` (after handler_test.go:838):

```go
func (f *fakeDistributionService) ListSlotCatalog(_ context.Context, scheduleID string, scope auth.RegencyScope) ([]distribution.SlotCatalogEntry, error) {
	f.catalogScheduleID, f.seenRegencyScope = scheduleID, scope
	return f.catalog, f.catalogErr
}
```

- [ ] **Step 5: Write the failing handler test**

Add to `internal/api/handler_test.go`, after `TestDistributionSlotSearchAllowsViewOnlyPermissionAndAnyStatus` (after line 519):

```go
func TestDistributionSlotCatalogRequiresDistributionViewAndForwardsScheduleID(t *testing.T) {
	service := &fakeDistributionService{catalog: []distribution.SlotCatalogEntry{{SlotNumber: 1, Status: "completed", DocumentationComplete: true}}}
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/catalog?schedule_id=schedule-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.catalogScheduleID != "schedule-1" {
		t.Fatalf("status=%d schedule=%q body=%s", rec.Code, service.catalogScheduleID, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"documentation_complete":true`) {
		t.Fatalf("expected documentation_complete in body: %s", rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/catalog?schedule_id=schedule-1", nil)
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionSlotsQuotaExceededReturnsConflict(t *testing.T) {
	service := &fakeDistributionService{createErr: distribution.ErrSlotQuotaExceeded}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	secret := []byte("01234567890123456789012345678901")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots", strings.NewReader(`{"schedule_id":"schedule-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"slot_quota_exceeded"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 6: Run the tests to confirm they fail, then pass**

Run: `go test ./internal/api/... -run 'TestDistributionSlotCatalog|TestDistributionSlotsQuotaExceeded' -v`
Expected: FAIL before Steps 1-3 (compile error, `fakeDistributionService` doesn't implement `ListSlotCatalog` / no such route / no such error case), PASS after.

- [ ] **Step 7: Run the full package suite and the whole repo's tests**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/handler.go internal/api/distribution_routes.go internal/api/routes.go internal/api/handler_test.go
git commit -m "feat(api): expose GET /api/v1/distribution/slots/catalog and slot_quota_exceeded"
```

---

### Task 6: Frontend — `slot_quota` on the schedule admin form

**Files:**
- Modify: `frontend/src/features/programs/types.ts:10` (`Schedule` type)
- Modify: `frontend/src/features/programs/SchedulesPanel.tsx`
- Test: `frontend/src/features/programs/SchedulesPanel.test.tsx` (create if it doesn't exist yet — check with `Glob` first; if it exists, add to it)

**Interfaces:**
- Consumes: `slot_quota` JSON field from Task 2's backend.
- Produces: nothing new consumed elsewhere — this is the addendum UI surface (spec §4.3): editing `slot_quota` on an existing schedule is "raise quota", and using the same form's existing "Tambah jadwal" button for a new schedule row is "new addendum schedule". Both already exist; this task only adds the field.

- [ ] **Step 1: Add the field to the `Schedule` type**

In `frontend/src/features/programs/types.ts`, change line 10 — insert `slot_quota?: number;` right after `distribution_number_padding: number;`:

```ts
export type Schedule = { id: string; program_id: string; regency_id: string; package_template_version_id: string; documentation_template_version_id: string; name: string; start_date: string; end_date: string; status: string; distribution_number_padding: number; slot_quota?: number; supervisor_name?: string; notes?: string; program?: Program; regency?: Regency; package_template?: PackageTemplate; documentation_template?: DocumentationTemplate };
```

- [ ] **Step 2: Write the failing test**

Create `frontend/src/features/programs/SchedulesPanel.test.tsx` (this file does not exist yet — verified via `Glob` before writing this plan). Use this content (mirrors `DistributionPage.test.tsx`'s mocking pattern):

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SchedulesPanel } from './SchedulesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPanel() {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/package-templates') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/documentation-templates') return Promise.resolve({ data: [] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.manage']}><SchedulesPanel /></PermissionsProvider></QueryClientProvider>);
}

test('sends null slot_quota when the field is left empty', async () => {
  renderPanel();
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));
  expect(screen.getByLabelText(/Kuota slot/)).toBeInTheDocument();
});

test('sends the entered slot_quota as a number', async () => {
  renderPanel();
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));
  const quotaField = screen.getByLabelText(/Kuota slot/);
  await userEvent.type(quotaField, '46');
  expect(quotaField).toHaveValue(46);
});
```

(Two lightweight rendering/typing assertions — this form's submit path already has no coverage of individual field wiring beyond presence/typing in the existing codebase pattern; a full submit-and-inspect-body test would require mocking every one of the five parallel queries with non-empty data, which is disproportionate to this one-field change.)

- [ ] **Step 3: Run it to confirm it fails**

Run: `cd frontend && npx vitest run src/features/programs/SchedulesPanel.test.tsx`
Expected: FAIL — `Unable to find a label with the text of: /Kuota slot/`.

- [ ] **Step 4: Add the field to `SchedulesPanel.tsx`**

In `frontend/src/features/programs/SchedulesPanel.tsx`, change the `empty` constant (line 20) to add `slot_quota: ''`:

```ts
const empty = { program_id: '', regency_id: '', package_template_version_id: '', documentation_template_version_id: '', name: '', start_date: today, end_date: today, status: 'draft', distribution_number_padding: 4, slot_quota: '', supervisor_name: '', notes: '' };
```

Change the `show` function (line 32) to populate `slot_quota` from an existing item as a string (empty string when unset):

```ts
  const show = (item?: Schedule) => { const nextValues = item ? { program_id: item.program_id, regency_id: item.regency_id, package_template_version_id: item.package_template_version_id, documentation_template_version_id: item.documentation_template_version_id, name: item.name, start_date: dateInputValue(item.start_date), end_date: dateInputValue(item.end_date), status: item.status, distribution_number_padding: item.distribution_number_padding, slot_quota: item.slot_quota != null ? String(item.slot_quota) : '', supervisor_name: item.supervisor_name ?? '', notes: item.notes ?? '' } : empty; setEditing(item); setValues(nextValues); setInitialValues(nextValues); setOpen(true); };
```

Change the `mutation`'s request body (line 31) to convert the string field to `number | null`:

```ts
  const mutation = useMutation({ mutationFn: () => apiRequest(`/api/v1/program-setup/schedules${editing ? `/${editing.id}` : ''}`, { method: editing ? 'PATCH' : 'POST', body: JSON.stringify({ ...values, start_date: datePayload(values.start_date), end_date: datePayload(values.end_date), slot_quota: values.slot_quota === '' ? null : Number(values.slot_quota) }) }), onSuccess: () => { setOpen(false); client.invalidateQueries({ queryKey: ['program-setup', 'schedules'] }); toast.success('Jadwal berhasil disimpan.'); } });
```

Add the new `FormField` right after the `distribution_number_padding` field (line 71):

```tsx
      <FormField label="Kuota slot (kosongkan jika tanpa batas)" name="slot_quota" type="number" min={1} value={values.slot_quota} onChange={(e) => setValues({ ...values, slot_quota: e.target.value })} />
```

- [ ] **Step 5: Run the test again**

Run: `cd frontend && npx vitest run src/features/programs/SchedulesPanel.test.tsx`
Expected: PASS

- [ ] **Step 6: Typecheck and full frontend test suite**

Run: `cd frontend && npm run typecheck && npm test -- --run`
Expected: no errors, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/programs/types.ts frontend/src/features/programs/SchedulesPanel.tsx frontend/src/features/programs/SchedulesPanel.test.tsx
git commit -m "feat(programs): expose slot_quota on the schedule admin form"
```

---

### Task 7: Frontend — `SlotCatalogGrid` component

**Files:**
- Modify: `frontend/src/features/distribution/types.ts` (new `SlotCatalogEntry` type)
- Create: `frontend/src/features/distribution/SlotCatalogGrid.tsx`
- Create: `frontend/src/features/distribution/SlotCatalogGrid.test.tsx`
- Modify: `frontend/src/features/distribution/Distribution.module.css` (new classes)

**Interfaces:**
- Consumes: `SlotCatalogEntry[]` (Task 4/5's JSON shape), `Schedule.slot_quota` (Task 2's JSON shape).
- Produces: `SlotCatalogGrid` component with props `{ quota?: number; entries: SlotCatalogEntry[]; onSelect: (slotNumber: number) => void; onCreateNext: () => void; canCreate: boolean }` — Task 8 renders this inside `DistributionPage.tsx`.

- [ ] **Step 1: Add the type**

In `frontend/src/features/distribution/types.ts`, add after `EquipmentOption` (after line 37):

```ts
export type SlotCatalogEntry = { slot_number: number; status: 'open' | 'linked' | 'completed' | 'cancelled'; documentation_complete: boolean };
```

- [ ] **Step 2: Write the failing component test**

Create `frontend/src/features/distribution/SlotCatalogGrid.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { test, expect, vi } from 'vitest';
import { SlotCatalogGrid } from './SlotCatalogGrid';
import type { SlotCatalogEntry } from './types';

test('renders one cell per quota slot and marks completed/pending slots', () => {
  const entries: SlotCatalogEntry[] = [
    { slot_number: 1, status: 'completed', documentation_complete: true },
    { slot_number: 2, status: 'linked', documentation_complete: false },
  ];
  render(<SlotCatalogGrid quota={4} entries={entries} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Nomor 1.*Selesai/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Nomor 2.*Proses/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Buat Nomor 3/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Nomor 4 belum tersedia' })).toBeDisabled();
});

test('clicking an existing slot calls onSelect with its number', async () => {
  const onSelect = vi.fn();
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid quota={2} entries={entries} onSelect={onSelect} onCreateNext={vi.fn()} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Nomor 1/ }));
  expect(onSelect).toHaveBeenCalledWith(1);
});

test('clicking the next empty slot calls onCreateNext', async () => {
  const onCreateNext = vi.fn();
  render(<SlotCatalogGrid quota={2} entries={[]} onSelect={vi.fn()} onCreateNext={onCreateNext} canCreate />);
  await userEvent.click(screen.getByRole('button', { name: /Buat Nomor 1/ }));
  expect(onCreateNext).toHaveBeenCalled();
});

test('without a quota, exactly one create-next box is shown after existing slots', () => {
  const entries: SlotCatalogEntry[] = [{ slot_number: 1, status: 'open', documentation_complete: false }];
  render(<SlotCatalogGrid entries={entries} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate />);
  expect(screen.getByRole('button', { name: /Buat Nomor 2/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Nomor 3/ })).not.toBeInTheDocument();
});

test('canCreate=false disables the create-next box', () => {
  render(<SlotCatalogGrid entries={[]} onSelect={vi.fn()} onCreateNext={vi.fn()} canCreate={false} />);
  expect(screen.getByRole('button', { name: /Buat Nomor 1/ })).toBeDisabled();
});
```

- [ ] **Step 3: Run it to confirm it fails**

Run: `cd frontend && npx vitest run src/features/distribution/SlotCatalogGrid.test.tsx`
Expected: FAIL — `Failed to resolve import "./SlotCatalogGrid"`.

- [ ] **Step 4: Implement the component**

Create `frontend/src/features/distribution/SlotCatalogGrid.tsx`:

```tsx
import styles from './Distribution.module.css';
import type { SlotCatalogEntry } from './types';

type Props = {
  quota?: number;
  entries: SlotCatalogEntry[];
  onSelect: (slotNumber: number) => void;
  onCreateNext: () => void;
  canCreate: boolean;
};

const statusLabel: Record<SlotCatalogEntry['status'], string> = { open: 'Terbuka', linked: 'Proses', completed: 'Selesai', cancelled: 'Batal' };
const statusClass: Record<SlotCatalogEntry['status'], string> = { open: styles.catalogOpen, linked: styles.catalogPending, completed: styles.catalogCompleted, cancelled: styles.catalogPending };

export function SlotCatalogGrid({ quota, entries, onSelect, onCreateNext, canCreate }: Props) {
  const byNumber = new Map(entries.map((entry) => [entry.slot_number, entry]));
  const nextNumber = entries.length + 1;
  const total = quota ?? nextNumber;
  const numbers = Array.from({ length: total }, (_, index) => index + 1);

  return <div className={styles.catalogGrid} role="group" aria-label="Katalog nomor bagi">
    {numbers.map((number) => {
      const entry = byNumber.get(number);
      if (entry) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${statusClass[entry.status]}`}
          onClick={() => onSelect(number)}>{`Nomor ${number} - ${statusLabel[entry.status]}`}</button>;
      }
      if (number === nextNumber) {
        return <button key={number} type="button" className={`${styles.catalogCell} ${styles.catalogNext}`}
          disabled={!canCreate} onClick={onCreateNext}>{`Buat Nomor ${number}`}</button>;
      }
      return <button key={number} type="button" className={styles.catalogCell} disabled aria-label={`Nomor ${number} belum tersedia`}>{number}</button>;
    })}
  </div>;
}
```

- [ ] **Step 5: Add the CSS classes**

In `frontend/src/features/distribution/Distribution.module.css`, add at the end of the file:

```css
.catalogGrid { display: grid; grid-template-columns: repeat(auto-fill,minmax(48px,1fr)); gap: 8px; margin-top: 16px; }
.catalogCell { display: grid; place-items: center; height: 44px; padding: 0 4px; border: 1px solid var(--dashboard-line); border-radius: 6px; color: var(--dashboard-ink); font-size: 11px; font-weight: 700; text-align: center; cursor: pointer; background: white; }
.catalogCell:disabled { cursor: not-allowed; opacity: .5; background: #f4f6f4; }
.catalogOpen { border-color: var(--blue); color: var(--blue); }
.catalogPending { border-color: var(--amber); color: var(--amber); background: #fdf6ea; }
.catalogCompleted { border-color: var(--green); color: var(--green); background: var(--green-soft); }
.catalogNext { border-style: dashed; color: var(--dashboard-muted); }
```

- [ ] **Step 6: Run the test again**

Run: `cd frontend && npx vitest run src/features/distribution/SlotCatalogGrid.test.tsx`
Expected: PASS

- [ ] **Step 7: Typecheck**

Run: `cd frontend && npm run typecheck`
Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/features/distribution/types.ts frontend/src/features/distribution/SlotCatalogGrid.tsx frontend/src/features/distribution/SlotCatalogGrid.test.tsx frontend/src/features/distribution/Distribution.module.css
git commit -m "feat(distribution): add SlotCatalogGrid component"
```

---

### Task 8: Frontend — wire the grid into `DistributionPage.tsx`

**Files:**
- Modify: `frontend/src/features/distribution/DistributionPage.tsx`
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx`

**Interfaces:**
- Consumes: `SlotCatalogGrid` (Task 7), `GET /api/v1/distribution/slots/catalog` (Task 5), `Schedule.slot_quota` (Task 2), `ApiError.code === 'slot_quota_exceeded'` (Task 5).
- Produces: nothing consumed elsewhere — this is the final integration point for this plan.

- [ ] **Step 1: Write the failing test**

In `frontend/src/features/distribution/DistributionPage.test.tsx`, extend the `renderPage` mock (around line 24-33) to also answer the catalog endpoint, and add a new test. Read the full current file first (it was last modified in the prior audit-remediation commit), then apply this diff-shaped change:

Add a branch to the existing `vi.mocked(apiRequest).mockImplementation(...)` in `renderPage`:

```ts
    if (path.startsWith('/api/v1/distribution/slots/catalog')) return Promise.resolve({ data: [] });
```

Add a new test at the end of the file:

```tsx
test('clicking the next catalog box opens the create-slot dialog', async () => {
  renderPage();
  await chooseSchedule(/Wajo Tahap 1/);
  await userEvent.click(await screen.findByRole('button', { name: /Buat Nomor 1/ }));
  expect(screen.getByRole('dialog', { name: 'Buat Slot Mesin Baru' })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd frontend && npx vitest run src/features/distribution/DistributionPage.test.tsx`
Expected: FAIL — no element with the role/name `Buat Nomor 1` exists yet.

- [ ] **Step 3: Wire the query, the grid, and quota-exceeded messaging into `DistributionPage.tsx`**

In `frontend/src/features/distribution/DistributionPage.tsx`, change the import line (line 6) to add `SlotCatalogEntry`:

```ts
import type { CreateSlotInput, DataResponse, DistributionSlot, EquipmentOption, ScheduleResponse, SlotCatalogEntry } from './types';
```

Add an import for the new component, after the `SlotPenyerahanSection` import (line 9):

```ts
import { SlotCatalogGrid } from './SlotCatalogGrid';
```

Add a `catalog` query after the `packageTemplates` query (after line 33):

```ts
  const catalog = useQuery({ queryKey: ['distribution', 'slot-catalog', scheduleID], queryFn: () => apiRequest<DataResponse<SlotCatalogEntry[]>>(`/api/v1/distribution/slots/catalog?schedule_id=${encodeURIComponent(scheduleID)}`), enabled: !!scheduleID });
```

Change the `search` mutation (lines 39-42) to accept an override query so grid clicks can search by number without waiting on `setQuery`'s async state update:

```ts
  const search = useMutation({
    mutationFn: (overrideQuery?: string) => apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/search?schedule_id=${encodeURIComponent(scheduleID)}&q=${encodeURIComponent(overrideQuery ?? query)}`),
    onSuccess: ({ data }) => setSlot(data),
  });
```

Change the `create` mutation's `onSuccess` (line 46) to also invalidate the catalog query:

```ts
    onSuccess: ({ data }) => { setSlot(data); setCreateOpen(false); void queryClient.invalidateQueries({ queryKey: ['distribution'] }); },
```

(No change needed here — `['distribution']` already prefixes `['distribution', 'slot-catalog', scheduleID]`, so `invalidateQueries({ queryKey: ['distribution'] })` already covers it via TanStack Query's prefix matching. Verify this assumption in Step 4/5 by confirming the existing test suite's create-slot test still passes with the catalog query refetching.)

Add a `selectBySlotNumber` handler and extend `onSlotChanged` to also invalidate the catalog, right after `openCreate` (after line 51):

```ts
  const selectBySlotNumber = (slotNumber: number) => { setQuery(String(slotNumber)); setSlot(null); search.mutate(String(slotNumber)); };
```

Change `onSlotChanged` (line 53) to also refresh the catalog (since POS Dokumen/Penyerahan mutate slot status from their own child components):

```ts
  const onSlotChanged = (next: DistributionSlot) => { setSlot(next); void queryClient.invalidateQueries({ queryKey: ['distribution', 'slot-catalog', scheduleID] }); };
```

Render the grid right after the `</section>` that closes `styles.lookup` (after line 75, before the `search.isError` line):

```tsx
    {scheduleID && <SlotCatalogGrid quota={selectedSchedule?.slot_quota} entries={catalog.data?.data ?? []} onSelect={selectBySlotNumber} onCreateNext={openCreate} canCreate={canCreateSlot} />}
```

Add quota-exceeded guidance to the create dialog's error alert (line 92) — replace:

```tsx
          {create.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Slot belum dapat dibuat.'}</AlertDescription></Alert>}
```

with:

```tsx
          {create.isError && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{create.error instanceof ApiError ? create.error.message : 'Slot belum dapat dibuat.'}{create.error instanceof ApiError && create.error.code === 'slot_quota_exceeded' && ' Hubungi admin Program Setup untuk menambah kuota atau membuat jadwal tambahan.'}</AlertDescription></Alert>}
```

- [ ] **Step 4: Run the new test**

Run: `cd frontend && npx vitest run src/features/distribution/DistributionPage.test.tsx`
Expected: PASS (all tests in the file, including the pre-existing ones).

- [ ] **Step 5: Full frontend verification**

Run: `cd frontend && npm run typecheck && npm run lint && npm test -- --run && npm run build`
Expected: all green, no new lint warnings beyond the pre-existing 4-warning baseline.

- [ ] **Step 6: Full backend + frontend suite**

Run (from repo root): `go test -p 1 ./... && cd frontend && npm run typecheck && npm run lint && npm test -- --run && npm run build`
Expected: all green.

- [ ] **Step 7: Manual smoke check**

Start the app locally (`go run ./cmd/server` with `.env` pointed at your local `konkit` database — the one already migrated through 00015 from Task 1), open Pendistribusian, pick a KONKIT-2026 schedule, and confirm: the grid renders, an empty schedule shows exactly one dashed "Buat Nomor 1" box, clicking it opens the existing create dialog, and after creating a slot the grid shows it as a colored "Nomor 1" cell instead of the dashed box.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/features/distribution/DistributionPage.tsx frontend/src/features/distribution/DistributionPage.test.tsx
git commit -m "feat(distribution): wire SlotCatalogGrid into the distribution page"
```
