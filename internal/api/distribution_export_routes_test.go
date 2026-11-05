package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

type fakeDistributionExportService struct {
	fakeDistributionService
	scheduleID  string
	slotNumbers []int
}

func (f *fakeDistributionExportService) ExportMachineSerials(_ context.Context, scheduleID string, slotNumbers []int, _ auth.RegencyScope) (distribution.ExportFile, error) {
	f.scheduleID, f.slotNumbers = scheduleID, slotNumbers
	return distribution.ExportFile{Filename: "nomor-mesin-WNG-20261104.xlsx", Data: []byte("xlsx")}, nil
}

func TestDistributionSlotExportDownloadsChosenSlots(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}}
	service := &fakeDistributionExportService{}
	req := authenticatedRequest(http.MethodPost, "/api/v1/distribution/slots/export", `{"schedule_id":"schedule-1","slot_numbers":[3,1,2]}`, secret)
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: viewer, Distribution: service, SessionSecret: secret}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "xlsx" || !strings.Contains(rec.Header().Get("Content-Disposition"), "nomor-mesin-WNG-20261104.xlsx") {
		t.Fatalf("status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if service.scheduleID != "schedule-1" || !slices.Equal(service.slotNumbers, []int{3, 1, 2}) {
		t.Fatalf("schedule=%q slots=%v", service.scheduleID, service.slotNumbers)
	}

	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{}}
	deniedRec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: denied, Distribution: service, SessionSecret: secret}).ServeHTTP(deniedRec, authenticatedRequest(http.MethodPost, "/api/v1/distribution/slots/export", `{"schedule_id":"schedule-1","slot_numbers":[1]}`, secret))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d", deniedRec.Code)
	}
}
