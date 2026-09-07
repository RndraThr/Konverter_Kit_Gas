package administration

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

func (r *Repository) ListUsers(ctx context.Context, filter UserFilter) (UserPage, error) {
	active := any(nil)
	if filter.Active != nil {
		active = *filter.Active
	}
	search := "%" + filter.Search + "%"
	const where = `
		WHERE ($1 = '%%' OR users.full_name ILIKE $1 OR users.username ILIKE $1 OR users.email ILIKE $1)
		  AND ($2::boolean IS NULL OR users.is_active = $2)
		  AND ($3 = '' OR EXISTS (
		      SELECT 1 FROM user_roles ur
		      JOIN roles role_filter ON role_filter.id = ur.role_id
		      WHERE ur.user_id = users.id AND role_filter.code = $3
		  ))
	`
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM users "+where, search, active, filter.RoleCode).Scan(&total); err != nil {
		return UserPage{}, fmt.Errorf("count users: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, full_name, username, email, is_active, last_login_at, created_at, updated_at
		FROM users
	`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5
	`, search, active, filter.RoleCode, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	items := make([]UserDetail, 0, filter.PageSize)
	for rows.Next() {
		var user UserDetail
		if err := rows.Scan(&user.ID, &user.FullName, &user.Username, &user.Email, &user.IsActive, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return UserPage{}, fmt.Errorf("scan user: %w", err)
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, fmt.Errorf("iterate users: %w", err)
	}
	for index := range items {
		roles, err := r.rolesForUser(ctx, r.pool, items[index].ID)
		if err != nil {
			return UserPage{}, err
		}
		items[index].Roles = roles
	}
	return UserPage{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *Repository) UserCounts(ctx context.Context) (UserCounts, error) {
	var counts UserCounts
	err := r.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE is_active), count(*) FILTER (WHERE NOT is_active)
		FROM users
	`).Scan(&counts.Total, &counts.Active, &counts.Inactive)
	if err != nil {
		return UserCounts{}, fmt.Errorf("count users by status: %w", err)
	}
	return counts, nil
}

func (r *Repository) GetUser(ctx context.Context, userID string) (UserDetail, error) {
	return r.userByID(ctx, userID)
}

func (r *Repository) CreateUser(ctx context.Context, actor auth.Principal, input CreateUserInput, passwordHash string, meta auth.ClientMeta) (UserDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return UserDetail{}, fmt.Errorf("begin create user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roles, err := roleRefsForIDs(ctx, tx, input.RoleIDs)
	if err != nil {
		return UserDetail{}, err
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (full_name, username, email, password_hash, is_active, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text
	`, input.FullName, input.Username, input.Email, passwordHash, input.IsActive, actor.UserID).Scan(&userID)
	if uniqueViolation(err) {
		return UserDetail{}, ErrIdentityInUse
	}
	if err != nil {
		return UserDetail{}, fmt.Errorf("insert user: %w", err)
	}
	if err := replaceUserRoles(ctx, tx, userID, input.RoleIDs); err != nil {
		return UserDetail{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "user.created",
		ResourceType: "user",
		ResourceID:   userID,
		Metadata:     map[string]any{"username": input.Username, "email": input.Email, "is_active": input.IsActive, "roles": roleCodes(roles)},
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	}); err != nil {
		return UserDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserDetail{}, fmt.Errorf("commit create user: %w", err)
	}
	return r.userByID(ctx, userID)
}

func (r *Repository) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input UpdateUserInput, meta auth.ClientMeta) (UserDetail, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return UserDetail{}, fmt.Errorf("begin update user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var previous UserDetail
	if err := tx.QueryRow(ctx, `
		SELECT id::text, full_name, username, email, is_active, last_login_at, created_at, updated_at
		FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&previous.ID, &previous.FullName, &previous.Username, &previous.Email, &previous.IsActive, &previous.LastLoginAt, &previous.CreatedAt, &previous.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, ErrNotFound
	} else if err != nil {
		return UserDetail{}, fmt.Errorf("lock user: %w", err)
	}
	previous.Roles, err = r.rolesForUser(ctx, tx, userID)
	if err != nil {
		return UserDetail{}, err
	}
	nextRoles, err := roleRefsForIDs(ctx, tx, input.RoleIDs)
	if err != nil {
		return UserDetail{}, err
	}
	if hasRole(previous.Roles, "super_admin") && (!input.IsActive || !hasRole(nextRoles, "super_admin")) {
		if err := ensureAnotherActiveSuperAdmin(ctx, tx, userID); err != nil {
			return UserDetail{}, err
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE users
		SET full_name = $2, username = $3, email = $4, is_active = $5,
		    updated_by = $6, updated_at = now()
		WHERE id = $1
	`, userID, input.FullName, input.Username, input.Email, input.IsActive, actor.UserID)
	if uniqueViolation(err) {
		return UserDetail{}, ErrIdentityInUse
	}
	if err != nil {
		return UserDetail{}, fmt.Errorf("update user: %w", err)
	}
	if err := replaceUserRoles(ctx, tx, userID, input.RoleIDs); err != nil {
		return UserDetail{}, err
	}
	if previous.IsActive && !input.IsActive {
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", userID); err != nil {
			return UserDetail{}, fmt.Errorf("revoke user sessions: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "user.updated",
		ResourceType: "user",
		ResourceID:   userID,
		Metadata: map[string]any{
			"previous": map[string]any{"full_name": previous.FullName, "username": previous.Username, "email": previous.Email, "is_active": previous.IsActive, "roles": roleCodes(previous.Roles)},
			"current":  map[string]any{"full_name": input.FullName, "username": input.Username, "email": input.Email, "is_active": input.IsActive, "roles": roleCodes(nextRoles)},
		},
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
	}); err != nil {
		return UserDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserDetail{}, fmt.Errorf("commit update user: %w", err)
	}
	return r.userByID(ctx, userID)
}

func (r *Repository) SetPassword(ctx context.Context, actor auth.Principal, userID, passwordHash string, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin administrative password change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, updated_by = $3, updated_at = now() WHERE id = $1`, userID, passwordHash, actor.UserID)
	if err != nil {
		return fmt.Errorf("set user password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", userID); err != nil {
		return fmt.Errorf("revoke password-reset sessions: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "user.password_changed", ResourceType: "user", ResourceID: userID, Metadata: map[string]any{}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit administrative password change: %w", err)
	}
	return nil
}

func (r *Repository) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, code, name, COALESCE(description, '') FROM permissions ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()
	result := make([]Permission, 0)
	for rows.Next() {
		var permission Permission
		if err := rows.Scan(&permission.ID, &permission.Code, &permission.Name, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		result = append(result, permission)
	}
	return result, rows.Err()
}

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

func (r *Repository) GetRole(ctx context.Context, roleID string) (Role, error) {
	return r.roleByID(ctx, roleID)
}

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

func (r *Repository) DeleteRole(ctx context.Context, actor auth.Principal, roleID string, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete role: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var code string
	var system bool
	if err := tx.QueryRow(ctx, `SELECT code, is_system FROM roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&code, &system); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock deleted role: %w", err)
	}
	if system {
		return ErrSystemRole
	}
	var userCount int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE role_id = $1`, roleID).Scan(&userCount); err != nil {
		return fmt.Errorf("count role users: %w", err)
	}
	if userCount > 0 {
		return ErrRoleInUse
	}
	if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "role.deleted", ResourceType: "role", ResourceID: roleID, Metadata: map[string]any{"code": code}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete role: %w", err)
	}
	return nil
}

func (r *Repository) userByID(ctx context.Context, userID string) (UserDetail, error) {
	var user UserDetail
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, full_name, username, email, is_active, last_login_at, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.FullName, &user.Username, &user.Email, &user.IsActive, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, ErrNotFound
	}
	if err != nil {
		return UserDetail{}, fmt.Errorf("get user: %w", err)
	}
	user.Roles, err = r.rolesForUser(ctx, r.pool, userID)
	return user, err
}

func (r *Repository) rolesForUser(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, userID string) ([]RoleRef, error) {
	rows, err := db.Query(ctx, `
		SELECT roles.id::text, roles.code, roles.name
		FROM roles JOIN user_roles ON user_roles.role_id = roles.id
		WHERE user_roles.user_id = $1 ORDER BY roles.code
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user roles: %w", err)
	}
	defer rows.Close()
	result := make([]RoleRef, 0)
	for rows.Next() {
		var role RoleRef
		if err := rows.Scan(&role.ID, &role.Code, &role.Name); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		result = append(result, role)
	}
	return result, rows.Err()
}

