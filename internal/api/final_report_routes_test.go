package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/finalreport"
)

type stubFinalReport struct{ generateErr error }

func (stubFinalReport) GetSettings(context.Context, string, string, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (stubFinalReport) PutSettings(context.Context, auth.Principal, finalreport.Settings, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (stubFinalReport) ListPhotoPicks(context.Context, string, string, string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (stubFinalReport) ReplacePhotoPicks(context.Context, string, string, string, []string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (stubFinalReport) Preview(context.Context, string, string, auth.RegencyScope) (finalreport.Preview, error) {
	return finalreport.Preview{}, nil
}
func (s stubFinalReport) Generate(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (finalreport.Document, error) {
	return finalreport.Document{}, s.generateErr
}
func (stubFinalReport) ListDocuments(context.Context, string, string, auth.RegencyScope) ([]finalreport.Document, error) {
	return nil, nil
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
		NewHandler(Dependencies{Auth: viewer, FinalReport: stubFinalReport{}, SessionSecret: secret}).ServeHTTP(rec, req)
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
		NewHandler(Dependencies{Auth: viewer, FinalReport: stubFinalReport{}}).ServeHTTP(rec, req)
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
		{finalreport.ErrGenerateConflict, http.StatusConflict},
		{finalreport.ErrNotFound, http.StatusNotFound},
		{nil, http.StatusCreated},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := authenticatedRequest(http.MethodPost, "/api/v1/final-report/generate", `{"program_id":"p","regency_id":"r"}`, secret)
		NewHandler(Dependencies{Auth: manager, FinalReport: stubFinalReport{generateErr: tc.err}, SessionSecret: secret}).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("err=%v: expected %d, got %d body=%s", tc.err, tc.want, rec.Code, rec.Body.String())
		}
	}
}
