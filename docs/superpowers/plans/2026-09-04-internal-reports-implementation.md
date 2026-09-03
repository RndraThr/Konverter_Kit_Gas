# Internal Reports Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only, per-jadwal Laporan module that summarizes allocation/distribution/documentation status for a program schedule, lists the underlying rows with filters, and exports the filtered view to Excel and PDF.

**Architecture:** Add a new bounded domain `internal/reports` that queries the existing `programs`/`dcp3`/`distribution` tables (no new tables, no new permissions) through a single filtered CTE, expose it over `/api/v1/reports/schedule/{schedule_id}/*` guarded by the existing `distribution.view` permission, and consume it from a new React feature that mirrors the schedule-scoped pattern already used by Pendistribusian.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgx v5, `github.com/xuri/excelize/v2` (already a dependency) for Excel export, `github.com/go-pdf/fpdf` (new dependency) for PDF export, React 19, TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-internal-reports-design.md`

## Global Constraints

- Laporan is scoped to exactly one `schedule_id` per request; there is no cross-schedule aggregate view.
- No new database tables and no new permission. Every endpoint authorizes against the existing `distribution.view` permission.
- NIK is shown in full in this module (unlike Pendistribusian, which masks it) — do not mask NIK in reports responses or exports.
- Filters (`allocation_status`, `distribution_status`, `documentation_status`) apply identically to the summary, the row listing, and both export formats — never let an export silently ignore an active filter.
- Every export request (Excel or PDF) writes an audit event `reports.exported` with actor, schedule, format, and the active filter — never with row data.
- PDF export uses a pure-Go PDF library (`github.com/go-pdf/fpdf`), not a headless-browser HTML-to-PDF renderer.
- Follow existing code patterns exactly: domain package with `models.go`/`service.go`/`repository.go`, `writeData`/`writeError`/`writeServiceError` response helpers, `auth.Principal`/`auth.ClientMeta` signatures, and the `apiRequest`/TanStack Query frontend conventions already used by `distribution` and `programs` features.

---

### Task 1: Reports Domain Models and Service

**Files:**
- Create: `internal/reports/models.go`
- Create: `internal/reports/service.go`
- Create: `internal/reports/service_test.go`

**Interfaces:**
- Produces `reports.Filter{AllocationStatus, DistributionStatus, DocumentationStatus string}`.
- Produces `reports.Summary{TotalAllocations int, AllocationStatusCounts, DistributionStatusCounts []StatusCount, DocumentationIncomplete int}` and `reports.StatusCount{Status string, Count int}`.
- Produces `reports.Row{DistributionNumber int, FullName, NIK, SectorIdentifier, Village, District, AllocationStatus, DistributionStatus string, DocumentationComplete bool, CompletedAt *time.Time}`.
- Produces `reports.NewService(repository) *Service` with `Summary(ctx, scheduleID string, filter Filter) (Summary, error)` and `Rows(ctx, scheduleID string, filter Filter) ([]Row, error)`.
- Consumed by Task 4 (API routes) and Task 3 (export generation, which extends this same service).

- [ ] **Step 1: Write the failing service tests**

```go
package reports

import (
	"context"
	"errors"
	"testing"
)

type repositoryStub struct {
	summary    Summary
	rows       []Row
	scheduleID string
	filter     Filter
}

func (r *repositoryStub) Summary(_ context.Context, scheduleID string, filter Filter) (Summary, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.summary, nil
}

func (r *repositoryStub) Rows(_ context.Context, scheduleID string, filter Filter) ([]Row, error) {
	r.scheduleID, r.filter = scheduleID, filter
	return r.rows, nil
}

