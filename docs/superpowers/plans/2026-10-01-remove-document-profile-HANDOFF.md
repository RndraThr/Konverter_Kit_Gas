# HANDOFF — Penghapusan Profil Dokumen & Branding Berita Acara

> Dokumen serah-terima untuk AI agent berikutnya. Baca **bersama** plan utama:
> `docs/superpowers/plans/2026-10-01-remove-document-profile-and-ba-branding.md`
> dan spec: `docs/superpowers/specs/2026-10-01-remove-document-profile-and-ba-branding-design.md`.
> Eksekusi **INLINE** (tanpa subagent — akun kena monthly spend limit). Respons ke user **Bahasa Indonesia**.

## Status ringkas (per 2026-10-01)

Branch `main`. Eksekusi plan 9-task, **additive-first / remove-last**, tiap commit harus build+test hijau.

| Task | Status | Commit |
|------|--------|--------|
| 1. Migration 00023 (`program_ba_logo_assets` + copy logos) | ✅ selesai | `eae05b4` |
| 2. BA branding domain + repository | ✅ selesai | `e97b91e` |
| 3. Branding service + `/api/v1/bast/branding` API | ✅ selesai | `ac554cb` |
| 4. Repoint `bast` → branding + fiscal_year + renderer | 🔶 **IN PROGRESS** | belum commit |
| 5. Frontend Logo Tender panel di Berita Acara | ⬜ belum | |
| 6. Hapus tab Profil Dokumen dari Program Setup (frontend) | ⬜ belum | |
| 7. Hapus backend Document Profile | ⬜ belum | |
| 8. Migration drop tabel profil (**renumber ke 00025**) | ⬜ belum | |
| 9. Selaraskan Template Paket KONKIT-2026 + rebuild bundle | ⬜ belum | |

### ⚠️ Kondisi working tree SAAT INI
- Working tree **BERSIH & build hijau** (edit Task 4 di models.go sudah di-revert agar tidak meninggalkan build broken). Mulai Task 4 dari awal mengikuti daftar edit di bawah.
- Task 4 bersifat **atomik** — selesaikan SELURUH edit di bawah (termasuk `internal/api/routes.go`) lalu commit sebagai satu unit. JANGAN commit parsial (repo harus build+test hijau tiap commit).

## Keputusan penting (rulings)
1. **Tiga migration, bukan satu** (spec minta satu). `00023` additive (DONE). `00024` (Task 4) = drop kolom `profile_version_id` dari `bast_individual_documents` + `bast_daily_bundles` (kolom **NOT NULL + FK** ke `program_document_profile_versions`, jadi harus dilepas agar Task 4 bisa berhenti mengisinya). `00025` (Task 8) = drop tabel `program_document_logo_assets` + `program_document_profile_versions` + trigger `program_document_profile_immutable` + function `reject_published_program_document_profile_change`.
2. **PDF referensi** = `D:\KSM\Konkit\Draft BAST Petani 2024\Draft BAST Petani 2024\006. BAST - Penerima Paket (Perorangan).pdf`. Renderer Codex (`pdf_renderer.go`) **sudah mengikuti struktur referensi** (4 logo, judul, subjudul, deskripsi, No.BAST, Tanggal, Data Penerima, 3 tabel peralatan + tabel komponen, pernyataan, 3 kolom TTD). Jadi Task 4 untuk renderer = ganti sumber teks saja (profil → konstanta + fiscal_year), bukan tulis ulang layout. Seri dokumen = konstanta `KSM-KKT`.
3. Series `DocumentSeries` jadi konstanta `KSM-KKT` (bukan dari DB profil).

