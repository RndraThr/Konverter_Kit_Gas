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
	"konkit/internal/slotexamples"
)

type fakeDocumentationExampleService struct {
	saved   slotexamples.SaveInput
	deleted string
	opened  string
}

func (f *fakeDocumentationExampleService) List(context.Context) ([]slotexamples.Slot, error) {
	return []slotexamples.Slot{{Code: "recipient_id_card", Label: "Foto KTP", Stage: "dokumen", Example: &slotexamples.Example{Caption: "Dari depan", ImageURL: "/x"}}}, nil
}
func (f *fakeDocumentationExampleService) Save(_ context.Context, _ auth.Principal, input slotexamples.SaveInput, _ auth.ClientMeta) (slotexamples.Example, error) {
	f.saved = input
	return slotexamples.Example{Caption: input.Caption}, nil
}
func (f *fakeDocumentationExampleService) Delete(_ context.Context, _ auth.Principal, code string, _ auth.ClientMeta) error {
	f.deleted = code
	return nil
}
func (f *fakeDocumentationExampleService) Open(_ context.Context, code string) (slotexamples.Content, error) {
	f.opened = code
	return slotexamples.Content{Reader: io.NopCloser(strings.NewReader("png")), MimeType: "image/png", Filename: "ktp.png"}, nil
}

func TestDocumentationExamplesReadableByFieldOfficers(t *testing.T) {
	officer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeDocumentationExampleService{}
	for _, path := range []string{"/api/v1/documentation-examples", "/api/v1/documentation-examples/recipient_id_card/image?v=1"} {
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: officer, DocumentationExamples: service}).ServeHTTP(rec, authenticatedRequest(http.MethodGet, path, "", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if service.opened != "recipient_id_card" {
		t.Fatalf("opened=%q", service.opened)
	}
}

func TestDocumentationExampleChangesNeedManagePermission(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	officer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"distribution.view": true, "settings.manage": true}}
	service := &fakeDocumentationExampleService{}
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: officer, DocumentationExamples: service, SessionSecret: secret}).ServeHTTP(rec, authenticatedRequest(http.MethodDelete, "/api/v1/documentation-examples/recipient_id_card", "", secret))
	if rec.Code != http.StatusForbidden || service.deleted != "" {
		t.Fatalf("status=%d deleted=%q", rec.Code, service.deleted)
	}
}

func TestDocumentationExampleSaveForwardsCaptionAndPhoto(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	admin := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"documentation_examples.manage": true}}
	service := &fakeDocumentationExampleService{}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("caption", "Dari depan")
	part, _ := writer.CreateFormFile("file", "ktp.png")
	_, _ = part.Write([]byte("png-bytes"))
	_ = writer.Close()
	req := authenticatedRequest(http.MethodPut, "/api/v1/documentation-examples/recipient_id_card", "", secret)
	req.Body = io.NopCloser(&body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: admin, DocumentationExamples: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || service.saved.SlotCode != "recipient_id_card" || service.saved.Caption != "Dari depan" || string(service.saved.Image) != "png-bytes" || service.saved.OriginalFilename != "ktp.png" {
		t.Fatalf("status=%d saved=%+v body=%s", rec.Code, service.saved, rec.Body.String())
	}
}
