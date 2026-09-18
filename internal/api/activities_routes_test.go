package api

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
)

type fakeActivitiesService struct {
	page             activities.Page
	uploaded         activities.ActivityMedia
	uploadErr        error
	deleteErr        error
	content          activities.MediaContent
	contentErr       error
	seenFilter       activities.Filter
	seenUploadInput  activities.UploadInput
	seenRegencyScope auth.RegencyScope
	seenDeleteID     string
}

func (f *fakeActivitiesService) List(_ context.Context, filter activities.Filter, scope auth.RegencyScope) (activities.Page, error) {
	f.seenFilter, f.seenRegencyScope = filter, scope
	return f.page, nil
}
func (f *fakeActivitiesService) Upload(_ context.Context, _ auth.Principal, input activities.UploadInput, _ auth.ClientMeta, scope auth.RegencyScope) (activities.ActivityMedia, error) {
	f.seenUploadInput, f.seenRegencyScope = input, scope
	return f.uploaded, f.uploadErr
}
func (f *fakeActivitiesService) Delete(_ context.Context, _ auth.Principal, id string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.seenDeleteID, f.seenRegencyScope = id, scope
	return f.deleteErr
}
func (f *fakeActivitiesService) OpenContent(_ context.Context, id string, scope auth.RegencyScope) (activities.MediaContent, error) {
	f.seenDeleteID, f.seenRegencyScope = id, scope
	return f.content, f.contentErr
}

func TestActivitiesListRequiresViewPermissionAndForwardsFilters(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.view": true}}
	service := &fakeActivitiesService{page: activities.Page{Page: 1, PageSize: 24, Total: 1, Items: []activities.ActivityMedia{{ID: "media-1", DisplayName: "WJO-RAKOR-20260916-154500"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media?regency_id=regency-1&activity_type=rakor&page=2&page_size=48", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "WJO-RAKOR-20260916-154500") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenFilter.RegencyID != "regency-1" || service.seenFilter.ActivityType != "rakor" || service.seenFilter.Page != 2 || service.seenFilter.PageSize != 48 {
		t.Fatalf("filters were not forwarded: %+v", service.seenFilter)
	}

	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{}}
	denied := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media", nil)
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Activities: service}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without activities.view, got %d", deniedRecorder.Code)
	}
}

func TestActivitiesUploadRequiresManagePermissionAndParsesMultipart(t *testing.T) {
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{uploaded: activities.ActivityMedia{ID: "media-1", DisplayName: "WJO-RAKOR-20260916-154500"}}

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "foto.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake jpeg bytes"))
	_ = writer.WriteField("source", "gallery")
	_ = writer.WriteField("activity_type", "rakor")
	_ = writer.WriteField("regency_id", "regency-1")
	_ = writer.Close()

	secret := []byte("01234567890123456789012345678901")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/media", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenUploadInput.RegencyID != "regency-1" || service.seenUploadInput.ActivityType != "rakor" || service.seenUploadInput.Source != "gallery" || service.seenUploadInput.OriginalFilename != "foto.jpg" {
		t.Fatalf("upload input not forwarded: %+v", service.seenUploadInput)
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"activities.view": true}}
	deniedRecorder := httptest.NewRecorder()
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/activities/media", strings.NewReader(body.String()))
	deniedReq.Header.Set("Content-Type", writer.FormDataContentType())
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	NewHandler(Dependencies{Auth: viewer, Activities: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without activities.manage, got %d", deniedRecorder.Code)
	}
}

func TestActivityMediaDeleteRequiresManagePermission(t *testing.T) {
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}
	secret := []byte("01234567890123456789012345678901")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/activities/media/media-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || service.seenDeleteID != "media-1" {
		t.Fatalf("status=%d seenDeleteID=%q", rec.Code, service.seenDeleteID)
	}
}

func TestActivityMediaContentStreamsWithMimeType(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"activities.view": true}}
	service := &fakeActivitiesService{content: activities.MediaContent{Reader: io.NopCloser(strings.NewReader("bytes")), MimeType: "image/jpeg", Filename: "foto.jpg"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities/media/media-1/content", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.String() != "bytes" {
		t.Fatalf("status=%d content-type=%s body=%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
