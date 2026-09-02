package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"konkit/internal/administration"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/health"
	"konkit/internal/profile"
)

func TestHealthReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHealthRejectsPostWithJSONResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if rec.Header().Get("Location") != "" {
		t.Fatal("API errors must not redirect to HTML routes")
	}
}

func TestProtectedAPIReturnsJSONUnauthorizedWithoutSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: &fakeAuthService{authenticateErr: auth.ErrSessionNotFound}}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("expected JSON unauthorized response, got %s", rec.Body.String())
	}
}

func TestMeReturnsPrincipalPermissionsAndCSRFToken(t *testing.T) {
	service := &fakeAuthService{
		principal:   auth.Principal{UserID: "user-1", FullName: "Admin Konkit", Username: "admin", Email: "admin@konkit.test", Roles: []string{"super_admin"}},
		permissions: []string{"*"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			FullName    string   `json:"full_name"`
			Permissions []string `json:"permissions"`
		} `json:"data"`
		Meta struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.FullName != "Admin Konkit" || len(payload.Data.Permissions) != 1 || payload.Meta.CSRFToken == "" {
		t.Fatalf("unexpected me response: %+v", payload)
	}
}

func TestMeIncludesCurrentProfileState(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, permissions: []string{"dashboard.view"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	profiles := fakeProfileService{profile: profile.Profile{ID: "user-1", FullName: "Admin Program", Username: "admin", Email: "admin@konkit.test", Roles: []string{"operator"}, IsActive: true}}

	NewHandler(Dependencies{Auth: service, Profile: profiles, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"is_active":true`) || !strings.Contains(rec.Body.String(), `"roles":["operator"]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMutationRejectsMissingCSRFBeforeCallingService(t *testing.T) {
	service := &fakeAuthService{
		principal:   auth.Principal{UserID: "user-1", Roles: []string{"super_admin"}},
		permissions: []string{"*"},
		allowed:     true,
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me", strings.NewReader(`{"full_name":"Admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"csrf_invalid"`) {
		t.Fatalf("expected CSRF rejection, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdministrationRoutesUseDocumentedPrefixes(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	for _, path := range []string{"/api/v1/admin/users", "/api/v1/admin/roles", "/api/v1/admin/permissions", "/api/v1/system/settings", "/api/v1/system/audit-logs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		rec := httptest.NewRecorder()

		NewHandler(Dependencies{Auth: service}).ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("documented endpoint %s was not routed", path)
		}
	}
}

func TestProtectedReadinessReturnsServiceUnavailableWhenDegraded(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: service, Health: fakeHealthService{status: "degraded"}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestValidationErrorsUseBadRequestAndFieldDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, profile.ErrEmailInvalid)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"fields":{"email":`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthorizeAnyAcceptsUsersManagerForRoleOptions(t *testing.T) {
	service := &fakeAuthService{allowedPermissions: map[string]bool{"users.manage": true}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/role-options", nil)
	if !(&Handler{deps: Dependencies{Auth: service}}).authorizeAny(rec, req, auth.Principal{}, "roles.view", "users.manage") {
		t.Fatalf("authorization rejected: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoleOptionsReturnsRedactedRolesForUsersViewer(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"users.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/role-options", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	adminService := &fakeAdministrationService{roles: []administration.Role{{ID: "role-1", Code: "operator", Name: "Operator", Permissions: []administration.Permission{{Code: "secret.permission"}}, UserCount: 9}}}

	NewHandler(Dependencies{Auth: service, Administration: adminService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"operator"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret.permission") || strings.Contains(rec.Body.String(), "user_count") {
		t.Fatalf("role options leaked administrative details: %s", rec.Body.String())
	}
}

func TestDashboardSummaryUsesOnlyDashboardPermission(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dashboard.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	adminService := &fakeAdministrationService{roles: []administration.Role{{ID: "role-1"}}, counts: administration.UserCounts{Total: 5, Active: 4, Inactive: 1}}
	profiles := fakeProfileService{profile: profile.Profile{FullName: "Dashboard User", Email: "private@konkit.test"}}
	audits := &fakeAuditService{page: audit.Page{Items: []audit.Entry{{ID: "event-1", Action: "user.updated", ActorName: "Admin", IPAddress: "192.0.2.1", UserAgent: "private-agent", Metadata: map[string]any{"private": true}, CreatedAt: time.Now()}}}}

	NewHandler(Dependencies{Auth: service, Administration: adminService, Profile: profiles, Health: fakeHealthService{status: health.StatusHealthy}, Audit: audits}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active_users":4`) || !strings.Contains(rec.Body.String(), `"inactive_users":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, private := range []string{"private@konkit.test", "192.0.2.1", "private-agent", `"metadata"`} {
		if strings.Contains(rec.Body.String(), private) {
			t.Fatalf("dashboard summary leaked %q: %s", private, rec.Body.String())
		}
	}
}

func TestConflictErrorsIncludeIdentityFields(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, administration.ErrIdentityInUse)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"username":`) || !strings.Contains(rec.Body.String(), `"email":`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthorizeRejectsMissingPermission(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	if (&Handler{deps: Dependencies{Auth: &fakeAuthService{}}}).authorize(rec, req, auth.Principal{}, "users.view") {
		t.Fatal("authorization unexpectedly succeeded")
	}
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecodeJSONRequiresApplicationJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
	var target map[string]string
	if decodeJSON(rec, req, &target) || rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecodeJSONRejectsBodiesLargerThanOneMiB(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"data":"`+strings.Repeat("x", maxRequestBody)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	var target map[string]string
	if decodeJSON(rec, req, &target) || rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

const validSessionToken = "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio"

type fakeAuthService struct {
	principal          auth.Principal
	authenticateErr    error
	permissions        []string
	allowed            bool
	allowedPermissions map[string]bool
}

type fakeHealthService struct{ status string }
type fakeProfileService struct{ profile profile.Profile }
type fakeAdministrationService struct {
	AdministrationService
	roles  []administration.Role
	counts administration.UserCounts
}
type fakeAuditService struct {
	AuditService
	page audit.Page
}

func (f *fakeAuditService) List(context.Context, audit.Filter) (audit.Page, error) {
	return f.page, nil
}

func (f *fakeAdministrationService) ListRoles(context.Context) ([]administration.Role, error) {
	return f.roles, nil
}

func (f *fakeAdministrationService) UserCounts(context.Context) (administration.UserCounts, error) {
	return f.counts, nil
}

func (f fakeProfileService) Get(context.Context, string) (profile.Profile, error) {
	return f.profile, nil
}
func (f fakeProfileService) Update(context.Context, auth.Principal, profile.UpdateInput, auth.ClientMeta) (profile.Profile, error) {
	return f.profile, nil
}
func (f fakeProfileService) ChangePassword(context.Context, auth.Principal, string, profile.PasswordInput, auth.ClientMeta) error {
	return nil
}

func (f fakeHealthService) Check(context.Context) health.Report {
	return health.Report{Status: f.status}
}

func (f *fakeAuthService) Authenticate(context.Context, string) (auth.Principal, error) {
	return f.principal, f.authenticateErr
}

func (f *fakeAuthService) Can(_ context.Context, _ auth.Principal, permission string) (bool, error) {
	return f.allowed || f.allowedPermissions[permission], nil
}

func (f *fakeAuthService) Permissions(context.Context, auth.Principal) ([]string, error) {
	return f.permissions, nil
}
