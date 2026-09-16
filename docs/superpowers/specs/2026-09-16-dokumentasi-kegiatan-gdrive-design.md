# Rancangan Dokumentasi Kegiatan Lapangan (9 Modul) + Penyimpanan Google Drive

Versi: 1.0
Tanggal: 2026-09-16
Status: Menunggu review pengguna

## 1. Tujuan

Sidebar `Dokumentasi` saat ini hanya berisi Pendistribusian dan Laporan. Fase ini menambahkan 9 modul dokumentasi kegiatan lapangan baru — Ceremony & Sosialisasi, Pelatihan Teknis, Rakor, Training 10%, Training 100%, Unloading Konkit, Unloading Mesin Pompa, Unloading Oli, Unloading Selang Hisap & Buang, Unloading Tabung Gas — masing-masing berupa galeri unggah foto/video bebas (tidak terikat slot wajib seperti Pendistribusian), dan memindahkan penyimpanan file ke Google Drive (menggantikan/mendampingi disk lokal).

Laporan dipindahkan keluar dari grup sidebar Dokumentasi menjadi grup tersendiri, karena secara konsep bukan modul dokumentasi.

## 2. Non-tujuan

- Tidak mengubah alur Pendistribusian yang sudah ada (`documentation_slots`/`media_files` tetap seperti sekarang, tidak disentuh skema-nya) — modul baru ini pakai tabel & alur terpisah, bukan perluasan tabel lama.
- File lama yang sudah ada di disk lokal (termasuk foto Pendistribusian) **tidak dimigrasi** ke Google Drive pada fase ini. Hanya upload baru yang diarahkan ke backend penyimpanan yang aktif.
- Fitur "file manager" untuk mengelola/reorganisasi file di Drive (disebut sebagai kebutuhan masa depan) **tidak** dibangun di fase ini — desain penyimpanan (Service Account, struktur folder) disiapkan supaya kompatibel dengan itu nanti, tapi implementasinya menyusul.
- Folder "BERITA ACARA (BA)" dan "DOKUMEN PENDUKUNG" di tiap kabupaten **disiapkan strukturnya saja** (dibuat kosong) — isinya adalah fitur terpisah di masa depan, di luar scope fase ini.
- Tidak ada perubahan pada `documentation_template_slots`/pola template Pendistribusian.

## 3. Model Data

Satu tabel baru, generik untuk ke-10 jenis kegiatan (termasuk Pendistribusian? **Tidak** — Pendistribusian tetap pakai `media_files`; tabel ini khusus 9 modul baru):

```sql
CREATE TABLE activity_media (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    regency_id uuid NOT NULL REFERENCES regencies(id),
    activity_type text NOT NULL CHECK (activity_type IN (
        'ceremony_sosialisasi', 'pelatihan_teknis', 'rakor',
        'training_10', 'training_100',
        'unloading_konkit', 'unloading_mesin_pompa', 'unloading_oli',
        'unloading_selang', 'unloading_tabung_gas'
    )),
    storage_key text NOT NULL UNIQUE,
    display_name text NOT NULL,
    original_filename text NOT NULL,
    media_type text NOT NULL CHECK (media_type IN ('image', 'video')),
    mime_type text NOT NULL CHECK (mime_type IN (
        'image/jpeg', 'image/png', 'image/webp',
        'video/mp4', 'video/webm', 'video/quicktime'
    )),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 104857600), -- 100 MiB
    checksum char(64) NOT NULL,
    source text NOT NULL CHECK (source IN ('camera', 'gallery')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleted')),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_media_regency_type_idx ON activity_media (regency_id, activity_type, status, uploaded_at DESC);
```

