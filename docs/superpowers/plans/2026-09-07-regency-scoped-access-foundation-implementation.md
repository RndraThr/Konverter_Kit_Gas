# Regency-Scoped Access Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the reusable mechanism that restricts a role (and therefore its users) to specific kabupaten, wire it into the Role management screen, and apply it to the first two read endpoints (`ListRegencies`, `ListSchedules`) so Persiapan Program already respects it. This is Part 1 of 2 — DCP3, Pendistribusian, and Laporan enforcement follow in a separate plan once this foundation lands.

**Architecture:** Add `role_regencies` (many-to-many) and `roles.all_regencies_access` to the schema. Add a `RegencyScope` resolution to `internal/auth`, sitting alongside the existing `Can`/`Permissions` resolution. The API handler resolves `RegencyScope` once per request (like it already does for permissions) and passes it as a plain parameter into domain service methods — no domain package calls back into `auth`, matching how `auth.Principal`/`auth.ClientMeta` are already threaded through this codebase explicitly rather than via context values.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgx v5, goose, React 19, TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-07-regency-scoped-access-design.md`

## Global Constraints

- One role can be linked to more than one kabupaten (many-to-many via `role_regencies`).
- A role can be marked `all_regencies_access` instead of (or regardless of) an explicit kabupaten list — when true, the explicit list is ignored/cleared.
- Super Admin always resolves to `Unrestricted: true` via the existing `principal.IsSuperAdmin()` check; it never needs `role_regencies` rows.
- A role with neither `all_regencies_access` nor any `role_regencies` rows resolves to **zero access** (fail-safe default) — there is no migration/backfill concern because there is no production data yet.
- `RegencyScope` is resolved once at the API handler layer and passed as an explicit parameter into service methods — never resolved a second time inside a domain package, never carried via `context.Context`.
- This plan covers only `programs.ListRegencies` and `programs.ListSchedules`. `ListPrograms`, `ListPackageTemplates`, and `ListDocumentationTemplates` are intentionally **not** scoped — those entities are not tied to one kabupaten. DCP3, Pendistribusian, and Laporan enforcement is a separate follow-up plan.
- Follow existing code patterns exactly: the `sessionStore` interface / `Service` delegation pattern already used for `Can`/`Permissions` in `internal/auth`, the `replaceRolePermissions`/`permissionRefsForCodes` pattern already used for role-permission assignment in `internal/administration`, and the `h.authorize` helper pattern already used in `internal/api`.

---

### Task 1: Schema Migration

**Files:**
- Create: `internal/database/migrations/00006_role_regency_access.sql`

**Interfaces:**
- Produces `roles.all_regencies_access` column and `role_regencies` table consumed by Tasks 2 and 3.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
ALTER TABLE roles ADD COLUMN all_regencies_access boolean NOT NULL DEFAULT false;

CREATE TABLE role_regencies (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, regency_id)
);

-- +goose Down
DROP TABLE role_regencies;
ALTER TABLE roles DROP COLUMN all_regencies_access;
```

- [ ] **Step 2: Apply and verify**

Run:

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Expected: migration `00006_role_regency_access.sql` applied; status lists versions 1 through 6.

- [ ] **Step 3: Commit**

```powershell
git add internal/database/migrations/00006_role_regency_access.sql
git commit -m "feat: add role regency access schema"
```

---

### Task 2: Auth Domain — RegencyScope Resolution

**Files:**
- Modify: `internal/auth/models.go`
- Modify: `internal/auth/session.go`
- Modify: `internal/auth/session_test.go`
- Modify: `internal/auth/repository.go`
- Modify: `internal/auth/repository_integration_test.go`

**Interfaces:**
- Produces `auth.RegencyScope{Unrestricted bool, RegencyIDs []string}` with method `Allows(regencyID string) bool`.
- Produces `auth.Service.RegencyScope(ctx context.Context, principal Principal) (RegencyScope, error)`.
- Consumed by Task 4 (programs) and Task 5 (API layer).

- [ ] **Step 1: Write the failing service test**

`internal/auth/session_test.go` already has one concrete stub, `fakeSessionStore`, that directly implements every method of the `sessionStore` interface (it does not embed the interface — unlike the `fake*` stubs in `internal/api`). Adding a method to `sessionStore` means `fakeSessionStore` must implement it too, or the whole file fails to compile. Extend that existing stub — do not create a second one.

Add two fields to `fakeSessionStore`:

```go
type fakeSessionStore struct {
	user        User
	findErr     error
	principal   Principal
	createdHash []byte
	deletedHash []byte
	expiresAt   time.Time
	permissions []string
	regencyScope RegencyScope
	regencyScopeErr error
}
```

Add the method next to `PermissionsForPrincipal`:

```go
func (f *fakeSessionStore) RegencyScopeForPrincipal(_ context.Context, _ Principal) (RegencyScope, error) {
	return f.regencyScope, f.regencyScopeErr
}
```

Append the new tests after `TestPermissionsReturnsStorePermissions`:

```go
func TestRegencyScopeReturnsStoreScope(t *testing.T) {
	store := &fakeSessionStore{regencyScope: RegencyScope{RegencyIDs: []string{"regency-1"}}}
	scope, err := NewService(store, time.Hour, 24*time.Hour).RegencyScope(context.Background(), Principal{UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if scope.Unrestricted || len(scope.RegencyIDs) != 1 || scope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope=%+v", scope)
	}
}

func TestRegencyScopeAllows(t *testing.T) {
	unrestricted := RegencyScope{Unrestricted: true}
	if !unrestricted.Allows("any-regency") {
		t.Fatal("unrestricted scope must allow any regency")
	}
	scoped := RegencyScope{RegencyIDs: []string{"regency-1"}}
	if !scoped.Allows("regency-1") || scoped.Allows("regency-2") {
		t.Fatalf("scoped allow check wrong: %+v", scoped)
	}
	empty := RegencyScope{}
	if empty.Allows("regency-1") {
		t.Fatal("empty scope must allow nothing")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/auth -run 'TestRegencyScope' -count=1`