func roleRefsForIDs(ctx context.Context, tx pgx.Tx, roleIDs []string) ([]RoleRef, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, code, name FROM roles WHERE id::text = ANY($1) ORDER BY code FOR KEY SHARE`, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("validate user roles: %w", err)
	}
	defer rows.Close()
	result := make([]RoleRef, 0, len(roleIDs))
	for rows.Next() {
		var role RoleRef
		if err := rows.Scan(&role.ID, &role.Code, &role.Name); err != nil {
			return nil, fmt.Errorf("scan validated role: %w", err)
		}
		result = append(result, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) != len(roleIDs) {
		return nil, ErrRoleNotFound
	}
	return result, nil
}

func replaceUserRoles(ctx context.Context, tx pgx.Tx, userID string, roleIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clear user roles: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE id::text = ANY($2)
	`, userID, roleIDs); err != nil {
		return fmt.Errorf("assign user roles: %w", err)
	}
	return nil
}

func ensureAnotherActiveSuperAdmin(ctx context.Context, tx pgx.Tx, excludedUserID string) error {
	var roleID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE code = 'super_admin' FOR UPDATE`).Scan(&roleID); err != nil {
		return fmt.Errorf("lock Super Admin role: %w", err)
	}
	var others int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM users
		JOIN user_roles ON user_roles.user_id = users.id
		WHERE user_roles.role_id = $1 AND users.is_active = true AND users.id <> $2
	`, roleID, excludedUserID).Scan(&others); err != nil {
		return fmt.Errorf("count active Super Admin users: %w", err)
	}
	if others == 0 {
		return ErrLastSuperAdmin
	}
	return nil
}