## Setup DB test (Windows, tanpa psql)
Postgres lokal: `postgres://postgres:admin@127.0.0.1:5432`. DB test wajib bernama `konkit_test`.
```bash
# buat DB test fresh (via Go one-off; psql tidak terpasang):
cat > ./mkdb_tmp.go <<'EOF'
package main
import ("context";"fmt";"os";"github.com/jackc/pgx/v5")
func main(){ c,err:=pgx.Connect(context.Background(),"postgres://postgres:admin@127.0.0.1:5432/postgres?sslmode=disable"); if err!=nil{fmt.Println(err);os.Exit(1)}; defer c.Close(context.Background()); _,_=c.Exec(context.Background(),"DROP DATABASE IF EXISTS konkit_test"); if _,err:=c.Exec(context.Background(),"CREATE DATABASE konkit_test");err!=nil{fmt.Println(err);os.Exit(1)}; fmt.Println("ok") }
EOF
go run ./mkdb_tmp.go; rm -f ./mkdb_tmp.go
# apply migrations:
DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable" go run ./cmd/migrate up
# run tests (integration butuh TEST_DATABASE_URL):
export TEST_DATABASE_URL="postgres://postgres:admin@127.0.0.1:5432/konkit_test?sslmode=disable"
go test ./internal/bast/ ./internal/api/
```
Integration test di `internal/bast` menjalankan `goose.Up` sendiri; cukup `TEST_DATABASE_URL` menunjuk ke `konkit_test` yang sudah ada.

---

## TASK 4 — daftar edit lengkap (selesaikan ini dulu, lalu commit)

Konteks: `bast` lama membaca teks dari `ProfileSnapshot` (Title/Subtitle/ProcurementDescription/DocumentSeries/Logos). Ganti jadi `RenderIdentity{FiscalYear int; Logos []LogoSnapshot}` + teks dari konstanta + logo dari tabel `program_ba_logo_assets`.

### models.go
- Ganti `ErrProfileNotPublished` → `ErrBrandingNotConfigured = errors.New("at least one active BA logo is required")`.
- Tambahkan konstanta (boleh di models.go): `const documentSeries = "KSM-KKT"`.
- Hapus `type ProfileSnapshot struct {...}` (baris ~98-105). Tambah:
  ```go
  type RenderIdentity struct {
      FiscalYear int            `json:"fiscal_year"`
      Logos      []LogoSnapshot `json:"logos"`
  }
  ```
- `Snapshot`: ganti field `Profile ProfileSnapshot `json:"profile"`` → `Render RenderIdentity `json:"render"``.
- `SourceData`: ganti `Profile ProfileSnapshot` → `Render RenderIdentity`.
- `IndividualDocument`: hapus field `ProfileVersionID string `json:"profile_version_id"``.
- `BundleActivation`: hapus field `ProfileVersionID string`.
- `SourceContext`: hapus `ProfileVersionID string`; pertahankan `DocumentSeries`; tambah `HasActiveLogo bool`.

### repository.go
- **GetSourceContext** (baris ~20-65): hapus `LEFT JOIN LATERAL (... program_document_profile_versions ...) profile`, hapus var `profileID, series`. Set `result.DocumentSeries = documentSeries`. Tambah kolom `EXISTS(SELECT 1 FROM program_ba_logo_assets WHERE program_id=p.id AND is_visible=true)` → scan ke `&result.HasActiveLogo`. Hapus assignment `result.ProfileVersionID` & `result.DocumentSeries = *series`.
- **LoadSourceData** (baris ~78-143): hapus `JOIN program_document_profile_versions profile ...`; hapus kolom `profile.*` dari SELECT & scan; tambah `p.fiscal_year` → scan ke `source.Render.FiscalYear`. Hapus argumen `sourceContext.ProfileVersionID`. Ganti query logo (baris ~130) jadi dari `program_ba_logo_assets WHERE program_id=$1 AND is_visible=true ORDER BY sort_order,id` (pakai `sourceContext.ProgramID`), append ke `source.Render.Logos`.
- **SaveFinalDocument** (INSERT baris ~165): hapus kolom `profile_version_id` dari INSERT + argumen `source.Profile.VersionID`. Sesuaikan placeholder `$n`.
- **getIndividualDocument** (baris ~178-192): hapus `profile_version_id` dari SELECT + scan `&item.ProfileVersionID`. Ganti `json.Unmarshal(payload, &item.Snapshot)` → `item.Snapshot, err = DecodeSnapshot(payload)`.
- **ActivateBundle** (INSERT baris ~339): hapus kolom `profile_version_id` + argumen `input.ProfileVersionID`. Sesuaikan placeholder.

