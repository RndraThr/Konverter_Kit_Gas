# Business Text Uppercase Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Use `superpowers:test-driven-development` for every behavior change and `superpowers:verification-before-completion` before claiming completion.

**Goal:** Make every newly entered or edited business-facing text value uppercase immediately in the UI, persist it uppercase through backend/import boundaries, and render legacy mixed-case data uppercase in all current Berita Acara PDFs.

**Architecture:** Use three explicit layers. The frontend provides immediate visual normalization without trimming while the user types. Backend domain services are the canonical persistence boundary and trim plus uppercase only classified business fields. PDF renderers uppercase presentation values so old records remain visually consistent without a database migration. Technical identifiers and case-sensitive values are deliberately excluded.

**Tech Stack:** Go services and tests, React 19 + TypeScript, Vitest + Testing Library, go-pdf/fpdf.

**Design Reference:** `docs/superpowers/specs/2026-10-02-business-text-uppercase-design.md`

---

## Global constraints

- Do not mass-update existing database rows.
- Do not apply global JSON middleware or global CSS `text-transform`; normalization must be attached to classified fields.
- Preserve email, username, password, URL/path, filename, MIME type, storage key, checksum, UUID, NIK, phone, dates, locale, timezone, date format, template keys, map keys, option codes, slot codes, component codes, status values, and other machine-facing identifiers.
- DCP3 `SourceValues` and uploaded source headers remain unchanged for auditability. Only normalized recipient fields become uppercase.
- Frontend conversion must not trim or collapse whitespace while typing. Backend normalization owns trimming before persistence.
- Go `strings.ToUpper` is the persistence/PDF canonicalizer. The browser helper uses `toLocaleUpperCase('id-ID')` for immediate display.
- Preserve unrelated working-tree changes. Stage only explicitly listed files if commits are requested; never use `git add .`.

## Review focus

1. A mixed-case Unicode name such as `Siti Núraeni` becomes `SITI NÚRAENI` without losing characters.
2. A user can still type spaces naturally; the frontend does not trim mid-entry.
3. Codes such as `machine_option_code: "shark-spwp8030"` and keys such as `machine_options` do not change.
4. DCP3 raw source cells remain byte-for-byte equivalent while normalized fields become uppercase.
5. PDFs created from legacy mixed-case snapshots display business values in uppercase in BA Perorangan, DP3, and Rekapitulasi Harian.

---

### Task 1: Add shared uppercase normalization primitives

**Files:**

- Create: `internal/textnorm/business.go`
- Create: `internal/textnorm/business_test.go`
- Create: `frontend/src/lib/text.ts`
- Create: `frontend/src/lib/text.test.ts`

**Step 1: Write failing Go tests**

Cover trimming, Unicode uppercasing, blank values, and preservation of punctuation/numbers:

```go
func TestBusinessUpperTrimsAndUppercasesUnicode(t *testing.T) {
    if got := BusinessUpper("  Siti Núraeni / Blok 2  "); got != "SITI NÚRAENI / BLOK 2" {
        t.Fatalf("BusinessUpper() = %q", got)
    }
}
```

Run: `go test ./internal/textnorm`

Expected: FAIL because the package/helper does not exist.

**Step 2: Implement the Go primitive**

```go
package textnorm

import "strings"

func BusinessUpper(value string) string {
    return strings.ToUpper(strings.TrimSpace(value))
}
```

**Step 3: Write the failing frontend helper test**

Assert that `uppercaseBusinessText('Siti Núraeni  ')` returns `SITI NÚRAENI  `, proving that typing-time normalization does not trim spaces.

Run: `npm.cmd test -- --run src/lib/text.test.ts` from `frontend`.

Expected: FAIL because `text.ts` does not exist.

**Step 4: Implement the frontend primitive**

```ts
export function uppercaseBusinessText(value: string): string {
  return value.toLocaleUpperCase('id-ID')
}
```

**Step 5: Verify task**

Run:

```powershell
go test ./internal/textnorm
cd frontend
npm.cmd test -- --run src/lib/text.test.ts
```

Expected: PASS.

**Step 6: Commit checkpoint (only when commits are authorized)**

```powershell
git add internal/textnorm/business.go internal/textnorm/business_test.go frontend/src/lib/text.ts frontend/src/lib/text.test.ts
git commit -m "feat: add business text uppercase helpers"
```

---

### Task 2: Canonicalize program setup and template business values

**Files:**

- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`

**Step 1: Add failing service tests**

Extend focused tests to send mixed-case values and capture repository input for:

- Regency: `ProvinceName`, `Name`, and `Notes` uppercase; `DocumentCode` remains its existing canonical uppercase behavior.
- Program: `Name` and `Notes` uppercase; `Code`, status, and program type retain their current identifier rules.
- Schedule: `Name`, `Notes`, and `SupervisorName` uppercase; IDs and dates unchanged.
- Zone: `Name` uppercase; code retains its stable-code normalization.
- Package template: template `Name` and known display members inside `Values` uppercase.
- Documentation template: template `Name`, slot `Label`, and slot `Instructions` uppercase.

For package template `Values`, explicitly assert:

```go
// Uppercase display values:
// machine_options[].brand/type/power/fuel_type
// hose_options[].brand/spec/suction_brand/suction_spec/discharge_brand/discharge_spec
// converter_options[].brand/spec
// components[].label/unit
// Preserve machine/hose/converter/component `code` and all map keys.
```

Run: `go test ./internal/programs`

Expected: FAIL because several fields are only trimmed today.

**Step 2: Add explicit normalizers in the programs service**

Import `konkit/internal/textnorm` and replace trimming only for business fields with `textnorm.BusinessUpper`.

Add a narrowly scoped template-value normalizer that:

- copies/normalizes only the known arrays and display properties listed above;
- never recursively uppercases arbitrary map values;
- does not modify `code`, structural keys, booleans, or numbers;
- returns values in the same JSON-compatible shapes expected by validation/repository code.

**Step 3: Verify the package**

Run: `go test ./internal/programs`

Expected: PASS, including existing validation tests.

**Step 4: Commit checkpoint (only when commits are authorized)**

```powershell
git add internal/programs/service.go internal/programs/service_test.go
git commit -m "feat: uppercase program setup business values"
```

---

### Task 3: Canonicalize recipient, account, settings, and BA settings writes

**Files:**

- Modify: `internal/recipients/service.go`
- Modify: `internal/recipients/service_test.go`
- Modify: `internal/administration/service.go`
- Modify: `internal/administration/service_test.go`
- Modify: `internal/profile/service.go`
- Modify: `internal/profile/service_test.go`
- Modify: `internal/settings/service.go`
- Modify: `internal/settings/service_test.go`
- Modify: `internal/bast/schedule_settings.go`
- Modify: `internal/bast/schedule_settings_service_test.go`

**Step 1: Write failing recipient tests**

For both create and update, assert uppercase persistence of `FullName`, `SectorIdentifier`, `Address`, `Village`, and `District`. Assert NIK, phone, schedule ID, and machine option code are unchanged except their existing validation/trim behavior.

Run: `go test ./internal/recipients`

Expected: FAIL on currently trim-only business fields.

**Step 2: Implement recipient normalization**

Use `textnorm.BusinessUpper` only for the five classified business fields. Keep numerical identifiers and technical option codes out of this helper.

**Step 3: Write failing identity and role tests**

Assert:

- user/profile `FullName` becomes uppercase;
- username and email remain lowercased according to existing behavior;
- passwords are untouched apart from existing password validation/hashing;
- role `Name` and `Description` become uppercase;
- role `Code` and permission codes retain existing stable identifier behavior.

Run: `go test ./internal/administration ./internal/profile`

Expected: FAIL on display names/descriptions.

**Step 4: Implement identity and role normalization**

Use `textnorm.BusinessUpper` in the existing boundary helpers, without changing username/email normalization or authentication behavior.

**Step 5: Write failing system-setting tests**

Use one payload containing all registered setting keys. Assert only `application_name` and `organization_name` become uppercase. Assert `timezone`, `date_format`, and `locale` exactly match their allowed values.

Run: `go test ./internal/settings`

Expected: FAIL for mixed-case application/organization names.

**Step 6: Implement allowlisted setting normalization**

Add an explicit business-text setting-key set or switch. Apply `BusinessUpper` only to `application_name` and `organization_name`, then run existing allowlist/max-length validation. Do not uppercase enum settings.

**Step 7: Write failing BA schedule-setting tests**

Update `TestScheduleSettingsPutTrimsAndPersists` to require uppercase for:

- `handover_location`
- `consultant_company_name`
- `agriculture_office_name`
- `installer_name`
- `supervisor_name`
- `pertamina_rep_name`

Keep NIP digits unchanged.

Run: `go test ./internal/bast -run ScheduleSettings`

Expected: FAIL on mixed-case inputs.

**Step 8: Implement BA schedule-setting normalization and verify**

Use `BusinessUpper` for named business fields and retain normal trim/validation for NIP.

Run:

```powershell
go test ./internal/recipients ./internal/administration ./internal/profile ./internal/settings
go test ./internal/bast -run ScheduleSettings
```

Expected: PASS.

**Step 9: Commit checkpoint (only when commits are authorized)**

```powershell
git add internal/recipients/service.go internal/recipients/service_test.go internal/administration/service.go internal/administration/service_test.go internal/profile/service.go internal/profile/service_test.go internal/settings/service.go internal/settings/service_test.go internal/bast/schedule_settings.go internal/bast/schedule_settings_service_test.go
git commit -m "feat: uppercase business values at service boundaries"
```

---

### Task 4: Canonicalize distribution and DCP3 normalized data

**Files:**

- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/dcp3/service.go`
- Modify: `internal/dcp3/service_test.go`