- **Tidak** ada kolom `schedule_id`/`program_id` — murni kabupaten + jenis kegiatan + waktu, sesuai keputusan pengguna.
- `display_name` = `{KODE_KABUPATEN}-{KODE_KEGIATAN}-{YYYYMMDD}-{HHMMSS}`, contoh `WJO-RAKOR-20260916-154500`. Digenerate server-side saat upload, bukan diketik user.
- `storage_key` bertipe `text` (bukan `uuid` seperti `media_files.storage_key`) karena Google Drive file ID bukan format UUID — untuk `LocalStorage` tetap dipakai UUID string seperti biasa.
- Cap ukuran file 100 MiB (menampung video) — lebih besar dari cap `media_files` (10 MiB, khusus foto, tidak diubah).
- Soft delete via `status` (pola sama seperti Batalkan/Pulihkan di Data Penerima).

### Migration permission

Ditambahkan `activities.view` / `activities.manage`, satu pasang untuk seluruh 9 modul (bukan per-modul), di-grant ke `super_admin`.

## 4. Penyimpanan: Google Drive + Local (pluggable)

### 4.1 Perubahan interface `internal/media.Storage`

Interface saat ini:
```go
type Storage interface {
    Put(context.Context, string, io.Reader) (int64, string, error)
    Open(context.Context, string) (io.ReadCloser, error)
    Delete(context.Context, string) error
}
```

`Put` tidak membawa informasi folder — cukup untuk `LocalStorage` yang flat, tapi Google Drive butuh tahu folder tujuan (Root → Konkit {tahun} → {Kabupaten} → Dokumentasi Foto & Video → {Jenis Kegiatan}). Interface diperluas:

```go
type Storage interface {
    Put(ctx context.Context, key string, folderPath []string, r io.Reader) (int64, string, error)
    Open(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}
```

