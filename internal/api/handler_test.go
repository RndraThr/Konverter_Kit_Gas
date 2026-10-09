package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"konkit/internal/administration"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/bast"
	"konkit/internal/dcp3"
	"konkit/internal/distribution"
	"konkit/internal/health"
	"konkit/internal/profile"
	"konkit/internal/programs"
	"konkit/internal/recipients"
	"konkit/internal/reports"
)

func TestHealthReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHealthRejectsPostWithJSONResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if rec.Header().Get("Location") != "" {
		t.Fatal("API errors must not redirect to HTML routes")
	}
}

func TestProtectedAPIReturnsJSONUnauthorizedWithoutSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: &fakeAuthService{authenticateErr: auth.ErrSessionNotFound}}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("expected JSON unauthorized response, got %s", rec.Body.String())
	}
}

func TestMeReturnsPrincipalPermissionsAndCSRFToken(t *testing.T) {
	service := &fakeAuthService{
		principal:   auth.Principal{UserID: "user-1", FullName: "Admin Konkit", Username: "admin", Email: "admin@konkit.test", Roles: []string{"super_admin"}},
		permissions: []string{"*"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			FullName    string   `json:"full_name"`
			Permissions []string `json:"permissions"`
		} `json:"data"`
		Meta struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.FullName != "Admin Konkit" || len(payload.Data.Permissions) != 1 || payload.Meta.CSRFToken == "" {
		t.Fatalf("unexpected me response: %+v", payload)
	}
}

func TestMeIncludesCurrentProfileState(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, permissions: []string{"dashboard.view"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	profiles := fakeProfileService{profile: profile.Profile{ID: "user-1", FullName: "Admin Program", Username: "admin", Email: "admin@konkit.test", Roles: []string{"operator"}, IsActive: true}}

	NewHandler(Dependencies{Auth: service, Profile: profiles, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"is_active":true`) || !strings.Contains(rec.Body.String(), `"roles":["operator"]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMutationRejectsMissingCSRFBeforeCallingService(t *testing.T) {
	service := &fakeAuthService{
		principal:   auth.Principal{UserID: "user-1", Roles: []string{"super_admin"}},
		permissions: []string{"*"},
		allowed:     true,
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me", strings.NewReader(`{"full_name":"Admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"csrf_invalid"`) {
		t.Fatalf("expected CSRF rejection, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdministrationRoutesUseDocumentedPrefixes(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	for _, path := range []string{"/api/v1/admin/users", "/api/v1/admin/roles", "/api/v1/admin/permissions", "/api/v1/system/settings", "/api/v1/system/audit-logs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		rec := httptest.NewRecorder()

		NewHandler(Dependencies{Auth: service}).ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("documented endpoint %s was not routed", path)
		}
	}
}

func TestAuditRouteForwardsSearchAndDateFilters(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	audits := &fakeAuditService{page: audit.Page{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/audit-logs?query=login&actor=rendra&action=user.updated&resource_type=users&actor_user_id=00000000-0000-0000-0000-000000000001&date_from=2026-10-01&date_to=2026-10-09&page_size=50", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Audit: audits}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	got := audits.filter
	if got.Query != "login" || got.Actor != "rendra" || got.Action != "user.updated" || got.ResourceType != "users" ||
		got.ActorUserID != "00000000-0000-0000-0000-000000000001" || got.DateFrom != "2026-10-01" || got.DateTo != "2026-10-09" || got.PageSize != 50 {
		t.Fatalf("filter not forwarded: %+v", got)
	}
}

func TestAuditRouteRejectsInvalidDateFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		field string
	}{
		{name: "invalid from", query: "date_from=09-10-2026", field: "date_from"},
		{name: "invalid to", query: "date_to=tomorrow", field: "date_to"},
		{name: "reversed", query: "date_from=2026-10-09&date_to=2026-10-01", field: "date_to"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
			audits := &fakeAuditService{}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/system/audit-logs?"+test.query, nil)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
			rec := httptest.NewRecorder()

			NewHandler(Dependencies{Auth: authService, Audit: audits}).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"validation_failed"`) || !strings.Contains(rec.Body.String(), `"`+test.field+`"`) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestProgramSetupRoutesUseDocumentedPrefixes(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	programService := &fakeProgramSetupService{}
	for _, path := range []string{
		"/api/v1/program-setup/regencies",
		"/api/v1/program-setup/programs",
		"/api/v1/program-setup/schedules",
		"/api/v1/program-setup/package-templates",
		"/api/v1/program-setup/documentation-templates",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		rec := httptest.NewRecorder()

		NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("endpoint %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestDistributionViewerCanReadMapFilterOptions(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"distribution.view": true},
	}
	programService := &fakeProgramSetupService{}
	for _, path := range []string{
		"/api/v1/program-setup/regencies",
		"/api/v1/program-setup/programs",
		"/api/v1/program-setup/schedules",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		rec := httptest.NewRecorder()

		NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("distribution viewer cannot read %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestProgramSetupMutationRequiresManagePermission(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"programs.view": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/program-setup/regencies", strings.NewReader(`{"province_name":"Sulawesi Selatan","name":"Wajo","document_code":"WJO","is_active":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: &fakeProgramSetupService{}, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProgramSetupPatchPassesResourceID(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"programs.manage": true}}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/program-setup/regencies/regency-1", strings.NewReader(`{"province_name":"Sulawesi Selatan","name":"Wajo","document_code":"WJO","is_active":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || programService.regencyInput.ID != "regency-1" {
		t.Fatalf("status=%d id=%q body=%s", rec.Code, programService.regencyInput.ID, rec.Body.String())
	}
}

func TestRegenciesEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"programs.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/program-setup/regencies", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}
}

func TestSchedulesEndpointAllowsBASTViewerAndForwardsRegencyScope(t *testing.T) {
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"bast.view": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/program-setup/schedules", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}
}

func TestSaveScheduleForwardsCallerRegencyScopeAndReportsOutOfScope(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"programs.manage": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	programService := &fakeProgramSetupService{scheduleResult: programs.Schedule{ID: "schedule-1", RegencyID: "regency-1"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/program-setup/schedules", strings.NewReader(`{"program_id":"program-1","regency_id":"regency-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: programService, SessionSecret: secret}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || programService.scheduleInput.RegencyID != "regency-1" {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, programService.scheduleInput, rec.Body.String())
	}
	if programService.seenRegencyScope.Unrestricted || len(programService.seenRegencyScope.RegencyIDs) != 1 || programService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", programService.seenRegencyScope)
	}

	rejecting := &fakeProgramSetupService{scheduleErr: programs.ErrRegencyOutOfScope}
	rejectedReq := httptest.NewRequest(http.MethodPost, "/api/v1/program-setup/schedules", strings.NewReader(`{"program_id":"program-1","regency_id":"regency-outside"}`))
	rejectedReq.Header.Set("Content-Type", "application/json")
	rejectedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rejectedReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rejectedRec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Programs: rejecting, SessionSecret: secret}).ServeHTTP(rejectedRec, rejectedReq)

	if rejectedRec.Code != http.StatusForbidden || !strings.Contains(rejectedRec.Body.String(), "regency_out_of_scope") {
		t.Fatalf("status=%d body=%s", rejectedRec.Code, rejectedRec.Body.String())
	}
}

func TestDCP3PreviewEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	dcp3Service := &fakeDCP3Service{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dcp3/previews/batch-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(dcp3Service.seenRegencyScope.RegencyIDs) != 1 || dcp3Service.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", dcp3Service.seenRegencyScope)
	}
}

func TestDistributionCandidatesEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	distributionService := &fakeDistributionService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/candidates?schedule_id=schedule-1&nik=7306014101900001", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Distribution: distributionService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(distributionService.seenRegencyScope.RegencyIDs) != 1 || distributionService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", distributionService.seenRegencyScope)
	}
}

func TestDistributionCandidateSuggestionsEndpointReturnsScopedMatches(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	distributionService := &fakeDistributionService{suggestions: []distribution.CandidateMatch{{AllocationID: "allocation-1", FullName: "SITI AMINAH", NIK: "7306014101900001"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/candidate-suggestions?schedule_id=schedule-1&nik_prefix=7306", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Distribution: distributionService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if distributionService.suggestionScheduleID != "schedule-1" || distributionService.suggestionPrefix != "7306" {
		t.Fatalf("search = schedule:%q prefix:%q", distributionService.suggestionScheduleID, distributionService.suggestionPrefix)
	}
	if len(distributionService.seenRegencyScope.RegencyIDs) != 1 || distributionService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", distributionService.seenRegencyScope)
	}
}

func TestReportsSummaryEndpointAppliesCallerRegencyScope(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"regency-1"}}}
	reportsService := &fakeReportsService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, Reports: reportsService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(reportsService.seenRegencyScope.RegencyIDs) != 1 || reportsService.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", reportsService.seenRegencyScope)
	}
}

