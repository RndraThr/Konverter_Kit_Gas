package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
	"konkit/internal/finalreport"
	"konkit/internal/media"
)

type stubFinalReport struct {
	generateErr error
	openErr     error
	openedID    string
	openScope   auth.RegencyScope
}

func (*stubFinalReport) GetSettings(context.Context, string, string, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (*stubFinalReport) PutSettings(context.Context, auth.Principal, finalreport.Settings, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (*stubFinalReport) ListPhotoPicks(context.Context, string, string, string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (*stubFinalReport) ReplacePhotoPicks(context.Context, string, string, string, []string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (*stubFinalReport) Preview(context.Context, string, string, auth.RegencyScope) (finalreport.Preview, error) {
	return finalreport.Preview{}, nil
}
func (s *stubFinalReport) Generate(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (finalreport.Document, error) {
	return finalreport.Document{}, s.generateErr
}
func (*stubFinalReport) ListDocuments(context.Context, string, string, auth.RegencyScope) ([]finalreport.Document, error) {
	return nil, nil
}
func (s *stubFinalReport) OpenDocument(_ context.Context, id string, scope auth.RegencyScope) (finalreport.DocumentContent, error) {
	s.openedID, s.openScope = id, scope
	if s.openErr != nil {
		return finalreport.DocumentContent{}, s.openErr
	}
	return finalreport.DocumentContent{Filename: "LAPORAN AKHIR - PETANI KAB. BIREUEN.pptx", Reader: io.NopCloser(strings.NewReader("pptx-bytes"))}, nil
}

func TestFinalReportMutationsRejectViewerWithoutManage(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.view": true}}
	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/final-report/generate", `{"program_id":"p","regency_id":"r"}`},
		{http.MethodPut, "/api/v1/final-report/settings", `{"program_id":"p","regency_id":"r"}`},
		{http.MethodPut, "/api/v1/final-report/photo-picks", `{"program_id":"p","regency_id":"r","slide_key":"rapat_koordinasi","activity_media_ids":[]}`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := authenticatedRequest(tc.method, tc.path, tc.body, secret)
		NewHandler(Dependencies{Auth: viewer, FinalReport: &stubFinalReport{}, SessionSecret: secret}).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: expected 403, got %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestFinalReportViewerCanRead(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.view": true}}
	for _, path := range []string{"settings", "photo-picks", "preview", "documents"} {
		rec := httptest.NewRecorder()
		req := authenticatedRequest(http.MethodGet, "/api/v1/final-report/"+path+"?program_id=p&regency_id=r", "", nil)
		NewHandler(Dependencies{Auth: viewer, FinalReport: &stubFinalReport{}}).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestFinalReportGenerateMapsServiceErrors(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.manage": true}}
	cases := []struct {
		err  error
		want int
	}{
		{finalreport.ErrRegencyOutOfScope, http.StatusForbidden},
		{finalreport.ErrProgramRequired, http.StatusBadRequest},
		{finalreport.ErrRegencyRequired, http.StatusBadRequest},
		{finalreport.ErrDuplicatePick, http.StatusBadRequest},
		{finalreport.ErrPhotoTypeInvalid, http.StatusBadRequest},
		{fmt.Errorf("build folder path: %w", media.ErrInvalidFolderPath), http.StatusConflict},
		{finalreport.ErrNotFound, http.StatusNotFound},
		{nil, http.StatusCreated},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := authenticatedRequest(http.MethodPost, "/api/v1/final-report/generate", `{"program_id":"p","regency_id":"r"}`, secret)
		NewHandler(Dependencies{Auth: manager, FinalReport: &stubFinalReport{generateErr: tc.err}, SessionSecret: secret}).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("err=%v: expected %d, got %d body=%s", tc.err, tc.want, rec.Code, rec.Body.String())
		}
	}
}

func TestFinalReportGenerateInvalidFolderPathHasIndonesianMessage(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.manage": true}}
	rec := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/api/v1/final-report/generate", `{"program_id":"p","regency_id":"r"}`, secret)
	NewHandler(Dependencies{Auth: manager, FinalReport: &stubFinalReport{generateErr: fmt.Errorf("x: %w", media.ErrInvalidFolderPath)}, SessionSecret: secret}).ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "folder_path_invalid") || !strings.Contains(rec.Body.String(), "Persiapan Program") {
		t.Fatalf("unexpected body %s", rec.Body.String())
	}
}

func TestFinalReportDocumentContentDownloads(t *testing.T) {
	viewer := &fakeAuthService{
		principal:          auth.Principal{UserID: "user"},
		allowedPermissions: map[string]bool{"final_report.view": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	service := &stubFinalReport{}
	rec := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodGet, "/api/v1/final-report/documents/doc-1/content", "", nil)
	NewHandler(Dependencies{Auth: viewer, FinalReport: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if service.openedID != "doc-1" || service.openScope.Unrestricted || len(service.openScope.RegencyIDs) != 1 {
		t.Fatalf("service saw id=%q scope=%+v", service.openedID, service.openScope)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Fatalf("content-type %q", got)
	}
	disposition, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != "LAPORAN AKHIR - PETANI KAB. BIREUEN.pptx" {
		t.Fatalf("content-disposition %q (err=%v)", rec.Header().Get("Content-Disposition"), err)
	}
	if rec.Body.String() != "pptx-bytes" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestFinalReportDocumentContentNotFoundAndPermission(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.view": true}}
	rec := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodGet, "/api/v1/final-report/documents/missing/content", "", nil)
	NewHandler(Dependencies{Auth: viewer, FinalReport: &stubFinalReport{openErr: finalreport.ErrNotFound}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown/out-of-scope id: expected 404, got %d", rec.Code)
	}

	outsider := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"dashboard.view": true}}
	rec = httptest.NewRecorder()
	req = authenticatedRequest(http.MethodGet, "/api/v1/final-report/documents/doc-1/content", "", nil)
	NewHandler(Dependencies{Auth: outsider, FinalReport: &stubFinalReport{}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without final_report.view: expected 403, got %d", rec.Code)
	}

	for _, path := range []string{"/api/v1/final-report/documents/doc-1", "/api/v1/final-report/documents/doc-1/other", "/api/v1/final-report/documents//content"} {
		rec = httptest.NewRecorder()
		req = authenticatedRequest(http.MethodGet, path, "", nil)
		NewHandler(Dependencies{Auth: viewer, FinalReport: &stubFinalReport{}}).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", path, rec.Code)
		}
	}
}

// A role holding only final_report.view must be able to load everything the
// Laporan Akhir page reads: program list, program zones (kabupaten picker),
// activity photo lists and their content.
func TestFinalReportOnlyRoleCanReadPageDependencies(t *testing.T) {
	viewer := &fakeAuthService{
		principal:          auth.Principal{UserID: "user"},
		allowedPermissions: map[string]bool{"final_report.view": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{}
	activityService := &fakeActivitiesService{content: activities.MediaContent{Reader: io.NopCloser(strings.NewReader("jpg")), MimeType: "image/jpeg", Filename: "a.jpg"}}
	handler := NewHandler(Dependencies{Auth: viewer, Programs: programService, Activities: activityService})
	for _, path := range []string{
		"/api/v1/program-setup/programs",
		"/api/v1/program-setup/programs/prog-1/zones",
		"/api/v1/activities/media?activity_type=rakor&regency_id=regency-1&program_id=prog-1&page_size=100",
		"/api/v1/activities/media/media-1/content",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authenticatedRequest(http.MethodGet, path, "", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if programService.seenRegencyScope.Unrestricted || activityService.seenRegencyScope.Unrestricted {
		t.Fatal("reads must stay regency-scoped")
	}
}
