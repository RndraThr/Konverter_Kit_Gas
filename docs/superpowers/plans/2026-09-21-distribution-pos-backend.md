# Distribution POS Backend Implementation Plan (Plan A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the single-flow Pendistribusian backend with a 3-station (POS Mesin / POS Dokumen / POS Penyerahan) model. Distribution numbers move from DCP3 import to POS Mesin; evidence stays attached to one persistent `distribution_slots` record across all 3 stations.

**Architecture:** New `distribution_slots` table (merged from the old `distribution_records`, gains `slot_number`/lifecycle status) is the anchor for evidence and equipment data from the moment a machine is tagged, through NIK-linking, to final handover. `package_allocations.distribution_number` becomes nullable and gets filled in by POS Dokumen instead of DCP3 import. `documentation_template_slots.stage` (already existed, unused) now drives which POS a slot belongs to.

**Tech Stack:** Go 1.26, PostgreSQL/pgx/v5, goose migrations.

**Spec:** No separate spec document — full design was worked out interactively with the user in this session's chat transcript; this plan encodes every decision made there. This is a deliberate deviation from the usual brainstorming→spec→plan flow, made at the user's explicit request to save time/tokens.

**No frontend in this plan.** A follow-up plan builds the 3 POS pages, removes the old Pendistribusian page, and adds the POS field to the Template Dokumentasi config UI. This plan must leave the backend buildable and fully tested on its own; frontend files that reference removed backend behavior are addressed in that follow-up plan, not here.

## Global Constraints

- Existing distributed/in-progress data must NOT be lost — the migration copies every `distribution_records` row into `distribution_slots`, preserving equipment fields, verification snapshot, and completion timestamps, reusing the same primary key so `documentation_slots`' FK repoint requires no remapping.
- `package_allocations.distribution_number` stays `UNIQUE (schedule_id, distribution_number)` but becomes nullable — Postgres treats multiple NULLs as non-conflicting, so unlinked allocations coexist fine.
- Evidence (`documentation_slots`/`media_files`) attaches to `distribution_slots` from the moment a slot is created at POS Mesin — snapshot ALL of a schedule's `documentation_template_slots` (all 3 stages) at slot-creation time, not per-POS. This is why evidence never needs to move between "folders" as a recipient's data crosses POS.
- NIK entered at POS Dokumen MUST match an existing, unlinked DCP3 candidate for that schedule (`package_allocations.distribution_number IS NULL`) — walk-ins not in DCP3 are rejected. This is a deliberate business rule, not a gap to relax.
- "Previously received" (cross-schedule repeat-recipient block) is checked at POS Dokumen (linking time) AND re-checked at POS Penyerahan (completion time) — both checks query `distribution_slots WHERE recipient_person_id=$1 AND status='completed'`.
- New permissions: `distribution.pos_mesin`, `distribution.pos_dokumen`, `distribution.pos_penyerahan` — one per station, granted to `super_admin`. These fully replace `distribution.manage`'s role in gating the old single-flow write actions; `distribution.view`/`documentation.manage` remain for read/media actions that still apply.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1 -p 1` (with `TEST_DATABASE_URL`) must stay green at the end of every task. Two pre-existing, unrelated failures are documented from earlier plans this session (`internal/recipients: TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal`, `internal/web: TestIntegrationLoginDashboardAndLogout`) — not this plan's concern.
- Work happens directly on `main`, no worktree — this session's established pattern.

---

### Task 1: Migration — `distribution_slots`, drop `distribution_records`, nullable `distribution_number`, POS permissions

**Files:**
- Create: `internal/database/migrations/00010_distribution_pos.sql`
- Create: `internal/distribution/schema_integration_test.go` (new file — no such test exists yet for this package; follows the established per-package pool-helper pattern)

**Interfaces:**
- Produces: `distribution_slots(id, schedule_id, slot_number, status, allocation_id, recipient_person_id, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at)`; `documentation_slots.distribution_slot_id` (renamed from `distribution_id`, now FK to `distribution_slots`); `package_allocations.distribution_number` nullable; `documentation_template_slots.stage` CHECK'd to `mesin`/`dokumen`/`penyerahan`; permissions `distribution.pos_mesin`/`pos_dokumen`/`pos_penyerahan`.

- [ ] **Step 1: Write the failing test**

Create `internal/distribution/schema_integration_test.go`:

```go
package distribution

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func distributionIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require konkit_test, got %q", config.ConnConfig.Database)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrationCreatesDistributionSlotsAndSeedsPOSPermissions(t *testing.T) {
	pool := distributionIntegrationPool(t)
	ctx := context.Background()

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'distribution_slots')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected distribution_slots table to exist")
	}
	var recordsExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'distribution_records')`).Scan(&recordsExists); err != nil {
		t.Fatal(err)
	}
	if recordsExists {
		t.Fatal("expected distribution_records table to be dropped")
	}

	var nullable string
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='package_allocations' AND column_name='distribution_number'`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != "YES" {
		t.Fatalf("expected package_allocations.distribution_number to be nullable, got is_nullable=%s", nullable)
	}

	var fkTarget string
	if err := pool.QueryRow(ctx, `
		SELECT ccu.table_name FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu ON tc.constraint_name = ccu.constraint_name
		WHERE tc.table_name = 'documentation_slots' AND tc.constraint_type = 'FOREIGN KEY' AND ccu.table_name != 'documentation_slots'
	`).Scan(&fkTarget); err != nil {
		t.Fatal(err)
	}
	if fkTarget != "distribution_slots" {
		t.Fatalf("expected documentation_slots to FK into distribution_slots, got %q", fkTarget)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('distribution.pos_mesin','distribution.pos_dokumen','distribution.pos_penyerahan')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 POS permissions seeded, got %d", count)
	}

	var superAdminGrants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp
		JOIN roles ON roles.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE roles.code = 'super_admin' AND p.code IN ('distribution.pos_mesin','distribution.pos_dokumen','distribution.pos_penyerahan')
	`).Scan(&superAdminGrants); err != nil {
		t.Fatal(err)
	}
	if superAdminGrants != 3 {
		t.Fatalf("expected super_admin granted all 3 POS permissions, got %d", superAdminGrants)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/distribution/... -run TestMigrationCreatesDistributionSlotsAndSeedsPOSPermissions -v`
Expected: FAIL — `distribution_slots` doesn't exist yet.

- [ ] **Step 3: Write the migration**

Create `internal/database/migrations/00010_distribution_pos.sql`:

```sql
-- +goose Up
CREATE TABLE distribution_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id),
    slot_number integer NOT NULL CHECK (slot_number > 0),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'linked', 'completed', 'cancelled')),
    allocation_id uuid REFERENCES package_allocations(id),
    recipient_person_id uuid REFERENCES people(id),
    machine_option_code text,
    machine_serial_number text,
    hose_option_code text,
    hose_serial_number text,
    converter_serial_number text,
    verification_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    distributed_at timestamptz,
    distributed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, slot_number),
    UNIQUE (allocation_id)
);
CREATE INDEX distribution_slots_recipient_idx ON distribution_slots (recipient_person_id, status, completed_at DESC);

INSERT INTO distribution_slots (id, schedule_id, slot_number, status, allocation_id, recipient_person_id, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at)
SELECT dr.id, pa.schedule_id, pa.distribution_number,
    CASE dr.status WHEN 'completed' THEN 'completed' WHEN 'cancelled' THEN 'cancelled' ELSE 'linked' END,
    pa.id, dr.recipient_person_id, dr.machine_option_code, dr.machine_serial_number, dr.hose_option_code, dr.hose_serial_number, dr.converter_serial_number,
    dr.verification_snapshot_json, dr.distributed_at, dr.distributed_by, dr.completed_at, dr.created_at, dr.updated_at
FROM distribution_records dr
JOIN package_allocations pa ON pa.id = dr.allocation_id;

ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_distribution_id_fkey;
ALTER TABLE documentation_slots RENAME COLUMN distribution_id TO distribution_slot_id;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_distribution_slot_id_fkey FOREIGN KEY (distribution_slot_id) REFERENCES distribution_slots(id) ON DELETE CASCADE;

DROP TABLE distribution_records;

