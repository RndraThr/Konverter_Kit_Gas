package programs

import (
	"context"
	"encoding/json"
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

func TestLegacyDocumentProfileSchemaIsRemoved(t *testing.T) {
	pool := programsIntegrationPool(t)
	ctx := context.Background()
	var profilesTable, logosTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.program_document_profile_versions')::text, to_regclass('public.program_document_logo_assets')::text`).Scan(&profilesTable, &logosTable); err != nil {
		t.Fatal(err)
	}
	if profilesTable != nil || logosTable != nil {
		t.Fatalf("legacy tables still exist: profiles=%v logos=%v", profilesTable, logosTable)
	}
	var legacyFunctionExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname='reject_published_program_document_profile_change')`).Scan(&legacyFunctionExists); err != nil {
		t.Fatal(err)
	}
	if legacyFunctionExists {
		t.Fatal("legacy document-profile trigger function still exists")
	}
}

func TestKonkitPackageUsesSingleCombinedHoseComponent(t *testing.T) {
	pool := programsIntegrationPool(t)
	var raw []byte
	if err := pool.QueryRow(context.Background(), `SELECT values_json->'components' FROM package_template_versions WHERE template_code='KONKIT-2026' AND version=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var components []struct {
		Code     string `json:"code"`
		Label    string `json:"label"`
		Quantity int    `json:"quantity"`
		Unit     string `json:"unit"`
	}
	if err := json.Unmarshal(raw, &components); err != nil {
		t.Fatal(err)
	}
	hoseCount := 0
	referenceDetails := 0
	for _, component := range components {
		if component.Code == "hose_clamp_accessories" && component.Label == "Selang, Clamp & Aksesorisnya" {
			hoseCount++
		}
		if component.Code == "suction_hose" || component.Code == "discharge_hose" {
			t.Fatalf("legacy split hose component still exists: %+v", component)
		}
		if component.Code == "oil" && component.Label == "OLI" && component.Quantity == 2 && component.Unit == "Ltr" {
			referenceDetails++
		}
		if component.Code == "bracket" && component.Label == "Braket" && component.Quantity == 1 && component.Unit == "Ea" {
			referenceDetails++
		}
	}
	if hoseCount != 1 {
		t.Fatalf("combined hose component count=%d components=%+v", hoseCount, components)
	}
	if referenceDetails != 2 {
		t.Fatalf("reference component details missing: %+v", components)
	}
}

func TestKonkitPackageSeparatesSuctionAndDischargeHoseDetails(t *testing.T) {
	pool := programsIntegrationPool(t)
	var raw []byte
	if err := pool.QueryRow(context.Background(), `SELECT values_json->'hose_options' FROM package_template_versions WHERE template_code='KONKIT-2026' AND version=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var options []struct {
		SuctionBrand   string `json:"suction_brand"`
		SuctionSpec    string `json:"suction_spec"`
		DischargeBrand string `json:"discharge_brand"`
		DischargeSpec  string `json:"discharge_spec"`
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].SuctionBrand != "TRILLIUNHOSE" || options[0].SuctionSpec != "6 M" || options[0].DischargeBrand != "YAMAKOYO" || options[0].DischargeSpec != "10 M" {
		t.Fatalf("hose_options=%s", raw)
	}
}

func schemaRegencyCode(suffix string) string {
	last := len(suffix) - 1
	return fmt.Sprintf("%c%c%c", 'A'+suffix[last]%20, 'A'+suffix[last-1]%20, 'A'+suffix[last-2]%20)
}
