package admin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"regexp"
	"strings"

	"konkit/internal/auth"
)

var (
	ErrUsernameInvalid  = errors.New("username must contain 3-64 lowercase letters, numbers, dots, underscores, or hyphens")
	ErrEmailInvalid     = errors.New("email is invalid")
	ErrPasswordTooShort = errors.New("password must contain at least 12 characters")
	ErrPasswordMismatch = errors.New("password confirmation does not match")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type PasswordReader func(prompt string) (string, error)

type Creator interface {
	CreateSuperAdmin(ctx context.Context, username, email, passwordHash string) error
}

func RunCreate(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	readPassword PasswordReader,
	creator Creator,
) error {
	scanner := bufio.NewScanner(in)
	if _, err := fmt.Fprint(out, "Username: "); err != nil {
		return err
	}
	username, err := scanValue(scanner)
	if err != nil {
		return fmt.Errorf("read username: %w", err)
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return ErrUsernameInvalid
	}

	if _, err := fmt.Fprint(out, "Email: "); err != nil {
		return err
	}
	email, err := scanValue(scanner)
	if err != nil {
		return fmt.Errorf("read email: %w", err)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		return ErrEmailInvalid
	}

	password, err := readPassword("Password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if len([]rune(password)) < 12 {
		return ErrPasswordTooShort
	}
	confirmation, err := readPassword("Ulangi password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	if password != confirmation {
		return ErrPasswordMismatch
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := creator.CreateSuperAdmin(ctx, username, email, passwordHash); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "Super Admin berhasil dibuat.")
	return err
}

func scanValue(scanner *bufio.Scanner) (string, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return scanner.Text(), nil
}
