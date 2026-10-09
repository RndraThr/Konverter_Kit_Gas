# System Administration Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan.

**Goal:** Upgrade Kesehatan Sistem, Pengaturan Sistem, and Riwayat Aktivitas into a responsive, operational administration center while preserving existing routes, permissions, settings registry, and audit immutability.

**Architecture:** Implement three independently testable vertical slices on the existing Go/PostgreSQL and React/React Query modules. Health gains read-only operational probes, settings gains updater identity plus a stateful responsive editor, and audit gains validated filters, filtered summaries, responsive list surfaces, and a detail sheet. No database migration or new dependency is required.

**Tech Stack:** Go 1.26, pgx v5, PostgreSQL, React 19, TypeScript, React Query, React Router, Vitest/Testing Library, Tailwind CSS, existing shadcn-style UI primitives and Lucide icons.

**Spec:** `docs/superpowers/specs/2026-10-09-system-administration-center-design.md`

## Global Constraints

- Preserve `GET /api/v1/system/health`, `GET|PUT /api/v1/system/settings`, and `GET /api/v1/system/audit-logs`.
- Preserve permissions `health.view`, `settings.view`, `settings.manage`, and `audit.view`.
- Do not expose secrets, raw backend errors, database URLs, credentials, or storage tokens.
- Do not add destructive health controls or mutation support to audit.
- Do not add a migration or dependency.
- Preserve all unrelated working-tree changes and stage only files from the current task at each commit.
- Keep UI copy in Bahasa Indonesia and reuse theme tokens and existing UI primitives.
- Use test-first cycles: write the named failing test, run it and observe the expected failure, implement the minimum behavior, rerun the focused test, then run the package/suite test.

## Review Focus

- A database ping failure must still return `unhealthy`; an operational-statistics failure must return `degraded` without exposing the database error.
- The health endpoint must remain usable before the media worker starts and for local storage where the move worker is not applicable.
- Settings updates must send and audit only changed keys; a failed save must retain the draft.
- Audit filters must remain parameterized, accept legacy filters, validate dates before repository execution, and calculate summaries against the same active filter set.
- IP address and user-agent must only appear after opening an audit detail sheet.
- At 375px, no primary content surface may depend on horizontal scrolling.

---

### Task 1: Extend the health report with safe operational metrics

**Files:**

- Modify: `internal/health/service.go`
- Modify: `internal/health/service_test.go`
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`

**Step 1: Write failing service tests for the extended report**

Extend the existing fake probe in `internal/health/service_test.go` with operational statistics and an optional error, then add tests covering healthy and degraded reports:

```go
func TestCheckIncludesStorageWorkerAndMediaMoveStats(t *testing.T) {
	probe := &fakeProbe{migrationVersion: 56, stats: OperationalStats{
		Queued: 3, Processing: 1, Retry: 2, Failed: 4,
	}}
	service := NewService(probe, "staging", "dev", "gdrive", true, time.Unix(100, 0))
	service.now = func() time.Time { return time.Unix(160, 0) }

	report := service.Check(context.Background())

	if report.Status != StatusHealthy || report.StorageBackend != "gdrive" || report.MediaWorkerStatus != "active" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.MediaMoves.Queued != 3 || report.MediaMoves.Failed != 4 {
		t.Fatalf("unexpected media moves: %#v", report.MediaMoves)
	}
}

func TestCheckDegradesWhenOperationalStatsCannotBeRead(t *testing.T) {
	probe := &fakeProbe{statsErr: errors.New("secret database detail")}
	service := NewService(probe, "staging", "dev", "local", false, time.Now())

	report := service.Check(context.Background())

	if report.Status != StatusDegraded || report.Operations.Code != "operational_stats_unavailable" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if strings.Contains(report.Operations.Message, "secret") {
		t.Fatalf("raw error leaked: %q", report.Operations.Message)
	}
}
```

Update constructor assertions in existing health and server tests to account for the storage backend and worker-state parameters.

**Step 2: Run the tests and confirm they fail for missing report fields**

Run:

```powershell
go test ./internal/health ./cmd/server -run "TestCheck|TestStartMediaMoveWorker" -count=1
```

Expected: compilation failure because `OperationalStats`, the new constructor parameters, and report fields do not exist.

**Step 3: Implement the operational health contract**

Add explicit JSON types in `internal/health/service.go`:

```go
type OperationalStats struct {
	Queued     int64 `json:"queued"`
	Processing int64 `json:"processing"`
	Retry      int64 `json:"retry"`
	Failed     int64 `json:"failed"`
}

