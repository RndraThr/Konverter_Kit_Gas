package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/config"
	"konkit/internal/database"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
)

const (
	seedUsername    = "e2e.admin"
	seedEmail       = "e2e.admin@konkit.test"
	seedPassword    = "Konkit-E2E-Password-2026"
	seedProgramCode = "E2E-PETANI-2026"
)

type seedOptions struct {
	cleanup    bool
	fixtureDir string
}

func main() {
	options, err := parseSeedOptions(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := validateSeedTarget(cfg.Env, cfg.DatabaseURL); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := cleanupE2E(ctx, tx); err != nil {
		log.Fatal(err)
	}
	if options.cleanup {
		if err := tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
		if options.fixtureDir != "" {
			_ = os.RemoveAll(options.fixtureDir)
		}
		_, _ = fmt.Fprintln(os.Stdout, "E2E data removed")
		return
	}
	if err := seedE2E(ctx, tx); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	if options.fixtureDir != "" {
		if err := writeDCP3Fixtures(options.fixtureDir); err != nil {
			log.Fatal(err)
		}
	}
	_, _ = fmt.Fprintln(os.Stdout, "E2E account, schedules, and DCP3 fixtures ready")
}

func parseSeedOptions(arguments []string) (seedOptions, error) {
	var result seedOptions
	for index := 0; index < len(arguments); index++ {
		switch argument := arguments[index]; {
		case argument == "cleanup":
			result.cleanup = true
		case argument == "-fixture-dir":
			index++
			if index >= len(arguments) || strings.TrimSpace(arguments[index]) == "" {
				return seedOptions{}, fmt.Errorf("-fixture-dir requires a path")
			}
			result.fixtureDir = arguments[index]
		case strings.HasPrefix(argument, "-fixture-dir="):
			result.fixtureDir = strings.TrimPrefix(argument, "-fixture-dir=")
			if strings.TrimSpace(result.fixtureDir) == "" {
				return seedOptions{}, fmt.Errorf("-fixture-dir requires a path")
			}
		default:
			return seedOptions{}, fmt.Errorf("usage: go run ./cmd/e2eseed [cleanup] [-fixture-dir PATH]")
		}
	}
	return result, nil
}

func seedE2E(ctx context.Context, tx pgx.Tx) error {
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		return err
	}
	var userID string
	if err := tx.QueryRow(ctx, `INSERT INTO users(full_name,username,email,password_hash,is_active) VALUES('Admin E2E',$1,$2,$3,true) RETURNING id::text`, seedUsername, seedEmail, hash).Scan(&userID); err != nil {
		return fmt.Errorf("seed e2e user: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='super_admin'`, userID); err != nil {
		return fmt.Errorf("seed e2e role: %w", err)
	}

	var packageID, documentationID string
	var packageSnapshot []byte
	if err := tx.QueryRow(ctx, `SELECT id::text,values_json FROM package_template_versions WHERE template_code='PETANI-LPG' AND version=1`).Scan(&packageID, &packageSnapshot); err != nil {
		return fmt.Errorf("find e2e package template: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM documentation_template_versions WHERE template_code='DOK-PETANI' AND version=1`).Scan(&documentationID); err != nil {
		return fmt.Errorf("find e2e documentation template: %w", err)
	}
	var regencyID, programID string
	if err := tx.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,notes) VALUES('Sulawesi Selatan','Wajo E2E','EEW','Playwright fixture') RETURNING id::text`).Scan(&regencyID); err != nil {
		return fmt.Errorf("seed e2e regency: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status,notes) VALUES($1,'Program Petani E2E 2026','farmer',2026,'active','Playwright fixture') RETURNING id::text`, seedProgramCode).Scan(&programID); err != nil {
		return fmt.Errorf("seed e2e program: %w", err)
	}
	insertSchedule := func(name, status string) (string, error) {
		var id string
		err := tx.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,notes) VALUES($1,$2,$3,$4,$5,'2026-09-01','2026-09-30',$6,'Playwright fixture') RETURNING id::text`, programID, regencyID, packageID, documentationID, name, status).Scan(&id)
		return id, err
	}
	for _, project := range []string{"desktop", "mobile"} {
		if _, err := insertSchedule("E2E Wajo "+project, "active"); err != nil {
			return fmt.Errorf("seed %s schedule: %w", project, err)
		}
	}
	historyScheduleID, err := insertSchedule("E2E Riwayat 2025", "completed")
	if err != nil {
		return fmt.Errorf("seed history schedule: %w", err)
	}

	var personID, batchID, rowID, nominationID, allocationID string
	if err := tx.QueryRow(ctx, `INSERT INTO people(full_name,nik,address,village,district,phone_number,verification_status) VALUES('Penerima Riwayat E2E','9100000000000099','Jalan Riwayat','Tempe','Sabbangparu','081200000099','verified') RETURNING id::text`).Scan(&personID); err != nil {
		return fmt.Errorf("seed prior recipient: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value,verified_at) VALUES($1,'farmer_card','E2EPRIOR','E2E-PRIOR',now())`, personID); err != nil {
		return fmt.Errorf("seed prior recipient card: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,status,total_rows,valid_rows,imported_at) VALUES($1,'e2e-history.xlsx',$2,'DCP3','imported',1,1,'2025-09-10') RETURNING id::text`, historyScheduleID, strings.Repeat("e", 64)).Scan(&batchID); err != nil {
		return fmt.Errorf("seed history batch: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,source_sequence_number,raw_data_json,normalized_data_json,validation_status) VALUES($1,2,1,'{"Nama":"Penerima Riwayat E2E"}','{"full_name":"Penerima Riwayat E2E"}','valid') RETURNING id::text`, batchID).Scan(&rowID); err != nil {
		return fmt.Errorf("seed history row: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO candidate_nominations(batch_id,import_row_id,person_id,program_type,source_snapshot_json,status) VALUES($1,$2,$3,'farmer','{"Nama":"Penerima Riwayat E2E"}','ready') RETURNING id::text`, batchID, rowID, personID).Scan(&nominationID); err != nil {
		return fmt.Errorf("seed history nomination: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,actual_recipient_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$3,1,'distributed',$4) RETURNING id::text`, historyScheduleID, nominationID, personID, packageSnapshot).Scan(&allocationID); err != nil {
		return fmt.Errorf("seed history allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id,status,verification_snapshot_json,distributed_at,completed_at) VALUES($1,$2,'completed','{"fixture":"e2e"}','2025-09-10T09:00:00Z','2025-09-10T09:00:00Z')`, allocationID, personID); err != nil {
		return fmt.Errorf("seed history distribution: %w", err)
	}
	return nil
}

func cleanupE2E(ctx context.Context, tx pgx.Tx) error {
	type cleanupStatement struct {
		query string
		args  []any
	}
	statements := []cleanupStatement{
		{`DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE username=$1)`, []any{seedUsername}},
		{`DELETE FROM eligibility_checks WHERE schedule_id IN (SELECT ps.id FROM program_schedules ps JOIN programs p ON p.id=ps.program_id WHERE p.code=$1) OR person_id IN (SELECT id FROM people WHERE full_name LIKE '% E2E%')`, []any{seedProgramCode}},
		{`DELETE FROM distribution_records WHERE allocation_id IN (SELECT a.id FROM package_allocations a JOIN program_schedules ps ON ps.id=a.schedule_id JOIN programs p ON p.id=ps.program_id WHERE p.code=$1)`, []any{seedProgramCode}},
		{`DELETE FROM package_allocations WHERE schedule_id IN (SELECT ps.id FROM program_schedules ps JOIN programs p ON p.id=ps.program_id WHERE p.code=$1)`, []any{seedProgramCode}},
		{`DELETE FROM candidate_nominations WHERE batch_id IN (SELECT b.id FROM dcp3_import_batches b JOIN program_schedules ps ON ps.id=b.schedule_id JOIN programs p ON p.id=ps.program_id WHERE p.code=$1)`, []any{seedProgramCode}},
		{`DELETE FROM dcp3_import_batches WHERE schedule_id IN (SELECT ps.id FROM program_schedules ps JOIN programs p ON p.id=ps.program_id WHERE p.code=$1)`, []any{seedProgramCode}},
		{`DELETE FROM program_schedules WHERE program_id IN (SELECT id FROM programs WHERE code=$1)`, []any{seedProgramCode}},
		{`DELETE FROM person_sector_identifiers WHERE person_id IN (SELECT id FROM people WHERE full_name LIKE '% E2E%')`, nil},
		{`DELETE FROM people WHERE full_name LIKE '% E2E%'`, nil},
		{`DELETE FROM programs WHERE code=$1`, []any{seedProgramCode}},
		{`DELETE FROM regencies WHERE document_code='EEW'`, nil},
		{`DELETE FROM users WHERE username LIKE 'petugas.e2e.%' OR username=$1`, []any{seedUsername}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("clean e2e data: %w", err)
		}
	}
	return nil
}

func writeDCP3Fixtures(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create e2e fixture directory: %w", err)
	}
	for index, project := range []string{"desktop", "mobile"} {
		workbook := excelize.NewFile()
		defaultSheet := workbook.GetSheetName(0)
		if defaultSheet != "DCP3" {
			if err := workbook.SetSheetName(defaultSheet, "DCP3"); err != nil {
				return err
			}
		}
		rows := [][]any{
			{"No", "Nama", "NIK", "No Kartu Petani", "Alamat", "Desa", "Kecamatan", "No HP"},
			{1, "Penerima Bersih E2E " + project, fmt.Sprintf("910000000000000%d", index+1), "E2E-" + strings.ToUpper(project), "Jalan Sawah", "Tempe", "Sabbangparu", ""},
			{2, "Penerima Riwayat E2E", "9100000000000099", "E2E-PRIOR", "Jalan Riwayat", "Tempe", "Sabbangparu", "081200000099"},
		}
		for rowIndex, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, rowIndex+1)
			if err := workbook.SetSheetRow("DCP3", cell, &row); err != nil {
				return err
			}
		}
		if err := workbook.SetColWidth("DCP3", "A", "H", 22); err != nil {
			return err
		}
		path := filepath.Join(directory, "dcp3-"+project+".xlsx")
		if err := workbook.SaveAs(path); err != nil {
			return fmt.Errorf("write %s fixture: %w", project, err)
		}
		if err := workbook.Close(); err != nil {
			return err
		}
	}
	return nil
}

func validateSeedTarget(environment, databaseURL string) error {
	if environment != "test" {
		return fmt.Errorf("e2e seed requires APP_ENV=test")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || strings.TrimPrefix(parsed.Path, "/") != "konkit_test" {
		return fmt.Errorf("e2e seed requires the konkit_test database")
	}
	return nil
}
