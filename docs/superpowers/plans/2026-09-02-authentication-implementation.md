# Konkit Authentication Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build working PostgreSQL-backed login, RBAC foundations, secure browser sessions, an interactive Super Admin CLI, a protected dashboard shell, and a versioned API boundary.

**Architecture:** The Go process loads validated environment configuration, connects to PostgreSQL through `pgx`, and delegates authentication to a focused `internal/auth` package. Browser requests use opaque server-side sessions and CSRF-protected form posts; authorization uses normalized role and permission tables, while `/api/v1` remains a JSON-only boundary ready for later token authentication.

**Tech Stack:** Go 1.26, PostgreSQL 18, `pgx/v5`, `goose/v3`, Argon2id from `golang.org/x/crypto`, React 19, TypeScript, Vite 7, Go `net/http` and `html/template`.

**Spec:** `docs/superpowers/specs/2026-09-02-authentication-design.md`

## Global Constraints

- Browser login uses server-side session cookies, not JWT.
- Passwords use Argon2id and are never logged or stored in plaintext.
- Migration creates RBAC tables and the system `super_admin` role, but no default password.
- Super Admin creation is interactive through `go run ./cmd/admin create`.
- Cookie flags are `HttpOnly`, `SameSite=Lax`, path `/`, and `Secure=true` only when configured.
- HTML handlers redirect anonymous users; `/api/v1` handlers return JSON and never HTML redirects.
- `.flyenv` contains local secrets and must not be committed.
- The repository is not initialized as Git, so commit steps are recorded but skipped until Git is initialized.

---

## File Structure

- Create `.gitignore`: excludes FlyEnv secrets, local caches, executables, and frontend dependencies.
- Create `.env.example`: documents required variables without real credentials.
- Create `internal/config/config.go`: typed environment loading and validation.
- Create `internal/config/config_test.go`: configuration unit tests.
- Create `internal/database/postgres.go`: PostgreSQL pool construction and ping.
- Create `internal/database/migrations/embed.go`: embeds SQL migrations for the migration command.
- Create `internal/database/migrations/00001_auth.sql`: users, sessions, roles, permissions, and join tables.
- Create `cmd/migrate/main.go`: explicit `up`, `down`, and `status` migration command.
- Create `internal/auth/password.go`: Argon2id hash creation and verification.
- Create `internal/auth/password_test.go`: password tests including malformed hashes.
- Create `internal/auth/models.go`: user, principal, and client metadata types.
- Create `internal/auth/repository.go`: PostgreSQL queries and transactions for users, RBAC, and sessions.
- Create `internal/auth/repository_integration_test.go`: optional PostgreSQL integration coverage via `TEST_DATABASE_URL`.
- Create `internal/admin/create.go`: testable interactive Super Admin creation workflow.
- Create `internal/admin/create_test.go`: CLI validation tests.
- Create `cmd/admin/main.go`: `create` command wiring and hidden password input.
- Create `internal/auth/session.go`: session token lifecycle and authentication service.
- Create `internal/auth/session_test.go`: service tests with a fake store and deterministic clock.
- Create `internal/auth/csrf.go`: session-bound HMAC CSRF token generation and verification.
- Create `internal/auth/csrf_test.go`: CSRF unit tests.
- Create `internal/auth/limiter.go`: bounded in-memory login attempt limiter.
- Create `internal/auth/limiter_test.go`: limiter window tests.
- Split `internal/web/server.go`: composition, static files, and Vite manifest only.
- Create `internal/web/auth.go`: login, logout, cookie, and auth middleware handlers.
- Create `internal/web/auth_test.go`: HTTP authentication behavior tests.
- Create `internal/web/dashboard.go`: protected dashboard shell handler.
- Create `web/templates/dashboard.html`: initial authenticated shell and CSRF logout form.
- Modify `frontend/src/pages/LoginPage/LoginPage.tsx`: required fields, submitting state, generic error message, and hidden reset link.
- Modify `frontend/src/pages/LoginPage/LoginPage.module.css`: accessible error and disabled-button states.
- Create `internal/api/handler.go`: JSON-only `/api/v1/health` boundary.
- Create `internal/api/handler_test.go`: content-type and JSON contract tests.
- Modify `cmd/server/main.go`: config, database, auth, API, and graceful shutdown wiring.
- Modify `go.mod` and `go.sum`: required Go dependencies.
- Create `README.md`: exact FlyEnv, migration, admin, and local-run commands.

