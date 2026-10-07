# Rancangan Staging Media dan Tanggal di POS Dokumen

Versi: 0.1  
Tanggal: 2026-10-07  
Status: Menunggu review pengguna  
Menggantikan keputusan penyimpanan media dan kepemilikan tanggal pada `2026-10-07-distribution-pos-responsibility-and-revision-design.md`.

## 1. Latar Belakang

Alur lapangan memulai pekerjaan dari POS Mesin sebelum tanggal distribusi dan penerima akhir selalu tersedia. POS Mesin seharusnya dapat langsung mengambil foto atau video tanpa mengisi tanggal, data penerima, atau data peralatan. Tanggal baru ditentukan di POS Dokumen bersama proses mounting NIK ke nomor bagi.

Struktur Google Drive permanen tetap harus berbasis tanggal dan nomor bagi. Karena tanggal belum tersedia saat pengambilan media, media membutuhkan lokasi staging yang persisten. Staging tidak boleh menggunakan RAM atau disk lokal VPS karena file harus tetap aman melewati restart, deployment, gangguan aplikasi, dan jeda operasional beberapa hari.

## 2. Tujuan

- POS Mesin hanya membuat/membuka slot dan mengelola dokumentasi tahap `mesin`.
- POS Dokumen mengelola tanggal, penerima, peralatan, dan dokumentasi tahap `dokumen`.
- Media yang diambil sebelum tanggal/penerima tersedia disimpan aman di Google Drive staging.
- Setelah tanggal dan penerima tersedia, media dipindahkan otomatis ke folder permanen tanpa upload ulang.
- Pemindahan tahan restart, idempotent, dapat dicoba ulang, dan tidak menghasilkan duplikasi.
- Perubahan tanggal tetap didukung dengan memindahkan media ke folder tanggal terbaru.
- Tidak ada media yang dihapus otomatis.
- Alur revisi slot, penyelesaian ulang, snapshot, dan invalidasi BA tetap berlaku.

## 3. Pembagian Tanggung Jawab POS

### 3.1 POS Mesin

Saat petugas memilih nomor slot yang belum digunakan:

1. Sistem membuat slot dengan `distribution_date` kosong.
2. Petugas mengambil atau mengunggah dokumentasi tahap `mesin`.
3. Media disimpan ke folder staging Drive.

POS Mesin tidak menampilkan atau mengelola:

- tanggal distribusi;
- penerima/NIK;
- jenis atau opsi mesin, converter, dan selang;
- nomor seri mesin, converter, dan selang.

Upload dan penghapusan media mesin membutuhkan permission `distribution.pos_mesin`. Media staging tidak dihapus otomatis, termasuk ketika slot tidak dilanjutkan atau dibatalkan. Penghapusan hanya melalui aksi manual yang diaudit.

### 3.2 POS Dokumen

POS Dokumen mengelola:

- tanggal distribusi;
- pencarian, mounting, edit, dan pergantian penerima;
- jenis/opsi dan nomor seri mesin, converter, serta selang;
- dokumentasi tahap `dokumen`.

Tanggal dan penerima boleh diisi dalam urutan mana pun. Setelah keduanya tersedia, sistem mengantrekan pemindahan seluruh media slot menuju folder permanen. Data peralatan bukan syarat pemindahan media.

Mengubah tanggal setelah slot selesai tetap memerlukan **Buka revisi**. Jika media sudah berada di folder final, perubahan tanggal membuat pekerjaan pemindahan baru ke folder tanggal terbaru; pengguna tidak perlu menghapus dan mengunggah ulang media.

### 3.3 POS Penyerahan

POS Penyerahan mengelola dokumentasi tahap `penyerahan` dan penyelesaian slot. Slot hanya dapat diselesaikan ketika:

- tanggal tersedia;
- penerima sudah terkait;
- data peralatan wajib valid;
- dokumentasi wajib lintas tahap memenuhi jumlah minimum;
- seluruh media wajib sudah berada di lokasi final.

