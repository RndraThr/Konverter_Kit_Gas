# Audit Kebersihan, Modularitas, dan Kesiapan Program

**Tanggal pemeriksaan:** 24 September 2026  
**Branch:** `main`  
**Commit yang diperiksa:** `9a572b0` (`feat(database): seed one KONKIT-2026 schedule per regency`)  
**Ruang lingkup:** backend Go, frontend React, migrasi database, pengujian, kontrol keamanan aplikasi, artefak build, dan konfigurasi deployment.

## Ringkasan eksekutif

Kondisi program terbaru **jauh lebih baik dibanding pemeriksaan sebelumnya**. Ketidaksesuaian skema distribusi yang dahulu menjadi risiko utama sudah diselesaikan: kode produksi tidak lagi menggunakan tabel lama `distribution_records`, dan implementasi sekarang konsisten menggunakan `distribution_slots` serta `distribution_slot_id`.

Backend, unit test frontend, type-check, dan build produksi seluruhnya lulus. Seeder E2E juga sudah dapat menjalankan migrasi sampai versi 14 dan membentuk fixture dengan benar. Kontrol keamanan dasar yang diperiksa masih diterapkan dengan baik.

Belum disarankan menyebut seluruh sistem sepenuhnya selesai atau bebas risiko karena masih ada tiga kelompok pekerjaan terbuka:

1. Empat skenario E2E gagal karena helper navigasi belum mengikuti struktur sidebar dan menu akun terbaru.
2. Bundle frontend yang tersimpan di `web/static/app` tidak identik dengan hasil build source terbaru.
3. Otomasi kualitas kode masih terbatas: belum ada CI, lint/format frontend, aturan line ending repository, dan pemindaian kerentanan dependency yang terdokumentasi.

Tidak ditemukan masalah **P0/kritis** dari pemeriksaan ini. Temuan prioritas tertinggi saat ini adalah konsistensi pengujian end-to-end dan artefak rilis.

## Status pemeriksaan

| Area | Status | Hasil utama |
|---|---|---|
| Kompilasi dan test Go | Lulus | `go test ./...` lulus seluruh package |
| Analisis statis Go | Lulus | `go vet ./...` lulus |
| Unit/component test frontend | Lulus | 32 file, 139 test lulus |
| TypeScript | Lulus | `tsc --noEmit` lulus |
| Build frontend produksi | Lulus dengan catatan | Build berhasil; bundle JavaScript 746,09 kB dan Vite memberi peringatan chunk >500 kB |
| E2E Playwright | Perlu perbaikan | 13 lulus, 4 gagal, 3 dilewati sesuai viewport |
| Migrasi dan seed E2E | Lulus | Migrasi sampai versi 14 dan fixture berhasil dibuat |
| Skema distribusi | Lulus | Referensi tabel lama tidak ditemukan pada kode produksi |
| Kontrol keamanan aplikasi | Baik dengan batasan audit | Session, CSRF, permission, request limit, dan hashing password tersedia |
| Deployment Docker | Layak dengan catatan operasional | Multi-stage build dan non-root baik; kontrak `.env.compose` perlu dibuat lebih tahan salah konfigurasi |
| Kebersihan tooling | Perlu ditingkatkan | Belum ada CI, lint/format script frontend, dan `.gitattributes` |

## Perbandingan dengan temuan sebelumnya

| Temuan sebelumnya | Kondisi terbaru | Status |
|---|---|---|
| Kode masih bergantung pada `distribution_records` sementara skema bergerak ke slot | Kode produksi memakai `distribution_slots`/`distribution_slot_id`; nama lama hanya muncul pada test yang memastikan tabel lama sudah tidak ada dan komentar historis | **Selesai** |
| Seeder/E2E gagal akibat ketidaksesuaian tabel distribusi | Migrasi dan seed E2E berhasil, termasuk data akun, jadwal, dan DCP3 | **Selesai** |
| Belum dapat memastikan seluruh package Go lulus | `go test ./...` dan `go vet ./...` lulus | **Selesai** |
| Integrasi tiga POS belum utuh | POS Mesin, POS Dokumen, dan POS Penyerahan sudah memiliki model, repository, endpoint, UI, dan test | **Selesai untuk fondasi** |
| File besar berisiko sulit dirawat | Beberapa hotspot masih besar, terutama `internal/administration/repository.go`, `internal/api/routes.go`, `internal/distribution/repository.go`, dan `frontend/src/app/AppShell.tsx` | **Masih terbuka** |
| Belum ada lint/format/CI frontend | Script masih terbatas pada dev, build, preview, unit test, watch, dan E2E | **Masih terbuka** |
| Risiko perbedaan line ending | `.gitattributes` belum tersedia dan konfigurasi Git lokal memakai konversi line ending | **Masih terbuka** |
| Bundle frontend besar dan belum dipecah per route | Tidak ditemukan `React.lazy`; bundle utama menghasilkan peringatan >500 kB | **Masih terbuka** |
| Deployment belum terstruktur | Dockerfile multi-stage, Compose, Caddy, dan dokumentasi VPS sudah tersedia | **Meningkat signifikan** |