## Task 1: Validated Configuration And Secret Hygiene

**Files:**
- Create: `.gitignore`
- Create: `.env.example`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` with `Env`, `Addr`, `BaseURL`, `DatabaseURL`, `SessionSecret`, `SessionCookieSecure`, `SessionTTL`, and `RememberTTL`.
- Produces: `config.Load() (config.Config, error)`.

- [ ] **Step 1: Write failing configuration tests**

Create table-driven tests using an injected lookup function:

```go
func TestLoadFromRejectsMissingDatabaseURL(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"SESSION_SECRET": "01234567890123456789012345678901",
	}))
	if !errors.Is(err, ErrDatabaseURLRequired) {
		t.Fatalf("expected ErrDatabaseURLRequired, got %v", err)
	}
}

func TestLoadFromParsesLocalConfiguration(t *testing.T) {
	cfg, err := loadFrom(mapLookup(map[string]string{
		"APP_ENV": "local", "APP_ADDR": ":8080",
		"APP_BASE_URL": "http://localhost:8080",
		"DATABASE_URL": "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET": "01234567890123456789012345678901",
		"SESSION_COOKIE_SECURE": "false", "SESSION_TTL": "12h",
	}))
	if err != nil { t.Fatal(err) }
	if cfg.SessionTTL != 12*time.Hour || cfg.RememberTTL != 30*24*time.Hour {
		t.Fatalf("unexpected durations: %+v", cfg)
	}
}
```

- [ ] **Step 2: Run the tests and confirm RED**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'
go test ./internal/config -count=1
```

Expected: FAIL because `internal/config` does not exist.

- [ ] **Step 3: Implement typed configuration**

Implement these exact public fields and defaults:

```go
type Config struct {
	Env                 string
	Addr                string
	BaseURL             string
	DatabaseURL         string
	SessionSecret       []byte
	SessionCookieSecure bool
	SessionTTL          time.Duration
	RememberTTL         time.Duration
}

func Load() (Config, error) { return loadFrom(os.LookupEnv) }
```

`loadFrom` must default `APP_ENV=local`, `APP_ADDR=:8080`, `APP_BASE_URL=http://localhost:8080`, `SESSION_TTL=12h`, and remembered duration to 720 hours. It must reject an empty `DATABASE_URL`, secrets shorter than 32 bytes, invalid booleans, and non-positive durations.

- [ ] **Step 4: Protect local secrets and document variables**

Create `.gitignore`:

```gitignore
.flyenv
.env
.cache/
.tmp/
frontend/node_modules/
*.exe
```

Create `.env.example` with safe non-secret examples:

```dotenv
APP_ENV=local
APP_ADDR=:8080
APP_BASE_URL=http://localhost:8080
DATABASE_URL=
SESSION_SECRET=
SESSION_COOKIE_SECURE=false
SESSION_TTL=12h
```

- [ ] **Step 5: Run tests and confirm GREEN**

Run `go test ./internal/config -count=1` and expect PASS.

- [ ] **Step 6: Commit checkpoint**

When Git exists, commit `chore: add validated application configuration`. Until then, leave files uncommitted without initializing Git implicitly.

## Task 2: PostgreSQL Connection And Embedded Migrations

**Files:**
- Modify: `go.mod`
- Create: `go.sum`
- Create: `internal/database/postgres.go`
- Create: `internal/database/postgres_test.go`
- Create: `internal/database/migrations/embed.go`
- Create: `internal/database/migrations/00001_auth.sql`
- Create: `cmd/migrate/main.go`

**Interfaces:**
- Consumes: `config.Config.DatabaseURL`.
- Produces: `database.Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)`.
- Produces: commands `go run ./cmd/migrate up|down|status`.

- [ ] **Step 1: Add dependencies**

Run:

```powershell
go get github.com/jackc/pgx/v5@latest
go get github.com/pressly/goose/v3@latest
```

