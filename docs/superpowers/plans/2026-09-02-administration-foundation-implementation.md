# Administration Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the protected Konkit administration dashboard with profile management, users, roles and permissions, safe system settings, dependency health, and append-only audit history.

**Architecture:** Keep the application as a Go modular monolith backed by one PostgreSQL database. Go owns session authentication, permission checks, CSRF, validation, transactions, and JSON APIs; React owns the authenticated dashboard experience while the existing login remains intact. Public liveness remains dependency-free, while protected readiness checks PostgreSQL and migration state.

**Tech Stack:** Go 1.26, `net/http`, pgx/v5, goose/v3, PostgreSQL 18, React 19, TypeScript, Vite 7, Base UI, React Router, TanStack Query, Lucide React, CSS Modules, Vitest, Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-02-administration-foundation-design.md`

## Global Constraints

- Keep browser authentication in the existing `konkit_session` HttpOnly cookie; never store credentials or session tokens in browser storage.
- Keep `.env`, `.flyenv`, database URLs, session secrets, and integration credentials out of Git and API responses.
- Every protected API route authenticates in Go; every mutation also verifies CSRF and its resource permission.
- `super_admin` has implicit full access, cannot be deleted, and the system must retain at least one active Super Admin.
- Use one PostgreSQL database for all future regencies and both Petani and Nelayan programs.
- Do not add regency, recipient, program, BAST, map, offline, SMTP, or microservice-token behavior in this phase.
- Use Indonesian display copy, ASCII source where practical, radius no greater than 8px, and responsive layouts at 360px and 1366px.
- Follow red-green-refactor for every behavior change and commit after every independently reviewable task.

---

### Task 1: Administration Schema And Permission Seed

**Files:**
- Create: `internal/database/migrations/00002_administration.sql`
- Modify: `internal/auth/models.go`
- Modify: `internal/auth/repository_integration_test.go`

**Interfaces:**
- Consumes: existing `users`, `roles`, `permissions`, `user_roles`, and `sessions` tables.
- Produces: `users.full_name`, `users.updated_by`, `system_settings`, `audit_logs`, and the nine permissions named in the spec.

- [ ] **Step 1: Write the failing migration integration test**

Add a test that applies embedded migrations to `konkit_test`, then asserts the new columns, tables, seeded setting keys, and permissions exist:

```go
func TestAdministrationMigrationCreatesFoundation(t *testing.T) {
	pool := migratedTestPool(t)
	for _, permission := range []string{
		"dashboard.view", "users.view", "users.manage", "roles.view",
		"roles.manage", "settings.view", "settings.manage", "health.view", "audit.view",
	} {
		var exists bool
		if err := pool.QueryRow(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM permissions WHERE code = $1)", permission,
		).Scan(&exists); err != nil || !exists {
			t.Fatalf("permission %s missing: exists=%v err=%v", permission, exists, err)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
go test ./internal/auth -run TestAdministrationMigrationCreatesFoundation -count=1
```

Expected: FAIL because migration `00002_administration.sql` and the new permissions do not exist.

- [ ] **Step 3: Add the migration**

Create an up/down goose migration that:

```sql
ALTER TABLE users ADD COLUMN full_name text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN updated_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX users_active_created_idx ON users (is_active, created_at DESC);

CREATE TABLE system_settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    value_type text NOT NULL CHECK (value_type IN ('string', 'timezone', 'date_format', 'locale')),
    description text,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip_address inet,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now()
);
```

Add indexes for `created_at`, `actor_user_id`, `action`, and `(resource_type, resource_id)`. Insert the missing permissions with `ON CONFLICT (code) DO UPDATE` for names/descriptions. Insert these setting keys: `application_name`, `timezone`, `date_format`, `locale`, and `organization_name`, using the defaults from the spec. The down migration drops audit/settings, removes only the new permission codes except `dashboard.view`, then removes the two user columns.

- [ ] **Step 4: Extend the authenticated principal model**

Add `FullName string` to `auth.User` and `auth.Principal`, select it in `FindUserByIdentity` and `PrincipalForUser`, and update existing fakes/tests with explicit names.

- [ ] **Step 5: Run migration and auth tests and verify GREEN**

Run:

```powershell
go test ./internal/auth ./internal/database -count=1
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Expected: tests PASS and goose reports both `00001_auth.sql` and `00002_administration.sql` applied.

- [ ] **Step 6: Commit**

```powershell
git add internal/database/migrations/00002_administration.sql internal/auth
git commit -m "feat: add administration schema and permissions"
```

---

### Task 2: Transactional Audit Recorder

**Files:**
- Create: `internal/audit/models.go`
- Create: `internal/audit/recorder.go`
- Create: `internal/audit/recorder_test.go`
- Create: `internal/audit/repository.go`
- Create: `internal/audit/repository_integration_test.go`

**Interfaces:**
- Consumes: `audit_logs` from Task 1 and pgx `Exec`/`Query` behavior.
- Produces: `audit.Record(ctx, querier, Event) error` and `Repository.List(ctx, Filter) (Page, error)`.

- [ ] **Step 1: Write failing tests for event sanitization and pagination**

Define and test these public types:

```go
type Event struct {
	ActorUserID string
	Action string
	ResourceType string
	ResourceID string
	Metadata map[string]any
	IPAddress string
	UserAgent string
}

type Filter struct {
	Page int
	PageSize int
	Action string
	ResourceType string
	ActorUserID string
}
```

The test must reject blank action/resource type, remove metadata keys `password`, `password_hash`, `session_secret`, and `database_url` recursively, clamp page size to 100, and return newest events first.

- [ ] **Step 2: Run audit tests and verify RED**

Run `go test ./internal/audit -count=1`.

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the recorder and repository**

Use a narrow transaction-compatible interface:

```go
type Querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func Record(ctx context.Context, db Querier, event Event) error
```

Marshal only sanitized metadata. Convert blank actor/resource/IP/user-agent values to SQL `NULL`. Implement `List` with parameterized filters, total count, deterministic `created_at DESC, id DESC`, and page metadata.

- [ ] **Step 4: Run unit and PostgreSQL integration tests and verify GREEN**

Run:

```powershell
go test ./internal/audit -count=1
```

Expected: PASS; integration tests skip only when `TEST_DATABASE_URL` is not configured.

- [ ] **Step 5: Commit**

```powershell
git add internal/audit
git commit -m "feat: add transactional administration audit log"
```

---

### Task 3: Profile Service

**Files:**
- Create: `internal/profile/models.go`
- Create: `internal/profile/service.go`
- Create: `internal/profile/service_test.go`
- Create: `internal/profile/repository.go`
- Create: `internal/profile/repository_integration_test.go`

**Interfaces:**
- Consumes: auth password hashing/verification, users/sessions tables, and `audit.Record`.
- Produces: `Service.Get`, `Service.Update`, and `Service.ChangePassword`.

- [ ] **Step 1: Write failing profile service tests**

Specify these methods:

```go
func (s *Service) Get(ctx context.Context, userID string) (Profile, error)
func (s *Service) Update(ctx context.Context, actor auth.Principal, input UpdateInput, meta auth.ClientMeta) (Profile, error)
func (s *Service) ChangePassword(ctx context.Context, actor auth.Principal, currentRawToken string, input PasswordInput, meta auth.ClientMeta) error
```

Test trimming names, lowercase identity normalization, duplicate identity conflicts, current-password verification, minimum 12-character new password, unchanged-password rejection, and deletion of all sessions except the SHA-256 hash of `currentRawToken`.

- [ ] **Step 2: Run profile tests and verify RED**

Run `go test ./internal/profile -count=1`.

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement service validation and repository transactions**

Define sentinel errors `ErrNotFound`, `ErrIdentityInUse`, `ErrCurrentPassword`, `ErrPasswordTooShort`, and `ErrPasswordUnchanged`. Update profile plus audit in one transaction. Change password, revoke other sessions, and record `profile.password_changed` without password metadata in one transaction.

- [ ] **Step 4: Run profile tests and verify GREEN**

Run `go test ./internal/profile -count=1`.

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/profile
git commit -m "feat: add self-service profile management"
```

---

### Task 4: User And Role Administration Services

**Files:**
- Create: `internal/administration/models.go`
- Create: `internal/administration/users.go`
- Create: `internal/administration/users_test.go`
- Create: `internal/administration/roles.go`
- Create: `internal/administration/roles_test.go`
- Create: `internal/administration/repository.go`
- Create: `internal/administration/repository_integration_test.go`

**Interfaces:**
- Consumes: auth password hashing, administration tables, and `audit.Record`.
- Produces: paginated user operations, role CRUD, and permission listing.

- [ ] **Step 1: Write failing user-service tests**

Use these operations:

```go
func (s *Service) ListUsers(ctx context.Context, filter UserFilter) (UserPage, error)
func (s *Service) CreateUser(ctx context.Context, actor auth.Principal, input CreateUserInput, meta auth.ClientMeta) (UserDetail, error)
func (s *Service) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input UpdateUserInput, meta auth.ClientMeta) (UserDetail, error)
func (s *Service) SetPassword(ctx context.Context, actor auth.Principal, userID, password string, meta auth.ClientMeta) error
```

Test validation, duplicate identity, unknown role IDs, password minimum, role replacement, session revocation on deactivation/password reset, self-deactivation rejection, and protection of the last active Super Admin.

- [ ] **Step 2: Write failing role-service tests**

Use these operations:

```go
func (s *Service) ListRoles(ctx context.Context) ([]Role, error)
func (s *Service) ListPermissions(ctx context.Context) ([]PermissionGroup, error)
func (s *Service) CreateRole(ctx context.Context, actor auth.Principal, input RoleInput, meta auth.ClientMeta) (Role, error)
func (s *Service) UpdateRole(ctx context.Context, actor auth.Principal, roleID string, input RoleInput, meta auth.ClientMeta) (Role, error)
func (s *Service) DeleteRole(ctx context.Context, actor auth.Principal, roleID string, meta auth.ClientMeta) error
```

Test stable lowercase codes matching `^[a-z][a-z0-9_]{2,49}$`, unknown permission rejection, system-role mutation rejection, role-in-use conflict, and transactional permission replacement.

- [ ] **Step 3: Run administration tests and verify RED**

Run `go test ./internal/administration -count=1`.

Expected: FAIL because the package does not exist.

- [ ] **Step 4: Implement service rules and PostgreSQL locking**

Use `SELECT ... FOR UPDATE` on the target user and active Super Admin membership before deactivation or removal of `super_admin`. Perform each mutation and its audit record in the same pgx transaction. Return typed errors so API handlers can map validation to `400`, missing rows to `404`, and conflicts to `409`.

- [ ] **Step 5: Run unit and integration tests and verify GREEN**

Run `go test ./internal/administration -count=1`.

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/administration
git commit -m "feat: add user and role administration services"
```

