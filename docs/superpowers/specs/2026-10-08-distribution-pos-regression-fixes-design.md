# Perbaikan Regresi POS Distribusi: Gate, Scan Barcode, dan Nama Berkas Final

Versi: 1.0
Tanggal: 2026-10-08
Status: Disetujui pengguna untuk implementasi
Terkait: `2026-10-07-staged-distribution-media-design.md`, `2026-09-25-distribution-slot-catalog-design.md`

## 1. Latar Belakang

Setelah alur staging media (`2026-10-07-staged-distribution-media-design.md`) diimplementasikan, ditemukan tiga masalah pada uji lapangan:

1. Petugas tidak dapat menyimpan tanggal distribusi sehingga media staging tidak pernah berpindah ke folder utama.
2. Tombol scan barcode pada nomor seri hilang dari antarmuka POS.
3. Nama berkas di Google Drive tidak mengikuti data penerima setelah mounting NIK.

## 2. Temuan dan Keputusan

### 2.1 Gate "dokumentasi mesin wajib lengkap" dicabut

Sebuah perubahan yang belum ter-commit menambahkan `ErrMachineDocumentationIncomplete` dan memanggil `requireMachineDocumentationComplete` pada sembilan jalur mutasi: `SetDistributionDate`, `UpdateEquipment`, `LinkSlot`, `UpdateRecipient`, `ReplaceRecipient`, `CompleteSlot`, `SaveMedia`, `DeleteMedia`, dan `RetryMediaMove`.

Aturan itu **bertentangan** dengan `2026-10-07-staged-distribution-media-design.md` §3.2: *"Tanggal dan penerima boleh diisi dalam urutan mana pun. Setelah keduanya tersedia, sistem mengantrekan pemindahan seluruh media slot menuju folder permanen. Data peralatan bukan syarat pemindahan media."* Rancangan hanya menaruh syarat dokumentasi pada penyelesaian slot (§3.3).

Syarat itu juga **redundan**: `CompleteSlot` sudah menolak penyelesaian ketika dokumen wajib kurang (`ErrDocumentationIncomplete`), media wajib belum final (`ErrMediaMovePending`), atau pemindahan gagal (`ErrMediaMoveFailed`).

**Keputusan:** gate dicabut sepenuhnya. Tanggal, penerima, peralatan, dan media kembali bebas sesuai rancangan staging; pembatasan hanya terjadi di `CompleteSlot` seperti sebelumnya.

### 2.2 Scan barcode dikembalikan di POS Dokumen

`2026-09-25-distribution-slot-catalog-design.md` §4.5 mewajibkan opsi scan barcode pada tiga field nomor seri (`machine_serial_number`, `hose_serial_number`, `converter_serial_number`).

Riwayat: POS Mesin dahulu memiliki tombol `Scan <label>` dengan `BarcodeScanner` lazy-loaded. Task "make machine POS documentation only" menghapus tanggal, peralatan, dan nomor seri dari POS Mesin **beserta scanner**, lalu task "move distribution date to document POS" memindahkan ketiga field nomor seri ke POS Dokumen **tanpa** membawa scanner. Akibatnya `BarcodeScanner.tsx` menjadi kode mati dan syarat §4.5 tidak lagi terpenuhi di mana pun.

**Keputusan:** tombol scan dipasang pada ketiga field nomor seri di POS Dokumen, memakai komponen `BarcodeScanner` yang sudah ada, hanya aktif ketika field dapat diedit. Pada POS Mesin tidak dipasang karena POS Mesin tidak lagi memiliki field nomor seri.

### 2.3 Nama berkas final menyertakan nama penerima

Saat ini nama berkas hanya dibentuk sekali pada waktu upload: `{LABEL DOKUMENTASI} - {NN}.ext` (`formatDistributionMediaFilename`). Pemindahan ke folder final hanya mengubah parent Google Drive; nama berkas tidak pernah disesuaikan, sehingga berkas di folder final tidak memuat identitas penerima.

**Keputusan:** nama berkas disesuaikan bersamaan dengan pemindahan ke folder final, bukan pada waktu upload dan bukan pada waktu mounting. Pola nama final:

```text
{NAMA PENERIMA} - {LABEL DOKUMENTASI} - {NN}.ext
```

Contoh: `AHMAD - FOTO MESIN - 01.jpg`

Ketentuan:

- Nama penerima dan label dokumentasi dinormalisasi dengan aturan business-uppercase yang sudah dipakai (`textnorm.BusinessUpper`).
- `NN` adalah urutan berkas diterima di dalam satu slot dokumentasi, dihitung dari `uploaded_at, id` agar stabil dan bebas benturan.
- Jika `max_files` slot dokumentasi bernilai satu, segmen urutan dihilangkan: `{NAMA PENERIMA} - {LABEL}.ext`.
- Nama penerima yang kosong membuat segmen nama dihilangkan, sehingga nama kembali ke `{LABEL} - {NN}.ext`.
- Nama tidak memuat NIK.
- Perubahan tanggal atau pergantian penerima menaikkan generasi target, sehingga nama dihitung ulang pada pemindahan berikutnya.

## 3. Perubahan Teknis

### 3.1 Backend storage

`media.MovableStorage.Move` menerima nama berkas tujuan:

```go
Move(ctx context.Context, storageKey string, targetPath []string, targetFilename string) error
```

- `GoogleDriveStorage.Move` menyelesaikan folder tujuan, membaca nama dan parent berkas saat ini, lalu mengirim satu operasi `Files.Update` berisi nama baru dan parent baru. Operasi ini idempoten: bila parent sudah sesuai dan nama sudah sama, tidak ada panggilan API tambahan.
- `LocalStorage.Move` tetap hanya memvalidasi (folder dan nama) dan mempertahankan `storage_key` stabil, karena penyimpanan lokal bersifat flat dan hanya dipakai untuk pengembangan/pengujian.

### 3.2 Backend distribusi

- Tabel `distribution_media_move_jobs` mendapat kolom `target_filename`.
- `queueMediaMove` menghitung nama final untuk setiap berkas ketika target path disusun, lalu menyimpannya bersama `target_path` dan `target_generation`.
- `ClaimMediaMove` mengembalikan `TargetFilename` sebagai bagian dari `MediaMoveJob`.
- Worker meneruskan `job.TargetFilename` ke `Move`.
- `CompleteMediaMove` menyimpan `target_filename` ke `media_files.original_filename` agar tampilan aplikasi dan nama di Google Drive tetap sama.

### 3.3 Frontend

- `SlotDokumenSection` memakai kembali `BarcodeScanner` (lazy, dalam `Suspense`) dan menampilkan tombol `Scan <label>` pada Serial Number Mesin, Serial Number Konkit/Reducer, dan Serial Number Selang ketika `editable`.
- Ketiga field nomor seri dirender sebagai `Label` + `Input` + `Button` di dalam grid yang sama, sehingga `getByLabelText` tetap menemukan field.
- Hasil scan mengisi field terkait dan dinormalisasi business-uppercase, lalu dialog scan ditutup.

## 4. Kriteria Penerimaan

1. Menyimpan tanggal distribusi dan mounting penerima berhasil tanpa menunggu dokumentasi mesin lengkap.
2. Begitu tanggal dan penerima tersedia, seluruh media slot yang masih `staging` masuk antrean pemindahan dan berpindah ke folder `{TANGGAL}/{NOMOR BAGI}`.
3. Penyelesaian slot tetap ditolak sampai dokumentasi wajib memenuhi `min_files` dan seluruh media wajib `final`.
4. Ketiga field nomor seri di POS Dokumen menyediakan tombol scan barcode ketika dapat diedit, dan hasil scan mengisi field yang benar.
5. Berkas di folder final bernama `{NAMA PENERIMA} - {LABEL} - {NN}.ext`.
6. Pemindahan tetap idempoten: mengulang pemindahan yang sudah berhasil tidak memindahkan atau menggandakan berkas.
7. Seluruh tes Go, tes frontend, dan typecheck lulus.

## 5. Di Luar Cakupan

- Perubahan pola nama berkas pada tahap staging (tetap `{LABEL} - {NN}.ext`).
- Penambahan NIK pada nama berkas atau path Google Drive.
- Perubahan struktur folder final.
