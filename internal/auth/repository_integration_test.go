package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationRepositoryCreatesAndFindsSuperAdmin(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()

	const email = "repository-admin@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lower(email) = lower($1)", email)
	})

	err := repository.CreateSuperAdmin(ctx, "Repository.Admin", email, "encoded-password-hash")
	if err != nil {
		t.Fatal(err)
	}
	user, err := repository.FindUserByIdentity(ctx, "REPOSITORY.ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if user.FullName != "repository.admin" || user.Username != "repository.admin" || user.Email != email || !user.IsActive {
		t.Fatalf("unexpected user: %+v", user)
	}

	principal, err := repository.PrincipalForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !principal.IsSuperAdmin() {
		t.Fatalf("expected super_admin role, got %v", principal.Roles)
	}
	if principal.FullName != "repository.admin" {
		t.Fatalf("expected principal full name, got %q", principal.FullName)
	}

	allowed, err := repository.HasPermission(ctx, principal, "dashboard.view")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("expected Super Admin to access dashboard.view")
	}
}

func TestIntegrationRepositoryRejectsDuplicateIdentity(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()

	const email = "repository-duplicate@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lower(email) = lower($1)", email)
	})

	if err := repository.CreateSuperAdmin(ctx, "duplicate.admin", email, "encoded-password-hash"); err != nil {
		t.Fatal(err)
	}
	err := repository.CreateSuperAdmin(ctx, "different.username", "REPOSITORY-DUPLICATE@KONKIT.TEST", "another-hash")
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
}

func TestIntegrationRepositoryPersistsExpiresAndDeletesSession(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()

	const email = "repository-session@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lower(email) = lower($1)", email)
	})
	if err := repository.CreateSuperAdmin(ctx, "session.admin", email, "encoded-password-hash"); err != nil {
		t.Fatal(err)
	}
	user, err := repository.FindUserByIdentity(ctx, email)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	tokenHash := []byte("01234567890123456789012345678901")
	if err := repository.CreateSession(ctx, user.ID, tokenHash, now.Add(time.Hour), ClientMeta{IPAddress: "127.0.0.1", UserAgent: "integration-test"}); err != nil {
		t.Fatal(err)
	}
	principal, err := repository.PrincipalForSession(ctx, tokenHash, now)
	if err != nil || principal.UserID != user.ID {
		t.Fatalf("unexpected principal: %+v err=%v", principal, err)
	}
	if _, err := repository.PrincipalForSession(ctx, tokenHash, now.Add(2*time.Hour)); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected expired session rejection, got %v", err)
	}
	if err := repository.DeleteSession(ctx, tokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PrincipalForSession(ctx, tokenHash, now); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected deleted session rejection, got %v", err)
	}
}

func TestIntegrationAdministrationMigrationCreatesFoundation(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	var userColumnCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'users'
		  AND column_name = ANY($1)
	`, []string{"full_name", "updated_by"}).Scan(&userColumnCount); err != nil {
		t.Fatal(err)
	}
	if userColumnCount != 2 {
		t.Fatalf("expected administration user columns, got %d", userColumnCount)
	}

	for _, table := range []string{"system_settings", "audit_logs"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('public.' || $1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("expected table %s", table)
		}
	}

	permissionCodes := []string{
		"dashboard.view",
		"users.view",
		"users.manage",
		"roles.view",
		"roles.manage",
		"settings.view",
		"settings.manage",
		"health.view",
		"audit.view",
	}
	var permissionCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM permissions WHERE code = ANY($1)", permissionCodes).Scan(&permissionCount); err != nil {
		t.Fatal(err)
	}
	if permissionCount != len(permissionCodes) {
		t.Fatalf("expected %d administration permissions, got %d", len(permissionCodes), permissionCount)
	}

	settingKeys := []string{"application_name", "timezone", "date_format", "locale", "organization_name"}
	var settingCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM system_settings WHERE key = ANY($1)", settingKeys).Scan(&settingCount); err != nil {
		t.Fatal(err)
	}
	if settingCount != len(settingKeys) {
		t.Fatalf("expected %d default settings, got %d", len(settingKeys), settingCount)
	}
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if poolConfig.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require database konkit_test, got %q", poolConfig.ConnConfig.Database)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