---

### Task 5: Typed System Settings And Protected Health

**Files:**
- Create: `internal/settings/registry.go`
- Create: `internal/settings/service.go`
- Create: `internal/settings/service_test.go`
- Create: `internal/settings/repository.go`
- Create: `internal/health/service.go`
- Create: `internal/health/service_test.go`

**Interfaces:**
- Consumes: `system_settings`, `audit.Record`, pgx pool ping/query, app environment, and process start time.
- Produces: typed settings read/update and `health.Service.Check(ctx) Report`.

- [ ] **Step 1: Write failing settings tests**

Define a closed registry containing only:

```go
var Definitions = map[string]Definition{
	"application_name": {Type: "string", MaxLength: 120},
	"timezone": {Type: "timezone", Allowed: []string{"Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"}},
	"date_format": {Type: "date_format", Allowed: []string{"02/01/2006", "02 January 2006"}},
	"locale": {Type: "locale", Allowed: []string{"id-ID"}},
	"organization_name": {Type: "string", MaxLength: 160},
}
```

Test unknown-key rejection, type checking, whitespace normalization, allowed values, atomic multi-key update, and audit old/new values.

- [ ] **Step 2: Write failing health tests**

Define:

```go
type Report struct {
	Status string `json:"status"`
	Database Component `json:"database"`
	MigrationVersion int64 `json:"migration_version"`
	Environment string `json:"environment"`
	Version string `json:"version"`
	UptimeSeconds int64 `json:"uptime_seconds"`
	CheckedAt time.Time `json:"checked_at"`
}
```