## Temuan terperinci

### P1 — Helper navigasi E2E tertinggal dari UI terbaru

Empat skenario gagal karena timeout 30 detik, bukan karena migrasi, API, atau seeder gagal:

- Desktop dan mobile: `administration foundation journey`.
- Desktop dan mobile: `DCP3 to completed package distribution`.

Akar masalah yang terlihat:

- Test administrasi mencoba mencari `Profil saya` sebagai tautan sidebar, sedangkan UI terbaru menempatkannya di dropdown `Menu akun`.
- Test DCP3 mencari `Persiapan program` saat kelompok sidebar `Operasional` masih tertutup. Helper hanya membuka drawer pada mobile dan belum membuka kelompok menu pemilik tautan.

Rekomendasi:

1. Ubah helper navigasi agar mengetahui kelompok sidebar tujuan dan membukanya ketika tautan belum terlihat.
2. Untuk profil, klik `Menu akun` lalu `Profil saya`, sehingga test mengikuti alur pengguna nyata.
3. Tambahkan assertion pendek untuk keberadaan trigger navigasi agar kegagalan tidak selalu menghabiskan timeout penuh 30 detik.
4. Jadikan seluruh skenario yang berlaku pada viewport terkait lulus sebelum rilis berikutnya.

### P1 — Artefak frontend tersimpan tidak sinkron dengan source

Build produksi dari source terbaru berhasil, tetapi menghasilkan hash aset yang berbeda dari bundle yang saat ini tersimpan di `web/static/app`:

- Hasil build terbaru: `index-BvDmvzLq.css` dan `index-PKDKclwp.js`.
- Manifest tersimpan mengarah ke: `index-BCi5cWG6.css` dan `index-xd8a5uwC.js`.

Dockerfile aman dari masalah ini karena membangun frontend dari source pada tahap image build. Namun, eksekusi server Go langsung dari working tree dapat menyajikan bundle lama bila `web/static/app` belum dibangun ulang.

Pilih dan dokumentasikan satu kebijakan:

- Jika artefak build tetap dilacak Git, build dan commit `web/static/app` setiap perubahan frontend serta verifikasi kesesuaiannya di CI.
- Jika artefak dianggap generated, keluarkan dari Git dan pastikan semua jalur run/deploy selalu membangunnya lebih dahulu.

### P2 — Konfigurasi Compose masih mudah salah digunakan

README sudah menjelaskan bahwa setiap perintah Compose harus memakai `--env-file .env.compose`. Tanpa file tersebut, `docker compose config --quiet` menghasilkan variabel kosong dan volume Google Drive menjadi spesifikasi tidak valid. Mode storage lokal juga meminta operator mengomentari volume OAuth secara manual.

Ini bukan celah keamanan langsung, tetapi meningkatkan risiko salah deployment. Rekomendasi:

- Gunakan ekspansi wajib seperti `${APP_DOMAIN:?APP_DOMAIN wajib diisi}` dan pola serupa untuk nilai kritis.
- Sediakan `.env.compose.example` tanpa rahasia untuk memperjelas kontrak variabel operasional.
- Pisahkan override Compose Google Drive agar mode local tidak memerlukan edit manual terhadap file utama.
- Tambahkan `docker compose --env-file .env.compose config --quiet` ke checklist deployment/CI.

### P2 — Otomasi kualitas belum lengkap

Belum ditemukan workflow CI di `.github`, dan `frontend/package.json` belum memiliki script lint atau format. Konsekuensinya, hasil yang lulus di mesin pengembang belum dipaksa secara konsisten pada setiap push atau pull request.

Minimum pipeline yang disarankan:

1. `go test ./...`
2. `go vet ./...`
3. `npm ci`
4. `npm test -- --run --maxWorkers=1`
5. `npm exec tsc -- --noEmit`
6. `npm run build`
7. Playwright dengan PostgreSQL dan migrasi nyata
8. Pemeriksaan bahwa bundle tersimpan sesuai source, jika bundle tetap dilacak

Tambahkan ESLint dan formatter yang disepakati, lalu sediakan script eksplisit seperti `lint`, `format:check`, dan `typecheck`.

### P2 — File hotspot masih perlu dipecah bertahap

Ukuran codebase saat pemeriksaan:

- Frontend: sekitar 112 file source dan 7.096 baris.
- Backend: sekitar 114 file Go dan 16.044 baris.

Hotspot utama:

- `internal/administration/repository.go`: sekitar 609 baris.
- `internal/api/routes.go`: sekitar 531 baris.
- `internal/distribution/repository.go`: sekitar 490 baris.
- `internal/programs/repository.go`: sekitar 406 baris.
- `internal/recipients/repository.go`: sekitar 388 baris.
- `frontend/src/app/AppShell.tsx`: sekitar 343 baris.
- `frontend/src/pages/DashboardPage.tsx`: sekitar 270 baris.

