# Distribution Equipment Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the equipment data BAST will eventually need — machine/hose option lists on package templates, an external supervisor name on schedules, and per-recipient serial numbers/option selections on distributions — without building the BAST generator itself.

**Architecture:** Extend the existing `programs` domain (package template `values_json`, new `program_schedules.supervisor_name` column) and the existing `distribution` domain (`distribution_records` gains five equipment columns, edited through the existing draft-save flow and folded into `verification_snapshot_json` at completion). No new tables, no new API endpoints, no new permissions.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgx v5, goose, React 19, TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-04-distribution-equipment-data-design.md`

## Global Constraints

- Machine and hose brand/type are per-schedule **option lists** (`machine_options`, `hose_options`), each entry shaped `{code, brand, type}` / `{code, brand, spec}`, stored inside the existing `package_template_versions.values_json` jsonb column — no migration needed for the template table itself.
- `converter_brand` and `components` in `values_json` are unchanged.
- A package template requires at least one `machine_options` entry and one `hose_options` entry (each with non-empty `code`, `brand`, and `type`/`spec`) **only when its status is `published`** — draft templates can be saved incomplete.
- `program_schedules.supervisor_name` (Konsultan Pengawas) is optional free text, set once per schedule.
- `distribution_records` gains five optional columns: `machine_option_code`, `machine_serial_number`, `hose_option_code`, `hose_serial_number`, `converter_serial_number`. They are edited through the existing `distribution.DraftInput`/`SaveDraft` flow — no new endpoint.
- Pelaksana Pemasangan is **not** a new field — it stays derived from `distribution_records.distributed_by`, already captured today.
- These new fields are not required for `Complete()` to succeed at this stage; that rule is deferred to the BAST Generator plan.
- Equipment field changes are covered by the existing `distribution.draft_updated` audit event — no new audit action.
- Follow existing code patterns exactly: the same draft-lock-update transaction shape already used in `distribution.Repository.SaveDraft`, the same versioned-template save logic already in `programs.Repository.SavePackageTemplate`, and the same `FormField`/`SetupDialog` frontend conventions already used in `programs` panels.

---

### Task 1: Schema Migration

**Files:**
- Create: `internal/database/migrations/00005_distribution_equipment.sql`

**Interfaces:**
- Produces columns consumed by Tasks 2 and 3.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
ALTER TABLE program_schedules ADD COLUMN supervisor_name text;

ALTER TABLE distribution_records
    ADD COLUMN machine_option_code text,
    ADD COLUMN machine_serial_number text,
    ADD COLUMN hose_option_code text,
    ADD COLUMN hose_serial_number text,
    ADD COLUMN converter_serial_number text;

UPDATE package_template_versions
SET values_json = values_json || '{"machine_options":[{"code":"shark-spwp8030","brand":"SHARK","type":"SPWP 80-30/3\""}],"hose_options":[{"code":"triliunhose-yamakoyo","brand":"TRILIUNHOSE/YAMAKOYO","spec":"Panjang Selang Hisap: 6m, Panjang Selang Buang: 10m"}]}'::jsonb
WHERE template_code IN ('PETANI-LPG', 'NELAYAN-LPG') AND version = 1;

-- +goose Down
UPDATE package_template_versions
SET values_json = values_json - 'machine_options' - 'hose_options'
WHERE template_code IN ('PETANI-LPG', 'NELAYAN-LPG') AND version = 1;

ALTER TABLE distribution_records
    DROP COLUMN machine_option_code,
    DROP COLUMN machine_serial_number,
    DROP COLUMN hose_option_code,
    DROP COLUMN hose_serial_number,
    DROP COLUMN converter_serial_number;

ALTER TABLE program_schedules DROP COLUMN supervisor_name;
```

The backfill `UPDATE` seeds real `machine_options`/`hose_options` onto the two existing published templates (`PETANI-LPG`, `NELAYAN-LPG`) so every fixture and manual test created after this migration already has selectable equipment options, matching the sample BAST document (`SHARK SPWP 80-30/3"`, `TRILIUNHOSE/YAMAKOYO`).

- [ ] **Step 2: Apply and verify**

Run:

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Expected: migration `00005_distribution_equipment.sql` is applied; status lists versions 1 through 5.

- [ ] **Step 3: Confirm the backfill**

Run:

```powershell
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
psql $url -c "SELECT template_code, values_json->'machine_options', values_json->'hose_options' FROM package_template_versions WHERE template_code IN ('PETANI-LPG','NELAYAN-LPG') AND version=1;"
```

Expected: both rows show a one-element `machine_options` array and a one-element `hose_options` array. (If `psql` is not on PATH, open the same query in any Postgres client instead — this step is a manual sanity check, not part of the automated suite.)

- [ ] **Step 4: Commit**