Test `healthy` for successful ping/version query, `degraded` when ping succeeds but migration state cannot be read, `unhealthy` for failed PostgreSQL, a maximum two-second check context, UTC check time, and absence of database host or credentials.

- [ ] **Step 3: Run focused tests and verify RED**

Run `go test ./internal/settings ./internal/health -count=1`.

Expected: FAIL because both packages do not exist.

- [ ] **Step 4: Implement settings and health services**

Settings updates must use a transaction and one audit event `settings.updated`. Health queries `SELECT 1` and `MAX(version_id) FROM goose_db_version WHERE is_applied = true`; errors become component messages with safe codes such as `database_unavailable`, not raw driver errors.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run `go test ./internal/settings ./internal/health -count=1`.

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/settings internal/health
git commit -m "feat: add typed settings and readiness health"
```

---

### Task 6: Authenticated JSON API

**Files:**
- Modify: `internal/auth/csrf.go`
- Create: `internal/auth/cookie.go`
- Replace: `internal/api/handler.go`
- Create: `internal/api/middleware.go`
- Create: `internal/api/errors.go`
- Create: `internal/api/me.go`
- Create: `internal/api/users.go`
- Create: `internal/api/roles.go`
- Create: `internal/api/system.go`
- Expand: `internal/api/handler_test.go`
- Modify: `internal/web/auth.go`
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`

