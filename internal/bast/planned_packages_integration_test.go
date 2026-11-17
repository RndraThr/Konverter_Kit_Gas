package bast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"konkit/internal/auth"
)

// BA Pemeriksaan is made before distribution, so its package count is the
// planned one: the schedule quota, else the active DCP3 recipients.
func TestIntegrationPlannedPackageRowsCountBeforeDistribution(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	code := freeRegencyCode(t, pool)
	var programID, regencyID, packageID, documentationID, scheduleID string
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'BA Plan Test','farmer',2026,'active') RETURNING id::text`, []any{"BAPLAN-" + suffix}, &programID)
	mustQuery(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, []any{"Plan BA " + suffix, code}, &regencyID)
	mustQuery(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer',$2::jsonb,'published') RETURNING id::text`,
		[]any{"BAPLANPKG-" + suffix, `{"machine_options":[{"code":"shark","brand":"SHARK","type":"SPWP","is_default":true},{"code":"honda","brand":"HONDA","type":"WB"}]}`}, &packageID)
	mustQuery(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, []any{"BAPLANDOC-" + suffix}, &documentationID)
	mustQuery(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,slot_quota) VALUES($1,$2,$3,$4,'Jadwal','2026-09-01','2026-09-30','active',120) RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)
	t.Cleanup(func() {
		background := context.Background()
		_, _ = pool.Exec(background, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(background, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(background, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(background, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(background, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	repository := NewRepository(pool)
	scope := auth.RegencyScope{Unrestricted: true}

	// No recipient and nothing distributed yet: the quota with the template's default machine.
	rows, total, err := repository.ListPlannedPackageRows(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if total != 120 || len(rows) != 1 || rows[0].MachineBrand != "SHARK" || rows[0].Count != 120 {
		t.Fatalf("total=%d rows=%+v", total, rows)
	}
	if dominantMachineBrand(rows) != "SHARK" {
		t.Fatalf("dominant brand = %q", dominantMachineBrand(rows))
	}

	// Without a quota the active recipients count.
	if _, err := pool.Exec(ctx, `UPDATE program_schedules SET slot_quota=NULL WHERE id=$1`, scheduleID); err != nil {
		t.Fatal(err)
	}
	if _, total, err := repository.ListPlannedPackageRows(ctx, scheduleID, scope); err != nil || total != 0 {
		t.Fatalf("no quota, no recipients: total=%d err=%v", total, err)
	}
	if _, _, err := repository.ListPlannedPackageRows(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); err != ErrNotFound {
		t.Fatalf("out of scope: %v", err)
	}
}
