package programs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestMigrationSeedsKonkit2026ZonesAndScheduleQuotas(t *testing.T) {
	pool := programsIntegrationPool(t)
	ctx := context.Background()

	type zoneSummary struct {
		Code      string
		Regencies []string
		Quota     int
	}
	want := []zoneSummary{
		{
			Code: "ZONA-1",
			Regencies: []string{
				"LNG", "BRN", "SBG", "DLS", "TDT", "SJJ", "PDP", "IRH", "IRL", "SIK", "RHL", "KPR",
				"CLP", "BMS", "GRB", "BLR", "WNG", "BBS", "TGL", "JPR", "KBM", "PBG", "PML",
			},
			Quota: 4470,
		},
		{
			Code: "ZONA-2",
			Regencies: []string{
				"OKI", "OKU", "OKT", "TJT", "MJB", "JMB", "KRC", "MRG", "LGS", "LGT", "LTM", "BKA", "BKB",
				"JBR", "LMJ", "PCT", "PNG", "BKL", "NGJ", "MJK", "TBN", "BJN", "MLG",
			},
			Quota: 4770,
		},
	}

	for _, expected := range want {
		rows, err := pool.Query(ctx, `
			SELECT r.document_code, s.slot_quota
			FROM programs p
			JOIN program_zones z ON z.program_id = p.id
			JOIN program_regency_assignments a ON a.program_id = p.id AND a.zone_id = z.id
			JOIN regencies r ON r.id = a.regency_id
			JOIN program_schedules s ON s.program_id = p.id AND s.regency_id = r.id
			WHERE p.code = 'KONKIT-2026' AND z.code = $1
			ORDER BY array_position($2::text[], r.document_code)
		`, expected.Code, expected.Regencies)
		if err != nil {
			t.Fatal(err)
		}

		var gotCodes []string
		gotQuota := 0
		for rows.Next() {
			var code string
			var quota int
			if err := rows.Scan(&code, &quota); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			gotCodes = append(gotCodes, code)
			gotQuota += quota
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotCodes, expected.Regencies) {
			t.Fatalf("%s regencies = %v, want %v", expected.Code, gotCodes, expected.Regencies)
		}
		if gotQuota != expected.Quota {
			t.Fatalf("%s quota = %d, want %d", expected.Code, gotQuota, expected.Quota)
		}
	}
}

