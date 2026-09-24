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
STORAGE_BACKEND=local
GDRIVE_SERVICE_ACCOUNT_JSON=
GDRIVE_ROOT_FOLDER_ID=
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

Role kini dapat dibatasi ke satu atau beberapa kabupaten, atau ditandai memiliki akses tanpa batas (mis. untuk peran pengawasan lintas kabupaten); Super Admin selalu tanpa batas. Pembatasan ini diterapkan pada listing kabupaten dan jadwal di Persiapan Program, serta pada seluruh endpoint DCP3, Pendistribusian, dan Laporan yang menerima `schedule_id`/`allocation_id`/`slot_id`/`media_id` — percobaan mengakses data di luar cakupan kabupaten dikembalikan sebagai galat "tidak ditemukan", konsisten dengan resource yang benar-benar tidak ada.

## Alur DCP3 Dan Pendistribusian

Fondasi operasional saat ini mencakup persiapan kabupaten/program/jadwal, template paket dan dokumentasi berversi, import DCP3 `.xlsx`, pencarian penerima, verifikasi identitas, riwayat penerimaan lintas kabupaten, dokumentasi foto, serta finalisasi distribusi transaksional.

Data DCP3 diproses melalui halaman `DCP3`: pilih jadwal aktif, unggah workbook, cocokkan nama kolom, periksa hasil, lalu import. Penerima yang sudah pernah menerima tetap dibuat sebagai alokasi berstatus perlu ditinjau dan ditampilkan dengan label blokir pada halaman `Pendistribusian`. Finalisasi hanya dapat dilakukan bila nama, NIK 16 digit, Kartu Petani/KUSUKA, dan seluruh slot dokumentasi wajib telah lengkap.

Workbook DCP3 tidak harus mengikuti template baku — nama kolom dicocokkan manual di step "Cocokkan kolom". Satu syarat struktural: baris header harus rata satu baris tanpa sel kosong/duplikat. Jika workbook punya baris judul/kop di atas header (format umum dari sebagian kabupaten), sistem menampilkan galat khusus beserta opsi "Lihat & pilih baris header" yang menampilkan pratinjau baris mentah agar pengguna dapat memilih baris header yang benar sebelum mencoba lagi.

Foto disimpan di `STORAGE_PATH` ketika `STORAGE_BACKEND=local` (default). Nilai relatif seperti `./storage` diperbolehkan untuk `APP_ENV=local`; gunakan path absolut di environment test, staging, dan production. Input `Buka kamera` bergantung pada dukungan browser/perangkat, sedangkan `Pilih galeri` dapat digunakan pada desktop maupun mobile.

`STORAGE_BACKEND=gdrive` (dengan `GDRIVE_SERVICE_ACCOUNT_JSON` dan `GDRIVE_ROOT_FOLDER_ID`) mengaktifkan penyimpanan media di Google Drive via Service Account. ID file Drive disimpan permanen sebagai `storage_key` di database (bukan di memori proses), sehingga membuka/menghapus foto tetap berfungsi setelah server di-restart. Yang masih menjadi catatan: upload dari fitur Pendistribusian saat ini belum diorganisir ke folder per kabupaten seperti modul dokumentasi kegiatan — seluruh foto Pendistribusian masuk ke folder root Drive yang sama. Pertimbangkan ini sebelum mengaktifkan `gdrive` di production untuk Pendistribusian.

Halaman `Laporan` menampilkan ringkasan dan tabel alokasi/distribusi/dokumentasi untuk satu jadwal terpilih, dengan export Excel dan PDF, memakai permission `distribution.view` yang sama dengan Pendistribusian.

Template paket kini mendefinisikan daftar opsi merk/tipe mesin (`machine_options`) dan merk/spesifikasi selang (`hose_options`) yang wajib diisi minimal satu sebelum template dipublikasikan; jadwal dapat mencatat nama Konsultan Pengawas opsional; dan halaman Pendistribusian menangkap serial number mesin, selang, serta konkit/reducer per penerima sebagai bagian dari draft yang sudah ada, sebagai fondasi data untuk modul BAST Generator berikutnya.

### Frontend UI

Frontend menggunakan Tailwind CSS v4 dan komponen Shadcn berbasis Base UI. Token tema berada di `frontend/src/styles/global.css`, primitive UI berada di `frontend/src/components/ui`, dan komponen domain tetap berada di folder feature masing-masing.

Tambahkan primitive baru dari direktori `frontend`; sebagai contoh, Button ditambahkan dengan `npx shadcn@latest add button`. Jangan mengubah kontrak API atau permission ketika melakukan perubahan presentasi.

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

### Docker

