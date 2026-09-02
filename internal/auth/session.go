package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"
)

const sessionTokenBytes = 32

type sessionStore interface {
	FindUserByIdentity(ctx context.Context, identity string) (User, error)
	CreateSession(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time, meta ClientMeta) error
	PrincipalForSession(ctx context.Context, tokenHash []byte, now time.Time) (Principal, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	HasPermission(ctx context.Context, principal Principal, permission string) (bool, error)
}

type Service struct {
	store       sessionStore
	sessionTTL  time.Duration
	rememberTTL time.Duration
	now         func() time.Time
	random      io.Reader
}

type sessionToken struct {
	raw  string
	hash []byte
}

func NewService(store sessionStore, sessionTTL, rememberTTL time.Duration) *Service {
	return &Service{
		store:       store,
		sessionTTL:  sessionTTL,
		rememberTTL: rememberTTL,
		now:         time.Now,
		random:      rand.Reader,
	}
}

func (s *Service) Login(ctx context.Context, identity, password string, remember bool, meta ClientMeta) (string, error) {
	user, err := s.store.FindUserByIdentity(ctx, identity)
	if errors.Is(err, ErrUserNotFound) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	if !user.IsActive {
		return "", ErrInvalidCredentials
	}

	valid, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return "", fmt.Errorf("verify stored password: %w", err)
	}
	if !valid {
		return "", ErrInvalidCredentials
	}

	token, err := s.newToken()
	if err != nil {
		return "", err
	}
	ttl := s.sessionTTL
	if remember {
		ttl = s.rememberTTL
	}
	if err := s.store.CreateSession(ctx, user.ID, token.hash, s.now().Add(ttl), meta); err != nil {
		return "", err
	}
	return token.raw, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	hash, err := hashSessionToken(rawToken)
	if err != nil {
		return Principal{}, ErrSessionNotFound
	}
	return s.store.PrincipalForSession(ctx, hash, s.now())
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	hash, err := hashSessionToken(rawToken)
	if err != nil {
		return nil
	}
	return s.store.DeleteSession(ctx, hash)
}

func (s *Service) Can(ctx context.Context, principal Principal, permission string) (bool, error) {
	return s.store.HasPermission(ctx, principal, permission)
}

func (s *Service) newToken() (sessionToken, error) {
	rawBytes := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(s.random, rawBytes); err != nil {
		return sessionToken{}, fmt.Errorf("generate session token: %w", err)
	}
	hash := sha256.Sum256(rawBytes)
	return sessionToken{
		raw:  base64.RawURLEncoding.EncodeToString(rawBytes),
		hash: hash[:],
	}, nil
}

func hashSessionToken(rawToken string) ([]byte, error) {
	rawBytes, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || len(rawBytes) != sessionTokenBytes {
		return nil, ErrSessionNotFound
	}
	hash := sha256.Sum256(rawBytes)
	return hash[:], nil
}

func SessionTokenHash(rawToken string) ([]byte, error) {
	return hashSessionToken(rawToken)
}