```powershell
git add internal/database/migrations/00005_distribution_equipment.sql
git commit -m "feat: add distribution equipment schema"
```

---

### Task 2: Programs Domain — Equipment Options and Supervisor Name

**Files:**
- Modify: `internal/programs/models.go`
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`
- Modify: `internal/programs/repository.go`
- Modify: `internal/programs/repository_integration_test.go`

**Interfaces:**
- Produces `ErrPackageOptionsRequired` error.
- Extends `Schedule`/`ScheduleInput` with `SupervisorName string`.
- Consumed by Task 4 (frontend) and read indirectly by Task 3 (equipment options travel to the client through the existing `package_snapshot_json`, not through a new programs endpoint).

- [ ] **Step 1: Write the failing service test**

Append to `internal/programs/service_test.go`:

```go
func TestSavePackageTemplateRequiresEquipmentOptionsWhenPublishing(t *testing.T) {
	service := NewService(&repositoryStub{})
	_, err := service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "published",
		Values: map[string]any{"converter_brand": "ERGAS"},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrPackageOptionsRequired) {
		t.Fatalf("missing options err=%v", err)
	}

	_, err = service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "draft",
		Values: map[string]any{"converter_brand": "ERGAS"},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("draft without options should be allowed: %v", err)
	}

	_, err = service.SavePackageTemplate(context.Background(), auth.Principal{}, PackageTemplateInput{
		TemplateCode: "TEST-PKG", Name: "Template", ProgramType: ProgramFarmer, Status: "published",
		Values: map[string]any{
			"converter_brand": "ERGAS",
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\""}},
			"hose_options":    []any{map[string]any{"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6m/10m"}},
		},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("complete options should be allowed: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/programs -run TestSavePackageTemplateRequiresEquipmentOptionsWhenPublishing -count=1`

Expected: FAIL because `ErrPackageOptionsRequired` does not exist.

- [ ] **Step 3: Add the error and validator**

Modify `internal/programs/models.go`: add to the `var (...)` error block:

```go
ErrPackageOptionsRequired = errors.New("package template requires at least one machine option and one hose option when published")
```

- [ ] **Step 4: Add `SupervisorName` to the schedule types**

Modify `internal/programs/models.go`: add `SupervisorName string` to both `Schedule` and `ScheduleInput`:

```go
type Schedule struct {
	ID                             string                 `json:"id"`
	ProgramID                      string                 `json:"program_id"`
	RegencyID                      string                 `json:"regency_id"`
	PackageTemplateVersionID       string                 `json:"package_template_version_id"`
	DocumentationTemplateVersionID string                 `json:"documentation_template_version_id"`
	Name                           string                 `json:"name"`
	StartDate                      time.Time              `json:"start_date"`
	EndDate                        time.Time              `json:"end_date"`
	Status                         string                 `json:"status"`
	DistributionNumberPadding      int                    `json:"distribution_number_padding"`
	ReceiptPolicy                  map[string]any         `json:"receipt_policy"`
	SupervisorName                 string                 `json:"supervisor_name,omitempty"`
	Notes                          string                 `json:"notes,omitempty"`
	Program                        *Program               `json:"program,omitempty"`
	Regency                        *Regency               `json:"regency,omitempty"`
	PackageTemplate                *PackageTemplate       `json:"package_template,omitempty"`
	DocumentationTemplate          *DocumentationTemplate `json:"documentation_template,omitempty"`
	CreatedAt                      time.Time              `json:"created_at"`
	UpdatedAt                      time.Time              `json:"updated_at"`
}

type ScheduleInput struct {
	ID                             string         `json:"id,omitempty"`
	ProgramID                      string         `json:"program_id"`
	RegencyID                      string         `json:"regency_id"`
	PackageTemplateVersionID       string         `json:"package_template_version_id"`
	DocumentationTemplateVersionID string         `json:"documentation_template_version_id"`
	Name                           string         `json:"name"`
	StartDate                      time.Time      `json:"start_date"`
	EndDate                        time.Time      `json:"end_date"`
	Status                         string         `json:"status"`
	DistributionNumberPadding      int            `json:"distribution_number_padding"`
	ReceiptPolicy                  map[string]any `json:"receipt_policy"`
	SupervisorName                 string         `json:"supervisor_name"`
	Notes                          string         `json:"notes"`
}
```

- [ ] **Step 5: Enforce the validator and trim supervisor name in the service**

Modify `internal/programs/service.go`: in `SavePackageTemplate`, after the existing `if input.Values == nil { ... }` block, add:

```go
if input.Status == "published" && !hasEquipmentOptions(input.Values) {
	return PackageTemplate{}, ErrPackageOptionsRequired
}
```

Add the validator function at the bottom of the file:

```go
func hasEquipmentOptions(values map[string]any) bool {
	return validOptionList(values["machine_options"], "brand", "type") && validOptionList(values["hose_options"], "brand", "spec")
}

func validOptionList(raw any, secondField string, thirdField string) bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, entry := range list {
		option, ok := entry.(map[string]any)
		if !ok || !nonEmptyString(option["code"]) || !nonEmptyString(option[secondField]) || !nonEmptyString(option[thirdField]) {
			return false
		}
	}
	return true
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}
```

In `SaveSchedule`, add supervisor name trimming next to the existing `input.Notes = strings.TrimSpace(input.Notes)` line:

```go
input.SupervisorName = strings.TrimSpace(input.SupervisorName)
```

- [ ] **Step 6: Run the service test to verify it passes**

Run: `go test ./internal/programs -count=1`

Expected: PASS.

- [ ] **Step 7: Persist `supervisor_name` in the repository**

Modify `internal/programs/repository.go`:

In `scheduleSelect`, add `COALESCE(s.supervisor_name,'')` right after `COALESCE(s.notes,'')`:

```go
const scheduleSelect = `
SELECT s.id::text,s.program_id::text,s.regency_id::text,s.package_template_version_id::text,s.documentation_template_version_id::text,s.name,s.start_date,s.end_date,s.status,s.distribution_number_padding,s.receipt_policy_json,COALESCE(s.notes,''),COALESCE(s.supervisor_name,''),s.created_at,s.updated_at,
p.id::text,p.code,p.name,p.program_type,p.fiscal_year,p.status,COALESCE(p.notes,''),p.created_at,p.updated_at,
r.id::text,r.province_name,r.name,r.document_code,r.is_active,COALESCE(r.notes,''),r.created_at,r.updated_at
FROM program_schedules s JOIN programs p ON p.id=s.program_id JOIN regencies r ON r.id=s.regency_id`
```

In `scanSchedule`, add `&item.SupervisorName` to the `Scan` call right after `&item.Notes`:

```go
err := row.Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.PackageTemplateVersionID, &item.DocumentationTemplateVersionID, &item.Name, &item.StartDate, &item.EndDate, &item.Status, &item.DistributionNumberPadding, &policy, &item.Notes, &item.SupervisorName, &item.CreatedAt, &item.UpdatedAt,
	&item.Program.ID, &item.Program.Code, &item.Program.Name, &item.Program.ProgramType, &item.Program.FiscalYear, &item.Program.Status, &item.Program.Notes, &item.Program.CreatedAt, &item.Program.UpdatedAt,
	&item.Regency.ID, &item.Regency.ProvinceName, &item.Regency.Name, &item.Regency.DocumentCode, &item.Regency.IsActive, &item.Regency.Notes, &item.Regency.CreatedAt, &item.Regency.UpdatedAt)
```

In `SaveSchedule`, add `supervisor_name` to both the `INSERT` and `UPDATE` statements:

```go
if id == "" {
	err = tx.QueryRow(ctx, `INSERT INTO program_schedules (program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json,notes,supervisor_name) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,'') FROM programs p JOIN package_template_versions pt ON pt.id=$3 JOIN documentation_template_versions dt ON dt.id=$4 WHERE p.id=$1 AND p.program_type=pt.program_type AND p.program_type=dt.program_type RETURNING id::text`, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName).Scan(&id)
} else {
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE program_schedules s SET program_id=$2,regency_id=$3,package_template_version_id=$4,documentation_template_version_id=$5,name=$6,start_date=$7,end_date=$8,status=$9,distribution_number_padding=$10,receipt_policy_json=$11,notes=NULLIF($12,''),supervisor_name=NULLIF($13,''),updated_at=now() WHERE s.id=$1 AND EXISTS (SELECT 1 FROM programs p JOIN package_template_versions pt ON pt.id=$4 JOIN documentation_template_versions dt ON dt.id=$5 WHERE p.id=$2 AND p.program_type=pt.program_type AND p.program_type=dt.program_type)`, id, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName)
	if err == nil && tag.RowsAffected() == 0 {
		return Schedule{}, ErrNotFound
	}
}
```

- [ ] **Step 8: Fix the existing integration test that publishes without equipment options**

Modify `internal/programs/repository_integration_test.go`: the existing `service.SavePackageTemplate` call with `Status: "published"` (around line 64-67) will now fail validation. Update its `Values`:

```go
template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
	TemplateCode: templateCode, Name: "Template Awal", ProgramType: ProgramFarmer,
	Values: map[string]any{
		"converter_brand": "ERGAS",
		"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\""}},
		"hose_options":    []any{map[string]any{"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6m/10m"}},
	},
	Status: "published",
}, meta)
```

Leave the second `SavePackageTemplate` call (which saves as `"draft"`) unchanged.

- [ ] **Step 9: Add a supervisor name integration assertion**

In the same test, after the existing schedule assertion block (`if schedule.Program == nil || ...`), add:

```go
scheduleWithSupervisor, err := service.SaveSchedule(ctx, actor, ScheduleInput{
	ID: schedule.ID, ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
	DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap 1",
	StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	EndDate:   time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
	SupervisorName: "  Andi Amrullah  ",
}, meta)
if err != nil {
	t.Fatal(err)
}
if scheduleWithSupervisor.SupervisorName != "Andi Amrullah" {
	t.Fatalf("supervisor name=%q", scheduleWithSupervisor.SupervisorName)
}
```

- [ ] **Step 10: Run the integration test**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test ./internal/programs -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```powershell
git add internal/programs
git commit -m "feat: add package equipment options and schedule supervisor name"
```

---

### Task 3: Distribution Domain — Equipment Fields on the Draft and Completion Snapshot

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/repository_integration_test.go`