Expected: FAIL because `RegencyScope` type and `Service.RegencyScope` do not exist.

- [ ] **Step 3: Add the `RegencyScope` type**

Modify `internal/auth/models.go`, append:

```go
type RegencyScope struct {
	Unrestricted bool
	RegencyIDs   []string
}

func (s RegencyScope) Allows(regencyID string) bool {
	if s.Unrestricted {
		return true
	}
	for _, id := range s.RegencyIDs {
		if id == regencyID {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Extend the `sessionStore` interface and add the service method**

Modify `internal/auth/session.go`: add `RegencyScopeForPrincipal(ctx context.Context, principal Principal) (RegencyScope, error)` to the `sessionStore` interface, and append after the existing `Permissions` method:

```go
func (s *Service) RegencyScope(ctx context.Context, principal Principal) (RegencyScope, error) {
	return s.store.RegencyScopeForPrincipal(ctx, principal)
}
```

- [ ] **Step 5: Run the service test to verify it passes**

Run: `go test ./internal/auth -run 'TestRegencyScope' -count=1`

Expected: PASS.

- [ ] **Step 6: Implement the repository resolution**

Modify `internal/auth/repository.go`, append:

```go
func (r *Repository) RegencyScopeForPrincipal(ctx context.Context, principal Principal) (RegencyScope, error) {
	if principal.IsSuperAdmin() {
		return RegencyScope{Unrestricted: true}, nil
	}
	var unrestricted bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_roles
			JOIN roles ON roles.id = user_roles.role_id
			WHERE user_roles.user_id = $1 AND roles.all_regencies_access = true
		)
	`, principal.UserID).Scan(&unrestricted); err != nil {
		return RegencyScope{}, fmt.Errorf("check unrestricted regency access: %w", err)
	}
	if unrestricted {
		return RegencyScope{Unrestricted: true}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT role_regencies.regency_id::text
		FROM user_roles
		JOIN role_regencies ON role_regencies.role_id = user_roles.role_id
		WHERE user_roles.user_id = $1
	`, principal.UserID)
	if err != nil {
		return RegencyScope{}, fmt.Errorf("list principal regency access: %w", err)
	}
	defer rows.Close()
	regencyIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return RegencyScope{}, fmt.Errorf("scan principal regency: %w", err)
		}
		regencyIDs = append(regencyIDs, id)
	}
	if err := rows.Err(); err != nil {
		return RegencyScope{}, fmt.Errorf("iterate principal regencies: %w", err)
	}
	return RegencyScope{RegencyIDs: regencyIDs}, nil
}
```

- [ ] **Step 7: Add an integration test**

Append to `internal/auth/repository_integration_test.go`. The pool-setup helper in this file is named `integrationPool` (not `authIntegrationPool` — verify the exact name at the bottom of the file before writing this test, since other packages' integration tests name theirs differently). There is no `codeFromSuffix`-style helper in this package yet; derive a 3-letter uppercase document code inline instead of adding a cross-package dependency:

```go
func TestIntegrationRegencyScopeReflectsRoleAssignment(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	documentCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)

	var regencyID string
	if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan',$1,$2) RETURNING id::text`, "Wajo Scope "+suffix, documentCode).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	var roleID string
	if err := pool.QueryRow(ctx, `INSERT INTO roles(code,name) VALUES($1,'Scope Test Role') RETURNING id::text`, "scope_role_"+suffix).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_regencies(role_id,regency_id) VALUES($1,$2)`, roleID, regencyID); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(full_name,username,email,password_hash) VALUES('Scope Test','scope.test.`+suffix+`','scope.test.`+suffix+`@konkit.test','x') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, userID, roleID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id=$1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM role_regencies WHERE role_id=$1`, roleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM roles WHERE id=$1`, roleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
	})

	scope, err := repository.RegencyScopeForPrincipal(ctx, Principal{UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if scope.Unrestricted || len(scope.RegencyIDs) != 1 || scope.RegencyIDs[0] != regencyID {
		t.Fatalf("scope=%+v", scope)
	}

	superAdminScope, err := repository.RegencyScopeForPrincipal(ctx, Principal{UserID: "irrelevant", Roles: []string{"super_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if !superAdminScope.Unrestricted {
		t.Fatalf("super admin scope=%+v", superAdminScope)
	}

	if _, err := pool.Exec(ctx, `UPDATE roles SET all_regencies_access = true WHERE id = $1`, roleID); err != nil {
		t.Fatal(err)
	}
	allAccessScope, err := repository.RegencyScopeForPrincipal(ctx, Principal{UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if !allAccessScope.Unrestricted {
		t.Fatalf("all-access scope=%+v", allAccessScope)
	}
}
```

Add `"fmt"` and `"time"` to this file's imports if not already present (`"time"` is already imported; add `"fmt"`).

