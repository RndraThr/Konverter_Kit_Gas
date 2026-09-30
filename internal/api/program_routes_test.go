package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

func TestProgramZonesGetRequiresViewPermission(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/program-setup/programs/program-1/zones", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProgramZonesGetListsAndAppliesScope(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.view": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/program-setup/programs/program-1/zones", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.zoneListProgramID != "program-1" {
		t.Fatalf("expected programID forwarded, got %q", programService.zoneListProgramID)
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}
}

func TestProgramZonesPostRequiresManagePermission(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.view": true},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/program-setup/programs/program-1/zones", strings.NewReader(`{"code":"zone-a","name":"Zona A","sort_order":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProgramZonesPostCreatesZoneWithProgramIDFromPath(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.manage": true},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/program-setup/programs/program-1/zones", strings.NewReader(`{"code":"zone-a","name":"Zona A","sort_order":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.zoneInput.ProgramID != "program-1" || programService.zoneInput.Code != "zone-a" {
		t.Fatalf("unexpected zone input: %+v", programService.zoneInput)
	}
}

func TestProgramZonePatchUpdatesZoneWithIDsFromPath(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.manage": true},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/program-setup/programs/program-1/zones/zone-9", strings.NewReader(`{"code":"zone-a","name":"Zona A Revisi","sort_order":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.zoneInput.ProgramID != "program-1" || programService.zoneInput.ID != "zone-9" {
		t.Fatalf("unexpected zone input: %+v", programService.zoneInput)
	}
}

func TestRegencyAssignmentPutRequiresManagePermission(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.view": true},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/program-setup/programs/program-1/assignments/regency-1", strings.NewReader(`{"zone_id":"zone-9"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRegencyAssignmentPutAssignsRegencyAndForwardsScope(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.manage": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/program-setup/programs/program-1/assignments/regency-1", strings.NewReader(`{"zone_id":"zone-9"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.assignmentInput.ProgramID != "program-1" || programService.assignmentInput.RegencyID != "regency-1" || programService.assignmentInput.ZoneID != "zone-9" {
		t.Fatalf("unexpected assignment input: %+v", programService.assignmentInput)
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}
}

func TestRegencyAssignmentOutOfScopeReturnsNotFoundWithoutDisclosingRegency(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.manage": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{assignErr: programs.ErrNotFound}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/program-setup/programs/program-1/assignments/regency-out-of-scope", strings.NewReader(`{"zone_id":"zone-9"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "regency-out-of-scope") {
		t.Fatalf("response must not disclose the regency: %s", rec.Body.String())
	}
}
