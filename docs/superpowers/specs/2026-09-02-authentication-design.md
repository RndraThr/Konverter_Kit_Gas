# Rancangan Fondasi Autentikasi Konkit

Versi: 1.0  
Tanggal: 2026-09-02  
Status: Revisi hybrid, menunggu persetujuan implementasi

## 1. Tujuan

Fase ini mengubah halaman login yang masih berupa tampilan menjadi autentikasi nyata untuk Super Admin. Hasil akhirnya adalah koneksi PostgreSQL, migration database, pembuatan akun awal melalui terminal, session login, logout, fondasi role dan permission, serta halaman dashboard terproteksi.

Pendekatan yang dipakai adalah hybrid: database dan batas modul disiapkan untuk banyak role serta API sejak awal, tetapi fitur yang bergantung pada layanan atau data lain diterapkan ketika dependensinya tersedia. Pengiriman email lupa password menunggu SMTP, token API menunggu API pertama, dan widget dashboard operasional menunggu modul kabupaten, penerima, pemasangan, serta BAST.

## 2. Pilihan Teknis

- Driver PostgreSQL: `pgx/v5`.
- Migration: `goose/v3`, dijalankan melalui command milik aplikasi.
- Password: Argon2id melalui `golang.org/x/crypto`.
- Session: opaque server-side session yang disimpan di PostgreSQL.
- Otorisasi: role-based access control (RBAC) dengan tabel role dan permission.
- Cookie: `HttpOnly`, `SameSite=Lax`, dan `Secure` berdasarkan environment.
- Akun awal: dibuat melalui CLI interaktif, bukan dari migration atau password di environment.

Session server-side dipilih karena mudah dicabut, mudah diaudit, dan cocok untuk dashboard internal. JWT belum digunakan untuk browser. API atau microservice dapat memakai skema token terpisah di fase berikutnya tanpa mengubah login dashboard.

## 3. Environment

FlyEnv menyediakan environment berikut:

```zsh
export APP_ENV="local"
export APP_ADDR=":8080"
export APP_BASE_URL="http://localhost:8080"
export DATABASE_URL="postgres://postgres:PASSWORD@127.0.0.1:5432/konkit?sslmode=disable"
export SESSION_SECRET="RANDOM_SECRET_MINIMAL_32_KARAKTER"
export SESSION_COOKIE_SECURE="false"
export SESSION_TTL="12h"
```

Aplikasi wajib berhenti saat startup apabila `DATABASE_URL`, `SESSION_SECRET`, atau durasi session tidak valid. Nilai rahasia tidak dicatat ke log.

Untuk local development, user database `postgres` masih diperbolehkan. Deployment Docker nantinya menggunakan role PostgreSQL khusus aplikasi.

## 4. Struktur Database

### 4.1 `users`

- `id` UUID primary key.
- `username` teks unik, disimpan dalam bentuk lowercase.
- `email` teks unik, disimpan dalam bentuk lowercase.
- `password_hash` teks hasil Argon2id.
- `is_active` boolean.
- `last_login_at` timestamp nullable.
- `created_at` dan `updated_at` timestamp.

Username dan email dapat dipakai pada field identitas login yang sama. Akun nonaktif tidak boleh membuat session baru. Role tidak disimpan langsung pada tabel ini agar satu pengguna dapat menerima lebih dari satu role tanpa mengubah skema.

### 4.2 `sessions`

- `id` UUID primary key.
- `user_id` foreign key ke `users` dengan cascade delete.
- `token_hash` byte array unik.
- `expires_at` timestamp.
- `created_at` dan `last_seen_at` timestamp.
- `ip_address` dan `user_agent` nullable untuk kebutuhan audit dasar.

Browser hanya menerima token acak. Database menyimpan SHA-256 dari token tersebut, sehingga kebocoran isi tabel tidak langsung menghasilkan cookie yang dapat digunakan.

### 4.3 `roles`

- `id` UUID primary key.
- `code` teks unik dan stabil, misalnya `super_admin`.
- `name` nama yang tampil pada UI.
- `description` nullable.
- `is_system` boolean untuk melindungi role inti.
- `created_at` dan `updated_at` timestamp.