- [ ] **Step 8: Run all auth tests**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test ./internal/auth -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add internal/auth
git commit -m "feat: add regency scope resolution"
```

---

### Task 3: Administration Domain — Role Regency Assignment

**Files:**
- Modify: `internal/administration/models.go`
- Modify: `internal/administration/service.go`
- Modify: `internal/administration/service_test.go`
- Modify: `internal/administration/repository.go`
- Modify: `internal/administration/repository_integration_test.go`

**Interfaces:**
- Extends `Role` with `AllRegenciesAccess bool` and `Regencies []RegencyRef`.
- Extends `RoleInput` with `AllRegenciesAccess bool` and `RegencyIDs []string`.
- Produces `ErrRegencyNotFound`.
- `CreateRole`/`UpdateRole`/`ListRoles`/`GetRole` method signatures are unchanged — only the `Role`/`RoleInput` struct shapes grow.

- [ ] **Step 1: Write the failing service test**

Append to `internal/administration/service_test.go`:

```go
func TestNormalizeRoleInputClearsRegencyIDsWhenAllAccessGranted(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	_, err := service.CreateRole(context.Background(), auth.Principal{UserID: "admin-1"}, RoleInput{
		Code: "regional_role", Name: "Regional Role",
		AllRegenciesAccess: true, RegencyIDs: []string{"regency-1", "regency-2"},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.createdRole.RegencyIDs) != 0 {
		t.Fatalf("regency ids not cleared: %+v", repository.createdRole)
	}
}
```

`fakeRepository` (defined further down in this file) implements the `repository` interface directly, one method per line, and does not currently record its `CreateRole` input anywhere. Add a field and update the method:

```go
type fakeRepository struct {
	created      CreateUserInput
	passwordHash string
	updatedID    string
	createdRole  RoleInput
}
```

```go
func (f *fakeRepository) CreateRole(_ context.Context, _ auth.Principal, input RoleInput, _ auth.ClientMeta) (Role, error) {
	f.createdRole = input
	return Role{Code: input.Code, Name: input.Name}, nil
}
```

(This replaces the existing one-line `CreateRole` body — keep `UpdateRole`, `DeleteRole`, `ListRoles`, `GetRole`, `ListPermissions` unchanged.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/administration -run TestNormalizeRoleInputClearsRegencyIDsWhenAllAccessGranted -count=1`

Expected: FAIL because `RoleInput` has no `AllRegenciesAccess`/`RegencyIDs` fields yet.

- [ ] **Step 3: Extend the models**

Modify `internal/administration/models.go`: add to the `var (...)` error block:

```go
ErrRegencyNotFound = errors.New("one or more regencies do not exist")
```

Add a new type:

```go
type RegencyRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DocumentCode string `json:"document_code"`
}
```

Add fields to `Role`, right after `Permissions`:

```go
type Role struct {
	ID                 string       `json:"id"`
	Code               string       `json:"code"`
	Name               string       `json:"name"`
	Description        string       `json:"description,omitempty"`
	IsSystem           bool         `json:"is_system"`
	Permissions        []Permission `json:"permissions"`
	AllRegenciesAccess bool         `json:"all_regencies_access"`
	Regencies          []RegencyRef `json:"regencies"`
	UserCount          int64        `json:"user_count"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}
```

Add fields to `RoleInput`, right after `PermissionCodes`:

```go
type RoleInput struct {
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	PermissionCodes    []string `json:"permission_codes"`
	AllRegenciesAccess bool     `json:"all_regencies_access"`
	RegencyIDs         []string `json:"regency_ids"`
}
```

- [ ] **Step 4: Normalize regency IDs in the service**

Modify `internal/administration/service.go`, in `normalizeRoleInput`:

```go
func normalizeRoleInput(input *RoleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.PermissionCodes = uniqueNonEmpty(input.PermissionCodes)
	input.RegencyIDs = uniqueNonEmpty(input.RegencyIDs)
	if input.AllRegenciesAccess {
		input.RegencyIDs = []string{}
	}
	if length := utf8.RuneCountInString(input.Name); length < 2 || length > 100 {
		return ErrInvalidInput
	}
	if utf8.RuneCountInString(input.Description) > 500 {
		return ErrInvalidInput
	}
	return nil
}
```

- [ ] **Step 5: Run the service test to verify it passes**

Run: `go test ./internal/administration -run TestNormalizeRoleInputClearsRegencyIDsWhenAllAccessGranted -count=1`

Expected: PASS.

- [ ] **Step 6: Persist regency assignment in the repository**

Modify `internal/administration/repository.go`. Add helper functions near `replaceRolePermissions`/`permissionsForRole`:

```go
func regencyIDsExist(ctx context.Context, tx pgx.Tx, regencyIDs []string) error {
	if len(regencyIDs) == 0 {
		return nil
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM regencies WHERE id::text = ANY($1)`, regencyIDs).Scan(&count); err != nil {
		return fmt.Errorf("validate role regencies: %w", err)
	}
	if count != len(regencyIDs) {
		return ErrRegencyNotFound
	}
	return nil
}

func replaceRoleRegencies(ctx context.Context, tx pgx.Tx, roleID string, regencyIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_regencies WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("clear role regencies: %w", err)
	}
	for _, regencyID := range regencyIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO role_regencies (role_id, regency_id) VALUES ($1, $2)`, roleID, regencyID); err != nil {
			return fmt.Errorf("assign role regency: %w", err)
		}
	}
	return nil
}

