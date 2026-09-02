package administration

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("administration resource not found")
	ErrIdentityInUse      = errors.New("username or email already in use")
	ErrInvalidInput       = errors.New("administration input is invalid")
	ErrPasswordTooShort   = errors.New("password must contain at least 12 characters")
	ErrRoleNotFound       = errors.New("one or more roles do not exist")
	ErrPermissionNotFound = errors.New("one or more permissions do not exist")
	ErrSelfDeactivation   = errors.New("users cannot deactivate their own account")
	ErrLastSuperAdmin     = errors.New("at least one active Super Admin is required")
	ErrRoleCodeInvalid    = errors.New("role code is invalid")
	ErrRoleInUse          = errors.New("role is assigned to users")
	ErrSystemRole         = errors.New("system role cannot be changed")
	ErrRoleCodeInUse      = errors.New("role code already exists")
)

type RoleRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type UserDetail struct {
	ID          string     `json:"id"`
	FullName    string     `json:"full_name"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	IsActive    bool       `json:"is_active"`
	Roles       []RoleRef  `json:"roles"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type UserFilter struct {
	Page     int
	PageSize int
	Search   string
	Active   *bool
	RoleCode string
}

type UserPage struct {
	Items    []UserDetail `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

type UserCounts struct {
	Total    int64 `json:"total"`
	Active   int64 `json:"active"`
	Inactive int64 `json:"inactive"`
}

type CreateUserInput struct {
	FullName string   `json:"full_name"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	IsActive bool     `json:"is_active"`
	RoleIDs  []string `json:"role_ids"`
}

type UpdateUserInput struct {
	FullName string   `json:"full_name"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	IsActive bool     `json:"is_active"`
	RoleIDs  []string `json:"role_ids"`
}

type Permission struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PermissionGroup struct {
	Resource    string       `json:"resource"`
	Permissions []Permission `json:"permissions"`
}

type Role struct {
	ID          string       `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	IsSystem    bool         `json:"is_system"`
	Permissions []Permission `json:"permissions"`
	UserCount   int64        `json:"user_count"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type RoleInput struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
}
