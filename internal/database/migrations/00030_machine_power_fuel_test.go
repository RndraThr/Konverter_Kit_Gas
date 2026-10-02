package migrations

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMachinePowerFuelMigrationPreservesExistingOptions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`CREATE TEMP TABLE package_template_versions (
		template_code text NOT NULL,
		version integer NOT NULL,
		values_json jsonb NOT NULL,
		updated_at timestamptz
	)`); err != nil {
		t.Fatal(err)
	}

	fixture := `{"machine_options":[
		{"code":"shark-spwp8030","brand":"SHARK","type":"SPWP 80-30/3\""},
		{"code":"yanmar-tf65","brand":"YANMAR","type":"TF 65","notes":"custom"}
	],"components":[{"code":"manual","quantity":1}]}`
	if _, err := tx.Exec(`INSERT INTO package_template_versions(template_code,version,values_json) VALUES('KONKIT-2026',1,$1::jsonb)`, fixture); err != nil {
		t.Fatal(err)
	}

	migration, err := FS.ReadFile("00030_machine_power_fuel.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	upSQL = strings.Replace(upSQL, "-- +goose Up", "", 1)
	if _, err := tx.Exec(upSQL); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	var raw []byte
	if err := tx.QueryRow(`SELECT values_json FROM package_template_versions WHERE template_code='KONKIT-2026' AND version=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var values struct {
		MachineOptions []map[string]any `json:"machine_options"`
		Components     []map[string]any `json:"components"`
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	if len(values.MachineOptions) != 2 {
		t.Fatalf("migration must preserve both machine options, got %d: %s", len(values.MachineOptions), raw)
	}
	if values.MachineOptions[0]["power"] != "5.5 HP" || values.MachineOptions[0]["fuel_type"] != "Bensin" {
		t.Fatalf("SHARK option was not enriched: %v", values.MachineOptions[0])
	}
	if values.MachineOptions[1]["code"] != "yanmar-tf65" || values.MachineOptions[1]["notes"] != "custom" {
		t.Fatalf("custom machine option changed: %v", values.MachineOptions[1])
	}
	if len(values.Components) != 1 || values.Components[0]["code"] != "manual" {
		t.Fatalf("unrelated template values changed: %s", raw)
	}
}