func regenciesForRole(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, roleID string) ([]RegencyRef, error) {
	rows, err := db.Query(ctx, `
		SELECT regencies.id::text, regencies.name, regencies.document_code
		FROM regencies JOIN role_regencies ON role_regencies.regency_id = regencies.id
		WHERE role_regencies.role_id = $1 ORDER BY regencies.name
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role regencies: %w", err)
	}
	defer rows.Close()
	result := make([]RegencyRef, 0)
	for rows.Next() {
		var regency RegencyRef
		if err := rows.Scan(&regency.ID, &regency.Name, &regency.DocumentCode); err != nil {
			return nil, fmt.Errorf("scan role regency: %w", err)
		}
		result = append(result, regency)
	}
	return result, rows.Err()
}
```

Modify `CreateRole` — validate regencies before insert, insert `all_regencies_access`, and assign regencies after permissions:

```go
func (r *Repository) CreateRole(ctx context.Context, actor auth.Principal, input RoleInput, meta auth.ClientMeta) (Role, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Role{}, fmt.Errorf("begin create role: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	permissions, err := permissionRefsForCodes(ctx, tx, input.PermissionCodes)
	if err != nil {
		return Role{}, err
	}
	if err := regencyIDsExist(ctx, tx, input.RegencyIDs); err != nil {
		return Role{}, err
	}
	var roleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO roles (code, name, description, all_regencies_access) VALUES ($1, $2, NULLIF($3, ''), $4) RETURNING id::text
	`, input.Code, input.Name, input.Description, input.AllRegenciesAccess).Scan(&roleID)
	if uniqueViolation(err) {
		return Role{}, ErrRoleCodeInUse
	}
	if err != nil {
		return Role{}, fmt.Errorf("insert role: %w", err)
	}
	if err := replaceRolePermissions(ctx, tx, roleID, permissions); err != nil {
		return Role{}, err
	}
	if err := replaceRoleRegencies(ctx, tx, roleID, input.RegencyIDs); err != nil {
		return Role{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "role.created", ResourceType: "role", ResourceID: roleID, Metadata: map[string]any{"code": input.Code, "permissions": input.PermissionCodes, "all_regencies_access": input.AllRegenciesAccess, "regency_ids": input.RegencyIDs}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Role{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Role{}, fmt.Errorf("commit create role: %w", err)
	}
	return r.roleByID(ctx, roleID)
}
```

Modify `UpdateRole` similarly — add `all_regencies_access` to the `UPDATE roles SET` clause and call `replaceRoleRegencies`:

```go
func (r *Repository) UpdateRole(ctx context.Context, actor auth.Principal, roleID string, input RoleInput, meta auth.ClientMeta) (Role, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Role{}, fmt.Errorf("begin update role: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var code, previousName, previousDescription string
	var system bool
	if err := tx.QueryRow(ctx, `SELECT code, name, COALESCE(description, ''), is_system FROM roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&code, &previousName, &previousDescription, &system); errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	} else if err != nil {
		return Role{}, fmt.Errorf("lock role: %w", err)
	}
	if system {
		return Role{}, ErrSystemRole
	}
	permissions, err := permissionRefsForCodes(ctx, tx, input.PermissionCodes)
	if err != nil {
		return Role{}, err
	}
	if err := regencyIDsExist(ctx, tx, input.RegencyIDs); err != nil {
		return Role{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE roles SET name = $2, description = NULLIF($3, ''), all_regencies_access = $4, updated_at = now() WHERE id = $1`, roleID, input.Name, input.Description, input.AllRegenciesAccess); err != nil {
		return Role{}, fmt.Errorf("update role: %w", err)
	}
	if err := replaceRolePermissions(ctx, tx, roleID, permissions); err != nil {
		return Role{}, err
	}
	if err := replaceRoleRegencies(ctx, tx, roleID, input.RegencyIDs); err != nil {
		return Role{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "role.updated", ResourceType: "role", ResourceID: roleID, Metadata: map[string]any{"code": code, "previous_name": previousName, "previous_description": previousDescription, "name": input.Name, "description": input.Description, "permissions": input.PermissionCodes, "all_regencies_access": input.AllRegenciesAccess, "regency_ids": input.RegencyIDs}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Role{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Role{}, fmt.Errorf("commit update role: %w", err)
	}
	return r.roleByID(ctx, roleID)
}
```

Modify `ListRoles` and `roleByID` to select `all_regencies_access` and populate `Regencies`:

```go
func (r *Repository) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT roles.id::text, roles.code, roles.name, COALESCE(roles.description, ''), roles.is_system, roles.all_regencies_access,
		       roles.created_at, roles.updated_at, count(DISTINCT user_roles.user_id)
		FROM roles LEFT JOIN user_roles ON user_roles.role_id = roles.id
		GROUP BY roles.id ORDER BY roles.is_system DESC, roles.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	result := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.AllRegenciesAccess, &role.CreatedAt, &role.UpdatedAt, &role.UserCount); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		result = append(result, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}
	for index := range result {
		permissions, err := permissionsForRole(ctx, r.pool, result[index].ID)
		if err != nil {
			return nil, err
		}
		result[index].Permissions = permissions
		regencies, err := regenciesForRole(ctx, r.pool, result[index].ID)
		if err != nil {
			return nil, err
		}
		result[index].Regencies = regencies
	}
	return result, nil
}

func (r *Repository) roleByID(ctx context.Context, roleID string) (Role, error) {
	var role Role
	err := r.pool.QueryRow(ctx, `
		SELECT roles.id::text, roles.code, roles.name, COALESCE(roles.description, ''), roles.is_system, roles.all_regencies_access,
		       roles.created_at, roles.updated_at, count(DISTINCT user_roles.user_id)
		FROM roles LEFT JOIN user_roles ON user_roles.role_id = roles.id
		WHERE roles.id = $1 GROUP BY roles.id
	`, roleID).Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.AllRegenciesAccess, &role.CreatedAt, &role.UpdatedAt, &role.UserCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, fmt.Errorf("get role: %w", err)
	}
	role.Permissions, err = permissionsForRole(ctx, r.pool, roleID)
	if err != nil {
		return Role{}, err
	}
	role.Regencies, err = regenciesForRole(ctx, r.pool, roleID)
	return role, err
}
```

- [ ] **Step 7: Add an integration assertion**

In `internal/administration/repository_integration_test.go`, inside `TestIntegrationUserAndRoleAdministration` (or as a new test if you prefer isolation — match the file's existing single-large-integration-test style first), after the existing `CreateRole` call, insert a regency fixture and verify assignment:

```go
var regencyID string
if err := pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code) VALUES('Sulawesi Selatan','Wajo Role Test','WRT') RETURNING id::text`).Scan(&regencyID); err != nil {
	t.Fatal(err)
}
t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regencyID) })

scopedRole, err := service.CreateRole(ctx, actor, RoleInput{
	Code: roleCode + "_scoped", Name: "Scoped Role", PermissionCodes: []string{"dashboard.view"},
	RegencyIDs: []string{regencyID},
}, auth.ClientMeta{})
if err != nil {
	t.Fatal(err)
}
t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM roles WHERE code = $1", roleCode+"_scoped") })
if scopedRole.AllRegenciesAccess || len(scopedRole.Regencies) != 1 || scopedRole.Regencies[0].ID != regencyID {
	t.Fatalf("scoped role regencies=%+v", scopedRole.Regencies)
}

_, err = service.CreateRole(ctx, actor, RoleInput{
	Code: roleCode + "_bogus", Name: "Bogus Role", RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"},
}, auth.ClientMeta{})
if !errors.Is(err, ErrRegencyNotFound) {
	t.Fatalf("expected ErrRegencyNotFound, got %v", err)
}
```

- [ ] **Step 8: Run administration tests**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test ./internal/administration -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add internal/administration
git commit -m "feat: add role regency assignment"
```

---

### Task 4: Programs Domain — Scope Regencies and Schedules

**Files:**
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`
- Modify: `internal/programs/repository.go`
- Modify: `internal/programs/repository_integration_test.go`

**Interfaces:**
- Changes `ListRegencies(ctx) ([]Regency, error)` → `ListRegencies(ctx, auth.RegencyScope) ([]Regency, error)`.
- Changes `ListSchedules(ctx) ([]Schedule, error)` → `ListSchedules(ctx, auth.RegencyScope) ([]Schedule, error)`.
- Consumed by Task 5 (API layer).

- [ ] **Step 1: Write the failing service test**

Modify `internal/programs/service_test.go`: update the `repositoryStub` methods and add a new test:

```go
func (r *repositoryStub) ListRegencies(context.Context, auth.RegencyScope) ([]Regency, error) { return nil, nil }
```

```go
func (r *repositoryStub) ListSchedules(context.Context, auth.RegencyScope) ([]Schedule, error) { return nil, nil }
```

Append:

```go
type scopedRepositoryStub struct {
	repositoryStub
	seenScope auth.RegencyScope
}

func (r *scopedRepositoryStub) ListRegencies(_ context.Context, scope auth.RegencyScope) ([]Regency, error) {
	r.seenScope = scope
	return nil, nil
}
func (r *scopedRepositoryStub) ListSchedules(_ context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	r.seenScope = scope
	return nil, nil
}

func TestListRegenciesAndSchedulesForwardRegencyScope(t *testing.T) {
	repository := &scopedRepositoryStub{}
	service := NewService(repository)
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}

	if _, err := service.ListRegencies(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("ListRegencies scope=%+v", repository.seenScope)
	}

	repository.seenScope = auth.RegencyScope{}
	if _, err := service.ListSchedules(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if len(repository.seenScope.RegencyIDs) != 1 || repository.seenScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("ListSchedules scope=%+v", repository.seenScope)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/programs -run TestListRegenciesAndSchedulesForwardRegencyScope -count=1`

Expected: FAIL (compile error — `ListRegencies`/`ListSchedules` do not accept a scope argument yet).

- [ ] **Step 3: Update the service**

Modify `internal/programs/service.go`. In the `repository` interface:

```go
ListRegencies(context.Context, auth.RegencyScope) ([]Regency, error)
```

```go
ListSchedules(context.Context, auth.RegencyScope) ([]Schedule, error)
```

Update the two service methods:

```go
func (s *Service) ListRegencies(ctx context.Context, scope auth.RegencyScope) ([]Regency, error) {
	return s.repository.ListRegencies(ctx, scope)
}
```

```go
func (s *Service) ListSchedules(ctx context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	return s.repository.ListSchedules(ctx, scope)
}
```

(`"konkit/internal/auth"` is already imported in this file for `auth.Principal`/`auth.ClientMeta`.)

- [ ] **Step 4: Run the service test to verify it passes**

Run: `go test ./internal/programs -run TestListRegenciesAndSchedulesForwardRegencyScope -count=1`

Expected: PASS.

- [ ] **Step 5: Apply the filter in the repository**

Modify `internal/programs/repository.go`:

```go
func (r *Repository) ListRegencies(ctx context.Context, scope auth.RegencyScope) ([]Regency, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, province_name, name, document_code, is_active, COALESCE(notes, ''), created_at, updated_at
		FROM regencies
		WHERE ($1 OR id::text = ANY($2))
		ORDER BY province_name, name
	`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list regencies: %w", err)
	}
	defer rows.Close()
	items := []Regency{}
	for rows.Next() {
		var item Regency
		if err := rows.Scan(&item.ID, &item.ProvinceName, &item.Name, &item.DocumentCode, &item.IsActive, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan regency: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
```

```go
func (r *Repository) ListSchedules(ctx context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	rows, err := r.pool.Query(ctx, scheduleSelect+` WHERE ($1 OR s.regency_id::text = ANY($2)) ORDER BY s.start_date DESC,s.name`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	items := []Schedule{}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
```

Add `"konkit/internal/auth"` to this file's imports if not already present (it is — used for `auth.Principal`/`auth.ClientMeta` in `SaveRegency`/`SaveSchedule` etc.).

- [ ] **Step 6: Add an integration test**

Append to `internal/programs/repository_integration_test.go` a new test (reuse `programsIntegrationPool` and the same suffix/fixture pattern as the existing test):

```go
func TestIntegrationListRegenciesAndSchedulesRespectRegencyScope(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-scope-integration-test"}

	regencyA, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Scope A " + suffix, DocumentCode: fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20), IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	regencyB, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Scope B " + suffix, DocumentCode: fmt.Sprintf("%c%c%c", 'B'+suffix[len(suffix)-1]%18, 'B'+suffix[len(suffix)-2]%18, 'B'+suffix[len(suffix)-3]%18), IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id IN ($1,$2)", regencyA.ID, regencyB.ID)
	})

	scoped, err := service.ListRegencies(ctx, auth.RegencyScope{RegencyIDs: []string{regencyA.ID}})
	if err != nil {
		t.Fatal(err)
	}
	foundA, foundB := false, false
	for _, item := range scoped {
		if item.ID == regencyA.ID {
			foundA = true
		}
		if item.ID == regencyB.ID {
			foundB = true
		}
	}
	if !foundA || foundB {
		t.Fatalf("scoped regencies foundA=%v foundB=%v", foundA, foundB)
	}

	unrestricted, err := service.ListRegencies(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	foundA, foundB = false, false
	for _, item := range unrestricted {
		if item.ID == regencyA.ID {
			foundA = true
		}
		if item.ID == regencyB.ID {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("unrestricted regencies foundA=%v foundB=%v", foundA, foundB)
	}

	noAccess, err := service.ListRegencies(ctx, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range noAccess {
		if item.ID == regencyA.ID || item.ID == regencyB.ID {
			t.Fatalf("empty scope leaked regency: %+v", item)
		}
	}
}
```

Add `"time"` to this file's imports if not already present (it is, used elsewhere in the file).

- [ ] **Step 7: Run programs tests**

Run:

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
$line=Get-Content .env | Where-Object { $_ -like 'DATABASE_URL=*' } | Select-Object -First 1
$url=$line.Substring('DATABASE_URL='.Length).Trim('"')
$env:TEST_DATABASE_URL=$url -replace '/konkit\?', '/konkit_test?'
go test ./internal/programs -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/programs
git commit -m "feat: scope regency and schedule listings by role access"
```

---

### Task 5: API Layer — Wire Regency Scope Resolution

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `internal/api/program_routes.go`

**Interfaces:**
- Extends `AuthService` with `RegencyScope(context.Context, auth.Principal) (auth.RegencyScope, error)`.
- Extends `ProgramSetupService` with the new `ListRegencies`/`ListSchedules` signatures from Task 4.
- Produces `Handler.regencyScope(w, r, principal) (auth.RegencyScope, bool)` helper, mirroring `Handler.authorize`.

- [ ] **Step 1: Write the failing handler test**

Append to `internal/api/handler_test.go`:

```go
func TestRegenciesEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"programs.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/program-setup/regencies", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api -run TestRegenciesEndpointAppliesCallerRegencyScope -count=1`

Expected: FAIL (compile error — `fakeAuthService.regencyScope` field and `fakeProgramSetupService.seenRegencyScope` do not exist, and the interfaces don't require `RegencyScope` yet).

- [ ] **Step 3: Extend `AuthService` and add the handler helper**

Modify `internal/api/handler.go`. Add to the `AuthService` interface:

```go
RegencyScope(context.Context, auth.Principal) (auth.RegencyScope, error)
```

Change the `ProgramSetupService` interface's two methods:

```go
ListRegencies(context.Context, auth.RegencyScope) ([]programs.Regency, error)
```

```go
ListSchedules(context.Context, auth.RegencyScope) ([]programs.Schedule, error)
```

Add a new helper next to `authorize`/`authorizeAny`:

```go
func (h *Handler) regencyScope(w http.ResponseWriter, r *http.Request, principal auth.Principal) (auth.RegencyScope, bool) {
	scope, err := h.deps.Auth.RegencyScope(r.Context(), principal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Tidak dapat memeriksa akses kabupaten")
		return auth.RegencyScope{}, false
	}
	return scope, true
}
```

- [ ] **Step 4: Wire the two call sites**

Modify `internal/api/program_routes.go`, in `handleRegencies`'s GET branch:

```go
if r.Method == http.MethodGet && id == "" {
	if !h.authorize(w, r, rc.principal, "programs.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Programs.ListRegencies(r.Context(), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
	return
}
```

And in `handleSchedules`'s GET branch:

```go
if r.Method == http.MethodGet && id == "" {
	if !h.authorize(w, r, rc.principal, "programs.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Programs.ListSchedules(r.Context(), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
	return
}
```

- [ ] **Step 5: Update the test fakes**

Modify `internal/api/handler_test.go`:

Add fields to `fakeAuthService`:

```go
type fakeAuthService struct {
	principal          auth.Principal
	authenticateErr    error
	permissions        []string
	allowed            bool
	allowedPermissions map[string]bool
	regencyScope       auth.RegencyScope
	regencyScopeErr    error
}
```

Add the method:

```go
func (f *fakeAuthService) RegencyScope(context.Context, auth.Principal) (auth.RegencyScope, error) {
	return f.regencyScope, f.regencyScopeErr
}
```

Update `fakeProgramSetupService`:

```go
type fakeProgramSetupService struct {
	ProgramSetupService
	regencyInput     programs.RegencyInput
	seenRegencyScope auth.RegencyScope
}

func (f *fakeProgramSetupService) ListRegencies(_ context.Context, scope auth.RegencyScope) ([]programs.Regency, error) {
	f.seenRegencyScope = scope
	return []programs.Regency{}, nil
}
func (f *fakeProgramSetupService) ListSchedules(_ context.Context, scope auth.RegencyScope) ([]programs.Schedule, error) {
	f.seenRegencyScope = scope
	return []programs.Schedule{}, nil
}
```

(Keep the existing `SaveRegency`, `ListPrograms`, `ListPackageTemplates`, `ListDocumentationTemplates` methods on `fakeProgramSetupService` unchanged.)

- [ ] **Step 6: Run the API tests**

Run: `go test ./internal/api -count=1`

Expected: PASS. This also re-verifies `TestProgramSetupRoutesUseDocumentedPrefixes` and `TestProgramSetupMutationRequiresManagePermission` still pass — they call `/api/v1/program-setup/regencies` and `/api/v1/program-setup/schedules` through the same fake, which now ignores the (zero-value) scope and returns an empty list, so their `status == 200` assertions are unaffected.

- [ ] **Step 7: Run the full backend suite and commit**

Run: `go test ./... -count=1` (unit-only is fine here if `TEST_DATABASE_URL` is unset in this shell — full run happens in Task 7).

```powershell
git add internal/api
git commit -m "feat: apply regency scope to program setup listings"
```

---

### Task 6: Frontend — Role Regency Assignment

**Files:**
- Modify: `frontend/src/features/roles/RoleDialog.tsx`
- Modify: `frontend/src/features/roles/RolesPage.tsx`
- Modify: `frontend/src/features/roles/RolesPage.test.tsx` (already exists — has one test, `'marks system roles as protected'`; do not replace it, append alongside)

**Interfaces:**
- Consumes `all_regencies_access`/`regencies`/`regency_ids` added to `Role`/`RoleInput` in Task 3, and `GET /api/v1/program-setup/regencies` (already scoped by Task 5, and Super Admin — the only principal with `roles.manage` today — is always unrestricted).

`RolesPage.test.tsx` does not wrap `RolesPage` in a `PermissionsProvider` today — `useCan`'s underlying context (`frontend/src/lib/permissions.tsx`) defaults to `['*']` (unrestricted) when no provider is present, so the existing test already exercises the `canManage: true` path without one. Match that: do not introduce a `PermissionsProvider` wrapper in this file.

- [ ] **Step 1: Write the failing test**

Modify `frontend/src/features/roles/RolesPage.test.tsx` — add the `userEvent` import (not currently imported in this file) and append a new test after the existing one:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { RolesPage } from './RolesPage';
```

```tsx
test('shows assigned regencies and toggles the all-regencies checkbox', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    const url = path as string;
    if (url.includes('/permissions')) return { data: [] };
    if (url.includes('/program-setup/regencies')) return { data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }, { id: 'regency-2', name: 'Bone', document_code: 'BON' }] };
    return { data: [{ id: 'role-1', code: 'petugas_wajo', name: 'Petugas Wajo', is_system: false, permissions: [], all_regencies_access: false, regencies: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }], user_count: 0 }] };
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><RolesPage /></QueryClientProvider>);

  await userEvent.click(await screen.findByRole('button', { name: 'Edit Petugas Wajo' }));

  expect(screen.getByRole('checkbox', { name: 'Wajo' })).toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Bone' })).not.toBeChecked();

  await userEvent.click(screen.getByRole('checkbox', { name: 'Akses semua kabupaten' }));
  expect(screen.queryByRole('checkbox', { name: 'Wajo' })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm.cmd --prefix frontend test -- --run src/features/roles/RolesPage.test.tsx`

Expected: FAIL because there is no "Akses semua kabupaten" checkbox or per-regency checkboxes yet.

- [ ] **Step 3: Extend `RoleDialog`**

Modify `frontend/src/features/roles/RoleDialog.tsx`:

```tsx
export type RegencyOption = { id: string; name: string; document_code: string };
export type Permission = { id: string; code: string; name: string };
export type PermissionGroup = { resource: string; permissions: Permission[] };
export type RoleRecord = { id: string; code: string; name: string; description?: string; is_system: boolean; permissions: Permission[]; all_regencies_access: boolean; regencies: RegencyOption[]; user_count: number };
export type RoleValues = { code: string; name: string; description: string; permission_codes: string[]; all_regencies_access: boolean; regency_ids: string[] };

export function RoleDialog({ open, onOpenChange, groups, regencies, role, pending, error, fields = {}, onSave }: { open: boolean; onOpenChange: (value: boolean) => void; groups: PermissionGroup[]; regencies: RegencyOption[]; role?: RoleRecord; pending?: boolean; error?: string; fields?: Record<string, string>; onSave: (values: RoleValues) => void }) {
  const empty = { code: '', name: '', description: '', permission_codes: [] as string[], all_regencies_access: false, regency_ids: [] as string[] };
  const [values, setValues] = useState<RoleValues>(empty);
  useEffect(() => setValues(role ? { code: role.code, name: role.name, description: role.description ?? '', permission_codes: role.permissions.map((item) => item.code), all_regencies_access: role.all_regencies_access, regency_ids: role.regencies.map((item) => item.id) } : empty), [role, open]);
  const toggle = (code: string) => setValues({ ...values, permission_codes: values.permission_codes.includes(code) ? values.permission_codes.filter((item) => item !== code) : [...values.permission_codes, code] });
  const toggleRegency = (id: string) => setValues({ ...values, regency_ids: values.regency_ids.includes(id) ? values.regency_ids.filter((item) => item !== id) : [...values.regency_ids, id] });
  const title = role ? `Edit ${role.name}` : 'Tambah role';
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Backdrop className="dialogBackdrop" /><Dialog.Popup className="dialogPopup" aria-label={title}><form onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(values); }}>
    <header className="dialogHeader"><div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>Pilih hak akses sesuai tanggung jawab.</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
    <div className="dialogBody"><FormField error={fields.code} label="Kode role" name="code" required disabled={Boolean(role)} value={values.code} onChange={(e) => setValues({ ...values, code: e.target.value })} /><FormField error={fields.name} label="Nama role" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value })} /><FormField className="fullField" error={fields.description} label="Deskripsi" name="description" value={values.description} onChange={(e) => setValues({ ...values, description: e.target.value })} />
      {groups.map((group) => <fieldset className="fullField" style={{ border: 0, padding: 0, margin: 0 }} key={group.resource}><legend style={{ fontSize: 13, fontWeight: 750, marginBottom: 8 }}>{group.resource}</legend>{group.permissions.map((permission) => <label className="checkboxField" key={permission.code}><input type="checkbox" checked={values.permission_codes.includes(permission.code)} onChange={() => toggle(permission.code)} />{permission.name}</label>)}</fieldset>)}{fields.permission_codes && <small className="fieldError fullField">{fields.permission_codes}</small>}
      <fieldset className="fullField" style={{ border: 0, padding: 0, margin: 0 }}>
        <legend style={{ fontSize: 13, fontWeight: 750, marginBottom: 8 }}>Akses kabupaten</legend>
        <label className="checkboxField"><input type="checkbox" checked={values.all_regencies_access} onChange={(e) => setValues({ ...values, all_regencies_access: e.target.checked })} />Akses semua kabupaten</label>
        {!values.all_regencies_access && regencies.map((regency) => <label className="checkboxField" key={regency.id}><input type="checkbox" checked={values.regency_ids.includes(regency.id)} onChange={() => toggleRegency(regency.id)} />{regency.name}</label>)}
        {fields.regency_ids && <small className="fieldError fullField">{fields.regency_ids}</small>}
      </fieldset>
      {error && <p className="formNotice fullField">{error}</p>}
    </div><footer className="dialogActions"><Dialog.Close className="secondaryButton">Batal</Dialog.Close><button className="primaryButton" disabled={pending}>Simpan role</button></footer>
  </form></Dialog.Popup></Dialog.Portal></Dialog.Root>;
}
```

- [ ] **Step 4: Wire `RolesPage` to fetch regencies and pass them down**

Modify `frontend/src/features/roles/RolesPage.tsx`: add a regencies query and pass it to `RoleDialog`:

```tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { DataTable } from '../../components/DataTable';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { PermissionGroup, RegencyOption, RoleDialog, RoleRecord, RoleValues } from './RoleDialog';