**Interfaces:**
- Extends `distribution.DraftInput` and `distribution.RecipientWorkspace` with five new fields.
- `Complete()` folds the same five values into `verification_snapshot_json.equipment`.
- Consumed by Task 5 (frontend).

- [ ] **Step 1: Write the failing service test**

Append to `internal/distribution/service_test.go` (the existing `repositoryStub.SaveDraft` already returns `r.workspace`; extend the assertion on saved input):

```go
func TestSaveDraftPassesEquipmentFieldsThrough(t *testing.T) {
	repository := &repositoryStub{workspace: RecipientWorkspace{NIK: "7306014101900001"}}
	service := NewService(repository)
	input := DraftInput{
		NIK: "7306014101900001",
		MachineOptionCode: " shark-spwp8030 ", MachineSerialNumber: " SP 06IABD 421291 ",
		HoseOptionCode: " triliunhose ", HoseSerialNumber: "",
		ConverterSerialNumber: " 240A005582 ",
	}

	_, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saved.MachineOptionCode != "shark-spwp8030" || repository.saved.MachineSerialNumber != "SP 06IABD 421291" {
		t.Fatalf("machine fields not trimmed/passed: %+v", repository.saved)
	}
	if repository.saved.HoseOptionCode != "triliunhose" || repository.saved.HoseSerialNumber != "" {
		t.Fatalf("hose fields not trimmed/passed: %+v", repository.saved)
	}
	if repository.saved.ConverterSerialNumber != "240A005582" {
		t.Fatalf("converter serial not trimmed/passed: %+v", repository.saved)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/distribution -run TestSaveDraftPassesEquipmentFieldsThrough -count=1`

