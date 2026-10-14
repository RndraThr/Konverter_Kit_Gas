# Laporan Akhir Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Laporan Akhir feature — a live-data presentation per Program + Kabupaten, viewable in-browser ("Present" mode) and exportable as a versioned `.pptx` saved directly inside the kabupaten's Drive folder (sibling to the 3 existing category folders).

**Architecture:** New backend domain `internal/finalreport` (models/repository/service, no Go-package imports of sibling domains — cross-domain reads go through its own repository's SQL, same pattern `bast` already uses against `program_schedules`). PPTX generation substitutes placeholder tokens into a pre-built master template (plain ZIP + XML text/image swap via Go stdlib, no new dependency). Frontend adds a new page with a 4-photo-gallery picker, a small settings form, a "Tampilkan" (Present) full-screen slideshow, and a "Generate PPTX" action with version history.

**Tech Stack:** Go (pgx/v5, goose migrations, net/http, `archive/zip`, `encoding/xml`), React 19/TS (Vite, TanStack Query), existing `media.Storage`/`auth.RegencyScope` infra.

**Spec:** `docs/superpowers/specs/2026-10-15-laporan-akhir-design.md`

## Global Constraints

- Permissions: `final_report.view` (read/Present), `final_report.manage` (settings/photo-picks/generate) — granted to `super_admin` by default, scoped via the existing `auth.RegencyScope` mechanism, same as every other domain.
- No new Go dependencies — PPTX substitution uses only `archive/zip` and `encoding/xml` from stdlib.
- Max 6 photo picks per `slide_key` (enforced in service, not DB constraint).
- `final_report_photo_picks.activity_media_id` has `ON DELETE CASCADE` to `activity_media(id)` — a pick silently disappears when its source photo is deleted, no cleanup job needed.
- Slides whose source data is empty render the placeholder `"Belum diisi"` — Preview/Present/Generate never fail because of incomplete data.
- v1 scope decision (not in the original spec table, decided here because no existing endpoint aggregates distribution photos per schedule): the `pendistribusian` slide (slide 8) ships **without** a photo picker — it still renders its live text fields (lokasi, tanggal, total paket). Photo support for that slide is a documented follow-up, not built in this plan.
- v1 scope decision: `final_report_settings` is extended beyond "susunan tim" to also hold the 3 delivery dates (Selang Buang / Mesin Pompa group / Tabung Gas) and the machine-manufacturer name — none of these exist anywhere else in the system, so they follow the same "input sendiri di page ini saja" rule the user already approved for susunan tim.
- `fiscal_year` is read from `programs.fiscal_year` (not `program_schedules`, which has no such column).
- When more than one `program_schedules` row matches `(program_id, regency_id)` (not DB-enforced unique, though seed data keeps it 1:1), the repository picks `ORDER BY (status='active') DESC, start_date DESC LIMIT 1`.

## Review Focus

- A kabupaten with zero Rakorda/Sosialisasi/distribution data generates a PPTX with "Belum diisi" placeholders, not a 500 — pinned in Task 8's and Task 10's tests.
- A photo pick referencing `activity_media` from a **different** regency than the one being reported on must be rejected by the service, not silently embedded — pinned in Task 7.
- Two near-simultaneous "Generate PPTX" clicks for the same program+kabupaten must produce two cleanly sequential versions, not a version collision — pinned in Task 10 (advisory lock, same pattern as `bast_aggregate_documents`).
- A user with `final_report.view` but not `final_report.manage` must get 403 from `PUT /settings`, `PUT /photo-picks`, and `POST /generate` even when called directly (not just hidden in the UI) — pinned in Task 11.
- Kabupaten/program names containing characters invalid for a Drive filename must be sanitized by `BuildFinalReportPath`, not crash or silently produce a broken name — pinned in Task 2.

---

### Task 1: Migration — tables + permissions

**Files:**
- Create: `internal/database/migrations/00071_final_report.sql`
- Test: `internal/database/migrations/00071_final_report_test.go`

**Interfaces:**
- Produces: tables `final_report_settings`, `final_report_photo_picks`, `final_report_documents`; permissions `final_report.view`, `final_report.manage`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE final_report_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    leader_count integer NOT NULL DEFAULT 0 CHECK (leader_count >= 0),
    admin_count integer NOT NULL DEFAULT 0 CHECK (admin_count >= 0),
    technician_count integer NOT NULL DEFAULT 0 CHECK (technician_count >= 0),
    helper_count integer NOT NULL DEFAULT 0 CHECK (helper_count >= 0),
    machine_manufacturer_name text NOT NULL DEFAULT '',
    delivery_hose_date date,
    delivery_machine_group_date date,
    delivery_gas_cylinder_date date,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (program_id, regency_id)
);

CREATE TABLE final_report_photo_picks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    slide_key text NOT NULL CHECK (slide_key IN ('pengiriman_penerimaan', 'rapat_koordinasi', 'sosialisasi')),
    activity_media_id uuid NOT NULL REFERENCES activity_media(id) ON DELETE CASCADE,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, regency_id, slide_key, activity_media_id)
);
CREATE INDEX idx_final_report_photo_picks_lookup ON final_report_photo_picks(program_id, regency_id, slide_key, sort_order);

CREATE TABLE final_report_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    filename text NOT NULL,
    checksum char(64) NOT NULL,
    storage_key text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded')),
    finalized_at timestamptz NOT NULL DEFAULT now(),
    finalized_by uuid REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (program_id, regency_id, version)
);
CREATE INDEX idx_final_report_documents_active ON final_report_documents(program_id, regency_id) WHERE status = 'active';