Migration pertama membuat role sistem `super_admin`. Role operasional seperti admin program, koordinator kabupaten, petugas lapangan, dan viewer ditambahkan setelah alur kerja serta batas aksesnya disetujui.

### 4.4 `permissions`

- `id` UUID primary key.
- `code` teks unik dengan pola `resource.action`.
- `name` nama yang tampil pada UI.
- `description` nullable.
- `created_at` timestamp.

Contoh kode permission berikutnya adalah `users.manage`, `regencies.view`, `regencies.manage`, `beneficiaries.import`, dan `bast.generate`. Migration autentikasi hanya menanam permission yang sudah digunakan oleh fitur pada fase berjalan.

### 4.5 `user_roles`

- `user_id` foreign key ke `users`.
- `role_id` foreign key ke `roles`.
- `assigned_at` timestamp.
- Composite primary key pada `user_id` dan `role_id`.

### 4.6 `role_permissions`

- `role_id` foreign key ke `roles`.
- `permission_id` foreign key ke `permissions`.
- Composite primary key pada `role_id` dan `permission_id`.

Role `super_admin` memiliki akses penuh melalui aturan sistem, sehingga tidak memerlukan penyalinan semua permission setiap kali permission baru ditambahkan.

## 5. Command Aplikasi

### 5.1 Migration

```text
go run ./cmd/migrate up
go run ./cmd/migrate status
go run ./cmd/migrate down
```

File SQL disimpan berurutan di `migrations/`. Command `down` hanya digunakan secara sadar saat development dan tidak dijalankan otomatis oleh web server.

### 5.2 Membuat Super Admin

```text
go run ./cmd/admin create
```

CLI meminta username, email, dan password secara interaktif. Password tidak ditampilkan di terminal dan diminta dua kali. CLI menolak username/email duplikat serta password yang terlalu pendek, kemudian memberikan role `super_admin`. Migration tidak pernah menanam password default.

## 6. Alur Web

### 6.1 Login

1. `GET /login` menampilkan form dan pesan error generik bila ada.
2. `POST /login` memvalidasi input dan mencari user berdasarkan lowercase username atau email.
3. Password diverifikasi menggunakan hash Argon2id.
4. Jika valid, session lama yang kedaluwarsa dibersihkan dan session baru dibuat.
5. Cookie session dikirim, lalu pengguna diarahkan ke `/dashboard`.
6. Jika gagal, pengguna kembali ke halaman login dengan pesan `Email/username atau password tidak sesuai` tanpa membocorkan field mana yang salah.

Checkbox `Ingat perangkat ini` memperpanjang masa session. Durasi normal mengikuti `SESSION_TTL`; durasi remembered direncanakan 30 hari.

### 6.2 Proteksi Dashboard

- `GET /dashboard` wajib melewati middleware autentikasi.
- Session dicari menggunakan hash token cookie dan harus belum kedaluwarsa.
- User harus aktif.
- Pengguna anonim diarahkan ke `/login`.
- Pengguna yang sudah login dan membuka `/login` diarahkan ke `/dashboard`.

Dashboard pada fase ini cukup berupa shell awal yang memastikan alur autentikasi bekerja. Desain dan modul dashboard lengkap dibuat pada fase berikutnya.

### 6.3 Logout

`POST /logout` menghapus session aktif dari database, menghapus cookie, lalu mengarahkan pengguna ke `/login`. Logout tidak menggunakan metode GET.

### 6.4 Otorisasi Role Dan Permission

- Middleware autentikasi memastikan pengguna sudah login.
- Middleware otorisasi memeriksa permission yang dibutuhkan route.
- `super_admin` selalu lolos pemeriksaan permission selama akun aktif.
- Pemeriksaan akses dilakukan di backend. Menyembunyikan menu di frontend hanya untuk pengalaman pengguna dan bukan batas keamanan.
- Pengelolaan role melalui UI dibuat setelah daftar role operasional dan matriks izinnya disepakati.

### 6.5 Fondasi API

