# Penghapusan Profil Dokumen & Branding Berita Acara — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline, chosen by user) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hapus subsistem Profil Dokumen Tender sepenuhnya; pindahkan konfigurasi logo (branding) ke fitur Berita Acara; jadikan setiap renderer BA mandiri dengan teks baku per jenis dan tahun dari `programs.fiscal_year`.

**Architecture:** Additive-first / remove-last. Pertama tambahkan tabel + service + API branding BA (tanpa menghapus apa pun), lalu arahkan ulang pipeline `bast` ke branding, lalu frontend, lalu hapus backend Profil Dokumen, terakhir migration drop tabel lama. Setiap task menghasilkan commit yang build + test hijau.

**Tech Stack:** Go 1.26 (pgx/v5, goose, net/http), React 19 + TypeScript (Vite, TanStack Query, Vitest), PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-10-01-remove-document-profile-and-ba-branding-design.md`

## Global Constraints

- Branding read butuh `bast.view`; mutasi butuh `bast.manage`. (Profil lama: `programs.view`/`programs.manage` — dihapus.)
- Unggahan logo dibatasi PNG/JPEG, maksimal 10 MiB (10485760 byte). Semua mutasi tercatat di audit log via `clientMeta(r)`.
- `program_ba_logo_assets.slot_code` mengikuti pola `^[a-z][a-z0-9_]{0,49}$`.
- BA final immutable: `snapshot_json` adalah sumber historis; decoder wajib menerima snapshot lama (`ProfileSnapshot`) tanpa mengaktifkan ulang Profil Dokumen.
- Tahun di teks BA berasal dari `programs.fiscal_year` (bukan dari tabel profil).
- Setiap commit: `go build ./...`, `go vet ./...`, paket yang disentuh lulus `go test`, dan `npm run -s typecheck` (frontend) tetap hijau.
- **Ruling (deviasi spec):** spec meminta satu migration create+copy+drop. Plan memecah menjadi `00023` (create+copy, additive) dan `00024` (drop, destruktif) agar tiap commit hijau selama eksekusi inline. Hasil akhir pada DB fresh identik. Biaya jika salah: dua baris riwayat migration alih-alih satu — tidak ada dampak data.
- **Reference PDF (authority):** `D:\KSM\Konkit\Draft BAST Petani 2024\Draft BAST Petani 2024\006. BAST - Penerima Paket (Perorangan).pdf` adalah otoritas visual BA Perorangan Petani. Renderer harus mengikuti strukturnya (lihat Task 4 Step 5). Salin PDF ke `docs/references/` saat Task 4 agar travel dengan repo.

---

## File Structure

- `internal/database/migrations/00023_program_ba_logo_assets.sql` — **create** tabel branding + salin logo dari profil.
- `internal/database/migrations/00024_drop_document_profiles.sql` — **create** drop tabel profil + lepas `profile_version_id`.
- `internal/bast/branding.go` + `branding_repository.go` — **create** domain + repo branding.
- `internal/bast/branding_service_test.go`, `branding_repository_integration_test.go` — **create** test.
- `internal/api/bast_routes.go` — **modify** tambah `GET/POST/PATCH /api/v1/bast/branding...`.
- `internal/bast/{models,repository,service,snapshot,pdf_renderer,bundle_service}.go` — **modify** lepas ketergantungan `ProfileSnapshot`; pakai `RenderIdentity` + konstanta renderer + `fiscal_year`.
- `internal/programs/{models,repository,service}.go` + tests — **modify/delete** hapus Document Profile.
- `internal/api/{handler,program_routes,routes}.go` + tests — **modify** hapus service interface, route, error mapping.
- `frontend/src/features/programs/{ProgramSetupPage.tsx,types.ts}` + hapus `DocumentProfilePanel.*` — **modify/delete**.
- `frontend/src/features/berita-acara/{BeritaAcaraPage.tsx,types.ts,LogoTenderPanel.tsx}` + test — **modify/create** area Logo Tender.

---

## Task 1: Migration — tabel branding + salin logo (additive)

**Files:**
- Create: `internal/database/migrations/00023_program_ba_logo_assets.sql`

**Interfaces:**
- Produces: tabel `program_ba_logo_assets(id, program_id, slot_code, storage_key, original_filename, mime_type, byte_size, checksum, sort_order, max_width_mm, max_height_mm, is_visible, created_at, updated_at)`, unik `(program_id, slot_code)` dan `storage_key` unik global.

- [ ] **Step 1: Tulis migration up/down**

```sql
-- +goose Up
CREATE TABLE program_ba_logo_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    slot_code text NOT NULL CHECK (slot_code ~ '^[a-z][a-z0-9_]{0,49}$'),
    storage_key text NOT NULL UNIQUE,
    original_filename text NOT NULL,
    mime_type text NOT NULL CHECK (mime_type IN ('image/png', 'image/jpeg')),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 10485760),
    checksum char(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
    sort_order integer NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    max_width_mm numeric(6,2) NOT NULL DEFAULT 35 CHECK (max_width_mm > 0),
    max_height_mm numeric(6,2) NOT NULL DEFAULT 18 CHECK (max_height_mm > 0),
    is_visible boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, slot_code)
);
CREATE INDEX program_ba_logo_assets_order_idx
    ON program_ba_logo_assets (program_id, sort_order, id);

