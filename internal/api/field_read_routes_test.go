package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
)

// Field roles (mobile app) read schedules and package templates without
// programs.view, which would also open the Persiapan Program menu.

func fieldReadRequest(t *testing.T, method, path string, permissions map[string]bool) (*httptest.ResponseRecorder, *fakeProgramSetupService) {
	t.Helper()
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "field-1"},
		allowedPermissions: permissions,
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{}
	var body *strings.Reader
	if method == http.MethodGet {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(`{}`)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	return rec, programService
}

func TestFieldRolesReadSchedulesWithinTheirRegencies(t *testing.T) {
	for _, permission := range []string{"distribution.view", "activities.view"} {
		rec, programs := fieldReadRequest(t, http.MethodGet, "/api/v1/program-setup/schedules", map[string]bool{permission: true})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", permission, rec.Code, rec.Body.String())
		}
		if programs.seenRegencyScope.Unrestricted || len(programs.seenRegencyScope.RegencyIDs) != 1 {
			t.Fatalf("%s: regency scope not applied: %+v", permission, programs.seenRegencyScope)
		}
	}
}

func TestFieldRolesReadPackageTemplates(t *testing.T) {
	rec, _ := fieldReadRequest(t, http.MethodGet, "/api/v1/program-setup/package-templates", map[string]bool{"distribution.view": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFieldReadStillRequiresSomeAccess(t *testing.T) {
	for _, path := range []string{"/api/v1/program-setup/schedules", "/api/v1/program-setup/package-templates"} {
		rec, _ := fieldReadRequest(t, http.MethodGet, path, map[string]bool{"dashboard.view": true})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d, want 403", path, rec.Code)
		}
	}
}

func TestFieldRolesCannotChangeProgramSetup(t *testing.T) {
	field := map[string]bool{"distribution.view": true, "activities.view": true}
	for _, path := range []string{"/api/v1/program-setup/schedules", "/api/v1/program-setup/package-templates"} {
		rec, _ := fieldReadRequest(t, http.MethodPost, path, field)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d, want 403", path, rec.Code)
		}
	}
	// Lists only Persiapan Program needs stay behind programs.view.
	rec, _ := fieldReadRequest(t, http.MethodGet, "/api/v1/program-setup/documentation-templates", field)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("documentation templates: status=%d, want 403", rec.Code)
	}
}
