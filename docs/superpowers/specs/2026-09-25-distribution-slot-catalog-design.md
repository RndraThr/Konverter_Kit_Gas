# Rancangan Katalog Slot Distribusi, Kuota, Badge Kelengkapan, dan Scan Barcode

Versi: 0.1
Tanggal: 2026-09-25
Status: Draft hasil diskusi — belum disetujui untuk implementasi, belum ada task/plan turunan

## 1. Latar Belakang

Dokumen ini merangkum diskusi desain lanjutan untuk modul Pendistribusian (`frontend/src/features/distribution/`, `internal/distribution/`) setelah data seed program KONKIT-2026 (46 jadwal kabupaten) tersedia. Tujuannya supaya sesi/agent AI lain dapat melanjutkan tanpa mengulang diskusi dari awal.

Belum ada implementasi apa pun untuk poin-poin di dokumen ini. Ini murni hasil brainstorming percakapan, dicatat agar tidak hilang.

## 2. Kondisi Saat Ini (baseline, sudah berjalan)

- `distribution_slots` dibuat **ad-hoc**, satu per satu, lewat tombol "Buat Slot Mesin Baru" di `DistributionPage.tsx:72`. Tidak ada batas jumlah slot per jadwal; `slot_number` dialokasikan otomatis (nomor berikutnya) oleh `Repository.CreateSlot` (`internal/distribution/repository.go:181-225`).
- Alur kerja 3 POS (berbasis permission):
  1. **POS Mesin** (`distribution.pos_mesin`) — isi kode/serial mesin & selang saat membuat slot. Status slot: `open`.
  2. **POS Dokumen** (`distribution.pos_dokumen`) — cari kandidat penerima via NIK (sumber: hasil import DCP3 → `people`/`candidate_nominations`/`package_allocations`), kaitkan ke slot. Status: `linked`.
  3. **POS Penyerahan** (`distribution.pos_penyerahan`) — upload bukti dokumentasi, selesaikan. Status: `completed`.
- Setiap slot punya salinan (snapshot) daftar dokumen wajib dari `documentation_template_slots`, disalin ke `documentation_slots` saat slot dibuat (`repository.go:210-216`). Field `stage` pada tiap dokumen (`mesin`/`dokumen`/`penyerahan`) menentukan **di POS mana kotak upload itu muncul** — ini murni gating di frontend (`SlotMesinSection.tsx`, `SlotDokumenSection.tsx`, `SlotPenyerahanSection.tsx` masing-masing filter by `stage`), backend (`CompleteSlot`, `repository.go:501-513`) tidak peduli stage, hanya cek semua dokumen wajib (`is_required=true`) sudah ter-upload minimal `min_files`.
- **Catatan tersendiri (bukan scope dokumen ini, tapi relevan):** saat ini ke-4 slot dokumentasi default (`recipient_package`, `machine_serial`, `package_completeness`, `signed_bast`) untuk `DOK-PETANI`/`DOK-NELAYAN` semuanya berlabel `stage='penyerahan'` — kondisi ini dikonfirmasi sama persis di staging (bukan bug lokal), artinya POS Mesin & POS Dokumen saat ini tidak menampilkan kotak upload apa pun untuk program KONKIT-2026. Kalau mau dipisah ke POS yang sesuai, itu diatur manual lewat Program Setup → Template Dokumentasi (UI sudah ada, `TemplatesPanel.tsx`), tidak butuh perubahan kode.
- `program_schedules` sudah eksplisit didesain mendukung **lebih dari satu jadwal per kabupaten** (untuk fase/tahap berbeda) — lihat guard `NOT EXISTS` di migrasi `00014_seed_konkit_2026_schedules.sql`.

## 3. Tujuan Perubahan

Mengganti/melengkapi alur "buat slot satu-satu tanpa batas" dengan pengalaman **katalog angka** yang lebih terstruktur untuk petugas lapangan, plus dua kebutuhan tambahan yang berkaitan erat (badge kelengkapan, scan barcode).