**Step 1: Add failing distribution tests**

Exercise the slot creation/update and recipient-link paths with mixed-case input. Assert:

- machine, hose, and converter serial numbers become uppercase;
- link input `SectorIdentifier`, `Address`, `Village`, and `District` become uppercase;
- phone, NIK, option codes, slot IDs, and schedule IDs remain unchanged.

If repository spies do not expose these calls yet, extend the local test spy only enough to capture the existing service arguments.

Run: `go test ./internal/distribution`

Expected: FAIL on trim-only values.

**Step 2: Implement distribution normalization**

Call `BusinessUpper` at the service boundary for serial and classified link display fields. Do not alter media filenames, MIME types, storage keys, or option codes.

**Step 3: Extend DCP3 row-normalization tests**

Use mixed-case source cells for name, address, village, district, and sector identifier. Assert:

- normalized row fields are uppercase;
- normalized NIK/phone retain existing formatting behavior;
- `SourceValues` still contain the original mixed-case strings;
- mapping/header keys remain unchanged;
- farmer and fisherman identifiers both follow the rule.

Run: `go test ./internal/dcp3`

Expected: FAIL for normalized display fields while raw-value assertions should already pass.

**Step 4: Implement DCP3 normalized-field uppercase**

Apply `BusinessUpper` only after extracting mapped source values into the normalized row. Do not mutate the source map. Keep `normalizeIdentifier` behavior for canonical sector identifiers and ensure it produces uppercase.

**Step 5: Verify task**

Run:

```powershell
go test ./internal/distribution
go test ./internal/dcp3
```

Expected: PASS.

**Step 6: Commit checkpoint (only when commits are authorized)**

```powershell
git add internal/distribution/service.go internal/distribution/service_test.go internal/dcp3/service.go internal/dcp3/service_test.go
git commit -m "feat: uppercase distribution and dcp3 business data"
```

---

### Task 5: Apply instant uppercase behavior to business-data forms

**Files:**