Tidak perlu refactor besar sekaligus. Pecah berdasarkan tanggung jawab saat fitur terkait disentuh, misalnya route registration, query/read repository, command/write repository, konfigurasi navigasi, account menu, dan responsive shell.

### P2 — Bundle utama perlu code splitting

Build menghasilkan JavaScript sekitar 746,09 kB (gzip 228,17 kB) dan peringatan Vite karena chunk lebih dari 500 kB. Belum ditemukan lazy loading route.

Rekomendasi:

- Terapkan lazy loading pada halaman berat, terutama persiapan program, dokumentasi, administrasi, laporan, dan alur distribusi.
- Ukur ulang setelah pemisahan; jangan membuat manual chunk tanpa data bila lazy route sudah cukup.
- Pertimbangkan budget bundle di CI agar pertumbuhan ukuran terlihat sebelum merge.

### P3 — Konsistensi line ending repository

Repository belum memiliki `.gitattributes`, sementara lingkungan Windows dapat mengubah LF/CRLF secara otomatis. Hal ini dapat memunculkan perubahan palsu pada file hasil build dan konfigurasi.

Rekomendasi awal:

```gitattributes
* text=auto
*.go text eol=lf
*.ts text eol=lf
*.tsx text eol=lf
*.js text eol=lf
*.json text eol=lf
*.yml text eol=lf
*.yaml text eol=lf
*.sh text eol=lf
```

Normalisasi harus dilakukan dalam commit terpisah agar review tidak bercampur dengan perubahan fitur.

### P3 — Migrasi otomatis pada startup perlu ditinjau saat scale-out

Container menjalankan migrasi sebelum server dimulai. Untuk satu instance saat ini pola ini praktis. Jika nanti beberapa replica dimulai bersamaan, migrasi startup dapat menjadi titik kompetisi operasional.

Saat menuju scale-out, pindahkan migrasi ke release job/deployment step tunggal sebelum replica aplikasi dinaikkan.

## Catatan keamanan

Kontrol yang terkonfirmasi melalui kode dan test:

- Password menggunakan Argon2id.
- Session cookie memakai `HttpOnly`, `SameSite=Lax`, dan mendukung flag `Secure` dari konfigurasi.
- Mutasi API memvalidasi token CSRF yang terikat pada sesi.
- Endpoint memeriksa permission dan beberapa alur juga menerapkan scope wilayah.
- Body JSON/multipart pada beberapa endpoint dibatasi dengan `http.MaxBytesReader`.
- `SESSION_SECRET` wajib minimal 32 byte.
- File `.env`, `.env.staging`, token OAuth Google Drive, service-account JSON, storage, dan hasil test browser diabaikan Git.
- Docker runtime menjalankan aplikasi sebagai user non-root.
- Integrasi Google Drive terbaru memakai OAuth2 user-delegated token dan token tidak disimpan ke repository.

Batasan pemeriksaan keamanan:

- Ini adalah review statis terarah dan verifikasi test, bukan penetration test.
- Belum dilakukan dynamic security scan terhadap aplikasi yang berjalan.
- Belum dilakukan pemindaian CVE dependency yang menghasilkan baseline tersimpan.
- Validitas permission diuji cukup luas pada handler, tetapi audit matriks seluruh role terhadap seluruh endpoint tetap disarankan sebelum produksi.

## Urutan perbaikan yang disarankan

### Sebelum rilis berikutnya

- Perbarui helper E2E dan capai hasil hijau untuk semua skenario yang berlaku.
- Tentukan kebijakan `web/static/app`, lalu sinkronkan bundle dengan source.
- Validasi deployment menggunakan file `.env.compose` nyata pada staging.

### Dalam siklus pengembangan terdekat

- Tambahkan CI untuk Go, TypeScript, test frontend, build, dan E2E database nyata.
- Tambahkan lint/format frontend.
- Tambahkan `.gitattributes` melalui commit normalisasi terpisah.
- Buat `.env.compose.example` dan kurangi kebutuhan edit manual Compose.

### Bertahap saat fitur terkait disentuh

- Pecah file hotspot berdasarkan domain/tanggung jawab.
- Terapkan lazy loading per route.
- Tambahkan audit dependency dan matriks role-permission sebagai pemeriksaan berkala.

## Kesimpulan

Fondasi sistem terbaru **sudah sehat untuk dilanjutkan pengembangannya**. Risiko struktural terbesar dari audit sebelumnya—ketidaksesuaian skema distribusi—telah selesai, dan jalur build utama sekarang hijau. Fokus berikutnya bukan lagi perbaikan arsitektur darurat, melainkan memastikan test E2E mengikuti UI terbaru, artefak rilis selalu sinkron, dan quality gate otomatis tersedia.

Status keseluruhan: **baik, dengan pekerjaan P1 pada E2E dan sinkronisasi bundle sebelum dinyatakan siap rilis penuh**.

