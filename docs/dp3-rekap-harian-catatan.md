# Catatan Implementasi: DP3 & Rekapitulasi Harian

> **Status:** Implementasi inti selesai & semua tes hijau. Dokumen ini adalah catatan handoff untuk review dan melanjutkan sisa pekerjaan.
> **Spec:** `docs/superpowers/specs/2026-10-02-dp3-rekap-harian-design.md`
> **Plan:** `docs/superpowers/plans/2026-10-02-dp3-rekap-harian-implementation.md`
> **Tanggal:** 2 Oktober 2026

---

## 1. Ringkasan

Menambahkan dua jenis Berita Acara kolektif pada ruang kerja **Berita Acara**:

1. **DP3 (Daftar Penerima Paket Perdana)** — PDF A4 landscape dari daftar nominatif jadwal (impor DCP3 + penerima manual).
2. **Rekapitulasi Harian** — PDF A4 portrait dari distribusi selesai pada satu tanggal lokal (`Asia/Jakarta`).

Keduanya memakai pipeline dokumen agregat bersama: preview, finalisasi idempotent (checksum), snapshot, versioning (active/superseded), advisory lock, audit, unduh, dan sinkronisasi storage (lokal/Drive).

---

## 2. Migration baru (versi DB naik ke 32)

| File | Isi |
|---|---|
| `internal/database/migrations/00030_machine_power_fuel.sql` | Tambah `power` & `fuel_type` ke setiap `machine_option` template (KONKIT-2026, PETANI-LPG, NELAYAN-LPG). Nilai default: `5.5 HP` / `Bensin`. |
| `internal/database/migrations/00031_bast_schedule_settings.sql` | Tabel `bast_schedule_settings` (konfigurasi BA per jadwal) + seed dari `program_schedules.supervisor_name`. |
| `internal/database/migrations/00032_bast_aggregate_documents.sql` | Tabel `bast_aggregate_documents` + index **satu versi aktif per kunci logis** + version unik + `storage_key text`. |

> Catatan: `00027`–`00029` sudah ada sebelumnya (belum di-commit) dan saya biarkan apa adanya.

---

## 3. Backend (`internal/`)

### Snapshot alokasi & data mesin
- `programs/machine_snapshot.go` — `MachineOptionData`, `SelectMachine`, `BuildAllocationSnapshot`, `ResolveMachineCodeFromCell`, `MachineOptions`.
- `distribution/equipment_snapshot.go` — snapshot verifikasi kini menyimpan `machine_power` + `machine_fuel_type`.
- `dcp3/repository.go` — `Commit` membangun `package_snapshot_json` (berisi `selected_machine`) **per baris**; auto-select varian tunggal; multi-varian resolve dari kolom mapping.
- `recipients/repository.go` — penerima manual kini menyimpan snapshot alokasi `selected_machine` (bukan `{}`).

### Konfigurasi BA per jadwal
- `bast/schedule_settings.go`, `schedule_settings_repository.go`, `schedule_settings_service.go` (+ test).

### Dokumen agregat (lifecycle bersama)
- `bast/aggregate.go`, `aggregate_repository.go` (+ integration test) — `ActivateAggregate` (advisory lock, idempotensi checksum, supersede), `GetActiveAggregate`, `GetAggregateByID`, `ListAggregates`, `ListActiveAggregatesForType`, `NextAggregateVersion`.

### DP3
- `bast/dp3.go` (types + validasi + snapshot + resolve prioritas verifikasi→alokasi), `dp3_repository.go`, `dp3_service.go`, `dp3_renderer.go` (+ tests).

### Rekap Harian
- `bast/daily_recap.go`, `daily_recap_repository.go`, `daily_recap_service.go`, `daily_recap_renderer.go` (+ tests).

### Shared
- `bast/logo_loader.go` — baca byte logo aktif.
- Error sentinel baru di `bast/dp3.go` + mapping HTTP di `internal/api/routes.go`.

### Wiring API
- `internal/api/handler.go` (interface + deps `DP3`, `DailyRecap`, `BASTSettings`), `internal/api/routes.go` (routing), `internal/api/bast_aggregate_routes.go` (handler baru), `internal/api/bast_routes.go` (settings handler).
- `cmd/server/main.go` (instansiasi service).

---

## 4. API endpoints baru

### Konfigurasi jadwal
- `GET /api/v1/bast/schedules/{schedule_id}/settings` (`bast.view`)
- `PUT /api/v1/bast/schedules/{schedule_id}/settings` (`bast.manage`)

### DP3
- `GET /api/v1/bast/dp3/summary?schedule_id=...`
- `GET /api/v1/bast/dp3/recipients?schedule_id=...`
- `POST /api/v1/bast/dp3/preview` — body `{schedule_id, document_date}`
- `POST /api/v1/bast/dp3/finalize` — body `{schedule_id, document_date}`
- `GET /api/v1/bast/dp3/documents?schedule_id=...&date=...`
- `GET /api/v1/bast/dp3/documents/{id}/content`