INSERT INTO permissions (code, name, description) VALUES
    ('final_report.view', 'Lihat Laporan Akhir', 'Melihat dan menampilkan presentasi Laporan Akhir per kabupaten'),
    ('final_report.manage', 'Kelola Laporan Akhir', 'Mengatur susunan tim, memilih foto, dan membuat PPTX Laporan Akhir')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('final_report.view', 'final_report.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM permissions WHERE code IN ('final_report.view', 'final_report.manage');
DROP TABLE final_report_documents;
DROP TABLE final_report_photo_picks;
DROP TABLE final_report_settings;
```

- [ ] **Step 2: Write the migration test**

```go
package migrations

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestFinalReportMigrationCreatesExpectedSchema(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	for _, table := range []string{"final_report_settings", "final_report_photo_picks", "final_report_documents"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	var permCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE code IN ('final_report.view','final_report.manage')`).Scan(&permCount); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permCount != 2 {
		t.Fatalf("expected 2 final_report permissions, got %d", permCount)
	}
}

var _ = goose.Up // keep goose imported for parity with sibling migration tests
```

- [ ] **Step 3: Run the migration against the real test DB**

Run: `go run ./cmd/migrate up` (with `TEST_DATABASE_URL`/`DATABASE_URL` pointed at your local Postgres)
Expected: migration `00071_final_report` applies with no error.

- [ ] **Step 4: Run the test**

Run: `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable go test ./internal/database/migrations/... -run TestFinalReportMigration -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/00071_final_report.sql internal/database/migrations/00071_final_report_test.go
git commit -m "feat(database): add final_report tables and permissions"
```

---

### Task 2: `BuildFinalReportPath` — Drive folder placement

**Files:**
- Modify: `internal/media/folder_path.go`
- Test: `internal/media/folder_path_test.go`

**Interfaces:**
- Produces: `func BuildFinalReportPath(input FolderPathInput) ([]string, error)` — returns `[]string{programSegment, zoneSegment, regencySegment}`, ignoring `input.Category`/`input.Child`.

- [ ] **Step 1: Write the failing test**

Append to `internal/media/folder_path_test.go`:

```go
func TestBuildFinalReportPathOmitsCategory(t *testing.T) {
	path, err := BuildFinalReportPath(FolderPathInput{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Wajo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"PETANI", "ZONA 1", "KABUPATEN WAJO"}
	if len(path) != len(want) {
		t.Fatalf("got %v, want %v", path, want)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("got %v, want %v", path, want)
		}
	}
}

func TestBuildFinalReportPathRejectsInvalidSegment(t *testing.T) {
	if _, err := BuildFinalReportPath(FolderPathInput{ProgramType: "farmer", ZoneName: "Zona/1", RegencyName: "Kabupaten Wajo"}); !errors.Is(err, ErrInvalidFolderPath) {
		t.Fatalf("expected ErrInvalidFolderPath, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/media/... -run TestBuildFinalReportPath -v`
Expected: FAIL with "undefined: BuildFinalReportPath"

- [ ] **Step 3: Implement**

Append to `internal/media/folder_path.go`:

```go
// BuildFinalReportPath constructs the storage path for a Final Report PPTX:
// directly inside the regency folder, as a sibling of the 3 category
// folders, not nested under any of them.
func BuildFinalReportPath(input FolderPathInput) ([]string, error) {
	programSegment, err := programTypeSegment(input.ProgramType)
	if err != nil {
		return nil, err
	}
	zoneSegment, err := sanitizeFolderSegment(input.ZoneName)
	if err != nil {
		return nil, err
	}
	if zoneSegment == "" {
		return nil, ErrInvalidFolderPath
	}
	regencySegment, err := sanitizeFolderSegment(input.RegencyName)
	if err != nil {
		return nil, err
	}
	if regencySegment == "" {
		return nil, ErrInvalidFolderPath
	}
	return []string{programSegment, zoneSegment, regencySegment}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/media/... -run TestBuildFinalReportPath -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/media/folder_path.go internal/media/folder_path_test.go
git commit -m "feat(media): add BuildFinalReportPath for sibling-to-category Drive placement"
```

---

### Task 3: Build the master PPTX template from the reference file

**Files:**
- Create: `tmp/build_final_report_template.go` (`//go:build ignore`, deleted after running — same throwaway-script pattern already used in this repo for one-off data fixes)
- Create (generated output, committed): `internal/finalreport/template/final_report_template.pptx`
- Create: `internal/finalreport/template/placeholders.go`

**Interfaces:**
- Produces: `const` placeholder tokens in `placeholders.go`, consumed by Task 9's substitution engine; the committed template file, consumed by Task 9's tests and Task 10's `Generate`.

The user supplied a reference file (`LAPORAN KEGIATAN PETANI WAJO.pptx`) with real data already filled in. This task bakes placeholder tokens into a copy of that file, once, by exact literal-string replacement — no PowerPoint needed, since the exact original strings are already known from inspecting the file.

- [ ] **Step 1: Define the placeholder tokens**

```go
// internal/finalreport/template/placeholders.go
package template

// Placeholder tokens baked into final_report_template.pptx's text runs.
// Task 9's substitution engine replaces every occurrence of each token with
// the corresponding live value before saving/presenting.
const (
	TokenRegencyLabel            = "{{REGENCY_LABEL}}"             // "KABUPATEN WAJO"
	TokenFiscalYear              = "{{FISCAL_YEAR}}"
	TokenAgricultureOfficeName   = "{{AGRICULTURE_OFFICE_NAME}}"
	TokenConsultantCompanyName   = "{{CONSULTANT_COMPANY_NAME}}"
	TokenMachineManufacturerName = "{{MACHINE_MANUFACTURER_NAME}}"
	TokenDeliveryHoseDate        = "{{DELIVERY_HOSE_DATE}}"
	TokenDeliveryMachineGroupDate = "{{DELIVERY_MACHINE_GROUP_DATE}}"
	TokenDeliveryGasCylinderDate = "{{DELIVERY_GAS_CYLINDER_DATE}}"
	TokenDeliveryRangeStart      = "{{DELIVERY_RANGE_START}}"
	TokenDeliveryRangeEnd        = "{{DELIVERY_RANGE_END}}"
	TokenRakordaDate             = "{{RAKORDA_DATE}}"
	TokenSosialisasiDate         = "{{SOSIALISASI_DATE}}"
	TokenLeaderCount             = "{{LEADER_COUNT}}"
	TokenAdminCount              = "{{ADMIN_COUNT}}"
	TokenTechnicianCount         = "{{TECHNICIAN_COUNT}}"
	TokenHelperCount             = "{{HELPER_COUNT}}"
	TokenHandoverLocation        = "{{HANDOVER_LOCATION}}"
	TokenDistributionStartDate   = "{{DISTRIBUTION_START_DATE}}"
	TokenDistributionEndDate     = "{{DISTRIBUTION_END_DATE}}"
	TokenDistributionTotal       = "{{DISTRIBUTION_TOTAL}}"
)

// AllTokens lists every token above, used by Task 9's tests to assert the
// committed template actually contains each one at least once.
var AllTokens = []string{
	TokenRegencyLabel, TokenFiscalYear, TokenAgricultureOfficeName, TokenConsultantCompanyName,
	TokenMachineManufacturerName, TokenDeliveryHoseDate, TokenDeliveryMachineGroupDate,
	TokenDeliveryGasCylinderDate, TokenDeliveryRangeStart, TokenDeliveryRangeEnd, TokenRakordaDate,
	TokenSosialisasiDate, TokenLeaderCount, TokenAdminCount, TokenTechnicianCount, TokenHelperCount,
	TokenHandoverLocation, TokenDistributionStartDate, TokenDistributionEndDate, TokenDistributionTotal,
}
```

- [ ] **Step 2: Write the one-time conversion script**

```go
//go:build ignore

// Run once: go run tmp/build_final_report_template.go
// Reads the user-supplied reference PPTX, replaces every literal string that
// carries one kabupaten's specific data with its placeholder token, and
// writes the result to internal/finalreport/template/final_report_template.pptx.
// Delete this file after running it — its output is the lasting artifact.
package main

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var replacements = []struct{ old, new string }{
	{"KABUPATEN WAJO", "{{REGENCY_LABEL}}"},
	{"TAHUN ANGGARAN 2024", "TAHUN ANGGARAN {{FISCAL_YEAR}}"},
	{"Dinas Pertanian dan Ketahanan Pangan Sumedang", "{{AGRICULTURE_OFFICE_NAME}}"},
	{"Dinas Pertanian dan Ketahanan Pangan Wajo", "{{AGRICULTURE_OFFICE_NAME}}"},
	{"Tanggal 21 November 2024 (Selang Buang)", "Tanggal {{DELIVERY_HOSE_DATE}} (Selang Buang)"},
	{"Tanggal 22 November (Mesin Pompa), (Konkit), (Oli), (Bracket), (Name Plate) dan (Selang Hisap)", "Tanggal {{DELIVERY_MACHINE_GROUP_DATE}} (Mesin Pompa), (Konkit), (Oli), (Bracket), (Name Plate) dan (Selang Hisap)"},
	{"Tanggal 28 November 2024 (Tabung Gas 3 Kg) ", "Tanggal {{DELIVERY_GAS_CYLINDER_DATE}} (Tabung Gas 3 Kg) "},
	{"25 November 2024", "{{RAKORDA_DATE}}"},
	{"KSO Multi karya (Konsultan Pengawas)", "{{CONSULTANT_COMPANY_NAME}} (Konsultan Pengawas)"},
	{"KSO Multi Karya (Konsultan Pengawas)", "{{CONSULTANT_COMPANY_NAME}} (Konsultan Pengawas)"},
	{"PT Sharpindo Dinamika Prima (Pabrikan Mesin SHARK)", "{{MACHINE_MANUFACTURER_NAME}} (Pabrikan Mesin)"},
	{"09 Desember 2024", "{{SOSIALISASI_DATE}}"},
	{"Leader (1 orang)", "Leader ({{LEADER_COUNT}} orang)"},
	{"Administrasi (2 orang)", "Administrasi ({{ADMIN_COUNT}} orang)"},
	{"Teknisi (2 orang)", "Teknisi ({{TECHNICIAN_COUNT}} orang)"},
	{"Helper (2 orang)", "Helper ({{HELPER_COUNT}} orang)"},
	{"Gudang Desa Patila, Kec. Pammana, Kab. Wajo ", "{{HANDOVER_LOCATION}} "},
	{"Gudang Desa Patila, Kec. Pammana, Kab. Wajo", "{{HANDOVER_LOCATION}}"},
	{"Gudang Desa Patila", "{{HANDOVER_LOCATION}}"},
	{"Tanggal Mulai 30 November 2024", "Tanggal Mulai {{DISTRIBUTION_START_DATE}}"},
	{"Tanggal Selesai 20 Desember 2024", "Tanggal Selesai {{DISTRIBUTION_END_DATE}}"},
	{"Total Paket 2275 Unit", "Total Paket {{DISTRIBUTION_TOTAL}} Unit"},
	{"Tanggal 21 November s.d. 18 Desember 2024", "Tanggal {{DELIVERY_RANGE_START}} s.d. {{DELIVERY_RANGE_END}}"},
}

func main() {
	src := `C:\Users\Rendra\Downloads\LAPORAN KEGIATAN PETANI WAJO.pptx`
	dst := filepath.Join("internal", "finalreport", "template", "final_report_template.pptx")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		log.Fatal(err)
	}
	reader, err := zip.OpenReader(src)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	out, err := os.Create(dst)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	writer := zip.NewWriter(out)
	defer writer.Close()

	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			log.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Fatal(err)
		}
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			text := string(content)
			for _, r := range replacements {
				text = strings.ReplaceAll(text, r.old, r.new)
			}
			content = []byte(text)
		}
		header, err := zip.FileInfoHeader(file.FileInfo())
		if err != nil {
			log.Fatal(err)
		}
		header.Name = file.Name
		header.Method = zip.Deflate
		w, err := writer.CreateHeader(header)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("wrote", dst)
}
```

- [ ] **Step 3: Run the script**

Run: `go run tmp/build_final_report_template.go`
Expected: prints `wrote internal/finalreport/template/final_report_template.pptx`; the file exists and opens in PowerPoint/LibreOffice with every dynamic value visibly replaced by its `{{TOKEN}}` text.

- [ ] **Step 4: Verify manually, then delete the throwaway script**

Run: `rm tmp/build_final_report_template.go`

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/template/final_report_template.pptx internal/finalreport/template/placeholders.go
git commit -m "feat(finalreport): add master PPTX template with placeholder tokens"
```

---

### Task 4: `internal/finalreport` models + Settings repository

**Files:**
- Create: `internal/finalreport/models.go`
- Create: `internal/finalreport/repository.go`
- Test: `internal/finalreport/repository_integration_test.go`

**Interfaces:**
- Consumes: `auth.RegencyScope`, `auth.Principal`, `auth.ClientMeta` from `konkit/internal/auth`.
- Produces: `type Settings struct{...}`, `func (r *Repository) GetSettings(ctx, programID, regencyID string, scope auth.RegencyScope) (Settings, error)`, `func (r *Repository) PutSettings(ctx, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error)` — consumed by Task 7's service.

- [ ] **Step 1: Write models.go**

```go
package finalreport

import (
	"errors"
	"time"
)

var (
	ErrProgramRequired = errors.New("program_id is required")
	ErrRegencyRequired = errors.New("regency_id is required")
	ErrNotFound         = errors.New("final report resource not found")
	ErrCountNegative    = errors.New("team counts must not be negative")
	ErrTooManyPicks     = errors.New("at most 6 photos may be picked per slide")
	ErrSlideKeyInvalid  = errors.New("slide_key is not recognized")
	ErrPhotoOutOfScope     = errors.New("photo does not belong to this program/regency")
	ErrRegencyOutOfScope   = errors.New("regency is outside the caller's assigned scope")
	ErrGenerateConflict    = errors.New("final report document was generated by another operator; reload")
)

const (
	SlideKeyPengirimanPenerimaan = "pengiriman_penerimaan"
	SlideKeyRapatKoordinasi      = "rapat_koordinasi"
	SlideKeySosialisasi          = "sosialisasi"
)

var validSlideKeys = map[string]bool{
	SlideKeyPengirimanPenerimaan: true,
	SlideKeyRapatKoordinasi:      true,
	SlideKeySosialisasi:          true,
}

// Settings is the per-program-per-regency input that has no other home in
// the system (susunan tim, delivery dates, machine manufacturer name).
type Settings struct {
	ProgramID               string    `json:"program_id"`
	RegencyID                string    `json:"regency_id"`
	LeaderCount              int       `json:"leader_count"`
	AdminCount               int       `json:"admin_count"`
	TechnicianCount          int       `json:"technician_count"`
	HelperCount              int       `json:"helper_count"`
	MachineManufacturerName  string    `json:"machine_manufacturer_name"`
	DeliveryHoseDate         string    `json:"delivery_hose_date"`
	DeliveryMachineGroupDate string    `json:"delivery_machine_group_date"`
	DeliveryGasCylinderDate  string    `json:"delivery_gas_cylinder_date"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// PhotoPick is one chosen photo for one documentation slide.
type PhotoPick struct {
	ID              string    `json:"id"`
	ProgramID       string    `json:"program_id"`
	RegencyID       string    `json:"regency_id"`
	SlideKey        string    `json:"slide_key"`
	ActivityMediaID string    `json:"activity_media_id"`
	SortOrder       int       `json:"sort_order"`
	DisplayName     string    `json:"display_name"`
	ContentURL      string    `json:"content_url"`
	// StorageKey is the underlying activity_media.storage_key, not exposed
	// in the JSON the frontend sees (`json:"-"`) — it exists only so
	// Task 10's Generate can open the photo's bytes via media.Storage.Open.
	StorageKey string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

// Document is one generated PPTX version, mirroring bast_aggregate_documents.
type Document struct {
	ID          string    `json:"id"`
	ProgramID   string    `json:"program_id"`
	RegencyID   string    `json:"regency_id"`
	Version     int       `json:"version"`
	Filename    string    `json:"filename"`
	Checksum    string    `json:"checksum"`
	StorageKey  string    `json:"storage_key"`
	Status      string    `json:"status"`
	FinalizedAt time.Time `json:"finalized_at"`
}
```

- [ ] **Step 2: Write repository.go (Settings only for this task)**

```go
package finalreport

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const settingsColumns = `program_id::text, regency_id::text, leader_count, admin_count, technician_count, helper_count, machine_manufacturer_name, COALESCE(delivery_hose_date::text,''), COALESCE(delivery_machine_group_date::text,''), COALESCE(delivery_gas_cylinder_date::text,''), updated_at`

func (r *Repository) GetSettings(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Settings, error) {
	var s Settings
	err := r.pool.QueryRow(ctx, `
		SELECT `+settingsColumns+`
		FROM final_report_settings
		WHERE program_id=$1 AND regency_id=$2 AND ($3 OR regency_id::text=ANY($4))
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&s.ProgramID, &s.RegencyID, &s.LeaderCount, &s.AdminCount, &s.TechnicianCount, &s.HelperCount,
		&s.MachineManufacturerName, &s.DeliveryHoseDate, &s.DeliveryMachineGroupDate, &s.DeliveryGasCylinderDate, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{ProgramID: programID, RegencyID: regencyID}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get final report settings: %w", err)
	}
	return s, nil
}

func (r *Repository) PutSettings(ctx context.Context, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error) {
	var s Settings
	err := r.pool.QueryRow(ctx, `
		INSERT INTO final_report_settings (program_id, regency_id, leader_count, admin_count, technician_count, helper_count, machine_manufacturer_name, delivery_hose_date, delivery_machine_group_date, delivery_gas_cylinder_date, updated_by)
		SELECT $1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8,'')::date, NULLIF($9,'')::date, NULLIF($10,'')::date, NULLIF($11,'')::uuid
		WHERE $12 OR $2::text=ANY($13)
		ON CONFLICT (program_id, regency_id) DO UPDATE SET
			leader_count=EXCLUDED.leader_count, admin_count=EXCLUDED.admin_count, technician_count=EXCLUDED.technician_count,
			helper_count=EXCLUDED.helper_count, machine_manufacturer_name=EXCLUDED.machine_manufacturer_name,
			delivery_hose_date=EXCLUDED.delivery_hose_date, delivery_machine_group_date=EXCLUDED.delivery_machine_group_date,
			delivery_gas_cylinder_date=EXCLUDED.delivery_gas_cylinder_date, updated_by=EXCLUDED.updated_by, updated_at=now()
		RETURNING `+settingsColumns,
		input.ProgramID, input.RegencyID, input.LeaderCount, input.AdminCount, input.TechnicianCount, input.HelperCount,
		input.MachineManufacturerName, input.DeliveryHoseDate, input.DeliveryMachineGroupDate, input.DeliveryGasCylinderDate,
		actor.UserID, scope.Unrestricted, scope.RegencyIDs,
	).Scan(
		&s.ProgramID, &s.RegencyID, &s.LeaderCount, &s.AdminCount, &s.TechnicianCount, &s.HelperCount,
		&s.MachineManufacturerName, &s.DeliveryHoseDate, &s.DeliveryMachineGroupDate, &s.DeliveryGasCylinderDate, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrRegencyOutOfScope
	}
	if err != nil {
		return Settings{}, fmt.Errorf("put final report settings: %w", err)
	}
	return s, nil
}
```

This mirrors `internal/activities/repository.go`'s exact pool type (`*pgxpool.Pool` from `github.com/jackc/pgx/v5/pgxpool`) and the domain-local `ErrRegencyOutOfScope` convention already used by `internal/programs` (`programs.ErrRegencyOutOfScope`, mapped to 403 `regency_out_of_scope` in `routes.go`) rather than inventing a new shared `auth` package error.

- [ ] **Step 3: Write the integration test**

```go
package finalreport

import (
	"context"
	"os"
	"testing"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func unrestrictedScope() auth.RegencyScope { return auth.RegencyScope{Unrestricted: true} }

func TestGetSettingsReturnsZeroValueWhenNoRowExists(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}

	settings, err := repo.GetSettings(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.LeaderCount != 0 {
		t.Fatalf("expected zero-value settings, got %+v", settings)
	}
}

func TestPutSettingsUpsertsAndRejectsOutOfScopeRegency(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	actor := auth.Principal{UserID: ""}

	saved, err := repo.PutSettings(ctx, actor, Settings{ProgramID: programID, RegencyID: regencyID, LeaderCount: 1, AdminCount: 2, TechnicianCount: 2, HelperCount: 2}, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.LeaderCount != 1 || saved.AdminCount != 2 {
		t.Fatalf("got %+v", saved)
	}

	scoped := auth.RegencyScope{Unrestricted: false, RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}
	if _, err := repo.PutSettings(ctx, actor, Settings{ProgramID: programID, RegencyID: regencyID, LeaderCount: 9}, scoped); err == nil {
		t.Fatal("expected out-of-scope error, got nil")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable go test ./internal/finalreport/... -v`
Expected: PASS (after fixing the `pgxPool`/`ErrOutOfScope` placeholders noted in Step 2 against the real conventions in `internal/activities/repository.go` and `internal/auth`)

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/models.go internal/finalreport/repository.go internal/finalreport/repository_integration_test.go
git commit -m "feat(finalreport): add Settings repository"
```

---

### Task 5: Photo-picks repository

**Files:**
- Modify: `internal/finalreport/repository.go`
- Modify: `internal/finalreport/repository_integration_test.go`

**Interfaces:**
- Consumes: `Settings`/error vars from Task 4.
- Produces: `func (r *Repository) ListPhotoPicks(ctx, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error)`, `func (r *Repository) ReplacePhotoPicks(ctx, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error)` — consumed by Task 7.

- [ ] **Step 1: Write the failing test**

```go
func TestReplacePhotoPicksRejectsPhotoFromAnotherRegency(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyA, regencyB string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyA); err != nil {
		t.Fatalf("fixture regency a: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id OFFSET 1 LIMIT 1`).Scan(&regencyB); err != nil {
		t.Fatalf("fixture regency b: %v", err)
	}

	var mediaID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_media (regency_id, activity_type, storage_key, display_name, original_filename, media_type, mime_type, byte_size, checksum, source)
		VALUES ($1::uuid, 'rakor', $2, 'TEST-RAKOR', 'test.jpg', 'image', 'image/jpeg', 100, repeat('a',64), 'gallery')
		RETURNING id::text
	`, regencyB, "finalreport-test-"+t.Name()).Scan(&mediaID); err != nil {
		t.Fatalf("seed activity_media: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM activity_media WHERE id=$1::uuid`, mediaID) })

	_, err := repo.ReplacePhotoPicks(ctx, programID, regencyA, SlideKeyRapatKoordinasi, []string{mediaID}, unrestrictedScope())
	if !errors.Is(err, ErrPhotoOutOfScope) {
		t.Fatalf("expected ErrPhotoOutOfScope, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestReplacePhotoPicks -v`
Expected: FAIL with "undefined: ReplacePhotoPicks" (or "undefined: ErrPhotoOutOfScope" if that's missing — it was added in Task 4).

- [ ] **Step 3: Implement**

Append to `repository.go`:

```go
func (r *Repository) ListPhotoPicks(ctx context.Context, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id::text, p.program_id::text, p.regency_id::text, p.slide_key, p.activity_media_id::text, p.sort_order,
		       m.display_name, '/api/v1/activities/media/'||m.id::text||'/content', m.storage_key, p.created_at
		FROM final_report_photo_picks p
		JOIN activity_media m ON m.id = p.activity_media_id
		WHERE p.program_id=$1 AND p.regency_id=$2 AND p.slide_key=$3 AND ($4 OR p.regency_id::text=ANY($5))
		ORDER BY p.sort_order
	`, programID, regencyID, slideKey, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list photo picks: %w", err)
	}
	defer rows.Close()
	items := []PhotoPick{}
	for rows.Next() {
		var p PhotoPick
		if err := rows.Scan(&p.ID, &p.ProgramID, &p.RegencyID, &p.SlideKey, &p.ActivityMediaID, &p.SortOrder, &p.DisplayName, &p.ContentURL, &p.StorageKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// ReplacePhotoPicks swaps the entire pick list for one slide_key in a single
// transaction: validates every activity_media_id belongs to regencyID (and is
// within scope), deletes the old picks, inserts the new ones in order.
func (r *Repository) ReplacePhotoPicks(ctx context.Context, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, mediaID := range activityMediaIDs {
		var belongs bool
		if err := tx.QueryRow(ctx, `SELECT regency_id=$2::uuid FROM activity_media WHERE id=$1::uuid AND status='active'`, mediaID, regencyID).Scan(&belongs); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPhotoOutOfScope
			}
			return nil, fmt.Errorf("check photo scope: %w", err)
		}
		if !belongs {
			return nil, ErrPhotoOutOfScope
		}
	}
	if !scope.Unrestricted {
		allowed := false
		for _, id := range scope.RegencyIDs {
			if id == regencyID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrRegencyOutOfScope
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE program_id=$1 AND regency_id=$2 AND slide_key=$3`, programID, regencyID, slideKey); err != nil {
		return nil, fmt.Errorf("clear photo picks: %w", err)
	}
	for i, mediaID := range activityMediaIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO final_report_photo_picks (program_id, regency_id, slide_key, activity_media_id, sort_order) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5)`, programID, regencyID, slideKey, mediaID, i); err != nil {
			return nil, fmt.Errorf("insert photo pick: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListPhotoPicks(ctx, programID, regencyID, slideKey, scope)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/finalreport/... -run TestReplacePhotoPicks -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/repository.go internal/finalreport/repository_integration_test.go
git commit -m "feat(finalreport): add photo-picks repository with cross-regency rejection"
```

---

### Task 6: Documents repository (versioned, advisory-locked)

**Files:**
- Modify: `internal/finalreport/repository.go`
- Modify: `internal/finalreport/repository_integration_test.go`

**Interfaces:**
- Produces: `func (r *Repository) ListDocuments(ctx, programID, regencyID string, scope auth.RegencyScope) ([]Document, error)`, `func (r *Repository) InsertDocumentVersion(ctx, actor auth.Principal, programID, regencyID, filename, checksum, storageKey string, scope auth.RegencyScope) (Document, string, error)` (3rd return: previous active document's storage_key, empty if none — caller uses it to delete the superseded Drive file) — consumed by Task 10's `Generate`.

- [ ] **Step 1: Write the failing test**

```go
func TestInsertDocumentVersionIncrementsAndSupersedes(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: ""}

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID) })

	first, prevKey, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum1", "key1", unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Version != 1 || prevKey != "" {
		t.Fatalf("got version=%d prevKey=%q, want version=1 prevKey=\"\"", first.Version, prevKey)
	}

	second, prevKey, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum2", "key2", unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Version != 2 || prevKey != "key1" {
		t.Fatalf("got version=%d prevKey=%q, want version=2 prevKey=\"key1\"", second.Version, prevKey)
	}

	docs, err := repo.ListDocuments(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 2 || docs[0].Status != "active" || docs[1].Status != "superseded" {
		t.Fatalf("got %+v", docs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestInsertDocumentVersion -v`
Expected: FAIL with "undefined: InsertDocumentVersion"

- [ ] **Step 3: Implement**

Append to `repository.go`:

```go
const documentColumns = `id::text, program_id::text, regency_id::text, version, filename, checksum, storage_key, status, finalized_at`

func (r *Repository) ListDocuments(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+documentColumns+` FROM final_report_documents
		WHERE program_id=$1 AND regency_id=$2 AND ($3 OR regency_id::text=ANY($4))
		ORDER BY version DESC
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list final report documents: %w", err)
	}
	defer rows.Close()
	items := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.ProgramID, &d.RegencyID, &d.Version, &d.Filename, &d.Checksum, &d.StorageKey, &d.Status, &d.FinalizedAt); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// InsertDocumentVersion locks on (programID, regencyID) via an advisory
// transaction lock — same pattern as bast's ActivateAggregate — so two
// concurrent "Generate PPTX" clicks produce sequential versions, never a
// collision. Returns the previous active document's storage_key (empty if
// this is the first version) so the caller can delete the superseded file
// from Drive.
func (r *Repository) InsertDocumentVersion(ctx context.Context, actor auth.Principal, programID, regencyID, filename, checksum, storageKey string, scope auth.RegencyScope) (Document, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Document{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, programID+":"+regencyID); err != nil {
		return Document{}, "", err
	}
	if !scope.Unrestricted {
		allowed := false
		for _, id := range scope.RegencyIDs {
			if id == regencyID {
				allowed = true
				break
			}
		}
		if !allowed {
			return Document{}, "", ErrRegencyOutOfScope
		}
	}

	var previousKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM final_report_documents WHERE program_id=$1 AND regency_id=$2 AND status='active'`, programID, regencyID).Scan(&previousKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Document{}, "", fmt.Errorf("find previous active document: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE final_report_documents SET status='superseded' WHERE program_id=$1 AND regency_id=$2 AND status='active'`, programID, regencyID); err != nil {
		return Document{}, "", fmt.Errorf("supersede previous document: %w", err)
	}

	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM final_report_documents WHERE program_id=$1 AND regency_id=$2`, programID, regencyID).Scan(&version); err != nil {
		return Document{}, "", fmt.Errorf("next version: %w", err)
	}

	var d Document
	err = tx.QueryRow(ctx, `
		INSERT INTO final_report_documents (program_id, regency_id, version, filename, checksum, storage_key, status, finalized_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,'active',NULLIF($7,'')::uuid)
		RETURNING `+documentColumns,
		programID, regencyID, version, filename, checksum, storageKey, actor.UserID,
	).Scan(&d.ID, &d.ProgramID, &d.RegencyID, &d.Version, &d.Filename, &d.Checksum, &d.StorageKey, &d.Status, &d.FinalizedAt)
	if err != nil {
		return Document{}, "", fmt.Errorf("insert document version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, "", err
	}
	return d, previousKey, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/finalreport/... -run TestInsertDocumentVersion -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/repository.go internal/finalreport/repository_integration_test.go
git commit -m "feat(finalreport): add versioned, advisory-locked documents repository"
```

---

### Task 7: Service layer — Settings, PhotoPicks, Documents

**Files:**
- Create: `internal/finalreport/service.go`
- Create: `internal/finalreport/service_test.go`

**Interfaces:**
- Consumes: `Repository` methods from Tasks 4-6; `programs.StorageContext`/`ResolveStorageContext` via a local `programContextResolver` interface (same pattern as `internal/activities/service.go:35`).
- Produces: `NewService(repo *Repository, programContexts programContextResolver) *Service`, `(s *Service) GetSettings`, `(s *Service) PutSettings`, `(s *Service) ListPhotoPicks`, `(s *Service) ReplacePhotoPicks`, `(s *Service) ListDocuments` — consumed by Task 11's API routes.

- [ ] **Step 1: Write the failing test**

```go
package finalreport

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

type stubProgramContexts struct{ ctx programs.StorageContext }

func (s stubProgramContexts) ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error) {
	return s.ctx, nil
}

func TestPutSettingsRejectsNegativeCounts(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	_, err := svc.PutSettings(context.Background(), auth.Principal{}, Settings{ProgramID: "p", RegencyID: "r", LeaderCount: -1}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrCountNegative) {
		t.Fatalf("expected ErrCountNegative, got %v", err)
	}
}

func TestReplacePhotoPicksRejectsMoreThanSix(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	ids := []string{"1", "2", "3", "4", "5", "6", "7"}
	_, err := svc.ReplacePhotoPicks(context.Background(), "p", "r", SlideKeyRapatKoordinasi, ids, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrTooManyPicks) {
		t.Fatalf("expected ErrTooManyPicks, got %v", err)
	}
}
```

`NewServiceForTest` is a test-only constructor you add alongside `NewService`, accepting a `nil` repository — used only for validation-before-repository-call tests like these two, since the repository is never reached (validation fails first).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestPutSettingsRejectsNegativeCounts -v`
Expected: FAIL with "undefined: NewServiceForTest"

- [ ] **Step 3: Implement service.go**

```go
package finalreport

import (
	"context"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

type programContextResolver interface {
	ResolveStorageContext(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (programs.StorageContext, error)
}

type Service struct {
	repo            *Repository
	programContexts programContextResolver
}

func NewService(repo *Repository, programContexts programContextResolver) *Service {
	return &Service{repo: repo, programContexts: programContexts}
}

// NewServiceForTest builds a Service with a nil repository, for tests that
// only exercise input validation (which short-circuits before any
// repository call).
func NewServiceForTest(repo *Repository, programContexts programContextResolver) *Service {
	return &Service{repo: repo, programContexts: programContexts}
}

func (s *Service) GetSettings(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Settings, error) {
	programID, regencyID = strings.TrimSpace(programID), strings.TrimSpace(regencyID)
	if programID == "" {
		return Settings{}, ErrProgramRequired
	}
	if regencyID == "" {
		return Settings{}, ErrRegencyRequired
	}
	return s.repo.GetSettings(ctx, programID, regencyID, scope)
}

func (s *Service) PutSettings(ctx context.Context, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error) {
	input.ProgramID, input.RegencyID = strings.TrimSpace(input.ProgramID), strings.TrimSpace(input.RegencyID)
	if input.ProgramID == "" {
		return Settings{}, ErrProgramRequired
	}
	if input.RegencyID == "" {
		return Settings{}, ErrRegencyRequired
	}
	if input.LeaderCount < 0 || input.AdminCount < 0 || input.TechnicianCount < 0 || input.HelperCount < 0 {
		return Settings{}, ErrCountNegative
	}
	input.MachineManufacturerName = strings.TrimSpace(input.MachineManufacturerName)
	return s.repo.PutSettings(ctx, actor, input, scope)
}

func (s *Service) ListPhotoPicks(ctx context.Context, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	return s.repo.ListPhotoPicks(ctx, programID, regencyID, slideKey, scope)
}

func (s *Service) ReplacePhotoPicks(ctx context.Context, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	if len(activityMediaIDs) > 6 {
		return nil, ErrTooManyPicks
	}
	return s.repo.ReplacePhotoPicks(ctx, programID, regencyID, slideKey, activityMediaIDs, scope)
}

func (s *Service) ListDocuments(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]Document, error) {
	return s.repo.ListDocuments(ctx, programID, regencyID, scope)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/finalreport/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/service.go internal/finalreport/service_test.go
git commit -m "feat(finalreport): add service layer for settings, photo picks, documents"
```

---

### Task 8: Preview data assembly

**Files:**
- Modify: `internal/finalreport/repository.go`
- Modify: `internal/finalreport/service.go`
- Create: `internal/finalreport/preview.go`
- Modify: `internal/finalreport/repository_integration_test.go`
- Modify: `internal/finalreport/service_test.go`

**Interfaces:**
- Produces: `type Preview struct{...}` (one field per placeholder token from Task 3, plus `PhotoPicks map[string][]PhotoPick`), `func (r *Repository) GetPreviewSourceData(ctx, programID, regencyID string, scope auth.RegencyScope) (previewSourceData, error)`, `func (s *Service) Preview(ctx, programID, regencyID string, scope auth.RegencyScope) (Preview, error)` — consumed by Task 9 (Generate) and Task 11 (API `/preview`).

- [ ] **Step 1: Write `preview.go` with the `Preview` struct and placeholder-blank rule**

```go
package finalreport

// Preview is the fully-assembled, live data for one Program+Kabupaten,
// shaped 1:1 with the placeholder tokens in template/placeholders.go. Every
// string field is "Belum diisi" when its source data doesn't exist yet —
// Preview/Present/Generate never fail because of incomplete data.
type Preview struct {
	RegencyLabel            string `json:"regency_label"`
	FiscalYear              string `json:"fiscal_year"`
	AgricultureOfficeName   string `json:"agriculture_office_name"`
	ConsultantCompanyName   string `json:"consultant_company_name"`
	MachineManufacturerName string `json:"machine_manufacturer_name"`
	DeliveryHoseDate         string `json:"delivery_hose_date"`
	DeliveryMachineGroupDate string `json:"delivery_machine_group_date"`
	DeliveryGasCylinderDate  string `json:"delivery_gas_cylinder_date"`
	DeliveryRangeStart       string `json:"delivery_range_start"`
	DeliveryRangeEnd         string `json:"delivery_range_end"`
	RakordaDate              string `json:"rakorda_date"`
	SosialisasiDate          string `json:"sosialisasi_date"`
	LeaderCount              string `json:"leader_count"`
	AdminCount               string `json:"admin_count"`
	TechnicianCount          string `json:"technician_count"`
	HelperCount              string `json:"helper_count"`
	HandoverLocation         string `json:"handover_location"`
	DistributionStartDate    string `json:"distribution_start_date"`
	DistributionEndDate      string `json:"distribution_end_date"`
	DistributionTotal        string `json:"distribution_total"`
	PhotoPicks               map[string][]PhotoPick `json:"photo_picks"`
}

const blank = "Belum diisi"

func orBlank(value string) string {
	if value == "" {
		return blank
	}
	return value
}
```

- [ ] **Step 2: Write the failing repository test**

```go
func TestGetPreviewSourceDataFillsBlanksWhenDataMissing(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `
		SELECT ps.program_id::text, ps.regency_id::text FROM program_schedules ps
		LEFT JOIN bast_schedule_settings bs ON bs.schedule_id = ps.id
		WHERE bs.handover_location IS NULL OR bs.handover_location = ''
		LIMIT 1
	`).Scan(&programID, &regencyID); err != nil {
		t.Skip("no schedule without handover_location in test DB; skipping blank-field assertion")
	}

	data, err := repo.GetPreviewSourceData(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.handoverLocation != "" {
		t.Fatalf("expected empty handoverLocation from source data, got %q", data.handoverLocation)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestGetPreviewSourceData -v`
Expected: FAIL with "undefined: GetPreviewSourceData"

- [ ] **Step 4: Implement `GetPreviewSourceData` in repository.go**

```go
// previewSourceData is the raw (possibly-empty) values pulled live from
// sibling domains' tables via direct SQL — finalreport never imports the
// bast/distribution/activities Go packages, the same way bast's own
// repository reads program_schedules directly instead of importing
// internal/programs.
type previewSourceData struct {
	regencyName, provinceName                                   string
	fiscalYear                                                   int
	agricultureOfficeName, consultantCompanyName, consultantDefault string
	handoverLocation                                              string
	rakordaDate, sosialisasiDate                                   string
	distributionStart, distributionEnd                             string
	distributionTotal                                              int
}

func (r *Repository) GetPreviewSourceData(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (previewSourceData, error) {
	var data previewSourceData
	err := r.pool.QueryRow(ctx, `
		SELECT reg.name, reg.province_name, p.fiscal_year
		FROM regencies reg, programs p
		WHERE reg.id=$1::uuid AND p.id=$2::uuid AND ($3 OR reg.id::text=ANY($4))
	`, regencyID, programID, scope.Unrestricted, scope.RegencyIDs).Scan(&data.regencyName, &data.provinceName, &data.fiscalYear)
	if errors.Is(err, pgx.ErrNoRows) {
		return previewSourceData{}, ErrNotFound
	}
	if err != nil {
		return previewSourceData{}, fmt.Errorf("get regency/program: %w", err)
	}

	var scheduleID string
	err = r.pool.QueryRow(ctx, `
		SELECT id::text FROM program_schedules
		WHERE program_id=$1::uuid AND regency_id=$2::uuid
		ORDER BY (status='active') DESC, start_date DESC LIMIT 1
	`, programID, regencyID).Scan(&scheduleID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return previewSourceData{}, fmt.Errorf("resolve schedule: %w", err)
	}
	if scheduleID == "" {
		return data, nil // no schedule yet — every remaining field stays blank
	}

	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(agriculture_office_name,''), COALESCE(consultant_company_name,''), COALESCE(handover_location,'')
		FROM bast_schedule_settings WHERE schedule_id=$1::uuid
	`, scheduleID).Scan(&data.agricultureOfficeName, &data.consultantCompanyName, &data.handoverLocation)

	_ = r.pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_settings WHERE key='organization_name'`).Scan(&data.consultantDefault)
	if data.consultantCompanyName == "" {
		data.consultantCompanyName = data.consultantDefault
	}

	// Rakorda and Sosialisasi share one table (bast_rakorda_uploads), told
	// apart by document_kind (migration 00043_bast_sosialisasi.sql) — not
	// two separate tables.
	_ = r.pool.QueryRow(ctx, `SELECT to_char(min(event_date),'DD Month YYYY') FROM bast_rakorda_uploads WHERE schedule_id=$1::uuid AND document_kind='rakorda'`, scheduleID).Scan(&data.rakordaDate)
	_ = r.pool.QueryRow(ctx, `SELECT to_char(min(event_date),'DD Month YYYY') FROM bast_rakorda_uploads WHERE schedule_id=$1::uuid AND document_kind='sosialisasi'`, scheduleID).Scan(&data.sosialisasiDate)

	_ = r.pool.QueryRow(ctx, `
		SELECT to_char(min(completed_at),'DD Month YYYY'), to_char(max(completed_at),'DD Month YYYY'), count(*)
		FROM distribution_slots WHERE schedule_id=$1::uuid AND status='completed'
	`, scheduleID).Scan(&data.distributionStart, &data.distributionEnd, &data.distributionTotal)

	return data, nil
}
```

- [ ] **Step 5: Add `Preview` assembly to `service.go`**

```go
func (s *Service) Preview(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Preview, error) {
	data, err := s.repo.GetPreviewSourceData(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}
	settings, err := s.repo.GetSettings(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}

	picks := map[string][]PhotoPick{}
	for _, key := range []string{SlideKeyPengirimanPenerimaan, SlideKeyRapatKoordinasi, SlideKeySosialisasi} {
		list, err := s.repo.ListPhotoPicks(ctx, programID, regencyID, key, scope)
		if err != nil {
			return Preview{}, err
		}
		picks[key] = list
	}

	deliveryStart, deliveryEnd := earliestLatest(settings.DeliveryHoseDate, settings.DeliveryMachineGroupDate, settings.DeliveryGasCylinderDate)

	return Preview{
		RegencyLabel:             orBlank(strings.ToUpper("Kabupaten " + data.regencyName)),
		FiscalYear:               orBlank(itoaOrEmpty(data.fiscalYear)),
		AgricultureOfficeName:    orBlank(data.agricultureOfficeName),
		ConsultantCompanyName:    orBlank(data.consultantCompanyName),
		MachineManufacturerName:  orBlank(settings.MachineManufacturerName),
		DeliveryHoseDate:         orBlank(settings.DeliveryHoseDate),
		DeliveryMachineGroupDate: orBlank(settings.DeliveryMachineGroupDate),
		DeliveryGasCylinderDate:  orBlank(settings.DeliveryGasCylinderDate),
		DeliveryRangeStart:       orBlank(deliveryStart),
		DeliveryRangeEnd:         orBlank(deliveryEnd),
		RakordaDate:              orBlank(data.rakordaDate),
		SosialisasiDate:          orBlank(data.sosialisasiDate),
		LeaderCount:              itoaOrEmpty(settings.LeaderCount),
		AdminCount:               itoaOrEmpty(settings.AdminCount),
		TechnicianCount:          itoaOrEmpty(settings.TechnicianCount),
		HelperCount:              itoaOrEmpty(settings.HelperCount),
		HandoverLocation:         orBlank(data.handoverLocation),
		DistributionStartDate:    orBlank(data.distributionStart),
		DistributionEndDate:      orBlank(data.distributionEnd),
		DistributionTotal:        orBlank(itoaOrEmptyNonZero(data.distributionTotal)),
		PhotoPicks:               picks,
	}, nil
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return "0"
	}
	return strconv.Itoa(n)
}

func itoaOrEmptyNonZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// earliestLatest returns the min/max of the non-empty dates among the 3
// delivery-date settings fields, formatted the same way the SQL-sourced
// dates are ("DD Month YYYY" is not produced here — these are already
// plain ISO strings from Settings, displayed as-is).
func earliestLatest(dates ...string) (string, string) {
	var nonEmpty []string
	for _, d := range dates {
		if d != "" {
			nonEmpty = append(nonEmpty, d)
		}
	}
	if len(nonEmpty) == 0 {
		return "", ""
	}
	sort.Strings(nonEmpty)
	return nonEmpty[0], nonEmpty[len(nonEmpty)-1]
}
```

Add `"sort"` and `"strconv"` to `service.go`'s imports.

- [ ] **Step 6: Write the service-level blank-field test**

```go
func TestPreviewFillsBlankWhenNoDataExists(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	svc := NewService(repo, stubProgramContexts{})
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	// A regency with no program_schedules row at all for this program:
	if err := pool.QueryRow(ctx, `
		SELECT reg.id::text FROM regencies reg
		WHERE NOT EXISTS (SELECT 1 FROM program_schedules ps WHERE ps.program_id=$1::uuid AND ps.regency_id=reg.id)
		LIMIT 1
	`, programID).Scan(&regencyID); err != nil {
		t.Skip("every regency already has a schedule for this program in the test DB")
	}

	preview, err := svc.Preview(ctx, programID, regencyID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.RakordaDate != "Belum diisi" || preview.HandoverLocation != "Belum diisi" {
		t.Fatalf("expected blank placeholders, got %+v", preview)
	}
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable go test ./internal/finalreport/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/finalreport/preview.go internal/finalreport/repository.go internal/finalreport/service.go internal/finalreport/repository_integration_test.go internal/finalreport/service_test.go
git commit -m "feat(finalreport): assemble live Preview data with blank-field fallback"
```

---

### Task 9: PPTX template substitution engine

**Files:**
- Create: `internal/finalreport/pptx.go`
- Create: `internal/finalreport/pptx_test.go`

**Interfaces:**
- Consumes: `Preview` (Task 8), `template.AllTokens` (Task 3).
- Produces: `func RenderPPTX(templatePath string, preview Preview, photoBytes map[string][][]byte) ([]byte, error)` — `photoBytes` keyed by `slide_key`, ordered list of raw image bytes to splice into that slide's picture placeholders, in order. Consumed by Task 10.

- [ ] **Step 1: Write the failing test**

```go
package finalreport

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"konkit/internal/finalreport/template"
)

func TestRenderPPTXSubstitutesEveryToken(t *testing.T) {
	preview := Preview{
		RegencyLabel: "KABUPATEN TEST", FiscalYear: "2026", AgricultureOfficeName: "DINAS TEST",
		ConsultantCompanyName: "PT TEST", MachineManufacturerName: "PT MESIN TEST",
		DeliveryHoseDate: "1 Januari 2026", DeliveryMachineGroupDate: "2 Januari 2026", DeliveryGasCylinderDate: "3 Januari 2026",
		DeliveryRangeStart: "1 Januari 2026", DeliveryRangeEnd: "3 Januari 2026",
		RakordaDate: "4 Januari 2026", SosialisasiDate: "5 Januari 2026",
		LeaderCount: "1", AdminCount: "2", TechnicianCount: "3", HelperCount: "4",
		HandoverLocation: "GUDANG TEST", DistributionStartDate: "1 Februari 2026", DistributionEndDate: "28 Februari 2026", DistributionTotal: "100",
	}

	output, err := RenderPPTX("template/final_report_template.pptx", preview, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	var allText strings.Builder
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		allText.WriteString(buf.String())
	}
	combined := allText.String()
	for _, token := range template.AllTokens {
		if strings.Contains(combined, token) {
			t.Errorf("token %s was not substituted", token)
		}
	}
	if !strings.Contains(combined, "KABUPATEN TEST") || !strings.Contains(combined, "GUDANG TEST") {
		t.Fatalf("expected substituted values present in output")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestRenderPPTXSubstitutesEveryToken -v`
Expected: FAIL with "undefined: RenderPPTX"

- [ ] **Step 3: Implement `pptx.go`**

```go
package finalreport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"konkit/internal/finalreport/template"
)

// slideImageOrder maps each photo-bearing slide to the ppt/media file names
// that RenderPPTX overwrites with the picked photos' bytes. Read directly
// off the reference file's ppt/slides/_rels/slide{5,6,7}.xml.rels (slide5 =
// Pengiriman & Penerimaan, slide6 = Rapat Koordinasi, slide7 = Sosialisasi);
// image9.png and image5.png appear in all three rels files and are the
// shared header/logo graphics repeated on every slide, not content photos,
// so they're excluded here. Task 3's conversion script copies ppt/media/*
// byte-for-byte unchanged, so these file names carry over identically into
// the committed template.
var slideImageOrder = map[string][]string{
	SlideKeyPengirimanPenerimaan: {"ppt/media/image1.jpg", "ppt/media/image2.jpg", "ppt/media/image12.jpg"},
	SlideKeyRapatKoordinasi:      {"ppt/media/image8.jpg", "ppt/media/image15.jpg"},
	SlideKeySosialisasi:          {"ppt/media/image3.jpg", "ppt/media/image6.jpg", "ppt/media/image16.jpg", "ppt/media/image14.jpg"},
}

func RenderPPTX(templatePath string, preview Preview, photoBytes map[string][][]byte) ([]byte, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("read pptx template: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pptx template as zip: %w", err)
	}

	replacements := map[string]string{
		template.TokenRegencyLabel: preview.RegencyLabel, template.TokenFiscalYear: preview.FiscalYear,
		template.TokenAgricultureOfficeName: preview.AgricultureOfficeName, template.TokenConsultantCompanyName: preview.ConsultantCompanyName,
		template.TokenMachineManufacturerName: preview.MachineManufacturerName,
		template.TokenDeliveryHoseDate: preview.DeliveryHoseDate, template.TokenDeliveryMachineGroupDate: preview.DeliveryMachineGroupDate,
		template.TokenDeliveryGasCylinderDate: preview.DeliveryGasCylinderDate,
		template.TokenDeliveryRangeStart: preview.DeliveryRangeStart, template.TokenDeliveryRangeEnd: preview.DeliveryRangeEnd,
		template.TokenRakordaDate: preview.RakordaDate, template.TokenSosialisasiDate: preview.SosialisasiDate,
		template.TokenLeaderCount: preview.LeaderCount, template.TokenAdminCount: preview.AdminCount,
		template.TokenTechnicianCount: preview.TechnicianCount, template.TokenHelperCount: preview.HelperCount,
		template.TokenHandoverLocation: preview.HandoverLocation,
		template.TokenDistributionStartDate: preview.DistributionStartDate, template.TokenDistributionEndDate: preview.DistributionEndDate,
		template.TokenDistributionTotal: preview.DistributionTotal,
	}

	imageOverrides := map[string][]byte{}
	for slideKey, files := range slideImageOrder {
		pics := photoBytes[slideKey]
		for i, name := range files {
			if i < len(pics) {
				imageOverrides[name] = pics[i]
			}
		}
	}

	out := new(bytes.Buffer)
	writer := zip.NewWriter(out)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, err)
		}

		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			text := string(content)
			for token, value := range replacements {
				text = strings.ReplaceAll(text, token, value)
			}
			content = []byte(text)
		}
		if override, ok := imageOverrides[file.Name]; ok {
			content = override
		}

		header, err := zip.FileInfoHeader(file.FileInfo())
		if err != nil {
			return nil, err
		}
		header.Name = file.Name
		header.Method = zip.Deflate
		w, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close pptx zip: %w", err)
	}
	return out.Bytes(), nil
}
```

If Task 3's reference file ever changes (a different example PPTX supplied later), re-derive this map the same way: `unzip -p internal/finalreport/template/final_report_template.pptx ppt/slides/_rels/slide5.xml.rels` (and slide6/slide7), excluding whichever image names repeat identically across all three rels files (those are the shared header/logo graphic, not content photos).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/finalreport/... -run TestRenderPPTXSubstitutesEveryToken -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/pptx.go internal/finalreport/pptx_test.go
git commit -m "feat(finalreport): add PPTX template substitution engine"
```

---

### Task 10: `Generate` — orchestrate Preview + PPTX + Drive + versioning

**Files:**
- Modify: `internal/finalreport/service.go`
- Modify: `internal/finalreport/service_test.go`

**Interfaces:**
- Consumes: `media.Storage`, `media.PutNamed`, `media.BuildFinalReportPath` (Task 2), `RenderPPTX` (Task 9), `Repository.InsertDocumentVersion` (Task 6).
- Produces: `func NewService(repo *Repository, programContexts programContextResolver, storage media.Storage, templatePath string) *Service` (signature grows), `func (s *Service) Generate(ctx, actor auth.Principal, programID, regencyID string, scope auth.RegencyScope, meta auth.ClientMeta) (Document, error)` — consumed by Task 11.

- [ ] **Step 1: Write the failing test**

```go
type fakeStorage struct{ putCount int }

func (f *fakeStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	f.putCount++
	data, _ := io.ReadAll(source)
	return key, int64(len(data)), "fakechecksum", nil
}
func (f *fakeStorage) Open(context.Context, string) (io.ReadCloser, error) { return nil, errors.New("not implemented") }
func (f *fakeStorage) Delete(context.Context, string) error                { return nil }
func (f *fakeStorage) EnsureFolders(context.Context, [][]string) error     { return nil }

func TestGenerateProducesSequentialVersions(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	storage := &fakeStorage{}
	svc := NewService(repo, stubProgramContexts{ctx: programs.StorageContext{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Test", ProgramID: "p", RegencyID: "r"}}, storage, "template/final_report_template.pptx")
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID) })

	doc1, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	doc2, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if doc1.Version != 1 || doc2.Version != 2 {
		t.Fatalf("got versions %d, %d; want 1, 2", doc1.Version, doc2.Version)
	}
	if storage.putCount != 2 {
		t.Fatalf("expected 2 Put calls, got %d", storage.putCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/finalreport/... -run TestGenerateProducesSequentialVersions -v`
Expected: FAIL (signature mismatch — `NewService` doesn't yet take `storage`/`templatePath`)

- [ ] **Step 3: Implement**

Update `NewService` and add `Generate` to `service.go`:

```go
type Service struct {
	repo            *Repository
	programContexts programContextResolver
	storage         media.Storage
	templatePath    string
}

func NewService(repo *Repository, programContexts programContextResolver, storage media.Storage, templatePath string) *Service {
	return &Service{repo: repo, programContexts: programContexts, storage: storage, templatePath: templatePath}
}

func (s *Service) Generate(ctx context.Context, actor auth.Principal, programID, regencyID string, scope auth.RegencyScope, meta auth.ClientMeta) (Document, error) {
	preview, err := s.Preview(ctx, programID, regencyID, scope)
	if err != nil {
		return Document{}, err
	}

	photoBytes := map[string][][]byte{}
	for slideKey, picks := range preview.PhotoPicks {
		for _, pick := range picks {
			reader, err := s.storage.Open(ctx, pick.StorageKey)
			if err != nil {
				continue // a pick whose underlying file is unreachable is skipped, not fatal
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				continue
			}
			photoBytes[slideKey] = append(photoBytes[slideKey], data)
		}
	}

	rendered, err := RenderPPTX(s.templatePath, preview, photoBytes)
	if err != nil {
		return Document{}, fmt.Errorf("render pptx: %w", err)
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, programID, regencyID, scope)
	if err != nil {
		return Document{}, err
	}
	folderPath, err := media.BuildFinalReportPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName, RegencyName: storageContext.RegencyName,
	})
	if err != nil {
		return Document{}, err
	}
	filename := fmt.Sprintf("LAPORAN AKHIR - %s %s.pptx", strings.ToUpper(string(storageContext.ProgramType)), strings.ToUpper(storageContext.RegencyName))

	key := "finalreport-" + programID + "-" + regencyID
	storageKey, _, checksum, err := media.PutNamed(ctx, s.storage, key, filename, folderPath, bytes.NewReader(rendered))
	if err != nil {
		return Document{}, fmt.Errorf("upload pptx: %w", err)
	}

	doc, previousKey, err := s.repo.InsertDocumentVersion(ctx, actor, programID, regencyID, filename, checksum, storageKey, scope)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return Document{}, err
	}
	if previousKey != "" && previousKey != storageKey {
		_ = s.storage.Delete(context.Background(), previousKey) // best-effort; a leftover superseded file is not fatal
	}
	return doc, nil
}
```

Add `"bytes"` and `"konkit/internal/media"` to `service.go`'s imports.

`pick.StorageKey` is the `activity_media.storage_key` column added to `PhotoPick` in Task 4 and selected in Task 5's `ListPhotoPicks` — not `pick.ActivityMediaID` (that's the row's own UUID, meaningless to `media.Storage.Open`).

- [ ] **Step 4: Run test to verify it passes**

Run: `TEST_DATABASE_URL=postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable go test ./internal/finalreport/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/finalreport/service.go internal/finalreport/service_test.go internal/finalreport/models.go internal/finalreport/repository.go
git commit -m "feat(finalreport): add Generate — assemble, render, upload, version"
```

---

### Task 11: API layer

**Files:**
- Modify: `internal/api/handler.go`
- Create: `internal/api/final_report_routes.go`
- Create: `internal/api/final_report_routes_test.go`
- Modify: `internal/api/routes.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `finalreport.Service` methods (Tasks 7, 8, 10).
- Produces: `GET/PUT /api/v1/final-report/settings`, `GET/PUT /api/v1/final-report/photo-picks`, `GET /api/v1/final-report/preview`, `POST /api/v1/final-report/generate`, `GET /api/v1/final-report/documents` — consumed by Task 12's frontend.

- [ ] **Step 1: Add the service interface and dependency to `handler.go`**

```go
type FinalReportService interface {
	GetSettings(context.Context, string, string, auth.RegencyScope) (finalreport.Settings, error)
	PutSettings(context.Context, auth.Principal, finalreport.Settings, auth.RegencyScope) (finalreport.Settings, error)
	ListPhotoPicks(context.Context, string, string, string, auth.RegencyScope) ([]finalreport.PhotoPick, error)
	ReplacePhotoPicks(context.Context, string, string, string, []string, auth.RegencyScope) ([]finalreport.PhotoPick, error)
	Preview(context.Context, string, string, auth.RegencyScope) (finalreport.Preview, error)
	Generate(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (finalreport.Document, error)
	ListDocuments(context.Context, string, string, auth.RegencyScope) ([]finalreport.Document, error)
}
```

Add `FinalReport FinalReportService` to the `Dependencies` struct, and `"konkit/internal/finalreport"` to the imports.

- [ ] **Step 2: Wire the top-level route dispatch**

In `handler.go`'s `ServeHTTP` path switch (same `case strings.HasPrefix(path, "...")` style used for `activities/supporting-documents/...`), add:

```go
case path == "final-report/settings":
	h.handleFinalReportSettings(w, r, rc)
case path == "final-report/photo-picks":
	h.handleFinalReportPhotoPicks(w, r, rc)
case path == "final-report/preview":
	h.handleFinalReportPreview(w, r, rc)
case path == "final-report/generate":
	h.handleFinalReportGenerate(w, r, rc)
case path == "final-report/documents":
	h.handleFinalReportDocuments(w, r, rc)
```

- [ ] **Step 3: Write `final_report_routes.go`**

```go
package api

import (
	"net/http"
	"strings"

	"konkit/internal/finalreport"
)

func (h *Handler) handleFinalReportSettings(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "final_report.view") {
			return
		}
		programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		settings, err := h.deps.FinalReport.GetSettings(r.Context(), programID, regencyID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, settings)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "final_report.manage") {
			return
		}
		var input finalreport.Settings
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		settings, err := h.deps.FinalReport.PutSettings(r.Context(), rc.principal, input, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleFinalReportPhotoPicks(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	programID, regencyID, slideKey := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id")), strings.TrimSpace(r.URL.Query().Get("slide_key"))
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "final_report.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		picks, err := h.deps.FinalReport.ListPhotoPicks(r.Context(), programID, regencyID, slideKey, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, picks)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "final_report.manage") {
			return
		}
		var input struct {
			ProgramID        string   `json:"program_id"`
			RegencyID        string   `json:"regency_id"`
			SlideKey         string   `json:"slide_key"`
			ActivityMediaIDs []string `json:"activity_media_ids"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		picks, err := h.deps.FinalReport.ReplacePhotoPicks(r.Context(), input.ProgramID, input.RegencyID, input.SlideKey, input.ActivityMediaIDs, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, picks)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleFinalReportPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.view") {
		return
	}
	programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	preview, err := h.deps.FinalReport.Preview(r.Context(), programID, regencyID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, preview)
}

func (h *Handler) handleFinalReportGenerate(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.manage") {
		return
	}
	var body struct {
		ProgramID string `json:"program_id"`
		RegencyID string `json:"regency_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	doc, err := h.deps.FinalReport.Generate(r.Context(), rc.principal, body.ProgramID, body.RegencyID, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, doc)
}

func (h *Handler) handleFinalReportDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.view") {
		return
	}
	programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	docs, err := h.deps.FinalReport.ListDocuments(r.Context(), programID, regencyID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, docs)
}
```

- [ ] **Step 4: Add error mappings to `routes.go`**

In the not-found case list, add `errors.Is(err, finalreport.ErrNotFound)`. In a new validation_failed case, add:

```go
case errors.Is(err, finalreport.ErrProgramRequired), errors.Is(err, finalreport.ErrRegencyRequired), errors.Is(err, finalreport.ErrCountNegative),
	errors.Is(err, finalreport.ErrSlideKeyInvalid), errors.Is(err, finalreport.ErrTooManyPicks), errors.Is(err, finalreport.ErrPhotoOutOfScope):
	writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), map[string]string{"request": err.Error()})
case errors.Is(err, finalreport.ErrGenerateConflict):
	writeError(w, http.StatusConflict, "document_conflict", "Dokumen Laporan Akhir telah diubah operator lain; muat ulang data")
case errors.Is(err, finalreport.ErrRegencyOutOfScope):
	writeError(w, http.StatusForbidden, "regency_out_of_scope", "Kabupaten ini berada di luar akses Anda")
```

Add `"konkit/internal/finalreport"` to `routes.go`'s imports.

- [ ] **Step 5: Wire in `main.go`**

```go
finalReportRepository := finalreport.NewRepository(pool)
finalReportService := finalreport.NewService(finalReportRepository, programService, mediaStorage, "internal/finalreport/template/final_report_template.pptx")
```

Add `FinalReport: finalReportService,` to the `Dependencies{...}` literal, and `"konkit/internal/finalreport"` to `main.go`'s imports.

- [ ] **Step 6: Write `final_report_routes_test.go`**

```go
package api

import (
	"net/http/httptest"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/finalreport"
)

type stubFinalReport struct{}

func (stubFinalReport) GetSettings(context.Context, string, string, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (stubFinalReport) PutSettings(context.Context, auth.Principal, finalreport.Settings, auth.RegencyScope) (finalreport.Settings, error) {
	return finalreport.Settings{}, nil
}
func (stubFinalReport) ListPhotoPicks(context.Context, string, string, string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (stubFinalReport) ReplacePhotoPicks(context.Context, string, string, string, []string, auth.RegencyScope) ([]finalreport.PhotoPick, error) {
	return nil, nil
}
func (stubFinalReport) Preview(context.Context, string, string, auth.RegencyScope) (finalreport.Preview, error) {
	return finalreport.Preview{}, nil
}
func (stubFinalReport) Generate(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (finalreport.Document, error) {
	return finalreport.Document{}, nil
}
func (stubFinalReport) ListDocuments(context.Context, string, string, auth.RegencyScope) ([]finalreport.Document, error) {
	return nil, nil
}

func TestFinalReportGenerateRejectsViewerWithoutManage(t *testing.T) {
	viewer := &fakeAuthService{principal: auth.Principal{UserID: "user"}, allowedPermissions: map[string]bool{"final_report.view": true}, regencyScope: auth.RegencyScope{Unrestricted: true}}
	rec := httptest.NewRecorder()
	req := authenticatedRequest("POST", "/api/v1/final-report/generate", `{"program_id":"p","regency_id":"r"}`, nil)
	NewHandler(Dependencies{Auth: viewer, FinalReport: stubFinalReport{}}).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

`fakeAuthService` and `authenticatedRequest` are both existing shared test helpers in package `api` (`handler_test.go:1156`, `bast_routes_test.go:184`) — reused as-is, not redefined in this new test file.

- [ ] **Step 7: Run tests**

Run: `go build ./... && go vet ./... && go test ./internal/api/... ./internal/finalreport/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/api/handler.go internal/api/final_report_routes.go internal/api/final_report_routes_test.go internal/api/routes.go cmd/server/main.go
git commit -m "feat(api): wire final-report settings/photo-picks/preview/generate/documents routes"
```

---

### Task 12: Frontend — types, settings form, photo picker, generate

**Files:**
- Create: `frontend/src/features/finalreport/types.ts`
- Create: `frontend/src/features/finalreport/FinalReportPage.tsx`
- Create: `frontend/src/features/finalreport/FinalReportPage.test.tsx`

**Interfaces:**
- Consumes: `ProgramOption`/`ProgramZone` pattern from `activities/types.ts`; `/api/v1/final-report/*` endpoints (Task 11); `/api/v1/activities/media?activity_type=...` (existing) for the photo gallery source.
- Produces: `export function FinalReportPage({ programId, regencyId }: { programId: string; regencyId: string })`; `export default function FinalReportRoute()` — consumed by Task 14's route wiring and by Task 13's Present button.

- [ ] **Step 1: Write `types.ts`**

```typescript
export type FinalReportSettings = {
  program_id: string;
  regency_id: string;
  leader_count: number;
  admin_count: number;
  technician_count: number;
  helper_count: number;
  machine_manufacturer_name: string;
  delivery_hose_date: string;
  delivery_machine_group_date: string;
  delivery_gas_cylinder_date: string;
};

export type FinalReportPhotoPick = {
  id: string;
  activity_media_id: string;
  slide_key: string;
  sort_order: number;
  display_name: string;
  content_url: string;
};

export type FinalReportPreview = {
  regency_label: string;
  fiscal_year: string;
  agriculture_office_name: string;
  consultant_company_name: string;
  machine_manufacturer_name: string;
  delivery_hose_date: string;
  delivery_machine_group_date: string;
  delivery_gas_cylinder_date: string;
  delivery_range_start: string;
  delivery_range_end: string;
  rakorda_date: string;
  sosialisasi_date: string;
  leader_count: string;
  admin_count: string;
  technician_count: string;
  helper_count: string;
  handover_location: string;
  distribution_start_date: string;
  distribution_end_date: string;
  distribution_total: string;
  photo_picks: Record<string, FinalReportPhotoPick[]>;
};

export type FinalReportDocument = {
  id: string;
  version: number;
  filename: string;
  status: string;
  finalized_at: string;
};

export const SLIDE_KEYS = ['pengiriman_penerimaan', 'rapat_koordinasi', 'sosialisasi'] as const;
export type SlideKey = (typeof SLIDE_KEYS)[number];
export const SLIDE_KEY_LABELS: Record<SlideKey, string> = {
  pengiriman_penerimaan: 'Pengiriman & Penerimaan Barang',
  rapat_koordinasi: 'Rapat Koordinasi',
  sosialisasi: 'Seremoni & Sosialisasi',
};
export const SLIDE_KEY_ACTIVITY_TYPES: Record<SlideKey, string[]> = {
  pengiriman_penerimaan: ['unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli', 'unloading_selang', 'unloading_tabung_gas'],
  rapat_koordinasi: ['rakor'],
  sosialisasi: ['ceremony_sosialisasi'],
};
```

- [ ] **Step 2: Write `FinalReportPage.tsx`**

```tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { lazy, Suspense, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Button } from '../../components/ui/button';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { Checkbox } from '../../components/ui/checkbox';
import type { DataResponse } from '../programs/types';
import type { ActivityMediaPage, ProgramOption, ProgramZone } from '../activities/types';
import { SLIDE_KEYS, SLIDE_KEY_LABELS, SLIDE_KEY_ACTIVITY_TYPES, type FinalReportSettings, type FinalReportPhotoPick, type FinalReportDocument } from './types';

const FinalReportPresent = lazy(() => import('./FinalReportPresent'));

export function FinalReportPage({ programId, regencyId }: { programId: string; regencyId: string }) {
  const canManage = useCan('final_report.manage');
  const client = useQueryClient();
  const [presentOpen, setPresentOpen] = useState(false);

  const settingsQuery = useQuery({
    queryKey: ['final-report', 'settings', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportSettings>>(`/api/v1/final-report/settings?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });
  const [draft, setDraft] = useState({ leader_count: 0, admin_count: 0, technician_count: 0, helper_count: 0, machine_manufacturer_name: '', delivery_hose_date: '', delivery_machine_group_date: '', delivery_gas_cylinder_date: '' });
  useEffect(() => {
    const current = settingsQuery.data?.data;
    if (current) setDraft({ leader_count: current.leader_count, admin_count: current.admin_count, technician_count: current.technician_count, helper_count: current.helper_count, machine_manufacturer_name: current.machine_manufacturer_name, delivery_hose_date: current.delivery_hose_date, delivery_machine_group_date: current.delivery_machine_group_date, delivery_gas_cylinder_date: current.delivery_gas_cylinder_date });
  }, [settingsQuery.data]);

  const saveSettings = useMutation({
    mutationFn: () => apiRequest<DataResponse<FinalReportSettings>>('/api/v1/final-report/settings', { method: 'PUT', body: JSON.stringify({ program_id: programId, regency_id: regencyId, ...draft }) }),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['final-report', 'settings', programId, regencyId] }); toast.success('Pengaturan tersimpan.'); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Pengaturan belum dapat disimpan.'),
  });

  const documentsQuery = useQuery({
    queryKey: ['final-report', 'documents', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportDocument[]>>(`/api/v1/final-report/documents?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });
  const generate = useMutation({
    mutationFn: () => apiRequest<DataResponse<FinalReportDocument>>('/api/v1/final-report/generate', { method: 'POST', body: JSON.stringify({ program_id: programId, regency_id: regencyId }) }),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['final-report', 'documents', programId, regencyId] }); toast.success('PPTX berhasil dibuat.'); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'PPTX belum dapat dibuat.'),
  });

  return <div className="space-y-6">
    {canManage && <section className="space-y-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Susunan Tim & Rincian Pengiriman</h2>
      <div className="grid gap-3 sm:grid-cols-4">
        <FormField label="Leader" name="leader_count" type="number" value={String(draft.leader_count)} onChange={(e) => setDraft((d) => ({ ...d, leader_count: Number(e.target.value) }))} />
        <FormField label="Administrasi" name="admin_count" type="number" value={String(draft.admin_count)} onChange={(e) => setDraft((d) => ({ ...d, admin_count: Number(e.target.value) }))} />
        <FormField label="Teknisi" name="technician_count" type="number" value={String(draft.technician_count)} onChange={(e) => setDraft((d) => ({ ...d, technician_count: Number(e.target.value) }))} />
        <FormField label="Helper" name="helper_count" type="number" value={String(draft.helper_count)} onChange={(e) => setDraft((d) => ({ ...d, helper_count: Number(e.target.value) }))} />
      </div>
      <FormField label="Nama Pabrikan Mesin" name="machine_manufacturer_name" value={draft.machine_manufacturer_name} onChange={(e) => setDraft((d) => ({ ...d, machine_manufacturer_name: e.target.value }))} />
      <div className="grid gap-3 sm:grid-cols-3">
        <FormField label="Tanggal Kirim Selang Buang" name="delivery_hose_date" type="date" value={draft.delivery_hose_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_hose_date: e.target.value }))} />
        <FormField label="Tanggal Kirim Mesin/Konkit/Oli/dll" name="delivery_machine_group_date" type="date" value={draft.delivery_machine_group_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_machine_group_date: e.target.value }))} />
        <FormField label="Tanggal Kirim Tabung Gas" name="delivery_gas_cylinder_date" type="date" value={draft.delivery_gas_cylinder_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_gas_cylinder_date: e.target.value }))} />
      </div>
      <Button type="button" onClick={() => saveSettings.mutate()} disabled={saveSettings.isPending}>{saveSettings.isPending ? 'Menyimpan...' : 'Simpan Pengaturan'}</Button>
    </section>}

    {SLIDE_KEYS.map((slideKey) => <PhotoPickerSection key={slideKey} programId={programId} regencyId={regencyId} slideKey={slideKey} canManage={canManage} />)}

    <section className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" onClick={() => setPresentOpen(true)}>Tampilkan</Button>
      {canManage && <Button type="button" onClick={() => generate.mutate()} disabled={generate.isPending}>{generate.isPending ? 'Membuat...' : 'Generate PPTX'}</Button>}
    </section>

    {documentsQuery.data && documentsQuery.data.data.length > 0 && <ul className="space-y-1 text-sm">
      {documentsQuery.data.data.map((doc) => <li key={doc.id} className="flex items-center justify-between rounded border p-2">
        <span>v{doc.version} — {doc.filename}</span>
        <span className="text-muted-foreground">{doc.status === 'active' ? 'Aktif' : 'Lama'}</span>
      </li>)}
    </ul>}

    {presentOpen && <Suspense fallback={null}>
      <FinalReportPresent programId={programId} regencyId={regencyId} onClose={() => setPresentOpen(false)} />
    </Suspense>}
  </div>;
}

function PhotoPickerSection({ programId, regencyId, slideKey, canManage }: { programId: string; regencyId: string; slideKey: (typeof SLIDE_KEYS)[number]; canManage: boolean }) {
  const client = useQueryClient();
  const activityTypes = SLIDE_KEY_ACTIVITY_TYPES[slideKey];

  const available = useQuery({
    queryKey: ['activities', 'media', 'final-report-source', regencyId, slideKey],
    queryFn: async () => {
      const pages = await Promise.all(activityTypes.map((type) => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=${type}&regency_id=${encodeURIComponent(regencyId)}`)));
      return pages.flatMap((page) => page.data.items);
    },
  });
  const picks = useQuery({
    queryKey: ['final-report', 'photo-picks', programId, regencyId, slideKey],
    queryFn: () => apiRequest<DataResponse<FinalReportPhotoPick[]>>(`/api/v1/final-report/photo-picks?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}&slide_key=${slideKey}`),
  });
  const pickedIDs = new Set((picks.data?.data ?? []).map((p) => p.activity_media_id));

  const replace = useMutation({
    mutationFn: (ids: string[]) => apiRequest<DataResponse<FinalReportPhotoPick[]>>('/api/v1/final-report/photo-picks', { method: 'PUT', body: JSON.stringify({ program_id: programId, regency_id: regencyId, slide_key: slideKey, activity_media_ids: ids }) }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['final-report', 'photo-picks', programId, regencyId, slideKey] }),
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Pilihan foto belum dapat disimpan.'),
  });

  const toggle = (mediaID: string) => {
    const next = new Set(pickedIDs);
    if (next.has(mediaID)) next.delete(mediaID);
    else {
      if (next.size >= 6) { toast.error('Maksimal 6 foto per slide.'); return; }
      next.add(mediaID);
    }
    replace.mutate([...next]);
  };

  return <section className="space-y-2 rounded-lg border p-4">
    <h3 className="text-sm font-semibold">{SLIDE_KEY_LABELS[slideKey]}</h3>
    {available.isPending
      ? <p className="text-sm text-muted-foreground">Memuat foto...</p>
      : (available.data ?? []).length === 0
        ? <DataState kind="empty" title="Belum ada foto" description="Unggah dokumentasi kegiatan ini terlebih dahulu." />
        : <ul className="grid grid-cols-3 gap-2 sm:grid-cols-6">
          {(available.data ?? []).map((item) => <li key={item.id} className="relative">
            <img src={item.content_url} alt={item.display_name} className="aspect-square w-full rounded border object-cover" />
            {canManage && <Checkbox className="absolute right-1 top-1 bg-background" checked={pickedIDs.has(item.id)} onCheckedChange={() => toggle(item.id)} aria-label={`Pilih ${item.display_name}`} />}
          </li>)}
        </ul>}
  </section>;
}

export default function FinalReportRoute() {
  const [params, setParams] = useSearchParams();
  const programID = params.get('program_id') ?? '';
  const regencyID = params.get('regency_id') ?? '';

  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<ProgramOption[]>>('/api/v1/program-setup/programs') });
  const zones = useQuery({
    queryKey: ['program-setup', 'programs', programID, 'zones'],
    queryFn: () => apiRequest<DataResponse<ProgramZone[]>>(`/api/v1/program-setup/programs/${programID}/zones`),
    enabled: programID !== '',
  });
  const regencies = zones.data?.data.flatMap((zone) => zone.regencies ?? []) ?? [];

  const setProgram = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('program_id', value); else next.delete('program_id'); next.delete('regency_id'); return next; });
  const setRegency = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); return next; });

  return <div className="page">
    <PageHeader title="Laporan Akhir" description="Presentasi ringkasan kegiatan per kabupaten, datanya otomatis mengikuti kondisi lapangan terbaru." />
    <section className="grid gap-4 py-5 sm:grid-cols-2" aria-label="Konteks laporan akhir">
      <div className="grid gap-1.5">
        <Label id="final-report-program-label">Program</Label>
        <Select disabled={programs.isPending} value={programID} onValueChange={setProgram}>
          <SelectTrigger className="w-full" aria-labelledby="final-report-program-label"><SelectValue placeholder={programs.isPending ? 'Memuat program...' : 'Pilih program'} /></SelectTrigger>
          <SelectContent>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <div className="grid gap-1.5">
        <Label id="final-report-regency-label">Kabupaten</Label>
        <Select disabled={!programID || zones.isPending} value={regencyID} onValueChange={setRegency}>
          <SelectTrigger className="w-full" aria-labelledby="final-report-regency-label"><SelectValue placeholder={!programID ? 'Pilih program terlebih dahulu' : zones.isPending ? 'Memuat kabupaten...' : 'Pilih kabupaten'} /></SelectTrigger>
          <SelectContent>{regencies.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
    </section>
    {!programID || !regencyID
      ? <DataState kind="empty" title="Pilih program dan kabupaten terlebih dahulu" description="Pilih program lalu kabupaten untuk melihat Laporan Akhir." />
      : <FinalReportPage programId={programID} regencyId={regencyID} />}
  </div>;
}
```

- [ ] **Step 3: Write `FinalReportPage.test.tsx`**

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { FinalReportPage } from './FinalReportPage';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

afterEach(() => { vi.restoreAllMocks(); });

function renderPage(permissions: string[]) {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path.includes('/final-report/settings')) return Promise.resolve({ data: { program_id: 'p1', regency_id: 'r1', leader_count: 0, admin_count: 0, technician_count: 0, helper_count: 0, machine_manufacturer_name: '', delivery_hose_date: '', delivery_machine_group_date: '', delivery_gas_cylinder_date: '' } });
    if (path.includes('/final-report/documents')) return Promise.resolve({ data: [] });
    if (path.includes('/final-report/photo-picks')) return Promise.resolve({ data: [] });
    if (path.includes('/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter><FinalReportPage programId="p1" regencyId="r1" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

test('renders settings form and photo sections for a manager', async () => {
  renderPage(['final_report.view', 'final_report.manage']);
  expect(await screen.findByText('Susunan Tim & Rincian Pengiriman')).toBeVisible();
  expect(await screen.findByText('Pengiriman & Penerimaan Barang')).toBeVisible();
});

test('hides the settings form for a viewer without manage', async () => {
  renderPage(['final_report.view']);
  await waitFor(() => expect(screen.queryByText('Susunan Tim & Rincian Pengiriman')).not.toBeInTheDocument());
  expect(await screen.findByRole('button', { name: 'Tampilkan' })).toBeVisible();
});
```

