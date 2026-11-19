package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/banner"
)

type fakeBannerService struct {
	kind     banner.Kind
	settings banner.Settings
}

func (f *fakeBannerService) Overview(context.Context, string, auth.RegencyScope) (banner.Overview, error) {
	return banner.Overview{ScheduleID: "schedule-1", RegencyName: "KAB. BONE"}, nil
}
func (f *fakeBannerService) SaveSettings(_ context.Context, _ auth.Principal, _ string, in banner.Settings, _ auth.RegencyScope, _ auth.ClientMeta) (banner.Overview, error) {
	f.settings = in
	return banner.Overview{Settings: in}, nil
}
func (f *fakeBannerService) SaveLogo(context.Context, auth.Principal, string, string, []byte, auth.RegencyScope, auth.ClientMeta) (banner.Overview, error) {
	return banner.Overview{HasLogo: true}, nil
}
func (f *fakeBannerService) Logo(context.Context, string, auth.RegencyScope) ([]byte, string, error) {
	return nil, "", banner.ErrNotFound
}
func (f *fakeBannerService) Banner(_ context.Context, _ string, kind banner.Kind, _ auth.RegencyScope) ([]byte, string, error) {
	f.kind = kind
	return []byte("%PDF-1.7"), "PLAKAT PETANI KABUPATEN BONE.pdf", nil
}

// Viewers download banners; only managers change their text.
func TestBannerRoutesPermissionsAndDownload(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeBannerService{}
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"banner.view": true}, regencyScope: auth.RegencyScope{Unrestricted: true}}
	request := func(method, path, body string, authSvc *fakeAuthService) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authSvc, Banner: service, SessionSecret: secret}).ServeHTTP(rec, req)
		return rec
	}

	rec := request(http.MethodGet, "/api/v1/banners/plakat.pdf?schedule_id=schedule-1", "", viewer)
	if rec.Code != http.StatusOK || service.kind != banner.KindPlakat || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), `attachment; filename="PLAKAT PETANI KABUPATEN BONE.pdf"`) {
		t.Fatalf("download: %d %v", rec.Code, rec.Header())
	}
	if rec := request(http.MethodGet, "/api/v1/banners", "", viewer); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing schedule: %d", rec.Code)
	}
	if rec := request(http.MethodPut, "/api/v1/banners/settings?schedule_id=schedule-1", `{"seremoni_name":"Ir. Lamhot"}`, viewer); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer saved settings: %d", rec.Code)
	}
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"banner.manage": true}, regencyScope: auth.RegencyScope{Unrestricted: true}}
	if rec := request(http.MethodPut, "/api/v1/banners/settings?schedule_id=schedule-1", `{"seremoni_name":"Ir. Lamhot"}`, manager); rec.Code != http.StatusOK || service.settings.SeremoniName != "Ir. Lamhot" {
		t.Fatalf("manager save: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/api/v1/banners/logo?schedule_id=schedule-1", "", viewer); rec.Code != http.StatusNotFound {
		t.Fatalf("missing logo: %d", rec.Code)
	}
}
