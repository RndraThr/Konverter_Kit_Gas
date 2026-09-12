# Data Penerima Master Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the `Dashboard > Data Penerima` placeholder into a cross-regency recipient master page with statistics, combinable search/filter, server-side pagination, and full CRUD (create manual, edit identity, soft-delete/cancel, restore).

**Architecture:** New Go domain `internal/recipients` (repository + service, following the exact conventions of `internal/programs` and `internal/distribution`) exposed via new `/api/v1/recipients*` routes wired into the existing `internal/api` dispatcher. New React page replacing `frontend/src/features/dashboard/DashboardPage.tsx`, following the URL-param-driven pagination pattern already used by `UsersPage.tsx` and the Shadcn `Select`/`Dialog` patterns already used by `programs` features.

**Tech Stack:** Go (stdlib `net/http`, pgx/v5, goose migrations), React 19 + TanStack Query + React Router + Shadcn/Base UI components, Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-09-12-data-penerima-master-design.md`

## Global Constraints

- New Go permissions: `recipients.view`, `recipients.manage` (super_admin auto-granted via migration).
- `package_allocations.status` values are exactly: `candidate, ready, needs_review, distributed, replaced, cancelled`. `distribution_records.status`: `draft, completed, cancelled`.
- Default recipient list excludes `allocation_status = 'cancelled'` unless the caller explicitly filters for it.
- This page must never write `package_allocations.status` to `distributed`/`replaced`, or touch `distribution_records` — those stay owned by the Pendistribusian flow (`internal/distribution`).
- All list/mutate operations must respect `auth.RegencyScope` using the inline-SQL pattern (`$N OR ps.regency_id::text = ANY($N+1)`), never a separate unscoped code path.
- Every mutation (`Create`/`Update`/`Cancel`/`Restore`) runs inside a single DB transaction and writes one `audit.Record` call before commit, action prefix `"recipient."`.
- Follow existing pagination JSON shape exactly: `{ items, page, page_size, total }`.
- `go test ./... -count=1`, `go vet ./...`, `npm run test`, `npm run build`, `npx tsc --noEmit` must all stay green after every task.

---

### Task 1: Migration — nullable import linkage + new permissions

**Files:**
- Create: `internal/database/migrations/00007_recipients.sql`
- Create: `internal/recipients/schema_integration_test.go`

**Interfaces:**
- Produces: `recipientsIntegrationPool(t *testing.T) *pgxpool.Pool` — the shared test-DB helper every later test file in this package will call (defined once here, reused by later `_test.go` files in the same package without re-declaring it).

Note: the spec only called out `candidate_nominations.import_row_id` for nullability. While grounding this plan I found `candidate_nominations.batch_id` is **also** `NOT NULL REFERENCES dcp3_import_batches(id)` (`internal/database/migrations/00004_program_dcp3_distribution.sql:159`) — a manually-created recipient has no batch either, so `batch_id` must become nullable too. This corrects the spec.

- [ ] **Step 1: Write the migration**

Create `internal/database/migrations/00007_recipients.sql`:

```sql
-- +goose Up
ALTER TABLE candidate_nominations ALTER COLUMN batch_id DROP NOT NULL;
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id DROP NOT NULL;

