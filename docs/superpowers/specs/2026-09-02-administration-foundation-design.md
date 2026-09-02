# Rancangan Administration Foundation Konkit

Versi: 1.0  
Tanggal: 2026-09-02  
Status: Menunggu persetujuan implementasi

## 1. Tujuan

Fase ini membangun area kerja setelah login sebagai fondasi administrasi sebelum modul operasional kabupaten, penerima, Petani, Nelayan, pemasangan, BAST, laporan, dan peta dikembangkan. Fondasi mencakup dashboard responsif, profil pengguna, manajemen pengguna, role dan permission, pengaturan sistem, health monitoring, serta audit log.

Seluruh data tetap berada dalam satu PostgreSQL. Fase ini tidak membuat pemisahan database per kabupaten dan tidak memecah aplikasi menjadi microservice. Aplikasi tetap berupa modular monolith Go dengan kontrak JSON API yang dapat dipisahkan secara bertahap apabila kebutuhan skala benar-benar muncul.

## 2. Prinsip Arsitektur

- Go menjadi pemilik autentikasi, otorisasi, validasi, transaksi database, audit, dan API.
- React menjadi UI area dashboard dan memakai endpoint `/api/v1/*`.
- Login yang sudah ada tetap digunakan dan tidak ditulis ulang.
- Session browser tetap berupa cookie `HttpOnly`; dashboard tidak menyimpan token autentikasi di local storage.
- Pemeriksaan permission selalu dilakukan oleh backend. Kondisi menu pada frontend hanya membantu pengalaman pengguna.
- Environment dan rahasia deployment tetap berasal dari `.env` atau environment Docker, bukan tabel settings.
- API dirancang berdasarkan resource agar bisa digunakan klien lain dan menjadi batas yang jelas jika kelak dipindahkan ke service terpisah.

## 3. Struktur Navigasi

```text
Dashboard
Administrasi
  Pengguna
  Role & Permission
Sistem
  Pengaturan
  System Health
  Audit Log
Akun
  Profil Saya
  Keluar
```

Sidebar tampil permanen pada desktop dan menjadi drawer pada mobile. Header memuat judul halaman, breadcrumb singkat, notifikasi status bila ada, dan menu akun. Menu hanya tampil jika pengguna memiliki permission yang sesuai.

## 4. Arah Visual Dashboard

Dashboard mengikuti identitas login: latar terang, hijau Ergas sebagai warna aksi dan status sehat, serta emas KSM sebagai aksen terbatas. UI dibuat sebagai aplikasi administrasi yang padat, tenang, dan mudah dipindai, bukan landing page.

- Sidebar memakai permukaan putih atau hijau sangat gelap dengan kontras aksesibel.
- Logo Ergas dan KSM tampil proporsional pada brand area tanpa latar hitam.
- Radius panel maksimal 8px dan bayangan digunakan tipis.
- Tombol alat memakai ikon Lucide bila tersedia, disertai tooltip untuk ikon yang tidak langsung dikenal.
- Tabel mendukung pencarian, filter, pagination, empty state, loading state, dan error state.
- Konten utama memiliki lebar fleksibel dan tetap nyaman pada desktop 1366px maupun mobile 360px.
- Tidak ada statistik palsu. Sebelum modul operasional tersedia, dashboard menampilkan status fondasi dan empty state yang jujur.

## 5. Dashboard Awal

Dashboard awal menampilkan informasi yang sudah benar-benar tersedia:

- Sapaan dan identitas pengguna aktif.
- Status aplikasi dan database secara ringkas.
- Jumlah pengguna aktif, pengguna nonaktif, serta role tersedia.
- Waktu login terakhir akun.
- Aktivitas administrasi terbaru dari audit log.
- Area persiapan modul program yang menjelaskan bahwa data kabupaten dan penerima belum tersedia melalui empty state singkat.

Dashboard tidak menghitung penerima, pemasangan, atau BAST sampai tabel domain terkait benar-benar dibuat.

## 6. Profil Saya

Halaman profil dapat diakses semua pengguna yang sudah login.

### 6.1 Informasi Akun

- Menampilkan nama lengkap, username, email, role, status akun, dan waktu login terakhir.
- Pengguna dapat memperbarui nama lengkap, username, dan email miliknya.
- Username dan email dinormalisasi dan harus unik tanpa membedakan huruf besar-kecil.
- Perubahan identitas dicatat pada audit log.

