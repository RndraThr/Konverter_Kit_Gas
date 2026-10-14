# Rancangan Fitur Dokumen Pendukung

Versi: 1.0
Tanggal: 2026-10-13
Status: Menunggu review pengguna

## 1. Tujuan

Folder "DOKUMEN PENDUKUNG" di Google Drive sudah disiapkan per kabupaten sejak fase dokumentasi-kegiatan-gdrive (`2026-09-16-dokumentasi-kegiatan-gdrive-design.md`), tapi selalu dibiarkan kosong — modul upload/kelolanya belum pernah dibangun. Fase ini membangun modul itu: ke-10 pada daftar Dokumentasi Kegiatan, khusus untuk menyimpan dokumen pendukung dalam bentuk PDF.

Dua cara menambah dokumen:
1. Upload file PDF langsung (file picker).
2. Scan dokumen fisik lewat kamera mobile — deteksi tepi otomatis, crop + koreksi perspektif, peningkatan kontras/ketajaman, bisa multi-halaman, hasil akhir satu file PDF.

Keduanya berakhir sebagai satu record di daftar Dokumen Pendukung per kabupaten, tanpa kategori/sub-jenis — semua masuk satu daftar, bisa lebih dari satu PDF.

## 2. Non-tujuan

- Tidak ada kategorisasi/sub-jenis dokumen dalam arti folder/struktur penyimpanan — semua PDF tetap masuk satu daftar flat di satu folder Drive. "Slot nama dokumen" (bagian 3.1) hanyalah daftar pilihan judul untuk penamaan otomatis saat upload, bukan kategori penyimpanan — satu Jadwal bisa punya banyak dokumen dengan nama slot yang sama (mis. dua "Surat Tugas" di tanggal berbeda), semuanya tetap satu daftar yang sama.
- Tidak ada mode filter warna pilihan (hitam-putih, dst.) — hanya satu jalur peningkatan kontras/ketajaman otomatis, bukan beberapa mode yang bisa dipilih user.
- Tidak mengubah 9 modul Dokumentasi Kegiatan (foto/video) yang sudah ada — ini modul ke-10 yang berdiri sendiri, memakai pola yang sama tapi dengan tipe file dan folder tujuan berbeda.
- Tidak membangun deteksi/validasi isi dokumen (OCR, pembacaan teks) — PDF disimpan sebagai berkas biner, tidak diproses lebih lanjut.
- Tidak ada migrasi dokumen pendukung lama — fitur ini hanya menangani upload baru.

## 3. Model Data

Tidak ada tabel baru. Dipakai ulang `activity_media` (lihat `2026-09-16-dokumentasi-kegiatan-gdrive-design.md` bagian 3) dengan `activity_type` baru:

```
dokumen_pendukung
```

Ditambahkan ke registry di `internal/activities/models.go`:

```go
var activityTypeCodes = map[string]string{
    // ... 9 entri yang sudah ada ...
    "dokumen_pendukung": "DOKPEND",
}
```

Berbeda dari 9 `activity_type` lain, `dokumen_pendukung` TIDAK memiliki entri di `activityTypeFolderNames` — foldernya adalah kategori `DOKUMEN PENDUKUNG` itu sendiri (lihat bagian 5), tidak ada subfolder per jenis.

`media_type` yang tersimpan untuk `dokumen_pendukung` selalu `"document"` (lihat bagian 4), berbeda dari 9 modul lain yang selalu `"image"`/`"video"`.

### 3.1 Tabel Baru: `supporting_document_slots`

9 modul Dokumentasi Kegiatan lain diakses lewat kombinasi Program+Kabupaten (zona), bukan Jadwal (`program_schedules`) tertentu. Dokumen Pendukung berbeda: daftar nama dokumen yang bisa dipilih saat upload/scan diatur **per Jadwal Program**, supaya Jadwal menjadi satu gerbang pengaturan (konsisten dengan `document_template_id`/`bast_schedule_settings` yang juga diatur per Jadwal, bukan global atau per-zona).

```sql
CREATE TABLE supporting_document_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id uuid NOT NULL REFERENCES program_schedules(id) ON DELETE CASCADE,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_supporting_document_slots_schedule ON supporting_document_slots(schedule_id) WHERE is_active;
```

Super Admin/Admin Kabupaten menyiapkan daftar nama (mis. "Surat Tugas", "SK Penetapan") saat mengatur Jadwal Program yang bersangkutan, di panel Persiapan Program yang sudah ada (sibling dari pengaturan paket/template dokumen). Petugas lapangan, saat upload/scan Dokumen Pendukung, memilih dari daftar ini — tidak mengetik nama bebas.