-- Salin logo dari profil terbaru per program (published diutamakan, lalu versi tertinggi).
INSERT INTO program_ba_logo_assets
    (program_id, slot_code, storage_key, original_filename, mime_type, byte_size,
     checksum, sort_order, max_width_mm, max_height_mm, is_visible, created_at, updated_at)
SELECT v.program_id, l.slot_code, l.storage_key, l.original_filename, l.mime_type,
       l.byte_size, l.checksum, l.sort_order, l.max_width_mm, l.max_height_mm,
       l.is_visible, l.created_at, l.updated_at
FROM program_document_logo_assets l
JOIN program_document_profile_versions v ON v.id = l.profile_version_id
JOIN LATERAL (
    SELECT id FROM program_document_profile_versions pv
    WHERE pv.program_id = v.program_id
    ORDER BY (pv.status = 'published') DESC, pv.version DESC
    LIMIT 1
) latest ON latest.id = v.id;

-- +goose Down
DROP TABLE program_ba_logo_assets;
```

- [ ] **Step 2: Terapkan pada DB fresh & verifikasi**

Run (PowerShell): drop/create `konkit_test`, lalu `goose ... up`. Expected: semua migration naik tanpa error; `program_ba_logo_assets` ada; pada DB fresh jumlah baris tersalin = 0 (tidak ada profil).

- [ ] **Step 3: Commit**

```
feat(bast): add program_ba_logo_assets table and copy logos from profiles (migration 00023)
```

---

## Task 2: Domain + repository branding BA

**Files:**
- Create: `internal/bast/branding.go`, `internal/bast/branding_repository.go`
- Test: `internal/bast/branding_repository_integration_test.go`

**Interfaces:**
- Produces:
  - `type LogoAsset struct { ID, ProgramID, SlotCode, StorageKey, OriginalFilename, MimeType string; ByteSize int64; Checksum string; SortOrder int; MaxWidthMM, MaxHeightMM float64; IsVisible bool }`
  - `type LogoUploadInput struct { ProgramID, SlotCode, OriginalFilename string; Data []byte; SortOrder int; MaxWidthMM, MaxHeightMM float64 }`
  - `type LogoPatchInput struct { ID string; SortOrder *int; MaxWidthMM, MaxHeightMM *float64; IsVisible *bool }`
  - `BrandingRepository` interface: `ListLogos(ctx, programID) ([]LogoAsset, error)`, `InsertLogo(ctx, LogoUploadInput, mime, checksum, storageKey) (LogoAsset, error)`, `UpdateLogo(ctx, LogoPatchInput) (LogoAsset, error)`, `OpenLogo(ctx, logoID) (LogoAsset, []byte, error)`, `ProgramExists(ctx, programID) (bool, error)`.
- Consumes: `program_ba_logo_assets` (Task 1), `media.Storage` untuk baca/tulis byte berdasarkan `storage_key` (pola yang sama dengan repo profil lama di `internal/programs/repository.go`).

- [ ] **Step 1: Tulis `branding.go`** — tipe di atas + `var ErrLogoNotFound = errors.New("ba logo not found")`, `ErrProgramNotFound = errors.New("program not found")`, validasi MIME/ukuran/slot (reuse helper dari repo profil lama: sniff PNG/JPEG, `<=10 MiB`, slot regex).

- [ ] **Step 2: Tulis test integrasi** — insert via `InsertLogo`, `ListLogos` memfilter per program & urut `sort_order,id`; `UpdateLogo` ubah `is_visible`/`sort_order`; `OpenLogo` kembalikan byte. Ikuti pola `internal/bast/repository_integration_test.go` (guard `KONKIT_TEST_DATABASE_URL`).

- [ ] **Step 3: Jalankan test → gagal (belum ada repo).**

- [ ] **Step 4: Implement `branding_repository.go`** — SQL CRUD terhadap `program_ba_logo_assets`; `InsertLogo` upsert by `(program_id, slot_code)` lalu simpan byte ke storage pakai `storage_key`; `OpenLogo` baca byte dari storage. Mirror logika storage dari `UploadDocumentLogo`/`OpenDocumentLogo` lama.

- [ ] **Step 5: Test lulus.**

- [ ] **Step 6: Commit** — `feat(bast): add BA branding domain and repository`

---

## Task 3: Branding service + API `/api/v1/bast/branding`

**Files:**
- Create: `internal/bast/branding_service.go`, `internal/bast/branding_service_test.go`
- Modify: `internal/api/bast_routes.go`, `internal/api/handler.go` (wiring interface), `internal/api/routes.go` (error mapping bila perlu)

**Interfaces:**
- Produces: `BrandingService` dengan `ListBranding(ctx, programID) ([]LogoAsset, error)`, `UploadLogo(ctx, principal, LogoUploadInput, clientMeta) (LogoAsset, error)`, `PatchLogo(ctx, principal, LogoPatchInput, clientMeta) (LogoAsset, error)`, `OpenLogo(ctx, programID, logoID) (content, error)`. Service memvalidasi `ProgramExists`, MIME/ukuran, lalu audit-log.
- API baru:
  - `GET /api/v1/bast/branding?program_id=...` → `bast.view`
  - `POST /api/v1/bast/branding/logos` (multipart) → `bast.manage`
  - `PATCH /api/v1/bast/branding/logos/{logoID}` → `bast.manage`
  - (opsional) `GET /api/v1/bast/branding/logos/{logoID}/content` untuk pratinjau → `bast.view`

- [ ] **Step 1: Test service** (fake repo): `bast.view` boleh `ListBranding`, tidak boleh upload/patch (cek dilakukan di handler via permission — service test fokus validasi MIME/ukuran/program-exists + audit dipanggil).

- [ ] **Step 2: Jalankan → gagal.**

- [ ] **Step 3: Implement service** mengikuti pola `internal/programs/service.go` untuk Document Logo (validasi + `auditLog`).

- [ ] **Step 4: Tambah handler di `bast_routes.go`** — mirror struktur handler bast yang ada; cek permission `bast.view`/`bast.manage` via `rc.principal`. Tambah field `Branding BrandingService` ke `Handler.deps` dan wiring di konstruktor.

- [ ] **Step 5: Test API** di `internal/api/bast_routes_test.go` — view bisa baca, manage bisa upload/patch, non-authorized 403, file non-PNG/JPEG ditolak, >10 MiB ditolak.

- [ ] **Step 6: Build + test + commit** — `feat(bast): add BA branding service and /api/v1/bast/branding endpoints`

---

## Task 4: Repoint `bast` pipeline ke branding + fiscal_year + konstanta renderer

**Files:**
- Modify: `internal/bast/models.go`, `repository.go`, `service.go`, `snapshot.go`, `pdf_renderer.go`, `bundle_service.go` + test terkait.

**Interfaces:**
- Produces:
  - `type RenderIdentity struct { FiscalYear int `json:"fiscal_year"`; Logos []LogoSnapshot `json:"logos"` }` menggantikan `ProfileSnapshot` di `Snapshot`/`SourceData`.
  - Konstanta baku BA Perorangan Petani di `pdf_renderer.go`: `baPeroranganTitle`, `baPeroranganSubtitle`, `baPeroranganProcurementTemplate` (dengan `%d` untuk tahun). Seri dokumen tetap `KSM-KKT` (sudah dibangun di penomoran — konfirmasi di `service`/penomoran yang ada, bukan dari profil).
  - `ErrBrandingNotConfigured = errors.New("at least one active BA logo is required")` menggantikan `ErrProfileNotPublished`.
- Consumes: `program_ba_logo_assets` (logo aktif per program), `programs.fiscal_year`.

- [ ] **Step 1: Ganti struktur snapshot** — di `models.go`: hapus `ProfileSnapshot`, tambah `RenderIdentity`; `Snapshot.Profile ProfileSnapshot` → `Snapshot.Render RenderIdentity`; `SourceData.Profile` → `SourceData.Render` + simpan `FiscalYear int`. Hapus `ProfileVersionID` dari `SourceData`, `IndividualDocument`, input bundle (gunakan nilai historis hanya via snapshot). Hapus `ErrProfileNotPublished`, tambah `ErrBrandingNotConfigured`.

- [ ] **Step 2: `repository.go`** — hapus JOIN `program_document_profile_versions` di `ResolveStorageContext` dan di query sumber dokumen; ambil `programs.fiscal_year`; ambil logo aktif dari `program_ba_logo_assets WHERE program_id=$ AND is_visible=true ORDER BY sort_order,id`. Hapus kolom `profile_version_id` dari INSERT `bast_individual_documents` & `bast_daily_bundles` (akan di-drop di Task 7) — sementara kirim `NULL`. SELECT berhenti membaca `profile_version_id`.

- [ ] **Step 3: `service.go`** — ganti validasi `ProfileVersionID == "" → ErrProfileNotPublished` menjadi "tidak ada logo aktif → `ErrBrandingNotConfigured`". Tahun diisi dari `fiscal_year`.

- [ ] **Step 4: `snapshot.go` + decoder** — `BuildSnapshot` isi `Render{FiscalYear, Logos}`. Tambah `DecodeSnapshot(raw []byte) (Snapshot, error)` yang: coba unmarshal format baru; jika field `profile` lama ada, map `profile.logos → Render.Logos` dan `Render.FiscalYear` = 0 (atau parse tahun dari `document_number`/`procurement_description` bila tersedia) tanpa mengaktifkan Profil. Repo `GetIndividual` memakai decoder ini.

- [ ] **Step 5: `pdf_renderer.go` — ikuti PDF referensi.** `registerLogos`/`profileLogoKey` memakai `snapshot.Render.Logos` (rename `profileLogoKey`→`renderLogoKey`, key dari urutan storage_key karena tak ada VersionID). Error "published profile has no logos" → `ErrBrandingNotConfigured`. Layout satu penerima per halaman, urutan persis referensi:
  1. **Baris logo** branding (urut `sort_order`) di atas, satu baris horizontal.
  2. **Judul** `BERITA ACARA SERAH TERIMA` (bold, underline, center); **subjudul** `(FORM PENERIMA PAKET)` (italic, center).
  3. **Deskripsi pengadaan** (center, multi-line): `fmt.Sprintf(baPeroranganProcurementTemplate, snapshot.Render.FiscalYear)` =
     `"Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana Liquefied Petroleum Gas (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga"`.
  4. **No. BAST:** `snapshot.DocumentNumber` (format `NOMOR/JUMLAH/KSM-KKT-KODEKOTA/BULAN/TAHUN`). **Tanggal:** `snapshot.LocalDate`.
  5. **Data Penerima** (label : nilai): Nama, Alamat, Kota/Kabupaten (`Recipient.Regency`), No. KTP (`Recipient.NIK`), No. Kartu Petani (`Recipient.SectorIdentifier`), No. HP (`Recipient.PhoneNumber`).
  6. **`A. Data Paket Perdana yang akan diterima.`** lalu 4 tabel:
     - Mesin: kolom `Merk Mesin | Tipe Mesin | Serial Number | Checklist` → `Equipment.MachineBrand | MachineType | MachineSerial | √`.
     - Selang: `Merk Selang Hisap dan Selang Buang | Spesifikasi Selang Hisap dan Selang Buang | Serial Number | Checklist` → `Equipment.HoseBrand | HoseSpec | HoseSerial | √`.
     - Konkit/Reducer: `Merk Konkit / Reducer | Serial Number | Checklist` → `Equipment.ConverterBrand | ConverterSerial | √`.
     - Komponen: `Komponen Paket, Aksesoris & Kelengkapan | Jumlah | Satuan | Checklist` → iterasi `snapshot.Components` (`Label | Quantity | Unit | √ jika Checked`).
  7. **Paragraf pernyataan** (justify): `"Dengan ini kami menyatakan bahwa Seluruh Material/Produk/Barang tercantum diatas telah diterima dan dapat berfungsi dengan baik dengan jumlah yang benar serta telah diperiksa dengan seksama oleh masing-masing pihak."`
  8. **Tanda tangan** 3 kolom: `PENERIMA PAKET/ PETANI` | `PELAKSANA PEMASANGAN & PENDISTRIBUSIAN` | `KONSULTAN PENGAWAS`, tiap kolom kotak + `Nama :` (`Signatures.ReceiverName` / `ExecutorName` / `SupervisorName`). Detail perilaku tanda tangan tetap di luar cakupan (spec §22/57) — cukup render struktur + nama.

- [ ] **Step 6: `bundle_service.go`** — `documents[0].Snapshot.Profile.VersionID` dihapus (bundle tak lagi menyimpan profile_version_id); logo per dokumen dari `document.Snapshot.Render.Logos`.

- [ ] **Step 7: Update test bast** (`snapshot_test.go`, `pdf_renderer_test.go`, `bundle_service_test.go`, `repository_integration_test.go`, `schema_integration_test.go`) ke struktur baru + tambah test decoder snapshot lama.

- [ ] **Step 8: `go build ./... && go vet ./... && go test ./internal/bast/...` hijau.**

- [ ] **Step 9: Commit** — `refactor(bast): render BA from branding logos and fiscal_year, drop document-profile dependency`

---

## Task 5: Frontend — Logo Tender di Berita Acara

**Files:**
- Create: `frontend/src/features/berita-acara/LogoTenderPanel.tsx`, `LogoTenderPanel.test.tsx`
- Modify: `frontend/src/features/berita-acara/{BeritaAcaraPage.tsx,types.ts}`

**Interfaces:**
- Consumes: `GET/POST/PATCH /api/v1/bast/branding` (Task 3). Tipe `BaLogo = { id; program_id; slot_code; original_filename; mime_type; byte_size; sort_order; max_width_mm; max_height_mm; is_visible; content_url }`.
- Produces: panel "Logo Tender" dalam halaman BA, memakai konteks program yang sudah dipakai halaman BA. Pratinjau + urutan + status tampil; aksi unggah/urut/visibilitas hanya untuk `bast.manage`.

- [ ] **Step 1: Test** — render daftar logo dari query; kontrol kelola tersembunyi tanpa `bast.manage`; keadaan kosong menampilkan petunjuk konfigurasi (bukan crash).
- [ ] **Step 2: Jalankan → gagal.**
- [ ] **Step 3: Implement panel** (TanStack Query query+mutations), sisipkan ke `BeritaAcaraPage` sebagai area terpisah (bukan tab jenis BA).
- [ ] **Step 4: Typecheck + test hijau.**
- [ ] **Step 5: Commit** — `feat(berita-acara): add Logo Tender branding panel`

---

## Task 6: Frontend — hapus Profil Dokumen dari Persiapan Program

**Files:**
- Delete: `frontend/src/features/programs/DocumentProfilePanel.tsx`, `DocumentProfilePanel.test.tsx`
- Modify: `frontend/src/features/programs/ProgramSetupPage.tsx` (hapus tab + import), `types.ts` (hapus `DocumentProfile`, `DocumentLogo`)

- [ ] **Step 1: Hapus file panel + test.**
- [ ] **Step 2: Hapus tab "Profil Dokumen" + referensi di `ProgramSetupPage.tsx`.**
- [ ] **Step 3: Hapus tipe `DocumentProfile`/`DocumentLogo` di `types.ts` + referensinya.**
- [ ] **Step 4: `npm run -s typecheck` + `npm test` hijau; grep memastikan tak ada sisa `DocumentProfile`/`document-profile` di frontend.**
- [ ] **Step 5: Commit** — `refactor(programs): remove Document Profile tab and types from Program Setup`

---

## Task 7: Hapus backend Document Profile

**Files:**
- Modify: `internal/programs/{models,repository,service}.go` (+ test), `internal/api/{handler,program_routes,routes}.go` (+ test)

- [ ] **Step 1: `handler.go`** — hapus `DocumentProfileService` interface + field.
- [ ] **Step 2: `program_routes.go`** — hapus `case "document-profiles"`, `handleDocumentProfiles`, `handleDocumentLogos`.
- [ ] **Step 3: `routes.go`** — hapus error mapping `document_profile_published`/`document_profile_incomplete` dan `ErrProfileNotPublished` (digantikan mapping `bast.ErrBrandingNotConfigured` → 409 `ba_logo_required`).
- [ ] **Step 4: `internal/programs`** — hapus `DocumentProfile`, `DocumentProfileInput`, `DocumentLogo*`, `ErrDocumentProfile*`, dan method repo/service terkait (`ListDocumentProfiles`, `SaveDocumentProfile`, `PublishDocumentProfile`, `UploadDocumentLogo`, `OpenDocumentLogo`, dll). Hapus test terkait.
- [ ] **Step 5: `go build ./... && go vet ./...` hijau; grep memastikan tak ada sisa referensi `program_document_` di Go (selain migration 00020/00024).**
- [ ] **Step 6: `go test ./internal/programs/... ./internal/api/...` hijau.**
- [ ] **Step 7: Commit** — `refactor: remove Document Profile backend (service, repository, API, models)`

---

## Task 8: Migration — drop tabel profil + lepas profile_version_id

**Files:**
- Create: `internal/database/migrations/00024_drop_document_profiles.sql`

- [ ] **Step 1: Tulis migration**

```sql
-- +goose Up
ALTER TABLE bast_individual_documents DROP COLUMN IF EXISTS profile_version_id;
ALTER TABLE bast_daily_bundles DROP COLUMN IF EXISTS profile_version_id;
DROP TRIGGER IF EXISTS reject_published_program_document_profile_change ON program_document_profile_versions;
DROP FUNCTION IF EXISTS reject_published_program_document_profile_change();
DROP TABLE IF EXISTS program_document_logo_assets;
DROP TABLE IF EXISTS program_document_profile_versions;

