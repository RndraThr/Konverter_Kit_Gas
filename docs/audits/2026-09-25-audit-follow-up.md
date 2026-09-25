# Audit Ulang Perbaikan Temuan 24 September 2026

> **Catatan status:** Dokumen ini merekam kondisi sebelum remediasi lanjutan. Blocker yang dijelaskan di bawah sudah ditangani; hasil terbaru tersedia di `2026-09-25-audit-remediation-report.md`.

**Tanggal verifikasi:** 25 September 2026  
**Basis repository:** `main` pada commit `9a572b0`  
**Kondisi perubahan:** seluruh perbaikan yang diaudit masih berada di working tree dan belum di-commit/push  
**Dokumen acuan:** `docs/audits/2026-09-24-codebase-cleanliness-modularity-audit.md`

## Kesimpulan singkat

Sebagian besar file yang disebutkan dalam ringkasan perubahan memang tersedia dan beberapa perbaikannya bekerja. Namun, status keseluruhan **belum dapat dinyatakan selesai**.

Temuan terpenting:

1. Perbaikan helper navigasi E2E belum berhasil; hasil tetap 13 lulus, 4 gagal, 3 dilewati.
2. `.env.compose` belum masuk `.gitignore`, padahal file tersebut menyimpan password PostgreSQL.
3. Konfigurasi storage pada contoh environment dan README bertentangan: `.env.staging.example` menggunakan `gdrive`, tetapi perintah default menjalankan Compose tanpa override token Google Drive.
4. CI baru mencakup pemeriksaan unit/static/build; belum mencakup E2E, PostgreSQL integration test, formatter, atau verifikasi sinkronisasi bundle.
5. Semua perubahan masih lokal dan belum aktif di branch remote/GitHub Actions.

Bundle frontend, type-check, lint, unit test, Go test, Go vet, serta validasi sintaks Compose berhasil.

## Matriks klaim dan hasil audit

| Klaim perubahan | Hasil audit ulang | Status |
|---|---|---|
| Helper navigasi E2E selesai | Empat test yang sama masih timeout karena locator link tersembunyi tidak dapat menemukan ancestor grup | **Belum selesai** |
| Bundle frontend sinkron | Build ulang menghasilkan hash yang sama dengan file baru: `index-BvDmvzLq.css` dan `index-PKDKclwp.js` | **Selesai secara lokal** |
| Konfigurasi Compose selesai | Base dan override lolos `docker compose config`, tetapi ada risiko secret dan kontradiksi dokumentasi/default storage | **Parsial** |
| Otomasi kualitas selesai | Type-check, lint, unit test, build, dan Go checks tersedia; E2E/integration/format/bundle-sync belum ada di CI | **Parsial** |
| ESLint strict | Konfigurasi memakai `tseslint.configs.recommended`, bukan `strict`/`strictTypeChecked`; script hanya lint `src` | **Klaim tidak tepat** |
| `.gitattributes` selesai | File dan aturan tersedia, tetapi belum di-commit; working tree masih memuat CRLF pada sejumlah file | **Selesai secara konfigurasi, belum terintegrasi** |
| Code splitting | Belum dilakukan; bundle utama masih 746,09 kB | **Belum dilakukan, sesuai catatan** |
| Pemecahan hotspot | Belum dilakukan | **Belum dilakukan, sesuai catatan** |
| Migrasi scale-out | Belum dilakukan | **Belum dilakukan, sesuai catatan** |

## Hasil verifikasi aktual

| Pemeriksaan | Hasil |
|---|---|
| `go test ./...` | Lulus |
| `go vet ./...` | Lulus |
| `npm run typecheck` | Lulus |
| `npm run lint` | Lulus dengan 0 error dan 4 warning |
| `npm test -- --run --maxWorkers=1` | 32 file dan 139 test lulus |
| `npm run build` | Lulus; 2.297 module ditransformasi |
| Bundle produksi | JS 746,09 kB, gzip 228,17 kB; Vite tetap memberi peringatan chunk >500 kB |
| `npm run e2e` | 13 lulus, 4 gagal, 3 dilewati |
| Compose base | `config --quiet` lulus saat file environment tersedia |
| Compose + GDrive override | `config --quiet` lulus dengan path token terisi |
| `npm audit --omit=dev --audit-level=high` | 0 vulnerability dependency produksi |
| `govulncheck ./...` | Belum dijalankan karena tool tidak terpasang |
| `git diff --check` | Menemukan blank line tambahan di akhir `docker-compose.yml` |

## Temuan prioritas

### P1 — Perbaikan E2E belum bekerja

Empat kegagalan tetap sama:

- `administration foundation journey` pada desktop dan mobile.
- `DCP3 to completed package distribution` pada desktop dan mobile.

Kode saat ini melakukan:

```ts
const link = page.getByRole('link', { name: label, exact: true });
if (!await link.isVisible()) {
  await link.locator('xpath=ancestor::section').getByRole('button').first().click();
}
```

