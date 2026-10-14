package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/activities"
	"konkit/internal/auth"
)

func (f *fakeActivitiesService) ScanUpload(_ context.Context, _ auth.Principal, input activities.ScanUploadInput, _ auth.ClientMeta, _ auth.RegencyScope) (activities.ActivityMedia, error) {
	f.listedRegencyID = input.RegencyID // reuse the field; the assertion below only needs the count and IDs
	f.patchedID = input.SlotID
	f.scannedPageCount = len(input.Pages)
	return activities.ActivityMedia{ID: "media-1", MediaType: "document", MimeType: "application/pdf"}, nil
}

const fakeJPEGBytes0 = "\xff\xd8\xff\xe0\x00\x10JFIF\x00" // minimal JPEG SOI+APP0 header, enough for http.DetectContentType in the real service — this stub never calls DetectUpload, so any non-empty bytes are fine here

func TestHandleActivitiesSupportingDocumentsScanPostsAllPages(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	officer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("program_id", "program-1")
	_ = writer.WriteField("regency_id", "regency-1")
	_ = writer.WriteField("slot_id", "slot-1")
	for i := 0; i < 3; i++ {
		part, _ := writer.CreateFormFile("page", "page.jpg")
		_, _ = part.Write([]byte(fakeJPEGBytes0))
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/supporting-documents/scan", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))

	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: officer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.listedRegencyID != "regency-1" || service.patchedID != "slot-1" || service.scannedPageCount != 3 {
		t.Fatalf("regencyID = %q, slotID = %q, pages = %d", service.listedRegencyID, service.patchedID, service.scannedPageCount)
	}
}

func TestHandleActivitiesSupportingDocumentsScanRejectsOversizedPage(t *testing.T) {
	secret := []byte(supportingSlotsTestSecret0)
	officer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"activities.manage": true}}
	service := &fakeActivitiesService{}

	oversized := bytes.Repeat([]byte("a"), int(activities.MaxScanPageBytes)+1)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("program_id", "program-1")
	_ = writer.WriteField("regency_id", "regency-1")
	_ = writer.WriteField("slot_id", "slot-1")
	part, _ := writer.CreateFormFile("page", "page.jpg")
	_, _ = part.Write(oversized)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/supporting-documents/scan", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))

	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: officer, Activities: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s, want 413", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("media_too_large")) {
		t.Fatalf("body = %s, want media_too_large error code", rec.Body.String())
	}
	if service.scannedPageCount != 0 {
		t.Fatal("ScanUpload must not be called with a truncated oversized page")
	}
}