Media yang masih `moving` atau `move_failed` membuat penyelesaian ditolak dengan pesan yang menjelaskan status pemindahan.

## 4. Struktur Penyimpanan Google Drive

### 4.1 Folder staging

Media yang belum memiliki tanggal dan penerima disimpan pada pola:

`{JENIS PROGRAM}/{ZONA}/{KABUPATEN}/DOKUMENTASI (FOTO)/PENDISTRIBUSIAN/_PENDING/{JADWAL}/{NOMOR SLOT}`

Identitas jadwal pada path harus stabil dan bebas benturan. Implementasi boleh memakai kode tampilan yang disertai ID pendek atau ID jadwal penuh. Resolver folder tetap idempotent: folder yang sudah ada digunakan kembali.

### 4.2 Folder final

Setelah tanggal dan penerima tersedia, media dipindahkan ke:

`{JENIS PROGRAM}/{ZONA}/{KABUPATEN}/DOKUMENTASI (FOTO)/PENDISTRIBUSIAN/{TANGGAL}/{NOMOR BAGI}`

Nomor bagi berasal dari `distribution_slots.slot_number`. Folder tanggal dan nomor bagi yang sudah ada digunakan kembali. File dipindahkan melalui perubahan parent Google Drive, bukan diunduh dan diunggah ulang, sehingga ID file, checksum, dan URL konten tetap stabil.

### 4.3 Keselamatan pemindahan

- File tidak dihapus dari staging sebelum Google Drive mengonfirmasi operasi move.
- Jika move gagal, file tetap dapat dibuka dari lokasi lamanya.
- Penghapusan folder staging atau folder final yang kosong tidak dilakukan otomatis.
- Penghapusan media berdasarkan ID file Drive tetap bekerja tanpa bergantung pada folder saat ini.

## 5. State Media dan Antrean Pemindahan

Status konten media (`accepted`, `deleted`, dan status validasi lain yang sudah ada) tetap terpisah dari status lokasinya. Media aktif memiliki `storage_state`:

- `staging`: aman di folder `_PENDING` dan menunggu tanggal/penerima;
- `moving`: sedang diproses worker;
- `final`: berada di folder tanggal/nomor bagi terkini;
- `move_failed`: percobaan terakhir gagal dan akan dicoba ulang.

Database menyimpan antrean persisten untuk setiap media yang perlu dipindahkan. Pekerjaan memuat target folder, generasi target, jumlah percobaan, waktu percobaan berikutnya, dan pesan kesalahan terakhir.

Worker:

1. mengambil pekerjaan siap proses dengan database row locking;
2. memastikan folder tujuan tersedia secara idempotent;
3. memindahkan file Drive ke parent tujuan;
4. memastikan target belum berubah selama pekerjaan berlangsung;
5. menandai media `final` jika target masih sama;
6. mengantrekan ulang media bila tanggal berubah ketika move sedang berjalan;
7. menyimpan kegagalan dan retry berikutnya dengan exponential backoff terbatas.

Restart VPS tidak menghapus pekerjaan. Beberapa instance aplikasi memakai `FOR UPDATE SKIP LOCKED` atau mekanisme ekuivalen agar file yang sama tidak dikerjakan bersamaan.

## 6. Pemicu Pemindahan dan Perubahan Tanggal

Pemeriksaan kesiapan pemindahan dipanggil setelah:

- tanggal disimpan atau diubah;
- penerima pertama kali di-mount;
- penerima diganti;
- media baru selesai diunggah ketika tanggal dan penerima sudah tersedia;
- pengguna memilih **Coba pindahkan lagi**.

Jika tanggal atau penerima belum tersedia, media tetap `staging`. Jika keduanya tersedia, pekerjaan pemindahan di-upsert, bukan dibuat berulang kali.