## 4. Desain yang Diusulkan

### 4.1 Katalog Nomor Slot (Grid UI)

Alih-alih tombol "buat slot baru" yang selalu menambah nomor berikutnya, tampilan Pendistribusian per-jadwal menampilkan **grid angka berurutan** (1, 2, 3, ..., N). Klik satu angka membuka detail slot itu — tampilan 3-POS (mesin/dokumen/penyerahan) yang sudah ada sekarang, tidak berubah.

- Kalau slot untuk nomor itu belum ada → klik pertama kali memicu pembuatan slot dengan `slot_number` tersebut (mengisi form POS Mesin seperti sekarang).
- Kalau slot sudah ada → klik langsung membuka detail slot yang sudah berjalan (lanjut ke POS Dokumen/Penyerahan sesuai status).

### 4.2 Kuota per Jadwal

- Kuota (jumlah maksimal slot) diikat ke **`program_schedules`**, bukan ke kabupaten (`regencies`) langsung — karena satu kabupaten bisa punya beberapa jadwal lintas fase, dan kuota adalah properti per-fase, bukan properti tetap kabupaten.
- Kuota bersifat **opsional/nullable**. Kalau diisi → grid tampil sebagai kotak tetap 1..kuota (termasuk kotak kosong yang belum dibuat slot-nya). Kalau kosong → grid tetap berperilaku seperti sekarang, terbuka/tumbuh sesuai slot yang sudah dibuat, tanpa batas atas.
- Perubahan skema yang kemungkinan dibutuhkan: kolom baru (nama sementara `slot_quota integer NULL`) di `program_schedules`. Perlu migrasi baru (menyusul setelah `00014`).

### 4.3 Addendum / Penambahan Penerima di Tengah Jalan

Dua jalur, belum diputuskan mana yang jadi jalur utama (atau dua-duanya didukung):

1. **Koreksi kecil, jadwal/fase sama** — cukup naikkan angka kuota pada jadwal yang sama. Grid otomatis menambah kotak kosong baru di ujung, slot yang sudah selesai tidak terganggu.
2. **Addendum formal** (ada keputusan/berkas terpisah, tanggal berbeda) — buat **jadwal baru** untuk kabupaten yang sama (pola penamaan mengikuti yang sudah ada, mis. `"<KABUPATEN> - KONVERTER KIT 2026 - Tambahan"`), dengan kuota sendiri. Ini tidak butuh perubahan arsitektur — sistem sudah mendukung multi-jadwal per kabupaten.

Rekomendasi (belum final): opsi 2 sebagai jalur utama untuk addendum yang punya alasan/persetujuan formal (riwayat lebih jelas), opsi 1 untuk koreksi teknis biasa.

### 4.4 Badge Kelengkapan Dokumen

Setiap kotak angka di grid (§4.1) diberi indikator status visual, karena ini berkaitan langsung dengan bukti/evidence pekerjaan lapangan:

- Kosong (belum ada slot) — netral/abu-abu.
- Ada slot, status `open` — belum ada penerima terkait.
- Ada slot, status `linked`, dokumen wajib belum lengkap — perlu perhatian (mis. kuning).
- Ada slot, status `completed` (semua dokumen wajib lengkap) — selesai (mis. hijau).

Detail visual (warna persis, ikon vs warna, dsb.) belum diputuskan — ini keputusan desain UI, bukan keputusan data. Data yang dibutuhkan untuk menghitung badge sudah tersedia dari kombinasi `distribution_slots.status` + jumlah `documentation_slots` wajib yang sudah terisi (query serupa dengan yang dipakai `CompleteSlot`, `repository.go:501-513`, tapi untuk banyak slot sekaligus — kemungkinan perlu endpoint list baru, bukan query per-slot).

### 4.5 Scan Barcode untuk Serial Number