ALTER TABLE package_allocations ALTER COLUMN distribution_number DROP NOT NULL;

ALTER TABLE documentation_template_slots ADD CONSTRAINT documentation_template_slots_stage_check CHECK (stage IN ('mesin', 'dokumen', 'penyerahan'));
UPDATE documentation_template_slots SET stage = 'penyerahan' WHERE stage NOT IN ('mesin', 'dokumen', 'penyerahan');
ALTER TABLE documentation_template_slots ALTER COLUMN stage DROP DEFAULT;

INSERT INTO permissions(code, name, description) VALUES
    ('distribution.pos_mesin', 'POS Mesin', 'Menandai unit mesin dengan nomor bagi dan mengunggah bukti mesin'),
    ('distribution.pos_dokumen', 'POS Dokumen', 'Mengaitkan NIK penerima ke nomor bagi dan melengkapi data identitas'),
    ('distribution.pos_penyerahan', 'POS Penyerahan', 'Menyelesaikan serah terima dan mengunggah bukti penyerahan')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code IN ('distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan');

ALTER TABLE documentation_template_slots ALTER COLUMN stage SET DEFAULT 'distribution';
ALTER TABLE documentation_template_slots DROP CONSTRAINT documentation_template_slots_stage_check;

ALTER TABLE package_allocations ALTER COLUMN distribution_number SET NOT NULL;

CREATE TABLE distribution_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    allocation_id uuid NOT NULL UNIQUE REFERENCES package_allocations(id),
    recipient_person_id uuid REFERENCES people(id),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed', 'cancelled')),
    machine_option_code text,
    machine_serial_number text,
    hose_option_code text,
    hose_serial_number text,
    converter_serial_number text,
    verification_snapshot_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    distributed_at timestamptz,
    distributed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX distribution_records_recipient_idx ON distribution_records (recipient_person_id, status, completed_at DESC);

INSERT INTO distribution_records (id, allocation_id, recipient_person_id, status, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at)
SELECT id, allocation_id, recipient_person_id,
    CASE status WHEN 'completed' THEN 'completed' WHEN 'cancelled' THEN 'cancelled' ELSE 'draft' END,
    machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number,
    verification_snapshot_json, distributed_at, distributed_by, completed_at, created_at, updated_at
FROM distribution_slots WHERE allocation_id IS NOT NULL;

ALTER TABLE documentation_slots DROP CONSTRAINT documentation_slots_distribution_slot_id_fkey;
ALTER TABLE documentation_slots RENAME COLUMN distribution_slot_id TO distribution_id;
ALTER TABLE documentation_slots ADD CONSTRAINT documentation_slots_distribution_id_fkey FOREIGN KEY (distribution_id) REFERENCES distribution_records(id) ON DELETE CASCADE;

DROP TABLE distribution_slots;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/distribution/... -v`
Expected: new test passes. Existing `internal/distribution` tests that reference `distribution_records`/old columns will now FAIL to compile — this is expected and addressed starting Task 4; do not fix them in this task.

- [ ] **Step 5: Verify build**

Run: `cd "d:/KSM/Deployment/konkit" && go vet ./internal/database/... ./internal/distribution/...`
Expected: clean (the rest of the module will fail to build until Task 2/4 land — that's expected mid-plan breakage, not a Task 1 defect; do not run whole-module build/test yet).

- [ ] **Step 6: Commit**

```bash
git add internal/database/migrations/00010_distribution_pos.sql internal/distribution/schema_integration_test.go
git commit -m "feat(distribution): add distribution_slots table, migrate distribution_records data, seed POS permissions"
```

---

### Task 2: DCP3 import — stop assigning distribution numbers

**Files:**
- Modify: `internal/dcp3/repository.go`
- Modify: `internal/dcp3/repository_integration_test.go` (or wherever DCP3's integration tests for import live — read the file first to find the exact test names asserting on `distribution_number`/`distribution_records`/`documentation_slots` creation, and update them to match the new behavior)

**Interfaces:**
- Consumes: `distribution_slots` (Task 1) for the "previously received" check.
- Produces: DCP3 import no longer creates `distribution_records`/`documentation_slots`/a `distribution_number` — `package_allocations` rows are created with `distribution_number` left NULL.

- [ ] **Step 1: Read the current file and its test**

Read `internal/dcp3/repository.go` in full (the import loop is around lines 160-234, `nextDistributionNumber` around 314-329, per this plan's research) and `internal/dcp3/repository_integration_test.go` (or equivalent) to find every assertion touching `distribution_number`, `distribution_records`, or `documentation_slots` counts after an import.

- [ ] **Step 2: Update the failing/affected tests first**

For every test assertion that currently checks `package_allocations.distribution_number` was assigned a specific value after import, change it to assert the value is NULL instead. For every assertion that checks a `distribution_records` or `documentation_slots` row was created as a side effect of import, remove that assertion (or replace it with an assertion that NO such row exists yet, if the test's intent was "import creates a fully-formed candidate" — the new intent is "import creates a candidate with data, no slot"). Keep every assertion about `candidate_nominations`/`package_allocations`/`people` fields unrelated to numbering/documentation unchanged.

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/dcp3/... -v`
Expected: FAIL — the import code still assigns a number / creates the old rows, contradicting the updated assertions.

- [ ] **Step 4: Implement**

In `internal/dcp3/repository.go`, in the import loop, replace:

```go
distributionNumber, err := nextDistributionNumber(ctx, tx, scheduleID, normalized.SourceSequenceNumber)
if err != nil {
	return ImportResult{}, err
}
var allocationID string
if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6) RETURNING id::text`, scheduleID, nominationID, personID, distributionNumber, allocationStatus, packageSnapshot).Scan(&allocationID); err != nil {
	return ImportResult{}, fmt.Errorf("insert package allocation: %w", err)
}
var distributionID string
if err := tx.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id) VALUES($1,NULLIF($2,'')::uuid) RETURNING id::text`, allocationID, personID).Scan(&distributionID); err != nil {
	return ImportResult{}, fmt.Errorf("insert distribution draft: %w", err)
}
_, err = tx.Exec(ctx, `
	INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
	SELECT $1,slot_code,label,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order
	FROM documentation_template_slots WHERE template_version_id=$2
`, distributionID, documentationTemplateID)
if err != nil {
	return ImportResult{}, fmt.Errorf("snapshot documentation slots: %w", err)
}
```

with:

```go
var allocationID string
if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5) RETURNING id::text`, scheduleID, nominationID, personID, allocationStatus, packageSnapshot).Scan(&allocationID); err != nil {
	return ImportResult{}, fmt.Errorf("insert package allocation: %w", err)
}
```

Remove the now-unused `nextDistributionNumber` function entirely (lines ~314-329) and its `documentationTemplateID` variable if it's no longer referenced anywhere else in the file (check — `documentationTemplateID` was only used by the removed snippet's second query; if the surrounding function signature computed it solely for this purpose, remove that too, otherwise leave it if used elsewhere for validation).

Change the "previously received" check (around line 182) from:

```go
if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_records WHERE recipient_person_id=$1 AND status='completed')`, personID).Scan(&previouslyReceived); err != nil {
```

to:

```go
if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed')`, personID).Scan(&previouslyReceived); err != nil {
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/dcp3/... -v`
Expected: PASS.

- [ ] **Step 6: Verify build**

Run: `cd "d:/KSM/Deployment/konkit" && go vet ./internal/dcp3/...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/dcp3/repository.go internal/dcp3/repository_integration_test.go
git commit -m "feat(dcp3): stop assigning distribution numbers and creating distribution slots at import time"
```

---

### Task 3: Nullable `distribution_number` in Recipients, Reports, and their frontend types

**Files:**
- Modify: `internal/recipients/models.go`
- Modify: `internal/recipients/repository.go`
- Modify: `internal/reports/models.go`
- Modify: `internal/reports/repository.go`
- Modify: `frontend/src/features/dashboard/types.ts`
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx`

**Interfaces:**
- Produces: `recipients.Recipient.DistributionNumber *int` (was `int`), `reports.Row.DistributionNumber *int` (was `int`), frontend `distribution_number: number | null`.

- [ ] **Step 1: Update Go structs and scans**

In `internal/recipients/models.go`, change the `Recipient` struct's field:

```go
DistributionNumber *int `json:"distribution_number"`
```

In `internal/recipients/repository.go`, find the `Scan(...)` call that populates `&item.DistributionNumber` (from `pa.distribution_number` in `recipientSelect`) — it already works correctly once the struct field is `*int` (pgx scans SQL NULL into a nil `*int` automatically); no SQL change needed.

In `internal/reports/models.go`, change the `Row` struct's field:

```go
DistributionNumber *int `json:"distribution_number"`
```

In `internal/reports/repository.go`, same as above — the scan target becomes `&row.DistributionNumber`; confirm no other code in this file dereferences `row.DistributionNumber` as a plain int (e.g. in a `fmt.Sprintf` for Excel/PDF export) — if it does, guard it: replace direct interpolation with a helper that renders `"-"` for nil, e.g. wherever the export builds a display string for this column.

- [ ] **Step 2: Update frontend types and rendering**

In `frontend/src/features/dashboard/types.ts`, change:

```ts
distribution_number: number;
```

to:

```ts
distribution_number: number | null;
```

In `frontend/src/features/dashboard/DashboardPage.tsx`, change the cell render (currently `<td className={...}>{item.distribution_number}</td>`) to guard the null case:

```tsx
<td className={`${stickyNumber} font-semibold tabular-nums`}>{item.distribution_number ?? '-'}</td>
```

- [ ] **Step 3: Run tests**

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/recipients/... ./internal/reports/... -v`
Expected: existing tests that assert an exact `DistributionNumber` value now compare against a pointer — read each failing assertion and update it to dereference or compare via a helper (e.g. `*item.DistributionNumber == 3` guarded by a nil check, or construct expected values as `intPtr(3)` if a local test helper for that doesn't already exist, in which case add one: `func intPtr(v int) *int { return &v }`). Fix each one; do not weaken what it verifies.

Run: `cd "d:/KSM/Deployment/konkit/frontend" && npx vitest run src/features/dashboard && npx tsc --noEmit`
Expected: PASS / clean. Fix any test that asserts an exact numeric `distribution_number` on a fixture the same way (wrap the fixture value, e.g. `distribution_number: 3` still works fine as a fixture — the type change only requires updating assertions that expect the exact TS type `number`, e.g. a fixture explicitly typed to require non-null; adjust as needed).

- [ ] **Step 4: Commit**

```bash
git add internal/recipients/models.go internal/recipients/repository.go internal/reports/models.go internal/reports/repository.go frontend/src/features/dashboard/types.ts frontend/src/features/dashboard/DashboardPage.tsx
git commit -m "feat(recipients,reports): handle nullable distribution_number now that POS Mesin assigns it later"
```

(Also stage and commit any test files you had to touch in Step 3, in the same commit.)

---

### Task 4: `internal/distribution` — rewrite models.go for the 3-POS shape

**Files:**
- Modify: `internal/distribution/models.go`

**Interfaces:**
- Produces: `DistributionSlot`, `CreateSlotInput`, `CandidateMatch`, `LinkSlotInput`, `CompleteSlotInput` types; new sentinel errors — consumed by Tasks 5-8.
- This task ONLY changes types/errors. It will not compile standalone against `service.go`/`repository.go` (Task 5-8's job) — that's expected; do not attempt to make the whole package build after this task alone.

- [ ] **Step 1: Read the current file in full**

Read `internal/distribution/models.go` completely to see every existing type verbatim before editing (this plan's research already has the field lists; confirm nothing drifted before you edit).

- [ ] **Step 2: Replace the allocation/workspace/draft types**

Remove `SearchRecord`, `SearchResult`, `RecipientWorkspace`, `DraftInput`, `ReceiptHistory`, `DistributionRecord` (the old flat single-flow types) — these are fully superseded by the POS-shaped types below. Keep `SlotSummary`, `MediaSlot`, `MediaFileInput`, `MediaFile` unchanged (media handling is still per-`documentation_slots`-row, just re-scoped to `distribution_slots` in Task 8 — no type shape change needed here).

Add:

```go
type DistributionSlot struct {
	ID                    string        `json:"id"`
	ScheduleID            string        `json:"schedule_id"`
	SlotNumber            int           `json:"slot_number"`
	Status                string        `json:"status"`
	AllocationID          *string       `json:"allocation_id,omitempty"`
	FullName              string        `json:"full_name,omitempty"`
	NIK                   string        `json:"nik,omitempty"`
	MachineOptionCode     string        `json:"machine_option_code,omitempty"`
	MachineSerialNumber   string        `json:"machine_serial_number,omitempty"`
	HoseOptionCode        string        `json:"hose_option_code,omitempty"`
	HoseSerialNumber      string        `json:"hose_serial_number,omitempty"`
	ConverterSerialNumber string        `json:"converter_serial_number,omitempty"`
	Documentation         []SlotSummary `json:"documentation"`
	DistributedAt         *time.Time    `json:"distributed_at,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

type CreateSlotInput struct {
	ScheduleID             string `json:"schedule_id"`
	MachineOptionCode      string `json:"machine_option_code"`
	MachineSerialNumber    string `json:"machine_serial_number"`
	HoseOptionCode         string `json:"hose_option_code"`
	HoseSerialNumber       string `json:"hose_serial_number"`
	ConverterSerialNumber  string `json:"converter_serial_number"`
}

type CandidateMatch struct {
	AllocationID         string `json:"allocation_id"`
	FullName             string `json:"full_name"`
	NIK                  string `json:"nik"`
	SectorIdentifier     string `json:"sector_identifier"`
	SectorIdentifierType string `json:"sector_identifier_type"`
	Address              string `json:"address"`
	Village              string `json:"village"`
	District             string `json:"district"`
	PhoneNumber          string `json:"phone_number"`
	ProgramType          string `json:"program_type"`
}

type LinkSlotInput struct {
	ScheduleID           string `json:"schedule_id"`
	SlotNumber           int    `json:"slot_number"`
	NIK                  string `json:"nik"`
	Address              string `json:"address"`
	Village              string `json:"village"`
	District             string `json:"district"`
	PhoneNumber          string `json:"phone_number"`
	SectorIdentifier     string `json:"sector_identifier"`
	IdentityChangeReason string `json:"identity_change_reason"`
}

type CompleteSlotInput struct {
	ScheduleID string `json:"schedule_id"`
	SlotNumber int    `json:"slot_number"`
}
```

Add new sentinel errors alongside the existing ones. Keep these unchanged: `ErrScheduleRequired`, `ErrQueryRequired` (Task 7's `SearchLinkedSlot` needs it), `ErrNIKInvalid`, `ErrIdentityChangeReasonRequired`, `ErrIdentifierConflict`, `ErrMediaUnavailable`, `ErrMediaNotFound`, `ErrMediaTypeInvalid`, `ErrMediaTooLarge`, `ErrMediaSourceInvalid`, `ErrMediaLocationRequired`, `ErrMediaCapturedAtRequired`, `ErrMediaLimitReached`, `ErrIdentityIncomplete`, `ErrDocumentationIncomplete`, `ErrPreviouslyReceived`, `ErrAlreadyCompleted`. Remove `ErrQueryTooShort` and `ErrAllocationNotFound` — nothing in Tasks 5-9 uses either. Add:

```go
var (
	ErrSlotNotFound       = errors.New("distribution slot not found")
	ErrSlotNotOpen        = errors.New("distribution slot is not open")
	ErrSlotNotLinked      = errors.New("distribution slot is not linked to a recipient")
	ErrCandidateNotFound  = errors.New("no unlinked DCP3 candidate matches this NIK for this schedule")
	ErrSlotNumberRequired = errors.New("slot_number is required")
)
```

- [ ] **Step 3: Verify it's syntactically valid Go on its own**

Run: `cd "d:/KSM/Deployment/konkit" && gofmt -l internal/distribution/models.go`
Expected: no output (the file itself is syntactically valid, even though the package as a whole won't build until later tasks touch `service.go`/`repository.go`).

- [ ] **Step 4: Commit**

```bash
git add internal/distribution/models.go
git commit -m "feat(distribution): replace single-flow models with DistributionSlot/POS types"
```

---

### Task 5: `internal/distribution` — POS Mesin (create slot)

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Create: `internal/distribution/pos_mesin_test.go`

**Interfaces:**
- Consumes: `DistributionSlot`, `CreateSlotInput`, `ErrSlotNumberRequired` (Task 4).
- Produces: `(*Repository) CreateSlot(ctx, actor, input CreateSlotInput, meta) (DistributionSlot, error)`, `(*Service) CreateSlot(...)` — consumed by Task 9 (API routes).

This task will not make the whole `internal/distribution` package build — `Search`/`GetWorkspace`/`SaveDraft`/`Complete`/media methods referencing removed types still exist in the current `service.go`/`repository.go` and are replaced across Tasks 5-8. Scope this task's edits to ADDING the new `CreateSlot` methods without yet deleting the old ones; Task 8 does the final cleanup pass once all 3 POS are in place. Confirm this by running `go build ./internal/distribution/...` and expect it to still fail with errors ONLY in the old, not-yet-removed code — not in anything this task added.

- [ ] **Step 1: Write the failing test**

Create `internal/distribution/pos_mesin_test.go` (unit test with a repository stub):

```go
package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type mesinRepositoryStub struct {
	created  DistributionSlot
	createErr error
	seenInput CreateSlotInput
}

func (r *mesinRepositoryStub) CreateSlot(_ context.Context, _ auth.Principal, input CreateSlotInput, _ auth.ClientMeta) (DistributionSlot, error) {
	r.seenInput = input
	return r.created, r.createErr
}

func TestCreateSlotRequiresScheduleID(t *testing.T) {
	service := &Service{posMesinRepository: &mesinRepositoryStub{}}
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{}, auth.ClientMeta{})
	if !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateSlotTrimsEquipmentFieldsAndDelegates(t *testing.T) {
	repo := &mesinRepositoryStub{created: DistributionSlot{ID: "slot-1", SlotNumber: 1, Status: "open"}}
	service := &Service{posMesinRepository: repo}
	result, err := service.CreateSlot(context.Background(), auth.Principal{}, CreateSlotInput{
		ScheduleID: "schedule-1", MachineSerialNumber: "  MS-001  ", HoseSerialNumber: " HS-001 ", ConverterSerialNumber: " CV-001 ",
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SlotNumber != 1 || result.Status != "open" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenInput.MachineSerialNumber != "MS-001" || repo.seenInput.HoseSerialNumber != "HS-001" || repo.seenInput.ConverterSerialNumber != "CV-001" {
		t.Fatalf("seenInput not trimmed: %+v", repo.seenInput)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestCreateSlot -v`
Expected: FAIL — compile error, `Service.posMesinRepository`/`CreateSlot` don't exist yet.

- [ ] **Step 3: Implement**

In `internal/distribution/repository.go`, add:

```go
func (r *Repository) CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin create slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var documentationTemplateID string
	if err := tx.QueryRow(ctx, `SELECT documentation_template_version_id FROM program_schedules WHERE id=$1 FOR UPDATE`, input.ScheduleID).Scan(&documentationTemplateID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrScheduleRequired
		}
		return DistributionSlot{}, fmt.Errorf("lock schedule: %w", err)
	}

	var nextNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(slot_number),0)+1 FROM distribution_slots WHERE schedule_id=$1`, input.ScheduleID).Scan(&nextNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("allocate slot number: %w", err)
	}

	var slotID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO distribution_slots (schedule_id, slot_number, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''))
		RETURNING id::text
	`, input.ScheduleID, nextNumber, input.MachineOptionCode, input.MachineSerialNumber, input.HoseOptionCode, input.HoseSerialNumber, input.ConverterSerialNumber).Scan(&slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("insert distribution slot: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		SELECT $1,slot_code,label,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order
		FROM documentation_template_slots WHERE template_version_id=$2
	`, slotID, documentationTemplateID); err != nil {
		return DistributionSlot{}, fmt.Errorf("snapshot documentation slots: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_created", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"schedule_id": input.ScheduleID, "slot_number": nextNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit create slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) getSlotByID(ctx context.Context, id string) (DistributionSlot, error) {
	var slot DistributionSlot
	var machineOption, machineSerial, hoseOption, hoseSerial, converterSerial *string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, schedule_id::text, slot_number, status, allocation_id::text, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, distributed_at, created_at, updated_at
		FROM distribution_slots WHERE id=$1
	`, id).Scan(&slot.ID, &slot.ScheduleID, &slot.SlotNumber, &slot.Status, &slot.AllocationID, &machineOption, &machineSerial, &hoseOption, &hoseSerial, &converterSerial, &slot.DistributedAt, &slot.CreatedAt, &slot.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("get distribution slot: %w", err)
	}
	if machineOption != nil {
		slot.MachineOptionCode = *machineOption
	}
	if machineSerial != nil {
		slot.MachineSerialNumber = *machineSerial
	}
	if hoseOption != nil {
		slot.HoseOptionCode = *hoseOption
	}
	if hoseSerial != nil {
		slot.HoseSerialNumber = *hoseSerial
	}
	if converterSerial != nil {
		slot.ConverterSerialNumber = *converterSerial
	}
	slot.Documentation, err = r.listSlotDocumentation(ctx, id)
	if err != nil {
		return DistributionSlot{}, err
	}
	return slot, nil
}

func (r *Repository) listSlotDocumentation(ctx context.Context, distributionSlotID string) ([]SlotSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_code, ds.label_snapshot, ds.status, ds.is_required, ds.min_files, ds.max_files, ds.input_source, ds.require_location, ds.require_captured_at
		FROM documentation_slots ds WHERE ds.distribution_slot_id=$1 ORDER BY ds.sort_order
	`, distributionSlotID)
	if err != nil {
		return nil, fmt.Errorf("list slot documentation: %w", err)
	}
	defer rows.Close()
	var summaries []SlotSummary
	for rows.Next() {
		var summary SlotSummary
		if err := rows.Scan(&summary.ID, &summary.Code, &summary.Label, &summary.Status, &summary.Required, &summary.MinFiles, &summary.MaxFiles, &summary.InputSource, &summary.RequireLocation, &summary.RequireCapturedAt); err != nil {
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

`listMediaFiles` doesn't exist yet under that name — Task 8 introduces the shared media-listing helper this calls into; for THIS task, stub it minimally so the file compiles in isolation:

```go
func (r *Repository) listMediaFiles(ctx context.Context, documentationSlotID string) ([]MediaFile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, documentation_slot_id::text, storage_key::text, original_filename, mime_type, byte_size, source, captured_at, latitude, longitude, status, uploaded_at
		FROM media_files WHERE documentation_slot_id=$1 AND status='accepted' ORDER BY uploaded_at
	`, documentationSlotID)
	if err != nil {
		return nil, fmt.Errorf("list media files: %w", err)
	}
	defer rows.Close()
	var files []MediaFile
	for rows.Next() {
		var file MediaFile
		if err := rows.Scan(&file.ID, &file.SlotID, &file.StorageKey, &file.OriginalFilename, &file.MimeType, &file.ByteSize, &file.Source, &file.CapturedAt, &file.Latitude, &file.Longitude, &file.Status, &file.UploadedAt); err != nil {
			return nil, fmt.Errorf("scan media file: %w", err)
		}
		file.ContentURL = "/api/v1/distribution/media/" + file.ID + "/content"
		files = append(files, file)
	}
	return files, rows.Err()
}
```

In `internal/distribution/service.go`, add a `posMesinRepository` interface field to `Service` and the `CreateSlot` method:

```go
type posMesinRepository interface {
	CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error)
}
```

Add `posMesinRepository posMesinRepository` to the `Service` struct, and wire it from `repository` (the same `*Repository` satisfies it) inside `NewService`, matching the existing pattern where `mediaRepository`/`completionRepository` are type-asserted from the same concrete `repository` — add `service.posMesinRepository, _ = repository.(posMesinRepository)` alongside the existing assertions in `NewService`.

```go
func (s *Service) CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	input.MachineOptionCode = strings.TrimSpace(input.MachineOptionCode)
	input.MachineSerialNumber = strings.TrimSpace(input.MachineSerialNumber)
	input.HoseOptionCode = strings.TrimSpace(input.HoseOptionCode)
	input.HoseSerialNumber = strings.TrimSpace(input.HoseSerialNumber)
	input.ConverterSerialNumber = strings.TrimSpace(input.ConverterSerialNumber)
	if s.posMesinRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Mesin is unavailable")
	}
	return s.posMesinRepository.CreateSlot(ctx, actor, input, meta)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestCreateSlot -v`
Expected: PASS (the whole package still won't build due to old code Tasks 6-8 remove — that's expected; this targeted `-run` invocation only needs the files it touches to compile together, which they do once `models.go`/this task's additions are in place. If Go's build unit forces whole-package compilation and this genuinely blocks even a scoped `-run`, that confirms the old `service.go`/`repository.go` code must be provisionally commented out or stubbed to unblock — if you hit this, remove the dead `Search`/`GetWorkspace`/`SaveDraft`/`Complete`/old media methods now instead of waiting for Task 8, and note this deviation in your report).

- [ ] **Step 5: Commit**

```bash
git add internal/distribution/repository.go internal/distribution/service.go internal/distribution/pos_mesin_test.go
git commit -m "feat(distribution): add POS Mesin CreateSlot (repository + service)"
```

---

### Task 6: `internal/distribution` — POS Dokumen (search candidate + link slot)

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Create: `internal/distribution/pos_dokumen_test.go`

**Interfaces:**
- Consumes: `CandidateMatch`, `LinkSlotInput`, `ErrCandidateNotFound`, `ErrSlotNotOpen`, `ErrPreviouslyReceived` (Task 4); `getSlotByID` (Task 5).
- Produces: `(*Repository) SearchCandidate(ctx, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error)`, `(*Repository) LinkSlot(ctx, actor, input LinkSlotInput, meta, scope) (DistributionSlot, error)` — consumed by Task 9.

- [ ] **Step 1: Write the failing tests**

Create `internal/distribution/pos_dokumen_test.go`:

```go
package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type dokumenRepositoryStub struct {
	candidate    CandidateMatch
	candidateErr error
	linked       DistributionSlot
	linkErr      error
	seenLink     LinkSlotInput
}