### service.go
- **validateContext** (baris ~166-177): hapus cek `ProfileVersionID == "" || DocumentSeries == "" → ErrProfileNotPublished`. Ganti dengan: `if !value.HasActiveLogo { return ErrBrandingNotConfigured }`.

### snapshot.go
- **BuildSnapshot**: hapus cek `source.Profile.VersionID == ""`. Sort `source.Render.Logos` by SortOrder. Jika `len(source.Render.Logos)==0` → `return Snapshot{}, ErrBrandingNotConfigured`. Build `Snapshot{... Render: source.Render ...}` (bukan Profile).
- Tambah decoder (terima snapshot lama berfield `profile`):
  ```go
  func DecodeSnapshot(raw []byte) (Snapshot, error) {
      var snap Snapshot
      if err := json.Unmarshal(raw, &snap); err != nil { return Snapshot{}, err }
      if len(snap.Render.Logos) == 0 {
          var legacy struct{ Profile struct{ Logos []LogoSnapshot `json:"logos"` } `json:"profile"` }
          if json.Unmarshal(raw, &legacy) == nil && len(legacy.Profile.Logos) > 0 {
              snap.Render.Logos = legacy.Profile.Logos // FiscalYear tetap 0 utk dok lama
          }
      }
      return snap, nil
  }
  ```
  (tambah import `encoding/json`.)

### pdf_renderer.go
- Tambah konstanta:
  ```go
  const (
      baPeroranganTitle    = "BERITA ACARA SERAH TERIMA"
      baPeroranganSubtitle = "(FORM PENERIMA PAKET)"
      baPeroranganProcurementTemplate = "Pengadaan Barang Penyediaan dan Pendistribusian Paket Perdana Liquefied Petroleum Gas (LPG) untuk Mesin Pompa Air Bagi Petani Sasaran Tahun Anggaran %d di PT Pertamina Patra Niaga"
  )
  ```
- `logosByProfile`/`profileLogoKey(document.Snapshot.Profile)` → `logosByRender`/`renderLogoKey(document.Snapshot.Render)`. `renderLogoKey(r RenderIdentity)` bikin key dari daftar logo (tak ada VersionID lagi): gabung `AssetID:StorageKey:SortOrder`.
- `registerLogos`: pesan error `"published profile has no logos"` → bungkus `ErrBrandingNotConfigured`.
- `renderRecipient`: `snapshot.Profile.Logos` → `snapshot.Render.Logos`. Panggilan `renderHeader(pdf, snapshot.Profile, logos)` → `renderHeader(pdf, logos)`. Deskripsi: `pdf.MultiCell(..., fmt.Sprintf(baPeroranganProcurementTemplate, snapshot.Render.FiscalYear), ...)`.
- `renderHeader(pdf, profile ProfileSnapshot, logos)` → `renderHeader(pdf, logos []registeredLogo)`: pakai `baPeroranganTitle` (font "BU") & `baPeroranganSubtitle` (font "BI", italic sesuai referensi) alih-alih `profile.Title/Subtitle`.
- `addContinuationPage(pdf, slot, profile, logos, events)` → `addContinuationPage(pdf, slot, logos, events)` (buang profile; panggil `renderHeader(pdf, logos)`).
- **(opsional, sesuai referensi)** Data Penerima: referensi hanya punya 6 baris — Nama, Alamat, Kota/Kabupaten, No. KTP, No. Kartu Petani, No. HP. Saat ini ada tambahan Desa/Kelurahan & Kecamatan. Boleh diselaraskan ke 6 baris referensi (hapus 2 baris itu). Ini penyempurnaan, bukan blocker.

### Migration baru: internal/database/migrations/00024_release_bast_profile_version_id.sql
```sql
-- +goose Up
ALTER TABLE bast_individual_documents DROP COLUMN profile_version_id;
ALTER TABLE bast_daily_bundles DROP COLUMN profile_version_id;

-- +goose Down
ALTER TABLE bast_individual_documents ADD COLUMN profile_version_id uuid;
ALTER TABLE bast_daily_bundles ADD COLUMN profile_version_id uuid;
```
(DROP COLUMN otomatis melepas FK + NOT NULL.)

