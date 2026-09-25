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
GDRIVE_OAUTH_CLIENT_ID=
GDRIVE_OAUTH_CLIENT_SECRET=
GDRIVE_OAUTH_TOKEN_JSON=
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

`STORAGE_BACKEND=gdrive` mengaktifkan penyimpanan media di Google Drive lewat **OAuth2 user-delegated auth** — bukan Service Account. Service Account tidak punya kuota penyimpanan sendiri di akun Google personal/Google One dan tidak bisa upload file sama sekali (`storageQuotaExceeded`, batasan resmi Google, dikonfirmasi lewat percobaan nyata), walau sudah diberi akses Editor ke suatu folder. OAuth memakai kuota akun Google yang benar-benar login, jadi cocok untuk akun personal.

Setup sekali jalan (lokal, di komputer yang punya browser):
1. Buat OAuth Client ID di Google Cloud Console: APIs & Services → Credentials → Create Credentials → OAuth client ID → tipe **Desktop app**. Kalau project belum pernah pakai OAuth, lengkapi dulu OAuth consent screen (User Type: External, tambahkan email yang akan dipakai sebagai **Test user** — wajib selama app belum diverifikasi publik).
2. Jalankan: `go run ./cmd/gdrive-oauth-setup -client-id=<CLIENT_ID> -client-secret=<CLIENT_SECRET>`
3. Buka URL yang dicetak, login dengan akun Google yang kuotanya mau dipakai, izinkan akses (klik lewati peringatan "Google hasn't verified this app" kalau muncul — wajar untuk app internal yang belum diverifikasi).
4. Tool otomatis menangkap callback, menyimpan token ke `gdrive-oauth-token.json`, membuat folder root baru di Drive, dan mencetak `GDRIVE_ROOT_FOLDER_ID`.
5. Isi `GDRIVE_OAUTH_CLIENT_ID`/`GDRIVE_OAUTH_CLIENT_SECRET`/`GDRIVE_OAUTH_TOKEN_JSON` (path ke file token itu)/`GDRIVE_ROOT_FOLDER_ID` di `.env`/`.env.staging`. Token di-refresh otomatis oleh aplikasi selama berjalan — tidak perlu login ulang kecuali akses dicabut manual dari akun Google.

Yang masih menjadi catatan: upload dari fitur Pendistribusian saat ini belum diorganisir ke folder per kabupaten seperti modul dokumentasi kegiatan — seluruh foto Pendistribusian masuk ke folder root Drive yang sama.

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

