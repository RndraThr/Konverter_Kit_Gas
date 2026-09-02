package administration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationUserAndRoleAdministration(t *testing.T) {
	pool := administrationIntegrationPool(t)
	ctx := context.Background()
	const roleCode = "integration_operator"
	const email = "administration.integration@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	_, _ = pool.Exec(ctx, "DELETE FROM roles WHERE code = $1", roleCode)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lower(email) = lower($1)", email)
		_, _ = pool.Exec(context.Background(), "DELETE FROM roles WHERE code = $1", roleCode)
	})

	repository := NewRepository(pool)
	service := NewService(repository)
	actor := integrationActor(t, pool)
	role, err := service.CreateRole(ctx, actor, RoleInput{
		Code:            roleCode,
		Name:            "Integration Operator",
		PermissionCodes: []string{"dashboard.view"},
	}, auth.ClientMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateUser(ctx, actor, CreateUserInput{
		FullName: "Administration Integration",
		Username: "administration.integration",
		Email:    email,
		Password: "secure-password",
		IsActive: true,
		RoleIDs:  []string{role.ID},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.ListUsers(ctx, UserFilter{Search: "administration.integration", RoleCode: roleCode})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != user.ID {
		t.Fatalf("unexpected user page: %+v", page)
	}
	if err := service.DeleteRole(ctx, actor, role.ID, auth.ClientMeta{}); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("expected ErrRoleInUse, got %v", err)
	}
	var superRoleID string
	if err := pool.QueryRow(ctx, "SELECT id::text FROM roles WHERE code = 'super_admin'").Scan(&superRoleID); err != nil {
		t.Fatal(err)
	}
	_, err = repository.UpdateUser(ctx, auth.Principal{UserID: user.ID}, actor.UserID, UpdateUserInput{
		FullName: actor.FullName,
		Username: actor.Username,
		Email:    actor.Email,
		IsActive: false,
		RoleIDs:  []string{superRoleID},
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrLastSuperAdmin) {
		t.Fatalf("expected ErrLastSuperAdmin, got %v", err)
	}
	if err := repository.DeleteRole(ctx, actor, superRoleID, auth.ClientMeta{}); !errors.Is(err, ErrSystemRole) {
		t.Fatalf("expected ErrSystemRole, got %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + interval '1 hour')
	`, user.ID, []byte("administration-session-token-0001")); err != nil {
		t.Fatal(err)
	}
	_, err = service.UpdateUser(ctx, actor, user.ID, UpdateUserInput{
		FullName: user.FullName,
		Username: user.Username,
		Email:    user.Email,
		IsActive: false,
		RoleIDs:  []string{role.ID},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE user_id = $1", user.ID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("expected deactivation to revoke sessions, got %d", sessions)
	}

	if err := service.DeleteRole(ctx, actor, role.ID, auth.ClientMeta{}); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("inactive user assignment must still protect role, got %v", err)
	}
}

func integrationActor(t *testing.T, pool *pgxpool.Pool) auth.Principal {
	t.Helper()
	ctx := context.Background()
	const email = "administration.actor@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	actor := auth.Principal{
		FullName: "Administration Actor",
		Username: "administration.actor",
		Email:    email,
		Roles:    []string{"super_admin"},
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (full_name, username, email, password_hash)
		VALUES ($1, $2, $3, 'integration-hash')
		RETURNING id::text
	`, actor.FullName, actor.Username, actor.Email).Scan(&actor.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = 'super_admin'
	`, actor.UserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", actor.UserID) })
	return actor
}

func administrationIntegrationPool(t *testing.T) *pgxpool.Pool {
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
		t.Fatalf("integration tests require database konkit_test, got %q", config.ConnConfig.Database)
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
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