Expected: FAIL because `DraftInput` has no equipment fields yet.

- [ ] **Step 3: Extend the models**

Modify `internal/distribution/models.go`:

```go
type DraftInput struct {
	NIK                    string `json:"nik"`
	Address                string `json:"address"`
	Village                string `json:"village"`
	District               string `json:"district"`
	PhoneNumber            string `json:"phone_number"`
	SectorIdentifier       string `json:"sector_identifier"`
	IdentityChangeReason   string `json:"identity_change_reason"`
	MachineOptionCode      string `json:"machine_option_code"`
	MachineSerialNumber    string `json:"machine_serial_number"`
	HoseOptionCode         string `json:"hose_option_code"`
	HoseSerialNumber       string `json:"hose_serial_number"`
	ConverterSerialNumber  string `json:"converter_serial_number"`
}
```

Add the same five fields to `RecipientWorkspace`, right after `PhoneNumber`:

```go
	PhoneNumber            string           `json:"phone_number,omitempty"`
	MachineOptionCode      string           `json:"machine_option_code,omitempty"`
	MachineSerialNumber    string           `json:"machine_serial_number,omitempty"`
	HoseOptionCode         string           `json:"hose_option_code,omitempty"`
	HoseSerialNumber       string           `json:"hose_serial_number,omitempty"`
	ConverterSerialNumber  string           `json:"converter_serial_number,omitempty"`
```

(Keep every other field in both structs exactly as it is today — this only inserts the five new fields.)

- [ ] **Step 4: Trim the new fields in the service**

Modify `internal/distribution/service.go`, in `SaveDraft`, alongside the existing `input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)` line, add:

```go
input.MachineOptionCode = strings.TrimSpace(input.MachineOptionCode)
input.MachineSerialNumber = strings.TrimSpace(input.MachineSerialNumber)
input.HoseOptionCode = strings.TrimSpace(input.HoseOptionCode)
input.HoseSerialNumber = strings.TrimSpace(input.HoseSerialNumber)
input.ConverterSerialNumber = strings.TrimSpace(input.ConverterSerialNumber)
```

- [ ] **Step 5: Run the service test to verify it passes**

Run: `go test ./internal/distribution -count=1`

Expected: PASS.

- [ ] **Step 6: Persist equipment fields in `GetWorkspace`**

