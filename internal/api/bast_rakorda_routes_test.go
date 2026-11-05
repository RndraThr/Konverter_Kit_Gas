package api

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type fakeRakordaService struct {
	RakordaService
	uploaded bast.RakordaUploadInput
}

func (f *fakeRakordaService) Upload(_ context.Context, _ auth.Principal, input bast.RakordaUploadInput, _ auth.RegencyScope, _ auth.ClientMeta) (bast.RakordaUpload, error) {
	f.uploaded = input
	return bast.RakordaUpload{ID: "upload-1"}, nil
}

func scannedPageJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 210, 297))
	for y := 0; y < 297; y++ {
		for x := 0; x < 210; x++ {
			img.Set(x, y, color.RGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRakordaUploadBindsScannedPagesIntoOnePDF(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manager := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"bast.manage": true}}
	service := &fakeRakordaService{}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("schedule_id", "schedule-1")
	_ = form.WriteField("date", "2026-11-05")
	for _, name := range []string{"page-1.jpg", "page-2.jpg"} {
		part, err := form.CreateFormFile("page", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(scannedPageJPEG(t))
	}
	_ = form.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bast/rakorda/uploads", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: manager, Rakorda: service, SessionSecret: secret}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.HasPrefix(service.uploaded.Data, []byte("%PDF")) || service.uploaded.ScheduleID != "schedule-1" || service.uploaded.EventDate != "2026-11-05" {
		t.Fatalf("uploaded=%q schedule=%q date=%q", service.uploaded.Data[:min(8, len(service.uploaded.Data))], service.uploaded.ScheduleID, service.uploaded.EventDate)
	}
	if pages := bytes.Count(service.uploaded.Data, []byte("/Type /Page")) - bytes.Count(service.uploaded.Data, []byte("/Type /Pages")); pages != 2 {
		t.Fatalf("PDF pages = %d, want 2", pages)
	}
}