Nama slot TIDAK disimpan sebagai foreign key di `activity_media` — hanya dipakai sekali untuk menghasilkan `display_name` saat upload (lihat bagian 7 dan 8.1), sama seperti `activityTypeCodes` dipakai sekali untuk 9 modul lain. Ini supaya riwayat dokumen tidak berubah kalau nama slot di-rename atau dinonaktifkan kemudian (prinsip snapshot yang sudah dipakai di `bast_documents`).

## 4. Deteksi Tipe File — `internal/media/upload.go`

File ini dipakai lintas modul (Pendistribusian, Dokumentasi Kegiatan), jadi perubahan harus aditif, tidak mengubah perilaku yang sudah ada.

Tambahan:

```go
const (
    KindImage    Kind = "image"
    KindVideo    Kind = "video"
    KindDocument Kind = "document"   // baru

    MaxImageBytes    int64 = 25 << 20
    MaxVideoBytes    int64 = 500 << 20
    MaxDocumentBytes int64 = 20 << 20  // baru — PDF dokumen pendukung
)
```

Di `DetectUpload`, tambah case untuk PDF (deteksi lewat `http.DetectContentType` yang mengenali `application/pdf` dari magic bytes `%PDF-`):

```go
case "application/pdf":
    detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindDocument, mimeType, ".pdf", MaxDocumentBytes
```

`PolicyAllows` ditambah satu policy baru:

```go
case "document":
    return kind == KindDocument
```

9 `activity_type` lain tetap memvalidasi `KindImage`/`KindVideo` seperti sekarang — tidak berubah.

## 5. Folder Tujuan — `internal/media/folder_path.go` dan `internal/activities/service.go`

`FolderSupporting = "DOKUMEN PENDUKUNG"` sudah ada sebagai konstanta kategori sejak fase gdrive, belum pernah dipakai service manapun. `BuildFolderPath` sudah mendukung `Child` kosong (flat, tanpa subfolder) — tidak perlu ubah `folder_path.go`.

`internal/activities/service.go` saat ini meng-hardcode `Category: media.FolderPhotos` untuk semua `activity_type` di method `Upload`. Diganti jadi lookup per tipe:

```go
func folderDestination(activityType string) (media.FolderCategory, string) {
    if activityType == "dokumen_pendukung" {
        return media.FolderSupporting, ""
    }
    return media.FolderPhotos, activityTypeFolderNames[activityType]
}
```

Dipakai di `Upload` dan di method baru `ScanUpload` (bagian 7) saat memanggil `media.BuildFolderPath`.

## 6. Validasi Tipe File Per `activity_type`

`Upload` saat ini menolak upload yang bukan `KindImage`/`KindVideo` secara umum (lewat `media.DetectUpload` yang memang hanya mengenali image/video sebelum fase ini). Dengan `KindDocument` ditambahkan ke `DetectUpload`, perlu validasi tambahan per `activity_type` — dipakai ulang `media.PolicyAllows` (bagian 4) yang sudah menerima parameter policy string:

```go
func policyFor(activityType string) string {
    if activityType == "dokumen_pendukung" {
        return "document"
    }
    return "image_video"
}
```

Dipanggil setelah `media.DetectUpload` berhasil di `Upload`: `if !media.PolicyAllows(policyFor(input.ActivityType), detected.Kind) { return ActivityMedia{}, ErrMediaTypeInvalid }`. Pesan error tetap `ErrMediaTypeInvalid` yang sudah ada, tidak perlu pesan baru.

## 7. Endpoint Scan — Penyusunan PDF Dari Hasil Crop Client

Deteksi tepi, crop, koreksi perspektif, dan peningkatan kontras/ketajaman terjadi di client (bagian 9) — backend hanya menerima JPEG per halaman yang SUDAH diproses, dan menyusunnya jadi satu PDF.

Endpoint baru: `POST /api/v1/activities/supporting-documents/scan`

Request: `multipart/form-data` dengan:
- `schedule_id` — Jadwal Program yang sedang dikerjakan (menggantikan `program_id`+`regency_id` terpisah; keduanya diturunkan dari baris `program_schedules` ini, lihat bagian 7.1).
- `slot_id` — salah satu `supporting_document_slots` milik `schedule_id` tersebut, dan harus `is_active`.
- `page` (bisa berulang, urutan sesuai urutan dikirim) — tiap bagian adalah satu JPEG halaman hasil crop dari client.

