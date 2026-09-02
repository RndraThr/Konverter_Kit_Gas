package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
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

const validSessionToken = "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio"

type fakeAuthService struct {
	principal       auth.Principal
	authenticateErr error
	permissions     []string
	allowed         bool
}

func (f *fakeAuthService) Authenticate(context.Context, string) (auth.Principal, error) {
	return f.principal, f.authenticateErr
}

func (f *fakeAuthService) Can(context.Context, auth.Principal, string) (bool, error) {
	return f.allowed, nil
}

func (f *fakeAuthService) Permissions(context.Context, auth.Principal) ([]string, error) {
	return f.permissions, nil
}
