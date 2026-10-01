package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type fakeBASTService struct {
	dates      []bast.DateSummary
	recipients []bast.RecipientDocument
	preview    bast.BundlePreview
	finalized  bast.DailyBundle
	content    bast.BundleContent
	request    bast.BundleRequest
	lockResult bast.LockResult
	lockErr    error
}

func (f *fakeBASTService) ListDates(_ context.Context, programID, regencyID string, _ auth.RegencyScope) ([]bast.DateSummary, error) {
	f.request.ProgramID, f.request.RegencyID = programID, regencyID
	return f.dates, nil
}
func (f *fakeBASTService) ListRecipients(_ context.Context, programID, regencyID, date string, _ auth.RegencyScope) ([]bast.RecipientDocument, error) {
	f.request = bast.BundleRequest{ProgramID: programID, RegencyID: regencyID, LocalDate: date}
	return f.recipients, nil
}
func (f *fakeBASTService) LockRegencyTotal(_ context.Context, _ auth.Principal, programID, regencyID string, _ auth.RegencyScope, _ auth.ClientMeta) (bast.LockResult, error) {
	f.request.ProgramID, f.request.RegencyID = programID, regencyID
	return f.lockResult, f.lockErr
}
func (f *fakeBASTService) PreviewBundle(_ context.Context, request bast.BundleRequest, _ auth.RegencyScope) (bast.BundlePreview, error) {
	f.request = request
	return f.preview, nil
}
func (f *fakeBASTService) FinalizeBundle(_ context.Context, _ auth.Principal, request bast.BundleRequest, _ auth.RegencyScope, _ auth.ClientMeta) (bast.DailyBundle, error) {
	f.request = request
	return f.finalized, nil
}
func (f *fakeBASTService) OpenBundle(_ context.Context, _ string, _ auth.RegencyScope) (bast.BundleContent, error) {
	return f.content, nil
}

func TestBASTIndividualDatesRequiresViewAndForwardsScope(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency"}}}
	service := &fakeBASTService{dates: []bast.DateSummary{{LocalDate: "2024-12-10", RecipientCount: 50, ValidationStatus: "ready"}}}
	req := authenticatedRequest(http.MethodGet, "/api/v1/bast/individual/dates?program_id=program&regency_id=regency", "", nil)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, BAST: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "2024-12-10") || service.request.ProgramID != "program" {
		t.Fatalf("status=%d request=%+v body=%s", rec.Code, service.request, rec.Body.String())
	}
}

func TestBASTIndividualRecipientsRejectsMalformedDate(t *testing.T) {
	viewer := &fakeAuthService{allowedPermissions: map[string]bool{"bast.view": true}}
	service := &fakeBASTService{}
	req := authenticatedRequest(http.MethodGet, "/api/v1/bast/individual/recipients?program_id=p&regency_id=r&date=10-12-2024", "", nil)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, BAST: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBASTIndividualPreviewStreamsPDFWithIndonesianFilename(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.view": true}}
	service := &fakeBASTService{preview: bast.BundlePreview{PDF: []byte("%PDF-preview"), Filename: "SELASA, 10 DESEMBER 2024.pdf", RecipientCount: 2, PageCount: 2}}
	req := authenticatedRequest(http.MethodPost, "/api/v1/bast/individual/bundles/preview", `{"program_id":"p","regency_id":"r","local_date":"2024-12-10"}`, secret)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, BAST: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != "%PDF-preview" || !strings.Contains(rec.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestBASTIndividualFinalizeAndContent(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.manage": true, "bast.view": true}}
	service := &fakeBASTService{finalized: bast.DailyBundle{ID: "bundle", Filename: "SELASA, 10 DESEMBER 2024.pdf", Status: "active"}, content: bast.BundleContent{Reader: io.NopCloser(strings.NewReader("%PDF-final")), Filename: "SELASA, 10 DESEMBER 2024.pdf"}}
	finalize := authenticatedRequest(http.MethodPost, "/api/v1/bast/individual/bundles/finalize", `{"program_id":"p","regency_id":"r","local_date":"2024-12-10"}`, secret)
	finalizeRec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, BAST: service, SessionSecret: secret}).ServeHTTP(finalizeRec, finalize)
	if finalizeRec.Code != http.StatusOK || !strings.Contains(finalizeRec.Body.String(), "bundle") {
		t.Fatalf("status=%d body=%s", finalizeRec.Code, finalizeRec.Body.String())
	}
	content := authenticatedRequest(http.MethodGet, "/api/v1/bast/individual/bundles/bundle/content", "", nil)
	contentRec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, BAST: service}).ServeHTTP(contentRec, content)
	if contentRec.Code != http.StatusOK || contentRec.Body.String() != "%PDF-final" || !strings.Contains(contentRec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("status=%d headers=%v body=%s", contentRec.Code, contentRec.Header(), contentRec.Body.String())
	}
}