Pergantian penerima tidak memindahkan folder karena path final menggunakan nomor bagi, bukan NIK. Perubahan tanggal menaikkan generasi target seluruh media slot. Worker yang menyelesaikan target lama wajib melihat generasi baru dan mempertahankan pekerjaan sampai file berada di folder terbaru.

## 7. API dan Permission

Matriks permission menjadi:

| Operasi | Permission wajib |
|---|---|
| Buat slot tanpa tanggal, media `mesin`, buka revisi dari POS Mesin | `distribution.pos_mesin` |
| Isi/ubah tanggal, kaitkan/edit/ganti penerima, ubah peralatan, media `dokumen`, buka revisi dari POS Dokumen | `distribution.pos_dokumen` |
| Media `penyerahan`, selesaikan slot, buka revisi dari POS Penyerahan | `distribution.pos_penyerahan` |

Perubahan kontrak minimum:

- `CreateSlotInput` hanya memuat jadwal dan optional nomor slot;
- `distribution_date` pada respons slot menjadi nullable;
- endpoint tanggal tetap terpisah tetapi otorisasinya berpindah ke POS Dokumen;
- respons media memuat `storage_state` dan pesan kegagalan opsional;
- endpoint retry manual hanya mengaktifkan kembali pekerjaan yang ada atau meng-upsert satu pekerjaan idempotent;
- endpoint complete menolak media wajib yang belum `final`;
- endpoint upload menentukan staging/final target berdasarkan keadaan slot saat transaksi upload dimulai.

Backend selalu memvalidasi permission berdasarkan stage dokumentasi; permission frontend bukan batas keamanan.

## 8. Perubahan Database dan Storage Interface

Migrasi baru setelah `00043`:

- membuat `distribution_slots.distribution_date` nullable;
- menambah `storage_state`, `storage_last_error`, dan metadata lokasi yang diperlukan pada `media_files`;
- membuat tabel antrean pemindahan media dengan unique key per media;
- menambah indeks untuk pekerjaan siap proses dan status media per slot;
- melakukan backfill media yang sudah ada sebagai `final`;
- tidak memindahkan atau menghapus file lama saat migrasi.

Abstraksi storage ditambah operasi move berdasarkan `storage_key` dan folder tujuan. Implementasi Google Drive mengambil parent file saat ini lalu memperbarui parent menuju folder tujuan. Implementasi local storage untuk development/test memindahkan file secara aman di dalam root media.

## 9. UX

POS Mesin menampilkan status lokasi media:

- **Tersimpan sementara**;
- **Menunggu tanggal dan penerima**;
- **Sedang dipindahkan**;
- **Tersimpan di folder final**;
- **Pemindahan gagal — akan dicoba kembali**.

POS Dokumen menampilkan tanggal di atas data penerima dan peralatan. Setelah tanggal dan penerima lengkap, UI menjelaskan bahwa dokumentasi mesin sedang ditata otomatis. Status gagal menyediakan tombol **Coba pindahkan lagi** dan pesan backend yang aman ditampilkan.

POS Penyerahan menonaktifkan penyelesaian ketika media wajib belum final dan menampilkan alasan spesifik, bukan sekadar tombol disabled.

Preview media tersedia untuk media staging, moving, final, dan move_failed selama status kontennya masih aktif.

## 10. Revisi, Snapshot, dan Berita Acara

Aturan revisi dari rancangan sebelumnya tetap berlaku:

- slot completed harus dibuka melalui **Buka revisi** dengan alasan;
- pembukaan revisi mengembalikan slot ke `linked`, menandai `needs_recompletion`, dan menginvalidasi snapshot final;
- BA perorangan, bundle harian, DP3, rekap harian, closing titik serah, dan closing kabupaten yang terdampak menjadi stale;
- dokumen stale tetap dapat diunduh;
- penyelesaian ulang membuat snapshot terbaru dan memungkinkan finalisasi BA versi baru.

Jika revisi mengubah tanggal, pekerjaan pemindahan media menggunakan tanggal terbaru. BA lama tetap berada sebagai riwayat dan tidak mengendalikan lokasi media.