- Endpoint API berikutnya menggunakan prefix `/api/v1`.
- Handler web, handler API, dan service bisnis menggunakan batas package yang terpisah.
- Login browser tetap menggunakan session cookie.
- Token API pengguna atau kredensial antarlayanan dibuat ketika konsumen API pertama sudah diketahui.
- Middleware API mengembalikan JSON status `401` atau `403`, bukan redirect HTML.

### 6.6 Lupa Password

Alur reset password akan memakai token acak sekali pakai yang disimpan dalam bentuk hash dan memiliki masa kedaluwarsa. Implementasi pengiriman baru diaktifkan setelah SMTP tersedia. Sampai saat itu, tautan lupa password tidak ditampilkan agar pengguna tidak diarahkan ke alur yang belum dapat menyelesaikan permintaan.

## 7. Keamanan Dan Error Handling

- Password tidak pernah disimpan atau dicatat dalam bentuk asli.
- Perbandingan password menggunakan implementasi yang tahan timing attack.
- Token session dibuat dari random source kriptografis.
- Cookie memakai path `/`, `HttpOnly`, `SameSite=Lax`, dan `Secure=true` di production.
- Login memiliki pembatasan percobaan sederhana per IP dan identitas. Penyimpanan terdistribusi seperti Redis baru diperlukan ketika aplikasi dijalankan pada banyak instance.
- Pesan login gagal dibuat generik.
- Error database dicatat di server tanpa menampilkan detail internal kepada pengguna.
- Semua query menggunakan parameter, bukan gabungan string SQL.
- State-changing request berikutnya akan memakai perlindungan CSRF. Pada fase ini logout menggunakan token CSRF dari halaman dashboard.
- Setiap route sensitif wajib menentukan permission yang diperlukan saat route tersebut dibuat.

## 8. Perubahan UI Login

- Form tetap menggunakan desain saat ini.
- Field `identity` dan `password` menjadi wajib.
- Tombol menampilkan status proses dan tidak dapat diklik dua kali ketika request berjalan.
- Pesan validasi atau kredensial salah tampil dekat form dan dapat dibaca screen reader.
- Tautan `Lupa password?` disembunyikan sampai fitur reset password tersedia.

## 9. Pengujian

- Unit test konfigurasi environment dan validasi durasi.
- Unit test hash dan verifikasi password.
- Unit test penetapan role serta pemeriksaan permission.
- Integration test repository user dan session terhadap PostgreSQL test database bila tersedia.
- Handler test untuk login berhasil, login gagal, akun nonaktif, logout, session kedaluwarsa, dan proteksi dashboard.
- Frontend build dan test kontrak field form.
- `go test ./...` dan `npm run build` wajib lulus sebelum fase dinyatakan selesai.

## 10. Urutan Implementasi

1. Config loader dan validasi environment.
2. Koneksi PostgreSQL dan health check startup.
3. SQL migration untuk `users`, `sessions`, `roles`, `permissions`, `user_roles`, dan `role_permissions`.
4. Password hashing dan repository autentikasi.
5. CLI migration dan CLI pembuatan Super Admin.
6. Session service, middleware autentikasi, dan middleware otorisasi.
7. Handler login, logout, dan dashboard terproteksi.
8. Batas package awal untuk endpoint `/api/v1` tanpa menerbitkan token yang belum memiliki konsumen.
9. Integrasi pesan error/loading pada UI login.
10. Verifikasi end-to-end lokal.

## 11. Kriteria Selesai

- Database kosong dapat disiapkan dengan command migration.
- Super Admin dapat dibuat tanpa password tersimpan di source code, migration, atau environment.
- Kredensial benar membuka dashboard dan kredensial salah tetap di halaman login.
- Dashboard tidak dapat dibuka tanpa session valid.
- Logout mencabut session.
- Super Admin mendapatkan akses penuh melalui RBAC dan middleware permission dapat diuji secara terpisah.
- Struktur handler siap menampung API versi pertama tanpa mencampur respons JSON dengan alur halaman web.
- Konfigurasi lokal FlyEnv terdokumentasi dan tetap kompatibel dengan deployment Docker berikutnya.