func TestIntegrationRepositoryPersistsProgramSetupAndVersionsPublishedTemplate(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "TEST-" + suffix
	templateCode := "TEST-PKG-" + suffix
	regencyCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-integration-test"}

	regency, err := service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Test " + suffix,
		DocumentCode: regencyCode, IsActive: true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE user_agent = $1", meta.UserAgent)
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_schedules WHERE program_id IN (SELECT id FROM programs WHERE code = $1)", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM programs WHERE code = $1", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM package_template_versions WHERE template_code = $1", templateCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regency.ID)
	})

	_, err = service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Lain " + suffix,
		DocumentCode: regencyCode, IsActive: true,
	}, meta)
	if !errors.Is(err, ErrDocumentCodeInUse) {
		t.Fatalf("expected ErrDocumentCodeInUse, got %v", err)
	}

	program, err := service.SaveProgram(ctx, actor, ProgramInput{
		Code: programCode, Name: "Program Test", ProgramType: ProgramFarmer,
		FiscalYear: 2026, Status: "active",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	var placeholderCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM program_zones WHERE program_id=$1 AND code='UNASSIGNED' AND is_placeholder=true`, program.ID).Scan(&placeholderCount); err != nil {
		t.Fatal(err)
	}
	if placeholderCount != 1 {
		t.Fatalf("new program placeholder count=%d, want 1", placeholderCount)
	}

	template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		TemplateCode: templateCode, Name: "Template Awal", ProgramType: ProgramFarmer,
		Values: map[string]any{
			"converter_brand": "ERGAS",
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\"", "power": "5.5 HP", "fuel_type": "Bensin"}},
			"hose_options":    []any{map[string]any{"code": "triliunhose", "brand": "TRILIUNHOSE", "spec": "6m/10m"}},
		},
		Status: "published",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if template.Version != 1 || template.Status != "published" {
		t.Fatalf("unexpected first template: %+v", template)
	}

	next, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		ID: template.ID, TemplateCode: templateCode, Name: "Template Revisi",
		ProgramType: ProgramFarmer, Values: map[string]any{"converter_brand": "ERGAS 2"}, Status: "draft",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == template.ID || next.Version != 2 || next.Status != "draft" {
		t.Fatalf("published template was not versioned: old=%+v next=%+v", template, next)
	}
	var originalName, originalBrand string
	if err := pool.QueryRow(ctx, `SELECT name, values_json->>'converter_brand' FROM package_template_versions WHERE id = $1`, template.ID).Scan(&originalName, &originalBrand); err != nil {
		t.Fatal(err)
	}
	if originalName != "TEMPLATE AWAL" || originalBrand != "ERGAS" {
		t.Fatalf("published version mutated: name=%q brand=%q", originalName, originalBrand)
	}

	var documentationTemplateID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1`).Scan(&documentationTemplateID); err != nil {
		t.Fatal(err)
	}
	schedule, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap 1",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Program == nil || schedule.Program.Code != programCode || schedule.Regency == nil || schedule.Regency.DocumentCode != regencyCode {
		t.Fatalf("schedule context missing: %+v", schedule)
	}
	if _, err := service.ResolveStorageContext(ctx, program.ID, regency.ID, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrZoneNotConfigured) {
		t.Fatalf("new schedule must auto-assign its regency to placeholder zone, got %v", err)
	}

	scheduleWithSupervisor, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ID: schedule.ID, ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap 1",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
		SupervisorName: "  Andi Amrullah  ",
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if scheduleWithSupervisor.SupervisorName != "ANDI AMRULLAH" {
		t.Fatalf("supervisor name=%q", scheduleWithSupervisor.SupervisorName)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE user_agent = $1 AND action LIKE 'program_setup.%'`, meta.UserAgent).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 5 {
		t.Fatalf("expected at least 5 program setup audit events, got %d", auditCount)
	}
}

// TestSaveScheduleRoundTripsSlotQuota proves slot_quota survives INSERT, UPDATE, and re-read through
// ListSchedules against the live schema — not just that the query compiles.
func TestSaveScheduleRoundTripsSlotQuota(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "QTA-" + suffix
	templateCode := "QTA-PKG-" + suffix
	regencyCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-quota-test"}

	regency, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Kuota " + suffix, DocumentCode: regencyCode, IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	program, err := service.SaveProgram(ctx, actor, ProgramInput{Code: programCode, Name: "Program Kuota", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "active"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		TemplateCode: templateCode, Name: "Template Kuota", ProgramType: ProgramFarmer,
		Values: map[string]any{"machine_options": []any{map[string]any{"code": "m", "brand": "M", "type": "T", "power": "P", "fuel_type": "F"}}, "hose_options": []any{map[string]any{"code": "h", "brand": "H", "spec": "S"}}},
		Status: "published",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	var documentationTemplateID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1`).Scan(&documentationTemplateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_schedules WHERE program_id IN (SELECT id FROM programs WHERE code = $1)", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM programs WHERE code = $1", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM package_template_versions WHERE template_code = $1", templateCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regency.ID)
	})

	quota := 46
	created, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap Kuota",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		Status: "active", SlotQuota: &quota,
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if created.SlotQuota == nil || *created.SlotQuota != 46 {
		t.Fatalf("created.SlotQuota = %v, want 46", created.SlotQuota)
	}

	raised := 60
	updated, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ID: created.ID, ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap Kuota",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		Status: "active", SlotQuota: &raised,
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SlotQuota == nil || *updated.SlotQuota != 60 {
		t.Fatalf("updated.SlotQuota = %v, want 60", updated.SlotQuota)
	}

	list, err := service.ListSchedules(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range list {
		if item.ID == created.ID {
			found = true
			if item.SlotQuota == nil || *item.SlotQuota != 60 {
				t.Fatalf("listed item.SlotQuota = %v, want 60", item.SlotQuota)
			}
		}
	}
	if !found {
		t.Fatalf("schedule %s not found in ListSchedules result", created.ID)
	}
}

