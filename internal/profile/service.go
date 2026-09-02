package profile

import (
	"context"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"konkit/internal/auth"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type store interface {
	Get(context.Context, string) (Profile, error)
	Update(context.Context, auth.Principal, UpdateInput, auth.ClientMeta) (Profile, error)
	PasswordHash(context.Context, string) (string, error)
	ChangePassword(context.Context, auth.Principal, string, []byte, auth.ClientMeta) error
}

type Service struct {
	store store
}

func NewService(store store) *Service {
	return &Service{store: store}
}

func (s *Service) Get(ctx context.Context, userID string) (Profile, error) {
	return s.store.Get(ctx, strings.TrimSpace(userID))
}

func (s *Service) Update(ctx context.Context, actor auth.Principal, input UpdateInput, meta auth.ClientMeta) (Profile, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	if length := utf8.RuneCountInString(input.FullName); length < 2 || length > 120 {
		return Profile{}, ErrFullNameInvalid
	}
	if !usernamePattern.MatchString(input.Username) {
		return Profile{}, ErrUsernameInvalid
	}
	parsed, err := mail.ParseAddress(input.Email)
	if err != nil || parsed.Address != input.Email {
		return Profile{}, ErrEmailInvalid
	}
	return s.store.Update(ctx, actor, input, meta)
}

func (s *Service) ChangePassword(
	ctx context.Context,
	actor auth.Principal,
	currentRawToken string,
	input PasswordInput,
	meta auth.ClientMeta,
) error {
	if utf8.RuneCountInString(input.NewPassword) < 12 {
		return ErrPasswordTooShort
	}

	currentHash, err := s.store.PasswordHash(ctx, actor.UserID)
	if err != nil {
		return err
	}
	valid, err := auth.VerifyPassword(input.CurrentPassword, currentHash)
	if err != nil {
		return err
	}
	if !valid {
		return ErrCurrentPassword
	}
	unchanged, err := auth.VerifyPassword(input.NewPassword, currentHash)
	if err != nil {
		return err
	}
	if unchanged {
		return ErrPasswordUnchanged
	}

	newHash, err := auth.HashPassword(input.NewPassword)
	if err != nil {
		return err
	}
	keepSessionHash, err := auth.SessionTokenHash(currentRawToken)
	if err != nil {
		return err
	}
	return s.store.ChangePassword(ctx, actor, newHash, keepSessionHash, meta)
}
