package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/monitoring"
)

type fakeDashboardService struct {
	programID string
	err       error
}

func (f *fakeDashboardService) Dashboard(_ context.Context, programID string, _ auth.RegencyScope) (monitoring.Dashboard, error) {
	f.programID = programID
	return monitoring.Dashboard{ProgramID: programID}, f.err
}

func dashboardRequest(t *testing.T, permissions map[string]bool, service *fakeDashboardService, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	user := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: permissions}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: user, Dashboard: service}).ServeHTTP(rec, authenticatedRequest(method, path, "", nil))
	return rec
}

func TestDashboardProgressNeedsPermission(t *testing.T) {
	service := &fakeDashboardService{}
	rec := dashboardRequest(t, map[string]bool{"dashboard.view": true, "monitoring.view": true}, service, http.MethodGet, "/api/v1/dashboard/progress?program_id=p1")
	if rec.Code != http.StatusForbidden || service.programID != "" {
		t.Fatalf("status=%d program=%q", rec.Code, service.programID)
	}
}

func TestDashboardProgressReturnsData(t *testing.T) {
	service := &fakeDashboardService{}
	rec := dashboardRequest(t, map[string]bool{"progress_dashboard.view": true}, service, http.MethodGet, "/api/v1/dashboard/progress?program_id=p1")
	var body struct {
		Data monitoring.Dashboard `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || service.programID != "p1" || body.Data.ProgramID != "p1" {
		t.Fatalf("status=%d program=%q body=%s", rec.Code, service.programID, rec.Body.String())
	}
}

func TestDashboardProgressUnknownProgram(t *testing.T) {
	service := &fakeDashboardService{err: monitoring.ErrProgramNotFound}
	rec := dashboardRequest(t, map[string]bool{"progress_dashboard.view": true}, service, http.MethodGet, "/api/v1/dashboard/progress?program_id=x")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDashboardProgressOnlyServesGet(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	user := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"progress_dashboard.view": true}}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: user, Dashboard: &fakeDashboardService{}, SessionSecret: secret}).ServeHTTP(rec, authenticatedRequest(http.MethodPost, "/api/v1/dashboard/progress", "", secret))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
