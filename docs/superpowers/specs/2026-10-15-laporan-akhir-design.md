# Rancangan Fitur Laporan Akhir

Versi: 1.0
Tanggal: 2026-10-15
Status: Menunggu review pengguna

## 1. Tujuan

Membangun fitur baru "Laporan Akhir": satu dokumen presentasi (PPTX) per Program + Kabupaten, berisi ringkasan kegiatan — pengiriman/penerimaan barang, rapat koordinasi, sosialisasi, pendistribusian, dan dokumentasi foto — dengan format/layout tetap (mengikuti contoh referensi `LAPORAN KEGIATAN PETANI WAJO.pptx`), tapi isinya mengikuti data riil masing-masing kabupaten secara live, bukan diketik ulang manual tiap kali.

Dua cara menampilkan hasilnya:
1. **Mode Present** — slide ditampilkan langsung di browser (full-screen, next/prev), selalu mengikuti data terbaru saat dibuka.
2. **Generate PPTX** — tombol eksplisit yang menyuntik data saat itu ke master template, menghasilkan file `.pptx` yang disimpan ke Google Drive (dengan riwayat versi), untuk diunduh/dibagikan di luar aplikasi.

File PPTX ini diletakkan **langsung di dalam folder kabupaten** di Drive — sejajar dengan 3 folder kategori yang sudah ada (`BERITA ACARA (BA)`, `DOKUMEN PENDUKUNG`, `DOKUMENTASI (FOTO)`), bukan di dalam salah satunya.

Akses diatur per role memakai mekanisme permission + regency-scope yang sudah dipakai semua modul lain — tidak ada mekanisme otorisasi baru.

## 2. Non-tujuan

- Tidak membangun editor PPTX umum — layout tiap slide tetap (mengikuti master template), yang dinamis hanya teks dan foto yang disuntikkan.
- Tidak ada opsi upload/ganti master template dari UI pada versi ini — template dikelola sebagai aset di server (lihat bagian 5).
- Mode Present bukan konversi file PPTX ke gambar/video — dirender ulang sebagai HTML/CSS di React, dengan data yang sama persis dengan jalur generate PPTX.
- Tidak menyalin ulang foto/dokumen yang sudah ada — Laporan Akhir hanya menyimpan **referensi** (`activity_media.id`) ke foto yang dipilih, bukan salinan filenya.
- Tidak memblokir generate ketika sebagian data belum lengkap — slide yang datanya kosong ditampilkan dengan placeholder "Belum diisi", bukan error.
- Tidak mengubah 3 kategori folder yang sudah ada maupun fungsi `BuildFolderPath` yang melayaninya.

## 3. Model Data

Domain baru: `internal/finalreport`. Tiga tabel baru, satu permission baru.

### 3.1 `final_report_settings`

Input yang memang khusus untuk fitur ini saja (bukan dipinjam dari modul lain) — susunan tim di lapangan (slide 3 pada contoh referensi).

```sql
CREATE TABLE final_report_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    leader_count integer NOT NULL DEFAULT 0,
    admin_count integer NOT NULL DEFAULT 0,
    technician_count integer NOT NULL DEFAULT 0,
    helper_count integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id),
    UNIQUE (program_id, regency_id)
);
```

Semua count `>= 0`, divalidasi di service layer (bukan CHECK constraint, konsisten dengan pola validasi di modul lain).

### 3.2 `final_report_photo_picks`

Foto yang dipilih manual untuk tiap slide dokumentasi, sebagai referensi ke `activity_media` yang sudah ada (tidak menyalin file).

```sql
CREATE TABLE final_report_photo_picks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    slide_key text NOT NULL,
    activity_media_id uuid NOT NULL REFERENCES activity_media(id) ON DELETE CASCADE,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (program_id, regency_id, slide_key, activity_media_id)
);
CREATE INDEX idx_final_report_photo_picks_lookup ON final_report_photo_picks(program_id, regency_id, slide_key);
```