Saat grup ditutup, subtree memakai atribut HTML `hidden`. Secara default `getByRole` hanya mencari elemen dalam accessibility tree, sehingga locator `link` tidak memiliki hasil. Mencari `ancestor::section` dari locator kosong akan menunggu sampai timeout 30 detik.

Perbaikan yang disarankan:

- Buat pemetaan label item ke nama grup, misalnya `Persiapan program -> Operasional` dan `Pengguna -> Administrasi`.
- Klik tombol grup secara langsung lewat role dan nama ketika link belum terlihat.
- Setelah grup terbuka, baru cari dan klik link.
- Alternatif teknis adalah mencari link dengan `{ includeHidden: true }`, tetapi pemetaan grup dengan locator berbasis role lebih mudah dibaca dan lebih dekat dengan perilaku pengguna.
- Satukan helper navigasi di satu utility E2E agar tidak ada dua implementasi yang dapat berbeda.

Komentar “fail immediately” juga tidak akurat karena `expect(...).toBeVisible()` tetap memakai timeout assertion bawaan kecuali timeout eksplisit diberikan.

### P1 — `.env.compose` belum dilindungi `.gitignore`

README menyatakan `.env.compose` tidak pernah masuk Git, tetapi `.gitignore` hanya mencakup `.env` dan `.env.staging`. Pemeriksaan `git check-ignore .env.compose` mengonfirmasi file tersebut **tidak di-ignore**.

Ini penting karena `.env.compose` berisi `POSTGRES_PASSWORD`. Tambahkan setidaknya:

```gitignore
.env.compose
```

Tetap lacak `.env.compose.example`.

### P1 — Default storage dan perintah deployment saling bertentangan

Kondisi saat ini:

- `.env.staging.example` menetapkan `STORAGE_BACKEND=gdrive`.
- README menyebut mode local sebagai default.
- Perintah utama `docker compose --env-file .env.compose up -d --build` tidak menyertakan `docker-compose.gdrive.yml`.
- Token OAuth hanya dimount oleh `docker-compose.gdrive.yml`.

Jika operator menyalin `.env.staging.example` lalu menjalankan perintah default README, aplikasi masuk mode GDrive tetapi file token `/run/secrets/gdrive-oauth-token.json` tidak dimount. Startup atau akses storage akan gagal.

Pilih satu kontrak yang konsisten:

1. Ubah `.env.staging.example` menjadi `STORAGE_BACKEND=local` dan kosongkan nilai GDrive sebagai default aman; atau
2. Pertahankan GDrive sebagai default staging, tetapi semua perintah start/redeploy yang relevan wajib memakai kedua file Compose.

Perintah update/redeploy dan varian VPS nginx juga belum menjelaskan kapan harus menambahkan `-f docker-compose.gdrive.yml`. Jika deployment GDrive awal memakai override lalu redeploy hanya memakai file base, service app dapat dibuat ulang tanpa mount token.

### P2 — Dokumentasi Compose masih memiliki teks lama

Beberapa bagian belum diselaraskan:

- README masih mengatakan service `app` selalu me-mount token Google Drive.
- README mengatakan `GDRIVE_OAUTH_TOKEN_HOST_PATH` disubstitusikan di `docker-compose.yml`, padahal sekarang digunakan di file override.
- `.env.compose.example` merujuk `docker-compose.override.yml` dan instruksi “hapus komentar”, sedangkan implementasi baru memakai `docker-compose.gdrive.yml`.
- Komentar `docker-compose.gdrive.yml` mengatakan “menggantikan caddy”, padahal file tersebut menambah mount token dan tidak mengganti Caddy.

### P2 — CI belum memenuhi baseline audit sebelumnya

Workflow saat ini menjalankan:

- Go vet.
- Go unit test dengan `-short`.
- TypeScript type-check.
- ESLint.
- Unit test frontend.
- Build frontend.
- Informasi ukuran bundle.

Yang masih belum ada:

- Playwright E2E dengan PostgreSQL dan migrasi nyata.
- Integration test Go yang membutuhkan database.
- Formatter check.
- Pemeriksaan bahwa hasil build tracked sama dengan source.
- Audit dependency terjadwal.

Pemeriksaan bundle hanya mengirim warning jika chunk lebih dari 800 kB. Bundle saat ini 746,09 kB, sehingga CI tidak memberi warning meskipun Vite sudah memperingatkan batas 500 kB. Ini boleh menjadi batas sementara, tetapi bukan quality gate untuk masalah bundle yang dicatat audit.

Workflow juga belum aktif di GitHub karena file masih untracked dan belum di-push.

### P2 — Cakupan dan klaim ESLint belum tepat

`eslint.config.js` memiliki blok untuk `e2e`, tetapi script package menjalankan:

```json
"lint": "eslint src --max-warnings 5"
```