### Rekap Harian
- `GET /api/v1/bast/daily-recap/dates?schedule_id=...`
- `GET /api/v1/bast/daily-recap/recipients?schedule_id=...&date=...`
- `POST /api/v1/bast/daily-recap/preview` — body `{schedule_id, local_date}`
- `POST /api/v1/bast/daily-recap/finalize` — body `{schedule_id, local_date}`
- `GET /api/v1/bast/daily-recap/documents?schedule_id=...&date=...`
- `GET /api/v1/bast/daily-recap/documents/{id}/content`

Semua endpoint menerapkan regency scope (di luar scope → `404`). Kode error validasi (PRD §18) dipetakan ke HTTP 409: `zone_not_configured`, `ba_logo_required`, `handover_location_required`, `signatory_required`, `no_recipients`, `recipient_identity_incomplete`, `allocation_snapshot_incomplete`, `verification_snapshot_incomplete`, `machine_power_required`, `machine_fuel_required`, `document_conflict`.

---

## 5. Frontend (`frontend/src/features/`)

- `berita-acara/DP3Panel.tsx` — tab DP3 (ringkasan, tanggal dokumen, tabel penerima, preview/finalisasi, riwayat versi).
- `berita-acara/DailyRecapPanel.tsx` — tab Rekap Harian (daftar tanggal, tabel penerima + grand total per varian, preview/finalisasi).
- `berita-acara/ScheduleSettingsPanel.tsx` — editor konfigurasi BA per jadwal (dipakai kedua tab).
- `berita-acara/BeritaAcaraPage.tsx` + `types.ts` — wiring tab dp3 & rekap-harian.
- `programs/TemplatesPanel.tsx` + `types.ts` — field `power`/`fuel_type` di opsi mesin.
- `dcp3/types.ts`, `ColumnMappingStep.tsx`, `DCP3ImportPage.tsx` — field **"Varian mesin"** di mapping saat template multi-varian.

---

## 6. Verifikasi (semua hijau)

| Perintah | Hasil |
|---|---|
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `go test -p 1 ./...` (dengan `TEST_DATABASE_URL=...konkit_test`) | ✅ (unit + integrasi) |
| `npm --prefix frontend run -s typecheck` | ✅ |
| `npm --prefix frontend run -s test` | ✅ 171 tests |
| `npm --prefix frontend run -s build` | ✅ |
| Migration 00001–00032 di DB throwaway | ✅ |

**Artefak visual untuk review:** `tmp/dp3-qa.pdf` dan `tmp/rekap-qa.pdf` (sampel renderer dengan data realistis + logo KSM; 2 halaman masing-masing). Buka langsung untuk cek layout.

---

## 7. Yang BELUM selesai (untuk dilanjutkan)

### 7.1 Visual QA terhadap renderer referensi
- **Status:** belum, karena file referensi PDF/renderer Go yang disebut PRD ("renderer Go DP3 & Rekap Harian yang diberikan pengguna") **belum ada di repo** (`docs/references/` kosong).
- **Yang perlu dilakukan:** letakkan file referensi → cocokkan struktur/tata letak renderer saya (`bast/dp3_renderer.go`, `bast/daily_recap_renderer.go`) dengan referensi, lalu sesuaikan margin/kolom/header/signature.

### 7.2 E2E Playwright untuk DP3 & Rekap Harian
- **Status:** belum. Sudah ada fondasi e2e (`frontend/e2e/dcp3-distribution.spec.ts` menyelesaikan distribusi penuh).
- **Yang perlu dilakukan:**
  1. Perluas `cmd/e2eseed/main.go` agar menyiapkan prasyarat DP3/Rekap: **zona non-placeholder + assignment regency**, **logo aktif** (`program_ba_logo_assets` + salin file logo ke `STORAGE_PATH`), dan **`bast_schedule_settings`**.
  2. Tulis spec baru `frontend/e2e/berita-acara.spec.ts`: navigasi Berita Acara → konfigurasi settings → preview/finalisasi DP3 & Rekap Harian.
  3. Jalankan `npm --prefix frontend run e2e` (butuh Google Chrome lokal).

### 7.3 Catatan teknis lain
- **Migration concurrency:** setelah menambahkan migration baru, jalankan `migrate up` sekali (atau `go test -p 1 ./...`) sebelum `go test ./...` paralel, karena beberapa paket integrasi menjalankan `goose.Up` paralel ke `konkit_test` dan bisa race saat migration frontier belum diterapkan.
- **Nilai `power`/`fuel_type`:** default `5.5 HP`/`Bensin` untuk SHARK SPWP 80-30 adalah asumsi — **tolong konfirmasi nilai yang benar** dengan pemilik produk sebelum produksi.

---

## 8. Catatan penting

- **Tidak ada commit yang dibuat.** Working tree berisi campuran perubahan yang sudah ada sebelumnya (pekerjaan BA branding / Document Profile removal) + perubahan fitur ini. Perubahan yang sudah ada saya **biarkan apa adanya** sesuai instruksi.
- **`konkit_test` sudah dimigrasikan ke versi 32** (jalur normal saat boot/integration test). DB operasional `konkit` **tidak disentuh**.
- File baru fitur ini jelas terpisah (semua di `internal/bast/*.go` baru, `internal/api/bast_aggregate_routes.go`, `frontend/.../berita-acara/*Panel.tsx`, migration `00030`–`00032`, dll).