`slide_key` adalah salah satu dari 4 nilai tetap, sesuai pemetaan di bagian 4: `pengiriman_penerimaan`, `rapat_koordinasi`, `sosialisasi`, `pendistribusian`.

Kalau foto yang dirujuk dihapus dari Dokumentasi, baris pick-nya ikut terhapus (`ON DELETE CASCADE`) — otomatis tidak lagi tampil di Laporan Akhir, tanpa perlu pembersihan manual.

Maksimal foto per slide: **6** (divalidasi di service, bukan constraint) — cukup untuk grid 2x3 di layout referensi, dan menjaga ukuran file PPTX tetap wajar.

### 3.3 `final_report_documents`

Riwayat versi file PPTX yang sudah di-generate, mengikuti pola `bast_aggregate_documents`.

```sql
CREATE TABLE final_report_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id uuid NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    version integer NOT NULL,
    filename text NOT NULL,
    checksum text NOT NULL,
    storage_key text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    finalized_at timestamptz NOT NULL DEFAULT now(),
    finalized_by uuid REFERENCES users(id),
    UNIQUE (program_id, regency_id, version)
);
CREATE INDEX idx_final_report_documents_active ON final_report_documents(program_id, regency_id) WHERE status = 'active';
```

Setiap generate baru menambah versi baru (`status='active'`), versi sebelumnya ditandai `status='superseded'` — tidak dihapus, konsisten dengan pola audit trail BA.

### 3.4 Permission baru

```
final_report.view     — lihat daftar, buka Mode Present
final_report.manage   — isi susunan tim, pilih foto, generate/update PPTX
```

Scoping kabupaten memakai `auth.RegencyScope` yang sama persis dengan modul lain — tidak ada tabel/kolom scope baru.

## 4. Pemetaan Slide ke Sumber Data

| Slide | Isi | Sumber data | Catatan |
|---|---|---|---|
| 1. Judul | Nama program, PT KSM, nama Kabupaten, tahun anggaran | `programs`, `regencies`, `program_schedules.fiscal_year` | Statis per program+kabupaten |
| 2. Pengiriman & Penerimaan + Rapat Koordinasi | Tanggal per jenis barang, nama Dinas, tanggal & daftar hadir Rakor | `bast_schedule_settings` (nama Dinas dkk), data Rakorda di `internal/bast` | Daftar hadir memakai daftar pihak tetap (ESDM, Pertamina, Dinas, PT KSM, Konsultan, Pabrikan) — nama institusi dari `bast_schedule_settings`, sisanya teks tetap |
| 3. Sosialisasi + Susunan Tim | Tanggal & hadir Sosialisasi, jumlah Leader/Admin/Teknisi/Helper | Data Sosialisasi existing (`internal/bast`) + `final_report_settings` | Susunan tim adalah input baru khusus fitur ini |
| 4. Pendistribusian | Lokasi gudang, tanggal mulai–selesai, total paket | Modul `distribution` (agregat per program+kabupaten) | |
| 5. Dokumentasi Pengiriman & Penerimaan | Foto terpilih | `activity_media` (activity_type pengiriman/unloading) via `final_report_photo_picks` slide_key=`pengiriman_penerimaan` | |
| 6. Dokumentasi Rapat Koordinasi | Foto terpilih | `activity_media` (rakor) via slide_key=`rapat_koordinasi` | |
| 7. Dokumentasi Seremoni & Sosialisasi | Foto terpilih | `activity_media` (sosialisasi) via slide_key=`sosialisasi` | |
| 8. Dokumentasi Pendistribusian | Foto terpilih | `activity_media` (distribusi) via slide_key=`pendistribusian` | |
| 9. Terima Kasih | Statis | — | |

Kalau data sumber untuk suatu field belum ada (mis. Rakorda belum diisi untuk kabupaten itu), field tersebut dirender sebagai "Belum diisi" di slide — generate dan Present tetap jalan.

## 5. Mekanisme Generate PPTX

Tidak ada library Go yang matang dan gratis untuk membangun PPTX dari nol (berbeda dari PDF yang sudah memakai `github.com/go-pdf/fpdf` di `internal/bast`). Pendekatan yang dipakai: **substitusi master template**, bukan membangun dari nol.