## 11. Penanganan Kegagalan

- Kegagalan Drive tidak membatalkan penyimpanan tanggal atau mounting penerima.
- File tetap aman pada parent terakhir yang dikonfirmasi Drive.
- Worker menyimpan error dan retry otomatis.
- Tombol retry manual tidak membuat duplikasi pekerjaan.
- Jika storage tidak mendukung move atau kredensial kehilangan akses, slot tetap terbaca tetapi tidak dapat diselesaikan sampai masalah diperbaiki.
- Tidak ada cleanup berdasarkan umur file.

## 12. Strategi Pengujian

Backend dan migrasi harus membuktikan:

- slot dapat dibuat dan menerima media tanpa tanggal;
- media existing ter-backfill sebagai final;
- urutan tanggal-dahulu dan penerima-dahulu menghasilkan antrean yang sama;
- upload setelah tanggal+penerima langsung mengantrekan target final;
- restart worker mempertahankan pekerjaan;
- concurrency tidak memindahkan media dua kali;
- resolver folder staging/final memakai ulang folder;
- perubahan tanggal ketika job berjalan berakhir pada target terbaru;
- kegagalan Drive mempertahankan file dan menghasilkan retry;
- complete ditolak sampai seluruh media wajib final;
- permission lintas POS ditolak oleh API.

Frontend harus membuktikan:

- POS Mesin tidak memiliki field tanggal;
- POS Dokumen memiliki tanggal, penerima, dan peralatan;
- status staging/moving/final/failed terlihat;
- retry hanya tersedia sesuai permission;
- POS Penyerahan menjelaskan blokir pemindahan;
- alur revisi dan badge penyelesaian ulang tetap bekerja.

Verifikasi akhir mencakup seluruh tes Go, tes frontend, typecheck, build, migration order, dan smoke test staging.

## 13. Acceptance Criteria

1. POS Mesin dapat membuat slot dan mengunggah media tanpa tanggal.
2. Media staging tetap tersedia setelah restart atau deployment VPS.
3. Tidak ada penghapusan media otomatis.
4. Tanggal hanya dikelola POS Dokumen.
5. Tanggal dan mounting penerima dapat dilakukan dalam urutan mana pun.
6. Setelah keduanya lengkap, media otomatis menuju folder `{TANGGAL}/{NOMOR BAGI}`.
7. Folder staging dan final yang sudah ada digunakan kembali.
8. Pemindahan tidak mengunggah ulang atau menduplikasi file.
9. Perubahan tanggal memindahkan media ke folder tanggal terbaru.
10. Gangguan Drive mempertahankan file, status kegagalan, dan pekerjaan retry.
11. Slot tidak dapat diselesaikan sampai seluruh media wajib final.
12. Media tetap dapat dipreview selama belum dihapus.
13. Permission setiap POS diterapkan di backend.
14. Revisi, penyelesaian ulang, snapshot, dan lifecycle stale BA tetap berfungsi.

## 14. Di Luar Cakupan

- penghapusan otomatis media staging berdasarkan umur;
- penghapusan otomatis folder Drive kosong;
- penggunaan RAM atau disk lokal VPS sebagai penyimpanan staging produksi;
- perubahan bebas NIK tanpa proses pergantian penerima;
- migrasi file manual yang tidak tercatat di `media_files`;
- perubahan struktur folder BA.

## 15. Dampak terhadap Pekerjaan yang Sedang Berjalan

Implementation plan `2026-10-07-distribution-pos-revision.md` tidak boleh dilanjutkan apa adanya. Task backend revisi dan stale BA yang sudah selesai tetap dapat digunakan. Task frontend yang mengasumsikan tanggal berada di POS Mesin harus direvisi. Implementation plan baru harus terlebih dahulu mengaudit perubahan yang sudah ter-commit maupun masih staged agar tidak menggandakan atau menghilangkan pekerjaan yang valid.
