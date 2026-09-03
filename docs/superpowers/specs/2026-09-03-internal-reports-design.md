# Rancangan Modul Laporan Internal

Versi: 0.1
Tanggal: 2026-09-03
Status: Disetujui untuk implementasi

## 1. Tujuan

Menyediakan halaman laporan internal per jadwal kabupaten agar tim dapat memantau progres alokasi paket, distribusi, dan kelengkapan dokumentasi tanpa membuka satu per satu data penerima di halaman Pendistribusian. Laporan ini bersifat baca saja (read-only) dan tidak menambah entitas data baru; seluruh data ditarik dari domain `programs`, `dcp3`, dan `distribution` yang sudah ada.

## 2. Prinsip Utama

- Laporan di-scope ke satu `schedule_id` pada satu waktu; tidak ada rekap gabungan lintas kabupaten/jadwal pada versi ini, karena tiap kabupaten berjalan pada jadwal yang berbeda dan datanya tidak saling tercampur secara operasional.
- Laporan tidak memiliki tabel database sendiri. Modul `internal/reports` murni melakukan query agregasi/listing terhadap tabel domain lain.
- Filter yang diterapkan di layar (status alokasi, status distribusi, status dokumentasi) berlaku konsisten ke ringkasan angka, tabel, export Excel, dan export PDF — apa yang dilihat harus sama dengan apa yang diexport.
- Berbeda dari halaman Pendistribusian, laporan ini menampilkan **NIK penuh** (tidak disamarkan) karena sifatnya laporan internal/arsip untuk pihak yang sudah berwenang melihat data operasional (dilindungi permission `distribution.view` yang sama seperti halaman Pendistribusian).
- Tidak ada permission baru. Akses laporan mengikuti siapa saja yang sudah memiliki `distribution.view`.
- Setiap export (Excel maupun PDF) dicatat di audit log — metadata akses (aktor, waktu, jadwal, filter), bukan isi datanya — mengikuti prinsip audit yang sudah dipakai di modul lain untuk akses data sensitif.

## 3. Data Sumber

Query laporan menggabungkan:

- `package_allocations` — status alokasi, nomor pembagian, `schedule_id`, `intended_person_id`, `actual_recipient_person_id`.
- `people` — nama, NIK, desa, kecamatan (memakai `actual_recipient_person_id` bila terisi, jika tidak memakai `intended_person_id`, sebagai representasi "penerima saat ini").
- `person_sector_identifiers` — nomor Kartu Petani atau KUSUKA sesuai `program_type` jadwal.
- `distribution_records` — status distribusi (`draft`/`completed`/`cancelled`) dan `completed_at`.
- `documentation_slots` — dipakai untuk menghitung kelengkapan dokumentasi (semua slot wajib berstatus `complete` atau belum).

Tidak ada join ke `bast_documents` atau tabel BAST lain karena modul tersebut belum ada.

## 4. Ringkasan Angka

Ditampilkan sebagai kartu di atas tabel, dihitung dari hasil query yang sudah difilter jadwal (dan filter tambahan bila dipilih user):

- Total alokasi pada jadwal.
- Jumlah per status alokasi (`candidate`, `ready`, `needs_review`, `distributed`, `replaced`, `cancelled`).
- Jumlah per status distribusi (`draft`, `completed`, `cancelled`).
- Jumlah alokasi dengan dokumentasi wajib belum lengkap.

## 5. Tabel Data

Kolom: nomor pembagian, nama, NIK, nomor Kartu Petani/KUSUKA, desa/kecamatan, status alokasi, status distribusi, status dokumentasi (lengkap/belum), tanggal distribusi selesai.

Filter tersedia: status alokasi, status distribusi, status dokumentasi. Filter dikirim sebagai query parameter dan diterapkan di backend (bukan filter client-side), supaya hasil export selalu konsisten dengan hasil yang sedang dilihat.

## 6. API

```text
GET /api/v1/reports/schedule/{schedule_id}/summary
GET /api/v1/reports/schedule/{schedule_id}/rows?allocation_status=&distribution_status=&documentation_status=
GET /api/v1/reports/schedule/{schedule_id}/export.xlsx?allocation_status=&distribution_status=&documentation_status=
GET /api/v1/reports/schedule/{schedule_id}/export.pdf?allocation_status=&distribution_status=&documentation_status=
```

Seluruh endpoint memerlukan permission `distribution.view`. Endpoint export menulis entri audit `reports.exported` berisi aktor, `schedule_id`, format, dan filter yang dipakai — tanpa menyertakan data penerima.

## 7. Export

- **Excel**: memakai `excelize` (sudah menjadi dependency proyek untuk import DCP3), satu sheet berisi tabel sesuai kolom pada bagian 5.
- **PDF**: memakai library Go murni yang ringan (kandidat: `go-pdf/fpdf`) untuk merender kartu ringkasan dan tabel dalam layout sederhana. Tidak memakai render HTML-ke-PDF berbasis headless browser, karena kebutuhan saat ini hanya ringkasan bertabel, bukan dokumen dengan tata letak kompleks seperti BAST. Jika kelak dibutuhkan PDF dengan layout kaya (mis. BAST), itu ditangani modul BAST Generator terpisah, bukan modul ini.

## 8. Frontend

- Halaman baru: `frontend/src/features/reports/ReportsPage.tsx`, route `/laporan`, diproteksi permission `distribution.view`, ditempatkan pada grup navigasi yang sama dengan Pendistribusian.
- Alur: pilih jadwal → kartu ringkasan → kontrol filter status → tabel hasil → tombol "Export Excel" dan "Export PDF" yang memicu unduhan sesuai filter aktif.
- Tidak menyimpan keputusan otoritatif apa pun di client; halaman ini murni tampilan dan pemicu export.

## 9. Keamanan dan Audit

- NIK penuh ditampilkan di layar dan file export; halaman tetap memerlukan `distribution.view`, permission yang sama dengan yang sudah membatasi akses ke identitas lengkap di Pendistribusian.
- Aksi export (Excel/PDF) dicatat di audit log sebagai `reports.exported` dengan metadata aktor, waktu, jadwal, dan filter — bukan isi baris data.
- Endpoint mengikuti pola otorisasi backend yang sudah baku di proyek ini: pemeriksaan permission dilakukan di backend untuk setiap request, bukan hanya di frontend.

## 10. Di Luar Cakupan

- Rekap gabungan lintas kabupaten/jadwal.
- Kolom status BAST atau status pemasangan terstruktur (belum ada modul/data sumbernya).
- Pengaturan template/format laporan yang bisa dikustomisasi Super Admin — layout Excel/PDF versi awal ini tetap (fixed), belum ada editor template.
- Riwayat/arsip file laporan yang pernah diexport (setiap export dibuat sesuai permintaan, tidak disimpan sebagai berkas persisten).