type fakeBrandingService struct {
	logos    []bast.LogoAsset
	uploaded bast.LogoUploadInput
	patched  bast.LogoPatchInput
	listedID string
}

func (f *fakeBrandingService) ListBranding(_ context.Context, programID string) ([]bast.LogoAsset, error) {
	f.listedID = programID
	return f.logos, nil
}
func (f *fakeBrandingService) UploadLogo(_ context.Context, _ auth.Principal, input bast.LogoUploadInput, _ auth.ClientMeta) (bast.LogoAsset, error) {
	f.uploaded = input
	return bast.LogoAsset{ID: "logo-new", ProgramID: input.ProgramID, SlotCode: input.SlotCode}, nil
}
func (f *fakeBrandingService) PatchLogo(_ context.Context, _ auth.Principal, input bast.LogoPatchInput, _ auth.ClientMeta) (bast.LogoAsset, error) {
	f.patched = input
	return bast.LogoAsset{ID: input.ID, ProgramID: input.ProgramID, SortOrder: input.SortOrder, IsVisible: input.IsVisible}, nil
}
func (f *fakeBrandingService) OpenLogo(_ context.Context, _, _ string) (bast.LogoContent, error) {
	return bast.LogoContent{}, nil
}

func TestBASTBrandingListRequiresViewAndForwardsProgram(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.view": true}}
	service := &fakeBrandingService{logos: []bast.LogoAsset{{ID: "logo-1", SlotCode: "left"}}}
	req := authenticatedRequest(http.MethodGet, "/api/v1/bast/branding?program_id=prog", "", nil)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, BASTBranding: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "logo-1") || service.listedID != "prog" {
		t.Fatalf("status=%d listed=%q body=%s", rec.Code, service.listedID, rec.Body.String())
	}
}

func TestBASTBrandingPatchRejectsViewerWithoutManage(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.view": true}}
	service := &fakeBrandingService{}
	req := authenticatedRequest(http.MethodPatch, "/api/v1/bast/branding/logos/logo-1", `{"program_id":"prog","sort_order":2,"max_width_mm":35,"max_height_mm":18,"is_visible":true}`, secret)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, BASTBranding: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBASTBrandingUploadStoresLogoForManager(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"bast.manage": true}}
	service := &fakeBrandingService{}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("program_id", "prog")
	_ = writer.WriteField("slot_code", "left")
	_ = writer.WriteField("sort_order", "1")
	part, _ := writer.CreateFormFile("file", "logo.png")
	_, _ = part.Write(append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 16)...))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bast/branding/logos", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, BASTBranding: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.uploaded.ProgramID != "prog" || service.uploaded.SlotCode != "left" {
		t.Fatalf("status=%d uploaded=%+v body=%s", rec.Code, service.uploaded, rec.Body.String())
	}
}

func authenticatedRequest(method, target, body string, secret []byte) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	if len(secret) > 0 {
		req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	}
	return req
}
