package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
)

const supportingSlotsTestSecret0 = "01234567890123456789012345678901"

func TestHandleSupportingDocumentSlotsListRequiresActivitiesView(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	noPerm := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{}}
	service := &fakeActivitiesService{}
	req := authenticatedRequest(http.MethodGet, "/api/v1/activities/supporting-documents/slots?regency_id=regency-1", "", nil)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: noPerm, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if service.listedRegencyID != "" {
		t.Fatal("ListSlots must not be called without activities.view")
	}

	viewer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.view": true}}
	rec = httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.listedRegencyID != "regency-1" {
		t.Fatalf("status = %d, listedRegencyID = %q", rec.Code, service.listedRegencyID)
	}
}

func TestHandleSupportingDocumentSlotsCreateRequiresProgramsManage(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	fieldOfficer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}
	req := authenticatedRequest(http.MethodPost, "/api/v1/activities/supporting-documents/slots", `{"regency_id":"regency-1","name":"Surat Tugas"}`, secret)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: fieldOfficer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (activities.manage alone must not grant slot CRUD)", rec.Code)
	}
	if service.createdName != "" {
		t.Fatal("CreateSlot must not be called without programs.manage")
	}

	admin := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"programs.manage": true}}
	req = authenticatedRequest(http.MethodPost, "/api/v1/activities/supporting-documents/slots", `{"regency_id":"regency-1","name":"Surat Tugas"}`, secret)
	rec = httptest.NewRecorder()
	NewHandler(Dependencies{Auth: admin, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || service.createdName != "Surat Tugas" {
		t.Fatalf("status = %d, createdName = %q", rec.Code, service.createdName)
	}
}