// TestIntegrationSaveScheduleUpdateRejectsHijackOfOutOfScopeExistingRecord proves the repository's
// UPDATE WHERE clause — not just the service-level ErrRegencyOutOfScope check on the new value —
// blocks an id guessed for a schedule whose *current* regency sits outside the caller's scope, even
// when the attacker's new regency_id value is legitimately their own (so the service-level check on
// the new value alone would pass).
func TestIntegrationSaveScheduleUpdateRejectsHijackOfOutOfScopeExistingRecord(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "HJK-" + suffix
	templateCode := "HJK-PKG-" + suffix
	victimCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	attackerCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-4]%20, 'A'+suffix[len(suffix)-5]%20, 'A'+suffix[len(suffix)-6]%20)
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-hijack-test"}

	victimRegency, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Korban " + suffix, DocumentCode: victimCode, IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	attackerRegency, err := service.SaveRegency(ctx, actor, RegencyInput{ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Penyerang " + suffix, DocumentCode: attackerCode, IsActive: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	program, err := service.SaveProgram(ctx, actor, ProgramInput{Code: programCode, Name: "Program Hijack", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "active"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		TemplateCode: templateCode, Name: "Template Hijack", ProgramType: ProgramFarmer,
		Values: map[string]any{"machine_options": []any{map[string]any{"code": "m", "brand": "M", "type": "T", "power": "P", "fuel_type": "F"}}, "hose_options": []any{map[string]any{"code": "h", "brand": "H", "spec": "S"}}},
		Status: "published",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	var documentationTemplateID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code = 'DOK-PETANI' AND version = 1`).Scan(&documentationTemplateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_schedules WHERE program_id IN (SELECT id FROM programs WHERE code = $1)", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM programs WHERE code = $1", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM package_template_versions WHERE template_code = $1", templateCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = ANY($1)", []string{victimRegency.ID, attackerRegency.ID})
	})

	// Victim's schedule, created with unrestricted scope (simulating a different officer / admin).
	created, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ProgramID: program.ID, RegencyID: victimRegency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap Korban",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}

	// Attacker is scoped only to their own (legitimate) regency, so the service-level
	// scope.Allows(input.RegencyID) check alone would pass here — the new value IS theirs. Only the
	// repository's WHERE clause on the row's *current* regency_id can still catch this.
	attackerScope := auth.RegencyScope{RegencyIDs: []string{attackerRegency.ID}}
	_, err = service.SaveSchedule(ctx, actor, ScheduleInput{
		ID: created.ID, ProgramID: program.ID, RegencyID: attackerRegency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Dibajak",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
	}, attackerScope, meta)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, want ErrNotFound (repository-level guard on the row's current regency_id)", err)
	}

	list, err := service.ListSchedules(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.ID == created.ID && (item.Name != "TAHAP KORBAN" || item.RegencyID != victimRegency.ID) {
			t.Fatalf("schedule was hijacked despite rejected scope: %+v", item)
		}
	}
}

func TestIntegrationListRegenciesAndSchedulesRespectRegencyScope(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-scope-integration-test-" + suffix}
	actor := auth.Principal{}

	codeA := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	codeB := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-4]%20, 'A'+suffix[len(suffix)-5]%20, 'A'+suffix[len(suffix)-6]%20)

	regencyA, err := service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Scope A " + suffix,
		DocumentCode: codeA, IsActive: true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	regencyB, err := service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Scope B " + suffix,
		DocumentCode: codeB, IsActive: true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE user_agent = $1", meta.UserAgent)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = ANY($1)", []string{regencyA.ID, regencyB.ID})
	})

	scoped, err := service.ListRegencies(ctx, auth.RegencyScope{RegencyIDs: []string{regencyA.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !containsRegencyID(scoped, regencyA.ID) || containsRegencyID(scoped, regencyB.ID) {
		t.Fatalf("scoped listing should only include regencyA: %+v", scoped)
	}

	unrestricted, err := service.ListRegencies(ctx, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsRegencyID(unrestricted, regencyA.ID) || !containsRegencyID(unrestricted, regencyB.ID) {
		t.Fatalf("unrestricted listing should include both regencies: %+v", unrestricted)
	}

	empty, err := service.ListRegencies(ctx, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if containsRegencyID(empty, regencyA.ID) || containsRegencyID(empty, regencyB.ID) {
		t.Fatalf("empty scope should exclude both regencies: %+v", empty)
	}
}

func TestIntegrationZoneLifecycleUsesIDForUpdates(t *testing.T) {
	pool := programsIntegrationPool(t)
	repository := NewRepository(pool)
	service := NewService(repository)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "ZNE-" + suffix
	regencyCode := fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-1]%20, 'A'+suffix[len(suffix)-2]%20, 'A'+suffix[len(suffix)-3]%20)
	actor := auth.Principal{}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "programs-zone-integration-test-" + suffix}

	program, err := service.SaveProgram(ctx, actor, ProgramInput{
		Code: programCode, Name: "Program Zona", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "active",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	otherProgram, err := service.SaveProgram(ctx, actor, ProgramInput{
		Code: "ZNE-OTHER-" + suffix, Name: "Program Zona Lain", ProgramType: ProgramFarmer, FiscalYear: 2026, Status: "active",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	regency, err := service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Zona " + suffix, DocumentCode: regencyCode, IsActive: true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE user_agent = $1", meta.UserAgent)
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_regency_assignments WHERE program_id IN (SELECT id FROM programs WHERE code = $1)", programCode)
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_zones WHERE program_id IN (SELECT id FROM programs WHERE code = ANY($1))", []string{programCode, "ZNE-OTHER-" + suffix})
		_, _ = pool.Exec(context.Background(), "DELETE FROM programs WHERE code = ANY($1)", []string{programCode, "ZNE-OTHER-" + suffix})
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regency.ID)
	})

	// Reject blank names and unstable codes before hitting the database.
	if _, err := service.SaveZone(ctx, actor, ZoneInput{ProgramID: program.ID, Code: "ZONE-A", Name: "   "}, meta); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name err=%v", err)
	}
	if _, err := service.SaveZone(ctx, actor, ZoneInput{ProgramID: program.ID, Code: "zone a!", Name: "Zona A"}, meta); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unstable code err=%v", err)
	}

	zone, err := service.SaveZone(ctx, actor, ZoneInput{ProgramID: program.ID, Code: "zone-a", Name: " Zona A ", SortOrder: 1}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if zone.Code != "ZONE-A" || zone.Name != "ZONA A" || zone.IsPlaceholder {
		t.Fatalf("unexpected saved zone: %+v", zone)
	}

	// PATCH identity is authoritative: changing the code must update this exact row,
	// not insert a second zone selected only from the submitted code.
	updated, err := service.SaveZone(ctx, actor, ZoneInput{ID: zone.ID, ProgramID: program.ID, Code: "zone-a-revisi", Name: "Zona A Revisi", SortOrder: 5}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != zone.ID || updated.Name != "ZONA A REVISI" || updated.SortOrder != 5 {
		t.Fatalf("expected upsert of existing zone, got: %+v", updated)
	}

	// A zone belonging to another program cannot be assigned to a regency under this program.
	otherZone, err := service.SaveZone(ctx, actor, ZoneInput{ProgramID: otherProgram.ID, Code: "ZONE-X", Name: "Zona Lain"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignRegency(ctx, actor, RegencyAssignmentInput{
		ProgramID: program.ID, RegencyID: regency.ID, ZoneID: otherZone.ID,
	}, auth.RegencyScope{Unrestricted: true}, meta); !errors.Is(err, ErrZoneProgramMismatch) {
		t.Fatalf("expected ErrZoneProgramMismatch, got %v", err)
	}

	// An out-of-scope regency must not be assignable.
	if _, err := service.AssignRegency(ctx, actor, RegencyAssignmentInput{
		ProgramID: program.ID, RegencyID: regency.ID, ZoneID: updated.ID,
	}, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for out-of-scope regency, got %v", err)
	}

	assigned, err := service.AssignRegency(ctx, actor, RegencyAssignmentInput{
		ProgramID: program.ID, RegencyID: regency.ID, ZoneID: updated.ID,
	}, auth.RegencyScope{Unrestricted: true}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.ID != updated.ID {
		t.Fatalf("expected assignment to resolve to zone %s, got %+v", updated.ID, assigned)
	}

	zones, err := service.ListZones(ctx, program.ID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, z := range zones {
		if z.ID == updated.ID {
			found = true
			if len(z.Regencies) != 1 || z.Regencies[0].ID != regency.ID {
				t.Fatalf("expected zone to list assigned regency, got %+v", z.Regencies)
			}
		}
	}
	if !found {
		t.Fatalf("zone %s not found in ListZones result", updated.ID)
	}

	// Regencies outside scope must not be visible on the zone listing either.
	scopedZones, err := service.ListZones(ctx, program.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, z := range scopedZones {
		if z.ID == updated.ID && len(z.Regencies) != 0 {
			t.Fatalf("expected no regencies visible out of scope, got %+v", z.Regencies)
		}
	}

	// Every newly-created program owns one protected placeholder zone.
	var placeholderID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM program_zones WHERE program_id=$1 AND code='UNASSIGNED' AND is_placeholder=true`, program.ID).Scan(&placeholderID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveZone(ctx, actor, ZoneInput{ID: placeholderID, ProgramID: program.ID, Code: "UNASSIGNED", Name: "Coba Timpa"}, meta); !errors.Is(err, ErrZonePlaceholderImmutable) {
		t.Fatalf("expected ErrZonePlaceholderImmutable, got %v", err)
	}

	// ResolveStorageContext must surface ErrZoneNotConfigured while the regency sits on the placeholder.
	regency2, err := service.SaveRegency(ctx, actor, RegencyInput{
		ProvinceName: "Sulawesi Selatan", Name: "Kabupaten Zona Dua " + suffix,
		DocumentCode: fmt.Sprintf("%c%c%c", 'A'+suffix[len(suffix)-4]%20, 'A'+suffix[len(suffix)-5]%20, 'A'+suffix[len(suffix)-6]%20),
		IsActive:     true,
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM program_regency_assignments WHERE regency_id = $1", regency2.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM regencies WHERE id = $1", regency2.ID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO program_regency_assignments (program_id, regency_id, zone_id) VALUES ($1,$2,$3)`, program.ID, regency2.ID, placeholderID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveStorageContext(ctx, program.ID, regency2.ID, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrZoneNotConfigured) {
		t.Fatalf("expected ErrZoneNotConfigured, got %v", err)
	}

	// Resolving through the real assignment returns a full storage context.
	resolved, err := service.ResolveStorageContext(ctx, program.ID, regency.ID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProgramID != program.ID || resolved.ZoneID != updated.ID || resolved.RegencyID != regency.ID || resolved.ProgramType != ProgramFarmer {
		t.Fatalf("unexpected resolved storage context: %+v", resolved)
	}

	// Out-of-scope resolution must not leak the assignment.
	if _, err := service.ResolveStorageContext(ctx, program.ID, regency.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for out-of-scope resolution, got %v", err)
	}
}

func containsRegencyID(regencies []Regency, id string) bool {
	for _, regency := range regencies {
		if regency.ID == id {
			return true
		}
	}
	return false
}

func programsIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require konkit_test, got %q", config.ConnConfig.Database)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
