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
	"konkit/internal/dcp3"
	"konkit/internal/distribution"
	"konkit/internal/health"
	"konkit/internal/profile"
	"konkit/internal/programs"
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dcp3/imports", strings.NewReader(`{"batch_id":"batch-1","mapping":{"source_sequence":"No","full_name":"Nama"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{Auth: authService, DCP3: dcp3Service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || dcp3Service.batchID != "batch-1" || dcp3Service.mapping.FullName != "Nama" {
		t.Fatalf("status=%d batch=%q mapping=%+v body=%s", rec.Code, dcp3Service.batchID, dcp3Service.mapping, rec.Body.String())
	}
}

func TestDistributionSearchRequiresScheduleAndReturnsOnlyMaskedIdentity(t *testing.T) {
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	distributionService := &fakeDistributionService{results: []distribution.SearchResult{{AllocationID: "allocation-1", FullName: "Siti Aminah", MaskedNIK: "7306********0001"}}}
	missing := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/search?q=Siti", nil)
	missing.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	missingRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: distributionService}).ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing schedule status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/search?schedule_id=schedule-1&q=Siti&limit=7", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: distributionService}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || distributionService.scheduleID != "schedule-1" || distributionService.limit != 7 {
		t.Fatalf("status=%d schedule=%q limit=%d body=%s", rec.Code, distributionService.scheduleID, distributionService.limit, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "7306014101900001") || !strings.Contains(rec.Body.String(), "7306********0001") {
		t.Fatalf("search identity leak: %s", rec.Body.String())
	}
}

func TestDistributionDetailAndDraftUseSeparatePermissions(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeDistributionService{workspace: distribution.RecipientWorkspace{AllocationID: "allocation-1", FullName: "Siti Aminah"}}
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/distribution/allocations/allocation-1", nil)
	getRequest.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	getRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service}).ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK || service.allocationID != "allocation-1" {
		t.Fatalf("detail status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/distribution/allocations/allocation-1/draft", strings.NewReader(`{"nik":"7306014101900001"}`))
	patchRequest.Header.Set("Content-Type", "application/json")
	patchRequest.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	patchRequest.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	patchRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusForbidden {
		t.Fatalf("draft status=%d body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}
}

func TestDistributionCompletionRequiresManagePermissionAndReturnsStableConflicts(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	service := &fakeDistributionService{completed: distribution.DistributionRecord{ID: "distribution-1", AllocationID: "allocation-1", Status: "completed"}}
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.manage": true}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/allocations/allocation-1/complete", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.allocationID != "allocation-1" || !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatalf("status=%d allocation=%q body=%s", rec.Code, service.allocationID, rec.Body.String())
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	denied := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/allocations/allocation-1/complete", nil)
	denied.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	denied.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	deniedRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service, SessionSecret: secret}).ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}

	conflicts := []struct {
		err  error
		code string
	}{{distribution.ErrIdentityIncomplete, "identity_incomplete"}, {distribution.ErrDocumentationIncomplete, "documentation_incomplete"}, {distribution.ErrPreviouslyReceived, "previously_received"}, {distribution.ErrAlreadyCompleted, "already_completed"}}
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
	service := &fakeReportsService{rows: []reports.Row{{DistributionNumber: 7, FullName: "Siti Aminah", NIK: "7306014101900001"}}}
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
	authService := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"documentation.manage": true, "distribution.view": true}}
	service := &fakeDistributionService{media: distribution.MediaFile{ID: "media-1", MimeType: "image/jpeg", OriginalFilename: "penerima.jpg"}, mediaContent: []byte("jpeg-content")}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("source", "camera")
	file, _ := writer.CreateFormFile("file", "penerima.jpg")
	_, _ = file.Write(append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 32)...))
	_ = writer.Close()
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/distribution/slots/slot-1/media", body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	upload.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	upload.Header.Set("X-CSRF-Token", auth.CSRFToken([]byte("01234567890123456789012345678901"), validSessionToken))
	uploadRecorder := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: []byte("01234567890123456789012345678901")}).ServeHTTP(uploadRecorder, upload)
	if uploadRecorder.Code != http.StatusCreated || service.slotID != "slot-1" || service.upload.Source != "camera" {
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
	page audit.Page
}

type fakeProgramSetupService struct {
	ProgramSetupService
	regencyInput programs.RegencyInput
}

type fakeDCP3Service struct {
	DCP3Service
	preview    dcp3.ImportPreview
	result     dcp3.ImportResult
	scheduleID string
	filename   string
	batchID    string
	mapping    dcp3.Mapping
}

type fakeDistributionService struct {
	DistributionService
	results      []distribution.SearchResult
	workspace    distribution.RecipientWorkspace
	scheduleID   string
	query        string
	limit        int
	allocationID string
	media        distribution.MediaFile
	mediaContent []byte
	slotID       string
	upload       distribution.UploadMediaInput
	completed    distribution.DistributionRecord
	completeErr  error
}

