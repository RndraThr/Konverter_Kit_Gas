# Konkit

Sistem manajemen program konverter kit gas berbasis Go, PostgreSQL, dan React.

## Persiapan Lokal

Project menggunakan Go 1.26 dan PostgreSQL 18. Salin `.env.example` menjadi `.env`, lalu isi password PostgreSQL dan session secret lokal. Aplikasi otomatis membaca `.env` ketika dijalankan dari root project.

```dotenv
APP_ENV=local
APP_ADDR=:8080
APP_BASE_URL=http://localhost:8080
DATABASE_URL=postgres://postgres:PASSWORD_POSTGRES@127.0.0.1:5432/konkit?sslmode=disable
SESSION_SECRET=SECRET_RANDOM_MINIMAL_32_KARAKTER
SESSION_COOKIE_SECURE=false
SESSION_TTL=12h
```

Nilai environment dari sistem atau Docker tidak akan ditimpa oleh `.env`. File `.env` berisi rahasia lokal dan sudah dikecualikan melalui `.gitignore`; jangan memasukkannya ke repository atau Docker image. FlyEnv tetap dapat digunakan untuk menyediakan Go dan PostgreSQL, tanpa perlu mengatur Project Environment aplikasi.

## Menyiapkan Database Dan Admin

Pastikan database `konkit` sudah dibuat, kemudian jalankan:

```powershell
go run ./cmd/migrate up
go run ./cmd/admin create
```

Command admin meminta username, email, password, dan konfirmasi password secara interaktif. Password minimal 12 karakter dan tidak ditampilkan di terminal.

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
- API health: `http://localhost:8080/api/v1/health`

## Pengujian

```powershell
go test ./... -count=1
go vet ./...
```

Integration test repository memerlukan database terpisah bernama `konkit_test` melalui `TEST_DATABASE_URL`. Test akan dilewati bila variable tersebut tidak tersedia dan akan menolak database dengan nama selain `konkit_test`.