**Interfaces:**
- Consumes: services from Tasks 3-5 and existing `auth.Service`.
- Produces: all `/api/v1` endpoints listed in the spec and `GET /api/v1/me` bootstrap data.

- [ ] **Step 1: Write failing API middleware tests**

Test public `GET /api/v1/health`, JSON `401` without a session on protected routes, JSON `403` for missing permission, CSRF rejection on mutations, `405` plus `Allow`, `404`, a 1 MiB JSON body limit, and `Content-Type: application/json` enforcement.

The `/api/v1/me` success payload must include:

```json
{
  "data": {
    "id": "user-id",
    "full_name": "Admin Konkit",
    "username": "admin",
    "email": "admin@example.test",
    "roles": ["super_admin"],
    "permissions": ["*"]
  },
  "meta": {"csrf_token": "derived-token"}
}
```

- [ ] **Step 2: Run API tests and verify RED**

Run `go test ./internal/api -count=1`.

Expected: FAIL because the current handler only supports public health.

- [ ] **Step 3: Implement shared auth context and API dependencies**

Move the cookie name to `auth.SessionCookieName`. Add `AuthService` and service interfaces to:

```go
type Dependencies struct {
	Auth AuthService
	Profile ProfileService
	Administration AdministrationService
	Settings SettingsService
	Health HealthService
	Audit AuditService
	SessionSecret []byte
}

func NewHandler(deps Dependencies) http.Handler
```

Authentication stores both principal and raw session token in request context. Permission middleware calls `Auth.Can`. CSRF middleware verifies `X-CSRF-Token` against the raw token.

- [ ] **Step 4: Implement resource handlers and consistent errors**

Implement the exact methods/routes from the spec. Decode one JSON object, reject trailing JSON, map typed service errors to stable codes, and log internal failures without response details. Return list metadata as `{"page":1,"page_size":20,"total":42}`.

- [ ] **Step 5: Wire production dependencies**

Construct repositories/services from the single pgx pool in `cmd/server/main.go`. Keep the public health path operational without authentication and pass the same auth service to web and API handlers.

- [ ] **Step 6: Run API, web, and server tests and verify GREEN**

Run:

```powershell
go test ./internal/api ./internal/web ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/api internal/auth/cookie.go internal/auth/csrf.go internal/web/auth.go cmd/server
git commit -m "feat: expose protected administration API"
```

---