func (r *dokumenRepositoryStub) SearchCandidate(_ context.Context, _, _ string, _ auth.RegencyScope) (CandidateMatch, error) {
	return r.candidate, r.candidateErr
}
func (r *dokumenRepositoryStub) LinkSlot(_ context.Context, _ auth.Principal, input LinkSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	r.seenLink = input
	return r.linked, r.linkErr
}

func TestLinkSlotRequiresValidNIK(t *testing.T) {
	service := &Service{posDokumenRepository: &dokumenRepositoryStub{}}
	_, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{ScheduleID: "s1", SlotNumber: 1, NIK: "123"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkSlotRequiresSlotNumber(t *testing.T) {
	service := &Service{posDokumenRepository: &dokumenRepositoryStub{}}
	_, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{ScheduleID: "s1", NIK: "1234567890123456"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSlotNumberRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkSlotDelegatesValidInput(t *testing.T) {
	repo := &dokumenRepositoryStub{linked: DistributionSlot{ID: "slot-1", SlotNumber: 5, Status: "linked"}}
	service := &Service{posDokumenRepository: repo}
	result, err := service.LinkSlot(context.Background(), auth.Principal{}, LinkSlotInput{
		ScheduleID: "s1", SlotNumber: 5, NIK: "1234567890123456", Address: "  Jl. A  ",
	}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "linked" {
		t.Fatalf("result = %+v", result)
	}
	if repo.seenLink.Address != "Jl. A" {
		t.Fatalf("address not trimmed: %q", repo.seenLink.Address)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestLinkSlot -v`
Expected: FAIL — compile error, `Service.posDokumenRepository`/`LinkSlot` don't exist yet.

- [ ] **Step 3: Implement**

In `internal/distribution/repository.go`, add:

```go
func (r *Repository) SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error) {
	var match CandidateMatch
	err := r.pool.QueryRow(ctx, `
		SELECT pa.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
			COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''), cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		LEFT JOIN LATERAL (SELECT identifier_type, normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE pa.schedule_id = $1 AND p.nik = $2 AND pa.distribution_number IS NULL
			AND ($3 OR ps.regency_id::text = ANY($4))
	`, scheduleID, nik, scope.Unrestricted, scope.RegencyIDs).Scan(&match.AllocationID, &match.FullName, &match.NIK, &match.SectorIdentifierType, &match.SectorIdentifier, &match.Address, &match.Village, &match.District, &match.PhoneNumber, &match.ProgramType)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateMatch{}, ErrCandidateNotFound
	}
	if err != nil {
		return CandidateMatch{}, fmt.Errorf("search candidate: %w", err)
	}
	return match, nil
}

func (r *Repository) LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin link slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, slotStatus string
	if err := tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &slotStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrSlotNotFound
		}
		return DistributionSlot{}, fmt.Errorf("lock distribution slot: %w", err)
	}
	if slotStatus != "open" {
		return DistributionSlot{}, ErrSlotNotOpen
	}

	var allocationID, personID, programType string
	if err := tx.QueryRow(ctx, `
		SELECT pa.id::text, p.id::text, cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN people p ON p.id = cn.person_id
		WHERE pa.schedule_id=$1 AND p.nik=$2 AND pa.distribution_number IS NULL
		FOR UPDATE OF pa
	`, input.ScheduleID, input.NIK).Scan(&allocationID, &personID, &programType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrCandidateNotFound
		}
		return DistributionSlot{}, fmt.Errorf("lock candidate: %w", err)
	}

	var previouslyReceived bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed')`, personID).Scan(&previouslyReceived); err != nil {
		return DistributionSlot{}, fmt.Errorf("check previously received: %w", err)
	}
	if previouslyReceived {
		return DistributionSlot{}, ErrPreviouslyReceived
	}

	if _, err := tx.Exec(ctx, `UPDATE people SET address=COALESCE(NULLIF($2,''),address), village=COALESCE(NULLIF($3,''),village), district=COALESCE(NULLIF($4,''),district), phone_number=COALESCE(NULLIF($5,''),phone_number), updated_at=now() WHERE id=$1`, personID, input.Address, input.Village, input.District, input.PhoneNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("update person: %w", err)
	}
	if input.SectorIdentifier != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)
			ON CONFLICT (person_id,identifier_type) DO UPDATE SET normalized_value=EXCLUDED.normalized_value, display_value=EXCLUDED.display_value, updated_at=now()
		`, personID, identifierType, input.SectorIdentifier); err != nil {
			if code, ok := uniqueViolationConstraint(err); ok && code != "" {
				return DistributionSlot{}, ErrIdentifierConflict
			}
			return DistributionSlot{}, fmt.Errorf("upsert sector identifier: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=$2, status='ready', updated_at=now() WHERE id=$1`, allocationID, input.SlotNumber); err != nil {
		if code, ok := uniqueViolationConstraint(err); ok && code != "" {
			return DistributionSlot{}, ErrIdentifierConflict
		}
		return DistributionSlot{}, fmt.Errorf("update package allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id=$2, recipient_person_id=$3, status='linked', updated_at=now() WHERE id=$1`, slotID, allocationID, personID); err != nil {
		return DistributionSlot{}, fmt.Errorf("link distribution slot: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_linked", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": allocationID, "slot_number": input.SlotNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit link slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}
```

`uniqueViolationConstraint` doesn't exist in this package yet — add it (mirroring `internal/recipients/repository.go`'s helper of the same name):

```go
func uniqueViolationConstraint(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
```

(Add `"github.com/jackc/pgx/v5/pgconn"` to the import block if not already present — check first, `internal/distribution/repository.go` may already import it for other error handling.)

In `internal/distribution/service.go`, add the interface, struct field, wiring, and two methods:

```go
type posDokumenRepository interface {
	SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error)
	LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
}
```

Add `posDokumenRepository posDokumenRepository` to `Service`, wire via `service.posDokumenRepository, _ = repository.(posDokumenRepository)` in `NewService`.

```go
func (s *Service) SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error) {
	scheduleID, nik = strings.TrimSpace(scheduleID), stripNonDigits.ReplaceAllString(nik, "")
	if scheduleID == "" {
		return CandidateMatch{}, ErrScheduleRequired
	}
	if len(nik) != 16 {
		return CandidateMatch{}, ErrNIKInvalid
	}
	if s.posDokumenRepository == nil {
		return CandidateMatch{}, errors.New("distribution POS Dokumen is unavailable")
	}
	return s.posDokumenRepository.SearchCandidate(ctx, scheduleID, nik, scope)
}

func (s *Service) LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber <= 0 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	input.NIK = stripNonDigits.ReplaceAllString(input.NIK, "")
	if len(input.NIK) != 16 {
		return DistributionSlot{}, ErrNIKInvalid
	}
	input.Address = strings.TrimSpace(input.Address)
	input.Village = strings.TrimSpace(input.Village)
	input.District = strings.TrimSpace(input.District)
	input.PhoneNumber = stripNonDigits.ReplaceAllString(input.PhoneNumber, "")
	input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)
	input.IdentityChangeReason = strings.TrimSpace(input.IdentityChangeReason)
	if s.posDokumenRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Dokumen is unavailable")
	}
	return s.posDokumenRepository.LinkSlot(ctx, actor, input, meta, scope)
}
```

(`stripNonDigits`/`normalizeIdentifier` already exist in this package per the existing `service.go` — reuse them, don't redefine.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestLinkSlot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/distribution/repository.go internal/distribution/service.go internal/distribution/pos_dokumen_test.go
git commit -m "feat(distribution): add POS Dokumen SearchCandidate and LinkSlot"
```

