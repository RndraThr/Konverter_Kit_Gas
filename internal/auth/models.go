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
	FullName     string
	Username     string
	Email        string
	PasswordHash string
	IsActive     bool
}

type Principal struct {
	UserID   string
	FullName string
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

type RegencyScope struct {
	Unrestricted bool
	RegencyIDs   []string
}

func (s RegencyScope) Allows(regencyID string) bool {
	if s.Unrestricted {
		return true
	}
	for _, id := range s.RegencyIDs {
		if id == regencyID {
			return true
		}
	}
	return false
}