- Modify: `frontend/src/features/dashboard/RecipientDialog.tsx`
- Modify: `frontend/src/features/distribution/SlotDokumenSection.tsx`
- Modify: `frontend/src/features/distribution/SlotMesinCreate.tsx`
- Modify: `frontend/src/features/berita-acara/ScheduleSettingsPanel.tsx`
- Modify: `frontend/src/features/programs/RegenciesPanel.tsx`
- Modify: `frontend/src/features/programs/ProgramsPanel.tsx`
- Modify: `frontend/src/features/programs/SchedulesPanel.tsx`
- Modify: `frontend/src/features/programs/TemplatesPanel.tsx`
- Modify: `frontend/src/features/programs/ZonesPanel.tsx`
- Modify: `frontend/src/features/users/UserDialog.tsx`
- Modify: `frontend/src/features/roles/RoleDialog.tsx`
- Modify: `frontend/src/features/profile/ProfilePage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Create: `frontend/src/features/dashboard/RecipientDialog.test.tsx`
- Create: `frontend/src/features/programs/TemplatesPanel.test.tsx`
- Create: `frontend/src/features/users/UserDialog.test.tsx`

**Step 1: Write focused failing UI tests**

Using Testing Library/user-event, prove three representative boundaries:

1. `RecipientDialog`: typing `Jl. Melati` shows `JL. MELATI`, while phone/NIK stay numeric and are not sent through the uppercase helper.
2. `TemplatesPanel`: typing display values uppercases brand/type/power/fuel/labels/units, while option/component codes retain their intended lowercase stable-key behavior.
3. `UserDialog`: full name uppercases immediately, while username, email, and password preserve typed casing until their existing backend rules apply.

Mock query/mutation dependencies at module boundaries so tests focus on controlled form state.

Run:

```powershell
cd frontend
npm.cmd test -- --run src/features/dashboard/RecipientDialog.test.tsx src/features/programs/TemplatesPanel.test.tsx src/features/users/UserDialog.test.tsx
```

Expected: FAIL for fields not yet wired to the helper.

**Step 2: Replace ad-hoc uppercasing with the shared helper**

Import `uppercaseBusinessText` and use it in every business field setter. Existing `.toUpperCase()` sites must also move to the helper for one consistent rule.

Apply to:

- Recipient: full name, sector identifier, address, village, district.
- Distribution: sector identifier, address, village, district, and three serial-number inputs.
- BA settings: handover location, consultant company, office name, installer, supervisor, Pertamina representative. Exclude NIP.
- Regency/program/schedule/zone: all names, province, notes, supervisor, and business-facing codes already intended as uppercase. Exclude IDs, dates, types, statuses, and select values.
- Package/document template: template display names; machine brand/type/power/fuel; hose brands/specs; converter brand/spec; component label/unit; documentation slot label/instructions. Preserve lowercase stable codes.
- Account/profile: full names only. Role name/description uppercase; preserve role code.
- Settings: uppercase only `application_name` and `organization_name`; leave select-backed enum settings untouched.

**Step 3: Preserve form usability**

Confirm handlers pass the entire raw input to `uppercaseBusinessText` and do not call `.trim()`. Do not add a CSS-only transform because submitted state must already be uppercase.

**Step 4: Run focused and static verification**

Run:

```powershell
cd frontend
npm.cmd test
npm.cmd run typecheck
npm.cmd run lint
```

Expected: PASS. Existing lint warning count must not increase beyond configured allowance.

**Step 5: Commit checkpoint (only when commits are authorized)**

```powershell
git add frontend/src/features/dashboard/RecipientDialog.tsx frontend/src/features/distribution/SlotDokumenSection.tsx frontend/src/features/distribution/SlotMesinCreate.tsx frontend/src/features/berita-acara/ScheduleSettingsPanel.tsx frontend/src/features/programs/RegenciesPanel.tsx frontend/src/features/programs/ProgramsPanel.tsx frontend/src/features/programs/SchedulesPanel.tsx frontend/src/features/programs/TemplatesPanel.tsx frontend/src/features/programs/ZonesPanel.tsx frontend/src/features/users/UserDialog.tsx frontend/src/features/roles/RoleDialog.tsx frontend/src/features/profile/ProfilePage.tsx frontend/src/features/settings/SettingsPage.tsx frontend/src/features/dashboard/RecipientDialog.test.tsx frontend/src/features/programs/TemplatesPanel.test.tsx frontend/src/features/users/UserDialog.test.tsx
git commit -m "feat: uppercase business text while typing"
```

---

### Task 6: Uppercase legacy business values in all current BA PDF renderers

**Files:**

- Create: `internal/bast/render_text.go`
- Create: `internal/bast/render_text_test.go`
- Modify: `internal/bast/pdf_renderer.go`
- Modify: `internal/bast/pdf_renderer_test.go`
- Modify: `internal/bast/dp3_renderer.go`
- Modify: `internal/bast/dp3_test.go`
- Modify: `internal/bast/daily_recap_renderer.go`
- Modify: `internal/bast/daily_recap_test.go`

**Step 1: Write a failing renderer presentation-helper test**

Add a package-local helper test proving Unicode business text is trimmed and uppercased. The helper should delegate to the same canonical Go primitive:

```go
func renderBusinessText(value string) string {
    return textnorm.BusinessUpper(value)
}
```

Run: `go test ./internal/bast -run RenderBusinessText`

Expected: FAIL because the helper does not exist.

**Step 2: Add failing BA Perorangan PDF regression**

Build a legacy snapshot containing mixed-case recipient name/address/location, equipment display values, component label/unit, and signature names. Render it, decode PDF streams with the existing helper, then assert uppercase values are present and their mixed-case forms are absent.

Do not uppercase document number, NIK, phone, dates, or structural event strings.

Run: `go test ./internal/bast -run 'PetaniBundle.*Uppercase'`

Expected: FAIL before renderer wiring.

**Step 3: Wire BA Perorangan presentation values**

Apply the helper only where business values are passed into cells/text drawing. Normalize recipient/location, equipment, component label/unit, and signature display names. Avoid mutating persisted snapshots.

**Step 4: Add failing DP3 PDF regression and implement**

Construct a mixed-case `DP3Snapshot` covering header/location, recipient identity/location, equipment, and signatories. Assert decoded/rendered PDF business values are uppercase. Then apply `renderBusinessText` at DP3 drawing boundaries, leaving numbers/dates/IDs intact.

Run: `go test ./internal/bast -run 'DP3.*Uppercase'`

Expected after implementation: PASS.

**Step 5: Add failing Daily Recap PDF regression and implement**

Construct a mixed-case `DailyRecapSnapshot` covering header/location, grouped equipment descriptions, recipient rows, and signatories. Assert uppercase business values and absence of mixed-case equivalents. Apply the helper at drawing boundaries without changing grouping keys, quantities, document numbers, or dates.

Run: `go test ./internal/bast -run 'DailyRecap.*Uppercase'`

Expected after implementation: PASS.

**Step 6: Verify all BAST tests**

Run: `go test ./internal/bast`

Expected: PASS, including existing layout/page-count/content assertions.

**Step 7: Commit checkpoint (only when commits are authorized)**

```powershell
git add internal/bast/render_text.go internal/bast/render_text_test.go internal/bast/pdf_renderer.go internal/bast/pdf_renderer_test.go internal/bast/dp3_renderer.go internal/bast/dp3_test.go internal/bast/daily_recap_renderer.go internal/bast/daily_recap_test.go
git commit -m "feat: render berita acara business values uppercase"
```

---

### Task 7: Full verification and scope audit

**Files:**

- Review: every file changed in Tasks 1-6
- Update only if verification exposes a real issue: tests or implementation files already listed above

**Step 1: Format code**

Run:

```powershell
gofmt -w internal/textnorm/business.go internal/textnorm/business_test.go internal/programs/service.go internal/programs/service_test.go internal/recipients/service.go internal/recipients/service_test.go internal/administration/service.go internal/administration/service_test.go internal/profile/service.go internal/profile/service_test.go internal/settings/service.go internal/settings/service_test.go internal/bast/schedule_settings.go internal/bast/schedule_settings_service_test.go internal/distribution/service.go internal/distribution/service_test.go internal/dcp3/service.go internal/dcp3/service_test.go internal/bast/render_text.go internal/bast/render_text_test.go internal/bast/pdf_renderer.go internal/bast/pdf_renderer_test.go internal/bast/dp3_renderer.go internal/bast/dp3_test.go internal/bast/daily_recap_renderer.go internal/bast/daily_recap_test.go
```

**Step 2: Run backend verification**

Run:

```powershell
go test -p 1 ./...
go vet ./...
```

Expected: both exit 0.

**Step 3: Run frontend verification**

Run from `frontend`:

```powershell
npm.cmd test
npm.cmd run typecheck
npm.cmd run lint
npm.cmd run build
```

Expected: all exit 0.

**Step 4: Audit exclusions and missed business setters**

Run targeted searches:

```powershell
rg -n "\.toUpperCase\(|toLocaleUpperCase" frontend/src
rg -n "event\.target\.value|e\.target\.value" frontend/src/features
rg -n "strings\.TrimSpace" internal/programs internal/recipients internal/distribution internal/dcp3 internal/administration internal/profile internal/settings internal/bast
```

Review every result against the classification in Global constraints. Search/filter boxes are not persisted business values and may remain unmodified. Any remaining persisted business setter must use the shared helper; any excluded field must retain its current behavior.

**Step 5: Check diff hygiene**

Run:

```powershell
git diff --check
git status --short
git diff --stat
```

Expected: no whitespace errors; only intentional files appear. Do not discard unrelated pre-existing changes.

**Step 6: Manual acceptance pass**

With backend/frontend started manually by the operator:

1. Create a recipient with mixed-case name/address/location and confirm characters become uppercase during typing and persisted values reload uppercase.
2. Enter mixed-case machine/hose/converter serials and confirm reload is uppercase.
3. Edit template display text and confirm its stable codes are unchanged.
4. Import a mixed-case DCP3 row and confirm recipient/candidate display values are uppercase while preview/audit raw source remains faithful.
5. Generate BA Perorangan, DP3, and Rekapitulasi Harian using at least one legacy mixed-case record and confirm all business display values in the PDFs are uppercase.
6. Confirm login/profile email, username, password, file uploads, and technical codes still work unchanged.

**Step 7: Final commit checkpoint (only when commits are authorized)**

If verification required follow-up edits, stage only those explicit files and create a focused verification-fix commit. Otherwise do not create an empty commit.