Modify `internal/distribution/repository.go`, in `GetWorkspace`: add the five new columns to the `SELECT` list right after `n.source_snapshot_json,a.package_snapshot_json,` and before the `CASE` clause:

```go
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
```

Add the matching `Scan` destinations right after `&packageJSON,` and before `&result.Eligibility`:

```go
	).Scan(
		&result.AllocationID, &result.DistributionID, &result.ScheduleID, &result.DistributionNumber,
		&result.AllocationStatus, &result.DistributionStatus, &result.ProgramType, &result.ProgramName,
		&result.RegencyName, &result.FullName, &result.NIK, &result.SectorIdentifier,
		&result.SectorIdentifierType, &result.Address, &result.Village, &result.District,
		&result.PhoneNumber, &sourceJSON, &packageJSON,
		&result.MachineOptionCode, &result.MachineSerialNumber, &result.HoseOptionCode, &result.HoseSerialNumber, &result.ConverterSerialNumber,
		&result.Eligibility,
	)
```

- [ ] **Step 7: Persist equipment fields in `SaveDraft`**

Modify `internal/distribution/repository.go`, in `SaveDraft`: add `dr.id::text` to the lock query and lock `dr` alongside `p`:

```go
	var personID, distributionID, programType, oldNIK, oldAddress, oldVillage, oldDistrict, oldPhone string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text,dr.id::text,pr.program_type,COALESCE(p.nik,''),COALESCE(p.address,''),COALESCE(p.village,''),COALESCE(p.district,''),COALESCE(p.phone_number,'')
		FROM package_allocations a JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN program_schedules ps ON ps.id=a.schedule_id JOIN programs pr ON pr.id=ps.program_id
		JOIN distribution_records dr ON dr.allocation_id=a.id
		JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,dr.recipient_person_id,a.intended_person_id,n.person_id)
		WHERE a.id=$1 FOR UPDATE OF p, dr
	`, allocationID).Scan(&personID, &distributionID, &programType, &oldNIK, &oldAddress, &oldVillage, &oldDistrict, &oldPhone)
```

Right after the existing `UPDATE people ...` call and its error handling (before the sector-identifier block), add:

```go
	_, err = tx.Exec(ctx, `UPDATE distribution_records SET machine_option_code=NULLIF($2,''),machine_serial_number=NULLIF($3,''),hose_option_code=NULLIF($4,''),hose_serial_number=NULLIF($5,''),converter_serial_number=NULLIF($6,''),updated_at=now() WHERE id=$1`, distributionID, input.MachineOptionCode, input.MachineSerialNumber, input.HoseOptionCode, input.HoseSerialNumber, input.ConverterSerialNumber)
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("update recipient equipment: %w", err)
	}
```

- [ ] **Step 8: Fold equipment fields into the completion snapshot**

Modify `internal/distribution/repository.go`, in `Complete`: add the five columns to the initial lock `SELECT` (right after `a.package_snapshot_json`) and matching `Scan` destinations:

```go
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
		WHERE a.id=$1
		FOR UPDATE OF a,dr,p
	`, allocationID).Scan(&record.ID, &record.Status, &allocationStatus, &personID, &fullName, &nik, &programType, &sectorType, &sectorIdentifier, &packageJSON,
		&machineOptionCode, &machineSerialNumber, &hoseOptionCode, &hoseSerialNumber, &converterSerialNumber)
```

In the snapshot construction, add an `"equipment"` key alongside the existing `"identity"`, `"package"`, and `"documentation"` keys:

```go
	snapshot, err := json.Marshal(map[string]any{
		"identity": map[string]any{"person_id": personID, "full_name": fullName, "nik": nik, "sector_identifier_type": sectorType, "sector_identifier": sectorIdentifier, "program_type": programType},
		"package":  packageSnapshot,
		"documentation": documentation,
		"equipment": map[string]any{
			"machine_option_code": machineOptionCode, "machine_serial_number": machineSerialNumber,
			"hose_option_code": hoseOptionCode, "hose_serial_number": hoseSerialNumber,
			"converter_serial_number": converterSerialNumber,
		},
	})
```

- [ ] **Step 9: Add an integration test for equipment persistence and snapshotting**

Append to `internal/distribution/repository_integration_test.go`:

