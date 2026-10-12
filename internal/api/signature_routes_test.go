package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

type fakeSignatureService struct {
	DistributionService
	saved   distribution.SaveSignatureInput
	deleted int
}

func (f *fakeSignatureService) SaveRecipientSignature(_ context.Context, _ auth.Principal, input distribution.SaveSignatureInput, _ auth.ClientMeta, _ auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.saved = input
	return distribution.DistributionSlot{ID: "slot-1", SlotNumber: input.SlotNumber}, nil
}

func (f *fakeSignatureService) DeleteRecipientSignature(_ context.Context, _ auth.Principal, _ string, slotNumber int, _ auth.ClientMeta, _ auth.RegencyScope) (distribution.DistributionSlot, error) {
	f.deleted = slotNumber
	return distribution.DistributionSlot{ID: "slot-1", SlotNumber: slotNumber}, nil
}

func (f *fakeSignatureService) OpenRecipientSignature(_ context.Context, _ string, _ int, _ auth.RegencyScope) (distribution.SignatureContent, error) {
	return distribution.SignatureContent{ImagePNG: []byte("\x89PNG")}, nil
}

func TestDistributionSignatureRoutes(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	dokumen := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"distribution.pos_dokumen": true, "distribution.view": true}}
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "u"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	serve := func(authService *fakeAuthService, service *fakeSignatureService, req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		NewHandler(Dependencies{Auth: authService, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
		return rec
	}
	const path = "/api/v1/distribution/slots/7/signature?schedule_id=s1"

	service := &fakeSignatureService{}
	rec := serve(dokumen, service, authenticatedRequest(http.MethodPut, path, `{"image_png":"iVBORw=="}`, secret))
	if rec.Code != http.StatusOK || service.saved.SlotNumber != 7 || service.saved.ScheduleID != "s1" || len(service.saved.ImagePNG) == 0 {
		t.Fatalf("put: status=%d saved=%+v body=%s", rec.Code, service.saved, rec.Body.String())
	}
	if rec = serve(viewer, &fakeSignatureService{}, authenticatedRequest(http.MethodPut, path, `{"image_png":"iVBORw=="}`, secret)); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer put: status=%d", rec.Code)
	}
	rec = serve(viewer, &fakeSignatureService{}, authenticatedRequest(http.MethodGet, path, "", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("get: status=%d type=%s", rec.Code, rec.Header().Get("Content-Type"))
	}
	service = &fakeSignatureService{}
	if rec = serve(dokumen, service, authenticatedRequest(http.MethodDelete, path, "", secret)); rec.Code != http.StatusOK || service.deleted != 7 {
		t.Fatalf("delete: status=%d deleted=%d", rec.Code, service.deleted)
	}
}