- `folderPath` adalah daftar nama folder berurutan dari root logis (mis. `["Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"]`).
- **`LocalStorage.Put` mengabaikan `folderPath`** (tetap flat seperti sekarang) — perilaku Pendistribusian **tidak berubah sama sekali**.
- **`GoogleDriveStorage.Put`** memakai `folderPath` untuk resolve/membuat folder secara idempoten (lihat 4.3).
- Pemanggil lama (`internal/distribution/repository.go`'s panggilan ke `storage.Put`) diupdate untuk mengirim `nil` sebagai `folderPath` — perubahan mekanis, tidak mengubah hasil.

### 4.2 Autentikasi

Service Account Google Cloud. Kredensial JSON dibaca dari path di env var `GDRIVE_SERVICE_ACCOUNT_JSON`. Folder root ("Folder Induk") sudah di-share manual oleh pengguna ke email Service Account (Editor access) — ID-nya di env var `GDRIVE_ROOT_FOLDER_ID`. Library: `google.golang.org/api/drive/v3` + `golang.org/x/oauth2/google`.

### 4.3 Struktur folder & caching ID

```
{GDRIVE_ROOT_FOLDER_ID}/
  Konkit {tahun berjalan}/
    {Nama Kabupaten lengkap}/         (mis. "Wajo", bukan "WJO")
      BERITA ACARA (BA)/              (dibuat kosong, fitur masa depan)
      DOKUMEN PENDUKUNG/              (dibuat kosong, fitur masa depan)
      DOKUMENTASI FOTO & VIDEO/
        {Jenis Kegiatan}/             (mis. "Rakor", "Unloading Konkit")
```

Resolusi folder Drive API (search-by-name-under-parent) lambat & boros kuota kalau dipanggil tiap upload. Tabel cache baru:

```sql
CREATE TABLE drive_folder_cache (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    path_key text NOT NULL UNIQUE,   -- mis. "wajo/dokumentasi-foto-video/rakor"
    drive_folder_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
```

`GoogleDriveStorage.Put` cek cache dulu; kalau belum ada, resolve/buat folder via Drive API (membuat seluruh rantai folder yang belum ada, termasuk BA & Dokumen Pendukung sekali per kabupaten), lalu simpan ke cache.

### 4.4 Konfigurasi

Env var baru: `STORAGE_BACKEND` (`local` default, atau `gdrive`), `GDRIVE_SERVICE_ACCOUNT_JSON`, `GDRIVE_ROOT_FOLDER_ID`. `cmd/server/main.go` memilih implementasi `Storage` berdasarkan `STORAGE_BACKEND`; kalau `gdrive` tapi kredensial tidak ada, server gagal start dengan pesan jelas (fail-fast, bukan fallback diam-diam ke local).

### 4.5 Preview

`GET /api/v1/activities/media/{id}/content` — backend stream isi file dari Drive (via `Storage.Open`) ke response, sama persis pola `OpenMedia` yang sudah ada di Pendistribusian. File di Drive **tidak** di-share publik/"siapapun dengan link" — akses selalu lewat backend yang menegakkan permission Konkit.

## 5. Backend

Paket baru `internal/activities` (models.go, repository.go, service.go), mengikuti pola persis `internal/recipients` yang baru selesai dibangun.

- `List(ctx, Filter{RegencyID, ActivityType, Page, PageSize}, scope) (Page, error)`
- `Upload(ctx, actor, UploadInput{RegencyID, ActivityType, Filename, MimeType, Source, Content io.Reader}, meta, scope) (ActivityMedia, error)` — validasi kabupaten dalam scope, validasi `activity_type` & mime type, generate `display_name`, panggil `storage.Put` dengan `folderPath` sesuai 4.3, simpan metadata, audit `activity_media.uploaded`.
- `Delete(ctx, actor, id, meta, scope) error` — soft delete, audit `activity_media.deleted`.
- `OpenContent(ctx, id, scope) (io.ReadCloser, mimeType string, error)`.

API (`internal/api/activities_routes.go`), pola sama seperti `recipients_routes.go`:
- `GET /api/v1/activities/media` — `activities.view`
- `POST /api/v1/activities/media` — `activities.manage`, multipart upload
- `DELETE /api/v1/activities/media/{id}` — `activities.manage`
- `GET /api/v1/activities/media/{id}/content` — `activities.view`

## 6. Frontend

- Komponen generik `ActivityDocumentationPage.tsx` (props: `activityType`, `label`) — dipakai ulang untuk 9 route.
- Isi halaman: pilih Kabupaten (opsi mengikuti `RegencyScope` user) → galeri grid (thumbnail foto, ikon play untuk video) dengan pagination → tombol "Ambil Foto/Video" & "Pilih dari Galeri" (accept image+video) → upload → muncul di galeri → klik untuk preview modal (gambar penuh / video player) → tombol hapus kalau `activities.manage`.
- Route baru di `routes.tsx`, masing-masing dibungkus `ProtectedPage permission="activities.view"`.
- `AppShell.tsx`: grup "Dokumentasi" bertambah 9 item baru; "Laporan" dipindah keluar jadi grup sendiri.

## 7. Testing

- Backend: `internal/activities/service_test.go` (validasi), `internal/activities/repository_integration_test.go` (live Postgres, pola sama seperti `internal/recipients`).
- `GoogleDriveStorage` diuji lewat interface `Storage` dengan implementasi test-double (tanpa memanggil Drive API sungguhan di CI) — pengujian folder-path-resolution logic diuji terpisah dari pengujian network call.
- Frontend: test per halaman (mocked API), pastikan permission gating & upload flow (mocked fetch).

## 8. Risiko & Catatan Terbuka

- Perubahan signature `Storage.Put` menyentuh kode Pendistribusian yang sudah berjalan di produksi — perlu regresi penuh `internal/distribution` setelah perubahan (perilaku behavior harus identik, cuma tambah parameter `nil`).
- Google Drive API punya rate limit & bisa lebih lambat dari disk lokal — upload video besar (mendekati 100 MiB) perlu diuji end-to-end untuk memastikan tidak timeout di sisi handler HTTP.
- `drive_folder_cache` bisa jadi stale kalau folder dihapus/dipindah manual dari Drive — perlu penanganan error saat `Storage.Put` gagal karena folder ID di cache sudah tidak valid (fallback: hapus entry cache, re-resolve).
- Kredensial Service Account adalah rahasia sensitif — harus lewat `.env`/environment variable seperti `SESSION_SECRET`, tidak pernah masuk git.
