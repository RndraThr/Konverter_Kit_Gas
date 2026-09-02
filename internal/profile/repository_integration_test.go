package profile

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationProfileUpdateAndPasswordChangeAreAudited(t *testing.T) {
	pool := profileIntegrationPool(t)
	ctx := context.Background()
	const email = "profile.integration@konkit.test"
	_, _ = pool.Exec(ctx, `
		DELETE FROM users
		WHERE lower(email) IN (lower($1), 'profile.updated@konkit.test')
		   OR lower(username) IN ('profile.integration', 'profile.updated')
	`, email)

	passwordHash, err := auth.HashPassword("current-password")
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (full_name, username, email, password_hash)
		VALUES ('Profile Integration', 'profile.integration', $1, $2)
		RETURNING id::text
	`, email, passwordHash).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	service := NewService(NewRepository(pool))
	actor := auth.Principal{UserID: userID, Username: "profile.integration"}
	updated, err := service.Update(ctx, actor, UpdateInput{
		FullName: "Profile Updated",
		Username: "Profile.Updated",
		Email:    "PROFILE.UPDATED@KONKIT.TEST",
	}, auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "profile-integration"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "Profile Updated" || updated.Username != "profile.updated" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}

	rawToken := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	currentTokenHash, err := auth.SessionTokenHash(rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + interval '1 hour'), ($1, $3, now() + interval '1 hour')
	`, userID, currentTokenHash, []byte("another-session-token-hash-00000")); err != nil {
		t.Fatal(err)
	}

	if err := service.ChangePassword(ctx, actor, rawToken, PasswordInput{
		CurrentPassword: "current-password",
		NewPassword:     "new-secure-password",
	}, auth.ClientMeta{IPAddress: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}

	var sessionCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE user_id = $1", userID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected only current session, got %d", sessionCount)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE actor_user_id = $1
		  AND action IN ('profile.updated', 'profile.password_changed')
		  AND NOT (metadata ? 'password' OR metadata ? 'password_hash')
	`, userID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("expected two safe profile audit events, got %d", auditCount)
	}
}

func profileIntegrationPool(t *testing.T) *pgxpool.Pool {
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