### Task 7: React Dashboard Foundation

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/src/main.tsx`
- Create: `frontend/src/app/DashboardApp.tsx`
- Create: `frontend/src/app/AppShell.tsx`
- Create: `frontend/src/app/AppShell.module.css`
- Create: `frontend/src/app/routes.tsx`
- Create: `frontend/src/lib/api.ts`
- Create: `frontend/src/lib/queryClient.ts`
- Create: `frontend/src/styles/dashboardTokens.css`
- Create: `frontend/src/test/setup.ts`
- Create: `frontend/src/app/AppShell.test.tsx`
- Modify: `frontend/vite.config.ts`
- Modify: `web/templates/dashboard.html`
- Modify: `internal/web/dashboard.go`
- Modify: `internal/web/server.go`
- Expand: `internal/web/server_test.go`

**Interfaces:**
- Consumes: `GET /api/v1/me`, existing Vite manifest loader, and authenticated `/dashboard` web route.
- Produces: dashboard React mount, responsive navigation, route fallback, API client, and permission-aware menu.

- [ ] **Step 1: Install and configure frontend test/runtime dependencies**

Run:

```powershell
npm.cmd --prefix frontend install react-router-dom @tanstack/react-query lucide-react
npm.cmd --prefix frontend install --save-dev vitest jsdom @testing-library/react @testing-library/jest-dom @testing-library/user-event
```

Add scripts `test: "vitest run"` and `test:watch: "vitest"`, configure Vite `test.environment = "jsdom"`, and load `src/test/setup.ts`.

- [ ] **Step 2: Write the failing application-shell tests**

Render with a mocked `/api/v1/me` response. Assert Ergas/KSM branding, Dashboard/Administrasi/Sistem/Akun groups, profile menu, permission-hidden links, mobile menu toggle with accessible label, and redirect-to-login behavior on API `401`.

- [ ] **Step 3: Run the shell test and verify RED**

Run `npm.cmd --prefix frontend run test -- AppShell.test.tsx`.

Expected: FAIL because the dashboard app does not exist.

- [ ] **Step 4: Implement API client and dashboard shell**

`api.ts` must use `credentials: "same-origin"`, send JSON headers, attach the CSRF token from bootstrap metadata on non-GET requests, parse the common error shape, redirect to `/login` on `401`, and throw typed `ApiError` for other failures.

Render `LoginPage` only at `/login`; render `DashboardApp` for paths under `/dashboard`. Use `BrowserRouter basename="/dashboard"`, TanStack Query, Lucide icons, semantic navigation, a permanent desktop sidebar, and a focus-trapped Base UI drawer on mobile.

- [ ] **Step 5: Replace the dashboard HTML with a React shell**

Serve `#konkit-root` plus the current Vite script/styles from the manifest. Handle both `/dashboard` and `/dashboard/` descendants through the authenticated dashboard handler; unknown non-dashboard web paths remain `404`.

- [ ] **Step 6: Run frontend and web tests and verify GREEN**

Run:

```powershell
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
go test ./internal/web ./cmd/server -count=1
```

Expected: PASS and manifest points to the new dashboard-capable bundle.

- [ ] **Step 7: Commit**

```powershell
git add frontend web/templates/dashboard.html internal/web web/static/app
git commit -m "feat: add responsive React administration shell"
```

---

### Task 8: Dashboard, Profile, Users, And Roles UI

**Files:**
- Create: `frontend/src/features/dashboard/DashboardPage.tsx`
- Create: `frontend/src/features/dashboard/DashboardPage.module.css`
- Create: `frontend/src/features/profile/ProfilePage.tsx`
- Create: `frontend/src/features/profile/ChangePasswordForm.tsx`
- Create: `frontend/src/features/users/UsersPage.tsx`
- Create: `frontend/src/features/users/UserDialog.tsx`
- Create: `frontend/src/features/roles/RolesPage.tsx`
- Create: `frontend/src/features/roles/RoleDialog.tsx`
- Create: `frontend/src/components/DataTable.tsx`
- Create: `frontend/src/components/FormField.tsx`
- Create: `frontend/src/components/StatusBadge.tsx`
- Create tests beside every page/dialog component.
- Modify: `frontend/src/app/routes.tsx`

**Interfaces:**
- Consumes: `/me`, `/admin/users`, `/admin/roles`, and `/admin/permissions` APIs.
- Produces: complete profile, user, and role workflows plus truthful dashboard summaries.

- [ ] **Step 1: Write failing page-flow tests**

Test dashboard loading/empty/error states, profile update, current/new password validation, paginated user search/filter, create/edit/deactivate flows, role permission matrix, system-role disabled controls, conflict messages, and keyboard focus returning to the trigger after dialogs close.

- [ ] **Step 2: Run focused frontend tests and verify RED**

Run:

```powershell
npm.cmd --prefix frontend run test -- DashboardPage ProfilePage UsersPage RolesPage
```

Expected: FAIL because pages do not exist.

- [ ] **Step 3: Implement shared controls and dashboard/profile pages**

Use compact page headings, inline validation, skeleton rows with fixed dimensions, explicit empty states, and non-card page sections. Dashboard cards are allowed only for repeated metrics and must show real API values.

- [ ] **Step 4: Implement user and role pages**

Keep filtering/pagination in URL search params. Use dialogs for a single record edit, confirm destructive state changes, invalidate affected TanStack queries after success, and render field errors returned by the API next to their controls.

