package programs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

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

	template, err := service.SavePackageTemplate(ctx, actor, PackageTemplateInput{
		TemplateCode: templateCode, Name: "Template Awal", ProgramType: ProgramFarmer,
		Values: map[string]any{
			"converter_brand": "ERGAS",
			"machine_options": []any{map[string]any{"code": "shark-spwp8030", "brand": "SHARK", "type": "SPWP 80-30/3\""}},
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
	if originalName != "Template Awal" || originalBrand != "ERGAS" {
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
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Program == nil || schedule.Program.Code != programCode || schedule.Regency == nil || schedule.Regency.DocumentCode != regencyCode {
		t.Fatalf("schedule context missing: %+v", schedule)
	}

	scheduleWithSupervisor, err := service.SaveSchedule(ctx, actor, ScheduleInput{
		ID: schedule.ID, ProgramID: program.ID, RegencyID: regency.ID, PackageTemplateVersionID: template.ID,
		DocumentationTemplateVersionID: documentationTemplateID, Name: "Tahap 1",
		StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Status: "active",
		SupervisorName: "  Andi Amrullah  ",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if scheduleWithSupervisor.SupervisorName != "Andi Amrullah" {
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
