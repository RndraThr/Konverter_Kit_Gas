package bast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"
)

// Every location field is optional in the BA settings form; leaving one empty must save, not hit a
// NOT NULL column (rakorda_location used to be sent as NULL).
func TestScheduleSettingsSaveWithEmptyLocations(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	code := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	var programID, regencyID, packageID, documentationID, scheduleID string
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'BA Settings Test','farmer',2026,'active') RETURNING id::text`, []any{"BASET-" + suffix}, &programID)
	mustQuery(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, []any{"Settings BA " + suffix, code}, &regencyID)
	mustQuery(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer','{}','published') RETURNING id::text`, []any{"BASETPKG-" + suffix}, &packageID)
	mustQuery(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, []any{"BASETDOC-" + suffix}, &documentationID)
	mustQuery(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,'Jadwal','2026-09-01','2026-09-30','active') RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)
	t.Cleanup(func() {
		background := context.Background()
		_, _ = pool.Exec(background, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(background, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(background, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(background, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(background, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})

	service := NewScheduleSettingsService(NewRepository(pool))
	settings, err := service.Put(ctx, auth.Principal{}, ScheduleSettingsInput{ScheduleID: scheduleID}, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("save with empty locations: %v", err)
	}
	if settings.RakordaLocation != "" || settings.SosialisasiLocation != "" {
		t.Fatalf("locations = %q, %q; want empty", settings.RakordaLocation, settings.SosialisasiLocation)
	}
}