`Dockerfile` adalah multi-stage build: Node membangun frontend ke `web/static/app`, Go meng-compile tiga binary (`server`, `migrate`, `admin`), lalu image runtime akhir (`debian:bookworm-slim`, non-root) berisi `go.mod` (dipakai kode untuk menemukan folder `web/` relatif terhadap direktori kerja), `web/templates` (halaman login/dashboard sisi server), `web/static/app`, dan ketiga binary. `ENTRYPOINT` menjalankan `./migrate up` lalu `exec ./server`, sehingga migrasi otomatis diterapkan setiap kali container start.

```bash
docker build -t konkit:local .
```

### Stack staging (docker compose)

`docker-compose.yml` menyediakan tiga service: `postgres` (data di volume `postgres-data`), `app` (image di atas, mount volume `storage-data` ke `/app/storage` dan file kredensial Google Service Account read-only), dan `caddy` (reverse proxy, HTTPS otomatis via Let's Encrypt berdasarkan domain).

Ada **dua** file environment yang terpisah dan tidak boleh tertukar:
- `.env.staging` — dibaca oleh service `app` (lewat `env_file:`) untuk konfigurasi aplikasi Go (`DATABASE_URL`, `SESSION_SECRET`, `STORAGE_BACKEND`, dll). Salin dari `.env.staging.example`.
- `.env.compose` — dibaca oleh `docker compose` sendiri untuk substitusi variabel di `docker-compose.yml` (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `GDRIVE_CREDENTIALS_HOST_PATH`, `APP_DOMAIN`). **Wajib** diteruskan eksplisit lewat `--env-file .env.compose` di **setiap** perintah `docker compose` (termasuk `ps`/`logs`/`down`) — tanpa flag ini, compose diam-diam memakai default kosong.

Kedua file ini tidak pernah masuk git (`.gitignore`).

#### Setup awal di VPS (Ubuntu/Debian)

1. Install Docker Engine + Compose plugin dari repo resmi Docker:
   ```bash
   curl -fsSL https://get.docker.com | sudo sh
   sudo usermod -aG docker $USER
   ```
   (logout/login ulang agar keanggotaan grup `docker` aktif)

2. Arahkan DNS domain staging (mis. `staging.namadomain.com`) ke IP publik VPS — buat A record, tunggu propagasi sebelum lanjut ke langkah start (Caddy butuh domain sudah resolve untuk terbitkan sertifikat HTTPS).

3. Clone repo ke VPS:
   ```bash
   git clone https://github.com/RndraThr/Konverter_Kit_Gas.git konkit
   cd konkit
   ```

4. Siapkan kedua file environment:
   ```bash
   cp .env.staging.example .env.staging
   ```
   Edit `.env.staging`: isi `SESSION_SECRET` (`openssl rand -base64 32`), `APP_BASE_URL` (domain HTTPS staging), `DATABASE_URL` (password harus sama dengan `POSTGRES_PASSWORD` di bawah). `STORAGE_BACKEND=local` sudah default — foto disimpan di volume Docker `storage-data`.

   Buat `.env.compose` (tidak ada file contoh karena isinya murni operasional, bukan rahasia aplikasi):
   ```
   POSTGRES_USER=konkit
   POSTGRES_PASSWORD=<sama dengan di .env.staging>
   POSTGRES_DB=konkit
   APP_DOMAIN=staging.namadomain.com
   ```

   > **Catatan `STORAGE_BACKEND=gdrive`:** backend ini butuh Google Workspace dengan Shared Drive — Service Account pada akun Google personal/Google One tidak punya kuota penyimpanan sendiri dan tidak bisa upload file sama sekali ke folder biasa, walau sudah diberi akses Editor (`storageQuotaExceeded`, batasan resmi Google, bukan soal konfigurasi). Baru aktifkan `gdrive` kalau organisasi sudah punya Shared Drive; saat itu tambahkan kembali baris `GDRIVE_CREDENTIALS_HOST_PATH` di `.env.compose` dan un-comment volume kredensial di `docker-compose.yml`.

5. Jalankan stack:
   ```bash
   docker compose --env-file .env.compose up -d --build
   ```

6. Buat akun admin pertama:
   ```bash
   docker compose --env-file .env.compose exec app ./admin create
   ```

7. Verifikasi: `curl https://staging.namadomain.com/api/v1/health` harus mengembalikan `{"status":"ok"}`. Buka domain tersebut di browser dan login.

#### Update / redeploy

```bash
git pull
docker compose --env-file .env.compose up -d --build
```

Hanya service `app` yang di-rebuild dan direcreate bila source berubah; data `postgres`/`storage` di volume tetap ada. Migrasi baru otomatis diterapkan lewat `ENTRYPOINT` saat container `app` start ulang.