`docker-compose.yml` menyediakan tiga service: `postgres` (data di volume `postgres-data`), `app` (image di atas dan volume `storage-data` ke `/app/storage`), dan `caddy` (reverse proxy, HTTPS otomatis via Let's Encrypt berdasarkan domain). Mount token OAuth Google Drive ditambahkan secara eksplisit melalui `docker-compose.gdrive.yml`.

Ada **dua** file environment yang terpisah dan tidak boleh tertukar:
- `.env.staging` — dibaca oleh service `app` (lewat `env_file:`) untuk konfigurasi aplikasi Go (`DATABASE_URL`, `SESSION_SECRET`, `STORAGE_BACKEND`, dll). Salin dari `.env.staging.example`.
- `.env.compose` — dibaca oleh `docker compose` sendiri untuk substitusi variabel di file Compose (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `GDRIVE_OAUTH_TOKEN_HOST_PATH`, `APP_DOMAIN`). **Wajib** diteruskan eksplisit lewat `--env-file .env.compose` di **setiap** perintah `docker compose` (termasuk `ps`/`logs`/`down`).

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
   Edit `.env.staging`: isi `SESSION_SECRET` (`openssl rand -base64 32`), `APP_BASE_URL` (domain HTTPS staging), `DATABASE_URL` (password harus sama dengan `POSTGRES_PASSWORD` di bawah), dan `GDRIVE_OAUTH_CLIENT_ID`/`GDRIVE_OAUTH_CLIENT_SECRET`/`GDRIVE_OAUTH_TOKEN_JSON`/`GDRIVE_ROOT_FOLDER_ID` (lihat bagian OAuth Google Drive di atas untuk cara mendapatkannya — jalankan `cmd/gdrive-oauth-setup` di komputer lokal, lalu upload `gdrive-oauth-token.json` yang dihasilkan ke VPS, di luar folder repo).

   Buat `.env.compose` dari contoh yang tersedia:
   ```bash
   cp .env.compose.example .env.compose
   ```
   Edit `.env.compose`: isi `POSTGRES_PASSWORD` (harus sama dengan yang ada di `.env.staging`), `APP_DOMAIN` (domain staging tanpa `https://`). Jika memakai Google Drive, isi juga `GDRIVE_OAUTH_TOKEN_HOST_PATH`.

   Variabel kritis (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `APP_DOMAIN`) memiliki validasi wajib — Docker Compose akan berhenti dengan pesan error yang jelas jika salah satu kosong.

   **Mode Google Drive** adalah default contoh staging (`STORAGE_BACKEND=gdrive` di `.env.staging.example`). Isi `GDRIVE_OAUTH_TOKEN_HOST_PATH` dengan path absolut token pada host dan gunakan file override pada setiap perintah yang dapat membuat ulang service `app`:

   ```bash
   docker compose -f docker-compose.yml -f docker-compose.gdrive.yml \
     --env-file .env.compose up -d --build
   ```

   **Mode penyimpanan lokal**: ubah `.env.staging` menjadi `STORAGE_BACKEND=local`, kosongkan konfigurasi GDrive aplikasi bila tidak digunakan, lalu jalankan hanya file Compose utama tanpa override GDrive.

5. Jalankan stack staging default (Google Drive):
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.gdrive.yml \
     --env-file .env.compose up -d --build
   ```

6. Buat akun admin pertama:
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.gdrive.yml \
     --env-file .env.compose exec app ./admin create
   ```

7. Verifikasi: `curl https://staging.namadomain.com/api/v1/health` harus mengembalikan `{"status":"ok"}`. Buka domain tersebut di browser dan login.

#### Update / redeploy

```bash
git pull
docker compose -f docker-compose.yml -f docker-compose.gdrive.yml \
  --env-file .env.compose up -d --build
```

Hanya service `app` yang di-rebuild dan direcreate bila source berubah; data `postgres`/`storage` di volume tetap ada. Migrasi baru otomatis diterapkan lewat `ENTRYPOINT` saat container `app` start ulang.

#### Varian: VPS bersama dengan nginx yang sudah ada

Langkah di atas mengasumsikan VPS kosong/khusus, dengan Caddy sebagai reverse proxy (pegang port 80/443 langsung). Kalau VPS sudah punya nginx aktif untuk situs lain (skenario staging saat ini di `konkit.ptkiansantang.com`, satu server dengan situs-situs `*.ptkiansantang.com` lain), pakai pola berikut — Caddy **tidak** dijalankan sama sekali, nginx yang sudah ada jadi reverse proxy:

1. Clone ke `/opt/konkit` (atau path lain yang konsisten dengan proyek lain di server yang sama).
2. Siapkan `.env.staging` seperti biasa (lihat langkah 4 di atas).
3. Tambahkan `docker-compose.override.yml` (Docker Compose otomatis menggabungkannya dengan `docker-compose.yml`) untuk publish port `app` ke `127.0.0.1` saja — bukan port publik, cuma bisa diakses dari nginx di server yang sama:
   ```yaml
   services:
     app:
       ports:
         - "127.0.0.1:8090:8080"
   ```
   (Ganti `8090` kalau port itu sudah dipakai proyek lain di server yang sama — cek dulu dengan `ss -tlnp`.)
4. Jalankan **hanya** `postgres` dan `app`, jangan `caddy`. Karena perintah memakai file Compose eksplisit, sertakan override nginx lokal dan override GDrive:
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.gdrive.yml \
     --env-file .env.compose up -d --build postgres app
   ```
5. Tambahkan site nginx baru, mengikuti pola situs lain di server yang sama (lihat `/etc/nginx/sites-available/` untuk contoh format yang sudah dipakai):
   ```nginx
   server {
       server_name konkit.ptkiansantang.com;

       location / {
           proxy_pass http://localhost:8090;
           proxy_http_version 1.1;
           proxy_set_header Upgrade $http_upgrade;
           proxy_set_header Connection 'upgrade';
           proxy_set_header Host $host;
           proxy_cache_bypass $http_upgrade;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }

       listen 80;
   }
   ```
   ```bash
   ln -sf /etc/nginx/sites-available/konkit.ptkiansantang.com /etc/nginx/sites-enabled/konkit.ptkiansantang.com
   nginx -t   # WAJIB: cek syntax dulu sebelum reload, supaya kalau ada typo tidak menjatuhkan situs lain di server yang sama
   systemctl reload nginx
   ```
6. Aktifkan HTTPS dengan certbot (server ini sudah pakai certbot untuk domain lain, jadi mengikuti pola yang sama):
   ```bash
   certbot --nginx -d konkit.ptkiansantang.com --non-interactive --agree-tos -m <email> --redirect
   ```

**Update/redeploy untuk varian ini** — selalu sebut service secara eksplisit, JANGAN jalankan `docker compose up -d --build` tanpa argumen (itu akan ikut mencoba menjalankan `caddy`, yang bentrok port 80/443 dengan nginx yang sudah ada):
```bash
cd /opt/konkit
git pull
docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.gdrive.yml \
  --env-file .env.compose up -d --build app
```