func TestDCP3PreviewAcceptsMultipartWorkbook(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	dcp3Service := &fakeDCP3Service{preview: dcp3.ImportPreview{ID: "batch-1"}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("schedule_id", "schedule-1")
	file, err := writer.CreateFormFile("file", "calon-penerima.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("PK\x03\x04workbook"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/previews", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || dcp3Service.scheduleID != "schedule-1" || dcp3Service.filename != "calon-penerima.xlsx" {
		t.Fatalf("status=%d schedule=%q filename=%q body=%s", rec.Code, dcp3Service.scheduleID, dcp3Service.filename, rec.Body.String())
	}
	if dcp3Service.seenHeaderRow != 1 {
		t.Fatalf("expected default header_row=1, got %d", dcp3Service.seenHeaderRow)
	}
}

func TestDCP3PreviewCreateUsesProvidedHeaderRow(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	dcp3Service := &fakeDCP3Service{preview: dcp3.ImportPreview{ID: "batch-1"}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("schedule_id", "schedule-1")
	_ = writer.WriteField("header_row", "3")
	file, err := writer.CreateFormFile("file", "calon-penerima.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("PK\x03\x04workbook"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/previews", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || dcp3Service.seenHeaderRow != 3 {
		t.Fatalf("status=%d header_row=%d body=%s", rec.Code, dcp3Service.seenHeaderRow, rec.Body.String())
	}
}

func TestDCP3RawPreviewReturnsRows(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	dcp3Service := &fakeDCP3Service{rawPreviewRows: [][]string{{"USULAN CALON PENERIMA"}, {"No", "Nama"}}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	file, err := writer.CreateFormFile("file", "calon-penerima.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("PK\x03\x04workbook"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/raw-preview", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "USULAN CALON PENERIMA") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestDCP3RawPreviewRequiresFile(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/raw-preview", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: &fakeDCP3Service{}, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDCP3PreviewRejectsNonWorkbook(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("schedule_id", "schedule-1")
	file, _ := writer.CreateFormFile("file", "calon-penerima.txt")
	_, _ = file.Write([]byte("plain text"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/previews", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: &fakeDCP3Service{}, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDCP3ImportPassesMapping(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dcp3.import": true}}
	dcp3Service := &fakeDCP3Service{result: dcp3.ImportResult{BatchID: "batch-1", TotalRows: 2}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/imports", strings.NewReader(`{"batch_id":"batch-1","mapping":{"full_name":"Nama"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || dcp3Service.batchID != "batch-1" || dcp3Service.mapping.FullName != "Nama" {
		t.Fatalf("status=%d batch=%q mapping=%+v body=%s", rec.Code, dcp3Service.batchID, dcp3Service.mapping, rec.Body.String())
	}
}

func TestDistributionSlotsRequiresPosMesinAndForwardsCreateSlotInput(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeDistributionService{createdSlot: distribution.DistributionSlot{ID: "slot-1", ScheduleID: "schedule-1", SlotNumber: 1, Status: "open"}}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots", strings.NewReader(`{"schedule_id":"schedule-1","slot_number":4}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.createInput.ScheduleID != "schedule-1" || service.createInput.SlotNumber != 4 {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.createInput, rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots", strings.NewReader(`{"schedule_id":"schedule-1"}`))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionCandidatesRequiresPosDokumen(t *testing.T) {
	service := &fakeDistributionService{candidate: distribution.CandidateMatch{AllocationID: "allocation-1", FullName: "Siti Aminah", NIK: "7306014101900001"}}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/candidates?schedule_id=schedule-1&nik=7306014101900001", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.candidateScheduleID != "schedule-1" || service.candidateNIK != "7306014101900001" {
		t.Fatalf("status=%d schedule=%q nik=%q body=%s", rec.Code, service.candidateScheduleID, service.candidateNIK, rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/candidates?schedule_id=schedule-1&nik=7306014101900001", nil)
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionSlotLinkRequiresPosDokumenAndForwardsSlotNumber(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeDistributionService{linkedSlot: distribution.DistributionSlot{ID: "slot-1", ScheduleID: "schedule-1", SlotNumber: 5, Status: "linked"}}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/5/link", strings.NewReader(`{"schedule_id":"schedule-1","nik":"7306014101900001"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.linkInput.SlotNumber != 5 || service.linkInput.ScheduleID != "schedule-1" || service.linkInput.NIK != "7306014101900001" {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.linkInput, rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/5/link", strings.NewReader(`{"schedule_id":"schedule-1","nik":"7306014101900001"}`))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionSlotSearchRequiresDistributionView(t *testing.T) {
	service := &fakeDistributionService{searchedSlot: distribution.DistributionSlot{ID: "slot-1", ScheduleID: "schedule-1", SlotNumber: 5, Status: "linked"}}

	// distribution.pos_penyerahan alone no longer suffices: the lookup is now
	// gated on the broader distribution.view permission.
	posPenyerahanOnly := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_penyerahan": true}}
	posPenyerahanReq := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/search?schedule_id=schedule-1&q=Siti", nil)
	posPenyerahanReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	posPenyerahanRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: posPenyerahanOnly, Distribution: service}).ServeHTTP(posPenyerahanRecorder, posPenyerahanReq)
	if posPenyerahanRecorder.Code != http.StatusForbidden {
		t.Fatalf("pos_penyerahan-only status=%d body=%s", posPenyerahanRecorder.Code, posPenyerahanRecorder.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/search?schedule_id=schedule-1&q=Siti", nil)
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionSlotSearchAllowsViewOnlyPermissionAndAnyStatus(t *testing.T) {
	// The behavior change this task exists to make: a caller with only
	// distribution.view (no distribution.pos_penyerahan) can look up a slot
	// of any status, not just "linked".
	service := &fakeDistributionService{searchedSlot: distribution.DistributionSlot{ID: "slot-1", ScheduleID: "schedule-1", SlotNumber: 5, Status: "completed"}}
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/search?schedule_id=schedule-1&q=Siti", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.searchScheduleID != "schedule-1" || service.searchQuery != "Siti" {
		t.Fatalf("status=%d schedule=%q query=%q body=%s", rec.Code, service.searchScheduleID, service.searchQuery, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatalf("expected non-linked status in body: %s", rec.Body.String())
	}
}

func TestDistributionSlotCatalogRequiresDistributionViewAndForwardsScheduleID(t *testing.T) {
	service := &fakeDistributionService{catalog: []distribution.SlotCatalogEntry{{SlotNumber: 1, Status: "completed", DocumentationComplete: true}}}
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/catalog?schedule_id=schedule-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.catalogScheduleID != "schedule-1" {
		t.Fatalf("status=%d schedule=%q body=%s", rec.Code, service.catalogScheduleID, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"documentation_complete":true`) {
		t.Fatalf("expected documentation_complete in body: %s", rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/slots/catalog?schedule_id=schedule-1", nil)
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestDistributionSlotsQuotaExceededReturnsConflict(t *testing.T) {
	service := &fakeDistributionService{createErr: distribution.ErrSlotQuotaExceeded}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	secret := []byte("01234567890123456789012345678901")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots", strings.NewReader(`{"schedule_id":"schedule-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"slot_quota_exceeded"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDistributionSlotCompleteRequiresPosPenyerahanAndReturnsStableConflicts(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeDistributionService{completedSlot: distribution.DistributionSlot{ID: "slot-1", ScheduleID: "schedule-1", SlotNumber: 5, Status: "completed"}}
	operator := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_penyerahan": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/5/complete?schedule_id=schedule-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: operator, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.completeInput.SlotNumber != 5 || service.completeInput.ScheduleID != "schedule-1" || !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.completeInput, rec.Body.String())
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/5/complete?schedule_id=schedule-1", nil)
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}

	conflicts := []struct {
		err  error
		code string
	}{{distribution.ErrIdentityIncomplete, "identity_incomplete"}, {distribution.ErrDocumentationIncomplete, "documentation_incomplete"}, {distribution.ErrEquipmentOptionNotFound, "equipment_option_not_found"}, {distribution.ErrPreviouslyReceived, "previously_received"}, {distribution.ErrAlreadyCompleted, "already_completed"}, {distribution.ErrSlotNotOpen, "slot_not_open"}, {distribution.ErrSlotNotLinked, "slot_not_linked"}}
	for _, item := range conflicts {
		response := httptest.NewRecorder()
		writeServiceError(response, item.err)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"`+item.code+`"`) {
			t.Fatalf("error=%v status=%d body=%s", item.err, response.Code, response.Body.String())
		}
	}
}

func TestReportsEndpointsRequireDistributionViewPermission(t *testing.T) {
	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Reports: &fakeReportsService{}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportsRowsAppliesFilterAndReturnsFullNIK(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	distributionNumber := 7
	service := &fakeReportsService{rows: []reports.Row{{DistributionNumber: &distributionNumber, FullName: "Siti Aminah", NIK: "7306014101900001"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/rows?allocation_status=ready", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Reports: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.scheduleID != "schedule-1" || service.filter.AllocationStatus != "ready" {
		t.Fatalf("status=%d schedule=%q filter=%+v body=%s", rec.Code, service.scheduleID, service.filter, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "7306014101900001") {
		t.Fatalf("expected full NIK in reports body: %s", rec.Body.String())
	}
}

func TestReportsExportReturnsAttachmentHeaders(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeReportsService{exportData: []byte("excel-bytes")}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/schedule/schedule-1/export.xlsx", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Reports: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.exportFormat != "xlsx" {
		t.Fatalf("status=%d format=%q body=%s", rec.Code, service.exportFormat, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content-type=%q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("content-disposition=%q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.String() != "excel-bytes" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestDistributionMediaUploadAndContentHeaders(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true, "distribution.view": true}}
	service := &fakeDistributionService{documentationStage: "dokumen", media: distribution.MediaFile{ID: "media-1", MimeType: "image/jpeg", OriginalFilename: "penerima.jpg"}, mediaContent: []byte("jpeg-content")}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("source", "camera")
	_ = writer.WriteField("file_size", "43")
	file, _ := writer.CreateFormFile("file", "penerima.jpg")
	_, _ = file.Write(append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...))
	_ = writer.Close()
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/slot-1/media", body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	upload.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	upload.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	uploadRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(uploadRecorder, upload)
	if uploadRecorder.Code != http.StatusCreated || service.slotID != "slot-1" || service.upload.Source != "camera" || service.upload.DeclaredSize != 43 || service.upload.Data == nil {
		t.Fatalf("upload status=%d slot=%q body=%s", uploadRecorder.Code, service.slotID, uploadRecorder.Body.String())
	}

	content := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/media/media-1/content", nil)
	content.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	contentRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service}).ServeHTTP(contentRecorder, content)
	if contentRecorder.Code != http.StatusOK || contentRecorder.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(contentRecorder.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("content status=%d headers=%v", contentRecorder.Code, contentRecorder.Header())
	}
}

func TestDistributionMediaUploadRejectsFileBeforeMetadata(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	service := &fakeDistributionService{documentationStage: "dokumen"}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	file, _ := writer.CreateFormFile("file", "proof.jpg")
	_, _ = file.Write([]byte{0xff, 0xd8, 0xff, 0xe0})
	_ = writer.WriteField("source", "camera")
	_ = writer.WriteField("file_size", "4")
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/slot-1/media", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	recorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"multipart_invalid"`) || service.upload.Data != nil {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDistributionMediaUploadMapsPolicyAndBusyErrors(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{distribution.ErrMediaPolicyInvalid, http.StatusUnsupportedMediaType, "media_policy_invalid"},
		{distribution.ErrVideoUploadBusy, http.StatusTooManyRequests, "video_upload_busy"},
	} {
		authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
		service := &fakeDistributionService{documentationStage: "dokumen", uploadErr: tt.err}
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("source", "gallery")
		_ = writer.WriteField("file_size", "12")
		file, _ := writer.CreateFormFile("file", "proof.mp4")
		_, _ = file.Write([]byte("\x00\x00\x00\x18ftypisom"))
		_ = writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/slot-1/media", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
		recorder := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(recorder, req)
		if recorder.Code != tt.status || !strings.Contains(recorder.Body.String(), `"code":"`+tt.code+`"`) {
			t.Fatalf("err=%v status=%d body=%s", tt.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDistributionDateUpdateUsesOneDateForTheSlotNumber(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	date := "2026-10-20"
	service := &fakeDistributionService{datedSlot: distribution.DistributionSlot{ID: "slot-1", SlotNumber: 3, DistributionDate: &date}}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/date?schedule_id=schedule-1", strings.NewReader(`{"distribution_date":"2026-10-20"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.dateInput.ScheduleID != "schedule-1" || service.dateInput.SlotNumber != 3 || service.dateInput.DistributionDate != "2026-10-20" {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.dateInput, rec.Body.String())
	}

	mesinOnly := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	deniedReq := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/date?schedule_id=schedule-1", strings.NewReader(`{"distribution_date":"2026-10-21"}`))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedReq.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: mesinOnly, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("POS Mesin date update status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}
}

func TestRetryMediaMoveRequiresPosDokumenAndForwardsScope(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeDistributionService{retriedMedia: distribution.MediaFile{ID: "media-1", StorageState: "moving"}}
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/media/media-1/retry-move", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	recorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: secret}).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || service.retryMediaID != "media-1" {
		t.Fatalf("status=%d id=%q body=%s", recorder.Code, service.retryMediaID, recorder.Body.String())
	}

	service.retryMediaErr = distribution.ErrMediaNotFound
	notFoundReq := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/media/deleted-or-cross-regency/retry-move", nil)
	notFoundReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	notFoundReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	notFoundRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: secret}).ServeHTTP(notFoundRecorder, notFoundReq)
	if notFoundRecorder.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d body=%s", notFoundRecorder.Code, notFoundRecorder.Body.String())
	}
}

func TestCompleteReturnsDistinctMediaMoveConflicts(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_penyerahan": true}}
	for _, tt := range []struct {
		err  error
		code string
	}{
		{distribution.ErrMediaMoveFailed, "media_move_failed"},
		{distribution.ErrMediaMovePending, "media_move_pending"},
	} {
		service := &fakeDistributionService{completeErr: tt.err}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/1/complete?schedule_id=schedule-1", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
		recorder := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: secret}).ServeHTTP(recorder, req)
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"`+tt.code+`"`) {
			t.Fatalf("err=%v status=%d body=%s", tt.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDistributionEquipmentUpdateUsesSlotNumberFromThePath(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	service := &fakeDistributionService{equippedSlot: distribution.DistributionSlot{ID: "slot-1", SlotNumber: 3, MachineOptionCode: "shark-spwp8030"}}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/equipment?schedule_id=schedule-1", strings.NewReader(`{"machine_option_code":"shark-spwp8030","machine_serial_number":"msn-9"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.equipmentInput.ScheduleID != "schedule-1" || service.equipmentInput.SlotNumber != 3 || service.equipmentInput.MachineOptionCode != "shark-spwp8030" || service.equipmentInput.MachineSerialNumber != "msn-9" {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.equipmentInput, rec.Body.String())
	}
}

func TestDistributionEquipmentUpdateRejectsPosMesinOnly(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/equipment?schedule_id=schedule-1", strings.NewReader(`{"machine_option_code":"shark"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: &fakeDistributionService{}, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDistributionEquipmentSerialsUpdateAllowsPosMesinAndUsesPathSlot(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_mesin": true}}
	service := &fakeDistributionService{serialsSlot: distribution.DistributionSlot{ID: "slot-1", SlotNumber: 3, MachineSerialNumber: "MESIN-9"}}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/equipment-serials?schedule_id=schedule-1", strings.NewReader(`{"machine_serial_number":"mesin-9","converter_serial_number":"konkit-8"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.serialsInput.ScheduleID != "schedule-1" || service.serialsInput.SlotNumber != 3 || service.serialsInput.MachineSerialNumber != "mesin-9" || service.serialsInput.ConverterSerialNumber != "konkit-8" {
		t.Fatalf("status=%d input=%+v body=%s", rec.Code, service.serialsInput, rec.Body.String())
	}
}

func TestDistributionRecipientAndReopenRoutes(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	service := &fakeDistributionService{recipientSlot: distribution.DistributionSlot{ID: "slot-1", Status: "linked"}, reopenedSlot: distribution.DistributionSlot{ID: "slot-1", Status: "linked", NeedsRecompletion: true}}
	for _, tt := range []struct{ method, path, body string }{
		{http.MethodPatch, "/api/v1/distribution/slots/3/recipient?schedule_id=schedule-1", `{"address":"JL. BARU"}`},
		{http.MethodPost, "/api/v1/distribution/slots/3/replace-recipient?schedule_id=schedule-1", `{"nik":"9171031707010004"}`},
		{http.MethodPost, "/api/v1/distribution/slots/3/reopen?schedule_id=schedule-1", `{"stage":"dokumen","reason":"Koreksi"}`},
	} {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
		req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
	if service.recipientInput.ScheduleID != "schedule-1" || service.recipientInput.SlotNumber != 3 || service.replaceInput.NIK != "9171031707010004" || service.reopenInput.Stage != "dokumen" {
		t.Fatalf("recipient=%+v replace=%+v reopen=%+v", service.recipientInput, service.replaceInput, service.reopenInput)
	}
}

func TestDistributionMediaPermissionRequiresMatchingPOSStage(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"documentation.manage": true}}
	service := &fakeDistributionService{documentationStage: "dokumen", media: distribution.MediaFile{ID: "media-1"}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("source", "camera")
	_ = writer.WriteField("file_size", "4")
	file, _ := writer.CreateFormFile("file", "proof.jpg")
	_, _ = file.Write([]byte{0xff, 0xd8, 0xff, 0xe0})
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/docs-1/media", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDistributionEquipmentUpdateReportsLockedEquipment(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true}}
	service := &fakeDistributionService{equipmentErr: distribution.ErrEquipmentLocked}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/slots/3/equipment?schedule_id=schedule-1", strings.NewReader(`{"machine_option_code":"shark-spwp8030"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "equipment_locked") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProtectedReadinessReturnsServiceUnavailableWhenDegraded(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowed: true}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: service, Health: fakeHealthService{status: "degraded"}}).ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestValidationErrorsUseBadRequestAndFieldDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, profile.ErrEmailInvalid)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"fields":{"email":`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthorizeAnyAcceptsUsersManagerForRoleOptions(t *testing.T) {
	service := &fakeAuthService{allowedPermissions: map[string]bool{"users.manage": true}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/role-options", nil)
	if !(&Handler{deps: Dependencies{Auth: service}}).authorizeAny(rec, req, auth.Principal{}, "roles.view", "users.manage") {
		t.Fatalf("authorization rejected: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoleOptionsReturnsRedactedRolesForUsersViewer(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"users.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/role-options", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	adminService := &fakeAdministrationService{roles: []administration.Role{{ID: "role-1", Code: "operator", Name: "Operator", Permissions: []administration.Permission{{Code: "secret.permission"}}, UserCount: 9}}}

	NewHandler(Dependencies{Auth: service, Administration: adminService}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"operator"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret.permission") || strings.Contains(rec.Body.String(), "user_count") {
		t.Fatalf("role options leaked administrative details: %s", rec.Body.String())
	}
}

func TestDashboardSummaryUsesOnlyDashboardPermission(t *testing.T) {
	service := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"dashboard.view": true}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	adminService := &fakeAdministrationService{roles: []administration.Role{{ID: "role-1"}}, counts: administration.UserCounts{Total: 5, Active: 4, Inactive: 1}}
	profiles := fakeProfileService{profile: profile.Profile{FullName: "Dashboard User", Email: "private@konkit.test"}}
	audits := &fakeAuditService{page: audit.Page{Items: []audit.Entry{{ID: "event-1", Action: "user.updated", ActorName: "Admin", IPAddress: "192.0.2.1", UserAgent: "private-agent", Metadata: map[string]any{"private": true}, CreatedAt: time.Now()}}}}

	NewHandler(Dependencies{Auth: service, Administration: adminService, Profile: profiles, Health: fakeHealthService{status: health.StatusHealthy}, Audit: audits}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active_users":4`) || !strings.Contains(rec.Body.String(), `"inactive_users":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, private := range []string{"private@konkit.test", "192.0.2.1", "private-agent", `"metadata"`} {
		if strings.Contains(rec.Body.String(), private) {
			t.Fatalf("dashboard summary leaked %q: %s", private, rec.Body.String())
		}
	}
}

func TestBAEquipmentUnavailableUsesActionableConflict(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, bast.ErrEquipmentUnavailable)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"equipment_unavailable"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestConflictErrorsIncludeIdentityFields(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, administration.ErrIdentityInUse)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"username":`) || !strings.Contains(rec.Body.String(), `"email":`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthorizeRejectsMissingPermission(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	if (&Handler{deps: Dependencies{Auth: &fakeAuthService{}}}).authorize(rec, req, auth.Principal{}, "users.view") {
		t.Fatal("authorization unexpectedly succeeded")
	}
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecodeJSONRequiresApplicationJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
	var target map[string]string
	if decodeJSON(rec, req, &target) || rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecodeJSONRejectsBodiesLargerThanOneMiB(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"data":"`+strings.Repeat("x", maxRequestBody)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	var target map[string]string
	if decodeJSON(rec, req, &target) || rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

const validSessionToken = "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio"

type fakeAuthService struct {
	principal          auth.Principal
	authenticateErr    error
	permissions        []string
	allowed            bool
	allowedPermissions map[string]bool
	regencyScope       auth.RegencyScope
	regencyScopeErr    error
}

type fakeHealthService struct{ status string }
type fakeProfileService struct{ profile profile.Profile }
type fakeAdministrationService struct {
	AdministrationService
	roles  []administration.Role
	counts administration.UserCounts
}
type fakeAuditService struct {
	AuditService
	page   audit.Page
	filter audit.Filter
}

type fakeProgramSetupService struct {
	ProgramSetupService
	regencyInput      programs.RegencyInput
	seenRegencyScope  auth.RegencyScope
	zoneListProgramID string
	zoneInput         programs.ZoneInput
	zoneResult        programs.ProgramZone
	zoneErr           error
	assignmentInput   programs.RegencyAssignmentInput
	assignmentResult  programs.ProgramZone
	assignErr         error
	scheduleInput     programs.ScheduleInput
	scheduleResult    programs.Schedule
	scheduleErr       error
}

func (f *fakeProgramSetupService) SaveSchedule(_ context.Context, _ auth.Principal, input programs.ScheduleInput, scope auth.RegencyScope, _ auth.ClientMeta) (programs.Schedule, error) {
	f.scheduleInput, f.seenRegencyScope = input, scope
	return f.scheduleResult, f.scheduleErr
}

type fakeDCP3Service struct {
	DCP3Service
	preview          dcp3.ImportPreview
	result           dcp3.ImportResult
	scheduleID       string
	filename         string
	batchID          string
	mapping          dcp3.Mapping
	seenRegencyScope auth.RegencyScope
	seenHeaderRow    int
	rawPreviewRows   [][]string
	rawPreviewErr    error
}

type fakeDistributionService struct {
	DistributionService
	createInput          distribution.CreateSlotInput
	createdSlot          distribution.DistributionSlot
	createErr            error
	candidateScheduleID  string
	candidateNIK         string
	candidate            distribution.CandidateMatch
	candidateErr         error
	suggestionScheduleID string
	suggestionPrefix     string
	suggestions          []distribution.CandidateMatch
	linkInput            distribution.LinkSlotInput
	linkedSlot           distribution.DistributionSlot
	linkErr              error
	searchScheduleID     string
	searchQuery          string
	searchedSlot         distribution.DistributionSlot
	searchErr            error
	completeInput        distribution.CompleteSlotInput
	completedSlot        distribution.DistributionSlot
	completeErr          error
	dateInput            distribution.SetDistributionDateInput
	retriedMedia         distribution.MediaFile
	retryMediaID         string
	retryMediaErr        error
	datedSlot            distribution.DistributionSlot
	equipmentInput       distribution.UpdateEquipmentInput
	equippedSlot         distribution.DistributionSlot
	equipmentErr         error
	serialsInput         distribution.UpdateEquipmentSerialsInput
	serialsSlot          distribution.DistributionSlot
	recipientInput       distribution.UpdateRecipientInput
	replaceInput         distribution.ReplaceRecipientInput
	reopenInput          distribution.ReopenSlotInput
	recipientSlot        distribution.DistributionSlot
	reopenedSlot         distribution.DistributionSlot
	documentationStage   string
	mediaStage           string
	media                distribution.MediaFile
	uploadErr            error
	mediaContent         []byte
	slotID               string
	upload               distribution.UploadMediaInput
	uploadReadErr        error
	seenRegencyScope     auth.RegencyScope
	catalogScheduleID    string
	catalog              []distribution.SlotCatalogEntry
	catalogErr           error
}

func (f *fakeDistributionService) CreateSlot(_ context.Context, _ auth.Principal, input distribution.CreateSlotInput, scope auth.RegencyScope, _ auth.ClientMeta) (distribution.DistributionSlot, error) {
	f.createInput, f.seenRegencyScope = input, scope
	return f.createdSlot, f.createErr
}
func (f *fakeDistributionService) SearchCandidate(_ context.Context, scheduleID, nik string, scope auth.RegencyScope) (distribution.CandidateMatch, error) {
	f.candidateScheduleID, f.candidateNIK, f.seenRegencyScope = scheduleID, nik, scope
	return f.candidate, f.candidateErr
}
func (f *fakeDistributionService) SuggestCandidates(_ context.Context, scheduleID, nikPrefix string, scope auth.RegencyScope) ([]distribution.CandidateMatch, error) {
	f.suggestionScheduleID, f.suggestionPrefix, f.seenRegencyScope = scheduleID, nikPrefix, scope
	return f.suggestions, nil
}
func (f *fakeDistributionService) LinkSlot(_ context.Context, _ auth.Principal, input distribution.LinkSlotInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.linkInput, f.seenRegencyScope = input, scope
	return f.linkedSlot, f.linkErr
}
func (f *fakeDistributionService) SearchSlot(_ context.Context, scheduleID, query string, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.searchScheduleID, f.searchQuery, f.seenRegencyScope = scheduleID, query, scope
	return f.searchedSlot, f.searchErr
}
func (f *fakeDistributionService) CompleteSlot(_ context.Context, _ auth.Principal, input distribution.CompleteSlotInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.completeInput, f.seenRegencyScope = input, scope
	return f.completedSlot, f.completeErr
}
func (f *fakeDistributionService) SetDistributionDate(_ context.Context, _ auth.Principal, input distribution.SetDistributionDateInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.dateInput, f.seenRegencyScope = input, scope
	return f.datedSlot, nil
}

func (f *fakeDistributionService) RetryMediaMove(_ context.Context, _ auth.Principal, mediaID string, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.MediaFile, error) {
	f.retryMediaID, f.seenRegencyScope = mediaID, scope
	return f.retriedMedia, f.retryMediaErr
}
func (f *fakeDistributionService) UpdateEquipment(_ context.Context, _ auth.Principal, input distribution.UpdateEquipmentInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.equipmentInput, f.seenRegencyScope = input, scope
	return f.equippedSlot, f.equipmentErr
}
func (f *fakeDistributionService) UpdateEquipmentSerials(_ context.Context, _ auth.Principal, input distribution.UpdateEquipmentSerialsInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.serialsInput, f.seenRegencyScope = input, scope
	return f.serialsSlot, nil
}
func (f *fakeDistributionService) UpdateRecipient(_ context.Context, _ auth.Principal, input distribution.UpdateRecipientInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.recipientInput, f.seenRegencyScope = input, scope
	return f.recipientSlot, nil
}
func (f *fakeDistributionService) ReplaceRecipient(_ context.Context, _ auth.Principal, input distribution.ReplaceRecipientInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.replaceInput, f.seenRegencyScope = input, scope
	return f.recipientSlot, nil
}
func (f *fakeDistributionService) ReopenSlot(_ context.Context, _ auth.Principal, input distribution.ReopenSlotInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.reopenInput, f.seenRegencyScope = input, scope
	return f.reopenedSlot, nil
}
func (f *fakeDistributionService) DocumentationSlotStage(_ context.Context, _ string, _ auth.RegencyScope) (string, error) {
	return f.documentationStage, nil
}
func (f *fakeDistributionService) MediaStage(_ context.Context, _ string, _ auth.RegencyScope) (string, error) {
	return f.mediaStage, nil
}
func (f *fakeDistributionService) UploadMedia(_ context.Context, _ auth.Principal, input distribution.UploadMediaInput, _ auth.ClientMeta, scope auth.RegencyScope) (distribution.MediaFile, error) {
	f.slotID, f.upload, f.seenRegencyScope = input.SlotID, input, scope
	if input.Data != nil {
		_, f.uploadReadErr = io.Copy(io.Discard, input.Data)
	}
	return f.media, f.uploadErr
}
func (f *fakeDistributionService) DeleteMedia(_ context.Context, _ auth.Principal, _ string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.seenRegencyScope = scope
	return nil
}
func (f *fakeDistributionService) OpenMedia(_ context.Context, _ string, scope auth.RegencyScope) (distribution.MediaContent, error) {
	f.seenRegencyScope = scope
	return distribution.MediaContent{Reader: io.NopCloser(bytes.NewReader(f.mediaContent)), MimeType: f.media.MimeType, Filename: f.media.OriginalFilename}, nil
}
func (f *fakeDistributionService) ListSlotCatalog(_ context.Context, scheduleID string, scope auth.RegencyScope) ([]distribution.SlotCatalogEntry, error) {
	f.catalogScheduleID, f.seenRegencyScope = scheduleID, scope
	return f.catalog, f.catalogErr
}

type fakeReportsService struct {
	ReportsService
	scheduleID       string
	filter           reports.Filter
	summary          reports.Summary
	rows             []reports.Row
	exportData       []byte
	exportFormat     string
	seenRegencyScope auth.RegencyScope
}

func (s *fakeReportsService) Summary(_ context.Context, scheduleID string, filter reports.Filter, scope auth.RegencyScope) (reports.Summary, error) {
	s.scheduleID, s.filter, s.seenRegencyScope = scheduleID, filter, scope
	return s.summary, nil
}
func (s *fakeReportsService) Rows(_ context.Context, scheduleID string, filter reports.Filter, scope auth.RegencyScope) ([]reports.Row, error) {
	s.scheduleID, s.filter, s.seenRegencyScope = scheduleID, filter, scope
	return s.rows, nil
}
func (s *fakeReportsService) ExportExcel(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat, s.seenRegencyScope = scheduleID, filter, "xlsx", scope
	return s.exportData, nil
}
func (s *fakeReportsService) ExportPDF(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta, scope auth.RegencyScope) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat, s.seenRegencyScope = scheduleID, filter, "pdf", scope
	return s.exportData, nil
}

func (f *fakeDCP3Service) Preview(_ context.Context, _ auth.Principal, scheduleID, filename string, _ io.Reader, _ auth.ClientMeta, scope auth.RegencyScope, headerRow int) (dcp3.ImportPreview, error) {
	f.scheduleID, f.filename, f.seenRegencyScope, f.seenHeaderRow = scheduleID, filename, scope, headerRow
	return f.preview, nil
}
func (f *fakeDCP3Service) RawPreview(context.Context, io.Reader) ([][]string, error) {
	return f.rawPreviewRows, f.rawPreviewErr
}
func (f *fakeDCP3Service) GetPreview(_ context.Context, _ string, scope auth.RegencyScope) (dcp3.ImportPreview, error) {
	f.seenRegencyScope = scope
	return f.preview, nil
}
func (f *fakeDCP3Service) Commit(_ context.Context, _ auth.Principal, batchID string, mapping dcp3.Mapping, _ auth.ClientMeta, scope auth.RegencyScope) (dcp3.ImportResult, error) {
	f.batchID, f.mapping, f.seenRegencyScope = batchID, mapping, scope
	return f.result, nil
}

func (f *fakeProgramSetupService) ListRegencies(_ context.Context, scope auth.RegencyScope) ([]programs.Regency, error) {
	f.seenRegencyScope = scope
	return []programs.Regency{}, nil
}
func (f *fakeProgramSetupService) SaveRegency(_ context.Context, _ auth.Principal, input programs.RegencyInput, _ auth.ClientMeta) (programs.Regency, error) {
	f.regencyInput = input
	return programs.Regency{ID: input.ID, Name: input.Name}, nil
}
func (f *fakeProgramSetupService) ListPrograms(context.Context) ([]programs.Program, error) {
	return []programs.Program{}, nil
}
func (f *fakeProgramSetupService) ListSchedules(_ context.Context, scope auth.RegencyScope) ([]programs.Schedule, error) {
	f.seenRegencyScope = scope
	return []programs.Schedule{}, nil
}
func (f *fakeProgramSetupService) ListPackageTemplates(context.Context) ([]programs.PackageTemplate, error) {
	return []programs.PackageTemplate{}, nil
}
func (f *fakeProgramSetupService) ListDocumentationTemplates(context.Context) ([]programs.DocumentationTemplate, error) {
	return []programs.DocumentationTemplate{}, nil
}

func (f *fakeProgramSetupService) ListZones(_ context.Context, programID string, scope auth.RegencyScope) ([]programs.ProgramZone, error) {
	f.zoneListProgramID, f.seenRegencyScope = programID, scope
	return []programs.ProgramZone{}, nil
}
func (f *fakeProgramSetupService) SaveZone(_ context.Context, _ auth.Principal, input programs.ZoneInput, _ auth.ClientMeta) (programs.ProgramZone, error) {
	f.zoneInput = input
	if f.zoneErr != nil {
		return programs.ProgramZone{}, f.zoneErr
	}
	return programs.ProgramZone{ID: input.ID, ProgramID: input.ProgramID, Code: input.Code, Name: input.Name}, nil
}
func (f *fakeProgramSetupService) DeleteZone(context.Context, auth.Principal, string, string, auth.ClientMeta) error {
	return nil
}

func (f *fakeProgramSetupService) AssignRegency(_ context.Context, _ auth.Principal, input programs.RegencyAssignmentInput, scope auth.RegencyScope, _ auth.ClientMeta) (programs.ProgramZone, error) {
	f.assignmentInput, f.seenRegencyScope = input, scope
	if f.assignErr != nil {
		return programs.ProgramZone{}, f.assignErr
	}
	return f.assignmentResult, nil
}

func (f *fakeAuditService) List(_ context.Context, filter audit.Filter) (audit.Page, error) {
	f.filter = filter
	return f.page, nil
}

func (f *fakeAdministrationService) ListRoles(context.Context) ([]administration.Role, error) {
	return f.roles, nil
}

func (f *fakeAdministrationService) UserCounts(context.Context) (administration.UserCounts, error) {
	return f.counts, nil
}

func (f fakeProfileService) Get(context.Context, string) (profile.Profile, error) {
	return f.profile, nil
}
func (f fakeProfileService) Update(context.Context, auth.Principal, profile.UpdateInput, auth.ClientMeta) (profile.Profile, error) {
	return f.profile, nil
}
func (f fakeProfileService) ChangePassword(context.Context, auth.Principal, string, profile.PasswordInput, auth.ClientMeta) error {
	return nil
}

func (f fakeHealthService) Check(context.Context) health.Report {
	return health.Report{Status: f.status}
}

func (f *fakeAuthService) Authenticate(context.Context, string) (auth.Principal, error) {
	return f.principal, f.authenticateErr
}

func (f *fakeAuthService) Can(_ context.Context, _ auth.Principal, permission string) (bool, error) {
	return f.allowed || f.allowedPermissions[permission], nil
}

func (f *fakeAuthService) Permissions(context.Context, auth.Principal) ([]string, error) {
	return f.permissions, nil
}

func (f *fakeAuthService) RegencyScope(context.Context, auth.Principal) (auth.RegencyScope, error) {
	return f.regencyScope, f.regencyScopeErr
}

type fakeRecipientsService struct {
	page             recipients.Page
	stats            recipients.Stats
	created          recipients.Recipient
	updated          recipients.Recipient
	createInput      recipients.CreateInput
	updateInput      recipients.UpdateInput
	allocationID     string
	seenRegencyScope auth.RegencyScope
	seenFilter       recipients.Filter
	cancelErr        error
	restoreErr       error
}

func (f *fakeRecipientsService) List(_ context.Context, filter recipients.Filter, scope auth.RegencyScope) (recipients.Page, error) {
	f.seenFilter, f.seenRegencyScope = filter, scope
	return f.page, nil
}
func (f *fakeRecipientsService) Stats(_ context.Context, filter recipients.Filter, scope auth.RegencyScope) (recipients.Stats, error) {
	f.seenFilter, f.seenRegencyScope = filter, scope
	return f.stats, nil
}
func (f *fakeRecipientsService) MapRegions(_ context.Context, filter recipients.Filter, scope auth.RegencyScope) (recipients.MapData, error) {
	f.seenFilter, f.seenRegencyScope = filter, scope
	return recipients.MapData{Regions: []recipients.MapRegion{}}, nil
}
func (f *fakeRecipientsService) Create(_ context.Context, _ auth.Principal, input recipients.CreateInput, _ auth.ClientMeta, scope auth.RegencyScope) (recipients.Recipient, error) {
	f.createInput, f.seenRegencyScope = input, scope
	return f.created, nil
}
func (f *fakeRecipientsService) Update(_ context.Context, _ auth.Principal, allocationID string, input recipients.UpdateInput, _ auth.ClientMeta, scope auth.RegencyScope) (recipients.Recipient, error) {
	f.allocationID, f.updateInput, f.seenRegencyScope = allocationID, input, scope
	return f.updated, nil
}
func (f *fakeRecipientsService) Cancel(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.cancelErr
}
func (f *fakeRecipientsService) Restore(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta, scope auth.RegencyScope) error {
	f.allocationID, f.seenRegencyScope = allocationID, scope
	return f.restoreErr
}

func TestRecipientsListRequiresViewPermissionAndForwardsFilters(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.view": true}}
	service := &fakeRecipientsService{page: recipients.Page{Page: 1, PageSize: 20, Total: 1, Items: []recipients.Recipient{{AllocationID: "allocation-1", FullName: "Siti Aminah"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients?search=Siti&page=2&page_size=50&regency_id=regency-1&program_id=program-1&zone_id=zone-1&schedule_id=schedule-1&district=Sabbangparu&allocation_status=ready&distribution_status=draft&evidence_status=partial&sort=full_name&direction=asc", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Recipients: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Siti Aminah") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenFilter.Page != 2 || service.seenFilter.PageSize != 50 || service.seenFilter.ZoneID != "zone-1" || service.seenFilter.ScheduleID != "schedule-1" || service.seenFilter.District != "Sabbangparu" || service.seenFilter.EvidenceStatus != "partial" || service.seenFilter.SortBy != "full_name" || service.seenFilter.SortDirection != "asc" {
		t.Fatalf("combined filters were not forwarded: %+v", service.seenFilter)
	}

	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{}}
	denied := httptest.NewRequest(http.MethodGet, "/api/v1/recipients", nil)
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Recipients: service}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden without recipients.view, got %d", deniedRecorder.Code)
	}
}

func TestRecipientMapAllowsDistributionViewerAndForwardsFilters(t *testing.T) {
	viewer := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"distribution.view": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	service := &fakeRecipientsService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients/map?regency_id=regency-1&program_id=program-1&schedule_id=schedule-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: viewer, Recipients: service}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenFilter.RegencyID != "regency-1" || service.seenFilter.ProgramID != "program-1" || service.seenFilter.ScheduleID != "schedule-1" {
		t.Fatalf("map filters were not forwarded: %+v", service.seenFilter)
	}
	if len(service.seenRegencyScope.RegencyIDs) != 1 || service.seenRegencyScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("map scope was not forwarded: %+v", service.seenRegencyScope)
	}

	deniedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/recipients/map", nil)
	deniedRequest.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: &fakeAuthService{principal: auth.Principal{UserID: "user-2"}}, Recipients: service}).ServeHTTP(deniedRecorder, deniedRequest)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected map to remain forbidden without a view permission, got %d", deniedRecorder.Code)
	}
}

func TestRecipientStatsForwardsTheSameCombinedFiltersAsTheList(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.view": true}}
	service := &fakeRecipientsService{stats: recipients.Stats{Total: 3}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients/stats?search=Siti&regency_id=regency-1&program_id=program-1&zone_id=zone-1&schedule_id=schedule-1&district=Sabbangparu&allocation_status=ready&distribution_status=draft&evidence_status=partial", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: viewer, Recipients: service}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || service.seenFilter.Search != "Siti" || service.seenFilter.ZoneID != "zone-1" || service.seenFilter.ScheduleID != "schedule-1" || service.seenFilter.District != "Sabbangparu" || service.seenFilter.EvidenceStatus != "partial" {
		t.Fatalf("status=%d filter=%+v body=%s", rec.Code, service.seenFilter, rec.Body.String())
	}
}

func TestRecipientsCreateRequiresManagePermission(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeRecipientsService{created: recipients.Recipient{AllocationID: "allocation-1", FullName: "Budi"}}
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.manage": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recipients", strings.NewReader(`{"schedule_id":"schedule-1","full_name":"Budi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.createInput.ScheduleID != "schedule-1" {
		t.Fatalf("status=%d schedule=%q body=%s", rec.Code, service.createInput.ScheduleID, rec.Body.String())
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}, allowedPermissions: map[string]bool{"recipients.view": true}}
	denied := httptest.NewRequest(http.MethodPost, "/api/v1/recipients", strings.NewReader(`{}`))
	denied.Header.Set("Content-Type", "application/json")
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	denied.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Recipients: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for viewer, got %d", deniedRecorder.Code)
	}
}

func TestRecipientCancelAndRestoreUseManagePermissionAndReturnConflicts(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"recipients.manage": true}}
	service := &fakeRecipientsService{cancelErr: recipients.ErrAlreadyCancelled}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recipients/allocation-1/cancel", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || service.allocationID != "allocation-1" {
		t.Fatalf("status=%d allocation=%q body=%s", rec.Code, service.allocationID, rec.Body.String())
	}

	restoreService := &fakeRecipientsService{}
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/recipients/allocation-1/restore", nil)
	restoreReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	restoreReq.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	restoreRec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Recipients: restoreService, SessionSecret: secret}).ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusNoContent || restoreService.allocationID != "allocation-1" {
		t.Fatalf("status=%d allocation=%q", restoreRec.Code, restoreService.allocationID)
	}
}