- File `.pptx` referensi disimpan sebagai aset statis di server (`internal/finalreport/template/final_report_template.pptx`), dengan placeholder teks tertanam di tiap run teks (`{{REGENCY_NAME}}`, `{{HANDOVER_DATE_X}}`, dst. — daftar lengkap placeholder didefinisikan sebagai konstanta Go, satu per field di tabel bagian 4).
- Karena `.pptx` adalah ZIP berisi XML per slide, substitusi dilakukan dengan Go stdlib murni:
  1. Buka arsip dengan `archive/zip`.
  2. Untuk tiap `ppt/slides/slideN.xml`, cari teks placeholder di dalam elemen `<a:t>` dan ganti dengan nilai asli (string replace sederhana — placeholder memakai pola unik `{{...}}` yang tidak akan bentrok dengan teks biasa).
  3. Untuk slide dokumentasi (5-8), ganti isi file gambar di `ppt/media/imageN.*` yang menjadi rel dari slide itu dengan byte foto yang dipilih (diambil lewat `media.Storage.Open`), tanpa mengubah struktur relationship/posisi.
  4. Tulis ulang sebagai arsip ZIP baru (checksum dihitung dari hasil akhir).
- Tidak ada dependency baru (`encoding/xml`, `archive/zip` sudah di stdlib Go).

### Mode Present

Dirender terpisah di frontend sebagai komponen React (`FinalReportPresent.tsx`) — bukan konversi PPTX ke gambar, tapi membangun ulang tiap slide sebagai HTML/CSS yang layout-nya dibuat semirip mungkin dengan template, dipasok dari endpoint JSON yang sama (lihat `GET /final-report/preview` di bagian 6) yang juga jadi sumber data generate PPTX — satu sumber kebenaran, dua cara tampil. Fullscreen, navigasi next/prev/Esc, tanpa dependency library presentasi eksternal.

## 6. API

```
GET   /api/v1/final-report/settings?program_id=&regency_id=
PUT   /api/v1/final-report/settings
GET   /api/v1/final-report/photo-picks?program_id=&regency_id=&slide_key=
PUT   /api/v1/final-report/photo-picks
GET   /api/v1/final-report/preview?program_id=&regency_id=      -- data lengkap untuk Mode Present (JSON)
POST  /api/v1/final-report/generate                              -- body: program_id, regency_id
GET   /api/v1/final-report/documents?program_id=&regency_id=     -- riwayat versi
```

`PUT /photo-picks` menggantikan seluruh daftar pick untuk satu `slide_key` dalam satu panggilan (bukan tambah satu-satu), supaya urutan (`sort_order`) selalu konsisten dengan urutan checkbox terakhir di UI.

Semua endpoint memerlukan `final_report.view` (GET) atau `final_report.manage` (PUT/POST), dan di-scope kabupaten lewat `auth.RegencyScope` seperti modul lain.

## 7. Folder Tujuan di Drive

Fungsi baru di `internal/media/folder_path.go`, terpisah dari `BuildFolderPath` (tidak mengubah/melonggarkan validasi yang ada untuk 3 kategori lain):

```go
// BuildFinalReportPath constructs the storage path for a Final Report PPTX:
// directly inside the regency folder, as a sibling of the 3 category
// folders, not nested under any of them.
func BuildFinalReportPath(input FolderPathInput) ([]string, error)
```

Hasilnya `[]string{programSegment, zoneSegment, regencySegment}` (tanpa segmen kategori) — `input.Category` dan `input.Child` diabaikan untuk fungsi ini. Nama file: `LAPORAN AKHIR - <NAMA PROGRAM> <NAMA KABUPATEN>.pptx`.

## 8. Frontend