- [ ] **Step 5: Run frontend tests and build and verify GREEN**

Run:

```powershell
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src frontend/package.json frontend/package-lock.json web/static/app
git commit -m "feat: add profile user and role dashboard workflows"
```

---

### Task 9: Settings, Health, And Audit UI

**Files:**
- Create: `frontend/src/features/settings/SettingsPage.tsx`
- Create: `frontend/src/features/settings/SettingsPage.test.tsx`
- Create: `frontend/src/features/health/HealthPage.tsx`
- Create: `frontend/src/features/health/HealthPage.test.tsx`
- Create: `frontend/src/features/audit/AuditPage.tsx`
- Create: `frontend/src/features/audit/AuditPage.test.tsx`
- Modify: `frontend/src/app/routes.tsx`

**Interfaces:**
- Consumes: `/system/settings`, `/system/health`, and `/system/audit-logs` APIs.
- Produces: typed settings editor, protected readiness view, and read-only audit history.

- [ ] **Step 1: Write failing system-page tests**

Test settings grouped controls and save-state behavior, health `healthy/degraded/unhealthy` presentation without credential fields, manual refresh, audit filters/pagination, read-only audit details, and permission-hidden routes.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
npm.cmd --prefix frontend run test -- SettingsPage HealthPage AuditPage
```

Expected: FAIL because pages do not exist.

- [ ] **Step 3: Implement settings, health, and audit pages**

Use select controls for timezone/date format/locale, text inputs with server limits for names, status icons plus text for health, and a dense filterable audit table. Never render arbitrary environment entries, raw backend errors, or secret-like metadata.

- [ ] **Step 4: Run frontend tests and build and verify GREEN**

Run:

```powershell
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add frontend/src/features frontend/src/app/routes.tsx web/static/app
git commit -m "feat: add settings health and audit views"
```

---

### Task 10: End-To-End Verification And Documentation

**Files:**
- Create: `frontend/e2e/administration.spec.ts`
- Create: `frontend/playwright.config.ts`
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `README.md`

**Interfaces:**
- Consumes: the completed backend, migrated `konkit_test`, and built React dashboard.
- Produces: reproducible E2E coverage, responsive screenshots, and operator documentation.

- [ ] **Step 1: Add Playwright and write the failing administration journey**

Install `@playwright/test` as a dev dependency and add an `e2e` script. The test logs in with a seeded test-only account, visits dashboard/profile/users/roles/settings/health/audit, creates then deactivates a non-admin test user, verifies audit entries, logs out, and confirms protected routes return to login.

- [ ] **Step 2: Run E2E and verify RED before final wiring**

Run `npm.cmd --prefix frontend run e2e`.

Expected: FAIL until the test server/seed setup and all selectors are connected.

- [ ] **Step 3: Add deterministic E2E setup and documentation**

Configure Playwright web server to use `APP_ADDR=:8082`, `APP_BASE_URL=http://127.0.0.1:8082`, and the dedicated `konkit_test` database. Seed only through a test helper guarded by a database-name assertion. Document `.env`, migration, admin creation, frontend build, all test commands, health endpoints, and the fact that protected health requires login.

- [ ] **Step 4: Verify desktop and mobile visuals**

Capture and inspect screenshots at 1366x768 and 390x844 for every dashboard route. Confirm no overlap, horizontal overflow, clipped controls, blank content, nested cards, unreadable statuses, or layout shift when drawers/dialogs open.

- [ ] **Step 5: Run the complete verification suite**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
go test ./... -count=1
go vet ./...
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
npm.cmd --prefix frontend run e2e
go run ./cmd/migrate status
```

Expected: all commands exit `0`; migration status lists `00001` and `00002` as applied.

- [ ] **Step 6: Run a tracked-secret check**

Run:

```powershell
git grep -n -E "DATABASE_URL=.*@|SESSION_SECRET=.{32}" -- ':!*.example' ':!docs/**'
```

Expected: no matches.

- [ ] **Step 7: Commit**

```powershell
git add frontend README.md web/static/app
git commit -m "test: verify administration foundation workflows"
```

- [ ] **Step 8: Review the branch diff**

Run:

```powershell
git status --short
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
```

Expected: clean working tree, ten focused feature/test commits, and only administration-foundation files in the diff.