### Tests Task 4 (update ke struktur baru)
File: `snapshot_test.go`, `pdf_renderer_test.go`, `bundle_service_test.go`, `repository_integration_test.go`, `schema_integration_test.go`.
- Ganti semua `Profile:`/`.Profile`/`ProfileSnapshot`/`ProfileVersionID` ke `Render`/`RenderIdentity`.
- Hapus insert `program_document_profile_versions` di integration test bila dipakai untuk BA source (ganti: insert ≥1 baris `program_ba_logo_assets` untuk program agar `HasActiveLogo` true & logo tersedia). Contoh insert logo:
  `INSERT INTO program_ba_logo_assets(program_id,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm,max_height_mm,is_visible) VALUES($1,'pertamina','key-...','p.png','image/png',10,repeat('a',64),1,35,18,true)`.
- Tambah 1 test `DecodeSnapshot` menerima JSON lama berfield `"profile":{"logos":[...]}` dan memetakan ke `Render.Logos`.
- `bundle_service_test.go` `bundleRepositoryStub` + `render()` sudah pakai `.Snapshot.Profile.Logos` di produksi — pastikan stub LoadSourceData mengisi `Render`.

### Commit Task 4 (setelah hijau)
```
refactor(bast): render BA from branding logos and fiscal_year, drop document-profile dependency (migration 00024)
```
Verifikasi dulu: `go build ./... && go vet ./... && go test ./internal/bast/ ./internal/api/` (dengan TEST_DATABASE_URL) hijau.

---

## TASK 5 — Frontend Logo Tender (Berita Acara)
- Buat `frontend/src/features/berita-acara/LogoTenderPanel.tsx` + `.test.tsx`; sisipkan ke `BeritaAcaraPage.tsx` sebagai area terpisah (bukan tab jenis BA).
- Pakai API Task 3: `GET /api/v1/bast/branding?program_id=`, `POST /api/v1/bast/branding/logos` (multipart: `program_id`, `slot_code`, `sort_order`, `max_width_mm`, `max_height_mm`, `file`), `PATCH /api/v1/bast/branding/logos/{id}` (JSON: `program_id`, `sort_order`, `max_width_mm`, `max_height_mm`, `is_visible`), preview `GET .../logos/{id}/content?program_id=`.
- Tipe `BaLogo` di `berita-acara/types.ts`. Kontrol unggah/urut/visibilitas hanya untuk `bast.manage`; `bast.view` hanya lihat. Keadaan kosong → petunjuk konfigurasi, bukan crash.
- `npm run -s typecheck` + `npm test` hijau. Commit: `feat(berita-acara): add Logo Tender branding panel`.

## TASK 6 — Hapus Profil Dokumen dari Program Setup (frontend)
- Hapus `frontend/src/features/programs/DocumentProfilePanel.tsx` + `.test.tsx`.
- `ProgramSetupPage.tsx`: hapus tab "Profil Dokumen" + import + query/mutation terkait.
- `programs/types.ts`: hapus tipe `DocumentProfile` & `DocumentLogo` (baris 5-6).
- Grep pastikan tak ada sisa `DocumentProfile`/`document-profile` di `frontend/src`. Typecheck+test hijau. Commit: `refactor(programs): remove Document Profile tab and types from Program Setup`.