---

### Task 7: `internal/distribution` — POS Penyerahan (search linked slots + complete)

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Create: `internal/distribution/pos_penyerahan_test.go`

**Interfaces:**
- Consumes: `CompleteSlotInput`, `ErrSlotNotLinked`, `ErrAlreadyCompleted`, `ErrIdentityIncomplete`, `ErrDocumentationIncomplete`, `ErrPreviouslyReceived` (Task 4).
- Produces: `(*Repository) SearchLinkedSlot(ctx, scheduleID, query string, scope) (DistributionSlot, error)`, `(*Repository) CompleteSlot(ctx, actor, input CompleteSlotInput, meta, scope) (DistributionSlot, error)` — consumed by Task 9.

- [ ] **Step 1: Write the failing tests**

Create `internal/distribution/pos_penyerahan_test.go`:

```go
package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type penyerahanRepositoryStub struct {
	found       DistributionSlot
	searchErr   error
	completed   DistributionSlot
	completeErr error
}

func (r *penyerahanRepositoryStub) SearchLinkedSlot(_ context.Context, _, _ string, _ auth.RegencyScope) (DistributionSlot, error) {
	return r.found, r.searchErr
}
func (r *penyerahanRepositoryStub) CompleteSlot(_ context.Context, _ auth.Principal, _ CompleteSlotInput, _ auth.ClientMeta, _ auth.RegencyScope) (DistributionSlot, error) {
	return r.completed, r.completeErr
}

func TestCompleteSlotRequiresSlotNumber(t *testing.T) {
	service := &Service{posPenyerahanRepository: &penyerahanRepositoryStub{}}
	_, err := service.CompleteSlot(context.Background(), auth.Principal{}, CompleteSlotInput{ScheduleID: "s1"}, auth.ClientMeta{}, auth.RegencyScope{})
	if !errors.Is(err, ErrSlotNumberRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompleteSlotDelegatesValidInput(t *testing.T) {
	repo := &penyerahanRepositoryStub{completed: DistributionSlot{ID: "slot-1", Status: "completed"}}
	service := &Service{posPenyerahanRepository: repo}
	result, err := service.CompleteSlot(context.Background(), auth.Principal{}, CompleteSlotInput{ScheduleID: "s1", SlotNumber: 1}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestCompleteSlot -v`
