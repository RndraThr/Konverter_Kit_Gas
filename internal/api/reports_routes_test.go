package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/reports"
)

type fakeReportsService struct {
	scheduleID       string
	exportFormat     string
	synced           bool
	seenRegencyScope auth.RegencyScope
}

func (s *fakeReportsService) Summary(_ context.Context, scheduleID string, scope auth.RegencyScope) (reports.Summary, error) {
	s.scheduleID, s.seenRegencyScope = scheduleID, scope
	return reports.Summary{TotalAllocations: 200}, nil
}
func (s *fakeReportsService) Report(_ context.Context, scheduleID string, scope auth.RegencyScope) (reports.Report, error) {
	s.scheduleID, s.seenRegencyScope = scheduleID, scope
	return reports.Report{
		ScheduleInfo: reports.ScheduleInfo{ScheduleID: scheduleID, RegencyName: "Wajo"},
		Rows:         []reports.Row{{RowNumber: 1, DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001"}},
	}, nil
}
func (s *fakeReportsService) ExportExcel(_ context.Context, _ auth.Principal, scheduleID string, _ auth.ClientMeta, scope auth.RegencyScope) (reports.File, error) {
	s.scheduleID, s.exportFormat, s.seenRegencyScope = scheduleID, "xlsx", scope
	return reports.File{Filename: "LAPORAN DP3 - WAJO.xlsx", Data: []byte("excel-bytes")}, nil
}
func (s *fakeReportsService) ExportPDF(_ context.Context, _ auth.Principal, scheduleID string, _ auth.ClientMeta, scope auth.RegencyScope) (reports.File, error) {
	s.scheduleID, s.exportFormat, s.seenRegencyScope = scheduleID, "pdf", scope
	return reports.File{Filename: "LAPORAN DP3 - WAJO.pdf", Data: []byte("%PDF-bytes")}, nil
}
func (s *fakeReportsService) DriveStatus(_ context.Context, scheduleID string, scope auth.RegencyScope) (reports.DriveStatus, error) {
	s.scheduleID, s.seenRegencyScope = scheduleID, scope
	return reports.DriveStatus{Available: true, RecipientCount: 1}, nil
}
func (s *fakeReportsService) SyncDrive(_ context.Context, scheduleID string, scope auth.RegencyScope) (reports.DriveStatus, error) {
	s.scheduleID, s.synced, s.seenRegencyScope = scheduleID, true, scope
	return reports.DriveStatus{Available: true, RecipientCount: 1}, nil
}

func serveReports(t *testing.T, permissions map[string]bool, service *fakeReportsService, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	secret := []byte("01234567890123456789012345678901")
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: permissions, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Reports: service, SessionSecret: secret}).ServeHTTP(rec, authenticatedRequest(method, path, "", secret))
	return rec
}

func TestReportsRowsReturnsRecipientsInCallerScope(t *testing.T) {
	service := &fakeReportsService{}
	rec := serveReports(t, map[string]bool{"distribution.view": true}, service, http.MethodGet, "/api/v1/reports/schedule/schedule-1/rows")
	if rec.Code != http.StatusOK || service.scheduleID != "schedule-1" || !strings.Contains(rec.Body.String(), "7306014101900001") {
		t.Fatalf("status=%d schedule=%q body=%s", rec.Code, service.scheduleID, rec.Body.String())
	}
	if len(service.seenRegencyScope.RegencyIDs) != 1 || service.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", service.seenRegencyScope)
	}
	if strings.Contains(rec.Body.String(), "status") {
		t.Fatalf("laporan DP3 must not carry statuses: %s", rec.Body.String())
	}
}

func TestReportsEndpointsRequireDistributionViewPermission(t *testing.T) {
	for _, path := range []string{"rows", "export.xlsx", "export.pdf", "drive"} {
		rec := serveReports(t, map[string]bool{}, &fakeReportsService{}, http.MethodGet, "/api/v1/reports/schedule/schedule-1/"+path)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d", path, rec.Code)
		}
	}
}

func TestReportsExportsAreNamedAttachments(t *testing.T) {
	for _, tc := range []struct{ path, contentType, filename, body string }{
		{"export.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "LAPORAN DP3 - WAJO.xlsx", "excel-bytes"},
		{"export.pdf", "application/pdf", "LAPORAN DP3 - WAJO.pdf", "%PDF-bytes"},
	} {
		rec := serveReports(t, map[string]bool{"distribution.view": true}, &fakeReportsService{}, http.MethodGet, "/api/v1/reports/schedule/schedule-1/"+tc.path)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != tc.contentType || rec.Body.String() != tc.body {
			t.Fatalf("%s: status=%d headers=%v body=%q", tc.path, rec.Code, rec.Header(), rec.Body.String())
		}
		if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, tc.filename) {
			t.Fatalf("%s: content-disposition=%q", tc.path, disposition)
		}
	}
}

func TestReportsDriveSyncNeedsDistributionManage(t *testing.T) {
	service := &fakeReportsService{}
	if rec := serveReports(t, map[string]bool{"distribution.view": true}, service, http.MethodPost, "/api/v1/reports/schedule/schedule-1/drive/sync"); rec.Code != http.StatusForbidden || service.synced {
		t.Fatalf("viewer sync: status=%d synced=%v", rec.Code, service.synced)
	}
	if rec := serveReports(t, map[string]bool{"distribution.view": true}, service, http.MethodGet, "/api/v1/reports/schedule/schedule-1/drive"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":true`) {
		t.Fatalf("drive status: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := serveReports(t, map[string]bool{"distribution.view": true, "distribution.manage": true}, service, http.MethodPost, "/api/v1/reports/schedule/schedule-1/drive/sync"); rec.Code != http.StatusOK || !service.synced {
		t.Fatalf("manager sync: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := serveReports(t, map[string]bool{"distribution.view": true}, service, http.MethodGet, "/api/v1/reports/schedule/schedule-1/summary"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total_allocations":200`) {
		t.Fatalf("summary kept for the mobile app: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
