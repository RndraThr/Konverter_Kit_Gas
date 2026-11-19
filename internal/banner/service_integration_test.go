package banner

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
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

func bannerIntegrationPool(t *testing.T) *pgxpool.Pool {
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

// A schedule's banners take the kabupaten's master service center for the
// machine brand, keep typed overrides, and use the uploaded logo as is.
func TestIntegrationBannerSettingsLogoAndRender(t *testing.T) {
	pool := bannerIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	regencyName := "KAB. BANNER " + suffix[len(suffix)-6:]
	var code string
	for range 100 {
		code = string([]byte{byte('A' + rand.IntN(26)), byte('A' + rand.IntN(26)), byte('A' + rand.IntN(26))})
		var taken bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM regencies WHERE document_code = $1)`, code).Scan(&taken); err != nil {
			t.Fatal(err)
		}
		if !taken {
			break
		}
	}
	var programID, regencyID, packageID, documentationID, scheduleID string
	must := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Banner Test','farmer',2026,'active') RETURNING id::text`, []any{"BANNER-" + suffix}, &programID)
	must(`INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, []any{regencyName, code}, &regencyID)
	must(`INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket','farmer',$2::jsonb,'published') RETURNING id::text`,
		[]any{"BANNERPKG-" + suffix, `{"machine_options":[{"code":"loncin","brand":"LONCIN","is_default":true}]}`}, &packageID)
	must(`INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok','farmer','published') RETURNING id::text`, []any{"BANNERDOC-" + suffix}, &documentationID)
	must(`INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status) VALUES($1,$2,$3,$4,'Jadwal Banner','2026-09-01','2026-09-30','active') RETURNING id::text`, []any{programID, regencyID, packageID, documentationID}, &scheduleID)
	if _, err := pool.Exec(ctx, `INSERT INTO banner_service_centers(regency_name,brand,sort_order,name,address,phone) VALUES($1,'shark',1,'Bengkel Shark','Jl. Shark','0811'),($1,'loncin',1,'Bengkel Loncin','Jl. Loncin 1','0812'),($1,'loncin',2,'Bengkel Kedua','Jl. Dua','0813')`, regencyName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		background := context.Background()
		_, _ = pool.Exec(background, `DELETE FROM audit_logs WHERE resource_id IN ($1, $2)`, scheduleID, regencyID)
		_, _ = pool.Exec(background, `DELETE FROM banner_service_centers WHERE regency_name = $1`, regencyName)
		_, _ = pool.Exec(background, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(background, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(background, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(background, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(background, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	service := NewService(pool)
	scope := auth.RegencyScope{Unrestricted: true}

	overview, err := service.Overview(ctx, scheduleID, scope)
	if err != nil {
		t.Fatal(err)
	}
	// The package's default machine is Loncin, so its first Loncin center is used.
	if overview.Brand != "loncin" || overview.ServiceCenter.Name != "Bengkel Loncin" || overview.HasLogo || !overview.Supported || len(overview.Kinds) != 4 {
		t.Fatalf("overview = %+v", overview)
	}
	if _, err := service.Overview(ctx, scheduleID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope: %v", err)
	}

	saved, err := service.SaveSettings(ctx, auth.Principal{}, scheduleID, Settings{
		ServiceCenterName: "Bengkel Loncin", ServiceCenterPhone: "0899-0000-1111",
		SeremoniName: "  ir.  lamhot  sinaga ", PlakatName: "Dr. Andi", PlakatDate: "2026-11-29",
	}, scope, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	// A value equal to the master data is not stored, so master changes still apply.
	if saved.Settings.ServiceCenterName != "" || saved.ServiceCenter.Phone != "0899-0000-1111" || saved.ServiceCenter.Address != "Jl. Loncin 1" ||
		saved.Settings.SeremoniName != "ir. lamhot sinaga" || saved.Settings.PlakatDate != "2026-11-29" {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := service.SaveSettings(ctx, auth.Principal{}, scheduleID, Settings{PlakatDate: "29-11-2026"}, scope, auth.ClientMeta{}); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("bad date: %v", err)
	}

	if _, err := service.SaveLogo(ctx, auth.Principal{}, scheduleID, "logo.txt", []byte("not an image"), scope, auth.ClientMeta{}); !errors.Is(err, ErrInvalidLogo) {
		t.Fatalf("text logo: %v", err)
	}
	logo := testLogo(t)
	if overview, err = service.SaveLogo(ctx, auth.Principal{}, scheduleID, "logo.png", logo, scope, auth.ClientMeta{}); err != nil || !overview.HasLogo {
		t.Fatalf("logo: %+v %v", overview, err)
	}
	stored, mime, err := service.Logo(ctx, scheduleID, scope)
	if err != nil || mime != "image/png" || !bytes.Equal(stored, logo) {
		t.Fatalf("stored logo changed: mime=%s err=%v", mime, err)
	}

	pdf, filename, err := service.Banner(ctx, scheduleID, KindPlakat, scope)
	if err != nil || len(pdf) == 0 || filename != "PLAKAT PETANI "+RegencyDisplayNameUpper(regencyName)+".pdf" {
		t.Fatalf("plakat: %d bytes %q %v", len(pdf), filename, err)
	}
	if _, _, err := service.Banner(ctx, scheduleID, Kind("spanduk"), scope); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown kind: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE programs SET program_type='fisherman' WHERE id=$1`, programID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Banner(ctx, scheduleID, KindBackdrop, scope); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("fisherman: %v", err)
	}
}

// RegencyDisplayNameUpper is the regency part of a banner file name.
func RegencyDisplayNameUpper(name string) string {
	return string(bytes.ToUpper([]byte(RegencyDisplayName(name))))
}