Expected: `go.mod` and `go.sum` include pgx and goose.

- [ ] **Step 2: Write a failing database URL test**

```go
func TestOpenRejectsInvalidURL(t *testing.T) {
	_, err := Open(context.Background(), "://invalid")
	if err == nil { t.Fatal("expected invalid database URL error") }
}
```

Run `go test ./internal/database -count=1`; expect FAIL because `Open` is undefined.

- [ ] **Step 3: Implement pool creation**

`Open` parses with `pgxpool.ParseConfig`, sets `MaxConns=10`, sets `MinConns=1`, opens the pool, and performs a five-second `Ping`. On ping failure it closes the pool and returns an error wrapped as `ping postgres: %w`.

- [ ] **Step 4: Add the migration schema**

Create a goose SQL migration with this up-order and required goose directives:

```sql
-- +goose Up
CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    description text,
    is_system boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    description text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_username_lower_key ON users (lower(username));
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    ip_address inet,
    user_agent text
);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

INSERT INTO roles (code, name, description, is_system)
VALUES ('super_admin', 'Super Admin', 'Akses penuh ke seluruh sistem', true);
INSERT INTO permissions (code, name, description)
VALUES ('dashboard.view', 'Lihat Dashboard', 'Mengakses halaman dashboard');

-- +goose Down
DROP TABLE sessions;
DROP TABLE role_permissions;
DROP TABLE user_roles;
DROP TABLE users;
DROP TABLE permissions;
DROP TABLE roles;
```

The down section drops `sessions`, `role_permissions`, `user_roles`, `users`, `permissions`, then `roles` in that order.

- [ ] **Step 5: Embed and expose migrations**

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

`cmd/migrate/main.go` loads config, opens `database/sql` with the pgx stdlib driver, calls `goose.SetBaseFS(migrations.FS)`, sets dialect `postgres`, accepts only `up`, `down`, and `status`, then invokes `goose.RunContext(ctx, command, db, ".")`.

- [ ] **Step 6: Verify migration against local PostgreSQL**

Run:

```powershell
go test ./internal/database -count=1
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Expected: tests PASS; migration status shows `00001_auth.sql` applied.

- [ ] **Step 7: Commit checkpoint**

When Git exists, commit `feat: add postgres connection and auth migrations`.

## Task 3: Argon2id Passwords And RBAC Repository

**Files:**
- Create: `internal/auth/models.go`
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`
- Create: `internal/auth/repository.go`
- Create: `internal/auth/repository_integration_test.go`

**Interfaces:**
- Produces: `auth.HashPassword(password string) (string, error)` and `auth.VerifyPassword(password, encoded string) (bool, error)`.
- Produces: `auth.Repository.FindUserByIdentity`, `CreateSuperAdmin`, `HasPermission`, and session persistence methods.

- [ ] **Step 1: Add crypto dependency and failing password tests**

Run `go get golang.org/x/crypto@latest`, then add tests:

```go
func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil { t.Fatal(err) }
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok { t.Fatalf("verify failed: ok=%v err=%v", ok, err) }
	wrong, err := VerifyPassword("wrong password", hash)
	if err != nil || wrong { t.Fatalf("wrong password accepted: ok=%v err=%v", wrong, err) }
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("password", "not-a-phc-hash"); err == nil {
		t.Fatal("expected malformed hash error")
	}
}
```

Run `go test ./internal/auth -run Password -count=1`; expect FAIL.

- [ ] **Step 2: Implement Argon2id PHC encoding**

Use parameters `memory=64*1024`, `iterations=3`, `parallelism=2`, `saltLength=16`, and `keyLength=32`. Encode hashes as `$argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>` and compare derived keys with `subtle.ConstantTimeCompare`.

- [ ] **Step 3: Define auth models**

```go
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInactiveUser = errors.New("inactive user")

type User struct {
	ID, Username, Email, PasswordHash string
	IsActive bool
}

type Principal struct {
	UserID, Username, Email string
	Roles []string
}

func (p Principal) IsSuperAdmin() bool {
	return slices.Contains(p.Roles, "super_admin")
}

type ClientMeta struct { IPAddress, UserAgent string }
```