Validasi:
- `schedule_id` harus ada dan dalam cakupan regency scope aktor.
- `slot_id` harus milik `schedule_id` yang dikirim dan `is_active` — kalau tidak, `ErrSlotInvalid`.
- Minimal 1 halaman, maksimal 30 halaman per dokumen (`ErrScanTooManyPages` jika lebih).
- Tiap halaman divalidasi sebagai `KindImage` lewat `media.DetectUpload` yang sudah ada (menolak file yang bukan JPEG/PNG asli), DAN dibatasi maksimal 6 MB per halaman (`MaxScanPageBytes int64 = 6 << 20`) — lebih kecil dari `MaxImageBytes` (25 MB) karena ini hasil crop+kompresi dari client, bukan foto mentah.
- Total ukuran request (seluruh halaman) dibatasi 60 MB (`MaxScanRequestBytes int64 = 60 << 20`) sebagai pagar tambahan terhadap limit per-halaman × jumlah halaman, supaya `fpdf` tidak memegang buffer yang terlalu besar di memori sekaligus.

Proses: `Service.ScanUpload` memvalidasi `schedule_id`/`slot_id` (bagian 7.1), membuat satu dokumen `fpdf.New(...)`, untuk setiap halaman: `AddPage()`, hitung ukuran halaman dari rasio gambar, `ImageOptions(...)` untuk menempelkan JPEG sebagai isi halaman penuh. Hasil `fpdf` di-`Output` ke buffer, lalu buffer itu diteruskan ke jalur penyimpanan yang sama dengan `Upload` (helper internal dibagi antara `Upload` dan `ScanUpload` — keduanya berakhir di: hitung `storageKey`, `media.PutNamed`, `repository.Insert` dengan `MediaType: "document"`, `MimeType: "application/pdf"`, `DisplayName` dari nama slot yang dipilih).

`ScanUpload` TIDAK menerima `activity_type` sebagai parameter — selalu `"dokumen_pendukung"`, karena endpoint ini khusus untuk fitur ini.

### 7.1 Resolusi `schedule_id` → `program_id`/`regency_id`, dan Penamaan Dari Slot

Dokumen Pendukung memakai `schedule_id` sebagai satu-satunya pilihan yang perlu dibuat petugas lapangan (bukan pilih Program lalu pilih Kabupaten terpisah seperti 9 modul lain) — ini wujud "satu gerbang" yang dimaksud. `Service.Upload`/`ScanUpload`, khusus untuk `activity_type == "dokumen_pendukung"`, menerima `ScheduleID` dan `SlotID` menggantikan `ProgramID`/`RegencyID` langsung di request:

1. Ambil baris `program_schedules` dari `schedule_id` (lewat `programContexts`, perlu satu method baru `ResolveScheduleContext(ctx, scheduleID, scope) (programs.StorageContext, SlotMasterScheduleID string, err error)` atau memakai `programs.Service.GetSchedule` yang sudah ada untuk membaca `ProgramID`/`RegencyID`-nya) — hasilnya dipakai sebagai `ProgramID`/`RegencyID` ke `ResolveStorageContext` yang sudah ada (bagian 5), tidak ada perubahan di `ResolveStorageContext` itu sendiri.
2. Ambil baris `supporting_document_slots` dari `slot_id`, pastikan `schedule_id`-nya cocok dan `is_active`.
3. `displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, sanitizeSlotName(slot.Name), now.Format("20060102-150405"))` — pola sama seperti 9 modul lain, hanya kode aktivitas (`activityTypeCodes[type]`) diganti nama slot yang disanitasi (huruf besar, spasi jadi `-`), supaya nama file yang terlihat di Drive langsung mencerminkan judul yang dipilih, bukan kode generik.

Untuk upload file langsung (bukan scan), `UploadInput` untuk `dokumen_pendukung` juga memakai `ScheduleID`+`SlotID` (bukan `ProgramID`/`RegencyID`), lewat jalur yang sama di atas. 9 `activity_type` lain tidak berubah — tetap memakai `ProgramID`/`RegencyID` langsung seperti sekarang.

## 8. API

### 8.1 Dipakai Ulang (Tidak Berubah)

