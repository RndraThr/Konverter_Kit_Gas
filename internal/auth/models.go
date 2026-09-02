package auth

import (
	"errors"
	"slices"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserExists         = errors.New("username or email already exists")
)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	IsActive     bool
}

type Principal struct {
	UserID   string
	Username string
	Email    string
	Roles    []string
}

func (p Principal) IsSuperAdmin() bool {
	return slices.Contains(p.Roles, "super_admin")
}

type ClientMeta struct {
	IPAddress string
	UserAgent string
}