- [ ] **Step 4: Implement parameterized repository methods**

Use a `Repository` holding `*pgxpool.Pool`. `FindUserByIdentity` executes:

```sql
SELECT id::text, username, email, password_hash, is_active
FROM users
WHERE lower(username) = lower($1) OR lower(email) = lower($1)
LIMIT 1
```

`CreateSuperAdmin` runs one transaction: insert normalized user, select the `super_admin` role, insert `user_roles`, then commit. `HasPermission` returns true immediately for `super_admin`; otherwise it joins `user_roles`, `roles`, `role_permissions`, and `permissions` by user ID and permission code.

- [ ] **Step 5: Add opt-in repository integration tests**

Tests read `TEST_DATABASE_URL`; if absent they call `t.Skip`. The URL must point to a dedicated `konkit_test` database and the test rejects a database name other than `konkit_test` before any cleanup. When valid, tests apply embedded migrations, truncate auth tables between tests, and verify case-insensitive identity lookup, duplicate rejection, role assignment, and `dashboard.view` access.

- [ ] **Step 6: Run tests and confirm GREEN**

Run:

```powershell
go test ./internal/auth -count=1
go test ./internal/auth -run Integration -count=1
```

Expected: unit tests PASS. Integration tests PASS when FlyEnv provides a URL ending in `/konkit_test?sslmode=disable`; otherwise they report SKIP. Integration cleanup removes only rows in `konkit_test` and never drops a database.

- [ ] **Step 7: Commit checkpoint**

When Git exists, commit `feat: add password hashing and rbac repository`.

## Task 4: Interactive Super Admin CLI

**Files:**
- Create: `internal/admin/create.go`
- Test: `internal/admin/create_test.go`
- Create: `cmd/admin/main.go`

**Interfaces:**
- Consumes: `auth.Repository.CreateSuperAdmin` and `auth.HashPassword`.
- Produces: `go run ./cmd/admin create`.

- [ ] **Step 1: Write failing workflow tests**

Test `RunCreate` with injected readers:

```go
func TestRunCreateRejectsMismatchedPasswords(t *testing.T) {
	input := strings.NewReader("admin\nadmin@konkit.local\n")
	passwords := sequencePasswords("long-enough-password", "different-password")
	err := RunCreate(context.Background(), input, io.Discard, passwords, fakeCreator{})
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}
```

Also verify username normalization, valid email requirement, minimum 12-character password, and that plaintext is not passed to the repository.

- [ ] **Step 2: Run tests and confirm RED**

Run `go test ./internal/admin -count=1`; expect FAIL because the package does not exist.

- [ ] **Step 3: Implement the testable workflow**

Define:

```go
type PasswordReader func(prompt string) (string, error)
type Creator interface {
	CreateSuperAdmin(ctx context.Context, username, email, passwordHash string) error
}
func RunCreate(ctx context.Context, in io.Reader, out io.Writer, readPassword PasswordReader, creator Creator) error
```

The function reads username/email with `bufio.Scanner`, reads and confirms the password, hashes it, calls `CreateSuperAdmin`, and prints only `Super Admin berhasil dibuat.`

- [ ] **Step 4: Wire the terminal command**

Run `go get golang.org/x/term@latest`. `cmd/admin/main.go` accepts only `create`, loads config, opens PostgreSQL, and reads hidden passwords with `term.ReadPassword(int(os.Stdin.Fd()))`.

- [ ] **Step 5: Verify CLI tests and create the local account**

Run:

```powershell
go test ./internal/admin -count=1
go run ./cmd/admin create
```

Expected: tests PASS and the command prompts for username, email, password, and confirmation without echoing passwords.

- [ ] **Step 6: Commit checkpoint**

When Git exists, commit `feat: add interactive super admin command`.

## Task 5: Session Service, CSRF, And Login Throttling

**Files:**
- Modify: `internal/auth/repository.go`
- Create: `internal/auth/session.go`
- Test: `internal/auth/session_test.go`
- Create: `internal/auth/csrf.go`
- Test: `internal/auth/csrf_test.go`
- Create: `internal/auth/limiter.go`
- Test: `internal/auth/limiter_test.go`

