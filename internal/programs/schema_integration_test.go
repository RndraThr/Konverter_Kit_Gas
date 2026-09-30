package programs

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestProgramZoneSchemaEnforcesOneAssignmentPerProgramRegency(t *testing.T) {
	pool := programsIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "ZONE-SCHEMA-" + suffix
	regencyCode := schemaRegencyCode(suffix)

	var programID, regencyID, firstZoneID, secondZoneID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO programs(code,name,program_type,fiscal_year,status)
		VALUES($1,'Program Zona Schema','farmer',2026,'draft') RETURNING id::text
	`, programCode).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO regencies(province_name,name,document_code,is_active)
		VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text
	`, "Kabupaten Zona "+suffix, regencyCode).Scan(&regencyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_regency_assignments WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_zones WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM regencies WHERE id=$1`, regencyID)
	})

	if err := pool.QueryRow(ctx, `
		INSERT INTO program_zones(program_id,code,name,sort_order)
		VALUES($1,'Z1','ZONA 1',10) RETURNING id::text
	`, programID).Scan(&firstZoneID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO program_zones(program_id,code,name,sort_order)
		VALUES($1,'Z2','ZONA 2',20) RETURNING id::text
	`, programID).Scan(&secondZoneID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO program_regency_assignments(program_id,regency_id,zone_id)
		VALUES($1,$2,$3)
	`, programID, regencyID, firstZoneID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO program_regency_assignments(program_id,regency_id,zone_id)
		VALUES($1,$2,$3)
	`, programID, regencyID, secondZoneID); err == nil {
		t.Fatal("expected duplicate program/regency assignment to be rejected")
	}
}

func TestProgramDocumentProfileVersionIsUniquePerProgramVersion(t *testing.T) {
	pool := programsIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "PROFILE-SCHEMA-" + suffix

	var programID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO programs(code,name,program_type,fiscal_year,status)
		VALUES($1,'Program Profil Schema','farmer',2026,'draft') RETURNING id::text
	`, programCode).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_document_profile_versions WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_zones WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
	})

	insert := `
		INSERT INTO program_document_profile_versions(
			program_id,version,title,subtitle,procurement_description,document_series,status
		) VALUES($1,1,'BERITA ACARA SERAH TERIMA','FORM PENERIMA PAKET','Pengadaan paket perdana','KSM-KKT','draft')
	`
	if _, err := pool.Exec(ctx, insert, programID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, insert, programID); err == nil {
		t.Fatal("expected duplicate program/profile version to be rejected")
	}
}

func TestProgramDocumentProfilePublishedVersionIsImmutable(t *testing.T) {
	pool := programsIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	programCode := "PROFILE-IMMUTABLE-" + suffix

	var programID, profileID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO programs(code,name,program_type,fiscal_year,status)
		VALUES($1,'Program Profil Immutable','farmer',2026,'draft') RETURNING id::text
	`, programCode).Scan(&programID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_document_profile_versions WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_zones WHERE program_id=$1`, programID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=$1`, programID)
	})
	if err := pool.QueryRow(ctx, `
		INSERT INTO program_document_profile_versions(
			program_id,version,title,subtitle,procurement_description,document_series,status,published_at
		) VALUES($1,1,'BERITA ACARA SERAH TERIMA','FORM PENERIMA PAKET','Pengadaan paket perdana','KSM-KKT','published',now())
		RETURNING id::text
	`, programID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE program_document_profile_versions SET title='DIUBAH' WHERE id=$1`, profileID); err == nil {
		t.Fatal("expected published profile update to be rejected")
	}
}

func schemaRegencyCode(suffix string) string {
	last := len(suffix) - 1
	return fmt.Sprintf("%c%c%c", 'A'+suffix[last]%20, 'A'+suffix[last-1]%20, 'A'+suffix[last-2]%20)
}