type Report struct {
	Status            string           `json:"status"`
	Database          Component        `json:"database"`
	Operations        Component        `json:"operations"`
	MigrationVersion  int64            `json:"migration_version"`
	Environment       string           `json:"environment"`
	Version           string           `json:"version"`
	StorageBackend    string           `json:"storage_backend"`
	MediaWorkerStatus string           `json:"media_worker_status"`
	MediaMoves        OperationalStats `json:"media_moves"`
	UptimeSeconds     int64            `json:"uptime_seconds"`
	CheckedAt         time.Time        `json:"checked_at"`
}
```

Extend `Probe` with `OperationalStats(context.Context) (OperationalStats, error)`. Implement `PostgresProbe.OperationalStats` using one parameterized aggregate query that counts `distribution_media_move_jobs.status` values `queued`, `processing`, and `retry`, plus `media_files.storage_state = 'move_failed'`. Treat a statistics error as degraded only after database and migration checks succeed.

Change the constructor to:

```go
func NewService(probe Probe, environment, version, storageBackend string, mediaWorkerActive bool, startedAt time.Time) *Service
```

Set `media_worker_status` to `active` only when the worker was started; otherwise use `not_applicable`. In `cmd/server/main.go`, retain the boolean returned by `startMediaMoveWorker` and pass it with `cfg.StorageBackend` to `health.NewService`.

**Step 4: Run focused and package tests**

Run:

```powershell
go test ./internal/health ./cmd/server -count=1
```

Expected: PASS.

**Step 5: Commit the backend health slice**

```powershell
git add internal/health/service.go internal/health/service_test.go cmd/server/main.go cmd/server/main_test.go
git commit -m "feat: expose operational system health"
```

---

### Task 2: Redesign the Kesehatan Sistem page

**Files:**

- Modify: `frontend/src/features/health/HealthPage.tsx`
- Modify: `frontend/src/features/health/HealthPage.module.css`
- Modify: `frontend/src/features/health/HealthPage.test.tsx`

**Step 1: Write failing UI tests for metrics, queue state, and refresh**

Update the health fixture with the new API fields and add assertions:

```tsx
it('shows operational metrics and refreshes on demand', async () => {
  const user = userEvent.setup()
  renderHealthPage(healthyReport)

  expect(await screen.findByText('Sistem sehat')).toBeInTheDocument()
  expect(screen.getByText('Google Drive')).toBeInTheDocument()
  expect(screen.getByText('3 menunggu')).toBeInTheDocument()
  expect(screen.getByText('4 gagal')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Periksa ulang' }))
  expect(api.get).toHaveBeenCalledTimes(2)
})

it('uses labelled list regions that remain readable on mobile', async () => {
  renderHealthPage(healthyReport)
  expect(await screen.findByRole('region', { name: 'Ringkasan kesehatan' })).toBeInTheDocument()
  expect(screen.getByRole('region', { name: 'Antrean pemindahan media' })).toBeInTheDocument()
})
```

Also test degraded copy and verify no raw error property is rendered.

**Step 2: Run the focused test and observe the failure**

Run:

```powershell
npm.cmd test -- HealthPage.test.tsx
```

Working directory: `frontend`

Expected: failures for missing labels, metrics, and refresh action.

**Step 3: Implement the responsive health dashboard**

Update the response type and render:

- one status hero with icon, `checked_at`, and a stable-width refresh button;
- a labelled metric grid for database, latency, migration, uptime, and storage;
- a labelled operational panel for worker status and queued/processing/retry/failed counts;
- existing `DataState` behavior for initial loading and fatal errors;
- stale data during background refetch using React Query's existing cached data.

Use CSS grid with `repeat(auto-fit, minmax(min(100%, 12rem), 1fr))`, single-column behavior under 640px, 44px minimum controls, visible focus, and reduced-motion handling. Do not add horizontal overflow to the content container.

**Step 4: Run focused tests, typecheck, and commit**

Run:

```powershell
npm.cmd test -- HealthPage.test.tsx
npm.cmd run typecheck
```

Working directory: `frontend`

Expected: PASS.

```powershell
git add frontend/src/features/health/HealthPage.tsx frontend/src/features/health/HealthPage.module.css frontend/src/features/health/HealthPage.test.tsx
git commit -m "feat: redesign system health dashboard"
```

---

### Task 3: Enrich settings with updater identity

**Files:**

- Modify: `internal/settings/models.go`
- Modify: `internal/settings/repository.go`
- Modify: `internal/settings/repository_integration_test.go`

**Step 1: Write a failing repository integration test**

Add a test that inserts or uses a user, updates one setting through the repository, lists settings, and asserts both identity fields:

```go
if got.UpdatedBy != actor.UserID {
	t.Fatalf("updated_by=%q want %q", got.UpdatedBy, actor.UserID)
}
if got.UpdatedByName != actor.FullName {
	t.Fatalf("updated_by_name=%q want %q", got.UpdatedByName, actor.FullName)
}
```

Keep the existing atomic update and audit assertions. Ensure the update input contains one changed key so the test documents partial-update compatibility.

**Step 2: Run the integration test and confirm the missing field**

Run with the repository's existing `TEST_DATABASE_URL` convention:

```powershell
go test ./internal/settings -run TestRepository -count=1
```

Expected: compilation failure because `UpdatedByName` does not exist.

**Step 3: Implement the left join**

Add to `Setting`:

```go
UpdatedByName string `json:"updated_by_name,omitempty"`
```

Change `Repository.List` to select from `system_settings` and left join `users` by `updated_by`, using `COALESCE(NULLIF(users.full_name, ''), users.username, '')`. Preserve `updated_by` for backward compatibility and update the scan order.

**Step 4: Run settings tests and commit**

```powershell
go test ./internal/settings -count=1
git add internal/settings/models.go internal/settings/repository.go internal/settings/repository_integration_test.go
git commit -m "feat: expose system setting updater"
```

Expected: PASS; integration tests may skip only when the documented test database variable is absent.

---

### Task 4: Redesign Pengaturan Sistem with preview and dirty-state controls

**Files:**

- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Create: `frontend/src/features/settings/settingsPresentation.ts`
- Create: `frontend/src/features/settings/settingsPresentation.test.ts`

**Step 1: Write failing pure tests for date/time preview**

Define a small presentation function and its expectations:

```ts
expect(formatSettingsPreview('2026-10-09T09:15:00Z', 'Asia/Jakarta', '02/01/2006', 'id-ID'))
  .toContain('09/10/2026')
expect(formatSettingsPreview('2026-10-09T09:15:00Z', 'Asia/Makassar', '02 January 2006', 'id-ID'))
  .toContain('09 Oktober 2026')
```

The helper must map only the registry-supported date formats and use `Intl.DateTimeFormat` for timezone-safe time output.

**Step 2: Write failing page behavior tests**

Add tests for:

- unchanged form disables `Simpan perubahan` and `Batalkan perubahan`;
- editing one field enables actions;
- cancel restores server values;
- save sends only the changed key, for example `{ values: { application_name: 'Konkit Baru' } }`;
- a rejected save keeps the draft visible;
- updater name/time and live preview render;
- missing `settings.manage` shows disabled controls and the read-only explanation.

Use the existing auth/permission test wrapper rather than mocking permission behavior inside the component.

**Step 3: Run focused tests and confirm behavior failures**

```powershell
npm.cmd test -- settingsPresentation.test.ts SettingsPage.test.tsx
```

Working directory: `frontend`

Expected: failures for the missing helper, dirty state, partial payload, preview, and read-only explanation.

**Step 4: Implement the editor**

In `SettingsPage.tsx`:

- normalize the server array into `serverValues` and keep a separate draft;
- derive `changedValues` by comparing the five registered keys;
- send `{ values: changedValues }` only;
- reset the draft only after a successful mutation/refetch;
- split fields into `Identitas` and `Regional` sections;
- add a side preview showing application, organization, formatted date/time, timezone, and locale;
- show the most recent `updated_at` entry and its `updated_by_name` fallback;
- provide a persistent action bar with save/cancel state and dirty copy;
- preserve the draft on mutation error;
- disable all editing controls and actions without `settings.manage`.

Keep native labels connected to inputs/selects and use existing `Button`, `Input`, `Select`, `Alert`, and `DataState` primitives.

**Step 5: Run tests, typecheck, and commit**

```powershell
npm.cmd test -- settingsPresentation.test.ts SettingsPage.test.tsx
npm.cmd run typecheck
```

Working directory: `frontend`

Expected: PASS.

```powershell
git add frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx frontend/src/features/settings/settingsPresentation.ts frontend/src/features/settings/settingsPresentation.test.ts
git commit -m "feat: redesign system settings editor"
```

---

### Task 5: Add validated audit filters and filtered summaries

**Files:**

- Modify: `internal/audit/models.go`
- Modify: `internal/audit/repository.go`
- Modify: `internal/audit/repository_test.go`
- Modify: `internal/audit/repository_integration_test.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler_test.go`

**Step 1: Write failing filter normalization tests**

Extend `Filter` with `Query`, `Actor`, `DateFrom`, and `DateTo`. Add a package test asserting page limits remain intact and whitespace is trimmed before use. Define summary output:

```go
type Summary struct {
	Today  int64 `json:"today"`
	System int64 `json:"system"`
}

type Page struct {
	Items    []Entry `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int64 `json:"total"`
	Summary  Summary `json:"summary"`
}
```

**Step 2: Write failing route tests for date validation and forwarding**

In `internal/api/handler_test.go`, use the existing fake audit service to capture the filter:

```go
req := authenticatedRequest(http.MethodGet,
	"/api/v1/system/audit-logs?query=login&actor=rendra&date_from=2026-10-01&date_to=2026-10-09&page_size=50", nil)
// assert Query, Actor, DateFrom, DateTo, and PageSize were forwarded
```

Add separate requests for invalid `date_from`, invalid `date_to`, and `date_from > date_to`, expecting HTTP 400, code `validation_failed`, and field-specific errors. Keep a regression assertion for `actor_user_id`.

**Step 3: Run focused tests and observe missing contract failures**

```powershell
go test ./internal/audit ./internal/api -run "Test.*Audit|TestNormalizeFilter" -count=1
```

Expected: compilation or assertion failures for the new filter and summary fields.

**Step 4: Implement route-level date validation**

Add an unexported helper beside `handleAudit` that accepts only `YYYY-MM-DD`, trims all string filters, and reports field errors before calling the service. Keep dates as validated strings in `audit.Filter` so PostgreSQL can apply the configured system timezone consistently.

Forward all filters:

```go
audit.Filter{
	Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 20),
	Query: q.Get("query"), Action: q.Get("action"), ResourceType: q.Get("resource_type"),
	ActorUserID: q.Get("actor_user_id"), Actor: q.Get("actor"),
	DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"),
}
```

Reject reversed ranges after parsing both dates.

**Step 5: Implement one parameterized repository filter**

Build a fixed SQL `WHERE` clause with positional parameters only. The general query must search action, resource type, resource ID, full name, username, and email. The actor query must search full name, username, and email. Preserve exact action/resource/actor UUID filters.

Use the configured timezone without a schema change:

```sql
COALESCE(
  (SELECT value #>> '{}' FROM system_settings WHERE key = 'timezone'),
  'Asia/Jakarta'
)
```

Apply inclusive date-only bounds as local midnight `>= date_from` and `< date_to + 1 day`. Reuse exactly the same active-filter predicate for:

- total filtered count;
- today's filtered count in the configured timezone;
- filtered system-event count where `actor_user_id IS NULL`;
- paginated item query.

Keep sorting by `created_at DESC, id DESC` and the page-size cap of 100.

**Step 6: Add repository integration coverage**

Seed user and system events on both sides of the requested date boundary. Assert:

- `query` matches a resource ID and actor email;
- `actor` matches name/username/email;
- date bounds are inclusive in the configured timezone;
- exact `actor_user_id` still works;
- `total`, `summary.today`, and `summary.system` reflect the same filter;
- pagination remains deterministic.

**Step 7: Run backend tests and commit**

```powershell
go test ./internal/audit ./internal/api -count=1
git add internal/audit/models.go internal/audit/repository.go internal/audit/repository_test.go internal/audit/repository_integration_test.go internal/api/routes.go internal/api/handler_test.go
git commit -m "feat: expand audit search and summaries"
```

Expected: PASS.

---

### Task 6: Redesign Riwayat Aktivitas for desktop and mobile

**Files:**

- Modify: `frontend/src/features/audit/AuditPage.tsx`
- Modify: `frontend/src/features/audit/AuditPage.test.tsx`
- Create: `frontend/src/features/audit/AuditDetailSheet.tsx`
- Create: `frontend/src/features/audit/AuditMobileCard.tsx`
- Create: `frontend/src/features/audit/auditPresentation.ts`
- Create: `frontend/src/features/audit/auditPresentation.test.ts`

**Step 1: Write failing presentation tests**

Cover human-readable labels with deterministic fallbacks:

```ts
expect(presentAuditAction('settings.updated')).toEqual({
  label: 'Memperbarui pengaturan sistem',
  code: 'settings.updated',
})
expect(presentAuditAction('custom.event').label).toBe('Aktivitas sistem')
```

Also test actor fallback (`Sistem`), resource label fallback, metadata row conversion, and stable JSON formatting.

**Step 2: Write failing page interaction tests**

Add page tests for:

- submitting general and advanced filters updates the request URL and resets page to 1;
- active filter chips render and remove only their own query key;
- `Reset semua` clears filter parameters but preserves the route;
- summary values render;
- page size accepts 10/20/50/100;
- desktop table and mobile card regions both have responsive classes/semantics;
- opening `Lihat detail` shows IP, user-agent, structured metadata, and expandable raw JSON;
- before opening the sheet, IP and user-agent values are absent from the document;
- empty state mentions changing filters;
- pagination renders `Halaman X dari Y` below the list.

Mock the API response with `summary` and at least one event containing IP/user-agent.

**Step 3: Run focused tests and confirm failures**

```powershell
npm.cmd test -- auditPresentation.test.ts AuditPage.test.tsx
```

Working directory: `frontend`

Expected: failures for missing presentation helpers, controls, cards, summaries, and sheet.

**Step 4: Implement URL-backed filters**

Keep a submitted-filter model sourced from `useSearchParams` and a local draft for text/date controls. Send only non-empty parameters to the endpoint. On apply, reset `page=1`; on chip removal or reset, update URL parameters and refetch. Keep legacy `action`, `resource_type`, and `actor_user_id` parameters intact when present.

Render the simple search and `Terapkan` action first, with advanced fields in an accessible disclosure containing action, resource type, actor, `date_from`, `date_to`, and page size.

**Step 5: Implement responsive result surfaces and details**

In `AuditPage.tsx`:

- add a quiet three-metric summary for total, today, and system events;
- render a compact desktop table hidden below the desktop breakpoint;
- render `AuditMobileCard` in a mobile-only list without horizontal scrolling;
- place pagination after both surfaces;
- use action/resource presentation helpers while retaining the original code as secondary text.

In `AuditDetailSheet.tsx`, use the existing `Sheet` primitive. Render full time, actor, action code, resource, resource ID, then IP and user-agent, then readable metadata rows. Put raw JSON in a native `<details>` element so it is collapsed initially. Ensure long IDs, user-agents, and JSON use safe wrapping.

**Step 6: Run focused tests, accessibility contracts, typecheck, and commit**

```powershell
npm.cmd test -- auditPresentation.test.ts AuditPage.test.tsx AccessibilityContracts.test.tsx
npm.cmd run typecheck
```

Working directory: `frontend`

Expected: PASS.

```powershell
git add frontend/src/features/audit/AuditPage.tsx frontend/src/features/audit/AuditPage.test.tsx frontend/src/features/audit/AuditDetailSheet.tsx frontend/src/features/audit/AuditMobileCard.tsx frontend/src/features/audit/auditPresentation.ts frontend/src/features/audit/auditPresentation.test.ts
git commit -m "feat: redesign system activity history"
```

---

### Task 7: Verify integration, responsive behavior, and production builds

**Files:**

- Modify only if a failing verification identifies a defect in files already listed above.

**Step 1: Run formatting and static checks**

```powershell
gofmt -w internal/health/service.go internal/health/service_test.go cmd/server/main.go cmd/server/main_test.go internal/settings/models.go internal/settings/repository.go internal/settings/repository_integration_test.go internal/audit/models.go internal/audit/repository.go internal/audit/repository_test.go internal/audit/repository_integration_test.go internal/api/routes.go internal/api/handler_test.go
go vet ./...
npm.cmd run lint
npm.cmd run typecheck
```

Run the npm commands from `frontend`. Expected: PASS with no newly introduced warnings.

**Step 2: Run complete automated test suites**

Run backend tests with the existing `konkit_test` database configuration and complete frontend tests:

```powershell
go test ./... -count=1
npm.cmd test
```

Working directory for npm: `frontend`.

Expected: all Go packages and all Vitest files PASS; database-backed tests must not be accepted as verified if they only skipped because the test database variable was omitted.

**Step 3: Build both applications**

```powershell
go build ./cmd/server
npm.cmd run build
```

Working directory for npm: `frontend`.

Expected: both builds complete successfully.

**Step 4: Perform responsive and privacy checks**

Serve the application locally using the project's normal development command and inspect all three routes at 375px and a desktop viewport. Confirm:

- no main-content horizontal scrollbar;
- all controls are keyboard reachable with visible focus;
- health metrics collapse to one column;
- settings form, preview, and action bar stack correctly;
- audit uses cards on mobile and a table on desktop;
- IP/user-agent are absent until the detail sheet opens;
- long audit values wrap rather than expanding the viewport;
- reduced-motion mode does not rely on animation to communicate state.

Record any defect as a failing focused test before fixing it.

**Step 5: Check repository integrity**

```powershell
git diff --check
git status --short
git log --oneline -7
```

Expected: no whitespace errors; only expected pre-existing unrelated changes remain unstaged; commits for Tasks 1-6 are visible.

**Step 6: Commit verification-only fixes if needed**

If verification required changes, stage only the affected task files and commit:

```powershell
git commit -m "fix: harden system administration center"
```

If no files changed, do not create an empty commit.

---

## Completion Criteria

- The three existing routes retain their permissions and URLs.
- Health reports database, migration, uptime, storage, worker state, and media queues without leaking internals.
- Settings provide identity/regional grouping, live preview, updater identity, dirty-state reset, read-only mode, and changed-only saves.
- Audit supports general search, actor/date/page-size filters, removable chips, filtered summaries, desktop table, mobile cards, and private details in a sheet.
- No database migration or dependency was introduced.
- Focused tests, complete tests, typecheck, lint, vet, production builds, responsive inspection, and `git diff --check` all pass.