func (f *fakeDistributionService) Search(_ context.Context, scheduleID, query string, limit int) ([]distribution.SearchResult, error) {
	f.scheduleID, f.query, f.limit = scheduleID, query, limit
	return f.results, nil
}
func (f *fakeDistributionService) GetWorkspace(_ context.Context, allocationID string) (distribution.RecipientWorkspace, error) {
	f.allocationID = allocationID
	return f.workspace, nil
}
func (f *fakeDistributionService) SaveDraft(_ context.Context, _ auth.Principal, allocationID string, _ distribution.DraftInput, _ auth.ClientMeta) (distribution.RecipientWorkspace, error) {
	f.allocationID = allocationID
	return f.workspace, nil
}
func (f *fakeDistributionService) Complete(_ context.Context, _ auth.Principal, allocationID string, _ auth.ClientMeta) (distribution.DistributionRecord, error) {
	f.allocationID = allocationID
	return f.completed, f.completeErr
}
func (f *fakeDistributionService) UploadMedia(_ context.Context, _ auth.Principal, input distribution.UploadMediaInput, _ auth.ClientMeta) (distribution.MediaFile, error) {
	f.slotID, f.upload = input.SlotID, input
	return f.media, nil
}
func (f *fakeDistributionService) DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta) error {
	return nil
}
func (f *fakeDistributionService) OpenMedia(context.Context, string) (distribution.MediaContent, error) {
	return distribution.MediaContent{Reader: io.NopCloser(bytes.NewReader(f.mediaContent)), MimeType: f.media.MimeType, Filename: f.media.OriginalFilename}, nil
}

type fakeReportsService struct {
	ReportsService
	scheduleID   string
	filter       reports.Filter
	summary      reports.Summary
	rows         []reports.Row
	exportData   []byte
	exportFormat string
}

func (s *fakeReportsService) Summary(_ context.Context, scheduleID string, filter reports.Filter) (reports.Summary, error) {
	s.scheduleID, s.filter = scheduleID, filter
	return s.summary, nil
}
func (s *fakeReportsService) Rows(_ context.Context, scheduleID string, filter reports.Filter) ([]reports.Row, error) {
	s.scheduleID, s.filter = scheduleID, filter
	return s.rows, nil
}
func (s *fakeReportsService) ExportExcel(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat = scheduleID, filter, "xlsx"
	return s.exportData, nil
}
func (s *fakeReportsService) ExportPDF(_ context.Context, _ auth.Principal, scheduleID string, filter reports.Filter, _ auth.ClientMeta) ([]byte, error) {
	s.scheduleID, s.filter, s.exportFormat = scheduleID, filter, "pdf"
	return s.exportData, nil
}

func (f *fakeDCP3Service) Preview(_ context.Context, _ auth.Principal, scheduleID, filename string, _ io.Reader, _ auth.ClientMeta) (dcp3.ImportPreview, error) {
	f.scheduleID, f.filename = scheduleID, filename
	return f.preview, nil
}
func (f *fakeDCP3Service) GetPreview(context.Context, string) (dcp3.ImportPreview, error) {
	return f.preview, nil
}
func (f *fakeDCP3Service) Commit(_ context.Context, _ auth.Principal, batchID string, mapping dcp3.Mapping, _ auth.ClientMeta) (dcp3.ImportResult, error) {
	f.batchID, f.mapping = batchID, mapping
	return f.result, nil
}

func (f *fakeProgramSetupService) ListRegencies(context.Context) ([]programs.Regency, error) {
	return []programs.Regency{}, nil
}
func (f *fakeProgramSetupService) SaveRegency(_ context.Context, _ auth.Principal, input programs.RegencyInput, _ auth.ClientMeta) (programs.Regency, error) {
	f.regencyInput = input
	return programs.Regency{ID: input.ID, Name: input.Name}, nil
}
func (f *fakeProgramSetupService) ListPrograms(context.Context) ([]programs.Program, error) {
	return []programs.Program{}, nil
}
func (f *fakeProgramSetupService) ListSchedules(context.Context) ([]programs.Schedule, error) {
	return []programs.Schedule{}, nil
}
func (f *fakeProgramSetupService) ListPackageTemplates(context.Context) ([]programs.PackageTemplate, error) {
	return []programs.PackageTemplate{}, nil
}
func (f *fakeProgramSetupService) ListDocumentationTemplates(context.Context) ([]programs.DocumentationTemplate, error) {
	return []programs.DocumentationTemplate{}, nil
}

func (f *fakeAuditService) List(context.Context, audit.Filter) (audit.Page, error) {
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