- `GET /api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=...` — daftar dokumen, sudah ada.
- `POST /api/v1/activities/media` dengan `activity_type=dokumen_pendukung` — upload file PDF langsung (jalur file picker); body menambah `schedule_id`+`slot_id` menggantikan `regency_id` mentah (lihat 7.1), sisanya memakai jalur yang sudah ada, cukup diterima lewat validasi tipe file baru di bagian 6.
- `DELETE /api/v1/activities/media/{id}` — hapus dokumen, sudah ada.
- `GET /api/v1/activities/media/{id}/content` — unduh/lihat PDF, sudah ada.
- Permission dokumen: `activities.view` (lihat daftar, unduh), `activities.manage` (upload, scan, hapus) — keduanya sudah ada, tidak ada permission baru.

### 8.2 Baru — Pengelolaan `supporting_document_slots`

- `GET /api/v1/activities/supporting-documents/slots?schedule_id=...` — daftar slot aktif milik satu Jadwal (dipakai untuk pilihan saat upload/scan). Permission: `activities.view` (field officer yang bisa upload juga bisa melihat daftar pilihan).
- `POST /api/v1/activities/supporting-documents/slots` — buat slot baru `{schedule_id, name}`. Permission: `programs.manage` (sama seperti pengaturan Jadwal Program lain, mis. `bast_schedule_settings` di `bast_routes.go`).
- `PATCH /api/v1/activities/supporting-documents/slots/{id}` — ubah `name` dan/atau `is_active` (nonaktifkan, bukan hapus — dokumen yang sudah terupload lewat slot itu tidak terpengaruh karena namanya sudah jadi snapshot di `display_name`). Permission: `programs.manage`.
- Tidak ada hard-delete endpoint — konsisten dengan prinsip snapshot di bagian 3.1.

## 9. Frontend

### 9.1 Daftar & Upload Biasa

Modul ke-10 di sidebar Dokumentasi, memakai ulang komponen galeri generik yang sudah ada untuk 9 modul lain (list, hapus), dengan perbedaan:
- Pemilih konteks di atas galeri adalah **Jadwal Program** (satu dropdown `schedule_id`), bukan Program lalu Kabupaten dua langkah seperti 9 modul lain — konsisten dengan "satu gerbang" di bagian 7.1.
- `accept="application/pdf"` pada input file (bukan image/video).
- Tampilan item daftar: ikon PDF + nama slot yang dipilih saat upload (bukan thumbnail gambar).
- Tombol upload file maupun "Scan dari Kamera" (9.2) sama-sama diawali dialog kecil **"Pilih nama dokumen"** — dropdown isi `supporting_document_slots` milik Jadwal yang aktif (dari `GET .../slots?schedule_id=...`). Kalau daftar masih kosong (belum diisi Admin di Persiapan Program), tombol upload/scan nonaktif dengan pesan "Minta Admin Kabupaten menyiapkan daftar nama dokumen di Persiapan Program dulu."

`frontend/src/features/activities/types.ts`: tambah `'dokumen_pendukung'` ke union `ActivityType` dan `activityTypeLabels`; tambah tipe `SupportingDocumentSlot = { id: string; schedule_id: string; name: string; is_active: boolean }`.

### 9.1a Panel Kelola Slot — Persiapan Program

Di halaman Persiapan Program (`frontend/src/features/programs/`), pada pengaturan satu Jadwal, ditambah satu panel kecil **"Nama Dokumen Pendukung"** (sibling dari panel paket/template dokumen yang sudah ada): daftar nama yang sudah dibuat (dengan toggle aktif/nonaktif dan tombol ubah nama), plus form tambah nama baru. Memakai endpoint 8.2, permission `programs.manage` (sudah otomatis konsisten karena halaman Persiapan Program sendiri sudah di-gate permission itu).

### 9.2 Komponen Scan Kamera — `DocumentScanner.tsx` (baru)

Lazy-loaded (`lazy(() => import('./DocumentScanner'))`) seperti `BarcodeScanner`, supaya OpenCV.js (berat) hanya dimuat saat tombol "Scan dari Kamera" ditekan.