## TASK 7 — Hapus backend Document Profile
- `internal/api/handler.go`: hapus interface `DocumentProfileService` (baris ~87-95) + field `Programs`? TIDAK — hanya hapus bagian DocumentProfile. (Catatan: `ProgramSetupService` tetap.) Hapus referensi service cast `h.deps.Programs.(DocumentProfileService)`.
- `internal/api/program_routes.go`: hapus `case "document-profiles"`, fungsi `handleDocumentProfiles`, `handleDocumentLogos`.
- `internal/api/routes.go`: hapus mapping error `document_profile_published` / `document_profile_incomplete` (baris ~466-468) & referensi `ErrProfileNotPublished` (sudah tak ada di bast). Tambah mapping `bast.ErrBrandingNotConfigured` → 409 `ba_logo_required` (atau 422) — **cek**: routes.go baris ~471 masih map `bast.ErrProfileNotPublished` yang sudah dihapus; ganti ke `bast.ErrBrandingNotConfigured`. (Idealnya mapping ini sudah diperbaiki saat Task 4 agar build `api` hijau — bila Task 4 belum menyentuh routes.go, build `internal/api` akan gagal setelah Task 4; **periksa & perbaiki routes.go saat Task 4** agar seluruh repo build hijau. Lihat catatan di bawah.)
- `internal/programs/{models,repository,service}.go`: hapus `DocumentProfile`, `DocumentProfileInput`, `DocumentLogo*`, `ErrDocumentProfile*`, `documentProfileRepository`, method `ListDocumentProfiles/SaveDocumentProfile/PublishDocumentProfile/UploadDocumentLogo/UpdateDocumentLogo/OpenDocumentLogo/GetDocumentProfile/SaveDocumentLogo/GetDocumentLogo/newDocumentAssetKey/slotCodePattern(bila tak dipakai lagi)`. Hapus test terkait. (CATATAN: `programs.Service` punya `storage media.Storage` — mungkin jadi unused setelah logo dihapus; cek apakah masih dipakai fitur lain. Jika tidak, boleh dibersihkan, tapi hati-hati.)
- Grep pastikan tak ada sisa `program_document_` di Go selain migration. Build+vet+test hijau. Commit: `refactor: remove Document Profile backend (service, repository, API, models)`.

> **PENTING urutan build-green:** Task 4 mengubah `bast` dan membuat `internal/api` (routes.go yang mereferensikan `bast.ErrProfileNotPublished`) GAGAL build. Agar Task 4 commit hijau, **perbaiki `internal/api/routes.go` saat Task 4** (ganti `bast.ErrProfileNotPublished` → `bast.ErrBrandingNotConfigured`). Endpoint Document Profile di `program_routes.go`/`handler.go` masih valid (programs masih punya tipe itu sampai Task 7) sehingga tetap build. Baru Task 7 menghapus sisi programs.

## TASK 8 — Migration drop tabel profil (RENUMBER → 00025)
File: `internal/database/migrations/00025_drop_document_profiles.sql`:
```sql
-- +goose Up
DROP TRIGGER IF EXISTS program_document_profile_immutable ON program_document_profile_versions;
DROP FUNCTION IF EXISTS reject_published_program_document_profile_change();
DROP TABLE IF EXISTS program_document_logo_assets;
DROP TABLE IF EXISTS program_document_profile_versions;

-- +goose Down
SELECT 1; -- forward-only; pemulihan via restore backup
```
Terapkan ke DB fresh, `go test ./...` hijau. Commit: `feat(db): drop document-profile tables (migration 00025)`.

## TASK 9 — Selaraskan Template Paket KONKIT-2026 + rebuild bundle
- Grep `Selang Hisap` / `Selang Buang` di seed/migration/kode. Referensi tabel komponen pakai SATU baris `Selang, Clamp & Aksesorisnya` (lihat PDF). Buat migration/seed penyesuaian bila data sudah terpasang.
- `cd frontend && npm run build`; sinkron bundle ke `web/static/app` (ikuti pola commit "chore(frontend): sync bundle" sebelumnya — update `.vite/manifest.json`, `index.html`, assets). Commit: `feat(programs): align KONKIT-2026 package template (single hose row)` + `chore(frontend): sync bundle`.

## Setelah semua task
- Jalankan full suite: `go test ./...` (dengan TEST_DATABASE_URL) + `cd frontend && npm run -s typecheck && npm test`.
- `main` sudah ahead of origin (banyak commit) — **JANGAN push tanpa persetujuan user**. Tanyakan dulu.
- Update todo & lapor ke user dalam Bahasa Indonesia.

## Memory relevan (sudah tersimpan)
- `reference_ba_perorangan_pdf.md` (path PDF otoritas layout).
- `feedback_bahasa_indonesia.md` (respons Bahasa Indonesia).
- `feedback_clean_schema.md` (buang kolom/DB mati proaktif).