### 6.2 Ubah Password

- Memerlukan password saat ini.
- Password baru minimal 12 karakter dan dikonfirmasi dua kali pada UI.
- Setelah berhasil, seluruh session lain milik pengguna dicabut; session yang dipakai saat ini tetap aktif.
- Password asli tidak pernah dicatat pada log atau audit metadata.

## 7. Manajemen Pengguna

Pengelolaan pengguna tersedia bagi akun dengan permission `users.view` dan `users.manage`.

### 7.1 Daftar Pengguna

- Pencarian berdasarkan username atau email.
- Filter status aktif/nonaktif dan role.
- Pagination berbasis server.
- Kolom nama lengkap, username, email, role, status, login terakhir, dan tanggal dibuat.

### 7.2 Operasi Pengguna

- Membuat pengguna dengan nama lengkap, username, email, password awal, status, dan satu atau lebih role.
- Mengubah nama lengkap, username, email, status, dan role.
- Mengatur password baru secara administratif tanpa menampilkan password lama.
- Menonaktifkan akun sekaligus mencabut seluruh session miliknya.
- Mengaktifkan kembali akun.

Pengguna tidak dihapus secara fisik pada fase ini agar riwayat audit dan kepemilikan data masa depan tetap utuh. Sistem menolak penonaktifan akun sendiri dan menolak operasi yang menyebabkan tidak ada Super Admin aktif.

## 8. Role Dan Permission

### 8.1 Role

- `super_admin` tetap menjadi role sistem dengan akses penuh secara implisit.
- Super Admin dapat membuat role custom, mengubah nama dan deskripsi, serta memasang permission.
- Kode role dibuat stabil saat pembuatan dan tidak dapat diubah setelah digunakan.
- Role sistem tidak dapat dihapus.
- Role custom yang masih digunakan tidak dapat dihapus sebelum seluruh pengguna dipindahkan.

### 8.2 Permission Awal

```text
dashboard.view
users.view
users.manage
roles.view
roles.manage
settings.view
settings.manage
health.view
audit.view
```

Permission domain seperti `regencies.manage`, `beneficiaries.import`, dan `bast.generate` baru ditambahkan bersama fitur terkait. UI role menampilkan matriks permission yang dikelompokkan berdasarkan resource.

## 9. System Settings

System Settings hanya menyimpan konfigurasi bisnis dan tampilan yang aman disimpan di database. Environment, database URL, session secret, credential email, dan rahasia integrasi tidak dapat dilihat atau diubah dari halaman ini.

### 9.1 Settings Awal

- Nama aplikasi, default `Sistem Manajemen Program Konkit Gas`.
- Zona waktu, default `Asia/Jakarta`.
- Format tanggal tampilan, default format Indonesia.
- Bahasa aplikasi, pada fase ini hanya `id-ID`.
- Nama organisasi pelaksana untuk tampilan administratif.

Format nomor BAST, kode kabupaten, template dokumen, daftar komponen, dan data tipikal dokumen belum dimasukkan pada fase ini. Pengaturan tersebut akan menjadi bagian modul persiapan program agar validasinya mengikuti jenis Petani atau Nelayan.

### 9.2 Penyimpanan Settings

Tabel `system_settings` menggunakan key yang unik, nilai JSON, tipe nilai, deskripsi, dan informasi pengguna terakhir yang memperbarui. Backend memiliki daftar key yang diizinkan dan memvalidasi tipe serta nilai setiap key; endpoint tidak menerima key bebas.

Perubahan dilakukan secara atomik dan dicatat pada audit log dengan nilai lama dan baru untuk field nonrahasia.

## 10. System Health

Health dibagi menjadi dua tingkat.

### 10.1 Public Liveness

`GET /api/v1/health` tetap publik dan ringan. Endpoint hanya menyatakan proses HTTP hidup:

```json
{
  "status": "ok"
}
```

Endpoint ini tidak melakukan query database agar tetap berguna untuk liveness probe Docker.

### 10.2 Protected Readiness

`GET /api/v1/system/health` memerlukan `health.view` dan mengembalikan:

- Status keseluruhan `healthy`, `degraded`, atau `unhealthy`.
- Status PostgreSQL dan durasi ping.
- Versi migration aktif.
- Uptime proses.
- Versi aplikasi/build bila tersedia.
- Nama environment seperti `local` atau `production`.
- Waktu pemeriksaan dalam UTC.

Respons tidak memuat host database, username database, credential, isi environment, stack trace, atau path server. Pemeriksaan memiliki timeout pendek agar halaman health tidak menggantung. Kegagalan dependency menghasilkan HTTP `503`; kondisi sehat menghasilkan HTTP `200`.

## 11. Audit Log

Audit log merekam tindakan administratif berikut:

- Pembuatan dan perubahan pengguna.
- Aktivasi atau penonaktifan akun.
- Perubahan assignment role.
- Pembuatan, perubahan, dan penghapusan role custom.
- Perubahan system settings.
- Perubahan profil dan password tanpa menyimpan nilai password.

Setiap record berisi pelaku, action code, resource type, resource ID, metadata JSON yang telah disaring, IP address, user agent, dan waktu. Audit log bersifat append-only dari aplikasi dan tidak memiliki operasi edit/hapus melalui UI.

## 12. Perubahan Database

Migration baru menambahkan:

### 12.1 Perubahan `users`

- `full_name` text not null untuk nama pengguna yang tampil pada aplikasi dan dokumen audit.
- `updated_by` UUID nullable untuk perubahan administratif.
- Index pendukung daftar berdasarkan status dan tanggal dibuat.

Field profil khusus petugas seperti nomor telepon, jabatan, dan cakupan kabupaten belum ditambahkan sampai struktur petugas lapangan disepakati.

### 12.2 `system_settings`

- `key` text primary key.
- `value` jsonb not null.
- `value_type` text not null.
- `description` text nullable.
- `updated_by` UUID nullable ke `users` dengan `ON DELETE SET NULL`.
- `updated_at` timestamptz.

### 12.3 `audit_logs`

- `id` UUID primary key.
- `actor_user_id` UUID nullable dengan `ON DELETE SET NULL`.
- `action` text.
- `resource_type` text.
- `resource_id` text nullable.
- `metadata` jsonb.
- `ip_address` inet nullable.
- `user_agent` text nullable.
- `created_at` timestamptz.

Index disediakan untuk waktu, pelaku, action, dan resource.

## 13. Kontrak API

Seluruh endpoint selain public health memerlukan session cookie. Mutasi memerlukan CSRF token melalui header dan permission yang sesuai.

```text
GET    /api/v1/me
PATCH  /api/v1/me
PUT    /api/v1/me/password

GET    /api/v1/admin/users
POST   /api/v1/admin/users
GET    /api/v1/admin/users/{id}
PATCH  /api/v1/admin/users/{id}
PUT    /api/v1/admin/users/{id}/password

GET    /api/v1/admin/roles
POST   /api/v1/admin/roles
GET    /api/v1/admin/roles/{id}
PATCH  /api/v1/admin/roles/{id}
DELETE /api/v1/admin/roles/{id}
GET    /api/v1/admin/permissions

GET    /api/v1/system/settings
PATCH  /api/v1/system/settings
GET    /api/v1/system/health
GET    /api/v1/system/audit-logs
```