-- +goose Down
-- Irreversible forward migration (tabel profil dihapus permanen).
-- Pemulihan dilakukan dengan restore backup, bukan down-migration.
SELECT 1;
```

> Catatan: konfirmasi nama trigger/function persis dari `00020_program_zones_drive_profiles.sql` sebelum menulis (gunakan nama aktual). `bast_individual_documents`/`bast_daily_bundles` dibuat di `00021` — pastikan nama kolom benar.

- [ ] **Step 2: Terapkan pada DB fresh** — drop/create `konkit_test` + `goose up`. Expected: semua migration naik; `program_document_*` tidak ada; `bast_individual_documents`/`bast_daily_bundles` tanpa `profile_version_id`.
- [ ] **Step 3: `go test ./...` (atau paket terdampak) hijau terhadap skema baru.**
- [ ] **Step 4: Commit** — `feat(db): drop document-profile tables and release profile_version_id (migration 00024)`

---

## Task 9: Selaraskan Template Paket KONKIT-2026 + rebuild bundle

**Files:**
- Modify: template/seed KONKIT-2026 (cari `Selang Hisap`/`Selang Buang` di seed/migration/kode) agar komponen referensi memakai satu baris `Selang, Clamp & Aksesorisnya`.
- Rebuild: `frontend` production bundle + sinkron `web/static/app`.

- [ ] **Step 1: Cari sumber komponen** `Selang Hisap`/`Selang Buang` (grep). Tentukan apakah seed migration baru diperlukan atau penyesuaian template default. Buat migration bila data sudah terpasang.
- [ ] **Step 2: Ubah menjadi satu baris** `Selang, Clamp & Aksesorisnya` sesuai referensi.
- [ ] **Step 3: `npm run build` di frontend; salin/commit bundle ke `web/static/app` sesuai pola commit bundle yang ada.**
- [ ] **Step 4: Verifikasi server lokal menyajikan bundle baru (manifest).**
- [ ] **Step 5: Commit** — `feat(programs): align KONKIT-2026 package template with BA reference (single hose row)` + `chore(frontend): sync bundle`.

---

## Self-Review (diisi setelah menulis)

- **Spec coverage:** Profil dihapus (Task 6/7/8); branding di BA (Task 1/2/3/5); renderer mandiri + tahun dari fiscal_year (Task 4); snapshot decoder lama (Task 4 Step 4); template paket (Task 9); API branding + hapus endpoint profil (Task 3/7); migration forward (Task 1/8). ✓
- **Reference PDF:** tersedia (path di Global Constraints), layout konkret di Task 4 Step 5. Mekanisme/perilaku tanda tangan tetap di luar cakupan (spec) — hanya struktur + nama yang dirender.
- **Type consistency:** `RenderIdentity`/`Render` dipakai konsisten di models/snapshot/renderer/bundle (Task 4). `LogoAsset`/`LogoUploadInput`/`LogoPatchInput` konsisten Task 2↔3.