**Interfaces:**
- Produces: `Service.Login`, `Authenticate`, `Logout`, `Can`, `CSRFToken`, and `VerifyCSRF`.
- Produces: `LoginLimiter.Allow(key string, now time.Time) bool` and `Reset(key string)`.

- [ ] **Step 1: Write failing session lifecycle tests**

Using a fake store and fixed clock, verify a normal login expires after `SessionTTL`, remembered login after `RememberTTL`, wrong password returns `ErrInvalidCredentials`, inactive users return the same public error, expired sessions are rejected, and logout deletes the token hash.

```go
token, err := service.Login(ctx, "admin", "valid-password", false, ClientMeta{})
if err != nil { t.Fatal(err) }
principal, err := service.Authenticate(ctx, token)
if err != nil || principal.Username != "admin" { t.Fatalf("unexpected principal: %+v %v", principal, err) }
```

Run `go test ./internal/auth -run 'Session|Login' -count=1`; expect FAIL.

- [ ] **Step 2: Implement opaque session tokens**

Generate 32 random bytes with `crypto/rand`, expose the raw value as unpadded base64url, and persist only `sha256.Sum256(rawBytes)`. The repository inserts sessions, resolves an unexpired active user with aggregated role codes, updates `last_login_at`, deletes one session hash, and deletes expired sessions.

- [ ] **Step 3: Write and implement CSRF tests**

Define:

```go
func CSRFToken(secret []byte, sessionToken string) string
func VerifyCSRF(secret []byte, sessionToken, submitted string) bool
```

Use HMAC-SHA256 over the raw session token and constant-time comparison. Tests must reject changed session tokens, changed submitted tokens, and empty values.

- [ ] **Step 4: Write and implement limiter tests**

`NewLoginLimiter(5, 15*time.Minute)` permits five attempts for the same normalized `IP|identity` key, rejects the sixth within the window, permits attempts after the window, and `Reset` clears successful-login history. A cleanup pass removes expired keys so the map remains bounded.

- [ ] **Step 5: Run all auth tests**

Run `go test ./internal/auth -count=1`; expect PASS.

- [ ] **Step 6: Commit checkpoint**

When Git exists, commit `feat: add secure session and csrf services`.

## Task 6: Web Login, Logout, And Authentication Middleware

**Files:**
- Modify: `internal/web/server.go`
- Create: `internal/web/auth.go`
- Create: `internal/web/auth_test.go`
- Modify: `internal/web/server_test.go`

**Interfaces:**
- Consumes: an `AuthService` interface implemented by `*auth.Service`.
- Produces: `web.NewHandler(web.Dependencies) http.Handler`.
- Produces: `GET/POST /login`, `POST /logout`, and `RequireAuth` behavior.

- [ ] **Step 1: Define the web dependency interface in failing tests**

```go
type AuthService interface {
	Login(context.Context, string, string, bool, auth.ClientMeta) (string, error)
	Authenticate(context.Context, string) (auth.Principal, error)
	Logout(context.Context, string) error
	Can(context.Context, auth.Principal, string) (bool, error)
}

type Dependencies struct {
	Auth AuthService
	API http.Handler
	SessionSecret []byte
	CookieSecure bool
	SessionTTL time.Duration
	RememberTTL time.Duration
}
```

Handler tests cover method restrictions, missing fields, invalid credentials, successful cookie flags, successful redirect, authenticated `/login` redirect, anonymous dashboard redirect, invalid CSRF logout, and valid logout.

- [ ] **Step 2: Run tests and confirm RED**

Run `go test ./internal/web -count=1`; expect compile failure until constructor and handlers accept dependencies.

- [ ] **Step 3: Refactor server composition**

Keep project-root discovery, static serving, template parsing, favicon, and Vite manifest loading in `server.go`. Constructor becomes `NewHandler(deps Dependencies) http.Handler`; tests pass a fake auth implementation and API handler rather than a real database.

- [ ] **Step 4: Implement login and cookie behavior**

