package profile

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"konkit/internal/auth"
)

func TestIntegrationSessionsListAndRevoke(t *testing.T) {
	pool := profileIntegrationPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE username IN ('session.integration', 'session.other')")

	newUser := func(username string) string {
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (full_name, username, email, password_hash)
			VALUES ('Session Integration', $1, $1 || '@konkit.test', 'x')
			RETURNING id::text
		`, username).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id) })
		return id
	}
	userID := newUser("session.integration")
	otherUserID := newUser("session.other")

	rawToken := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	currentHash, err := auth.SessionTokenHash(rawToken)
	if err != nil {
		t.Fatal(err)
	}
	var phoneID, foreignID string
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, $2, now() + interval '1 hour', 'Konkit Web')
	`, userID, currentHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip_address, user_agent)
		VALUES ($1, 'phone-session-token-hash-0000000', now() + interval '1 hour', '10.0.0.5', 'Dart/3.13 (dart:io)')
		RETURNING id::text
	`, userID).Scan(&phoneID); err != nil {
		t.Fatal(err)
	}
	// Expired sessions are not listed.
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, 'expired-session-token-hash-00000', now() - interval '1 minute')
	`, userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, 'foreign-session-token-hash-00000', now() + interval '1 hour')
		RETURNING id::text
	`, otherUserID).Scan(&foreignID); err != nil {
		t.Fatal(err)
	}

	service := NewService(NewRepository(pool))
	actor := auth.Principal{UserID: userID}
	sessions, err := service.Sessions(ctx, actor, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || !sessions[0].Current || sessions[1].Current {
		t.Fatalf("expected current session first then the phone, got %+v", sessions)
	}
	if sessions[1].IPAddress != "10.0.0.5" || sessions[1].UserAgent != "Dart/3.13 (dart:io)" {
		t.Fatalf("unexpected phone session: %+v", sessions[1])
	}

	if err := service.RevokeSession(ctx, actor, rawToken, sessions[0].ID, auth.ClientMeta{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("current session must not be revoked here, got %v", err)
	}
	if err := service.RevokeSession(ctx, actor, rawToken, foreignID, auth.ClientMeta{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("another user's session must not be revoked, got %v", err)
	}
	if err := service.RevokeSession(ctx, actor, rawToken, "not-a-uuid", auth.ClientMeta{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid id must be not found, got %v", err)
	}
	if err := service.RevokeSession(ctx, actor, rawToken, phoneID, auth.ClientMeta{UserAgent: "session-test"}); err != nil {
		t.Fatal(err)
	}
	sessions, err = service.Sessions(ctx, actor, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("expected only the current session left, got %+v", sessions)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_user_id = $1 AND action = 'profile.session_revoked'`, userID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("expected one revoke audit event, got %d", audits)
	}
}
