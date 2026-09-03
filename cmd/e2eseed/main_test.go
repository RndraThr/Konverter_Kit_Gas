package main

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestValidateSeedTarget(t *testing.T) {
	if err := validateSeedTarget("test", "postgres://localhost/konkit_test?sslmode=disable"); err != nil {
		t.Fatalf("valid test target rejected: %v", err)
	}
	for _, test := range []struct{ env, url string }{
		{env: "local", url: "postgres://localhost/konkit_test"},
		{env: "test", url: "postgres://localhost/konkit"},
	} {
		if err := validateSeedTarget(test.env, test.url); err == nil {
			t.Fatalf("unsafe target accepted: env=%s url=%s", test.env, test.url)
		}
	}
}

func TestParseSeedOptionsAcceptsFixtureDirectoryAndCleanup(t *testing.T) {
	options, err := parseSeedOptions([]string{"-fixture-dir", "fixtures"})
	if err != nil || options.cleanup || options.fixtureDir != "fixtures" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
	options, err = parseSeedOptions([]string{"cleanup", "-fixture-dir", "fixtures"})
	if err != nil || !options.cleanup || options.fixtureDir != "fixtures" {
		t.Fatalf("cleanup options=%+v err=%v", options, err)
	}
}

func TestWriteDCP3FixturesCreatesProjectSpecificWorkbooks(t *testing.T) {
	directory := t.TempDir()
	if err := writeDCP3Fixtures(directory); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"desktop", "mobile"} {
		workbook, err := excelize.OpenFile(filepath.Join(directory, "dcp3-"+project+".xlsx"))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := workbook.GetRows("DCP3")
		_ = workbook.Close()
		if err != nil || len(rows) != 3 || rows[0][0] != "No" || rows[1][1] != "Penerima Bersih E2E "+project || rows[2][1] != "Penerima Riwayat E2E" {
			t.Fatalf("project=%s rows=%v err=%v", project, rows, err)
		}
	}
}