The POST handler calls `ParseForm`, validates `identity` and `password`, applies the limiter key, calls `Auth.Login`, and sets cookie name `konkit_session`. Cookie uses `HttpOnly`, `SameSite=http.SameSiteLaxMode`, configured `Secure`, and the selected max age. Failures redirect to `/login?error=invalid`; throttling redirects to `/login?error=throttled`.

- [ ] **Step 5: Implement auth middleware and logout**

Middleware reads `konkit_session`, authenticates it, places `auth.Principal` and raw session token into request context, and redirects invalid HTML sessions to `/login`. Logout verifies form field `csrf_token`, revokes the session, expires the cookie with `MaxAge=-1`, and redirects to `/login`.

- [ ] **Step 6: Run web tests and confirm GREEN**

Run `go test ./internal/web -count=1`; expect PASS.

- [ ] **Step 7: Commit checkpoint**

When Git exists, commit `feat: connect web login to authentication service`.

## Task 7: Login Feedback And Protected Dashboard Shell

**Files:**
- Modify: `frontend/src/pages/LoginPage/LoginPage.tsx`
- Modify: `frontend/src/pages/LoginPage/LoginPage.module.css`
- Create: `internal/web/dashboard.go`
- Create: `web/templates/dashboard.html`
- Modify: `internal/web/auth_test.go`

**Interfaces:**
- Consumes: `/login?error=invalid|throttled` and an authenticated principal from request context.
- Produces: accessible login feedback and `GET /dashboard` with CSRF logout form.

- [ ] **Step 1: Add failing HTTP contract tests**

Assert authenticated dashboard HTML contains the username, `name="csrf_token"`, a non-empty token value, and a POST form targeting `/logout`. Assert anonymous requests receive `302 /login`.

- [ ] **Step 2: Implement dashboard shell**

`dashboard.go` renders `DashboardPageData{Title, Username, CSRFToken}`. The template contains a restrained header with Ergas/KSM identity, the heading `Dashboard Konkit`, text `Selamat datang, {{ .Username }}`, and this logout form:

```html
<form method="post" action="/logout">
  <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
  <button type="submit">Keluar</button>
</form>
```

- [ ] **Step 3: Improve the React login form**

Add `required`, `maxLength`, an `onSubmit` handler that sets `submitting=true`, and render a `role="alert"` message selected from `URLSearchParams`. Remove the inactive `Lupa password?` anchor. The button label becomes `Memproses...` while disabled.

- [ ] **Step 4: Add accessible styles**

Add `.formError` with a pale red background, dark red text, 1px border, and 7px radius. Add `button:disabled { cursor: wait; opacity: .72; transform: none; }`. Preserve all existing responsive carousel and logo rules.

- [ ] **Step 5: Build and test**

Run:

```powershell
Set-Location frontend
npm.cmd run build
Set-Location ..
go test ./internal/web -count=1
```

Expected: Vite build succeeds and web tests PASS.

- [ ] **Step 6: Commit checkpoint**

When Git exists, commit `feat: add protected dashboard shell and login feedback`.

## Task 8: Versioned JSON API Boundary

**Files:**
- Create: `internal/api/handler.go`
- Test: `internal/api/handler_test.go`
- Modify: `internal/web/server.go`

**Interfaces:**
- Produces: `api.NewHandler() http.Handler`.
- Produces: `GET /api/v1/health` returning JSON.

- [ ] **Step 1: Write the failing API contract test**

```go
func TestHealthReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	NewHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("status=%d", rec.Code) }
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test and confirm RED**

Run `go test ./internal/api -count=1`; expect FAIL because package does not exist.

- [ ] **Step 3: Implement and mount the API handler**

Create a dedicated `http.ServeMux`, allow only GET, set `Content-Type: application/json`, and encode `{ "status": "ok" }`. Pass it as `Dependencies.API` and mount it at `/api/v1/` without browser redirect middleware.

- [ ] **Step 4: Run API and web tests**

Run `go test ./internal/api ./internal/web -count=1`; expect PASS.

- [ ] **Step 5: Commit checkpoint**

When Git exists, commit `feat: establish versioned api boundary`.

## Task 9: Server Wiring And End-To-End Local Verification

**Files:**
- Modify: `cmd/server/main.go`
- Create: `README.md`
- Modify only on failure: files introduced by Tasks 1-8.

**Interfaces:**
- Consumes: validated config, pgx pool, auth repository/service, web handler, and API handler.
- Produces: a locally runnable application at `http://localhost:8080`.