Tiga field serial number di alur distribusi (`machine_serial_number`, `hose_serial_number`, `converter_serial_number` — lihat `internal/distribution/models.go:57` dan `DistributionPage.tsx:88`) mendapat opsi tambahan "scan barcode" di samping input manual.

**Keputusan arsitektur: client-side (browser), tanpa microservice.**

Alasan:
- Decoding barcode dari kamera bisa 100% jalan di browser (Web API `BarcodeDetector`, dengan fallback library JS untuk browser yang belum dukung native) — hasil scan langsung mengisi field, tanpa round-trip ke server.
- Field officer sering bekerja di lokasi dengan koneksi tidak stabil; scanning yang bergantung server menambah titik gagal yang tidak perlu.
- Microservice baru relevan kalau nanti ada kebutuhan lain: decode dari foto yang diunggah belakangan (bukan scan langsung), atau dipakai lintas banyak jenis klien — belum ada kebutuhan itu sekarang.

Belum diputuskan: library fallback spesifik (kandidat: `@zxing/browser` atau `quagga2`), format barcode yang perlu didukung (Code128? QR? tergantung barcode fisik yang tertempel di unit mesin/converter — perlu dicek contoh fisiknya), dan dampak ukuran bundle (CI sudah punya peringatan bundle >500kB, saat ini index.js ~746kB).

## 5. Cakupan Perubahan (perkiraan awal, belum plan resmi)

- Backend: migrasi baru untuk `program_schedules.slot_quota`; kemungkinan endpoint baru untuk list status slot 1..kuota sekaligus (untuk render grid + badge tanpa N+1 request).
- Frontend: komponen grid/katalog baru di `frontend/src/features/distribution/`; komponen scan barcode (kemungkinan modal/overlay kamera) dipasang di 3 titik input serial number; state/UX badge per kotak.
- Tidak ada perubahan pada logika inti POS Mesin/Dokumen/Penyerahan yang sudah ada — grid ini murni lapisan navigasi baru di atasnya.

## 6. Di Luar Cakupan (untuk sekarang)

- Perbaikan `stage` dokumen KONKIT-2026 (§2, catatan tersendiri) — itu masalah data/konfigurasi terpisah, diatur lewat UI, tidak butuh perubahan desain di sini.
- Microservice barcode — sudah diputuskan tidak diperlukan (§4.5).
- Mode offline/PWA untuk grid ini — belum dibahas, mengikuti pola umum dukungan offline yang sudah direncanakan di modul lain (lihat `2026-09-03-dcp3-distribution-documentation-design.md` §14) kalau relevan nanti.

## 7. Pertanyaan Terbuka

- Apakah kuota bersifat **enforced** (tidak bisa buat slot melebihi kuota) atau cuma **display cap** (grid tampil sampai kuota, tapi tetap bisa override/tambah manual)?
- Jalur addendum mana yang jadi default di UI — naikkan kuota, buat jadwal baru, atau dua-duanya ditawarkan sebagai pilihan saat petugas mencoba menambah di luar kuota?
- Format/warna badge yang dipakai — ikuti pola warna yang sudah ada di aplikasi (lihat label ringkas hijau/kuning/merah/biru/abu-abu di `2026-09-03-dcp3-distribution-documentation-design.md` §9) atau bikin skema baru khusus grid ini?
- Library barcode fallback yang dipakai, dan format barcode fisik yang perlu didukung (perlu sample barcode dari unit mesin/converter nyata).

## 8. Langkah Berikutnya

Dokumen ini belum disetujui untuk implementasi. Sebelum masuk ke implementation plan:

1. Jawab pertanyaan di §7 (minimal: enforced vs display-cap untuk kuota, jalur addendum default).
2. Kalau sudah, lanjut ke `superpowers:writing-plans` untuk memecah ini jadi task-task konkret (migrasi skema, endpoint list slot, komponen grid, komponen scan barcode) — kemungkinan besar dua plan terpisah (katalog+kuota+badge sebagai satu paket, scan barcode sebagai paket lain) karena keduanya independen satu sama lain.
