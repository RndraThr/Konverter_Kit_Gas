# Konkit

Sistem manajemen program konverter kit gas berbasis Go, PostgreSQL, dan React.

## Persiapan Lokal

Project menggunakan Go 1.26, PostgreSQL 18, React 19, dan Vite 7. Salin `.env.example` menjadi `.env`, lalu isi password PostgreSQL dan session secret lokal. Aplikasi otomatis membaca `.env` ketika dijalankan dari root project.

```dotenv
APP_ENV=local
APP_ADDR=:8080
APP_BASE_URL=http://localhost:8080
DATABASE_URL=<URL_DATABASE_POSTGRESQL>
SESSION_SECRET=<RAHASIA_ACAK>
SESSION_COOKIE_SECURE=false
SESSION_TTL=12h
STORAGE_PATH=./storage
```

Nilai environment dari sistem atau Docker tidak akan ditimpa oleh `.env`. File `.env` berisi rahasia lokal dan sudah dikecualikan melalui `.gitignore`; jangan memasukkannya ke repository atau Docker image. FlyEnv tetap dapat menyediakan runtime Go dan PostgreSQL tanpa menyimpan rahasia aplikasi di Project Environment.

## Menyiapkan Database Dan Admin

Siapkan database utama dan database pengujian:

```sql
CREATE DATABASE konkit;
CREATE DATABASE konkit_test;
```

Format URL PostgreSQL adalah `postgres://USER:PASSWORD@HOST:PORT/konkit?sslmode=disable`. `SESSION_SECRET` wajib berisi minimal 32 karakter acak.

Jalankan migration dan buat akun Super Admin pertama:

```powershell
go run ./cmd/migrate up
go run ./cmd/admin create
```

Command admin meminta username, email, password, dan konfirmasi password secara interaktif. Password minimal 12 karakter dan tidak ditampilkan di terminal. Migration harus dijalankan setiap ada perubahan schema.

## Menjalankan Aplikasi

Bangun frontend setelah mengubah React atau CSS:

```powershell
Set-Location frontend
npm.cmd run build
Set-Location ..
```

Jalankan server:

```powershell
go run ./cmd/server
```

Alamat lokal:

- Login: `http://localhost:8080/login`
- Dashboard: `http://localhost:8080/dashboard`
- Liveness publik: `http://localhost:8080/api/v1/health`
- Readiness terproteksi: `http://localhost:8080/api/v1/system/health`

Liveness hanya memastikan proses HTTP hidup. Readiness memerlukan login dan permission `health.view`, lalu memeriksa PostgreSQL serta versi migration tanpa mengirim connection string atau error driver ke browser.

## Fondasi Administrasi

Dashboard awal menyediakan:

- profil dan perubahan password;
- pengguna, aktivasi akun, serta penetapan role;
- role kustom dan permission;
- pengaturan aplikasi yang tervalidasi;
- status dependency dan migration;
- audit log read-only untuk perubahan penting.

Browser memakai cookie sesi `HttpOnly`. Mutation API memerlukan token CSRF dari bootstrap `GET /api/v1/me`. Session token dan rahasia tidak disimpan di browser storage.

Role kini dapat dibatasi ke satu atau beberapa kabupaten, atau ditandai memiliki akses tanpa batas (mis. untuk peran pengawasan lintas kabupaten); Super Admin selalu tanpa batas. Pembatasan ini sudah diterapkan pada listing kabupaten dan jadwal di Persiapan Program — penerapan pada DCP3, Pendistribusian, dan Laporan menyusul pada perubahan terpisah.

## Alur DCP3 Dan Pendistribusian

Fondasi operasional saat ini mencakup persiapan kabupaten/program/jadwal, template paket dan dokumentasi berversi, import DCP3 `.xlsx`, pencarian penerima, verifikasi identitas, riwayat penerimaan lintas kabupaten, dokumentasi foto, serta finalisasi distribusi transaksional.

Data DCP3 diproses melalui halaman `DCP3`: pilih jadwal aktif, unggah workbook, cocokkan nama kolom, periksa hasil, lalu import. Penerima yang sudah pernah menerima tetap dibuat sebagai alokasi berstatus perlu ditinjau dan ditampilkan dengan label blokir pada halaman `Pendistribusian`. Finalisasi hanya dapat dilakukan bila nama, NIK 16 digit, Kartu Petani/KUSUKA, dan seluruh slot dokumentasi wajib telah lengkap.

Foto disimpan di `STORAGE_PATH`. Nilai relatif seperti `./storage` diperbolehkan untuk `APP_ENV=local`; gunakan path absolut di environment test, staging, dan production. Input `Buka kamera` bergantung pada dukungan browser/perangkat, sedangkan `Pilih galeri` dapat digunakan pada desktop maupun mobile.

Halaman `Laporan` menampilkan ringkasan dan tabel alokasi/distribusi/dokumentasi untuk satu jadwal terpilih, dengan export Excel dan PDF, memakai permission `distribution.view` yang sama dengan Pendistribusian.

Template paket kini mendefinisikan daftar opsi merk/tipe mesin (`machine_options`) dan merk/spesifikasi selang (`hose_options`) yang wajib diisi minimal satu sebelum template dipublikasikan; jadwal dapat mencatat nama Konsultan Pengawas opsional; dan halaman Pendistribusian menangkap serial number mesin, selang, serta konkit/reducer per penerima sebagai bagian dari draft yang sudah ada, sebagai fondasi data untuk modul BAST Generator berikutnya.

## Pengujian

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
go test ./... -count=1
go vet ./...
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
```

Integration test repository memerlukan database terpisah bernama `konkit_test` melalui `TEST_DATABASE_URL`. Test akan dilewati bila variable tersebut tidak tersedia dan menolak database dengan nama selain `konkit_test`.

Pengujian end-to-end otomatis menurunkan `DATABASE_URL` lokal menjadi database `konkit_test`, menjalankan migration, membuat akun/jadwal/riwayat uji sementara, serta menghasilkan workbook DCP3 desktop dan mobile di `.cache/e2e`:

```powershell
npm.cmd --prefix frontend run e2e
```

Konfigurasi Playwright memakai instalasi Google Chrome lokal dan menjalankan skenario pada viewport desktop `1366x768` serta mobile `375x812`. Untuk menyiapkan fixture secara manual:

```powershell
$env:APP_ENV="test"
$env:DATABASE_URL="postgres://USER:PASSWORD@127.0.0.1:5432/konkit_test?sslmode=disable"
$env:STORAGE_PATH="$PWD/.cache/e2e-storage"
go run ./cmd/migrate up
go run ./cmd/e2eseed -fixture-dir .cache/e2e
go run ./cmd/server
```

File yang dihasilkan adalah `.cache/e2e/dcp3-desktop.xlsx` dan `.cache/e2e/dcp3-mobile.xlsx`. Hapus seluruh fixture E2E dengan `go run ./cmd/e2eseed cleanup -fixture-dir .cache/e2e`. Seeder hanya dapat berjalan saat `APP_ENV=test` dan nama database persis `konkit_test`; jangan menjalankannya terhadap database operasional.

## Alur Git Dan Deployment

Build frontend menghasilkan asset ter-hash di `web/static/app`, lalu binary Go menyajikan halaman, static asset, dan API dari satu origin. Konfigurasi tetap berbasis environment variable sehingga tahap Docker berikutnya tidak memerlukan perubahan kode aplikasi.
