package profile

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Get(ctx context.Context, userID string) (Profile, error) {
	var profile Profile
	err := r.pool.QueryRow(ctx, `
		SELECT
			users.id::text,
			users.full_name,
			users.username,
			users.email,
			users.is_active,
			users.last_login_at,
			users.created_at,
			users.updated_at,
			COALESCE(
				array_agg(roles.code ORDER BY roles.code) FILTER (WHERE roles.code IS NOT NULL),
				ARRAY[]::text[]
			)
		FROM users
		LEFT JOIN user_roles ON user_roles.user_id = users.id
		LEFT JOIN roles ON roles.id = user_roles.role_id
		WHERE users.id = $1
		GROUP BY users.id
	`, userID).Scan(
		&profile.ID,
		&profile.FullName,
		&profile.Username,
		&profile.Email,
		&profile.IsActive,
		&profile.LastLoginAt,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&profile.Roles,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return profile, nil
}

func (r *Repository) Update(
	ctx context.Context,
	actor auth.Principal,
	input UpdateInput,
	meta auth.ClientMeta,
) (Profile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Profile{}, fmt.Errorf("begin profile update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var previous UpdateInput
	if err := tx.QueryRow(ctx, `
		SELECT full_name, username, email
		FROM users
		WHERE id = $1
		FOR UPDATE
	`, actor.UserID).Scan(&previous.FullName, &previous.Username, &previous.Email); errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	} else if err != nil {
		return Profile{}, fmt.Errorf("lock profile: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE users
		SET full_name = $2, username = $3, email = $4,
		    updated_by = $1, updated_at = now()
		WHERE id = $1
	`, actor.UserID, input.FullName, input.Username, input.Email)
	if isUniqueViolation(err) {
		return Profile{}, ErrIdentityInUse
	}
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "profile.updated",
		ResourceType: "user",
		ResourceID:   actor.UserID,
		Metadata: map[string]any{
			"previous": map[string]any{"full_name": previous.FullName, "username": previous.Username, "email": previous.Email},
			"current":  map[string]any{"full_name": input.FullName, "username": input.Username, "email": input.Email},
		},
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
	}); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Profile{}, fmt.Errorf("commit profile update: %w", err)
	}
	return r.Get(ctx, actor.UserID)
}

func (r *Repository) PasswordHash(ctx context.Context, userID string) (string, error) {
	var passwordHash string
	err := r.pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1 AND is_active = true", userID).Scan(&passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get profile password: %w", err)
	}
	return passwordHash, nil
}

func (r *Repository) ChangePassword(
	ctx context.Context,
	actor auth.Principal,
	expectedPasswordHash string,
	passwordHash string,
	keepSessionHash []byte,
	meta auth.ClientMeta,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var storedPasswordHash string
	err = tx.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1 AND is_active = true FOR UPDATE", actor.UserID).Scan(&storedPasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock profile password: %w", err)
	}
	if storedPasswordHash != expectedPasswordHash {
		return ErrCurrentPassword
	}

	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_by = $1, updated_at = now()
		WHERE id = $1 AND is_active = true
	`, actor.UserID, passwordHash)
	if err != nil {
		return fmt.Errorf("change profile password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2", actor.UserID, keepSessionHash); err != nil {
		return fmt.Errorf("revoke other profile sessions: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "profile.password_changed",
		ResourceType: "user",
		ResourceID:   actor.UserID,
		Metadata:     map[string]any{},
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
