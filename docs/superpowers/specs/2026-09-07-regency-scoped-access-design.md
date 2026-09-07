# Rancangan Akses Data Berbasis Kabupaten per Role

Versi: 0.1
Tanggal: 2026-09-07
Status: Disetujui untuk implementasi

## 1. Tujuan

Mencegah petugas lapangan mengakses data operasional milik kabupaten lain — baik karena tidak sengaja (salah pilih di dropdown) maupun karena mengakali sistem lewat request langsung ke API. Pembatasan ini dibangun sebagai fondasi akses lintas modul, dipakai bersama oleh Persiapan Program, DCP3, Pendistribusian, Laporan, dan modul dokumentasi/Berita Acara yang akan dibangun setelah ini — bukan diimplementasikan ulang per modul.

## 2. Prinsip Utama

- Pembatasan kabupaten melekat ke **role**, bukan ke user secara langsung. Satu user mendapat gabungan cakupan kabupaten dari seluruh role yang dia punya.
- Satu role bisa dikaitkan ke **lebih dari satu kabupaten** (many-to-many).
- Role dapat ditandai **"akses semua kabupaten"** (`all_regencies_access`) sebagai alternatif dari daftar kabupaten eksplisit — untuk kasus seperti peran pemantau lintas kabupaten yang bukan Super Admin.
- Super Admin tetap bypass total seperti sekarang (tidak perlu entri di tabel role-kabupaten).
- Role baru yang belum diatur defaultnya **tidak punya akses ke kabupaten mana pun** (fail-safe). Karena sistem masih development dan belum ada data produksi, tidak ada strategi migrasi/backfill untuk role lama.
- Validasi dilakukan **di backend**, bukan hanya menyembunyikan pilihan di frontend. Dropdown yang terfilter adalah lapisan kenyamanan (mencegah salah klik), validasi backend adalah lapisan keamanan (mencegah request yang dimanipulasi langsung).

## 3. Struktur Data

- Tabel baru `role_regencies`: kolom `role_id` (FK ke `roles`), `regency_id` (FK ke `regencies`), unique pada pasangan keduanya.
- Kolom baru `roles.all_regencies_access boolean not null default false`.

## 4. Resolusi Cakupan Akses

Ditambahkan mekanisme baru di modul `auth`, sejajar dengan resolusi permission (`Can`, `Permissions`) yang sudah ada:

```go
type RegencyScope struct {
    Unrestricted bool
    RegencyIDs   []string
}

func (s *Service) RegencyScope(ctx context.Context, principal Principal) (RegencyScope, error)
```

Logika resolusi:
1. Jika `principal.IsSuperAdmin()` → `Unrestricted: true`.
2. Jika ada role milik user dengan `all_regencies_access = true` → `Unrestricted: true`.
3. Selain itu → `RegencyIDs` adalah gabungan (union) `regency_id` dari seluruh role milik user via `role_regencies`. Bisa berupa daftar kosong (tidak ada akses sama sekali).

`RegencyScope` dihitung sekali di lapisan API handler (seperti permission check yang sudah ada), lalu diteruskan sebagai parameter biasa ke method domain service yang membutuhkannya — modul domain (`programs`, `dcp3`, `distribution`, `reports`) tidak memanggil balik ke `auth`, mengikuti pola yang sudah dipakai untuk `auth.Principal` dan `auth.ClientMeta` saat ini.

## 5. Penerapan per Modul

### 5.1 Persiapan Program (`programs`)
- `ListRegencies` — difilter sesuai `RegencyScope` (kembalikan semua bila `Unrestricted`, atau hanya baris dengan id dalam `RegencyIDs`).
- `ListSchedules` — difilter berdasarkan `regency_id` milik jadwal.
- `ListPrograms`, `ListPackageTemplates`, `ListDocumentationTemplates` — **tidak difilter**, karena entitas ini tidak terikat satu kabupaten tertentu.

### 5.2 DCP3 (`dcp3`)
- Dropdown jadwal untuk upload/preview mengikuti `ListSchedules` yang sudah terfilter.
- `Preview` dan `Commit` memvalidasi ulang bahwa `schedule_id` yang dikirim ada dalam `RegencyScope` sebelum diproses; jika tidak, kembalikan galat yang sama seperti "jadwal tidak ditemukan" (tidak membocorkan keberadaan jadwal kabupaten lain).

### 5.3 Pendistribusian (`distribution`)
- `Search` memvalidasi `schedule_id` terhadap `RegencyScope` sebelum menjalankan pencarian.
- `GetWorkspace`, `SaveDraft`, `Complete`, serta operasi media (upload/hapus/buka slot) merujuk ke `allocation_id`/`slot_id`/`media_id` — masing-masing perlu menelusuri jadwal yang terkait dan memvalidasi `regency_id` jadwal tersebut ada dalam cakupan sebelum mengizinkan aksi.

### 5.4 Laporan (`reports`)
- Dropdown jadwal mengikuti `ListSchedules` yang terfilter.
- `Summary`, `Rows`, `ExportExcel`, `ExportPDF` memvalidasi `schedule_id` terhadap `RegencyScope` sebelum memproses.

### 5.5 Modul Mendatang
Dokumentasi tambahan dan berbagai jenis Berita Acara yang akan dibangun setelah ini mengikuti pola yang identik: terima `RegencyScope` dari handler, validasi `schedule_id`/`regency_id` terkait sebelum memproses.

## 6. Halaman Role (Administrasi)

Dialog Role (`RoleDialog`) mendapat bagian baru:
- Checkbox **"Akses semua kabupaten"**.
- Jika tidak dicentang: daftar kabupaten (checkbox multi-pilih) untuk memilih kabupaten yang diizinkan. Daftar ini tetap memakai `ListRegencies` yang sama dan difilter dengan `RegencyScope` milik pemanggil seperti modul lain — tidak perlu jalur terpisah, karena halaman Role hanya bisa diakses lewat permission `roles.manage` yang saat ini cuma dimiliki Super Admin, dan Super Admin selalu `Unrestricted` sehingga otomatis melihat semua kabupaten.

`RoleInput` bertambah field `AllRegenciesAccess bool` dan `RegencyIDs []string`.

## 7. Dampak ke Frontend Operasional

Semua dropdown "Jadwal" di halaman Persiapan Program (tab Jadwal), DCP3, Pendistribusian, dan Laporan otomatis hanya menampilkan opsi yang datanya sudah difilter oleh backend — tidak ada perubahan logika filter tambahan di frontend, karena data yang diterima memang sudah terbatas.

## 8. Keamanan dan Audit

- Percobaan mengakses jadwal/kabupaten di luar cakupan (lewat request langsung, bukan dropdown) dikembalikan sebagai galat "tidak ditemukan" — konsisten dengan pola yang sudah ada untuk resource yang tidak ada, supaya tidak membocorkan informasi bahwa data itu sebenarnya ada tapi di luar akses.
- Tidak ada audit event baru untuk percobaan akses ini pada tahap awal; bisa ditambahkan kemudian bila dibutuhkan pemantauan percobaan akses tidak sah.

## 9. Di Luar Cakupan

- Migrasi/backfill akses untuk role yang sudah ada sebelumnya (tidak relevan, belum ada data produksi).
- Audit khusus untuk percobaan akses di luar cakupan.
- Pembatasan serupa untuk modul Administrasi (Pengguna, Role, Pengaturan, Kesehatan Sistem, Riwayat Aktivitas) — modul-modul itu tetap berlaku permission-based seperti sekarang, tidak terikat kabupaten.
