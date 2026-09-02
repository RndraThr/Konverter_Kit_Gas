package profile

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

func TestUpdateNormalizesProfileAndPassesClientMetadata(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	actor := auth.Principal{UserID: "user-1", Username: "admin"}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "profile-test"}

	got, err := service.Update(context.Background(), actor, UpdateInput{
		FullName: "  Admin Konkit  ",
		Username: "  Admin.User  ",
		Email:    "  Admin@Konkit.Test  ",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != "Admin Konkit" || got.Username != "admin.user" || got.Email != "admin@konkit.test" {
		t.Fatalf("profile was not normalized: %+v", got)
	}
	if store.updatedBy != actor.UserID || store.meta != meta {
		t.Fatalf("actor metadata not forwarded: actor=%q meta=%+v", store.updatedBy, store.meta)
	}
}

func TestUpdateReturnsIdentityConflict(t *testing.T) {
	store := &fakeStore{updateErr: ErrIdentityInUse}
	_, err := NewService(store).Update(context.Background(), auth.Principal{UserID: "user-1"}, UpdateInput{
		FullName: "Admin Konkit",
		Username: "admin.user",
		Email:    "admin@konkit.test",
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrIdentityInUse) {
		t.Fatalf("expected ErrIdentityInUse, got %v", err)
	}
}

func TestChangePasswordVerifiesCurrentPasswordAndRevokesOtherSessions(t *testing.T) {
	currentHash, err := auth.HashPassword("current-password")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{passwordHash: currentHash}
	service := NewService(store)
	actor := auth.Principal{UserID: "user-1"}

	if err := service.ChangePassword(context.Background(), actor, validRawSessionToken(), PasswordInput{
		CurrentPassword: "current-password",
		NewPassword:     "new-secure-password",
	}, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if store.newPasswordHash == "" || store.newPasswordHash == "new-secure-password" {
		t.Fatal("expected encoded password hash")
	}
	valid, err := auth.VerifyPassword("new-secure-password", store.newPasswordHash)
	if err != nil || !valid {
		t.Fatalf("new hash does not verify: valid=%v err=%v", valid, err)
	}
	if len(store.keptSessionHash) != 32 {
		t.Fatalf("expected current session hash, got %d bytes", len(store.keptSessionHash))
	}
}

func TestChangePasswordRejectsInvalidInputs(t *testing.T) {
	currentHash, err := auth.HashPassword("current-password")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input PasswordInput
		want  error
	}{
		{name: "wrong current", input: PasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-secure-password"}, want: ErrCurrentPassword},
		{name: "too short", input: PasswordInput{CurrentPassword: "current-password", NewPassword: "short"}, want: ErrPasswordTooShort},
		{name: "unchanged", input: PasswordInput{CurrentPassword: "current-password", NewPassword: "current-password"}, want: ErrPasswordUnchanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{passwordHash: currentHash}
			err := NewService(store).ChangePassword(context.Background(), auth.Principal{UserID: "user-1"}, validRawSessionToken(), test.input, auth.ClientMeta{})
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if store.newPasswordHash != "" {
				t.Fatal("invalid password change reached persistence")
			}
		})
	}
}

func validRawSessionToken() string {
	return "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio"
}

type fakeStore struct {
	profile         Profile
	passwordHash    string
	updateErr       error
	updatedBy       string
	meta            auth.ClientMeta
	newPasswordHash string
	keptSessionHash []byte
}

func (f *fakeStore) Get(context.Context, string) (Profile, error) {
	return f.profile, nil
}

func (f *fakeStore) Update(_ context.Context, actor auth.Principal, input UpdateInput, meta auth.ClientMeta) (Profile, error) {
	f.updatedBy = actor.UserID
	f.meta = meta
	if f.updateErr != nil {
		return Profile{}, f.updateErr
	}
	f.profile = Profile{ID: actor.UserID, FullName: input.FullName, Username: input.Username, Email: input.Email}
	return f.profile, nil
}

func (f *fakeStore) PasswordHash(context.Context, string) (string, error) {
	return f.passwordHash, nil
}

func (f *fakeStore) ChangePassword(_ context.Context, _ auth.Principal, _ string, passwordHash string, keepSessionHash []byte, _ auth.ClientMeta) error {
	f.newPasswordHash = passwordHash
	f.keptSessionHash = append([]byte(nil), keepSessionHash...)
	return nil
}
