package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *Repository) FindUserByIdentity(ctx context.Context, identity string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, full_name, username, email, password_hash, is_active
		FROM users
		WHERE lower(username) = lower($1) OR lower(email) = lower($1)
		LIMIT 1
	`, strings.TrimSpace(identity)).Scan(
		&user.ID,
		&user.FullName,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by identity: %w", err)
	}
	return user, nil
}

func (r *Repository) CreateSuperAdmin(ctx context.Context, username, email, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create Super Admin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id::text
	`, normalizeIdentity(username), normalizeIdentity(email), passwordHash).Scan(&userID)
	if isUniqueViolation(err) {
		return ErrUserExists
	}
	if err != nil {
		return fmt.Errorf("insert Super Admin: %w", err)
	}

	var roleID string
	if err := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE code = 'super_admin'").Scan(&roleID); err != nil {
		return fmt.Errorf("find super_admin role: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
	`, userID, roleID); err != nil {
		return fmt.Errorf("assign super_admin role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create Super Admin: %w", err)
	}
	return nil
}

func (r *Repository) PrincipalForUser(ctx context.Context, userID string) (Principal, error) {
	var principal Principal
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, full_name, username, email
		FROM users
		WHERE id = $1 AND is_active = true
	`, userID).Scan(&principal.UserID, &principal.FullName, &principal.Username, &principal.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUserNotFound
	}
	if err != nil {
		return Principal{}, fmt.Errorf("find principal: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT roles.code
		FROM roles
		JOIN user_roles ON user_roles.role_id = roles.id
		WHERE user_roles.user_id = $1
		ORDER BY roles.code
	`, userID)
	if err != nil {
		return Principal{}, fmt.Errorf("find principal roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return Principal{}, fmt.Errorf("scan principal role: %w", err)
		}
		principal.Roles = append(principal.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return Principal{}, fmt.Errorf("iterate principal roles: %w", err)
	}
	return principal, nil
}

func (r *Repository) HasPermission(ctx context.Context, principal Principal, permission string) (bool, error) {
	if principal.IsSuperAdmin() {
		return true, nil
	}

	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_roles
			JOIN role_permissions ON role_permissions.role_id = user_roles.role_id
			JOIN permissions ON permissions.id = role_permissions.permission_id
			WHERE user_roles.user_id = $1 AND permissions.code = $2
		)
	`, principal.UserID, permission).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return allowed, nil
}

func (r *Repository) CreateSession(
	ctx context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
	meta ClientMeta,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create session: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, NULLIF($4, '')::inet, NULLIF($5, ''))
	`, userID, tokenHash, expiresAt, meta.IPAddress, meta.UserAgent); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = now(), updated_at = now() WHERE id = $1", userID); err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create session: %w", err)
	}
	return nil
}

func (r *Repository) PrincipalForSession(ctx context.Context, tokenHash []byte, now time.Time) (Principal, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `
		SELECT users.id::text
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_hash = $1
		  AND sessions.expires_at > $2
		  AND users.is_active = true
	`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrSessionNotFound
	}
	if err != nil {
		return Principal{}, fmt.Errorf("find session principal: %w", err)
	}
	return r.PrincipalForUser(ctx, userID)
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := r.pool.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func normalizeIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