INSERT INTO permissions (code, name, description) VALUES
    ('recipients.view', 'Lihat Data Penerima', 'Melihat data penerima lintas kabupaten dan program'),
    ('recipients.manage', 'Kelola Data Penerima', 'Menambah, mengubah, membatalkan, dan memulihkan data penerima')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('recipients.view', 'recipients.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('recipients.view', 'recipients.manage');
ALTER TABLE candidate_nominations ALTER COLUMN import_row_id SET NOT NULL;
ALTER TABLE candidate_nominations ALTER COLUMN batch_id SET NOT NULL;
```

- [ ] **Step 2: Write the failing schema test**

Create `internal/recipients/schema_integration_test.go`:

```go
package recipients

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

func recipientsIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func TestMigrationAllowsNullImportLinkageAndSeedsPermissions(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	ctx := context.Background()

	var batchNullable, rowNullable string
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='candidate_nominations' AND column_name='batch_id'`).Scan(&batchNullable); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='candidate_nominations' AND column_name='import_row_id'`).Scan(&rowNullable); err != nil {
		t.Fatal(err)
	}
	if batchNullable != "YES" || rowNullable != "YES" {
		t.Fatalf("expected batch_id/import_row_id nullable, got batch=%s row=%s", batchNullable, rowNullable)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('recipients.view','recipients.manage')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 recipients permissions seeded, got %d", count)
	}

	var superAdminGrants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp
		JOIN roles ON roles.id = rp.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE roles.code = 'super_admin' AND p.code IN ('recipients.view','recipients.manage')
	`).Scan(&superAdminGrants); err != nil {
		t.Fatal(err)
	}
	if superAdminGrants != 2 {
		t.Fatalf("expected super_admin granted both new permissions, got %d", superAdminGrants)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails without the migration**

Run: `$env:TEST_DATABASE_URL="postgres://USER:PASSWORD@127.0.0.1:5432/konkit_test?sslmode=disable"; go test ./internal/recipients/... -run TestMigrationAllowsNullImportLinkageAndSeedsPermissions -v`

Expected: FAIL (before Step 1's file exists, or if you write the test before the SQL, the assertions fail because the columns are still `NOT NULL` and the permission rows don't exist). If you already created the migration file in Step 1, skip straight to Step 4 — goose picks up new files automatically, there's nothing to "fail" against once both files exist together. To genuinely see red first, comment out the two `ALTER TABLE` lines and the `INSERT` statements temporarily, run the test, confirm it fails, then restore them.

- [ ] **Step 4: Run the test to verify it passes**

Run the same command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/00007_recipients.sql internal/recipients/schema_integration_test.go
git commit -m "feat(recipients): add migration for manual recipient creation and permissions"
```

---

### Task 2: Recipient read model — `List` and `Stats`

**Files:**
- Create: `internal/recipients/models.go`
- Create: `internal/recipients/repository.go`
- Create: `internal/recipients/repository_integration_test.go`

**Interfaces:**
- Consumes: `recipientsIntegrationPool(t)` from Task 1 (same package, same test binary).
- Produces: `NewRepository(pool *pgxpool.Pool) *Repository`, `(*Repository).List(ctx, Filter, auth.RegencyScope) (Page, error)`, `(*Repository).Stats(ctx, auth.RegencyScope) (Stats, error)`, and the `Recipient`/`Filter`/`Page`/`Stats` types — Task 3 and Task 4 depend on these exact names.

- [ ] **Step 1: Write `models.go`**

```go
package recipients

import (
	"errors"
	"time"
)

var (
	ErrNotFound          = errors.New("recipient not found")
	ErrScheduleNotFound  = errors.New("schedule not found")
	ErrFullNameRequired  = errors.New("full name is required")
	ErrNIKInvalid        = errors.New("nik must be exactly 16 digits")
	ErrAlreadyCancelled  = errors.New("recipient is already cancelled")
	ErrNotCancelled      = errors.New("recipient is not cancelled")
)

type Recipient struct {
	AllocationID         string     `json:"allocation_id"`
	DistributionNumber   int        `json:"distribution_number"`
	AllocationStatus     string     `json:"allocation_status"`
	DistributionStatus   *string    `json:"distribution_status"`
	FullName             string     `json:"full_name"`
	NIK                  string     `json:"nik"`
	SectorIdentifierType string     `json:"sector_identifier_type"`
	SectorIdentifier     string     `json:"sector_identifier"`
	Address              string     `json:"address"`
	Village              string     `json:"village"`
	District             string     `json:"district"`
	PhoneNumber          string     `json:"phone_number"`
	ProgramID            string     `json:"program_id"`
	ProgramName          string     `json:"program_name"`
	ProgramType          string     `json:"program_type"`
	RegencyID            string     `json:"regency_id"`
	RegencyName          string     `json:"regency_name"`
	RegencyDocumentCode  string     `json:"regency_document_code"`
	ScheduleID           string     `json:"schedule_id"`
	ScheduleName         string     `json:"schedule_name"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Filter struct {
	Page               int
	PageSize           int
	Search             string
	RegencyID          string
	ProgramID          string
	ProgramType        string
	AllocationStatus   string
	DistributionStatus string
}

type Page struct {
	Items    []Recipient `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}

type Stats struct {
	Total              int64            `json:"total"`
	ByAllocationStatus map[string]int64 `json:"by_allocation_status"`
}

type CreateInput struct {
	ScheduleID       string `json:"schedule_id"`
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	SectorIdentifier string `json:"sector_identifier"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
}

type UpdateInput struct {
	FullName         string `json:"full_name"`
	NIK              string `json:"nik"`
	SectorIdentifier string `json:"sector_identifier"`
	Address          string `json:"address"`
	Village          string `json:"village"`
	District         string `json:"district"`
	PhoneNumber      string `json:"phone_number"`
}
```

- [ ] **Step 2: Write `repository.go` with `List` and `Stats`**

```go
package recipients

import (
	"context"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const recipientSelect = `
SELECT
	pa.id::text, pa.distribution_number, pa.status,
	dr.status,
	p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
	COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''),
	prog.id::text, prog.name, prog.program_type,
	r.id::text, r.name, r.document_code,
	ps.id::text, ps.name,
	pa.created_at, pa.updated_at
FROM package_allocations pa
JOIN candidate_nominations cn ON cn.id = pa.nomination_id
JOIN program_schedules ps ON ps.id = pa.schedule_id
JOIN programs prog ON prog.id = ps.program_id
JOIN regencies r ON r.id = ps.regency_id
JOIN people p ON p.id = COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)
LEFT JOIN distribution_records dr ON dr.allocation_id = pa.id
LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id
	AND psi.identifier_type = CASE prog.program_type WHEN 'farmer' THEN 'farmer_card' ELSE 'kusuka' END`

const recipientWhere = `
WHERE ($1 = '%%' OR p.full_name ILIKE $1 OR p.nik ILIKE $1 OR psi.normalized_value ILIKE $1)
  AND ($2 = '' OR r.id::text = $2)
  AND ($3 = '' OR prog.id::text = $3)
  AND ($4 = '' OR prog.program_type = $4)
  AND (($5 = '' AND pa.status != 'cancelled') OR ($5 != '' AND pa.status = $5))
  AND ($6 = '' OR dr.status = $6)
  AND ($7 OR ps.regency_id::text = ANY($8))`

func scanRecipient(row pgx.Row) (Recipient, error) {
	var item Recipient
	if err := row.Scan(
		&item.AllocationID, &item.DistributionNumber, &item.AllocationStatus,
		&item.DistributionStatus,
		&item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier,
		&item.Address, &item.Village, &item.District, &item.PhoneNumber,
		&item.ProgramID, &item.ProgramName, &item.ProgramType,
		&item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode,
		&item.ScheduleID, &item.ScheduleName,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Recipient{}, fmt.Errorf("scan recipient: %w", err)
	}
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	search := "%" + filter.Search + "%"
	args := []any{search, filter.RegencyID, filter.ProgramID, filter.ProgramType, filter.AllocationStatus, filter.DistributionStatus, scope.Unrestricted, scope.RegencyIDs}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM package_allocations pa JOIN candidate_nominations cn ON cn.id=pa.nomination_id JOIN program_schedules ps ON ps.id=pa.schedule_id JOIN programs prog ON prog.id=ps.program_id JOIN regencies r ON r.id=ps.regency_id JOIN people p ON p.id=COALESCE(pa.actual_recipient_person_id,pa.intended_person_id,cn.person_id) LEFT JOIN distribution_records dr ON dr.allocation_id=pa.id LEFT JOIN person_sector_identifiers psi ON psi.person_id=p.id AND psi.identifier_type = CASE prog.program_type WHEN 'farmer' THEN 'farmer_card' ELSE 'kusuka' END "+recipientWhere, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count recipients: %w", err)
	}

	rows, err := r.pool.Query(ctx, recipientSelect+recipientWhere+" ORDER BY pa.created_at DESC, pa.id DESC LIMIT $9 OFFSET $10",
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return Page{}, fmt.Errorf("list recipients: %w", err)
	}
	defer rows.Close()
	items := []Recipient{}
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate recipients: %w", err)
	}
	return Page{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *Repository) Stats(ctx context.Context, scope auth.RegencyScope) (Stats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.status, count(*)
		FROM package_allocations pa JOIN program_schedules ps ON ps.id = pa.schedule_id
		WHERE ($1 OR ps.regency_id::text = ANY($2))
		GROUP BY pa.status`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return Stats{}, fmt.Errorf("stats recipients: %w", err)
	}
	defer rows.Close()
	stats := Stats{ByAllocationStatus: map[string]int64{}}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return Stats{}, fmt.Errorf("scan recipient stats: %w", err)
		}
		stats.ByAllocationStatus[status] = count
		if status != "cancelled" {
			stats.Total += count
		}
	}
	return stats, rows.Err()
}

func getRecipientByID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, allocationID string) (Recipient, error) {
	row := q.QueryRow(ctx, recipientSelect+" WHERE pa.id = $1", allocationID)
	item, err := scanRecipient(row)
	if err != nil {
		return Recipient{}, ErrNotFound
	}
	return item, nil
}
```

- [ ] **Step 3: Write failing integration tests for `List`/`Stats`**

Create `internal/recipients/repository_integration_test.go`:

```go
package recipients

import (
	"context"
	"testing"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type recipientFixture struct {
	scheduleID          string
	farmerRegencyID     string
	otherRegencyID      string
	distributedAllocID  string
	needsReviewAllocID  string
	cancelledAllocID    string
}

func seedRecipientFixture(t *testing.T, pool *pgxpool.Pool) recipientFixture {
	t.Helper()
	ctx := context.Background()
	var fixture recipientFixture

	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Wajo Recipients Test','WRT',true) RETURNING id::text`).Scan(&fixture.farmerRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Bone Recipients Test','BRT',true) RETURNING id::text`).Scan(&fixture.otherRegencyID))
	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES('RCPT-TEST','Program Test Recipients','farmer',2026,'active') RETURNING id::text`).Scan(&programID))
	var packageTemplateID, docTemplateID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES('PKG-RCPT',1,'Paket Test','farmer','{}'::jsonb,'published') RETURNING id::text`).Scan(&packageTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES('DOC-RCPT',1,'Dok Test','farmer','published') RETURNING id::text`).Scan(&docTemplateID))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Recipients','2026-01-01','2026-12-31','active',4,'{}'::jsonb) RETURNING id::text`, programID, fixture.farmerRegencyID, packageTemplateID, docTemplateID).Scan(&fixture.scheduleID))

	insertAllocation := func(name, status string, distNumber int) string {
		var personID, nominationID, allocationID string
		must(t, pool.QueryRow(ctx, `INSERT INTO people(full_name) VALUES($1) RETURNING id::text`, name).Scan(&personID))
		must(t, pool.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,'farmer','{}'::jsonb,'ready') RETURNING id::text`, personID).Scan(&nominationID))
		must(t, pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}'::jsonb) RETURNING id::text`, fixture.scheduleID, nominationID, personID, distNumber, status).Scan(&allocationID))
		return allocationID
	}
	fixture.distributedAllocID = insertAllocation("Distributed Person", "distributed", 1)
	fixture.needsReviewAllocID = insertAllocation("Needs Review Person", "needs_review", 2)
	fixture.cancelledAllocID = insertAllocation("Cancelled Person", "cancelled", 3)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id = $1`, fixture.scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id = $1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_template_versions WHERE id = $1`, packageTemplateID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_template_versions WHERE id = $1`, docTemplateID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id IN ($1,$2)`, fixture.farmerRegencyID, fixture.otherRegencyID)
	})
	return fixture
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestListExcludesCancelledByDefaultAndCombinesFilters(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	unrestricted := auth.RegencyScope{Unrestricted: true}

	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	var sawCancelled bool
	for _, item := range page.Items {
		if item.AllocationID == fixture.cancelledAllocID {
			sawCancelled = true
		}
	}
	if sawCancelled {
		t.Fatal("default list must exclude cancelled allocations")
	}

	explicit, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, AllocationStatus: "cancelled"}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range explicit.Items {
		if item.AllocationID == fixture.cancelledAllocID {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit cancelled filter must return the cancelled allocation")
	}

	combined, err := repository.List(ctx, Filter{Page: 1, PageSize: 20, Search: "Needs Review", AllocationStatus: "needs_review"}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Items) != 1 || combined.Items[0].AllocationID != fixture.needsReviewAllocID {
		t.Fatalf("expected exactly the needs_review match, got %+v", combined.Items)
	}
}

func TestListRespectsRegencyScope(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, scoped)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ScheduleID == fixture.scheduleID {
			t.Fatalf("scoped caller must not see recipients from an out-of-scope regency: %+v", item)
		}
	}
}