func permissionRefsForCodes(ctx context.Context, tx pgx.Tx, codes []string) ([]Permission, error) {
	if len(codes) == 0 {
		return []Permission{}, nil
	}
	rows, err := tx.Query(ctx, `SELECT id::text, code, name, COALESCE(description, '') FROM permissions WHERE code = ANY($1) ORDER BY code`, codes)
	if err != nil {
		return nil, fmt.Errorf("validate permissions: %w", err)
	}
	defer rows.Close()
	result := make([]Permission, 0, len(codes))
	for rows.Next() {
		var permission Permission
		if err := rows.Scan(&permission.ID, &permission.Code, &permission.Name, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan validated permission: %w", err)
		}
		result = append(result, permission)
	}
	if len(result) != len(codes) {
		return nil, ErrPermissionNotFound
	}
	return result, rows.Err()
}

func replaceRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, permissions []Permission) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("clear role permissions: %w", err)
	}
	for _, permission := range permissions {
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`, roleID, permission.ID); err != nil {
			return fmt.Errorf("assign role permission: %w", err)
		}
	}
	return nil
}

func permissionsForRole(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, roleID string) ([]Permission, error) {
	rows, err := db.Query(ctx, `
		SELECT permissions.id::text, permissions.code, permissions.name, COALESCE(permissions.description, '')
		FROM permissions JOIN role_permissions ON role_permissions.permission_id = permissions.id
		WHERE role_permissions.role_id = $1 ORDER BY permissions.code
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer rows.Close()
	result := make([]Permission, 0)
	for rows.Next() {
		var permission Permission
		if err := rows.Scan(&permission.ID, &permission.Code, &permission.Name, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		result = append(result, permission)
	}
	return result, rows.Err()
}

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

func hasRole(roles []RoleRef, code string) bool {
	for _, role := range roles {
		if role.Code == code {
			return true
		}
	}
	return false
}

func roleCodes(roles []RoleRef) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.Code)
	}
	return result
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