func TestSummaryRequiresScheduleAndValidatesFilter(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Summary(context.Background(), "", Filter{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{AllocationStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid allocation status err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{DistributionStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid distribution status err=%v", err)
	}
	if _, err := service.Summary(context.Background(), "schedule-1", Filter{DocumentationStatus: "bogus"}); !errors.Is(err, ErrFilterInvalid) {
		t.Fatalf("invalid documentation status err=%v", err)
	}
}

func TestRowsRequiresScheduleAndTrimsInput(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Rows(context.Background(), "   ", Filter{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
}

func TestRowsPassesTrimmedScheduleAndFilterToRepository(t *testing.T) {
	repository := &repositoryStub{rows: []Row{{DistributionNumber: 7, FullName: "Siti Aminah"}}}
	service := NewService(repository)

	rows, err := service.Rows(context.Background(), " schedule-1 ", Filter{AllocationStatus: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleID != "schedule-1" || repository.filter.AllocationStatus != "ready" {
		t.Fatalf("schedule=%q filter=%+v", repository.scheduleID, repository.filter)
	}
	if len(rows) != 1 || rows[0].FullName != "Siti Aminah" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestSummaryPassesTrimmedScheduleAndFilterToRepository(t *testing.T) {
	repository := &repositoryStub{summary: Summary{TotalAllocations: 3}}
	service := NewService(repository)

	summary, err := service.Summary(context.Background(), " schedule-1 ", Filter{DocumentationStatus: "incomplete"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.scheduleID != "schedule-1" || repository.filter.DocumentationStatus != "incomplete" {
		t.Fatalf("schedule=%q filter=%+v", repository.scheduleID, repository.filter)
	}
	if summary.TotalAllocations != 3 {
		t.Fatalf("summary=%+v", summary)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/reports -count=1`

Expected: FAIL because the `reports` package does not exist.

- [ ] **Step 3: Implement models**

Create `internal/reports/models.go`:

```go
package reports

import (
	"errors"
	"time"
)

var (
	ErrScheduleRequired = errors.New("report schedule is required")
	ErrFilterInvalid    = errors.New("report filter is invalid")
)

var allocationStatuses = map[string]bool{
	"candidate": true, "ready": true, "needs_review": true,
	"distributed": true, "replaced": true, "cancelled": true,
}

var distributionStatuses = map[string]bool{
	"draft": true, "completed": true, "cancelled": true,
}

var documentationStatuses = map[string]bool{
	"complete": true, "incomplete": true,
}

type Filter struct {
	AllocationStatus    string
	DistributionStatus  string
	DocumentationStatus string
}

func (f Filter) validate() error {
	if f.AllocationStatus != "" && !allocationStatuses[f.AllocationStatus] {
		return ErrFilterInvalid
	}
	if f.DistributionStatus != "" && !distributionStatuses[f.DistributionStatus] {
		return ErrFilterInvalid
	}
	if f.DocumentationStatus != "" && !documentationStatuses[f.DocumentationStatus] {
		return ErrFilterInvalid
	}
	return nil
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type Summary struct {
	TotalAllocations         int           `json:"total_allocations"`
	AllocationStatusCounts   []StatusCount `json:"allocation_status_counts"`
	DistributionStatusCounts []StatusCount `json:"distribution_status_counts"`
	DocumentationIncomplete  int           `json:"documentation_incomplete"`
}

type Row struct {
	DistributionNumber    int        `json:"distribution_number"`
	FullName              string     `json:"full_name"`
	NIK                   string     `json:"nik"`
	SectorIdentifier      string     `json:"sector_identifier"`
	Village               string     `json:"village"`
	District              string     `json:"district"`
	AllocationStatus      string     `json:"allocation_status"`
	DistributionStatus    string     `json:"distribution_status"`
	DocumentationComplete bool       `json:"documentation_complete"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
}
```

- [ ] **Step 4: Implement the service**

Create `internal/reports/service.go`:

```go
package reports

import (
	"context"
	"strings"
)

type repository interface {
	Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error)
	Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error)
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return Summary{}, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return Summary{}, err
	}
	return s.repository.Summary(ctx, scheduleID, filter)
}

func (s *Service) Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if err := filter.validate(); err != nil {
		return nil, err
	}
	return s.repository.Rows(ctx, scheduleID, filter)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/reports -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/reports
git commit -m "feat: add reports domain models and service"
```

---

### Task 2: Reports Repository — Summary and Rows Queries

**Files:**
- Create: `internal/reports/repository.go`
- Create: `internal/reports/repository_integration_test.go`

**Interfaces:**
- Produces `reports.NewRepository(pool *pgxpool.Pool) *Repository` implementing the `repository` interface from Task 1.
- Consumed by Task 4 (dependency wiring) and extended by Task 3 (`RecordExport`).

- [ ] **Step 1: Write the failing integration test**

Create `internal/reports/repository_integration_test.go`:

```go
package reports

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationSummaryAndRowsReflectAllocationsAndFilters(t *testing.T) {
	pool := reportsIntegrationPool(t)
	fixture := createReportsFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	summary, err := repository.Summary(ctx, fixture.scheduleID, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalAllocations != 3 {
		t.Fatalf("total=%d summary=%+v", summary.TotalAllocations, summary)
	}
	if summary.DocumentationIncomplete != 2 {
		t.Fatalf("documentation incomplete=%d summary=%+v", summary.DocumentationIncomplete, summary)
	}

	rows, err := repository.Rows(ctx, fixture.scheduleID, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].DistributionNumber != 1 || rows[0].NIK != fixture.primaryNIK {
		t.Fatalf("first row=%+v want nik=%q", rows[0], fixture.primaryNIK)
	}
	if !rows[1].DocumentationComplete || rows[1].DistributionStatus != "completed" {
		t.Fatalf("second row=%+v", rows[1])
	}

	filtered, err := repository.Rows(ctx, fixture.scheduleID, Filter{DocumentationStatus: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].DistributionNumber != 2 {
		t.Fatalf("filtered rows=%+v", filtered)
	}

	byAllocation, err := repository.Rows(ctx, fixture.scheduleID, Filter{AllocationStatus: "needs_review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byAllocation) != 1 || byAllocation[0].DistributionNumber != 3 {
		t.Fatalf("allocation-filtered rows=%+v", byAllocation)
	}
}

type reportsFixture struct {
	scheduleID string
	primaryNIK string
}

// insertAllocation generates its own 16-digit NIK per call (rather than a
// fixed literal) because internal/distribution's integration tests already
// use fixed NIKs like "7306014101900001" against the same shared
// konkit_test database, and Go runs different packages' tests in parallel
// by default — a hardcoded NIK here would intermittently collide with the
// unique index on people.nik.
func createReportsFixture(t *testing.T, pool *pgxpool.Pool) reportsFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())

	var packageID, documentationID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM package_template_versions WHERE template_code='PETANI-LPG' AND version=1`).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code='DOK-PETANI' AND version=1`).Scan(&documentationID); err != nil {
		t.Fatal(err)
	}

	var programID, regencyID, scheduleID string
	if err := pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Laporan Test','farmer',2026,'active') RETURNING id::text`, "RPT-"+suffix).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan',$1,$2) RETURNING id::text`, "Wajo Laporan "+suffix, codeFromSuffix("R", suffix)).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,'Wajo Tahap Laporan','2026-09-01','2026-09-30','active') RETURNING id::text`, programID, regencyID, packageID, documentationID).Scan(&scheduleID); err != nil {
		t.Fatal(err)
	}

	personIDs := make([]string, 0, 3)
	insertAllocation := func(number int, personName, allocationStatus, distributionStatus string, documentationComplete bool) (personID, nik string) {
		nik = fmt.Sprintf("9%015d", time.Now().UnixNano()%1_000_000_000_000_000)
		if err := pool.QueryRow(ctx, `INSERT INTO people(full_name,nik,village,district) VALUES($1,$2,'Tempe','Sabbangparu') RETURNING id::text`, personName, nik).Scan(&personID); err != nil {
			t.Fatal(err)
		}
		personIDs = append(personIDs, personID)
		var batchID, rowID, nominationID, allocationID string
		if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,status) VALUES($1,$2,$3,'Penerima','imported') RETURNING id::text`, scheduleID, fmt.Sprintf("laporan-%d.xlsx", number), fmt.Sprintf("%064d", time.Now().UnixNano()+int64(number))).Scan(&batchID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,source_sequence_number,raw_data_json,validation_status) VALUES($1,2,$2,'{}','valid') RETURNING id::text`, batchID, number).Scan(&rowID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO candidate_nominations(batch_id,import_row_id,person_id,program_type,source_snapshot_json,status) VALUES($1,$2,$3,'farmer','{}','ready') RETURNING id::text`, batchID, rowID, personID).Scan(&nominationID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,$5,'{}') RETURNING id::text`, scheduleID, nominationID, personID, number, allocationStatus).Scan(&allocationID); err != nil {
			t.Fatal(err)
		}
		var distributionID string
		if err := pool.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id,status) VALUES($1,$2,$3) RETURNING id::text`, allocationID, personID, distributionStatus).Scan(&distributionID); err != nil {
			t.Fatal(err)
		}
		slotStatus := "missing"
		if documentationComplete {
			slotStatus = "complete"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,status) VALUES($1,'recipient_package','Penerima dan paket',true,1,1,'both',$2)`, distributionID, slotStatus); err != nil {
			t.Fatal(err)
		}
		return personID, nik
	}

	_, primaryNIK := insertAllocation(1, "Siti Aminah", "ready", "draft", false)
	insertAllocation(2, "Siti Nur", "distributed", "completed", true)
	insertAllocation(3, "Aminah Wati", "needs_review", "draft", false)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slots WHERE distribution_id IN (SELECT d.id FROM distribution_records d JOIN package_allocations a ON a.id=d.allocation_id WHERE a.schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE resource_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_records WHERE allocation_id IN (SELECT id FROM package_allocations WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM package_allocations WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM candidate_nominations WHERE batch_id IN (SELECT id FROM dcp3_import_batches WHERE schedule_id=$1)`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dcp3_import_batches WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM people WHERE id=ANY($1)`, personIDs)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
	})

	return reportsFixture{scheduleID: scheduleID, primaryNIK: primaryNIK}
}

func codeFromSuffix(prefix, suffix string) string {
	return prefix + string(rune('A'+suffix[len(suffix)-2]%20)) + string(rune('A'+suffix[len(suffix)-1]%20))
}

func reportsIntegrationPool(t *testing.T) *pgxpool.Pool {
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/reports -run TestIntegrationSummaryAndRowsReflectAllocationsAndFilters -count=1`

Expected: FAIL (skips if `TEST_DATABASE_URL` unset; otherwise fails because `Repository` does not exist).

- [ ] **Step 3: Implement the repository**

Create `internal/reports/repository.go`:

```go
package reports

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const reportsBaseCTE = `
WITH base AS (
	SELECT a.distribution_number, a.status AS allocation_status,
		COALESCE(dr.status,'draft') AS distribution_status,
		COALESCE(p.full_name,'Data perlu ditinjau') AS full_name,
		COALESCE(p.nik,'') AS nik,
		COALESCE(psi.display_value,'') AS sector_identifier,
		COALESCE(p.village,'') AS village,
		COALESCE(p.district,'') AS district,
		dr.completed_at,
		NOT EXISTS (
			SELECT 1 FROM documentation_slots ds
			WHERE ds.distribution_id = dr.id AND ds.is_required AND ds.status <> 'complete'
		) AS documentation_complete
	FROM package_allocations a
	JOIN candidate_nominations n ON n.id = a.nomination_id
	JOIN program_schedules ps ON ps.id = a.schedule_id
	JOIN programs pr ON pr.id = ps.program_id
	LEFT JOIN distribution_records dr ON dr.allocation_id = a.id
	LEFT JOIN people p ON p.id = COALESCE(a.actual_recipient_person_id, a.intended_person_id, n.person_id)
	LEFT JOIN LATERAL (
		SELECT display_value FROM person_sector_identifiers
		WHERE person_id = p.id
			AND identifier_type = CASE WHEN pr.program_type = 'farmer' THEN 'farmer_card' ELSE 'kusuka' END
		LIMIT 1
	) psi ON true
	WHERE a.schedule_id = $1
)
`

const reportsFilterClause = `
WHERE ($2 = '' OR allocation_status = $2)
	AND ($3 = '' OR distribution_status = $3)
	AND ($4 = '' OR ($4 = 'complete' AND documentation_complete) OR ($4 = 'incomplete' AND NOT documentation_complete))
`

func (r *Repository) Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error) {
	query := reportsBaseCTE + `
SELECT count(*),
	count(*) FILTER (WHERE allocation_status = 'candidate'),
	count(*) FILTER (WHERE allocation_status = 'ready'),
	count(*) FILTER (WHERE allocation_status = 'needs_review'),
	count(*) FILTER (WHERE allocation_status = 'distributed'),
	count(*) FILTER (WHERE allocation_status = 'replaced'),
	count(*) FILTER (WHERE allocation_status = 'cancelled'),
	count(*) FILTER (WHERE distribution_status = 'draft'),
	count(*) FILTER (WHERE distribution_status = 'completed'),
	count(*) FILTER (WHERE distribution_status = 'cancelled'),
	count(*) FILTER (WHERE NOT documentation_complete)
FROM base
` + reportsFilterClause

	var summary Summary
	var candidate, ready, needsReview, distributed, replaced, cancelled int
	var draft, completed, distributionCancelled int
	err := r.pool.QueryRow(ctx, query, scheduleID, filter.AllocationStatus, filter.DistributionStatus, filter.DocumentationStatus).Scan(
		&summary.TotalAllocations, &candidate, &ready, &needsReview, &distributed, &replaced, &cancelled,
		&draft, &completed, &distributionCancelled, &summary.DocumentationIncomplete,
	)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize report: %w", err)
	}
	summary.AllocationStatusCounts = []StatusCount{
		{Status: "candidate", Count: candidate}, {Status: "ready", Count: ready}, {Status: "needs_review", Count: needsReview},
		{Status: "distributed", Count: distributed}, {Status: "replaced", Count: replaced}, {Status: "cancelled", Count: cancelled},
	}
	summary.DistributionStatusCounts = []StatusCount{
		{Status: "draft", Count: draft}, {Status: "completed", Count: completed}, {Status: "cancelled", Count: distributionCancelled},
	}
	return summary, nil
}

func (r *Repository) Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error) {
	query := reportsBaseCTE + `
SELECT distribution_number, allocation_status, distribution_status, full_name, nik, sector_identifier, village, district, completed_at, documentation_complete
FROM base
` + reportsFilterClause + `
ORDER BY distribution_number
`
	rows, err := r.pool.Query(ctx, query, scheduleID, filter.AllocationStatus, filter.DistributionStatus, filter.DocumentationStatus)
	if err != nil {
		return nil, fmt.Errorf("list report rows: %w", err)
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var item Row
		if err := rows.Scan(&item.DistributionNumber, &item.AllocationStatus, &item.DistributionStatus, &item.FullName, &item.NIK, &item.SectorIdentifier, &item.Village, &item.District, &item.CompletedAt, &item.DocumentationComplete); err != nil {
			return nil, fmt.Errorf("scan report row: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Apply migrations and run the integration test**

Run:

```powershell
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
$env:GOCACHE=(Join-Path (Get-Location) '.cache\go-build')
go test ./internal/reports -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/reports
git commit -m "feat: add reports repository queries"
```

---

### Task 3: Excel and PDF Export Generation

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/reports/export.go`
- Create: `internal/reports/export_test.go`
- Modify: `internal/reports/service.go`
- Modify: `internal/reports/service_test.go`
- Modify: `internal/reports/repository.go`
- Modify: `internal/reports/repository_integration_test.go`

**Interfaces:**
- Consumes: `Row`, `Summary` from Task 1.
- Produces `reports.Service.ExportExcel(ctx, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta) ([]byte, error)` and `reports.Service.ExportPDF(...)` with the same signature.
- Produces `reports.Repository.RecordExport(ctx, actor auth.Principal, scheduleID, format string, filter Filter, meta auth.ClientMeta) error`, writing audit event `reports.exported`.
- Consumed by Task 4 (API routes).

- [ ] **Step 1: Add the PDF dependency**

Run: `go get github.com/go-pdf/fpdf@latest`

Expected: `go.mod` and `go.sum` include `github.com/go-pdf/fpdf`.

- [ ] **Step 2: Write the failing export tests**

Create `internal/reports/export_test.go`:

```go
package reports

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestBuildExcelWritesHeaderAndRows(t *testing.T) {
	completedAt := time.Date(2026, 9, 10, 8, 30, 0, 0, time.UTC)
	data, err := buildExcel([]Row{
		{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001", SectorIdentifier: "KP01", Village: "Tempe", District: "Sabbangparu", AllocationStatus: "distributed", DistributionStatus: "completed", DocumentationComplete: true, CompletedAt: &completedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := file.GetRows("Laporan")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
	if rows[0][0] != "No. Pembagian" || rows[0][1] != "Nama" {
		t.Fatalf("header=%v", rows[0])
	}
	if rows[1][1] != "Siti Aminah" || rows[1][2] != "7306014101900001" || rows[1][8] != "Lengkap" {
		t.Fatalf("data row=%v", rows[1])
	}
}

func TestBuildPDFProducesNonEmptyDocument(t *testing.T) {
	summary := Summary{TotalAllocations: 1, AllocationStatusCounts: []StatusCount{{Status: "distributed", Count: 1}}, DistributionStatusCounts: []StatusCount{{Status: "completed", Count: 1}}}
	data, err := buildPDF(summary, []Row{{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("not a pdf, first bytes=%q", data[:min(len(data), 16)])
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/reports -run 'TestBuildExcel|TestBuildPDF' -count=1`

Expected: FAIL because `buildExcel` and `buildPDF` do not exist.

- [ ] **Step 4: Implement export generation**

Create `internal/reports/export.go`:

```go
package reports

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

var reportColumnHeaders = []string{
	"No. Pembagian", "Nama", "NIK", "No. Kartu Petani/KUSUKA", "Desa", "Kecamatan",
	"Status Alokasi", "Status Distribusi", "Dokumentasi", "Tanggal Selesai",
}

func reportRowValues(row Row) []string {
	return []string{
		fmt.Sprint(row.DistributionNumber), row.FullName, row.NIK, row.SectorIdentifier,
		row.Village, row.District, row.AllocationStatus, row.DistributionStatus,
		documentationLabel(row.DocumentationComplete), formatCompletedAt(row.CompletedAt),
	}
}

func documentationLabel(complete bool) string {
	if complete {
		return "Lengkap"
	}
	return "Belum lengkap"
}

func formatCompletedAt(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04")
}

func buildExcel(rows []Row) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	const sheet = "Laporan"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("name report sheet: %w", err)
	}
	for column, header := range reportColumnHeaders {
		cell, _ := excelize.CoordinatesToCellName(column+1, 1)
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, fmt.Errorf("write report header: %w", err)
		}
	}
	for index, row := range rows {
		for column, value := range reportRowValues(row) {
			cell, _ := excelize.CoordinatesToCellName(column+1, index+2)
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, fmt.Errorf("write report row: %w", err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode report excel: %w", err)
	}
	return buffer.Bytes(), nil
}

var pdfColumnWidths = []float64{18, 45, 32, 32, 25, 25, 25, 28, 22, 30}

func buildPDF(summary Summary, rows []Row) ([]byte, error) {
	doc := fpdf.New("L", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "B", 14)
	doc.CellFormat(0, 10, "Laporan Distribusi", "", 1, "L", false, 0, "")

	doc.SetFont("Helvetica", "", 11)
	doc.CellFormat(0, 7, fmt.Sprintf("Total alokasi: %d", summary.TotalAllocations), "", 1, "L", false, 0, "")
	for _, count := range summary.AllocationStatusCounts {
		doc.CellFormat(0, 6, fmt.Sprintf("Status alokasi %s: %d", count.Status, count.Count), "", 1, "L", false, 0, "")
	}
	for _, count := range summary.DistributionStatusCounts {
		doc.CellFormat(0, 6, fmt.Sprintf("Status distribusi %s: %d", count.Status, count.Count), "", 1, "L", false, 0, "")
	}
	doc.CellFormat(0, 6, fmt.Sprintf("Dokumentasi belum lengkap: %d", summary.DocumentationIncomplete), "", 1, "L", false, 0, "")
	doc.Ln(4)

	doc.SetFont("Helvetica", "B", 9)
	for index, header := range reportColumnHeaders {
		doc.CellFormat(pdfColumnWidths[index], 7, header, "1", 0, "L", false, 0, "")
	}
	doc.Ln(-1)
	doc.SetFont("Helvetica", "", 9)
	for _, row := range rows {
		for index, value := range reportRowValues(row) {
			doc.CellFormat(pdfColumnWidths[index], 6, value, "1", 0, "L", false, 0, "")
		}
		doc.Ln(-1)
	}

	var buffer bytes.Buffer
	if err := doc.Output(&buffer); err != nil {
		return nil, fmt.Errorf("encode report pdf: %w", err)
	}
	return buffer.Bytes(), nil
}
```

Fix `export_test.go` to import `"bytes"` and call `excelize.OpenReader(bytes.NewReader(data))` directly (drop the placeholder adapter mentioned in Step 2 — it was a note, not real code).

- [ ] **Step 5: Extend the repository with export audit logging**

Modify `internal/reports/repository.go` to add:

```go
import (
	"konkit/internal/audit"
	"konkit/internal/auth"
)

func (r *Repository) RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, filter Filter, meta auth.ClientMeta) error {
	return audit.Record(ctx, r.pool, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "reports.exported",
		ResourceType: "program_schedule",
		ResourceID:   scheduleID,
		Metadata: map[string]any{
			"format":                format,
			"allocation_status":    filter.AllocationStatus,
			"distribution_status":  filter.DistributionStatus,
			"documentation_status": filter.DocumentationStatus,
		},
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
	})
}
```

- [ ] **Step 6: Extend the service with export methods**

Modify `internal/reports/service.go`: extend the `repository` interface with `RecordExport(ctx context.Context, actor auth.Principal, scheduleID, format string, filter Filter, meta auth.ClientMeta) error`, add imports `"konkit/internal/auth"`, and append:

```go
func (s *Service) ExportExcel(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta) ([]byte, error) {
	rows, err := s.Rows(ctx, scheduleID, filter)
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

func (s *Service) ExportPDF(ctx context.Context, actor auth.Principal, scheduleID string, filter Filter, meta auth.ClientMeta) ([]byte, error) {
	summary, err := s.Summary(ctx, scheduleID, filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.Rows(ctx, scheduleID, filter)
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

- [ ] **Step 7: Update the service test stub and add export tests**

Modify `internal/reports/service_test.go`: **replace** the existing `repositoryStub` type definition (from Task 1) with this expanded version — do not add a second `type repositoryStub struct` declaration, Go does not allow redeclaring a type in the same package:

```go
type repositoryStub struct {
	summary        Summary
	rows           []Row
	scheduleID     string
	filter         Filter
	recordedActor  auth.Principal
	recordedFormat string
}
```

Keep the existing `Summary` and `Rows` methods on `repositoryStub` unchanged, and add the new method:

```go
func (r *repositoryStub) RecordExport(_ context.Context, actor auth.Principal, scheduleID, format string, filter Filter, _ auth.ClientMeta) error {
	r.recordedActor, r.recordedFormat, r.scheduleID, r.filter = actor, format, scheduleID, filter
	return nil
}
```

Add import `"konkit/internal/auth"` and append:

```go
func TestExportExcelRecordsAuditEventAfterBuildingFile(t *testing.T) {
	repository := &repositoryStub{rows: []Row{{DistributionNumber: 7, FullName: "Siti Aminah"}}}
	service := NewService(repository)

	data, err := service.ExportExcel(context.Background(), auth.Principal{UserID: "user-1"}, "schedule-1", Filter{AllocationStatus: "ready"}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty excel bytes")
	}
	if repository.recordedFormat != "xlsx" || repository.recordedActor.UserID != "user-1" || repository.filter.AllocationStatus != "ready" {
		t.Fatalf("repository=%+v", repository)
	}
}

func TestExportPDFRecordsAuditEventAfterBuildingFile(t *testing.T) {
	repository := &repositoryStub{summary: Summary{TotalAllocations: 1}, rows: []Row{{DistributionNumber: 7, FullName: "Siti Aminah"}}}
	service := NewService(repository)

	data, err := service.ExportPDF(context.Background(), auth.Principal{UserID: "user-1"}, "schedule-1", Filter{}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty pdf bytes")
	}
	if repository.recordedFormat != "pdf" {
		t.Fatalf("repository=%+v", repository)
	}
}
```

- [ ] **Step 8: Add an integration test for the audit trail**

Append to `internal/reports/repository_integration_test.go`:

```go
func TestIntegrationRecordExportWritesAuditEvent(t *testing.T) {
	pool := reportsIntegrationPool(t)
	fixture := createReportsFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()

	err := repository.RecordExport(ctx, auth.Principal{UserID: ""}, fixture.scheduleID, "xlsx", Filter{AllocationStatus: "ready"}, auth.ClientMeta{UserAgent: "reports-integration"})
	if err != nil {
		t.Fatal(err)
	}
	var action, metadata string
	if err := pool.QueryRow(ctx, `SELECT action,metadata::text FROM audit_logs WHERE resource_id=$1 AND user_agent='reports-integration'`, fixture.scheduleID).Scan(&action, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != "reports.exported" || !containsAll(metadata, `"format":"xlsx"`, `"allocation_status":"ready"`) {
		t.Fatalf("action=%q metadata=%q", action, metadata)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
```

Add imports `"konkit/internal/auth"` and `"strings"` to the integration test file.

- [ ] **Step 9: Run all reports tests**

Run:

```powershell
go test ./internal/reports -count=1
```

Expected: PASS (unit tests always run; integration tests run when `TEST_DATABASE_URL` is set per Task 2 Step 4).

- [ ] **Step 10: Commit**

```powershell
git add go.mod go.sum internal/reports
git commit -m "feat: export reports to excel and pdf"
```

---

### Task 4: Reports API Routes and Dependency Wiring

**Files:**
- Create: `internal/api/reports_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes `reports.Service` from Tasks 1–3.
- Produces `GET /api/v1/reports/schedule/{schedule_id}/summary`, `GET .../rows`, `GET .../export.xlsx`, `GET .../export.pdf`.

- [ ] **Step 1: Write the failing handler tests**

Append to `internal/api/handler_test.go`:

```go
type fakeReportsService struct {
	ReportsService
	scheduleID     string
	filter         reports.Filter
	summary        reports.Summary
	rows           []reports.Row
	exportData     []byte
	exportFormat   string
}

func (s *fakeReportsService) Summary(_ context.Context, scheduleID string, filter reports.Filter) (reports.Summary, error) {
	s.scheduleID, s.filter = scheduleID, filter
	return s.summary, nil
}

func (s *fakeReportsService) Rows(_ context.Context, scheduleID string, filter reports.Filter) ([]reports.Row, error) {
	s.scheduleID, s.filter = scheduleID, filter
	return s.rows, nil
}

func (s *fakeReportsService) ExportExcel(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat = scheduleID, filter, "xlsx"
	return s.exportData, nil
}

func (s *fakeReportsService) ExportPDF(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat = scheduleID, filter, "pdf"
	return s.exportData, nil
}

func TestReportsEndpointsRequireDistributionViewPermission(t *testing.T) {
	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Reports: &fakeReportsService{}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportsRowsAppliesFilterAndReturnsFullNIK(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeReportsService{rows: []reports.Row{{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/rows?allocation_status=ready", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Reports: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.scheduleID != "schedule-1" || service.filter.AllocationStatus != "ready" {
		t.Fatalf("status=%d schedule=%q filter=%+v body=%s", rec.Code, service.scheduleID, service.filter, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "7306014101900001") {
		t.Fatalf("expected full NIK in reports body: %s", rec.Body.String())
	}
}

func TestReportsExportReturnsAttachmentHeaders(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeReportsService{exportData: []byte("excel-bytes")}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/export.xlsx", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Reports: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.exportFormat != "xlsx" {
		t.Fatalf("status=%d format=%q body=%s", rec.Code, service.exportFormat, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content-type=%q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("content-disposition=%q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.String() != "excel-bytes" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
```

Add import `"konkit/internal/reports"` to `internal/api/handler_test.go`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api -count=1`

Expected: FAIL because `Dependencies.Reports`, `ReportsService`, and the route do not exist.

- [ ] **Step 3: Add the service interface and dependency field**

Modify `internal/api/handler.go`: add import `"konkit/internal/reports"`, then add after `DistributionService`:

```go
type ReportsService interface {
	Summary(context.Context, string, reports.Filter) (reports.Summary, error)
	Rows(context.Context, string, reports.Filter) ([]reports.Row, error)
	ExportExcel(context.Context, auth.Principal, string, reports.Filter, auth.ClientMeta) ([]byte, error)
	ExportPDF(context.Context, auth.Principal, string, reports.Filter, auth.ClientMeta) ([]byte, error)
}
```

Add `Reports ReportsService` to the `Dependencies` struct, and add this case to `routeProtected`'s switch, alongside the `distribution/` cases:

```go
case strings.HasPrefix(path, "reports/schedule/"):
	h.handleReportsSchedule(w, r, rc, strings.TrimPrefix(path, "reports/schedule/"))
```

- [ ] **Step 4: Implement the route handler**

Create `internal/api/reports_routes.go`:

```go
package api

import (
	"mime"
	"net/http"
	"strings"

	"konkit/internal/reports"
)

func (h *Handler) handleReportsSchedule(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Reports == nil {
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
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	scheduleID := parts[0]
	filter := reports.Filter{
		AllocationStatus:    strings.TrimSpace(r.URL.Query().Get("allocation_status")),
		DistributionStatus:  strings.TrimSpace(r.URL.Query().Get("distribution_status")),
		DocumentationStatus: strings.TrimSpace(r.URL.Query().Get("documentation_status")),
	}
	switch parts[1] {
	case "summary":
		result, err := h.deps.Reports.Summary(r.Context(), scheduleID, filter)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "rows":
		result, err := h.deps.Reports.Rows(r.Context(), scheduleID, filter)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "export.xlsx":
		data, err := h.deps.Reports.ExportExcel(r.Context(), rc.principal, scheduleID, filter, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, "laporan-distribusi.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
	case "export.pdf":
		data, err := h.deps.Reports.ExportPDF(r.Context(), rc.principal, scheduleID, filter, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, "laporan-distribusi.pdf", "application/pdf", data)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func writeAttachment(w http.ResponseWriter, filename, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
```

- [ ] **Step 5: Map reports errors to HTTP responses**

Modify `internal/api/routes.go`: add `"konkit/internal/reports"` to imports, and add this case to `writeServiceError` alongside the other `validation_failed` cases:

```go
case errors.Is(err, reports.ErrScheduleRequired), errors.Is(err, reports.ErrFilterInvalid):
	writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), map[string]string{"request": err.Error()})
```

- [ ] **Step 6: Wire the dependency in the server**

Modify `cmd/server/main.go`: add import `"konkit/internal/reports"` and add to the `Dependencies` literal:

```go
Reports: reports.NewService(reports.NewRepository(pool)),
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/api ./cmd/server -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/api cmd/server
git commit -m "feat: add reports API routes"
```

---

### Task 5: Reports Page — Schedule, Filters, Summary, and Table

**Files:**
- Create: `frontend/src/features/reports/types.ts`
- Create: `frontend/src/features/reports/ReportsPage.tsx`
- Create: `frontend/src/features/reports/ReportsPage.test.tsx`
- Create: `frontend/src/features/reports/Reports.module.css`

**Interfaces:**
- Consumes `GET /api/v1/reports/schedule/{schedule_id}/summary`, `.../rows`, and `GET /api/v1/program-setup/schedules` (already used by `DistributionPage`).
- Produces `ReportsPage` component consumed by Task 6's route.

- [ ] **Step 1: Write the failing UI test**

Create `frontend/src/features/reports/ReportsPage.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { ReportsPage } from './ReportsPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPage() {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/program-setup/schedules') {
      return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' } }] });
    }
    if (path.startsWith('/api/v1/reports/schedule/schedule-1/summary')) {
      return Promise.resolve({ data: {
        total_allocations: 3,
        allocation_status_counts: [{ status: 'ready', count: 1 }, { status: 'distributed', count: 2 }],
        distribution_status_counts: [{ status: 'draft', count: 1 }, { status: 'completed', count: 2 }],
        documentation_incomplete: 1,
      } });
    }
    if (path.startsWith('/api/v1/reports/schedule/schedule-1/rows')) {
      return Promise.resolve({ data: [
        { distribution_number: 7, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier: 'KP01', village: 'Tempe', district: 'Sabbangparu', allocation_status: 'distributed', distribution_status: 'completed', documentation_complete: true, completed_at: '2026-09-10T08:30:00Z' },
      ] });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><ReportsPage /></QueryClientProvider>);
}

afterEach(() => { vi.clearAllMocks(); });

test('loads summary and rows for the selected schedule with full NIK', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });

  expect(await screen.findByText('3')).toBeVisible();
  expect(screen.getByText('Siti Aminah')).toBeVisible();
  expect(screen.getByText('7306014101900001')).toBeVisible();
  expect(screen.getByText('Lengkap')).toBeVisible();
});

test('applies status filters to the rows request', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });
  await screen.findByText('Siti Aminah');
  fireEvent.change(screen.getByLabelText('Status alokasi'), { target: { value: 'distributed' } });

  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.startsWith('/api/v1/reports/schedule/schedule-1/rows') && path.includes('allocation_status=distributed'));
  expect(call).toBeTruthy();
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/reports/ReportsPage.test.tsx`

Expected: FAIL because `ReportsPage` does not exist.

- [ ] **Step 3: Add the types**

Create `frontend/src/features/reports/types.ts`:

```ts
import type { Schedule } from '../programs/types';

export type StatusCount = { status: string; count: number };
export type ReportSummary = {
  total_allocations: number;
  allocation_status_counts: StatusCount[];
  distribution_status_counts: StatusCount[];
  documentation_incomplete: number;
};
export type ReportRow = {
  distribution_number: number;
  full_name: string;
  nik: string;
  sector_identifier: string;
  village: string;
  district: string;
  allocation_status: string;
  distribution_status: string;
  documentation_complete: boolean;
  completed_at?: string;
};
export type DataResponse<T> = { data: T };
export type ScheduleResponse = DataResponse<Schedule[]>;
```

- [ ] **Step 4: Implement the page**

Create `frontend/src/features/reports/ReportsPage.tsx`:

```tsx
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { apiRequest } from '../../lib/api';
import type { DataResponse, ReportRow, ReportSummary, ScheduleResponse } from './types';
import styles from './Reports.module.css';

const ALLOCATION_STATUSES = ['candidate', 'ready', 'needs_review', 'distributed', 'replaced', 'cancelled'];
const DISTRIBUTION_STATUSES = ['draft', 'completed', 'cancelled'];

export function ReportsPage() {
  const [scheduleID, setScheduleID] = useState('');
  const [allocationStatus, setAllocationStatus] = useState('');
  const [distributionStatus, setDistributionStatus] = useState('');
  const [documentationStatus, setDocumentationStatus] = useState('');

  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<ScheduleResponse>('/api/v1/program-setup/schedules') });
  const params = new URLSearchParams({ allocation_status: allocationStatus, distribution_status: distributionStatus, documentation_status: documentationStatus });
  const queryString = params.toString();

  const summary = useQuery({
    queryKey: ['reports', 'summary', scheduleID, allocationStatus, distributionStatus, documentationStatus],
    queryFn: () => apiRequest<DataResponse<ReportSummary>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/summary?${queryString}`),
    enabled: Boolean(scheduleID),
  });
  const rows = useQuery({
    queryKey: ['reports', 'rows', scheduleID, allocationStatus, distributionStatus, documentationStatus],
    queryFn: () => apiRequest<DataResponse<ReportRow[]>>(`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/rows?${queryString}`),
    enabled: Boolean(scheduleID),
  });

  return <div className="page">
    <header className="pageHeader"><div><h1>Laporan</h1><p>Pantau progres alokasi, distribusi, dan dokumentasi per jadwal.</p></div></header>
    <section className={styles.filters}>
      <label><span>Jadwal</span><select value={scheduleID} onChange={(event) => setScheduleID(event.target.value)}>
        <option value="">Pilih kabupaten dan jadwal</option>
        {schedules.data?.data.map((schedule) => <option value={schedule.id} key={schedule.id}>{schedule.regency?.name} / {schedule.name}</option>)}
      </select></label>
      <label><span>Status alokasi</span><select value={allocationStatus} onChange={(event) => setAllocationStatus(event.target.value)}>
        <option value="">Semua</option>
        {ALLOCATION_STATUSES.map((status) => <option value={status} key={status}>{status}</option>)}
      </select></label>
      <label><span>Status distribusi</span><select value={distributionStatus} onChange={(event) => setDistributionStatus(event.target.value)}>
        <option value="">Semua</option>
        {DISTRIBUTION_STATUSES.map((status) => <option value={status} key={status}>{status}</option>)}
      </select></label>
      <label><span>Dokumentasi</span><select value={documentationStatus} onChange={(event) => setDocumentationStatus(event.target.value)}>
        <option value="">Semua</option>
        <option value="complete">Lengkap</option>
        <option value="incomplete">Belum lengkap</option>
      </select></label>
    </section>

    {scheduleID && <>
      <section className={styles.summary}>
        <article><strong>{summary.data?.data.total_allocations ?? '-'}</strong><span>Total alokasi</span></article>
        {summary.data?.data.allocation_status_counts.map((item) => <article key={`allocation-${item.status}`}><strong>{item.count}</strong><span>{item.status}</span></article>)}
        {summary.data?.data.distribution_status_counts.map((item) => <article key={`distribution-${item.status}`}><strong>{item.count}</strong><span>Distribusi {item.status}</span></article>)}
        <article><strong>{summary.data?.data.documentation_incomplete ?? '-'}</strong><span>Dokumentasi belum lengkap</span></article>
      </section>

      <table className={styles.table}>
        <thead><tr>
          <th>No. Pembagian</th><th>Nama</th><th>NIK</th><th>No. Kartu/KUSUKA</th><th>Desa/Kecamatan</th>
          <th>Status alokasi</th><th>Status distribusi</th><th>Dokumentasi</th><th>Tanggal selesai</th>
        </tr></thead>
        <tbody>{rows.data?.data.map((row) => <tr key={row.distribution_number}>
          <td>{row.distribution_number}</td>
          <td>{row.full_name}</td>
          <td>{row.nik}</td>
          <td>{row.sector_identifier}</td>
          <td>{[row.village, row.district].filter(Boolean).join(', ')}</td>
          <td>{row.allocation_status}</td>
          <td>{row.distribution_status}</td>
          <td>{row.documentation_complete ? 'Lengkap' : 'Belum lengkap'}</td>
          <td>{row.completed_at ? new Date(row.completed_at).toLocaleString('id-ID') : '-'}</td>
        </tr>)}</tbody>
      </table>
    </>}
  </div>;
}
```

- [ ] **Step 5: Add minimal styling**

Create `frontend/src/features/reports/Reports.module.css`:

```css
.filters { display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 1.5rem; }
.filters label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.875rem; }
.summary { display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 1.5rem; }
.summary article { display: flex; flex-direction: column; gap: 0.25rem; padding: 0.75rem 1rem; border: 1px solid var(--border-color, #d9d9d9); border-radius: 0.5rem; min-width: 8rem; }
.summary article strong { font-size: 1.5rem; }
.table { width: 100%; border-collapse: collapse; }
.table th, .table td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--border-color, #d9d9d9); white-space: nowrap; }
```

- [ ] **Step 6: Run the tests**

Run: `npm.cmd --prefix frontend test -- --run src/features/reports/ReportsPage.test.tsx`

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/features/reports
git commit -m "feat: add reports page with filters and summary"
```

---

### Task 6: Export Links, Navigation, and Verification

**Files:**
- Modify: `frontend/src/features/reports/ReportsPage.tsx`
- Modify: `frontend/src/features/reports/ReportsPage.test.tsx`
- Modify: `frontend/src/features/reports/Reports.module.css`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/routes.tsx`
- Modify: `README.md`

**Interfaces:**
- Consumes `GET /api/v1/reports/schedule/{schedule_id}/export.xlsx` and `.../export.pdf` from Task 4 (via direct link navigation, same pattern as `MediaFile.content_url`).
- Produces route `/laporan`.

- [ ] **Step 1: Write the failing export-link test**

Append to `frontend/src/features/reports/ReportsPage.test.tsx`:

```tsx
test('renders export links scoped to the selected schedule and active filters', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });
  await screen.findByText('Siti Aminah');
  fireEvent.change(screen.getByLabelText('Status alokasi'), { target: { value: 'distributed' } });

  const excelLink = screen.getByRole('link', { name: 'Export Excel' });
  const pdfLink = screen.getByRole('link', { name: 'Export PDF' });
  expect(excelLink.getAttribute('href')).toBe('/api/v1/reports/schedule/schedule-1/export.xlsx?allocation_status=distributed&distribution_status=&documentation_status=');
  expect(pdfLink.getAttribute('href')).toBe('/api/v1/reports/schedule/schedule-1/export.pdf?allocation_status=distributed&distribution_status=&documentation_status=');
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/reports/ReportsPage.test.tsx`

Expected: FAIL because the export links do not exist yet.

- [ ] **Step 3: Add the export links**

Modify `frontend/src/features/reports/ReportsPage.tsx`: inside the `{scheduleID && <>...</>}` block, add an export bar directly after the `<section className={styles.summary}>` block and before the `<table>`:

```tsx
<section className={styles.exportBar}>
  <a className={styles.exportButton} href={`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/export.xlsx?${queryString}`}>Export Excel</a>
  <a className={styles.exportButton} href={`/api/v1/reports/schedule/${encodeURIComponent(scheduleID)}/export.pdf?${queryString}`}>Export PDF</a>
</section>
```

Add to `frontend/src/features/reports/Reports.module.css`:

```css
.exportBar { display: flex; gap: 0.75rem; margin-bottom: 1rem; }
.exportButton { display: inline-flex; align-items: center; padding: 0.5rem 1rem; border: 1px solid var(--border-color, #d9d9d9); border-radius: 0.375rem; text-decoration: none; }
```

- [ ] **Step 4: Add navigation and the protected route**

Modify `frontend/src/app/AppShell.tsx`: add `FileText` to the `lucide-react` import list, and add to the `Dokumentasi` group (after `Pendistribusian`):

```tsx
{ label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
```

Modify `frontend/src/app/routes.tsx`: add the import `import { ReportsPage } from '../features/reports/ReportsPage';` and add this route inside `dashboardRoutes[0].children`, after `dokumentasi/pendistribusian`:

```tsx
{ path: 'laporan', element: <ProtectedPage permission="distribution.view"><ReportsPage /></ProtectedPage> },
```

- [ ] **Step 5: Document the feature**

Modify `README.md`: add one sentence after the "Alur DCP3 Dan Pendistribusian" section noting that `Laporan` provides a per-jadwal read-only summary and Excel/PDF export scoped to the same permission as Pendistribusian.

- [ ] **Step 6: Run full verification**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
go test ./... -count=1
go vet ./...
npm.cmd --prefix frontend run test -- --run
npm.cmd --prefix frontend run build
```

Expected: all commands exit 0.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/features/reports frontend/src/app README.md web/static/app
git commit -m "feat: add reports export links and navigation"
```
