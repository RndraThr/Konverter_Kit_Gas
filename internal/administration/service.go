package administration

import (
	"context"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"konkit/internal/auth"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
	roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,49}$`)
)

type repository interface {
	ListUsers(context.Context, UserFilter) (UserPage, error)
	GetUser(context.Context, string) (UserDetail, error)
	CreateUser(context.Context, auth.Principal, CreateUserInput, string, auth.ClientMeta) (UserDetail, error)
	UpdateUser(context.Context, auth.Principal, string, UpdateUserInput, auth.ClientMeta) (UserDetail, error)
	SetPassword(context.Context, auth.Principal, string, string, auth.ClientMeta) error
	ListRoles(context.Context) ([]Role, error)
	GetRole(context.Context, string) (Role, error)
	ListPermissions(context.Context) ([]Permission, error)
	CreateRole(context.Context, auth.Principal, RoleInput, auth.ClientMeta) (Role, error)
	UpdateRole(context.Context, auth.Principal, string, RoleInput, auth.ClientMeta) (Role, error)
	DeleteRole(context.Context, auth.Principal, string, auth.ClientMeta) error
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListUsers(ctx context.Context, filter UserFilter) (UserPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Search = strings.TrimSpace(filter.Search)
	filter.RoleCode = strings.TrimSpace(filter.RoleCode)
	return s.repository.ListUsers(ctx, filter)
}

func (s *Service) GetUser(ctx context.Context, userID string) (UserDetail, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return UserDetail{}, ErrNotFound
	}
	return s.repository.GetUser(ctx, userID)
}

func (s *Service) CreateUser(ctx context.Context, actor auth.Principal, input CreateUserInput, meta auth.ClientMeta) (UserDetail, error) {
	normalized, err := normalizeUserInput(input.FullName, input.Username, input.Email, input.RoleIDs)
	if err != nil {
		return UserDetail{}, err
	}
	if utf8.RuneCountInString(input.Password) < 12 {
		return UserDetail{}, ErrPasswordTooShort
	}
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return UserDetail{}, err
	}
	input.FullName = normalized.FullName
	input.Username = normalized.Username
	input.Email = normalized.Email
	input.RoleIDs = normalized.RoleIDs
	return s.repository.CreateUser(ctx, actor, input, passwordHash, meta)
}

func (s *Service) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input UpdateUserInput, meta auth.ClientMeta) (UserDetail, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return UserDetail{}, ErrNotFound
	}
	if userID == actor.UserID && !input.IsActive {
		return UserDetail{}, ErrSelfDeactivation
	}
	normalized, err := normalizeUserInput(input.FullName, input.Username, input.Email, input.RoleIDs)
	if err != nil {
		return UserDetail{}, err
	}
	input.FullName = normalized.FullName
	input.Username = normalized.Username
	input.Email = normalized.Email
	input.RoleIDs = normalized.RoleIDs
	return s.repository.UpdateUser(ctx, actor, userID, input, meta)
}

func (s *Service) SetPassword(ctx context.Context, actor auth.Principal, userID, password string, meta auth.ClientMeta) error {
	if utf8.RuneCountInString(password) < 12 {
		return ErrPasswordTooShort
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.repository.SetPassword(ctx, actor, strings.TrimSpace(userID), passwordHash, meta)
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repository.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, roleID string) (Role, error) {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return Role{}, ErrNotFound
	}
	return s.repository.GetRole(ctx, roleID)
}

func (s *Service) ListPermissions(ctx context.Context) ([]PermissionGroup, error) {
	permissions, err := s.repository.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]Permission)
	for _, permission := range permissions {
		resource, _, _ := strings.Cut(permission.Code, ".")
		groups[resource] = append(groups[resource], permission)
	}
	resources := make([]string, 0, len(groups))
	for resource := range groups {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	result := make([]PermissionGroup, 0, len(resources))
	for _, resource := range resources {
		result = append(result, PermissionGroup{Resource: resource, Permissions: groups[resource]})
	}
	return result, nil
}

func (s *Service) CreateRole(ctx context.Context, actor auth.Principal, input RoleInput, meta auth.ClientMeta) (Role, error) {
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	if !roleCodePattern.MatchString(input.Code) {
		return Role{}, ErrRoleCodeInvalid
	}
	if err := normalizeRoleInput(&input); err != nil {
		return Role{}, err
	}
	return s.repository.CreateRole(ctx, actor, input, meta)
}

func (s *Service) UpdateRole(ctx context.Context, actor auth.Principal, roleID string, input RoleInput, meta auth.ClientMeta) (Role, error) {
	if err := normalizeRoleInput(&input); err != nil {
		return Role{}, err
	}
	return s.repository.UpdateRole(ctx, actor, strings.TrimSpace(roleID), input, meta)
}

func (s *Service) DeleteRole(ctx context.Context, actor auth.Principal, roleID string, meta auth.ClientMeta) error {
	return s.repository.DeleteRole(ctx, actor, strings.TrimSpace(roleID), meta)
}

type normalizedUser struct {
	FullName string
	Username string
	Email    string
	RoleIDs  []string
}

func normalizeUserInput(fullName, username, email string, roleIDs []string) (normalizedUser, error) {
	result := normalizedUser{
		FullName: strings.TrimSpace(fullName),
		Username: strings.ToLower(strings.TrimSpace(username)),
		Email:    strings.ToLower(strings.TrimSpace(email)),
		RoleIDs:  uniqueNonEmpty(roleIDs),
	}
	if length := utf8.RuneCountInString(result.FullName); length < 2 || length > 120 {
		return normalizedUser{}, ErrInvalidInput
	}
	if !usernamePattern.MatchString(result.Username) {
		return normalizedUser{}, ErrInvalidInput
	}
	parsed, err := mail.ParseAddress(result.Email)
	if err != nil || parsed.Address != result.Email {
		return normalizedUser{}, ErrInvalidInput
	}
	if len(result.RoleIDs) == 0 {
		return normalizedUser{}, ErrRoleNotFound
	}
	return result, nil
}

func normalizeRoleInput(input *RoleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.PermissionCodes = uniqueNonEmpty(input.PermissionCodes)
	if length := utf8.RuneCountInString(input.Name); length < 2 || length > 100 {
		return ErrInvalidInput
	}
	if utf8.RuneCountInString(input.Description) > 500 {
		return ErrInvalidInput
	}
	return nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