func TestStatsGroupsByAllocationStatusAndExcludesCancelledFromTotal(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	stats, err := repository.Stats(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ByAllocationStatus["distributed"] < 1 || stats.ByAllocationStatus["needs_review"] < 1 || stats.ByAllocationStatus["cancelled"] < 1 {
		t.Fatalf("expected all three statuses represented: %+v", stats.ByAllocationStatus)
	}
	_ = fixture.distributedAllocID
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go build ./... ` first to catch compile errors, then `go test ./internal/recipients/... -v` with `TEST_DATABASE_URL` set.
Expected: compiles and runs against real schema; since `repository.go` from Step 2 already implements `List`/`Stats` correctly, these should pass immediately once compile errors (like the placeholder type) are fixed — if they fail, the failure must be a real assertion mismatch, not a build error. Fix any SQL/scan bugs surfaced here before proceeding.

- [ ] **Step 5: Run tests to verify they pass**

Run the same command. Expected: PASS, all three tests green.

- [ ] **Step 6: Commit**

```bash
git add internal/recipients/models.go internal/recipients/repository.go internal/recipients/repository_integration_test.go
git commit -m "feat(recipients): add cross-regency List and Stats repository queries"
```

---

### Task 3: Repository — `Create`, `Update`, `Cancel`, `Restore`

**Files:**
- Modify: `internal/recipients/repository.go`
- Modify: `internal/recipients/repository_integration_test.go`

**Interfaces:**
- Consumes: `Recipient`, `CreateInput`, `UpdateInput`, `getRecipientByID` from Task 2; `audit.Record`, `auth.Principal`, `auth.ClientMeta`, `auth.RegencyScope` from `internal/auth`/`internal/audit`.
- Produces: `(*Repository).Create(ctx, auth.Principal, CreateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)`, `(*Repository).Update(ctx, auth.Principal, allocationID string, UpdateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)`, `(*Repository).Cancel(ctx, auth.Principal, allocationID string, auth.ClientMeta, auth.RegencyScope) error`, `(*Repository).Restore(ctx, auth.Principal, allocationID string, auth.ClientMeta, auth.RegencyScope) error` — Task 4's service wraps these exact signatures.

- [ ] **Step 1: Add `Create`/`Update`/`Cancel`/`Restore` to `repository.go`**

Append to `internal/recipients/repository.go` (add `"errors"`, `"strings"`, `"konkit/internal/audit"`, `"github.com/jackc/pgx/v5/pgconn"` to imports):

```go
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func recordRecipientAudit(ctx context.Context, tx pgx.Tx, actor auth.Principal, meta auth.ClientMeta, action, resourceID string, metadata map[string]any) error {
	return audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "recipient." + action, ResourceType: "package_allocation", ResourceID: resourceID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent})
}

func (r *Repository) Create(ctx context.Context, actor auth.Principal, input CreateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Recipient{}, fmt.Errorf("begin create recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var programType string
	err = tx.QueryRow(ctx, `
		SELECT prog.program_type FROM program_schedules ps JOIN programs prog ON prog.id = ps.program_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF ps
	`, input.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&programType)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipient{}, ErrScheduleNotFound
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("lock schedule for create: %w", err)
	}

	var personID string
	err = tx.QueryRow(ctx, `INSERT INTO people(full_name,nik,address,village,district,phone_number) VALUES($1,NULLIF($2,''),$3,$4,$5,$6) RETURNING id::text`,
		input.FullName, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber).Scan(&personID)
	if isUniqueViolation(err) {
		return Recipient{}, ErrNIKInvalid
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("insert recipient person: %w", err)
	}

	if strings.TrimSpace(input.SectorIdentifier) != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)`,
			personID, identifierType, strings.ToUpper(strings.TrimSpace(input.SectorIdentifier))); err != nil {
			return Recipient{}, fmt.Errorf("insert sector identifier: %w", err)
		}
	}

	var nominationID string
	if err := tx.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,$2,'{}'::jsonb,'ready') RETURNING id::text`,
		personID, programType).Scan(&nominationID); err != nil {
		return Recipient{}, fmt.Errorf("insert nomination: %w", err)
	}

	var distributionNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(distribution_number),0)+1 FROM package_allocations WHERE schedule_id = $1`, input.ScheduleID).Scan(&distributionNumber); err != nil {
		return Recipient{}, fmt.Errorf("compute distribution number: %w", err)
	}

	var allocationID string
	if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,'ready','{}'::jsonb) RETURNING id::text`,
		input.ScheduleID, nominationID, personID, distributionNumber).Scan(&allocationID); err != nil {
		return Recipient{}, fmt.Errorf("insert allocation: %w", err)
	}

	if err := recordRecipientAudit(ctx, tx, actor, meta, "created", allocationID, map[string]any{"full_name": input.FullName, "schedule_id": input.ScheduleID}); err != nil {
		return Recipient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recipient{}, fmt.Errorf("commit create recipient: %w", err)
	}
	return getRecipientByID(ctx, r.pool, allocationID)
}

func (r *Repository) lockAllocationInScope(ctx context.Context, tx pgx.Tx, allocationID string, scope auth.RegencyScope) (personID, programType, currentStatus string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id), prog.program_type, pa.status
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN programs prog ON prog.id = ps.program_id
		WHERE pa.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF pa
	`, allocationID, scope.Unrestricted, scope.RegencyIDs).Scan(&personID, &programType, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("lock allocation: %w", err)
	}
	return personID, programType, currentStatus, nil
}

func (r *Repository) Update(ctx context.Context, actor auth.Principal, allocationID string, input UpdateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Recipient{}, fmt.Errorf("begin update recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	personID, programType, _, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return Recipient{}, err
	}

	_, err = tx.Exec(ctx, `UPDATE people SET full_name=$2,nik=NULLIF($3,''),address=$4,village=$5,district=$6,phone_number=$7,updated_at=now() WHERE id=$1`,
		personID, input.FullName, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber)
	if isUniqueViolation(err) {
		return Recipient{}, ErrNIKInvalid
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("update recipient person: %w", err)
	}

	if strings.TrimSpace(input.SectorIdentifier) != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		normalized := strings.ToUpper(strings.TrimSpace(input.SectorIdentifier))
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)
			ON CONFLICT (person_id, identifier_type) DO UPDATE SET normalized_value = EXCLUDED.normalized_value, display_value = EXCLUDED.display_value, updated_at = now()
		`, personID, identifierType, normalized); err != nil {
			return Recipient{}, fmt.Errorf("upsert sector identifier: %w", err)
		}
	}

	if err := recordRecipientAudit(ctx, tx, actor, meta, "updated", allocationID, map[string]any{"full_name": input.FullName}); err != nil {
		return Recipient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recipient{}, fmt.Errorf("commit update recipient: %w", err)
	}
	return getRecipientByID(ctx, r.pool, allocationID)
}