export function RolesPage() {
  const client = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<RoleRecord>();
  const canManage = useCan('roles.manage');
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => apiRequest<{ data: RoleRecord[] }>('/api/v1/admin/roles') });
  const permissions = useQuery({ queryKey: ['permissions'], queryFn: () => apiRequest<{ data: PermissionGroup[] }>('/api/v1/admin/permissions'), enabled: canManage });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies'), enabled: canManage });
  const save = useMutation({ mutationFn: (values: RoleValues) => apiRequest(selected ? `/api/v1/admin/roles/${selected.id}` : '/api/v1/admin/roles', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['roles'] }); } });
  const remove = useMutation({ mutationFn: (id: string) => apiRequest(`/api/v1/admin/roles/${id}`, { method: 'DELETE' }), onSuccess: () => client.invalidateQueries({ queryKey: ['roles'] }) });
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  return <div className="page"><header className="pageHeader"><div><h1>Role & akses</h1><p>Susun hak akses berdasarkan tanggung jawab pengguna.</p></div>{canManage && <button className="primaryButton" onClick={() => { save.reset(); setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah role</button>}</header>
    {roles.isError ? <div className="errorState">Role belum dapat dimuat.</div> : <DataTable label="Daftar role"><thead><tr><th>Role</th><th>Hak akses</th><th>Pengguna</th><th>Jenis</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{roles.data?.data.map((role) => <tr key={role.id}><td><strong>{role.name}</strong><br /><small>{role.code}</small></td><td>{role.permissions.length} permission</td><td>{role.user_count}</td><td>{role.is_system ? 'Role sistem' : 'Role kustom'}</td>{canManage && <td style={{ display: 'flex', gap: 7, alignItems: 'center' }}><button className="iconButton" aria-label={`Edit ${role.name}`} disabled={role.is_system} onClick={() => { save.reset(); setSelected(role); setDialogOpen(true); }}><Pencil /></button><button className="iconButton" aria-label={`Hapus ${role.name}`} disabled={role.is_system || role.user_count > 0} onClick={() => window.confirm(`Hapus role ${role.name}?`) && remove.mutate(role.id)}><Trash2 /></button></td>}</tr>)}</tbody></DataTable>}
    {remove.isError && <p className="formNotice">{(remove.error as ApiError).message}</p>}
    {canManage && <RoleDialog error={saveMessage} fields={saveFields} open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} groups={permissions.data?.data ?? []} regencies={regencies.data?.data ?? []} role={selected} pending={save.isPending} onSave={(values) => save.mutate(values)} />}
  </div>;
}
```

- [ ] **Step 5: Run the frontend tests**

Run:

```powershell
npm.cmd --prefix frontend test -- --run
npm.cmd --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/features/roles web/static/app
git commit -m "feat: assign regency access to roles"
```

---

### Task 7: Full Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes the complete Tasks 1-6 vertical slice.

- [ ] **Step 1: Document the change**

Modify `README.md`: add one sentence after the existing administration paragraph noting that roles can now be restricted to specific kabupaten (or marked unrestricted), and that Persiapan Program's kabupaten/jadwal listings already respect it — DCP3, Pendistribusian, and Laporan enforcement follow in a separate change.

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
go run ./cmd/migrate status
```

Expected: all commands exit 0; migration status lists versions 1 through 6.

- [ ] **Step 3: Commit**

```powershell
git add README.md web/static/app
git commit -m "docs: document role regency access"
```
