# Dashboard Grafik Super Admin — Rancangan

Tanggal: 22 November 2026. Backlog item 1.

## Maksud

Dashboard ini dipakai untuk monitoring. Tim pusat (super admin) melihat progres pekerjaan satu program dalam bentuk grafik, lalu cepat tahu zona dan kabupaten mana yang tertinggal. Pemilik proyek minta kabupaten **dibagi per zona** supaya jelas.

## Tujuan dan batasan

- **Tujuan**
  - kartu ringkas per program;
  - progres per zona;
  - progres per kabupaten yang dikelompokkan per zona;
  - penyerahan per hari;
  - foto dokumentasi kegiatan per jenis.
- **Bukan tujuan**
  - menggantikan Monitoring live (review foto dan penugasan tetap di sana);
  - filter tanggal;
  - export.
- **Anti-tujuan**
  - angka yang berbeda dengan Monitoring live atau Laporan DP3 untuk hal yang sama;
  - halaman yang lambat karena query per kabupaten.

## Keputusan pemilik proyek

- Semua grafik pada sketsa disetujui: kartu, zona, kabupaten, harian, dan dokumentasi kegiatan.
- Library grafik: **Recharts**, dimuat lazy hanya di halaman dashboard.
- Hanya super admin yang bisa melihat.
- Pengelompokan per zona.

## Pendekatan yang dipertimbangkan

1. **Recharts (dipilih).** Tooltip, legend, dan ukuran responsif sudah tersedia. Ukurannya sekitar 100 KB, ditaruh di chunk terpisah.
2. **SVG/CSS buatan sendiri.** Tanpa dependensi, tapi interaksinya terbatas dan lebih banyak kode yang harus dirawat.

## Isi halaman

```
[Program ▾]
┌ Kuota ┐ ┌ Tertaut ┐ ┌ Diserahkan ┐ ┌ Foto disetujui ┐ ┌ BA final ┐   (angka + % dari kuota)
Progres per zona          bar horizontal bertumpuk per zona
Per kabupaten             bar per kabupaten, dikelompokkan per zona (judul zona), urut sesuai Monitoring
Penyerahan per hari       bar harian + garis kumulatif, garis acuan = kuota
Dokumentasi kegiatan      bar horizontal: jumlah foto/video aktif per jenis kegiatan
```

Klik kabupaten membuka `/monitoring-distribusi` (keputusan saya, karena halaman itu sudah menjadi tempat tindak lanjut).

## Definisi angka

Semua angka dihitung **per program**, untuk jadwal dengan `status <> 'cancelled'`, mengikuti scope kabupaten.

| Angka | Sumber |
|---|---|
| Kuota | `sum(program_schedules.slot_quota)` |
| Tertaut | `distribution_slots` dengan `allocation_id IS NOT NULL` dan `status IN ('linked','completed')` |
| Diserahkan | `distribution_slots.status = 'completed'` (sama dengan `Completed` di Monitoring) |
| Foto disetujui | slot completed yang semua fotonya approved (sama dengan `Approved` di Monitoring, memakai ulang CTE `photoCounts`) |
| BA final | slot completed yang punya `bast_individual_documents` dengan `status = 'final'` |
| Per hari | `count` slot completed per `(completed_at AT TIME ZONE 'Asia/Jakarta')::date`, kumulatif dihitung di frontend |
| Dokumentasi | `activity_media` dengan `program_id = $1`, `status = 'active'`, regency di scope, `group by activity_type` |

Progres per tahap pada grafik zona dan kabupaten adalah **tertaut → diserahkan → foto disetujui → BA final**, dibandingkan dengan kuota.

## Backend

- Paket baru `internal/dashboard` berisi `Service.Summary(ctx, programID, scope)`.
  - Program default: program aktif terbaru, sama dengan Monitoring.
  - Satu query agregat per bagian: per kabupaten, per hari, dan dokumentasi. Total dan zona dijumlahkan di Go dari baris kabupaten.
  - Tidak ada query per kabupaten.
- Endpoint `GET /api/v1/dashboard/summary?program_id=`.
  - Response: `programs`, `program_id`, `totals`, `zones[]`, `regencies[]` (dengan `zone_name`, `zone_sort`), `daily[]`, `activities[]`.
  - Program tidak ditemukan menghasilkan 404.
- Migrasi `00111_dashboard_permission.sql` membuat permission `dashboard.view` dan memberikannya ke `super_admin`.
- Integration test dengan fixture kecil: dua kabupaten di dua zona, slot open/linked/completed, foto approved, satu BA final, dan activity media. Test memeriksa tiap angka serta scope kabupaten.

## Frontend

- `frontend/src/features/super-dashboard/` berisi `SuperDashboardPage.tsx`, `api.ts`, `types.ts`, dan komponen grafik.
- Rute `/dashboard` dengan `ProtectedPage permission="dashboard.view"`. Menu "Dashboard" menjadi item pertama di grup "Ringkasan".
- Halaman di-load secara lazy (`React.lazy`) supaya Recharts tidak masuk bundle utama.
- State loading, kosong (program belum punya jadwal), dan error. Di lebar ponsel grafik ditumpuk satu kolom, dan grafik kabupaten bisa di-scroll vertikal.
- Warna dan gaya mengikuti `design-system/konkit/MASTER.md`. Satu warna per tahap dipakai konsisten di semua grafik.
- Vitest: render angka dari data tiruan, pengelompokan zona, state kosong dan error, serta menu dan rute tersembunyi tanpa `dashboard.view`.
- Panduan: topik baru atau tambahan di `frontend/src/features/guide/topics/`.

## Diserahkan ke builder

Detail visual (ukuran, tooltip, format angka `id-ID`), nama komponen, dan struktur file test.

## Selesai bila

Grafik sesuai sketsa, angka cocok dengan Monitoring untuk Diserahkan dan Foto disetujui, hanya super admin yang bisa melihat, dan `go vet`, `go test -p 1 ./...`, `vitest`, `typecheck`, serta `eslint` semuanya lulus.
