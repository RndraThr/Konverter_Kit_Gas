package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type fakeBASTScheduleSettingsService struct {
	putInput bast.ScheduleSettingsInput
}

func (f *fakeBASTScheduleSettingsService) Get(context.Context, string, auth.RegencyScope) (bast.ScheduleSettings, error) {
	return bast.ScheduleSettings{}, nil
}

func (f *fakeBASTScheduleSettingsService) Put(_ context.Context, _ auth.Principal, input bast.ScheduleSettingsInput, _ auth.RegencyScope, _ auth.ClientMeta) (bast.ScheduleSettings, error) {
	f.putInput = input
	return bast.ScheduleSettings{
		ScheduleID:            input.ScheduleID,
		HandoverLocation:      input.HandoverLocation,
		ConsultantCompanyName: input.ConsultantCompanyName,
		AgricultureOfficeName: input.AgricultureOfficeName,
		AgricultureOfficeNIP:  input.AgricultureOfficeNIP,
		InstallerName:         input.InstallerName,
		SupervisorName:        input.SupervisorName,
		PertaminaRepName:      input.PertaminaRepName,
	}, nil
}

func TestBASTScheduleSettingsPutAcceptsFrontendJSONFields(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	authService := &fakeAuthService{
		principal:          auth.Principal{UserID: "user-1"},
		allowedPermissions: map[string]bool{"bast.manage": true},
		regencyScope:       auth.RegencyScope{RegencyIDs: []string{"regency-1"}},
	}
	settingsService := &fakeBASTScheduleSettingsService{}
	body := `{
		"schedule_id":"stale-client-value",
		"handover_location":"Gudang Kabupaten",
		"consultant_company_name":"PT Konsultan Distribusi",
		"agriculture_office_name":"Ibu Kepala Dinas",
		"agriculture_office_nip":"198001012010011001",
		"installer_name":"Bapak Pelaksana",
		"supervisor_name":"Ibu Pengawas",
		"pertamina_rep_name":"Bapak Pertamina"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/bast/schedules/schedule-1/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	req.Header.Set("X-CSRF-Token", auth.CSRFToken(secret, validSessionToken))
	rec := httptest.NewRecorder()

	NewHandler(Dependencies{
		Auth:          authService,
		BASTSettings:  settingsService,
		SessionSecret: secret,
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if settingsService.putInput.ScheduleID != "schedule-1" ||
		settingsService.putInput.HandoverLocation != "Gudang Kabupaten" ||
		settingsService.putInput.ConsultantCompanyName != "PT Konsultan Distribusi" ||
		settingsService.putInput.AgricultureOfficeName != "Ibu Kepala Dinas" ||
		settingsService.putInput.AgricultureOfficeNIP != "198001012010011001" ||
		settingsService.putInput.InstallerName != "Bapak Pelaksana" ||
		settingsService.putInput.SupervisorName != "Ibu Pengawas" ||
		settingsService.putInput.PertaminaRepName != "Bapak Pertamina" {
		t.Fatalf("frontend fields were not decoded: %+v", settingsService.putInput)
	}
	if !strings.Contains(rec.Body.String(), `"handover_location":"Gudang Kabupaten"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}