Alur komponen:
1. **Live kamera** — `getUserMedia`, tampilkan video stream. Overlay deteksi tepi dijalankan per-frame (throttled, mis. setiap 200–300ms, bukan setiap frame, supaya tidak membebani CPU HP) memakai library berbasis OpenCV.js (kelas library seperti `jscanify`) — overlay garis hijau di atas tepi dokumen yang terdeteksi.
2. **Capture** — tombol shutter mengambil frame penuh resolusi, menjalankan deteksi tepi final + `cv.warpPerspective` (atau API sejenis dari library yang dipakai) untuk crop & meratakan perspektif berdasarkan 4 titik yang terdeteksi.
3. **Review per halaman** — hasil crop ditampilkan dengan 4 titik sudut yang bisa digeser manual (untuk koreksi kalau deteksi otomatis kurang pas), tombol "Pakai halaman ini" menjalankan ulang warp dengan titik yang sudah disesuaikan (kalau digeser) lalu menjalankan peningkatan kontras/ketajaman otomatis (operasi OpenCV.js, mis. kombinasi `cv.filter2D` dengan kernel sharpen + penyesuaian kontras), hasil akhir disimpan sebagai JPEG di state lokal (array halaman untuk dokumen yang sedang disusun).
4. **Lanjut / Selesai** — tombol "Tambah Halaman" kembali ke langkah 1 (kamera) untuk halaman berikutnya pada dokumen yang sama; tombol "Selesai" mengirim seluruh halaman yang terkumpul ke `POST /api/v1/activities/supporting-documents/scan` sebagai satu request multipart, lalu menutup komponen dan me-refresh daftar.
5. Setelah "Selesai", user bisa menekan "Scan dari Kamera" lagi untuk membuat dokumen PDF baru (sesi baru, state halaman di-reset) — ini yang membuat "bisa lebih dari 1 PDF" dan "tambah dokumen lagi" dari requirement.

Library edge-detection/perspective yang dipakai, cara load (CDN vs bundle npm, ukuran WASM OpenCV.js) ditentukan dan diverifikasi saat implementation plan ditulis — spec ini mengikat pada pendekatannya (OpenCV.js di client, lazy-loaded), bukan pada nama paket npm spesifik.

## 10. Error Handling

- Kamera tidak bisa diakses (izin ditolak) — pesan yang sama seperti `BarcodeScanner` sudah tangani (lihat perbaikan Permissions-Policy nginx yang sudah ada).
- File PDF upload langsung yang bukan PDF asli (ekstensi dipalsukan) — ditolak lewat `media.DetectUpload`, pesan `ErrMediaTypeInvalid` yang sudah ada.
- Scan dengan 0 halaman terkirim — ditolak di service, pesan baru `ErrScanPagesRequired`.
- Scan dengan halaman melebihi batas (30) — ditolak, pesan baru `ErrScanTooManyPages`.
- `slot_id` tidak ditemukan, milik Jadwal lain, atau sudah dinonaktifkan — ditolak, pesan baru `ErrSlotInvalid`.
- `schedule_id` tidak ditemukan atau di luar cakupan regency scope aktor — error `not_found`, konsisten dengan pola resource lain yang di-scope regency di seluruh sistem (mis. `CheckSerial`/`FindSerialMatches`): percobaan akses di luar cakupan selalu dibalas sebagai "tidak ditemukan", bukan "ditolak".
- Salah satu halaman bukan gambar asli (manipulasi request) — ditolak lewat `media.DetectUpload` per halaman.
- Kegagalan OpenCV.js saat load (CDN/WASM gagal diambil, jaringan lemah di lapangan) — komponen scan menampilkan pesan error dan tombol "Coba lagi", TIDAK fallback diam-diam ke mode tanpa deteksi (konsisten dengan prinsip fail-fast yang sudah dipakai untuk `STORAGE_BACKEND=gdrive` di `config.go`).

## 11. Testing

- `internal/media/upload_test.go`: `DetectUpload` mengenali PDF asli sebagai `KindDocument`; menolak file dengan ekstensi `.pdf` tapi isi bukan PDF.
- `internal/activities/service_test.go`: `Upload` menerima PDF untuk `dokumen_pendukung`, menolak image/video untuk `dokumen_pendukung`, menolak PDF untuk 9 `activity_type` lain (regresi — pastikan tidak bocor). `ScanUpload` menyusun N gambar jadi PDF N halaman; menolak 0 halaman; menolak >30 halaman; menolak `slot_id` yang bukan milik `schedule_id` yang dikirim; menolak slot yang `is_active=false`; `display_name` hasil upload memuat nama slot yang disanitasi.
- `internal/activities/repository_integration_test.go` atau test baru khusus: CRUD `supporting_document_slots` (buat, ubah nama, nonaktifkan); listing hanya mengembalikan slot `is_active` milik `schedule_id` yang diminta.
- Frontend: `DocumentScanner.test.tsx` — alur dasar (mock `getUserMedia` dan library CV) memverifikasi state halaman bertambah per capture, dan request akhir ke endpoint scan berisi seluruh halaman dalam urutan yang benar, termasuk `schedule_id`+`slot_id` yang dipilih user di dialog sebelumnya.