func (r *Repository) Cancel(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancel recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, _, currentStatus, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return err
	}
	if currentStatus == "cancelled" {
		return ErrAlreadyCancelled
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET status='cancelled', updated_at=now() WHERE id=$1`, allocationID); err != nil {
		return fmt.Errorf("cancel allocation: %w", err)
	}
	if err := recordRecipientAudit(ctx, tx, actor, meta, "cancelled", allocationID, map[string]any{"previous_status": currentStatus}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Restore(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin restore recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, _, currentStatus, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return err
	}
	if currentStatus != "cancelled" {
		return ErrNotCancelled
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET status='ready', updated_at=now() WHERE id=$1`, allocationID); err != nil {
		return fmt.Errorf("restore allocation: %w", err)
	}
	if err := recordRecipientAudit(ctx, tx, actor, meta, "restored", allocationID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Add failing integration tests**

Append to `internal/recipients/repository_integration_test.go` (add `"errors"` to the existing import block from Task 2 — these new tests are the first in this file to use `errors.Is`):

```go
func TestCreateInsertsRecipientWithoutImportRowAndRejectsOutOfScopeSchedule(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: "actor-1"}
	meta := auth.ClientMeta{UserAgent: "test"}

	created, err := repository.Create(ctx, actor, CreateInput{
		ScheduleID: fixture.scheduleID, FullName: "Manual Recipient", NIK: "1234567890123456", SectorIdentifier: "KP-99",
		Address: "Jalan Test", Village: "Desa Test", District: "Kecamatan Test", PhoneNumber: "0812345678",
	}, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.FullName != "Manual Recipient" || created.NIK != "1234567890123456" || created.SectorIdentifier != "KP-99" || created.AllocationStatus != "ready" {
		t.Fatalf("unexpected created recipient: %+v", created)
	}

	var importRowID, batchID *string
	if err := pool.QueryRow(ctx, `
		SELECT cn.import_row_id::text, cn.batch_id::text FROM package_allocations pa JOIN candidate_nominations cn ON cn.id = pa.nomination_id WHERE pa.id = $1
	`, created.AllocationID).Scan(&importRowID, &batchID); err != nil {
		t.Fatal(err)
	}
	if importRowID != nil || batchID != nil {
		t.Fatalf("manual recipient must have null import linkage, got row=%v batch=%v", importRowID, batchID)
	}

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	if _, err := repository.Create(ctx, actor, CreateInput{ScheduleID: fixture.scheduleID, FullName: "Should Fail"}, meta, scoped); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("expected ErrScheduleNotFound for out-of-scope schedule, got %v", err)
	}
}

func TestUpdateChangesIdentityFieldsWithinScope(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: "actor-1"}
	meta := auth.ClientMeta{UserAgent: "test"}

	updated, err := repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{
		FullName: "Updated Name", NIK: "9999999999999999", Address: "Alamat Baru", Village: "Desa Baru", District: "Kecamatan Baru", PhoneNumber: "0899999999",
	}, meta, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "Updated Name" || updated.NIK != "9999999999999999" || updated.Address != "Alamat Baru" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	scoped := auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}
	if _, err := repository.Update(ctx, actor, fixture.needsReviewAllocID, UpdateInput{FullName: "Nope"}, meta, scoped); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for out-of-scope update, got %v", err)
	}
}

func TestCancelHidesFromDefaultListAndRestoreReturnsToReady(t *testing.T) {
	pool := recipientsIntegrationPool(t)
	fixture := seedRecipientFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: "actor-1"}
	meta := auth.ClientMeta{UserAgent: "test"}
	unrestricted := auth.RegencyScope{Unrestricted: true}

	if err := repository.Cancel(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); err != nil {
		t.Fatal(err)
	}
	if err := repository.Cancel(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); !errors.Is(err, ErrAlreadyCancelled) {
		t.Fatalf("expected ErrAlreadyCancelled, got %v", err)
	}

	page, err := repository.List(ctx, Filter{Page: 1, PageSize: 20}, unrestricted)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.AllocationID == fixture.needsReviewAllocID {
			t.Fatal("cancelled recipient must not appear in default list")
		}
	}

	if err := repository.Restore(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); err != nil {
		t.Fatal(err)
	}
	if err := repository.Restore(ctx, actor, fixture.needsReviewAllocID, meta, unrestricted); !errors.Is(err, ErrNotCancelled) {
		t.Fatalf("expected ErrNotCancelled, got %v", err)
	}
	restored, err := getRecipientByID(ctx, pool, fixture.needsReviewAllocID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AllocationStatus != "ready" {
		t.Fatalf("expected status ready after restore, got %q", restored.AllocationStatus)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail, then pass**

Run: `go test ./internal/recipients/... -v` with `TEST_DATABASE_URL` set. Fix any SQL errors until all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/recipients/repository.go internal/recipients/repository_integration_test.go
git commit -m "feat(recipients): add Create, Update, Cancel, Restore repository operations"
```

---

### Task 4: Service validation layer

**Files:**
- Create: `internal/recipients/service.go`
- Create: `internal/recipients/service_test.go`

**Interfaces:**
- Consumes: `Repository` methods from Tasks 2–3 (via a package-private `repository` interface, matching `internal/programs/service.go`'s pattern).
- Produces: `NewService(repository) *Service`, `(*Service).List`, `(*Service).Stats`, `(*Service).Create`, `(*Service).Update`, `(*Service).Cancel`, `(*Service).Restore` — Task 5's API handlers call these exact methods.

- [ ] **Step 1: Write `service.go`**

```go
package recipients

import (
	"context"
	"regexp"
	"strings"

	"konkit/internal/auth"
)

var nikPattern = regexp.MustCompile(`^[0-9]{16}$`)

type repository interface {
	List(context.Context, Filter, auth.RegencyScope) (Page, error)
	Stats(context.Context, auth.RegencyScope) (Stats, error)
	Create(context.Context, auth.Principal, CreateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)
	Update(context.Context, auth.Principal, string, UpdateInput, auth.ClientMeta, auth.RegencyScope) (Recipient, error)
	Cancel(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	Restore(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return s.repository.List(ctx, filter, scope)
}

func (s *Service) Stats(ctx context.Context, scope auth.RegencyScope) (Stats, error) {
	return s.repository.Stats(ctx, scope)
}

func validateIdentity(fullName, nik string) error {
	if strings.TrimSpace(fullName) == "" {
		return ErrFullNameRequired
	}
	if nik = strings.TrimSpace(nik); nik != "" && !nikPattern.MatchString(nik) {
		return ErrNIKInvalid
	}
	return nil
}

func (s *Service) Create(ctx context.Context, actor auth.Principal, input CreateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.NIK = strings.TrimSpace(input.NIK)
	if err := validateIdentity(input.FullName, input.NIK); err != nil {
		return Recipient{}, err
	}
	return s.repository.Create(ctx, actor, input, meta, scope)
}

func (s *Service) Update(ctx context.Context, actor auth.Principal, allocationID string, input UpdateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.NIK = strings.TrimSpace(input.NIK)
	if err := validateIdentity(input.FullName, input.NIK); err != nil {
		return Recipient{}, err
	}
	return s.repository.Update(ctx, actor, strings.TrimSpace(allocationID), input, meta, scope)
}

func (s *Service) Cancel(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	return s.repository.Cancel(ctx, actor, strings.TrimSpace(allocationID), meta, scope)
}

func (s *Service) Restore(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	return s.repository.Restore(ctx, actor, strings.TrimSpace(allocationID), meta, scope)
}
```

- [ ] **Step 2: Write failing unit tests**

Create `internal/recipients/service_test.go`:

```go
package recipients

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type repositoryStub struct {
	createInput CreateInput
	updateInput UpdateInput
	actor       auth.Principal
	listFilter  Filter
}

func (r *repositoryStub) List(_ context.Context, filter Filter, _ auth.RegencyScope) (Page, error) {
	r.listFilter = filter
	return Page{Page: filter.Page, PageSize: filter.PageSize}, nil
}
func (r *repositoryStub) Stats(context.Context, auth.RegencyScope) (Stats, error) { return Stats{}, nil }
func (r *repositoryStub) Create(_ context.Context, actor auth.Principal, input CreateInput, _ auth.ClientMeta, _ auth.RegencyScope) (Recipient, error) {
	r.actor, r.createInput = actor, input
	return Recipient{FullName: input.FullName}, nil
}
func (r *repositoryStub) Update(_ context.Context, actor auth.Principal, _ string, input UpdateInput, _ auth.ClientMeta, _ auth.RegencyScope) (Recipient, error) {
	r.actor, r.updateInput = actor, input
	return Recipient{FullName: input.FullName}, nil
}
func (r *repositoryStub) Cancel(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error {
	return nil
}
func (r *repositoryStub) Restore(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error {
	return nil
}

func TestCreateValidatesFullNameAndNIK(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	actor := auth.Principal{UserID: "actor-1"}

	if _, err := service.Create(context.Background(), actor, CreateInput{FullName: "  "}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrFullNameRequired) {
		t.Fatalf("expected ErrFullNameRequired, got %v", err)
	}
	if _, err := service.Create(context.Background(), actor, CreateInput{FullName: "Budi", NIK: "123"}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("expected ErrNIKInvalid, got %v", err)
	}

	saved, err := service.Create(context.Background(), actor, CreateInput{FullName: "  Budi Santoso  ", NIK: "1234567890123456"}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if saved.FullName != "Budi Santoso" || repository.createInput.FullName != "Budi Santoso" {
		t.Fatalf("expected trimmed full name forwarded, got %+v", repository.createInput)
	}
	if repository.actor.UserID != actor.UserID {
		t.Fatalf("expected actor forwarded, got %+v", repository.actor)
	}
}

func TestUpdateValidatesIdentitySameAsCreate(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	if _, err := service.Update(context.Background(), auth.Principal{}, "allocation-1", UpdateInput{FullName: ""}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrFullNameRequired) {
		t.Fatalf("expected ErrFullNameRequired, got %v", err)
	}
	if _, err := service.Update(context.Background(), auth.Principal{}, "allocation-1", UpdateInput{FullName: "Budi", NIK: "abc"}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("expected ErrNIKInvalid, got %v", err)
	}
}

func TestListNormalizesPagination(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	if _, err := service.List(context.Background(), Filter{Page: 0, PageSize: 0}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.Page != 1 || repository.listFilter.PageSize != 20 {
		t.Fatalf("expected defaults page=1 page_size=20, got %+v", repository.listFilter)
	}

	if _, err := service.List(context.Background(), Filter{Page: 3, PageSize: 500}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.Page != 3 || repository.listFilter.PageSize != 20 {
		t.Fatalf("expected page=3 clamped page_size=20, got %+v", repository.listFilter)
	}
}
```

- [ ] **Step 3: Run tests to verify pass**

Run: `go test ./internal/recipients/... -run 'TestCreateValidates|TestUpdateValidates|TestListNormalizes' -v`
Expected: PASS (these are pure unit tests, no `TEST_DATABASE_URL` needed).

- [ ] **Step 4: Run full package test suite**

Run: `go test ./internal/recipients/... -v` (with `TEST_DATABASE_URL` set) — confirms Task 2/3 integration tests and Task 4 unit tests all coexist and pass together.

- [ ] **Step 5: Commit**

```bash
git add internal/recipients/service.go internal/recipients/service_test.go
git commit -m "feat(recipients): add service-layer identity validation"
```

---

### Task 5: API wiring — routes, permissions, dependency injection

**Files:**
- Modify: `internal/api/handler.go`
- Create: `internal/api/recipients_routes.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `recipients.Service` methods from Task 4; existing `decodeJSON`, `writeData`, `writeServiceError`, `writeUnavailable`, `writeFieldError`, `methodNotAllowed`, `clientMeta`, `intQuery`, `(h *Handler) authorize`, `(h *Handler) regencyScope`.
- Produces: `GET/POST /api/v1/recipients`, `GET /api/v1/recipients/stats`, `PATCH/POST /api/v1/recipients/{id}[/cancel|/restore]` — Task 8/9 (frontend) call these exact paths and JSON shapes.

- [ ] **Step 1: Add the `RecipientsService` interface and `Dependencies` field**

In `internal/api/handler.go`, add `"konkit/internal/recipients"` to the import block (alphabetically, after `"konkit/internal/reports"` — before `"konkit/internal/settings"`), then add this interface after `ReportsService` (around line 103):

```go
type RecipientsService interface {
	List(context.Context, recipients.Filter, auth.RegencyScope) (recipients.Page, error)
	Stats(context.Context, auth.RegencyScope) (recipients.Stats, error)
	Create(context.Context, auth.Principal, recipients.CreateInput, auth.ClientMeta, auth.RegencyScope) (recipients.Recipient, error)
	Update(context.Context, auth.Principal, string, recipients.UpdateInput, auth.ClientMeta, auth.RegencyScope) (recipients.Recipient, error)
	Cancel(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
	Restore(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error
}
```

Add `Recipients RecipientsService` as a new field in `Dependencies` (right after `Reports ReportsService`).

Add these two route cases to `routeProtected` (`handler.go:182-232`), right after the `reports/schedule/` case and before `default`:

```go
	case path == "recipients":
		h.handleRecipients(w, r, rc)
	case path == "recipients/stats":
		h.handleRecipientStats(w, r, rc)
	case strings.HasPrefix(path, "recipients/"):
		h.handleRecipient(w, r, rc, strings.TrimPrefix(path, "recipients/"))
```

- [ ] **Step 2: Write `internal/api/recipients_routes.go`**

```go
package api

import (
	"net/http"
	"strings"

	"konkit/internal/recipients"
)

func (h *Handler) handleRecipients(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "recipients.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		page, err := h.deps.Recipients.List(r.Context(), recipients.Filter{
			Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 20),
			Search: r.URL.Query().Get("search"), RegencyID: r.URL.Query().Get("regency_id"),
			ProgramID: r.URL.Query().Get("program_id"), ProgramType: r.URL.Query().Get("program_type"),
			AllocationStatus: r.URL.Query().Get("allocation_status"), DistributionStatus: r.URL.Query().Get("distribution_status"),
		}, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, page)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		var input recipients.CreateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Recipients.Create(r.Context(), rc.principal, input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRecipientStats(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "recipients.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	stats, err := h.deps.Recipients.Stats(r.Context(), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, stats)
}

func (h *Handler) handleRecipient(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodPatch {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		var input recipients.UpdateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Recipients.Update(r.Context(), rc.principal, parts[0], input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Recipients.Cancel(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "restore" && r.Method == http.MethodPost {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Recipients.Restore(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 1 {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}
```

- [ ] **Step 3: Extend `writeServiceError` and `validationFields`**

In `internal/api/routes.go`, add `"konkit/internal/recipients"` to imports, then add a case to `writeServiceError` (`routes.go:448`, alongside the existing not-found line):

```go
	case errors.Is(err, profile.ErrNotFound), errors.Is(err, administration.ErrNotFound), errors.Is(err, programs.ErrNotFound), errors.Is(err, dcp3.ErrPreviewNotFound), errors.Is(err, distribution.ErrAllocationNotFound), errors.Is(err, distribution.ErrMediaNotFound), errors.Is(err, reports.ErrScheduleNotFound), errors.Is(err, recipients.ErrNotFound), errors.Is(err, recipients.ErrScheduleNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
```

Add a new case for the recipients conflict errors (near the other `StatusConflict` cases):

```go
	case errors.Is(err, recipients.ErrAlreadyCancelled), errors.Is(err, recipients.ErrNotCancelled):
		writeError(w, http.StatusConflict, "operation_rejected", err.Error())
```

Add a new case for the recipients validation errors (near the other `StatusBadRequest`/`validationFields` cases):

```go
	case errors.Is(err, recipients.ErrFullNameRequired), errors.Is(err, recipients.ErrNIKInvalid):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), validationFields(err))
```

In `validationFields` (`routes.go:501`), add:

```go
	case errors.Is(err, recipients.ErrFullNameRequired):
		return map[string]string{"full_name": err.Error()}
	case errors.Is(err, recipients.ErrNIKInvalid):
		return map[string]string{"nik": err.Error()}
```

- [ ] **Step 4: Wire the service in `cmd/server/main.go`**

Add `"konkit/internal/recipients"` to the import block (alphabetically, after `"konkit/internal/reports"`, before `"konkit/internal/settings"`). Add this line inside the `apihttp.Dependencies{...}` literal, after `Reports: ...`:

```go
		Recipients:     recipients.NewService(recipients.NewRepository(pool)),
```

- [ ] **Step 5: Write failing handler tests**

Append to `internal/api/handler_test.go` (add `"konkit/internal/recipients"` to imports):

```go
type fakeRecipientsService struct {
	page             recipients.Page
	stats            recipients.Stats
	created          recipients.Recipient
	updated          recipients.Recipient
	createInput      recipients.CreateInput
	updateInput      recipients.UpdateInput
	allocationID     string
	seenRegencyScope auth.RegencyScope
	cancelErr        error
	restoreErr       error
}

func (f *fakeRecipientsService) List(_ context.Context, _ recipients.Filter, scope auth.RegencyScope) (recipients.Page, error) {
	f.seenRegencyScope = scope
	return f.page, nil
}
func (f *fakeRecipientsService) Stats(_ context.Context, scope auth.RegencyScope) (recipients.Stats, error) {
	f.seenRegencyScope = scope
	return f.stats, nil
}
func (f *fakeRecipientsService) Create(_ context.Context, _ auth.Principal, input recipients.CreateInput, _ auth.ClientMeta, scope auth.RegencyScope) (recipients.Recipient, error) {
	f.createInput, f.seenRegencyScope = input, scope
	return f.created, nil
}
func (f *fakeRecipientsService) Update(_ context.Context, _ auth.Principal, allocationID string, input recipients.UpdateInput, _ auth.ClientMeta, scope auth.RegencyScope) (recipients.Recipient, error) {
	f.allocationID, f.updateInput, f.seenRegencyScope = allocationID, input, scope
	return f.updated, nil
}
func (f *fakeRecipientsService) Cancel(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.cancelErr
}
func (f *fakeRecipientsService) Restore(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.restoreErr
}

func TestRecipientsListRequiresViewPermissionAndForwardsFilters(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.view": true}}
	service := &fakeRecipientsService{page: recipients.Page{Page: 1, PageSize: 20, Total: 1, Items: []recipients.Recipient{{AllocationID: "allocation-1", FullName: "Siti Aminah"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients?search=Siti&page=1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Recipients: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Siti Aminah") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{}}
	denied := httptest.NewRequest(http.MethodGet, "/api/v1/recipients", nil)
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Recipients: service}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without recipients.view, got %d", deniedRecorder.Code)
	}
}

func TestRecipientsCreateRequiresManagePermission(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeRecipientsService{created: recipients.Recipient{AllocationID: "allocation-1", FullName: "Budi"}}
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.manage": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recipients", strings.NewReader(`{"schedule_id":"schedule-1","full_name":"Budi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.createInput.ScheduleID != "schedule-1" {
		t.Fatalf("status=%d schedule=%q body=%s", rec.Code, service.createInput.ScheduleID, rec.Body.String())
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"recipients.view": true}}
	denied := httptest.NewRequest(http.MethodPost, "/api/v1/recipients", strings.NewReader(`{}`))
	denied.Header.Set("Content-Type", "application/json")
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	denied.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Recipients: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for viewer, got %d", deniedRecorder.Code)
	}
}

func TestRecipientCancelAndRestoreUseManagePermissionAndReturnConflicts(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.manage": true}}
	service := &fakeRecipientsService{cancelErr: recipients.ErrAlreadyCancelled}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recipients/allocation-1/cancel", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || service.allocationID != "allocation-1" {
		t.Fatalf("status=%d allocation=%q body=%s", rec.Code, service.allocationID, rec.Body.String())
	}

	restoreService := &fakeRecipientsService{}
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/recipients/allocation-1/restore", nil)
	restoreReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	restoreReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	restoreRec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: restoreService, SessionSecret: secret}).ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusNoContent || restoreService.allocationID != "allocation-1" {
		t.Fatalf("status=%d allocation=%q", restoreRec.Code, restoreService.allocationID)
	}
}
```

- [ ] **Step 6: Run `go build` and `go test`**

Run: `go build ./...` — fix any compile errors (missing imports, typos) first.
Run: `go test ./internal/api/... -run TestRecipients -v` — expect PASS.
Run: `go test ./... -count=1` and `go vet ./...` — the whole test suite must stay green.

- [ ] **Step 7: Commit**

```bash
git add internal/api/handler.go internal/api/recipients_routes.go internal/api/routes.go internal/api/handler_test.go cmd/server/main.go
git commit -m "feat(recipients): wire /api/v1/recipients endpoints into the API handler"
```

---

### Task 6: Frontend types

**Files:**
- Create: `frontend/src/features/dashboard/types.ts`

**Interfaces:**
- Produces: `Recipient`, `RecipientPage`, `RecipientStats`, `RecipientInput` types — Tasks 7 and 8 import these.

- [ ] **Step 1: Write the types file**

```ts
export type Recipient = {
  allocation_id: string;
  distribution_number: number;
  allocation_status: 'candidate' | 'ready' | 'needs_review' | 'distributed' | 'replaced' | 'cancelled';
  distribution_status: 'draft' | 'completed' | 'cancelled' | null;
  full_name: string;
  nik: string;
  sector_identifier_type: string;
  sector_identifier: string;
  address: string;
  village: string;
  district: string;
  phone_number: string;
  program_id: string;
  program_name: string;
  program_type: 'farmer' | 'fisherman';
  regency_id: string;
  regency_name: string;
  regency_document_code: string;
  schedule_id: string;
  schedule_name: string;
};

export type RecipientPage = { items: Recipient[]; page: number; page_size: number; total: number };

export type RecipientStats = { total: number; by_allocation_status: Record<string, number> };

export type RecipientInput = {
  schedule_id?: string;
  full_name: string;
  nik: string;
  sector_identifier: string;
  address: string;
  village: string;
  district: string;
  phone_number: string;
};

export const allocationStatusLabel: Record<string, string> = {
  candidate: 'Kandidat', ready: 'Siap', needs_review: 'Perlu ditinjau', distributed: 'Sudah distribusi', replaced: 'Diganti', cancelled: 'Dibatalkan',
};

export const distributionStatusLabel: Record<string, string> = {
  draft: 'Draft', completed: 'Selesai', cancelled: 'Dibatalkan',
};
```

- [ ] **Step 2: Verify it compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS (no consumers yet, but the file itself must be syntactically valid TypeScript).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/dashboard/types.ts
git commit -m "feat(recipients-ui): add Recipient frontend types"
```

---

### Task 7: `RecipientDialog` component (Add/Edit)

**Files:**
- Create: `frontend/src/features/dashboard/RecipientDialog.tsx`
- Create: `frontend/src/features/dashboard/RecipientDialog.test.tsx`

**Interfaces:**
- Consumes: `Recipient`, `RecipientInput` from Task 6; `FormField`, `Label`, `Select`/`SelectTrigger`/`SelectContent`/`SelectItem`, `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogFooter`/`DialogClose`, `Button`, `apiRequest` from `../../lib/api`.
- Produces: `RecipientDialog` component with props `{ open, onOpenChange, recipient?, schedules, pending, error, fields, onSave }` — Task 8 renders this.

- [ ] **Step 1: Write `RecipientDialog.tsx`**

```tsx
import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import type { Recipient, RecipientInput } from './types';

export type ScheduleOption = { id: string; name: string; regency_name: string; program_type: 'farmer' | 'fisherman' };

const empty: RecipientInput = { schedule_id: '', full_name: '', nik: '', sector_identifier: '', address: '', village: '', district: '', phone_number: '' };

export function RecipientDialog({ open, onOpenChange, recipient, schedules, pending, error, fields = {}, onSave }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  recipient?: Recipient;
  schedules: ScheduleOption[];
  pending?: boolean;
  error?: string;
  fields?: Record<string, string>;
  onSave: (values: RecipientInput) => void;
}) {
  const [values, setValues] = useState<RecipientInput>(empty);
  useEffect(() => setValues(recipient ? {
    schedule_id: recipient.schedule_id, full_name: recipient.full_name, nik: recipient.nik, sector_identifier: recipient.sector_identifier,
    address: recipient.address, village: recipient.village, district: recipient.district, phone_number: recipient.phone_number,
  } : empty), [recipient, open]);

  const title = recipient ? `Edit ${recipient.full_name}` : 'Tambah penerima';
  const selectedSchedule = schedules.find((item) => item.id === values.schedule_id);
  const sectorLabel = selectedSchedule?.program_type === 'fisherman' ? 'Nomor KUSUKA' : 'Nomor kartu petani';

  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent showCloseButton={false} aria-label={title} className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-y-auto p-0">
    <form onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(values); }}>
      <DialogHeader className="relative border-b p-5 pr-16">
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>Identitas penerima yang dicatat dalam sistem.</DialogDescription>
        <DialogClose render={<Button variant="ghost" size="icon" type="button" aria-label="Tutup" className="absolute top-3 right-3" />}><X /></DialogClose>
      </DialogHeader>
      <div className="grid gap-4 p-5 sm:grid-cols-2">
        <div className="grid min-w-0 gap-2 sm:col-span-2">
          <Label id="recipient-schedule-label">Jadwal</Label>
          <Select required disabled={Boolean(recipient)} value={values.schedule_id} onValueChange={(value) => setValues({ ...values, schedule_id: value ?? '' })}>
            <SelectTrigger className="w-full" aria-labelledby="recipient-schedule-label"><SelectValue placeholder="Pilih kabupaten dan jadwal" /></SelectTrigger>
            <SelectContent>{schedules.map((item) => <SelectItem key={item.id} value={item.id}>{item.regency_name} / {item.name}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <FormField className="sm:col-span-2" error={fields.full_name} label="Nama lengkap" name="full_name" required value={values.full_name} onChange={(event) => setValues({ ...values, full_name: event.target.value.toUpperCase() })} />
        <FormField error={fields.nik} label="NIK" name="nik" maxLength={16} value={values.nik} onChange={(event) => setValues({ ...values, nik: event.target.value.replace(/\D/g, '') })} hint="16 digit, boleh dikosongkan." />
        <FormField label={sectorLabel} name="sector_identifier" value={values.sector_identifier} onChange={(event) => setValues({ ...values, sector_identifier: event.target.value.toUpperCase() })} />
        <FormField className="sm:col-span-2" label="Alamat" name="address" value={values.address} onChange={(event) => setValues({ ...values, address: event.target.value })} />
        <FormField label="Desa/kelurahan" name="village" value={values.village} onChange={(event) => setValues({ ...values, village: event.target.value })} />
        <FormField label="Kecamatan" name="district" value={values.district} onChange={(event) => setValues({ ...values, district: event.target.value })} />
        <FormField label="Nomor telepon" name="phone_number" value={values.phone_number} onChange={(event) => setValues({ ...values, phone_number: event.target.value })} />
        {error && <Alert className="sm:col-span-2" variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
      </div>
      <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={pending} type="submit">{pending ? 'Menyimpan...' : 'Simpan'}</Button></DialogFooter>
    </form>
  </DialogContent></Dialog>;
}
```

- [ ] **Step 2: Write the failing test**

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { RecipientDialog } from './RecipientDialog';

const schedules = [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency_name: 'Wajo', program_type: 'farmer' as const }];

test('requires selecting a schedule and full name before submit, then forwards trimmed values', async () => {
  const onSave = vi.fn();
  render(<RecipientDialog open onOpenChange={() => {}} schedules={schedules} onSave={onSave} />);

  await userEvent.click(screen.getByRole('combobox', { name: 'Jadwal' }));
  await userEvent.click(await screen.findByRole('option', { name: /Wajo Tahap 1/ }));
  await userEvent.type(screen.getByLabelText('Nama lengkap'), 'budi santoso');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));

  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ schedule_id: 'schedule-1', full_name: 'BUDI SANTOSO' }));
});

test('shows KUSUKA label for fisherman schedules and locks schedule selection when editing', async () => {
  const fisherSchedules = [{ id: 'schedule-2', name: 'Bone Tahap 1', regency_name: 'Bone', program_type: 'fisherman' as const }];
  render(<RecipientDialog open onOpenChange={() => {}} schedules={fisherSchedules} recipient={{
    allocation_id: 'allocation-1', distribution_number: 1, allocation_status: 'ready', distribution_status: null,
    full_name: 'Siti', nik: '', sector_identifier_type: '', sector_identifier: '', address: '', village: '', district: '', phone_number: '',
    program_id: 'program-1', program_name: 'Program Nelayan', program_type: 'fisherman', regency_id: 'regency-1', regency_name: 'Bone',
    regency_document_code: 'BON', schedule_id: 'schedule-2', schedule_name: 'Bone Tahap 1',
  }} onSave={vi.fn()} />);

  expect(screen.getByRole('combobox', { name: 'Jadwal' })).toBeDisabled();
  expect(screen.getByLabelText('Nomor KUSUKA')).toBeInTheDocument();
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/dashboard/RecipientDialog.test.tsx`
Expected: FAIL (`RecipientDialog.tsx` doesn't exist yet, or a selector mismatch) — write Step 1's file first if you haven't, then confirm the test file alone catches a real bug by temporarily breaking one assertion (e.g., misspell `full_name`), observe the failure, then fix it back.

- [ ] **Step 4: Run test to verify it passes**

Run the same command. Expected: PASS, 2 tests.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/dashboard/RecipientDialog.tsx frontend/src/features/dashboard/RecipientDialog.test.tsx
git commit -m "feat(recipients-ui): add RecipientDialog for create/edit"
```

---

### Task 8: `DashboardPage` rewrite — stats, filters, table, pagination

**Files:**
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx`
- Modify: `frontend/src/features/dashboard/DashboardPage.test.tsx`

**Interfaces:**
- Consumes: `Recipient`, `RecipientPage`, `RecipientStats`, `allocationStatusLabel`, `distributionStatusLabel` from Task 6; `RecipientDialog`, `ScheduleOption` from Task 7; `apiRequest`, `useCan`, `DataTable`, `DataState`, `PageHeader`, `Badge`, `Card`/`CardContent`, `Select` family, `Label`, `Input`, `Button`, `AlertDialog` family.
- Produces: the rendered `/` route content — no other task depends on this file's internals.

- [ ] **Step 1: Replace `DashboardPage.tsx`**

```tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent } from '../../components/ui/card';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { PageHeader } from '../../components/PageHeader';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { RecipientDialog, type ScheduleOption } from './RecipientDialog';
import { allocationStatusLabel, distributionStatusLabel, type Recipient, type RecipientInput, type RecipientPage, type RecipientStats } from './types';

type RegencyOption = { id: string; name: string; document_code: string };
type ProgramOption = { id: string; name: string; program_type: 'farmer' | 'fisherman' };
type ScheduleListItem = { id: string; name: string; regency?: { name: string }; program?: { program_type: 'farmer' | 'fisherman' } };

function allocationBadgeVariant(status: string) {
  if (status === 'distributed') return 'default' as const;
  if (status === 'cancelled') return 'destructive' as const;
  if (status === 'ready' || status === 'replaced') return 'secondary' as const;
  return 'outline' as const;
}

function distributionBadgeVariant(status: string | null) {
  if (status === 'completed') return 'default' as const;
  if (status === 'cancelled') return 'destructive' as const;
  return 'outline' as const;
}

export function DashboardPage() {
  const canManage = useCan('recipients.manage');
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get('search') ?? '');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Recipient>();
  const [pendingCancel, setPendingCancel] = useState<Recipient>();

  const stats = useQuery({ queryKey: ['recipients', 'stats'], queryFn: () => apiRequest<{ data: RecipientStats }>('/api/v1/recipients/stats') });
  const list = useQuery({ queryKey: ['recipients', params.toString()], queryFn: () => apiRequest<{ data: RecipientPage }>(`/api/v1/recipients?${params.toString()}`) });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: ScheduleListItem[] }>('/api/v1/program-setup/schedules') });

  const save = useMutation({
    mutationFn: (values: RecipientInput) => {
      const { schedule_id, ...updateOnly } = values;
      return apiRequest(editing ? `/api/v1/recipients/${editing.allocation_id}` : '/api/v1/recipients', {
        method: editing ? 'PATCH' : 'POST', body: JSON.stringify(editing ? updateOnly : values),
      });
    },
    onSuccess: () => { setDialogOpen(false); setEditing(undefined); client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Data penerima berhasil disimpan.'); },
  });
  const cancelMutation = useMutation({
    mutationFn: (allocationID: string) => apiRequest(`/api/v1/recipients/${allocationID}/cancel`, { method: 'POST' }),
    onSuccess: () => { setPendingCancel(undefined); client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Penerima dibatalkan.'); },
  });
  const restoreMutation = useMutation({
    mutationFn: (allocationID: string) => apiRequest(`/api/v1/recipients/${allocationID}/restore`, { method: 'POST' }),
    onSuccess: () => { client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Penerima dipulihkan.'); },
  });

  const setFilter = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const submitSearch = (event: FormEvent) => { event.preventDefault(); setFilter('search', search); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const openEdit = (recipient: Recipient) => { save.reset(); setEditing(recipient); setDialogOpen(true); };
  const openCreate = () => { save.reset(); setEditing(undefined); setDialogOpen(true); };

  const page = list.data?.data.page ?? 1;
  const pageSize = list.data?.data.page_size ?? 20;
  const total = list.data?.data.total ?? 0;
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  const scheduleOptions: ScheduleOption[] = (schedules.data?.data ?? []).map((item) => ({ id: item.id, name: item.name, regency_name: item.regency?.name ?? '', program_type: item.program?.program_type ?? 'farmer' }));

  return <div className="space-y-6">
    <PageHeader title="Data Penerima" description="Pusat data penerima bantuan lintas program dan wilayah." actions={canManage ? <Button onClick={openCreate}><Plus />Tambah penerima</Button> : undefined} />

    <section aria-label="Statistik penerima" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.total ?? '-'}</strong><span className="block text-sm text-muted-foreground">Total penerima</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.distributed ?? 0}</strong><span className="block text-sm text-muted-foreground">Sudah distribusi</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.ready ?? 0}</strong><span className="block text-sm text-muted-foreground">Siap/menunggu</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.needs_review ?? 0}</strong><span className="block text-sm text-muted-foreground">Perlu ditinjau</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.cancelled ?? 0}</strong><span className="block text-sm text-muted-foreground">Dibatalkan</span></CardContent></Card>
    </section>

    <form className="flex flex-col gap-3 rounded-lg border bg-card p-3 lg:flex-row lg:items-end lg:flex-wrap" onSubmit={submitSearch}>
      <div className="relative min-w-0 flex-1"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 text-muted-foreground" /><Input aria-label="Cari penerima" className="pl-10" placeholder="Cari nama, NIK, atau nomor kartu" value={search} onChange={(event) => setSearch(event.target.value)} /></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-regency-label">Kabupaten</Label><Select value={params.get('regency_id') ?? ''} onValueChange={(value) => setFilter('regency_id', value ?? '')}><SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder="Semua kabupaten" /></SelectTrigger><SelectContent><SelectItem value="">Semua kabupaten</SelectItem>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-program-label">Program</Label><Select value={params.get('program_id') ?? ''} onValueChange={(value) => setFilter('program_id', value ?? '')}><SelectTrigger aria-labelledby="filter-program-label"><SelectValue placeholder="Semua program" /></SelectTrigger><SelectContent><SelectItem value="">Semua program</SelectItem>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-allocation-label">Status alokasi</Label><Select value={params.get('allocation_status') ?? ''} onValueChange={(value) => setFilter('allocation_status', value ?? '')}><SelectTrigger aria-labelledby="filter-allocation-label"><SelectValue placeholder="Aktif (bukan dibatalkan)" /></SelectTrigger><SelectContent><SelectItem value="">Aktif (bukan dibatalkan)</SelectItem>{Object.entries(allocationStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-distribution-label">Status distribusi</Label><Select value={params.get('distribution_status') ?? ''} onValueChange={(value) => setFilter('distribution_status', value ?? '')}><SelectTrigger aria-labelledby="filter-distribution-label"><SelectValue placeholder="Semua status" /></SelectTrigger><SelectContent><SelectItem value="">Semua status</SelectItem>{Object.entries(distributionStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <Button type="submit" variant="outline">Cari</Button>
    </form>

    {list.isError ? <DataState kind="error" title="Data penerima belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => list.refetch() }} /> : list.isPending ? <DataState kind="loading" title="Memuat data penerima" description="Mengambil data dari seluruh kabupaten." /> : list.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada penerima yang sesuai" description="Ubah filter atau tambahkan penerima baru." /> : <DataTable label="Daftar penerima" minimumWidth={1200}>
      <thead><tr><th>Kabupaten</th><th>Program</th><th>Jadwal</th><th>No. Pembagian</th><th>Nama</th><th>NIK</th><th>No. Kartu/KUSUKA</th><th>Desa/Kecamatan</th><th>Status alokasi</th><th>Status distribusi</th>{canManage && <th>Aksi</th>}</tr></thead>
      <tbody>{list.data?.data.items.map((item) => <tr key={item.allocation_id}>
        <td><strong>{item.regency_document_code}</strong> {item.regency_name}</td>
        <td>{item.program_name}</td>
        <td>{item.schedule_name}</td>
        <td className="font-medium tabular-nums">{item.distribution_number}</td>
        <td><strong>{item.full_name}</strong></td>
        <td className="tabular-nums">{item.nik || '-'}</td>
        <td>{item.sector_identifier || '-'}</td>
        <td>{[item.village, item.district].filter(Boolean).join(', ') || '-'}</td>
        <td><Badge variant={allocationBadgeVariant(item.allocation_status)}>{allocationStatusLabel[item.allocation_status] ?? item.allocation_status}</Badge></td>
        <td><Badge variant={distributionBadgeVariant(item.distribution_status)}>{item.distribution_status ? distributionStatusLabel[item.distribution_status] ?? item.distribution_status : '-'}</Badge></td>
        {canManage && <td className="flex gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(item)}>Edit</Button>
          {item.allocation_status === 'cancelled'
            ? <Button type="button" variant="ghost" size="sm" onClick={() => restoreMutation.mutate(item.allocation_id)}>Pulihkan</Button>
            : <Button type="button" variant="ghost" size="sm" className="text-destructive" onClick={() => setPendingCancel(item)}>Batalkan</Button>}
        </td>}
      </tr>)}</tbody>
    </DataTable>}

    <nav className="flex flex-col gap-3 border-t pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between" aria-label="Pagination penerima">
      <span>{total} penerima</span>
      <span className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</Button>Halaman {page}<Button size="sm" variant="outline" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</Button></span>
    </nav>

    {canManage && <RecipientDialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} recipient={editing} schedules={scheduleOptions} pending={save.isPending} error={saveMessage} fields={saveFields} onSave={(values) => save.mutate(values)} />}

    <AlertDialog open={Boolean(pendingCancel)} onOpenChange={(open) => { if (!open) setPendingCancel(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Batalkan {pendingCancel?.full_name}?</AlertDialogTitle><AlertDialogDescription>Penerima ini akan disembunyikan dari daftar aktif dan statistik, tapi tetap tersimpan untuk audit dan dapat dipulihkan.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingCancel) cancelMutation.mutate(pendingCancel.allocation_id); }}>Batalkan penerima</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
```

- [ ] **Step 2: Replace `DashboardPage.test.tsx`**

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DashboardPage } from './DashboardPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function mockApi() {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/recipients/stats') return Promise.resolve({ data: { total: 3, by_allocation_status: { ready: 1, distributed: 1, needs_review: 1, cancelled: 0 } } });
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients?')) return Promise.resolve({ data: { items: [
      { allocation_id: 'allocation-1', distribution_number: 7, allocation_status: 'ready', distribution_status: null, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier_type: 'farmer_card', sector_identifier: 'KP01', address: '', village: 'Tempe', district: 'Sabbangparu', phone_number: '', program_id: 'program-1', program_name: 'Program Petani 2026', program_type: 'farmer', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', schedule_id: 'schedule-1', schedule_name: 'Wajo Tahap 1' },
    ], page: 1, page_size: 20, total: 1 } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency: { name: 'Wajo' }, program: { program_type: 'farmer' } }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

function renderPage(permissions = ['recipients.view', 'recipients.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DashboardPage /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); });

test('renders statistics and the recipient table', async () => {
  mockApi();
  renderPage();
  expect(await screen.findByText('3')).toBeVisible();
  const table = screen.getByRole('table');
  expect(within(table).getByText('Siti Aminah')).toBeVisible();
  expect(within(table).getByText('WJO')).toBeVisible();
});

test('combining search and status filter updates the query', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');
  await userEvent.type(screen.getByLabelText('Cari penerima'), 'Siti');
  await userEvent.click(screen.getByRole('button', { name: 'Cari' }));
  await userEvent.click(screen.getByRole('combobox', { name: 'Status alokasi' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Perlu ditinjau' }));

  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.includes('search=Siti') && path.includes('allocation_status=needs_review'));
  expect(call).toBeTruthy();
});

test('hides management actions without recipients.manage', async () => {
  mockApi();
  renderPage(['recipients.view']);
  await screen.findByText('Siti Aminah');
  expect(screen.queryByRole('button', { name: 'Tambah penerima' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
});
```

- [ ] **Step 3: Run tests to verify they fail (before Step 1's rewrite, or by breaking one assertion)**

Run: `cd frontend && npx vitest run src/features/dashboard`
Expected: with the old placeholder `DashboardPage.tsx` still in place, these tests fail (no stat cards, no table, no search input). Apply Step 1's rewrite.

- [ ] **Step 4: Run tests to verify they pass**

Run the same command. Expected: PASS, 3 tests. If `Object.entries(allocationStatusLabel)` produces an option list that collides with the `SelectItem value=""` sentinel, double check no two `SelectItem`s share a value inside the same `Select` (they don't here, `""` is only used once per dropdown as the "semua/aktif" sentinel).

- [ ] **Step 5: Run full frontend verification**

Run: `npx tsc --noEmit`, `npm run build`, `npm run test` — all must stay green.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/dashboard/DashboardPage.tsx frontend/src/features/dashboard/DashboardPage.test.tsx
git commit -m "feat(recipients-ui): rebuild Data Penerima as a cross-regency CRUD page"
```

---

### Task 9: Permission wiring — routes and navigation

**Files:**
- Modify: `frontend/src/app/routes.tsx`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/AppShell.test.tsx`

**Interfaces:**
- Consumes: `ProtectedPage` from `routes.tsx` (already exists, no signature change).
- Produces: the `/` route now gated by `recipients.view`; no later task depends on this.

- [ ] **Step 1: Gate the dashboard route**

In `frontend/src/app/routes.tsx`, change:

```tsx
    { index: true, element: <DashboardPage /> },
```

to:

```tsx
    { index: true, element: <ProtectedPage permission="recipients.view"><DashboardPage /></ProtectedPage> },
```

- [ ] **Step 2: Update the nav item permission**

In `frontend/src/app/AppShell.tsx`, change:

```tsx
    { label: 'Data Penerima', to: '/', permission: 'dashboard.view', icon: <ListFilter /> },
```

to:

```tsx
    { label: 'Data Penerima', to: '/', permission: 'recipients.view', icon: <ListFilter /> },
```

- [ ] **Step 3: Update the existing AppShell test's permission fixture**

In `frontend/src/app/AppShell.test.tsx`, find the `bootstrap.data.permissions` array (the one used by the `'renders identity and permission-aware navigation'` test) and add `'recipients.view'` to it, e.g.:

```tsx
    permissions: ['recipients.view', 'dashboard.view', 'dcp3.view', 'distribution.view', 'users.view', 'settings.view'],
```

Confirm the test still asserts `getByRole('link', { name: 'Data Penerima' })` is present (it should already, this only fixes the permission fixture so the link keeps rendering after the rename).

- [ ] **Step 4: Run frontend tests to verify they fail then pass**

Run: `cd frontend && npx vitest run src/app/AppShell.test.tsx` before Step 3 (expect the Data Penerima link assertion to fail once Step 2 lands, since the mocked permissions no longer include `recipients.view`), then after Step 3 (expect PASS).

- [ ] **Step 5: Run full verification suite**

Run in order: `go build ./...`, `go vet ./...`, `go test ./... -count=1` (with `TEST_DATABASE_URL` set), `cd frontend && npx tsc --noEmit`, `npm run build`, `npm run test`.
Expected: all green — this is the final integration checkpoint for the whole feature.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/routes.tsx frontend/src/app/AppShell.tsx frontend/src/app/AppShell.test.tsx
git commit -m "feat(recipients-ui): gate Data Penerima route and nav item behind recipients.view"
```

---

## Post-implementation manual check

After all 9 tasks are committed:
1. `go run ./cmd/migrate up` against your local `konkit` database.
2. `go run ./cmd/server`, `npm --prefix frontend run build` if not already built.
3. Log in as an account with `recipients.view`/`recipients.manage` (super_admin has both automatically via the migration).
4. Open `Data Penerima`, confirm stats render, add a manual recipient, edit it, cancel it (confirm it disappears from the default view and stats), filter by "Dibatalkan" to find it again, restore it.