```go
func TestIntegrationSaveDraftAndCompletePersistEquipmentFields(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := createDistributionFixture(t, pool)
	storage, err := mediastore.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(pool), storage)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.userID}
	meta := auth.ClientMeta{UserAgent: fixture.userAgent}

	// Use fixture.secondaryAllocationID (Siti Nur), not fixture.allocationID: createDistributionFixture
	// deliberately makes fixture.allocationID (Siti Aminah) "previously received" via historyAllocationID
	// in a different schedule, so Complete() on fixture.allocationID always returns ErrPreviouslyReceived
	// (see TestCompleteEnforcesFinalDistributionRules). secondaryAllocationID has no such history and has
	// no documentation_slots yet, so seed one directly satisfied (min_files=0) the same way the existing
	// TestCompleteSerializesConcurrentReceiptsForTheSamePerson test does.
	if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) SELECT id,'recipient_package','Penerima dan paket',true,0,1,'both','complete' FROM distribution_records WHERE allocation_id=$1`, fixture.secondaryAllocationID); err != nil {
		t.Fatal(err)
	}

	workspace, err := service.SaveDraft(ctx, actor, fixture.secondaryAllocationID, DraftInput{
		NIK: "7306014101900002", SectorIdentifier: "KP02",
		MachineOptionCode: "shark-spwp8030", MachineSerialNumber: "SP 06IABD 421291",
		HoseOptionCode: "triliunhose", HoseSerialNumber: "",
		ConverterSerialNumber: "240A005582",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.MachineOptionCode != "shark-spwp8030" || workspace.MachineSerialNumber != "SP 06IABD 421291" || workspace.ConverterSerialNumber != "240A005582" {
		t.Fatalf("workspace equipment=%+v", workspace)
	}

	record, err := service.Complete(ctx, actor, fixture.secondaryAllocationID, meta)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "completed" {
		t.Fatalf("record=%+v", record)
	}
	var snapshot string
	if err := pool.QueryRow(ctx, `SELECT verification_snapshot_json::text FROM distribution_records WHERE allocation_id=$1`, fixture.secondaryAllocationID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, `"machine_serial_number": "SP 06IABD 421291"`) || !strings.Contains(snapshot, `"converter_serial_number": "240A005582"`) {
		t.Fatalf("snapshot missing equipment data: %s", snapshot)
	}
}
```

This test reuses `createDistributionFixture`, so add `"strings"` to the file's imports if not already present (`internal/distribution/repository_integration_test.go` already imports `"strings"` for other assertions — confirm before adding a duplicate).

- [ ] **Step 10: Run the integration test**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test ./internal/distribution -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```powershell
git add internal/distribution
git commit -m "feat: capture distribution equipment fields and snapshot"
```

---

### Task 4: Program Setup Frontend — Supervisor Name and Template Hint

**Files:**
- Modify: `frontend/src/features/programs/types.ts`
- Modify: `frontend/src/features/programs/SchedulesPanel.tsx`
- Modify: `frontend/src/features/programs/ProgramSetupPage.test.tsx`
- Modify: `frontend/src/features/programs/TemplatesPanel.tsx`

**Interfaces:**
- Consumes the `supervisor_name` field added to `Schedule`/`ScheduleInput` in Task 2.
- No new component; extends the existing schedule dialog.

There is no dedicated `SchedulesPanel.test.tsx` in this codebase — schedule-dialog behavior is already covered inside `ProgramSetupPage.test.tsx` (see its `test('normalizes the schedule name to uppercase while typing', ...)`, which opens the "Jadwal" tab and the "Tambah jadwal" dialog). Extend that same file and pattern.

- [ ] **Step 1: Write the failing test**

Append to `frontend/src/features/programs/ProgramSetupPage.test.tsx`:

```tsx
test('accepts an optional supervisor name in the schedule dialog', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await userEvent.click(await screen.findByRole('tab', { name: 'Jadwal' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));

  const supervisor = screen.getByRole('textbox', { name: 'Konsultan pengawas' });
  await userEvent.type(supervisor, 'Andi Amrullah');

  expect(supervisor).toHaveValue('Andi Amrullah');
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/programs/ProgramSetupPage.test.tsx`

Expected: FAIL because no field is labeled "Konsultan pengawas" yet.

- [ ] **Step 3: Extend the `Schedule` type**

Modify `frontend/src/features/programs/types.ts`:

```ts
export type Schedule = { id: string; program_id: string; regency_id: string; package_template_version_id: string; documentation_template_version_id: string; name: string; start_date: string; end_date: string; status: string; distribution_number_padding: number; supervisor_name?: string; notes?: string; program?: Program; regency?: Regency; package_template?: PackageTemplate; documentation_template?: DocumentationTemplate };
```

- [ ] **Step 4: Add the field to the schedule form**

Modify `frontend/src/features/programs/SchedulesPanel.tsx`:

Add `supervisor_name: ''` to the `empty` object:

```ts
const empty = { program_id: '', regency_id: '', package_template_version_id: '', documentation_template_version_id: '', name: '', start_date: today, end_date: today, status: 'draft', distribution_number_padding: 4, supervisor_name: '', notes: '' };
```

Add `supervisor_name: item.supervisor_name ?? ''` to the `show()` function's populated-from-item branch:

```ts
const show = (item?: Schedule) => { setEditing(item); setValues(item ? { program_id: item.program_id, regency_id: item.regency_id, package_template_version_id: item.package_template_version_id, documentation_template_version_id: item.documentation_template_version_id, name: item.name, start_date: dateInputValue(item.start_date), end_date: dateInputValue(item.end_date), status: item.status, distribution_number_padding: item.distribution_number_padding, supervisor_name: item.supervisor_name ?? '', notes: item.notes ?? '' } : empty); setOpen(true); };
```

Add a `FormField` for it inside `<SetupDialog>`, right after the "Digit nomor pembagian" field and before "Catatan":

```tsx
<FormField label="Konsultan pengawas" name="supervisor_name" value={values.supervisor_name} onChange={(e) => setValues({ ...values, supervisor_name: e.target.value })} />
```

- [ ] **Step 5: Update the template panel hint text**

Modify `frontend/src/features/programs/TemplatesPanel.tsx`: update the existing hint under the package "Nilai paket" textarea to mention the new keys:

```tsx
<small>Struktur JSON menyimpan merek, tipe, komponen, jumlah, dan satuan, termasuk <code>machine_options</code> dan <code>hose_options</code> (masing-masing butuh <code>code</code>, <code>brand</code>, dan <code>type</code>/<code>spec</code>) sebelum template bisa dipublikasikan.</small>
```

- [ ] **Step 6: Run frontend verification**

Run:

```powershell
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/features/programs web/static/app
git commit -m "feat: add schedule supervisor name and template hint"
```

---

### Task 5: Distribution Frontend — Equipment Data Section

**Files:**
- Modify: `frontend/src/features/distribution/types.ts`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.tsx`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.test.tsx`

**Interfaces:**
- Consumes `machine_option_code`, `machine_serial_number`, `hose_option_code`, `hose_serial_number`, `converter_serial_number` from `RecipientWorkspaceData` (Task 3).
- Reads `machine_options`/`hose_options` out of the already-present `package_snapshot` field — no new API call.

- [ ] **Step 1: Write the failing UI test**

Modify `frontend/src/features/distribution/RecipientWorkspace.test.tsx`: extend the `ready` fixture's `package_snapshot` and add a new test:

```tsx
const ready: RecipientWorkspaceData = {
  allocation_id: 'allocation-1', distribution_id: 'distribution-1', schedule_id: 'schedule-1',
  distribution_number: 7, allocation_status: 'ready', distribution_status: 'draft',
  program_type: 'farmer', program_name: 'Program Petani 2026', regency_name: 'Wajo',
  full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier: 'KP01',
  sector_identifier_type: 'farmer_card', address: 'Jalan Sawah', village: 'Tempe',
  district: 'Sabbangparu', phone_number: '', eligibility: 'eligible', eligibility_reasons: [],
  source_snapshot: {}, package_snapshot: {
    converter_brand: 'ERGAS',
    machine_options: [{ code: 'shark-spwp8030', brand: 'SHARK', type: 'SPWP 80-30/3"' }],
    hose_options: [{ code: 'triliunhose', brand: 'TRILIUNHOSE', spec: '6m/10m' }],
  }, receipt_history: [], documentation: [{ id: 'slot-1', code: 'recipient_package', label: 'Penerima dan paket', status: 'complete', required: true, min_files: 1, max_files: 1, files: [] }],
};
```

Add a new test at the end of the file:

```tsx
test('saves equipment fields as part of the recipient draft', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...ready, machine_option_code: 'shark-spwp8030', machine_serial_number: 'SP 06IABD 421291' } });
  renderWorkspace();

  fireEvent.change(screen.getByLabelText('Merk/Tipe Mesin'), { target: { value: 'shark-spwp8030' } });
  fireEvent.change(screen.getByLabelText('Serial Number Mesin'), { target: { value: 'SP 06IABD 421291' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan draft' }));

  expect(await screen.findByText('Draft penerima tersimpan.')).toBeVisible();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/allocations/allocation-1/draft', expect.objectContaining({
    method: 'PATCH',
    body: expect.stringContaining('"machine_serial_number":"SP 06IABD 421291"'),
  }));
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/distribution/RecipientWorkspace.test.tsx`

Expected: FAIL because the "Merk/Tipe Mesin" field does not exist yet.

- [ ] **Step 3: Extend the types**

Modify `frontend/src/features/distribution/types.ts`:

```ts
export type DraftInput = {
  nik: string; sector_identifier: string; address: string; village: string; district: string; phone_number: string; identity_change_reason: string;
  machine_option_code: string; machine_serial_number: string; hose_option_code: string; hose_serial_number: string; converter_serial_number: string;
};
```

Add the same five optional fields to `RecipientWorkspaceData`, right after `phone_number`:

```ts
export type RecipientWorkspaceData = {
  allocation_id: string; distribution_id: string; schedule_id: string; distribution_number: number;
  allocation_status: string; distribution_status: string; program_type: 'farmer' | 'fisherman';
  program_name: string; regency_name: string; full_name: string; nik?: string;
  sector_identifier?: string; sector_identifier_type?: string; address?: string; village?: string;
  district?: string; phone_number?: string;
  machine_option_code?: string; machine_serial_number?: string; hose_option_code?: string; hose_serial_number?: string; converter_serial_number?: string;
  eligibility: string; eligibility_reasons: string[];
  source_snapshot: Record<string, unknown>; package_snapshot?: Record<string, unknown>; receipt_history: ReceiptHistory[]; documentation: SlotSummary[];
};
export type EquipmentOption = { code: string; brand: string; type?: string; spec?: string };
```

- [ ] **Step 4: Add the equipment section to the workspace**

Modify `frontend/src/features/distribution/RecipientWorkspace.tsx`:

Extend `draftFrom` to seed the five new keys:

```tsx
function draftFrom(data: RecipientWorkspaceData): DraftInput {
  return {
    nik: data.nik ?? '', sector_identifier: data.sector_identifier ?? '', address: data.address ?? '', village: data.village ?? '', district: data.district ?? '', phone_number: data.phone_number ?? '', identity_change_reason: '',
    machine_option_code: data.machine_option_code ?? '', machine_serial_number: data.machine_serial_number ?? '',
    hose_option_code: data.hose_option_code ?? '', hose_serial_number: data.hose_serial_number ?? '',
    converter_serial_number: data.converter_serial_number ?? '',
  };
}
```

Add the import `EquipmentOption` to the type import line:

```tsx
import type { DataResponse, DistributionRecord, DraftInput, EquipmentOption, RecipientWorkspaceData } from './types';
```

Inside the component, after the existing `packageEntries` line, read the option lists out of the snapshot:

```tsx
const machineOptions = (data.package_snapshot?.machine_options as EquipmentOption[] | undefined) ?? [];
const hoseOptions = (data.package_snapshot?.hose_options as EquipmentOption[] | undefined) ?? [];
```

Add a new "Data Peralatan" fields block inside the existing `<div className={styles.fields}>`, right after the "Nomor telepon" field and before the NIK-change-reason conditional field:

```tsx
<label><span>Merk/Tipe Mesin</span><select aria-label="Merk/Tipe Mesin" value={draft.machine_option_code} disabled={!editable} onChange={(event) => update('machine_option_code', event.target.value)}><option value="">Pilih mesin</option>{machineOptions.map((option) => <option key={option.code} value={option.code}>{option.brand} {option.type}</option>)}</select></label>
<label><span>Serial Number Mesin</span><input aria-label="Serial Number Mesin" value={draft.machine_serial_number} disabled={!editable} onChange={(event) => update('machine_serial_number', event.target.value)} /></label>
<label><span>Merk/Spesifikasi Selang</span><select aria-label="Merk/Spesifikasi Selang" value={draft.hose_option_code} disabled={!editable} onChange={(event) => update('hose_option_code', event.target.value)}><option value="">Pilih selang</option>{hoseOptions.map((option) => <option key={option.code} value={option.code}>{option.brand} {option.spec}</option>)}</select></label>
<label><span>Serial Number Selang</span><input aria-label="Serial Number Selang" value={draft.hose_serial_number} disabled={!editable} onChange={(event) => update('hose_serial_number', event.target.value)} /></label>
<label><span>Serial Number Konkit/Reducer</span><input aria-label="Serial Number Konkit/Reducer" value={draft.converter_serial_number} disabled={!editable} onChange={(event) => update('converter_serial_number', event.target.value)} /></label>
```

`update` and the `editable` flag already exist in this component and need no changes — they are generic over `keyof DraftInput`, so the five new keys work automatically.

- [ ] **Step 5: Run the frontend tests**

Run: `npm.cmd --prefix frontend test -- --run src/features/distribution/RecipientWorkspace.test.tsx`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/features/distribution
git commit -m "feat: add equipment data fields to recipient workspace"
```

---

### Task 6: Full Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes the complete Tasks 1-5 vertical slice.

- [ ] **Step 1: Document the change**

Modify `README.md`: extend the existing "Alur DCP3 Dan Pendistribusian" paragraph (or add one sentence after it) noting that package templates now define machine/hose option lists (`machine_options`, `hose_options`) required before publishing, schedules can record an optional Konsultan Pengawas, and Pendistribusian now captures per-recipient serial numbers as part of the existing draft save.

- [ ] **Step 2: Run the full backend and frontend suite**

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
go run ./cmd/migrate status
```

Expected: all commands exit 0; migration status lists versions 1 through 5. Use `-p 1` because several packages' integration tests share the `konkit_test` database and can otherwise contend with each other when run concurrently (a pre-existing characteristic of this test suite, unrelated to this feature).

- [ ] **Step 3: Commit**

```powershell
git add README.md web/static/app
git commit -m "docs: document distribution equipment data"
```
