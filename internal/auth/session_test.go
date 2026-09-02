package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoginCreatesNormalSessionAndAuthenticatesToken(t *testing.T) {
	fixedNow := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	passwordHash, err := HashPassword("valid-password")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeSessionStore{
		user:      User{ID: "user-1", Username: "admin", Email: "admin@konkit.local", PasswordHash: passwordHash, IsActive: true},
		principal: Principal{UserID: "user-1", Username: "admin", Roles: []string{"super_admin"}},
	}
	service := NewService(store, 12*time.Hour, 30*24*time.Hour)
	service.now = func() time.Time { return fixedNow }
	service.random = bytes.NewReader(bytes.Repeat([]byte{0x2a}, sessionTokenBytes))

	token, err := service.Login(context.Background(), "admin", "valid-password", false, ClientMeta{IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || len(store.createdHash) != 32 {
		t.Fatal("expected opaque token and SHA-256 token hash")
	}
	if want := fixedNow.Add(12 * time.Hour); !store.expiresAt.Equal(want) {
		t.Fatalf("expires at %v, want %v", store.expiresAt, want)
	}

	principal, err := service.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Username != "admin" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestLoginRememberedSessionUsesRememberTTL(t *testing.T) {
	fixedNow := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	passwordHash, _ := HashPassword("valid-password")
	store := &fakeSessionStore{user: User{ID: "user-1", PasswordHash: passwordHash, IsActive: true}}
	service := NewService(store, 12*time.Hour, 30*24*time.Hour)
	service.now = func() time.Time { return fixedNow }
	service.random = bytes.NewReader(bytes.Repeat([]byte{0x2a}, sessionTokenBytes))

	if _, err := service.Login(context.Background(), "admin", "valid-password", true, ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if want := fixedNow.Add(30 * 24 * time.Hour); !store.expiresAt.Equal(want) {
		t.Fatalf("expires at %v, want %v", store.expiresAt, want)
	}
}

func TestLoginReturnsGenericErrorForWrongPasswordAndInactiveUser(t *testing.T) {
	passwordHash, _ := HashPassword("valid-password")
	tests := []struct {
		name     string
		password string
		active   bool
	}{
		{name: "wrong password", password: "wrong-password", active: true},
		{name: "inactive user", password: "valid-password", active: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeSessionStore{user: User{PasswordHash: passwordHash, IsActive: tt.active}}
			service := NewService(store, time.Hour, 24*time.Hour)
			_, err := service.Login(context.Background(), "admin", tt.password, false, ClientMeta{})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("expected generic credential error, got %v", err)
			}
		})
	}
}

func TestLogoutHashesAndDeletesSessionToken(t *testing.T) {
	store := &fakeSessionStore{}
	service := NewService(store, time.Hour, 24*time.Hour)
	service.random = bytes.NewReader(bytes.Repeat([]byte{0x4b}, sessionTokenBytes))
	token, err := service.newToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), token.raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(store.deletedHash, token.hash) {
		t.Fatal("logout did not delete the hashed session token")
	}
}

func TestSessionTokenHashRejectsMalformedToken(t *testing.T) {
	if _, err := SessionTokenHash("not-a-session-token"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestPermissionsReturnsStorePermissions(t *testing.T) {
	store := &fakeSessionStore{permissions: []string{"dashboard.view", "users.view"}}
	got, err := NewService(store, time.Hour, 24*time.Hour).Permissions(context.Background(), Principal{UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != "users.view" {
		t.Fatalf("unexpected permissions: %v", got)
	}
}

type fakeSessionStore struct {
	user        User
	findErr     error
	principal   Principal
	createdHash []byte
	deletedHash []byte
	expiresAt   time.Time
	permissions []string
}

func (f *fakeSessionStore) FindUserByIdentity(context.Context, string) (User, error) {
	return f.user, f.findErr
}

func (f *fakeSessionStore) CreateSession(_ context.Context, userID string, tokenHash []byte, expiresAt time.Time, _ ClientMeta) error {
	f.createdHash = append([]byte(nil), tokenHash...)
	f.expiresAt = expiresAt
	return nil
}

func (f *fakeSessionStore) PrincipalForSession(_ context.Context, tokenHash []byte, _ time.Time) (Principal, error) {
	if !bytes.Equal(tokenHash, f.createdHash) {
		return Principal{}, ErrSessionNotFound
	}
	return f.principal, nil
}

func (f *fakeSessionStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	f.deletedHash = append([]byte(nil), tokenHash...)
	return nil
}

func (f *fakeSessionStore) HasPermission(context.Context, Principal, string) (bool, error) {
	return true, nil
}

func (f *fakeSessionStore) PermissionsForPrincipal(context.Context, Principal) ([]string, error) {
	return f.permissions, nil
}