Expected: FAIL — compile error.

- [ ] **Step 3: Implement**

In `internal/distribution/repository.go`, add:

```go
func (r *Repository) SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ds.status='linked' AND ($4 OR ps.regency_id::text = ANY($5))
			AND (ds.slot_number::text = $2 OR (p.nik IS NOT NULL AND p.nik = NULLIF($3,'')))
		LIMIT 1
	`, scheduleID, query, digits, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("search linked slot: %w", err)
	}
	return r.getSlotByID(ctx, id)
}

func (r *Repository) CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin complete slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status, allocationID, personID string
	var fullName, nik, sectorIdentifier string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, pa.id::text, p.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.normalized_value,'')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN package_allocations pa ON pa.id = ds.allocation_id
		JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN LATERAL (SELECT normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds, pa, p
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &allocationID, &personID, &fullName, &nik, &sectorIdentifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock distribution slot: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if status != "linked" {
		return DistributionSlot{}, ErrSlotNotLinked
	}
	if strings.TrimSpace(fullName) == "" || len(nik) != 16 || sectorIdentifier == "" {
		return DistributionSlot{}, ErrIdentityIncomplete
	}

	var previouslyReceived bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed' AND id<>$2)`, personID, slotID).Scan(&previouslyReceived); err != nil {
		return DistributionSlot{}, fmt.Errorf("check previously received: %w", err)
	}
	if previouslyReceived {
		return DistributionSlot{}, ErrPreviouslyReceived
	}

	var incomplete bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM documentation_slots ds
			LEFT JOIN (SELECT documentation_slot_id, count(*) AS accepted FROM media_files WHERE status='accepted' GROUP BY documentation_slot_id) m ON m.documentation_slot_id = ds.id
			WHERE ds.distribution_slot_id=$1 AND ds.is_required AND COALESCE(m.accepted,0) < ds.min_files
		)
	`, slotID).Scan(&incomplete); err != nil {
		return DistributionSlot{}, fmt.Errorf("check documentation completeness: %w", err)
	}
	if incomplete {
		return DistributionSlot{}, ErrDocumentationIncomplete
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET actual_recipient_person_id=$2, status='distributed', updated_at=now() WHERE id=$1`, allocationID, personID); err != nil {
		return DistributionSlot{}, fmt.Errorf("update package allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET status='completed', distributed_at=now(), distributed_by=$2, completed_at=now(), updated_at=now() WHERE id=$1`, slotID, actor.UserID); err != nil {
		return DistributionSlot{}, fmt.Errorf("complete distribution slot: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_completed", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": allocationID, "person_id": personID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit complete slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}
```

In `internal/distribution/service.go`, add the interface, struct field, wiring, and two methods:

```go
type posPenyerahanRepository interface {
	SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error)
	CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
}
```

Add `posPenyerahanRepository posPenyerahanRepository` to `Service`, wire via `service.posPenyerahanRepository, _ = repository.(posPenyerahanRepository)` in `NewService`.

```go
func (s *Service) SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
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
	return s.posPenyerahanRepository.SearchLinkedSlot(ctx, scheduleID, query, scope)
}

func (s *Service) CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber <= 0 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	if s.posPenyerahanRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Penyerahan is unavailable")
	}
	return s.posPenyerahanRepository.CompleteSlot(ctx, actor, input, meta, scope)
}
```

`ErrQueryRequired` — keep this sentinel from the original `models.go` if Task 4 removed it; add it back if needed, it's still used here.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -run TestCompleteSlot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/distribution/repository.go internal/distribution/service.go internal/distribution/pos_penyerahan_test.go
git commit -m "feat(distribution): add POS Penyerahan SearchLinkedSlot and CompleteSlot"
```

---

### Task 8: `internal/distribution` — re-scope media handling, remove old single-flow code, make the package build clean

