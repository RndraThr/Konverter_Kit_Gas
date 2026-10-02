# DP3 & Rekapitulasi Harian — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menambahkan dua jenis Berita Acara kolektif pada ruang kerja Berita Acara: **DP3 (Daftar Penerima Paket Perdana)** bersumber dari daftar nominatif jadwal, dan **Rekapitulasi Harian** bersumber dari distribusi selesai per tanggal lokal. Keduanya memakai pipeline dokumen agregat bersama (preview, finalisasi, snapshot, versioning, audit, unduh, sinkronisasi Drive) dengan renderer & query sumber yang khusus.

**Architecture:** Additive-first. Semua perubahan backend menambah tabel/module baru di `internal/bast` tanpa mengubah pipeline BA Perorangan yang sudah aktif. Query sumber dan renderer tetap terpisah per jenis dokumen. Snapshot disimpan sebelum render final; renderer final hanya membaca snapshot.

**Tech Stack:** Go 1.26 (pgx/v5, goose, net/http, go-pdf/fpdf), React 19 + TypeScript (Vite, TanStack Query, Vitest), PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-10-02-dp3-rekap-harian-design.md`

## Global Constraints

- Seluruh query DP3 & Rekap Harian memakai `schedule_id` sebagai batas data; program & kabupaten hanya atribut turunan + pemeriksaan scope.
- Membaca memerlukan `bast.view`; mutasi (konfigurasi & finalisasi) memerlukan `bast.manage`. Semua endpoint menerapkan regency scope; di luar scope → `404`.
- DP3 mencakup alokasi `ready` + `distributed` (bukan `candidate`/`needs_review`/`replaced`/`cancelled`), tidak bergantung `distribution_number`.
- Rekap Harian mencakup `distribution_slots.status='completed'` dengan `distributed_at` pada tanggal lokal `Asia/Jakarta`.
- Nomor bagi hanya ditetapkan saat mounting (sudah ada di `distribution.LinkSlot`); penerima manual disimpan tanpa `distribution_number` (migration 00028 sudah berlaku).
- Barang per penerima bersumber dari: (1) snapshot verifikasi distribusi, (2) snapshot alokasi saat nominasi; tanpa fallback diam-diam ke template terkini.
- Snapshot alokasi menyimpan satu `selected_machine` per penerima (`code`, `brand`, `type`, `power`, `fuel_type`).
- Dokumen final immutable & idempotent: checksum sama → kembalikan versi aktif tanpa upload ulang; checksum berbeda → versi baru, versi lama `superseded` (file tetap untuk audit).
- `storage_key` adalah string opaque (Google Drive file ID), wajib kolom `text`.
- Folder Drive: `{ROOT}/PETANI/{ZONE}/{REGENCY}/BERITA ACARA (BA)/1. DP3/` dan `.../3. REKAP HARIAN/`. Nama file: `DP3 - {KABUPATEN} - {YYYY-MM-DD} - V{version}.pdf` / `REKAP HARIAN - {KABUPATEN} - {YYYY-MM-DD} - V{version}.pdf`.
- Setiap task: `go build ./...`, `go vet ./...`, paket yang disentuh lulus `go test ./... -count=1` (unit) dan `npm run -s typecheck` + `npm run -s build` (frontend) tetap hijau.
- **Ruling (deviasi spec):** PRD §24 menganjurkan satu migration per concern. Plan memecah migration additive kecil (power/fuel_type → settings → aggregate documents) agar tiap commit hijau. Hasil akhir pada DB fresh identik.
- **Reference PDF (authority):** renderer Go DP3 & Rekap Harian yang diberikan pengguna adalah otoritas visual (lihat Task 7 & 11). Salin ke `docs/references/` saat task renderer agar travel dengan repo.

---

## File Structure (ringkasan)

- `internal/database/migrations/00030_machine_power_fuel.sql` — **create** tambah `power`/`fuel_type` ke machine_options template.
- `internal/database/migrations/00031_bast_schedule_settings.sql` — **create** tabel konfigurasi BA per jadwal + seed `supervisor_name`.
- `internal/database/migrations/00032_bast_aggregate_documents.sql` — **create** tabel dokumen agregat + index versi aktif unik.
- `internal/bast/allocation_snapshot.go` + test — **create** builder snapshot alokasi `selected_machine`.
- `internal/bast/schedule_settings.go`, `schedule_settings_repository.go`, `schedule_settings_service.go` + tests — **create** konfigurasi BA per jadwal.
- `internal/bast/aggregate.go`, `aggregate_repository.go`, `aggregate_service.go` + tests — **create** lifecycle dokumen agregat.
- `internal/bast/dp3_source.go`, `dp3_snapshot.go`, `dp3_renderer.go` + tests — **create** DP3.
- `internal/bast/daily_recap_source.go`, `daily_recap_snapshot.go`, `daily_recap_renderer.go` + tests — **create** Rekap Harian.
- `internal/bast/{models,repository}.go` — **modify** tambah struct/error untuk dokumen agregat.
- `internal/dcp3/repository.go` — **modify** tulis `selected_machine` ke `package_snapshot_json`.
- `internal/recipients/{repository,service,models}.go` — **modify** tulis `selected_machine` ke snapshot alokasi manual.
- `internal/programs/{service,models}.go` + test — **modify** validasi `power`/`fuel_type`.
- `internal/api/{handler,routes,bast_routes}.go` + tests — **modify** service interfaces, route, error mapping.
- `frontend/src/features/programs/{types.ts,TemplatesPanel.tsx}` — **modify** field power/fuel_type.
- `frontend/src/features/berita-acara/{types.ts,BeritaAcaraPage.tsx,DP3Panel.tsx,DailyRecapPanel.tsx,ScheduleSettingsPanel.tsx}` — **modify/create**.

---

## Task 1: Migration + validasi power/fuel_type machine_options

**Files:** create `00030_machine_power_fuel.sql`; modify `internal/programs/service.go`, `frontend/src/features/programs/types.ts`, `frontend/src/features/programs/TemplatesPanel.tsx`.

**Interfaces:**
- `MachineOption` jadi `{ code, brand, type, power, fuel_type }`.
- `hasEquipmentOptions` → `validOptionList(values["machine_options"], "brand", "type")` plus `power`/`fuel_type` non-kosong per opsi.

- [ ] **Step 1:** Migration update `values_json` machine_options KONKIT-2026 & PETANI-LPG menambahkan `power`/`fuel_type`.
- [ ] **Step 2:** Update validasi `validOptionList`/`hasEquipmentOptions` agar `power` & `fuel_type` wajib untuk publish.
- [ ] **Step 3:** Update `MachineOption` type + form machine options di TemplatesPanel.
- [ ] **Step 4:** Build + vet + unit test programs + typecheck.

## Task 2: Snapshot alokasi `selected_machine`

> **Selesai termasuk Task 2b:** pemetaan varian mesin DCP3 untuk multi-varian juga diimplementasikan — `Mapping.MachineOption` (kolom sumber), `programs.ResolveMachineCodeFromCell` (cocokkan kode/merek/"merek tipe"), resolusi per-baris di `dcp3.Commit`, `machine_options` di respons preview DCP3, dan field "Varian mesin" di `ColumnMappingStep` saat template multi-varian.

**Files:** create `internal/bast/allocation_snapshot.go` (+ test); modify `internal/dcp3/repository.go`, `internal/recipients/{models,service,repository}.go`.

**Interfaces:**
- `BuildAllocationSnapshot(packageValuesJSON, selectedMachineCode)` → `{ selected_machine: {code,brand,type,power,fuel_type}, package_template_version_id }`. Jika `selectedMachineCode` kosong tapi template punya tepat 1 opsi → auto-select; >1 opsi & kosong → tandai `allocation_snapshot_incomplete` (field `selected_machine` null).
- DCP3 `Commit`: ganti `packageSnapshot` = full `pt.values_json` menjadi hasil `BuildAllocationSnapshot`. (DCP3 mapping varian mesin untuk multi-varian ditunda ke Task 2b bila diperlukan; fase 1 auto-select satu varian.)
- `recipients.Create`: bangun snapshot alokasi dari `ps.package_template_version_id` + `CreateInput.MachineOptionCode` (auto-select bila satu varian).

- [ ] **Step 1:** Builder + unit test.
- [ ] **Step 2:** DCP3 Commit memakai builder (auto-select single variant).
- [ ] **Step 3:** Manual recipient memakai builder (auto-select single; multi → incomplete).
- [ ] **Step 4:** Build + vet + test dcp3/recipients/bast.

## Task 3-4: Konfigurasi BA per jadwal

**Files:** create `00031_bast_schedule_settings.sql`, `internal/bast/schedule_settings*.go` (+ tests); modify `internal/api/{handler,routes,bast_routes}.go`.

**Interfaces:**
- Tabel `bast_schedule_settings(schedule_id PK/FK, handover_location, consultant_company_name, agriculture_office_name, agriculture_office_nip, installer_name, supervisor_name, pertamina_rep_name, timestamps)`.
- Migration seed `supervisor_name` dari `program_schedules.supervisor_name`.
- `ScheduleSettingsService.Get/Put(ctx, actor, scheduleID, scope, meta)`.
- API: `GET/PUT /api/v1/bast/schedules/{schedule_id}/settings`.

- [ ] **Step 1:** Migration + seed.
- [ ] **Step 2:** Domain + repository + service + integration/unit test.
- [ ] **Step 3:** Route + handler + error mapping + handler test.
- [ ] **Step 4:** Build + vet + test.

## Task 5-6: Schema & repository dokumen agregat + lifecycle

**Files:** create `00032_bast_aggregate_documents.sql`, `internal/bast/aggregate*.go` (+ tests).

**Interfaces:**
- Tabel `bast_aggregate_documents(id, schedule_id, program_id, regency_id, document_type('dp3'|'daily_recap'), document_date, filename, recipient_count, page_count, version, status('active'|'superseded'), checksum char(64), storage_key text, snapshot_json, last_error, finalized_by, finalized_at, timestamps)`.
- Unique: satu `active` per `(schedule_id, document_type, document_date)`; version unik per kunci logis.
- `AggregateService.Finalize(ctx, actor, AggregateRequest, scope, meta)` — build snapshot → checksum → idempotent activate (advisory lock `pg_advisory_xact_lock` per kunci logis) → supersede lama → cleanup file gagal.
- `AggregateService.Open(ctx, id, scope)`, `List(ctx, scheduleID, docType, date, scope)`, `GetActive(...)`.

- [ ] **Step 1:** Migration (tabel + partial unique index aktif + version unique).
- [ ] **Step 2:** Repository (activate/supersede/list/get + advisory lock) + integration test.
- [ ] **Step 3:** Service lifecycle + unit test (idempotensi, versioning, cleanup).
- [ ] **Step 4:** Build + vet + test.

## Task 7-10: DP3

**Files:** create `internal/bast/dp3_*.go` (+ tests); modify `internal/api/bast_routes.go`, `frontend/src/features/berita-acara/{types.ts,BeritaAcaraPage.tsx}` + create `DP3Panel.tsx`.

**Interfaces:**
- `DP3Service.Summary(ctx, scheduleID, scope)` → total nominatif, jumlah bernomor & belum ter-mount, status validasi.
- `DP3Service.Recipients(ctx, scheduleID, scope)` → daftar penerima terurut (bernomor asc, lalu belum-mount by created_at+id).
- `DP3Service.Preview/Finalize(ctx, actor, {schedule_id, document_date}, scope, meta)` → PDF / versi aktif.
- `DP3Service.Documents(ctx, scheduleID, date, scope)` + `OpenContent(ctx, id, scope)`.
- API: `GET /bast/dp3/summary`, `GET /bast/dp3/recipients`, `POST /bast/dp3/preview`, `POST /bast/dp3/finalize`, `GET /bast/dp3/documents`, `GET /bast/dp3/documents/{id}/content`.
- Renderer A4 landscape: header logo+judul `DAFTAR PENERIMA PAKET PERDANA` + `(FORM DP3)`, metadata Hari/Tanggal+Lokasi+Konsultan Distribusi, tabel (No, Nama Petani, Alamat, No. KTP, Data Mesin: Merek/Tipe/Daya/Jenis BBM, Paraf), header ulang multi-page, halaman tanda tangan 3 kolom.

- [ ] **Step 1:** Source loader + snapshot + validation (priority verifikasi > alokasi).
- [ ] **Step 2:** Renderer PDF + renderer test.
- [ ] **Step 3:** API endpoints + handler tests.
- [ ] **Step 4:** UI DP3Panel + tab wiring.
- [ ] **Step 5:** Build + vet + test + typecheck.

## Task 11-14: Rekap Harian

**Files:** create `internal/bast/daily_recap_*.go` (+ tests); modify `internal/api/bast_routes.go`; create `frontend/src/features/berita-acara/DailyRecapPanel.tsx`.

**Interfaces:**
- `DailyRecapService.Dates(ctx, scheduleID, scope)` → daftar tanggal distribusi selesai (Asia/Jakarta).
- `DailyRecapService.Recipients(ctx, scheduleID, date, scope)` → baris terurut `slot_number`.
- `DailyRecapService.Preview/Finalize/Documents/OpenContent`.
- API: `GET /bast/daily-recap/dates`, `GET /bast/daily-recap/recipients`, `POST .../preview`, `POST .../finalize`, `GET .../documents`, `GET .../documents/{id}/content`.
- Renderer A4 portrait: judul `BERITA ACARA SERAH TERIMA` + `(FORM REKAPITULASI PENERIMA PAKET)`, metadata Hari/Tanggal+Lokasi+Konsultan Distribusi, tabel (No, Nama Petani, No. Kartu Petani, Merek Mesin, Tipe Mesin, No. Seri Mesin), Grand Total per varian, halaman tanda tangan 4 kolom (+ PT Pertamina Patra Niaga).

- [ ] **Step 1:** Source loader + snapshot + validation (verification snapshot wajib; grouping varian).
- [ ] **Step 2:** Renderer PDF + renderer test.
- [ ] **Step 3:** API endpoints + handler tests.
- [ ] **Step 4:** UI DailyRecapPanel + tab wiring.
- [ ] **Step 5:** Build + vet + test + typecheck.

## Task 15-16: Drive + audit + concurrency wiring + QA

**Files:** modify `internal/bast/{dp3_service,daily_recap_service,aggregate_service}.go`, `internal/api/routes.go`; test files.

**Interfaces:**
- Finalisasi upload ke folder Drive via `media.BuildFolderPath` + `media.PutNamed` dengan nama `DP3 - {KABUPATEN} - {date} - V{version}.pdf`.
- Audit events: `bast.schedule_settings_updated`, `bast.dp3_finalized`, `bast.daily_recap_finalized`, `bast.aggregate_document_downloaded`.
- Advisory lock per kunci logis; cleanup file gagal dicatat `last_error`.

- [ ] **Step 1:** Wire Drive upload + filename sanitasi + audit events.
- [ ] **Step 2:** Concurrency (advisory lock) + idempotensi test.
- [ ] **Step 3:** Unit/integration/renderer/frontend test lengkap.
- [ ] **Step 4:** e2e seed + alur e2e (manual → DP3 → mounting → distribusi → Rekap Harian).
- [ ] **Step 5:** Visual QA PDF DP3 & Rekap Harian; `go test ./...`, `go vet ./...`, `npm test`, `npm run build`.

---

## Acceptance Criteria (ringkasan dari PRD §23)

- Penerima impor & manual muncul pada jadwal tepat; input manual tidak membuat nomor bagi.
- DP3 dapat difinalisasi walau sebagian penerima belum bernomor; tiap baris menampilkan identitas & mesin penerima tersebut.
- Rekap Harian hanya berisi distribusi selesai tanggal terpilih; mesin & serial dari snapshot aktual; Grand Total per varian = jumlah penerima.
- Preview tidak mengubah DB/Drive; finalisasi identik idempotent; perubahan isi → versi baru tanpa menimpa riwayat; satu versi aktif per kunci logis.
- Seluruh operasi menghormati permission & regency scope; error storage/database → `500` dengan request ID tanpa bocor credential.
