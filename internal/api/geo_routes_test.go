package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
)

type fakeGeoService struct {
	address string
	lat     float64
	lng     float64
	calls   int
}

func (f *fakeGeoService) Reverse(_ context.Context, lat, lng float64) string {
	f.calls++
	f.lat, f.lng = lat, lng
	return f.address
}

func TestGeoReverseReturnsAddressForFieldRoles(t *testing.T) {
	field := &fakeAuthService{principal: auth.Principal{UserID: "field"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeGeoService{address: "Jl. Merdeka, Patila"}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: field, Geo: service}).ServeHTTP(rec, authenticatedRequest(http.MethodGet, "/api/v1/geo/reverse?lat=-4.01&lng=120.12", "", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"address":"Jl. Merdeka, Patila"`) || service.lat != -4.01 || service.lng != 120.12 {
		t.Fatalf("status=%d lat=%v lng=%v body=%s", rec.Code, service.lat, service.lng, rec.Body.String())
	}
}

func TestGeoReverseRequiresFieldPermission(t *testing.T) {
	nobody := &fakeAuthService{principal: auth.Principal{UserID: "nobody"}, allowedPermissions: map[string]bool{}}
	service := &fakeGeoService{address: "x"}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: nobody, Geo: service}).ServeHTTP(rec, authenticatedRequest(http.MethodGet, "/api/v1/geo/reverse?lat=1&lng=2", "", nil))
	if rec.Code != http.StatusForbidden || service.calls != 0 {
		t.Fatalf("status=%d calls=%d", rec.Code, service.calls)
	}
}

func TestGeoReverseRejectsInvalidCoordinates(t *testing.T) {
	field := &fakeAuthService{principal: auth.Principal{UserID: "field"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	for _, query := range []string{"lat=abc&lng=1", "lat=91&lng=1", "lat=1&lng=181", "lat=1", "lng=1", ""} {
		service := &fakeGeoService{address: "x"}
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: field, Geo: service}).ServeHTTP(rec, authenticatedRequest(http.MethodGet, "/api/v1/geo/reverse?"+query, "", nil))
		if rec.Code != http.StatusBadRequest || service.calls != 0 || !strings.Contains(rec.Body.String(), "validation_failed") {
			t.Fatalf("%q: status=%d calls=%d body=%s", query, rec.Code, service.calls, rec.Body.String())
		}
	}
}

func TestGeoReverseUnavailableWithoutService(t *testing.T) {
	field := &fakeAuthService{principal: auth.Principal{UserID: "field"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: field}).ServeHTTP(rec, authenticatedRequest(http.MethodGet, "/api/v1/geo/reverse?lat=1&lng=2", "", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}
