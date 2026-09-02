package profile

import (
	"errors"
	"time"
)

var (
	ErrNotFound          = errors.New("profile not found")
	ErrIdentityInUse     = errors.New("username or email already in use")
	ErrFullNameInvalid   = errors.New("full name is invalid")
	ErrUsernameInvalid   = errors.New("username is invalid")
	ErrEmailInvalid      = errors.New("email is invalid")
	ErrCurrentPassword   = errors.New("current password is incorrect")
	ErrPasswordTooShort  = errors.New("password must contain at least 12 characters")
	ErrPasswordUnchanged = errors.New("new password must differ from current password")
)

type Profile struct {
	ID          string     `json:"id"`
	FullName    string     `json:"full_name"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Roles       []string   `json:"roles"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type UpdateInput struct {
	FullName string `json:"full_name"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type PasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