Akibatnya folder `e2e` tidak diperiksa oleh CI. Pemeriksaan manual `npm exec eslint e2e` memang lulus saat audit ini, tetapi itu belum menjadi bagian pipeline.

Selain itu, konfigurasi memakai `tseslint.configs.recommended`, bukan konfigurasi strict type-checked. Penyebutan “TypeScript strict rules” pada ringkasan perubahan perlu diganti menjadi “recommended rules dengan parser project-aware”, atau konfigurasinya benar-benar dinaikkan ke strict secara bertahap.

Empat warning saat ini:

- Satu `no-explicit-any` pada komponen select.
- Tiga warning dependency React Hook pada halaman aktivitas, dialog role, dan dialog user.

Warning hook sebaiknya ditinjau secara fungsional, bukan hanya dibiarkan sampai batas warning terlampaui, karena stale closure dapat menyebabkan perilaku UI yang salah.

### P2 — Perubahan belum terintegrasi

`git status` menunjukkan seluruh file audit/perbaikan masih modified atau untracked pada branch `main`, sedangkan `HEAD` masih sama dengan `origin/main` di `9a572b0`.

Konsekuensinya:

- CI belum berjalan di remote.
- `.gitattributes` belum berlaku bagi checkout anggota tim lain.
- Bundle baru belum tersedia bagi deployment berbasis Git.
- Perbaikan yang sudah benar masih dapat hilang atau bercampur dengan perubahan berikutnya.

Sebelum commit, jangan masukkan klaim “E2E selesai”; perbaiki dan jalankan ulang terlebih dahulu.

### P3 — Kebersihan tambahan

- `git diff --check` menemukan blank line tambahan di akhir `docker-compose.yml`.
- `go test ./...` pada working tree lokal juga melihat package Go di dalam `frontend/node_modules/flatted/golang/pkg/flatted`. Test tetap lulus, tetapi glob package backend menjadi terkontaminasi dependency frontend lokal. CI backend yang belum menjalankan `npm ci` tidak terkena. Ini risiko rendah, tetapi dapat dipertimbangkan saat merapikan struktur/module boundary.
- `.gitattributes` sudah menetapkan LF, tetapi file working tree yang ada masih CRLF sampai dinormalisasi/staged ulang. Lakukan normalisasi dalam commit yang dapat direview dengan jelas.

## Temuan yang benar-benar sudah tervalidasi

- Bundle frontend baru sesuai dengan hasil build source saat ini.
- File lama dan manifest telah diarahkan ke hash baru di working tree.
- TypeScript type-check lulus.
- ESLint source lulus dengan 0 error dan 4 warning.
- Semua 139 unit/component test frontend lulus.
- Go test dan Go vet lulus.
- Compose base dan override GDrive valid secara sintaks saat environment yang dibutuhkan tersedia.
- Validasi variabel wajib Compose bekerja pada level konfigurasi.
- Dependency produksi npm tidak memiliki advisory yang dilaporkan saat pemeriksaan.
- `.gitattributes` memiliki pemisahan aturan text/binary yang masuk akal sebagai baseline.

## Urutan tindak lanjut yang disarankan

### Harus sebelum commit/push perbaikan audit

1. Tambahkan `.env.compose` ke `.gitignore`.
2. Perbaiki helper E2E berdasarkan tombol grup, lalu jalankan ulang seluruh 20 skenario.
3. Selaraskan default storage antara `.env.staging.example`, README, base Compose, dan override GDrive.
4. Perbaiki instruksi redeploy untuk mode GDrive dan varian nginx.
5. Rapikan teks lama di README dan `.env.compose.example`.
6. Hapus trailing blank line yang terdeteksi `git diff --check`.

### Setelah P1 hijau

1. Perluas lint agar mencakup `src` dan `e2e`.
2. Tambahkan job E2E dengan PostgreSQL ke CI.
3. Tambahkan formatter check dan bundle-sync check jika `web/static/app` tetap dilacak.
4. Commit/push, lalu pastikan workflow GitHub benar-benar hijau.

### Tetap menjadi pekerjaan lanjutan

- Code splitting route dengan `React.lazy`/dynamic import.
- Pemecahan file hotspot secara bertahap.
- Strategi migrasi terpisah saat aplikasi memakai multi-replica.
- Instal dan jalankan `govulncheck` sebagai baseline keamanan dependency Go.

## Status akhir audit ulang

**Belum siap dinyatakan seluruhnya selesai.**

Kualitas dasar aplikasi tetap baik dan pemeriksaan unit/build utama hijau. Namun, dua hal harus dianggap blocker sebelum perubahan ini diintegrasikan: E2E masih merah dan `.env.compose` yang berisi secret belum di-ignore. Kontrak mode storage/deployment juga harus diselaraskan agar petunjuk operasional tidak menghasilkan container GDrive tanpa token mount.