- [ ] **Step 4: Run tests**

Run: `cd frontend && npx vitest run src/features/finalreport/FinalReportPage.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/finalreport/types.ts frontend/src/features/finalreport/FinalReportPage.tsx frontend/src/features/finalreport/FinalReportPage.test.tsx
git commit -m "feat(finalreport-ui): add settings form, photo picker, and generate action"
```

---

### Task 13: Frontend — Present mode (live full-screen slideshow)

**Files:**
- Create: `frontend/src/features/finalreport/FinalReportPresent.tsx`
- Create: `frontend/src/features/finalreport/FinalReportPresent.test.tsx`

**Interfaces:**
- Consumes: `GET /api/v1/final-report/preview` (Task 11); `FinalReportPreview` type (Task 12).
- Produces: `export default function FinalReportPresent({ programId, regencyId, onClose }: { programId: string; regencyId: string; onClose: () => void })` — consumed by Task 12's `FinalReportPage`.

- [ ] **Step 1: Write the failing test**

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import FinalReportPresent from './FinalReportPresent';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

afterEach(() => { vi.restoreAllMocks(); });

test('navigates between slides and shows blank-field placeholders', async () => {
  vi.mocked(apiRequest).mockResolvedValue({
    data: {
      regency_label: 'KABUPATEN WAJO', fiscal_year: '2026', agriculture_office_name: 'Belum diisi',
      consultant_company_name: 'PT KSM', machine_manufacturer_name: 'Belum diisi',
      delivery_hose_date: 'Belum diisi', delivery_machine_group_date: 'Belum diisi', delivery_gas_cylinder_date: 'Belum diisi',
      delivery_range_start: 'Belum diisi', delivery_range_end: 'Belum diisi',
      rakorda_date: 'Belum diisi', sosialisasi_date: 'Belum diisi',
      leader_count: '0', admin_count: '0', technician_count: '0', helper_count: '0',
      handover_location: 'Belum diisi', distribution_start_date: 'Belum diisi', distribution_end_date: 'Belum diisi', distribution_total: 'Belum diisi',
      photo_picks: {},
    },
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><FinalReportPresent programId="p1" regencyId="r1" onClose={() => {}} /></QueryClientProvider>);

  expect(await screen.findByText('KABUPATEN WAJO')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Slide berikutnya' }));
  expect(await screen.findAllByText('Belum diisi')).not.toHaveLength(0);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/finalreport/FinalReportPresent.test.tsx`
Expected: FAIL (module doesn't exist)

- [ ] **Step 3: Implement**

```tsx
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { ChevronLeft, ChevronRight, LoaderCircle, X } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import type { FinalReportPreview } from './types';

type DataResponse<T> = { data: T };

export default function FinalReportPresent({ programId, regencyId, onClose }: { programId: string; regencyId: string; onClose: () => void }) {
  const [index, setIndex] = useState(0);
  const preview = useQuery({
    queryKey: ['final-report', 'preview', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportPreview>>(`/api/v1/final-report/preview?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });

  const slides = preview.data ? buildSlides(preview.data.data) : [];
  const total = slides.length;

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key === 'ArrowRight') setIndex((i) => Math.min(i + 1, total - 1));
      if (event.key === 'ArrowLeft') setIndex((i) => Math.max(i - 1, 0));
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [total, onClose]);

  return <div className="fixed inset-0 z-50 flex flex-col bg-black text-white" role="dialog" aria-label="Tampilan Laporan Akhir">
    <div className="flex items-center justify-end p-2"><button type="button" aria-label="Tutup" onClick={onClose}><X /></button></div>
    <div className="flex flex-1 items-center justify-center p-10">
      {preview.isPending ? <LoaderCircle className="size-10 animate-spin" aria-hidden="true" /> : slides[index]}
    </div>
    <div className="flex items-center justify-center gap-6 p-4">
      <button type="button" aria-label="Slide sebelumnya" disabled={index === 0} onClick={() => setIndex((i) => Math.max(i - 1, 0))}><ChevronLeft /></button>
      <span>{index + 1} / {total || 1}</span>
      <button type="button" aria-label="Slide berikutnya" disabled={index >= total - 1} onClick={() => setIndex((i) => Math.min(i + 1, total - 1))}><ChevronRight /></button>
    </div>
  </div>;
}

function buildSlides(data: FinalReportPreview) {
  return [
    <div key="title" className="space-y-4 text-center">
      <h1 className="text-3xl font-bold">LAPORAN KEGIATAN</h1>
      <p>PT KIAN SANTANG MULIATAMA TBK</p>
      <p className="text-2xl">{data.regency_label}</p>
      <p>TAHUN ANGGARAN {data.fiscal_year}</p>
    </div>,
    <div key="pengiriman" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Pengiriman & Penerimaan Barang</h2>
      <p>{data.agriculture_office_name}</p>
      <p>Tanggal {data.delivery_hose_date} (Selang Buang)</p>
      <p>Tanggal {data.delivery_machine_group_date} (Mesin Pompa, Konkit, Oli, Bracket, Name Plate, Selang Hisap)</p>
      <p>Tanggal {data.delivery_gas_cylinder_date} (Tabung Gas 3 Kg)</p>
      <h2 className="pt-4 text-xl font-semibold">Rapat Koordinasi, {data.rakorda_date}</h2>
      <p>{data.consultant_company_name} (Konsultan Pengawas), {data.machine_manufacturer_name} (Pabrikan Mesin)</p>
    </div>,
    <div key="sosialisasi" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Sosialisasi, {data.sosialisasi_date}</h2>
      <h3 className="pt-4 font-semibold">Anggota Tim</h3>
      <p>Leader ({data.leader_count} orang), Administrasi ({data.admin_count} orang), Teknisi ({data.technician_count} orang), Helper ({data.helper_count} orang)</p>
    </div>,
    <div key="pendistribusian" className="max-w-2xl space-y-2">
      <h2 className="text-xl font-semibold">Pendistribusian</h2>
      <p>{data.handover_location}</p>
      <p>Tanggal Mulai {data.distribution_start_date}</p>
      <p>Tanggal Selesai {data.distribution_end_date}</p>
      <p>Total Paket {data.distribution_total} Unit</p>
    </div>,
    ...(['pengiriman_penerimaan', 'rapat_koordinasi', 'sosialisasi'] as const).map((key) => <div key={'docs-' + key} className="grid max-w-3xl grid-cols-3 gap-2">
      {(data.photo_picks[key] ?? []).map((pick) => <img key={pick.id} src={pick.content_url} alt={pick.display_name} className="aspect-video w-full rounded object-cover" />)}
      {(data.photo_picks[key] ?? []).length === 0 && <p className="col-span-3 text-muted-foreground">Belum ada foto dipilih.</p>}
    </div>),
    <p key="thanks" className="text-3xl font-bold">Terima Kasih</p>,
  ];
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/features/finalreport/FinalReportPresent.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/finalreport/FinalReportPresent.tsx frontend/src/features/finalreport/FinalReportPresent.test.tsx
git commit -m "feat(finalreport-ui): add full-screen live Present mode"
```

---

### Task 14: Sidebar + route wiring

**Files:**
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/routes.tsx`

**Interfaces:**
- Consumes: `FinalReportRoute` (default export, Task 12).

- [ ] **Step 1: Add the sidebar item to the existing "Laporan" group**

In `AppShell.tsx`, change:

```tsx
{ label: 'Laporan', items: [
  { label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
] },
```

to:

```tsx
{ label: 'Laporan', items: [
  { label: 'Laporan', to: '/laporan', permission: 'distribution.view', icon: <FileText /> },
  { label: 'Laporan Akhir', to: '/laporan-akhir', permission: 'final_report.view', icon: <Presentation /> },
] },
```

Add `Presentation` to the `lucide-react` import list at the top of `AppShell.tsx` (check the existing import line and append it, rather than adding a second import statement).

- [ ] **Step 2: Add the route**

In `routes.tsx`, add the import:

```tsx
import FinalReportRoute from '../features/finalreport/FinalReportPage';
```

and the route entry (next to `laporan`):

```tsx
{ path: 'laporan-akhir', element: <ProtectedPage permission="final_report.view"><FinalReportRoute /></ProtectedPage> },
```

- [ ] **Step 3: Rebuild and smoke-check**

Run: `cd frontend && npm run typecheck && npm run lint && npm run build`
Expected: all clean; `/laporan-akhir` reachable from the sidebar under "Laporan", showing the Program/Kabupaten picker.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/AppShell.tsx frontend/src/app/routes.tsx
git commit -m "feat(ui): add Laporan Akhir to the Laporan sidebar group"
```

---

## Final Verification

- [ ] `go build ./... && go vet ./...` — clean
- [ ] `TEST_DATABASE_URL=... go test ./...` — all green
- [ ] `cd frontend && npm run typecheck && npm run lint && npx vitest run` — all green
- [ ] Manual smoke test: pick a Program+Kabupaten with some existing Rakorda/Sosialisasi/distribution data, fill susunan tim + delivery dates, pick photos, click "Tampilkan" (verify live data + Esc/arrow navigation), click "Generate PPTX" (verify it appears in Drive directly inside the kabupaten folder, sibling to the 3 category folders, and downloads/opens correctly in PowerPoint with photos and text in place), click "Generate PPTX" again (verify version increments to 2 and the old Drive file is replaced).