**Files:**
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go` (rewrite to match the new shape — read it first, the old `storageStub`/`mediaRepositoryStub`/`repositoryStub` and their tests for `Search`/`GetWorkspace`/`SaveDraft`/`Complete` are all obsolete and must be replaced with equivalents for `CreateSlot`/`LinkSlot`/`CompleteSlot`/media, OR deleted if Tasks 5-7's own test files now cover that ground — do not leave duplicate/dead test scaffolding)

**Interfaces:**
- Produces: final, buildable `internal/distribution` package — `UploadMedia`/`DeleteMedia`/`OpenMedia` re-scoped to `distribution_slots` via `documentation_slots.distribution_slot_id`; every remnant of `Search`/`GetWorkspace`/`SaveDraft`/`Complete`/`RecipientWorkspace`/`DraftInput` removed.

- [ ] **Step 1: Read the current state of the whole package**

Read `internal/distribution/repository.go` and `service.go` in full as they now stand (after Tasks 4-7's additions layered on top of the still-present old code) to get exact current line numbers before deleting anything.

- [ ] **Step 2: Remove the old single-flow code**

Delete from `service.go`: `Search`, `GetWorkspace`, `SaveDraft`, `Complete` methods, the `repository`/`completionRepository` interfaces (superseded by `posMesinRepository`/`posDokumenRepository`/`posPenyerahanRepository`), and any now-unused sentinel error left over from Task 4's cleanup pass.

Delete from `repository.go`: `Search`, `GetWorkspace`, `SaveDraft`, `Complete`, `listReceiptHistory`, and any SQL helper only used by those (e.g. the old `lockAllocation`-style helper if `Complete`/`SaveDraft` had one distinct from what Tasks 6-7 added).

Keep: `listSlots`/`listMediaFiles`-equivalent helpers (consolidate — Task 5 added `listSlotDocumentation`/`listMediaFiles`; if the old `repository.go` had a same-purpose `listSlots` helper for the old flow, delete the old one and keep Task 5's, updating any leftover caller).

- [ ] **Step 3: Re-scope media handling**

Update `UploadMedia`/`DeleteMedia`/`OpenMedia` (service.go) and `GetMediaSlot`/`SaveMedia`/`GetMedia`/`DeleteMedia`/`RestoreMedia` (repository.go) so every SQL join that currently goes `media_files → documentation_slots → distribution_records → package_allocations → program_schedules` instead goes `media_files → documentation_slots → distribution_slots → program_schedules` (one fewer join level, since `distribution_slots` now carries `schedule_id` directly rather than needing a `package_allocations` hop for regency scoping):

```go
func (r *Repository) GetMediaSlot(ctx context.Context, slotID string, scope auth.RegencyScope) (MediaSlot, error) {
	var slot MediaSlot
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text, ds.input_source, ds.require_location, ds.require_captured_at, ds.min_files, ds.max_files,
			(SELECT count(*) FROM media_files m WHERE m.documentation_slot_id=ds.id AND m.status='accepted')
		FROM documentation_slots ds
		JOIN distribution_slots dsl ON dsl.id = ds.distribution_slot_id
		JOIN program_schedules ps ON ps.id = dsl.schedule_id
		WHERE ds.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, slotID, scope.Unrestricted, scope.RegencyIDs).Scan(&slot.ID, &slot.InputSource, &slot.RequireLocation, &slot.RequireCapturedAt, &slot.MinFiles, &slot.MaxFiles, &slot.AcceptedFiles)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaSlot{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaSlot{}, fmt.Errorf("get media slot: %w", err)
	}
	return slot, nil
}
```

Apply the same `distribution_records`→`distribution_slots` join simplification to `GetMedia`/`DeleteMedia`'s scoped lookups (both currently join through `documentation_slots→distribution_records→package_allocations→program_schedules`; both drop the `package_allocations` hop the same way). `SaveMedia`/`RestoreMedia`/`updateSlotStatus` don't scope by regency (they operate on an already-resolved `documentation_slot_id`) — no change needed beyond confirming they still compile against the renamed column.

- [ ] **Step 4: Update `internal/distribution/service_test.go`**

Read the file's current content. Remove every test for the deleted `Search`/`GetWorkspace`/`SaveDraft`/`Complete` methods and their stubs. Keep/adapt tests for `UploadMedia`/`DeleteMedia`/`OpenMedia` — update the `storageStub`/`mediaRepositoryStub` structs' method signatures only if the interfaces they satisfy changed shape (they likely didn't — media interface signatures are unchanged, only the SQL underneath); if a stub method references a symbol that Task 4 removed (e.g. `MediaSlot` construction referencing old joined fields), fix the reference.

- [ ] **Step 5: Run tests to verify the whole package passes**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/distribution/... -v`
Expected: all pass — every test from Tasks 1, 5, 6, 7, and the surviving media tests in `service_test.go`.