- [ ] **Step 1: Write a failing composition test**

Extract `run(ctx context.Context, cfg config.Config) error` from `main.go` and add:

```go
func TestRunReturnsDatabaseConfigurationError(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "://invalid",
		SessionSecret: []byte("01234567890123456789012345678901"),
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected invalid database URL error")
	}
}
```

Run `go test ./cmd/server -count=1`; expect FAIL because `run` does not exist.

- [ ] **Step 2: Wire production dependencies**

`main` loads config, installs signal cancellation for `os.Interrupt` and `syscall.SIGTERM`, opens PostgreSQL, builds repository/service/limiter, constructs the web handler, creates an `http.Server` with `ReadHeaderTimeout=5s`, and performs graceful shutdown with a ten-second timeout.

- [ ] **Step 3: Document exact local commands**

`README.md` must include:

```powershell
go run ./cmd/migrate up
go run ./cmd/admin create
go run ./cmd/server
```

It must list required FlyEnv variables, state that `.flyenv` is secret, and identify `http://localhost:8080/login` and `http://localhost:8080/api/v1/health`.

- [ ] **Step 4: Generate the local session secret**

Generate a secret in PowerShell without printing it into project files:

```powershell
$konkitBytes = New-Object byte[] 48
[Security.Cryptography.RandomNumberGenerator]::Fill($konkitBytes)
[Convert]::ToBase64String($konkitBytes)
```

Place the resulting value only in FlyEnv's `SESSION_SECRET`. Do not paste it into source code, README, test output, or chat.

- [ ] **Step 5: Run the complete automated verification**

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'
go test ./... -count=1
Set-Location frontend
npm.cmd run build
Set-Location ..
go vet ./...
go build -o .\.tmp\konkit-server.exe .\cmd\server
```

Expected: every command exits with code 0.

- [ ] **Step 6: Run migration and start the local server**

```powershell
go run ./cmd/migrate up
Start-Process -FilePath '.\.tmp\konkit-server.exe' -WorkingDirectory 'D:\KSM\Deployment\konkit' -WindowStyle Hidden
```

Expected: the executable starts without a database or configuration error.

- [ ] **Step 7: Verify HTTP behavior**

```powershell
(Invoke-WebRequest -UseBasicParsing http://localhost:8080/login).StatusCode
(Invoke-WebRequest -UseBasicParsing http://localhost:8080/api/v1/health).Content
```

Expected: login returns `200`; API returns `{"status":"ok"}`. In a browser, invalid credentials show the generic error, valid credentials open `/dashboard`, direct anonymous dashboard access redirects to login, and logout returns to login.

- [ ] **Step 8: Commit checkpoint**

When Git exists, commit `feat: complete authentication foundation`.

## Self-Review

Spec coverage:

- Tasks 1-2 cover environment validation, PostgreSQL, and migrations.
- Tasks 3-5 cover Argon2id, users, sessions, RBAC, CSRF, and throttling.
- Task 4 creates the first Super Admin without a stored default password.
- Tasks 6-7 cover login, logout, protected dashboard, cookie behavior, and login UI states.
- Task 8 creates the JSON-only `/api/v1` boundary without premature token issuance.
- Task 9 covers graceful server wiring, documentation, and end-to-end local verification.
- SMTP reset delivery and operational dashboard widgets remain intentionally dependency-gated as approved in the hybrid spec.

Placeholder scan:

- Secret fields in `.env.example` are intentionally empty and fail application validation until configured.
- No implementation step contains an unfinished marker or an unspecified error-handling instruction.

Type consistency:

- `config.Config` field names match Tasks 1, 2, 6, and 9.
- `auth.Principal`, `auth.ClientMeta`, and the `AuthService` signatures match Tasks 3, 5, and 6.
- Cookie name is consistently `konkit_session`.
- Migration and repository identifiers consistently use `users`, `sessions`, `roles`, `permissions`, `user_roles`, and `role_permissions`.
