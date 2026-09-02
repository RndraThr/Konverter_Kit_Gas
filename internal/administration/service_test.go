package administration

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

func TestCreateUserNormalizesIdentityAndHashesPassword(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	actor := auth.Principal{UserID: "admin-1"}

	_, err := service.CreateUser(context.Background(), actor, CreateUserInput{
		FullName: "  Petugas Lapangan  ",
		Username: "  Field.User  ",
		Email:    "  FIELD@KONKIT.TEST  ",
		Password: "secure-password",
		IsActive: true,
		RoleIDs:  []string{"role-1"},
	}, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.created.FullName != "Petugas Lapangan" || repository.created.Username != "field.user" || repository.created.Email != "field@konkit.test" {
		t.Fatalf("input was not normalized: %+v", repository.created)
	}
	if repository.passwordHash == "" || repository.passwordHash == "secure-password" {
		t.Fatal("expected encoded password hash")
	}
	valid, err := auth.VerifyPassword("secure-password", repository.passwordHash)
	if err != nil || !valid {
		t.Fatalf("stored hash does not verify: valid=%v err=%v", valid, err)
	}
}

func TestUpdateUserRejectsSelfDeactivation(t *testing.T) {
	repository := &fakeRepository{}
	_, err := NewService(repository).UpdateUser(
		context.Background(),
		auth.Principal{UserID: "admin-1"},
		"admin-1",
		UpdateUserInput{FullName: "Admin", Username: "admin", Email: "admin@konkit.test", IsActive: false},
		auth.ClientMeta{},
	)
	if !errors.Is(err, ErrSelfDeactivation) {
		t.Fatalf("expected ErrSelfDeactivation, got %v", err)
	}
	if repository.updatedID != "" {
		t.Fatal("self-deactivation reached repository")
	}
}

func TestCreateRoleValidatesStableCode(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	_, err := service.CreateRole(context.Background(), auth.Principal{UserID: "admin-1"}, RoleInput{
		Code: "Invalid Role",
		Name: "Invalid",
	}, auth.ClientMeta{})
	if !errors.Is(err, ErrRoleCodeInvalid) {
		t.Fatalf("expected ErrRoleCodeInvalid, got %v", err)
	}
}

func TestSetPasswordRejectsShortPassword(t *testing.T) {
	repository := &fakeRepository{}
	err := NewService(repository).SetPassword(context.Background(), auth.Principal{UserID: "admin-1"}, "user-2", "short", auth.ClientMeta{})
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
	if repository.passwordHash != "" {
		t.Fatal("short password reached repository")
	}
}

type fakeRepository struct {
	created      CreateUserInput
	passwordHash string
	updatedID    string
}

func (f *fakeRepository) ListUsers(context.Context, UserFilter) (UserPage, error) {
	return UserPage{}, nil
}

func (f *fakeRepository) CreateUser(_ context.Context, _ auth.Principal, input CreateUserInput, passwordHash string, _ auth.ClientMeta) (UserDetail, error) {
	f.created = input
	f.passwordHash = passwordHash
	return UserDetail{FullName: input.FullName, Username: input.Username, Email: input.Email}, nil
}

func (f *fakeRepository) UpdateUser(_ context.Context, _ auth.Principal, userID string, input UpdateUserInput, _ auth.ClientMeta) (UserDetail, error) {
	f.updatedID = userID
	return UserDetail{ID: userID, FullName: input.FullName}, nil
}

func (f *fakeRepository) SetPassword(_ context.Context, _ auth.Principal, _ string, passwordHash string, _ auth.ClientMeta) error {
	f.passwordHash = passwordHash
	return nil
}

func (f *fakeRepository) ListRoles(context.Context) ([]Role, error)             { return nil, nil }
func (f *fakeRepository) ListPermissions(context.Context) ([]Permission, error) { return nil, nil }
func (f *fakeRepository) CreateRole(_ context.Context, _ auth.Principal, input RoleInput, _ auth.ClientMeta) (Role, error) {
	return Role{Code: input.Code, Name: input.Name}, nil
}
func (f *fakeRepository) UpdateRole(_ context.Context, _ auth.Principal, roleID string, input RoleInput, _ auth.ClientMeta) (Role, error) {
	return Role{ID: roleID, Name: input.Name}, nil
}
func (f *fakeRepository) DeleteRole(context.Context, auth.Principal, string, auth.ClientMeta) error {
	return nil
}
