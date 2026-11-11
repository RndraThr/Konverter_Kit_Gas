package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type fakeDP3ExcelService struct {
	scheduleID string
	synced     bool
}

func (f *fakeDP3ExcelService) Status(_ context.Context, scheduleID string, _ auth.RegencyScope) (bast.DP3ExcelStatus, error) {
	f.scheduleID = scheduleID
	return bast.DP3ExcelStatus{Available: true, Filename: "DP3 - WAJO - Data Penerima.xlsx", RecipientCount: 4}, nil
}
func (f *fakeDP3ExcelService) Open(_ context.Context, scheduleID string, _ auth.RegencyScope) (bast.DP3ExcelContent, error) {
	f.scheduleID = scheduleID
	return bast.DP3ExcelContent{Filename: "DP3 - WAJO - Data Penerima.xlsx", Reader: io.NopCloser(strings.NewReader("xlsx"))}, nil
}
func (f *fakeDP3ExcelService) Sync(_ context.Context, scheduleID string, _ auth.RegencyScope) (bast.DP3ExcelStatus, error) {
	f.scheduleID, f.synced = scheduleID, true
	return bast.DP3ExcelStatus{Available: true}, nil
}

func TestDP3ExcelRoutes(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"bast.view": true}}
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"bast.view": true, "bast.manage": true}}
	serve := func(authService *fakeAuthService, service *fakeDP3ExcelService, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authService, DP3Excel: service, SessionSecret: secret}).ServeHTTP(rec, authenticatedRequest(method, path, body, secret))
		return rec
	}

	service := &fakeDP3ExcelService{}
	rec := serve(viewer, service, http.MethodGet, "/api/v1/bast/dp3/excel?schedule_id=schedule-1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"recipient_count":4`) || service.scheduleID != "schedule-1" {
		t.Fatalf("status: code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = serve(viewer, service, http.MethodGet, "/api/v1/bast/dp3/excel/content?schedule_id=schedule-1", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "xlsx" ||
		!strings.Contains(rec.Header().Get("Content-Type"), "spreadsheetml") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "Data Penerima.xlsx") {
		t.Fatalf("content: code=%d headers=%v", rec.Code, rec.Header())
	}

	if rec = serve(viewer, service, http.MethodGet, "/api/v1/bast/dp3/excel", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing schedule: code=%d", rec.Code)
	}

	if rec = serve(viewer, service, http.MethodPost, "/api/v1/bast/dp3/excel/sync", `{"schedule_id":"schedule-1"}`); rec.Code != http.StatusForbidden || service.synced {
		t.Fatalf("sync without bast.manage: code=%d synced=%v", rec.Code, service.synced)
	}
	if rec = serve(manager, service, http.MethodPost, "/api/v1/bast/dp3/excel/sync", `{"schedule_id":"schedule-1"}`); rec.Code != http.StatusOK || !service.synced {
		t.Fatalf("sync: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