- Sidebar: item baru "Laporan Akhir" (permission `final_report.view`) masuk ke grup **"Laporan"** yang sudah ada di `AppShell.tsx`, bersama item "Laporan" lama — bukan grup baru.
- Halaman baru `FinalReportPage.tsx`, route `/laporan-akhir`: pilih Program → Kabupaten (pola dua-langkah yang sama seperti `ActivityDocumentationPage`/`SupportingDocumentsPage`), lalu untuk kombinasi yang dipilih menampilkan:
  - Form singkat 4 angka (Leader/Admin/Teknisi/Helper) — tersimpan otomatis saat blur, pola yang sama dengan `DocumentInfoFields.tsx`.
  - 4 galeri (satu per `slide_key`) menampilkan foto dari `activity_media` terkait dengan checkbox pilih (maks 6 per galeri) — mirip pola pilih cover.
  - Tombol **"Tampilkan"** → buka `FinalReportPresent.tsx` full-screen.
  - Tombol **"Generate PPTX"** (perlu `final_report.manage`) → panggil `POST /generate`, tampilkan status loading/sukses/gagal (toast, pola yang sama dengan modul lain), lalu riwayat versi di bawahnya dengan link unduh per versi.

## 9. Error Handling

- Slide dengan data sumber belum lengkap → placeholder "Belum diisi", tidak memblokir generate maupun Present.
- Foto yang dipilih tapi kemudian terhapus dari Dokumentasi → otomatis hilang dari pick (cascade di level DB), galeri pilihan di UI refresh menampilkan sisanya.
- Generate gagal (mis. Drive API error) → `final_report_documents` tidak bertambah baris, service mengembalikan error yang dipetakan ke pesan Indonesia di `writeServiceError` (pola yang sama dengan domain lain), UI menampilkan toast error tanpa mengubah riwayat versi yang sudah ada.
- Template PPTX master rusak/tidak ketemu di server → dianggap kesalahan konfigurasi server (500), bukan kesalahan input user — dicatat di log server untuk investigasi, bukan permission/validasi.

## 10. Testing

- Unit test `BuildFinalReportPath` — hasil path benar, tidak ada segmen kategori, karakter tidak valid ditolak (pola sama dengan test `folder_path_test.go` yang sudah ada).
- Unit test substitusi template — pakai file `.pptx` dummy kecil (2 slide placeholder) di `testdata/`, bukan template asli berukuran penuh; verifikasi placeholder terganti, gambar di slide dokumentasi berubah sesuai foto yang dipasok, dan arsip ZIP hasil akhir valid (bisa dibuka ulang).
- Service test `GenerateReport` — kombinasi data lengkap vs sebagian kosong (placeholder "Belum diisi" muncul yang seharusnya), batas 6 foto per slide_key ditegakkan.
- Integration test regency-scope — role tanpa akses kabupaten X ditolak di semua endpoint baru, konsisten dengan pola test regency-scope modul lain.
- Frontend test `FinalReportPage` — form susunan tim tersimpan saat blur, checkbox pilih foto menghormati batas 6, tombol Generate menampilkan loading/sukses/gagal.
- Frontend test `FinalReportPresent` — merender data dari `/preview` dengan benar, termasuk kasus field kosong menampilkan placeholder, navigasi next/prev/Esc berfungsi.

## 11. Review Focus

- Template PPTX master hilang/corrupt di server saat generate diminta — harus gagal dengan error 500 yang jelas di log, bukan diam-diam menghasilkan file kosong/corrupt.
- Foto yang dipilih berasal dari `activity_media` milik kabupaten/program lain (salah kirim ID) — service harus menolak, bukan ikut menyisipkannya ke PPTX kabupaten yang salah.
- Generate PPTX dua kali berturut-turut dengan cepat (klik ganda) — harus menghasilkan versi berurutan yang benar, bukan race condition pada nomor versi (pola lock yang sama dengan `bast_aggregate_documents`).
- User dengan `final_report.view` tapi tanpa `final_report.manage` mencoba memilih foto/generate lewat API langsung — harus ditolak 403, bukan cuma disembunyikan di UI.
- Nama kabupaten/program mengandung karakter yang tidak valid untuk nama file Drive — `BuildFinalReportPath`/nama file harus mensanitasi, bukan gagal diam-diam atau membuat file dengan nama rusak.