- [ ] **Step 6: Verify the whole module**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: clean — this is the first point in the plan where the WHOLE module must build (Tasks 5-7 only guaranteed `internal/distribution` itself compiled in isolation via scoped `-run`; this step confirms `internal/api`/`cmd/server` — which still reference the now-removed `Search`/`GetWorkspace`/`SaveDraft`/`Complete`/old types — will show compile errors. That's Task 9's job to fix; if `go build ./...` fails ONLY inside `internal/api`/`cmd/server` referencing symbols this task removed, that's expected and correctly deferred to Task 9. If it fails inside `internal/distribution` itself, fix it now.

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./internal/distribution/... ./internal/dcp3/... ./internal/recipients/... ./internal/reports/... ./internal/media/... -count=1`
Expected: all pass except the two documented pre-existing failures named in Global Constraints (neither is in this list of packages, so expect clean here).

- [ ] **Step 7: Commit**

```bash
git add internal/distribution/repository.go internal/distribution/service.go internal/distribution/service_test.go
git commit -m "feat(distribution): re-scope media handling to distribution_slots, remove old single-flow code"
```

---

### Task 9: `internal/api` — POS routes + wiring, remove old distribution routes

**Files:**
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go` (or wherever distribution route tests live — likely inline in `handler_test.go` per this session's established pattern; read it first)
- Modify: `cmd/server/main.go` (no change expected here — `distribution.NewService(distribution.NewRepository(pool), mediaStorage)` construction is unchanged; confirm, don't blind-edit)

**Interfaces:**
- Produces: `POST /api/v1/distribution/slots` (POS Mesin, `distribution.pos_mesin`), `GET /api/v1/distribution/candidates?schedule_id=&nik=` + `POST /api/v1/distribution/slots/:number/link` (POS Dokumen, `distribution.pos_dokumen`), `GET /api/v1/distribution/slots/search?schedule_id=&q=` + `POST /api/v1/distribution/slots/:number/complete` (POS Penyerahan, `distribution.pos_penyerahan`). Removes the old `distribution/search`, `distribution/allocations/*` (draft/complete) routes. Media routes (`distribution/slots/:id/media`, `distribution/media/:id`) stay, unchanged in shape.

- [ ] **Step 1: Read the current file**

Read `internal/api/distribution_routes.go`, the `DistributionService` interface in `internal/api/handler.go`, and the relevant `routeProtected` cases in `handler.go` in full — confirm exact current line numbers before editing.

- [ ] **Step 2: Write/update the failing tests**

In `internal/api/handler_test.go`, remove tests for the old `Search`/`GetWorkspace`/`SaveDraft`/`Complete` routes (`distribution/search`, `distribution/allocations/*`). Add a `fakeDistributionService` (or extend the existing one) implementing the new `DistributionService` interface shape (Step 3), and tests asserting: `POST /api/v1/distribution/slots` requires `distribution.pos_mesin` and forwards `CreateSlotInput`; `GET /api/v1/distribution/candidates` requires `distribution.pos_dokumen`; `POST /api/v1/distribution/slots/:number/link` requires `distribution.pos_dokumen`; `GET /api/v1/distribution/slots/search` requires `distribution.pos_penyerahan`; `POST /api/v1/distribution/slots/:number/complete` requires `distribution.pos_penyerahan`. Follow this session's established fake-service + `httptest` pattern exactly (same shape as `fakeActivitiesService`/`fakeRecipientsService` elsewhere in this file — mock the service, assert status codes and forwarded arguments, cover one permission-denied case per new route).

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/api/... -run TestDistribution -v`
Expected: FAIL — compile error, new `DistributionService` methods/routes don't exist yet.

- [ ] **Step 4: Update `DistributionService` interface** (`internal/api/handler.go`)

Replace the old interface (`Search`, `GetWorkspace`, `SaveDraft`, `Complete`, media methods) with:

```go
type DistributionService interface {
	CreateSlot(context.Context, auth.Principal, distribution.CreateSlotInput, auth.ClientMeta) (distribution.DistributionSlot, error)
	SearchCandidate(context.Context, string, string, auth.RegencyScope) (distribution.CandidateMatch, error)
	LinkSlot(context.Context, auth.Principal, distribution.LinkSlotInput, auth.ClientMeta, auth.RegencyScope) (distribution.DistributionSlot, error)
	SearchLinkedSlot(context.Context, string, string, auth.RegencyScope) (distribution.DistributionSlot, error)
	CompleteSlot(context.Context, auth.Principal, distribution.CompleteSlotInput, auth.ClientMeta, auth.RegencyScope) (distribution.DistributionSlot, error)
	UploadMedia(context.Context, auth.Principal, distribution.UploadMediaInput, auth.ClientMeta, auth.RegencyScope) (distribution.MediaFile, error)
	DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	OpenMedia(context.Context, string, auth.RegencyScope) (distribution.MediaContent, error)
}
```

(Keep `UploadMediaInput`/`MediaContent` type names as they already exist in `internal/distribution/models.go` — Task 4 didn't touch them.)

- [ ] **Step 5: Rewrite `internal/api/distribution_routes.go`**

Replace `handleDistributionSearch`/`handleDistributionAllocation` with:

```go
func (h *Handler) handleDistributionSlots(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.pos_mesin") {
		return
	}
	var input distribution.CreateSlotInput
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.deps.Distribution.CreateSlot(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) handleDistributionCandidates(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SearchCandidate(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("nik"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotLink(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	var input distribution.LinkSlotInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.SlotNumber = slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.LinkSlot(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotSearch(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.pos_penyerahan") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SearchLinkedSlot(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("q"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotComplete(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_penyerahan") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.CompleteSlot(r.Context(), rc.principal, distribution.CompleteSlotInput{ScheduleID: r.URL.Query().Get("schedule_id"), SlotNumber: slotNumber}, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
```

Add a small path-parsing dispatcher for the `distribution/slots/...` family. **Important:** `distribution/slots/{X}/media` uses a `documentation_slots` UUID for `{X}` (unchanged from the old route), while `distribution/slots/{X}/link` and `.../complete` use a numeric slot number — check the "media" case FIRST, by string comparison, before ever attempting to parse `{X}` as an integer, or every legitimate media upload will 404 (its UUID isn't a valid int):

```go
func (h *Handler) handleDistributionSlot(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "search" && r.Method == http.MethodGet {
		h.handleDistributionSlotSearch(w, r, rc)
		return
	}
	if len(parts) == 2 && parts[1] == "media" && r.Method == http.MethodPost {
		h.handleDistributionSlotMediaUpload(w, r, rc, parts[0])
		return
	}
	if len(parts) == 2 {
		slotNumber, err := strconv.Atoi(parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		switch {
		case parts[1] == "link" && r.Method == http.MethodPost:
			h.handleDistributionSlotLink(w, r, rc, slotNumber)
			return
		case parts[1] == "complete" && r.Method == http.MethodPost:
			h.handleDistributionSlotComplete(w, r, rc, slotNumber)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}
```

Rename the existing multipart-upload handler (currently `handleDistributionSlot`, keyed by a documentation-slot UUID for media upload) to `handleDistributionSlotMediaUpload` — its body is unchanged (still parses multipart form, calls `UploadMedia`), only its name changes to avoid colliding with the new dispatcher above, and its permission check switches from `documentation.manage` to... **keep it as `documentation.manage`** — media upload permission is unrelated to which POS is uploading (any POS with `documentation.manage` can attach evidence), this doesn't change. Same for `handleDistributionMedia` (content GET / DELETE) — unchanged, still gated by `distribution.view`/`documentation.manage`.

Add `"strconv"` and `"konkit/internal/distribution"` to the file's import block if not already present.

- [ ] **Step 6: Update `routeProtected`'s switch** (`internal/api/handler.go`)

Replace the old `distribution/search`/`distribution/allocations/` cases with:

```go
case path == "distribution/slots":
	h.handleDistributionSlots(w, r, rc)
case path == "distribution/candidates":
	h.handleDistributionCandidates(w, r, rc)
case strings.HasPrefix(path, "distribution/slots/"):
	h.handleDistributionSlot(w, r, rc, strings.TrimPrefix(path, "distribution/slots/"))
case strings.HasPrefix(path, "distribution/media/"):
	h.handleDistributionMedia(w, r, rc, strings.TrimPrefix(path, "distribution/media/"))
```

(Keep the existing `distribution/media/` case as-is; it's unchanged.)

- [ ] **Step 7: Update `writeServiceError`/`validationFields`** (`internal/api/routes.go`)

Add cases for the new sentinel errors from `internal/distribution/models.go` (Task 4): `ErrSlotNotFound`→404, `ErrSlotNotOpen`/`ErrSlotNotLinked`/`ErrAlreadyCompleted`/`ErrPreviouslyReceived`→409 "operation_rejected", `ErrCandidateNotFound`→404, `ErrSlotNumberRequired`/`ErrIdentityChangeReasonRequired`→400 validation_failed. Remove any case that ONLY referenced now-deleted errors like `ErrAllocationNotFound` if nothing else in the codebase still raises it (check first — `ErrAllocationNotFound` may have been kept if Task 4 chose to retain it for a still-used code path; if removed, remove its case here too).

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd "d:/KSM/Deployment/konkit" && go test ./internal/api/... -v`
Expected: all pass.

- [ ] **Step 9: Verify the whole module**

Run: `cd "d:/KSM/Deployment/konkit" && go build ./... && go vet ./...`
Expected: clean — this is the first point where the ENTIRE backend must compile end to end. `cmd/server/main.go`'s `Distribution: distribution.NewService(distribution.NewRepository(pool), mediaStorage)` line should need no change (verify by reading it — if it does need a change, e.g. a different constructor signature, make the minimal fix and note it in your report).

Run: `cd "d:/KSM/Deployment/konkit" && export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" && go test ./... -count=1 -p 1`
Expected: all pass except the two documented pre-existing failures.

- [ ] **Step 10: Manual smoke test**

Run: `cd "d:/KSM/Deployment/konkit" && go run ./cmd/server` — confirm it starts. Stop it once confirmed; do not leave it running.

- [ ] **Step 11: Commit**

```bash
git add internal/api/distribution_routes.go internal/api/handler.go internal/api/routes.go internal/api/handler_test.go
git commit -m "feat(api): replace single-flow distribution routes with POS Mesin/Dokumen/Penyerahan endpoints"
```

---

## After This Plan

- Backend fully supports the 3-POS model end to end, with no frontend consuming it yet — `frontend/src/features/distribution/*` still references the OLD API shape (`GET .../search`, `GET/PATCH/POST .../allocations/:id`) and will fail at runtime (not build time, since it's untyped fetch calls) until the follow-up frontend plan lands. Do not merge/deploy this plan's backend alone to a shared environment without immediately following up with the frontend plan, or land both before exposing this to real users.
- The follow-up plan builds: `POSMesinPage`, `POSDokumenPage`, `POSPenyerahanPage`, removes `DistributionPage`/`RecipientWorkspace.tsx`, adds the "POS" selector to the Template Dokumentasi slot config UI (`TemplatesPanel.tsx`), and updates `routes.tsx`/`AppShell.tsx`.
- **Known backend gap for the follow-up plan to close:** `documentation_template_slots.stage` is snapshotted into `documentation_slots` at slot creation, but the snapshot row itself has no `stage` column and `SlotSummary`/the API never expose it — so no POS station can currently filter its evidence list down to its own stage. The follow-up plan needs a small backend addition (add `documentation_slots.stage`, copy it from the template at `CreateSlot` snapshot time, surface it on `SlotSummary`) before the 3 POS pages can show only their own station's required photos. Found during this plan's final whole-branch review; deliberately not fixed here since no task in this plan was scoped to touch it and no current caller needs it.