Daftar pengguna dan audit memakai query `page`, `page_size`, `search`, dan filter resource terkait. Respons error memakai bentuk konsisten:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Data belum valid",
    "fields": {
      "email": "Email sudah digunakan"
    }
  }
}
```

Detail internal database hanya masuk log server. UI menerima pesan yang aman dan dapat ditindaklanjuti.

## 14. Struktur Modul Go

```text
internal/admin       service dan repository pengguna/role
internal/profile     service profil dan perubahan password
internal/settings    registry settings, validasi, dan repository
internal/health      readiness checks dan build information
internal/audit       pencatatan dan query audit log
internal/api         router, middleware JSON, dan handler
internal/web         session web dan penyajian shell dashboard
```

Service menerima interface repository agar aturan bisnis dapat diuji tanpa HTTP atau PostgreSQL. Handler hanya menangani parsing, autentikasi, pemetaan error, dan serialisasi.

## 15. Struktur Frontend

```text
frontend/src/app             router dan application shell
frontend/src/components      komponen UI bersama
frontend/src/features/me
frontend/src/features/users
frontend/src/features/roles
frontend/src/features/settings
frontend/src/features/health
frontend/src/features/audit
frontend/src/lib             API client dan utilitas
```

React Router digunakan untuk navigasi dashboard tanpa reload penuh. TanStack Query digunakan untuk cache request, loading state, invalidasi setelah mutasi, dan retry yang terkontrol. Base UI tetap menjadi fondasi komponen aksesibel; CSS Modules dan design tokens tetap menangani visual custom.

Halaman login tetap dapat memakai entry yang sekarang. Route dashboard menyajikan entry React dashboard dan backend menangani fallback route yang masih berada di bawah `/dashboard`.

## 16. Alur Autentikasi API

1. Browser membuka `/dashboard` dan server memvalidasi session sebelum mengirim shell React.
2. React meminta `GET /api/v1/me` untuk identitas, permission efektif, dan CSRF token.
3. API middleware mengambil session cookie dan memuat principal.
4. Handler memeriksa permission resource.
5. Mutasi memverifikasi CSRF header sebelum service dijalankan.
6. Jika session habis, API mengembalikan `401`; frontend mengarahkan ke `/login`.
7. Jika permission tidak cukup, API mengembalikan `403` dan frontend menampilkan halaman akses ditolak.

## 17. Konsistensi Dan Transaksi

- Perubahan user beserta assignment role dilakukan dalam satu transaksi.
- Penonaktifan user dan pencabutan session dilakukan dalam satu transaksi.
- Mutasi data utama dan penulisan audit berada dalam transaksi yang sama.
- Aturan Super Admin terakhir diperiksa dengan locking yang sesuai untuk mencegah dua request bersamaan menonaktifkan seluruh Super Admin.
- Update settings memakai validasi registry backend dan transaksi tunggal.

## 18. Pengujian

### 18.1 Backend

- Unit test aturan profile, user, role, settings, health, dan audit.
- Handler test untuk status `200`, `400`, `401`, `403`, `404`, `409`, dan `503` yang relevan.
- Integration test PostgreSQL untuk transaksi assignment role, perlindungan Super Admin terakhir, settings, dan audit.
- Test memastikan public health tidak bergantung pada database dan protected health tidak membocorkan konfigurasi.

### 18.2 Frontend

- Component test untuk shell, tabel, form, permission-based navigation, loading, empty, dan error state.
- Flow test untuk mengubah profil, membuat user, memasang role, mengubah setting, dan membuka health.
- Pemeriksaan visual desktop dan mobile dengan Playwright.
- Pemeriksaan keyboard navigation, focus state, label form, serta kontras.

## 19. Tahapan Implementasi

1. Migration database, permission seed, repository, dan audit foundation.
2. Middleware API session, permission, CSRF, dan format error.
3. API profil serta perubahan password.
4. API pengguna, role, dan permission.
5. API settings dan protected health.
6. React application shell dan design system dasar.
7. Halaman dashboard, profil, pengguna, role, settings, health, dan audit.
8. Integration test, build production, dan visual QA desktop/mobile.

## 20. Batas Fase

Fase ini belum mencakup:

- Master provinsi/kabupaten dan kode tiga huruf.
- Penjadwalan kabupaten.
- Program Petani dan Nelayan.
- Import Excel calon penerima.
- Pendataan lapangan dan mode offline.
- Format, nomor, preview, atau pencetakan BAST.
- Peta dan koordinat penerima.
- SMTP dan lupa password.
- API token untuk microservice eksternal.

Batas tersebut menjaga fondasi tetap fokus. Struktur permission, audit, settings, API, dan UI yang dibangun pada fase ini menjadi dasar langsung untuk seluruh modul tersebut.

## 21. Kriteria Selesai

- Pengguna terautentikasi dapat membuka dashboard React sesuai permission.
- Super Admin dapat mengelola user dan role tanpa menghilangkan Super Admin aktif terakhir.
- Pengguna dapat memperbarui profil dan password sendiri.
- Settings aman dapat dibaca dan diubah, sementara rahasia tetap hanya berada di environment.
- Public liveness dan protected readiness menghasilkan status yang benar.
- Seluruh mutasi penting menghasilkan audit log.
- UI berfungsi pada desktop dan mobile tanpa overflow atau overlap.
- Test backend, frontend, build production, dan visual QA lulus.
