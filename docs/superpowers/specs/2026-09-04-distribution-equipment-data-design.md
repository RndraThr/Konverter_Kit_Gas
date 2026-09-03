# Rancangan Data Peralatan Distribusi (Fondasi BAST)

Versi: 0.1
Tanggal: 2026-09-04
Status: Disetujui untuk implementasi

## 1. Tujuan

Melengkapi data yang dibutuhkan dokumen Berita Acara Serah Terima (BAST) per penerima — merk/tipe mesin, merk/spesifikasi selang, serial number mesin/selang/konkit-reducer, dan nama Konsultan Pengawas — tanpa membangun modul BAST Generator itu sendiri. Tahap ini murni menyiapkan fondasi data; render dokumen, penomoran, dan cetak PDF ditangani modul terpisah setelah ini.

Rujukan: contoh dokumen BAST perorangan (Pengadaan Konverter Kit LPG untuk Mesin Pompa Air, program Pertamina Patra Niaga/PT Kian Santang Mulitama Tbk) menunjukkan field mana yang tipikal per jadwal (masuk settings) dan mana yang spesifik per penerima.

## 2. Prinsip Utama

- Merk/Tipe Mesin dan Merk/Spesifikasi Selang adalah nilai tipikal per jadwal, tapi kabupaten berbeda mungkin pakai barang berbeda — jadi jadwal menyimpan **daftar opsi** (bukan satu nilai tetap), dan petugas memilih opsi yang sesuai saat distribusi.
- Merk Konkit/Reducer tetap satu nilai tetap per jadwal (`converter_brand`, sudah ada), tidak perlu banyak opsi.
- Serial Number (mesin, selang, konkit/reducer) adalah data spesifik per penerima, diketik petugas di halaman Pendistribusian yang sama dengan slot foto dokumentasi — bukan halaman terpisah.
- Konsultan Pengawas adalah pihak eksternal yang bisa berbeda per kabupaten, jadi disimpan sebagai field opsional di level jadwal.
- Pelaksana Pemasangan tidak perlu field atau role baru — diambil dari `distribution_records.distributed_by` (user yang menyelesaikan distribusi), karena tim pelaksana lapangan adalah tim internal yang sama dengan yang mengoperasikan sistem.
- Nilai yang sudah dipilih/diisi petugas ikut ter-snapshot ke `verification_snapshot_json` saat distribusi diselesaikan, mengikuti prinsip snapshot yang sudah dipakai untuk identitas dan paket — supaya perubahan template di jadwal berikutnya tidak mengubah data yang sudah final.
- Field-field ini belum diwajibkan untuk penyelesaian distribusi pada tahap ini; aturan wajib/opsional akan ditentukan bersamaan dengan modul BAST Generator.

## 3. Perluasan Template Paket

`package_template_versions.values_json` (kolom `jsonb` yang sudah ada, tidak perlu migration) mendapat dua kunci baru:

- `machine_options`: array objek `{brand, type}` — minimal satu entri wajib saat template dipublikasikan.
- `hose_options`: array objek `{brand, spec}` — minimal satu entri wajib saat template dipublikasikan.

`converter_brand` dan `components` tetap seperti sekarang. Validasi minimal-satu-opsi ditegakkan di lapisan service `programs`, konsisten dengan validasi lain yang sudah ada di sana (kode dokumen tiga huruf, tanggal jadwal, dst).

## 4. Perluasan Jadwal

Tabel `program_schedules` mendapat satu kolom baru:

- `supervisor_name text` — nama Konsultan Pengawas, opsional, diisi sekali per jadwal saat setup, berlaku untuk semua BAST pada jadwal itu.

## 5. Perluasan Data Distribusi

Tabel `distribution_records` mendapat lima kolom baru, semuanya opsional dan dapat diedit selama distribusi masih berstatus draft (pola yang sama dengan field identitas penerima yang sudah ada):

- `machine_option_code text` — kode opsi mesin yang dipilih dari `machine_options` template jadwal.
- `machine_serial_number text`
- `hose_option_code text` — kode opsi selang yang dipilih dari `hose_options` template jadwal.
- `hose_serial_number text`
- `converter_serial_number text`

Perubahan field ini memakai alur draft yang sama dengan field alamat/NIK yang sudah ada di `distribution.DraftInput`/`SaveDraft` — tidak perlu endpoint baru, cukup memperluas input dan query yang sudah ada. Saat distribusi diselesaikan (`Complete`), nilai-nilai ini disalin ke dalam `verification_snapshot_json` bersama snapshot identitas dan paket yang sudah ada.

Pelaksana Pemasangan tidak mendapat kolom baru — tetap dibaca dari `distribution_records.distributed_by` yang sudah ada.

## 6. Frontend

- **Panel Template** (Persiapan Program): tambah input berulang (tambah/hapus baris) untuk opsi merk/tipe mesin dan opsi merk/spesifikasi selang, di samping daftar komponen yang sudah ada.
- **Panel Jadwal** (Persiapan Program): tambah field teks opsional "Konsultan Pengawas".
- **Halaman Pendistribusian** (`RecipientWorkspace`): tambah bagian "Data Peralatan" berisi dropdown Merk/Tipe Mesin (dari opsi template jadwal aktif), input Serial Number Mesin; dropdown Merk/Spesifikasi Selang, input Serial Number Selang; input Serial Number Konkit/Reducer. Disimpan lewat tombol simpan draft yang sudah ada.

## 7. Struktur Data Konseptual (perubahan)

- `package_template_versions.values_json` — tambah `machine_options[]`, `hose_options[]` (tanpa migration, perubahan bentuk data di service layer).
- `program_schedules` — tambah `supervisor_name`.
- `distribution_records` — tambah `machine_option_code`, `machine_serial_number`, `hose_option_code`, `hose_serial_number`, `converter_serial_number`.

## 8. Keamanan dan Audit

Perubahan field ini masuk ke jalur audit yang sama dengan `distribution.draft_updated` yang sudah ada (tidak perlu event audit baru); tidak ada data sensitif baru (serial number dan nama pengawas bukan data pribadi terproteksi seperti NIK).

## 9. Di Luar Cakupan

- Nomor BAST, template dokumen, render/preview, dan export PDF — modul BAST Generator terpisah.
- Menjadikan field peralatan ini wajib sebelum distribusi bisa diselesaikan.
- Role/permission baru untuk Pelaksana Pemasangan — tetap memakai `distribution.manage` yang sudah ada.
- Override Pelaksana Pemasangan secara manual (di luar `distributed_by`) — belum dibutuhkan karena tim pelaksana dan tim input data adalah tim yang sama.
