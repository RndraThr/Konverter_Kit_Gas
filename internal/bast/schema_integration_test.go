package bast

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestBASTSchemaEnforcesSettingsDocumentsAndBundleVersions(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var scheduleID, programID, regencyID, packageTemplateID string
	if err := pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text,package_template_version_id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID, &programID, &regencyID, &packageTemplateID); err != nil {
		t.Fatal(err)
	}
	var profileID string
	if err := pool.QueryRow(ctx, `INSERT INTO program_document_profile_versions(program_id,version,title,procurement_description,document_series,status,published_at) VALUES($1,(SELECT COALESCE(max(version),0)+1 FROM program_document_profile_versions WHERE program_id=$1),'BAST','Pengadaan','KSM-KKT','published',now()) RETURNING id::text`, programID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	var firstSlotID, secondSlotID string
	base := int(time.Now().UnixNano()%1000000) + 1000000
	if err := pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at) VALUES($1,$2,'completed',now(),now()) RETURNING id::text`, scheduleID, base).Scan(&firstSlotID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distributed_at,completed_at) VALUES($1,$2,'completed',now(),now()) RETURNING id::text`, scheduleID, base+1).Scan(&secondSlotID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_daily_bundles WHERE program_id=$1 AND filename LIKE $2`, programID, "%"+suffix+"%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_individual_documents WHERE distribution_slot_id IN ($1,$2)`, firstSlotID, secondSlotID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_bast_settings WHERE program_id=$1 AND regency_id=$2`, programID, regencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM distribution_slots WHERE id IN ($1,$2)`, firstSlotID, secondSlotID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_document_profile_versions WHERE id=$1`, profileID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO program_regency_bast_settings(program_id,regency_id,final_total) VALUES($1,$2,$3)`, programID, regencyID, base+1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_regency_bast_settings(program_id,regency_id,final_total) VALUES($1,$2,$3)`, programID, regencyID, base+1); err == nil {
		t.Fatal("expected one setting row per program/regency")
	}

	insertDocument := func(slotID string, slotNumber int) (string, error) {
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,profile_version_id,package_template_version_id,snapshot_json,revision,status) VALUES($1,$2,$3,'2024-12-10',$4,$5,$6,$7,$8,'{}',1,'final') RETURNING id::text`, slotID, programID, regencyID, slotNumber, base+1, fmt.Sprintf("%d/%d/KSM-KKT-WJO/XII/2024", slotNumber, base+1), profileID, packageTemplateID).Scan(&id)
		return id, err
	}
	firstDocumentID, err := insertDocument(firstSlotID, base)
	if err != nil {
		t.Fatal(err)
	}
	secondDocumentID, err := insertDocument(secondSlotID, base+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,profile_version_id,package_template_version_id,snapshot_json,revision,status) VALUES($1,$2,$3,'2024-12-10',$4,$5,'duplicate',$6,$7,'{}',2,'final')`, firstSlotID, programID, regencyID, base, base+1, profileID, packageTemplateID); err == nil {
		t.Fatal("expected only one current final document per slot")
	}

	checksum := strings.Repeat("a", 64)
	var bundleID string
	if err := pool.QueryRow(ctx, `INSERT INTO bast_daily_bundles(program_id,regency_id,local_date,profile_version_id,filename,recipient_count,page_count,checksum,storage_key,version,status) VALUES($1,$2,'2024-12-10',$3,$4,2,2,$5,'bundle-key',1,'active') RETURNING id::text`, programID, regencyID, profileID, "SELASA, 10 DESEMBER 2024 "+suffix+".pdf", checksum).Scan(&bundleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bast_daily_bundles(program_id,regency_id,local_date,profile_version_id,filename,recipient_count,page_count,checksum,version,status) VALUES($1,$2,'2024-12-10',$3,$4,2,2,$5,2,'active')`, programID, regencyID, profileID, "duplicate "+suffix+".pdf", checksum); err == nil {
		t.Fatal("expected only one active bundle per date")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bast_daily_bundle_items(bundle_id,individual_document_id,item_order,page_start,page_end) VALUES($1,$2,10,2,2),($1,$3,2,1,1)`, bundleID, firstDocumentID, secondDocumentID); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT item_order FROM bast_daily_bundle_items WHERE bundle_id=$1 ORDER BY item_order`, bundleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	orders := []int{}
	for rows.Next() {
		var order int
		if err := rows.Scan(&order); err != nil {
			t.Fatal(err)
		}
		orders = append(orders, order)
	}
	if len(orders) != 2 || orders[0] != 2 || orders[1] != 10 {
		t.Fatalf("numeric order=%v", orders)
	}
}

func TestBASTSchemaGrantsManagePermissionToSuperAdmin(t *testing.T) {
	pool := bastIntegrationPool(t)
	var granted bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE r.code='super_admin' AND p.code='bast.manage')`).Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if !granted {
		t.Fatal("bast.manage is not granted to super_admin")
	}
}

func bastIntegrationPool(t *testing.T) *pgxpool.Pool {
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
