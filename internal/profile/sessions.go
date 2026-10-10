package profile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// Sessions lets a user see the devices signed in to their account and sign
// out the other ones (mobile Akun → Session).

// ErrSessionNotFound wraps ErrNotFound so the API answers 404.
var ErrSessionNotFound = fmt.Errorf("session not found: %w", ErrNotFound)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Session is one signed-in device of the current user.
type Session struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	// Current is the session making this request.
	Current bool `json:"current"`
}

// ListSessions returns the user's active sessions: the current one first,
// then the most recently active.
func (r *Repository) ListSessions(ctx context.Context, userID string, currentHash []byte) ([]Session, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(user_agent, ''), COALESCE(host(ip_address), ''),
			created_at, last_seen_at, expires_at, token_hash = $2
		FROM sessions
		WHERE user_id = $1 AND expires_at > now()
		ORDER BY token_hash = $2 DESC, last_seen_at DESC
	`, userID, currentHash)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserAgent, &s.IPAddress, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.Current); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// RevokeSession signs out another device of the user. The current session
// cannot be revoked here (use logout).
func (r *Repository) RevokeSession(ctx context.Context, actor auth.Principal, sessionID string, currentHash []byte, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin revoke session: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userAgent string
	err = tx.QueryRow(ctx, `
		DELETE FROM sessions
		WHERE id = $1 AND user_id = $2 AND token_hash <> $3
		RETURNING COALESCE(user_agent, '')
	`, sessionID, actor.UserID, currentHash).Scan(&userAgent)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "profile.session_revoked",
		ResourceType: "user",
		ResourceID:   actor.UserID,
		Metadata:     map[string]any{"session_id": sessionID, "user_agent": userAgent},
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revoke session: %w", err)
	}
	return nil
}

type sessionStore interface {
	ListSessions(context.Context, string, []byte) ([]Session, error)
	RevokeSession(context.Context, auth.Principal, string, []byte, auth.ClientMeta) error
}

func (s *Service) sessions() (sessionStore, error) {
	store, ok := s.store.(sessionStore)
	if !ok {
		return nil, fmt.Errorf("profile sessions are unavailable")
	}
	return store, nil
}

// Sessions lists the active sessions of the signed-in user.
func (s *Service) Sessions(ctx context.Context, actor auth.Principal, currentRawToken string) ([]Session, error) {
	store, err := s.sessions()
	if err != nil {
		return nil, err
	}
	currentHash, err := auth.SessionTokenHash(currentRawToken)
	if err != nil {
		return nil, err
	}
	return store.ListSessions(ctx, actor.UserID, currentHash)
}

// RevokeSession signs out one of the user's other devices.
func (s *Service) RevokeSession(ctx context.Context, actor auth.Principal, currentRawToken, sessionID string, meta auth.ClientMeta) error {
	sessionID = strings.TrimSpace(sessionID)
	if !uuidPattern.MatchString(sessionID) {
		return ErrSessionNotFound
	}
	store, err := s.sessions()
	if err != nil {
		return err
	}
	currentHash, err := auth.SessionTokenHash(currentRawToken)
	if err != nil {
		return err
	}
	return store.RevokeSession(ctx, actor, sessionID, currentHash, meta)
}
