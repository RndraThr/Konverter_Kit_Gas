package admin

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"konkit/internal/auth"
)

func TestRunCreateRejectsMismatchedPasswords(t *testing.T) {
	input := strings.NewReader("admin\nadmin@konkit.local\n")
	err := RunCreate(context.Background(), input, io.Discard, sequencePasswords(
		"long-enough-password",
		"different-password",
	), &fakeCreator{})
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}

func TestRunCreateRejectsInvalidEmail(t *testing.T) {
	err := RunCreate(
		context.Background(),
		strings.NewReader("admin\nnot-an-email\n"),
		io.Discard,
		sequencePasswords("long-enough-password", "long-enough-password"),
		&fakeCreator{},
	)
	if !errors.Is(err, ErrEmailInvalid) {
		t.Fatalf("expected ErrEmailInvalid, got %v", err)
	}
}

func TestRunCreateRejectsInvalidUsername(t *testing.T) {
	err := RunCreate(
		context.Background(),
		strings.NewReader("a username with spaces\nadmin@konkit.local\n"),
		io.Discard,
		sequencePasswords("long-enough-password", "long-enough-password"),
		&fakeCreator{},
	)
	if !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("expected ErrUsernameInvalid, got %v", err)
	}
}

func TestRunCreateRejectsShortPassword(t *testing.T) {
	err := RunCreate(
		context.Background(),
		strings.NewReader("admin\nadmin@konkit.local\n"),
		io.Discard,
		sequencePasswords("short", "short"),
		&fakeCreator{},
	)
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestRunCreateNormalizesIdentityAndHashesPassword(t *testing.T) {
	creator := &fakeCreator{}
	var output strings.Builder
	err := RunCreate(
		context.Background(),
		strings.NewReader("  Admin.User  \n  Admin@Konkit.Local  \n"),
		&output,
		sequencePasswords("long-enough-password", "long-enough-password"),
		creator,
	)
	if err != nil {
		t.Fatal(err)
	}
	if creator.username != "admin.user" || creator.email != "admin@konkit.local" {
		t.Fatalf("identity was not normalized: %#v", creator)
	}
	if creator.passwordHash == "long-enough-password" || creator.passwordHash == "" {
		t.Fatal("repository must receive an encoded password hash")
	}
	ok, err := auth.VerifyPassword("long-enough-password", creator.passwordHash)
	if err != nil || !ok {
		t.Fatalf("stored hash does not verify: ok=%v err=%v", ok, err)
	}
	if output.String() != "Username: Email: Super Admin berhasil dibuat.\n" {
		t.Fatalf("unexpected output %q", output.String())
	}
}

type fakeCreator struct {
	username     string
	email        string
	passwordHash string
}

func (f *fakeCreator) CreateSuperAdmin(_ context.Context, username, email, passwordHash string) error {
	f.username = username
	f.email = email
	f.passwordHash = passwordHash
	return nil
}

func sequencePasswords(values ...string) PasswordReader {
	index := 0
	return func(string) (string, error) {
		if index >= len(values) {
			return "", io.EOF
		}
		value := values[index]
		index++
		return value, nil
	}
}
