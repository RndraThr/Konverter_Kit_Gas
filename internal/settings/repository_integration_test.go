package settings

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationSettingsUpdateIsAtomicAndAudited(t *testing.T) {
	pool := settingsIntegrationPool(t)
	ctx := context.Background()
	actor := settingsActor(t, pool)

	var previous []byte
	if err := pool.QueryRow(ctx, "SELECT value FROM system_settings WHERE key = 'application_name'").Scan(&previous); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "UPDATE system_settings SET value = $1::jsonb WHERE key = 'application_name'", previous)
	})

	result, err := NewService(NewRepository(pool)).Update(ctx, actor, map[string]string{
		"application_name": "Konkit Integration",
	}, auth.ClientMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, setting := range result {
		if setting.Key == "application_name" && setting.Value == "Konkit Integration" {
			found = true
		}
	}
	if !found {
		t.Fatalf("updated setting missing: %+v", result)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE actor_user_id = $1 AND action = 'settings.updated'
		  AND metadata->'current'->>'application_name' = 'Konkit Integration'
	`, actor.UserID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected settings audit event, got %d", auditCount)
	}
}

func settingsActor(t *testing.T, pool *pgxpool.Pool) auth.Principal {
	t.Helper()
	ctx := context.Background()
	const email = "settings.actor@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	actor := auth.Principal{FullName: "Settings Actor", Username: "settings.actor", Email: email, Roles: []string{"super_admin"}}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (full_name, username, email, password_hash)
		VALUES ($1, $2, $3, 'integration-hash') RETURNING id::text
	`, actor.FullName, actor.Username, actor.Email).Scan(&actor.UserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", actor.UserID) })
	return actor
}

func settingsIntegrationPool(t *testing.T) *pgxpool.Pool {
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
