# Laporan Remediasi Audit Codebase

**Tanggal verifikasi:** 25 September 2026  
**Acuan:** `2026-09-24-codebase-cleanliness-modularity-audit.md` dan `2026-09-25-audit-follow-up.md`  
**Status:** remediasi prioritas selesai di working tree; belum dianggap aktif di remote sebelum commit, push, dan workflow GitHub selesai.

## Ringkasan

Seluruh blocker P1 dan kekurangan pipeline P2 yang ditemukan pada audit ulang telah diperbaiki dan diverifikasi secara lokal. Rangkaian E2E lengkap sekarang hijau pada desktop dan mobile. Kontrak Compose untuk mode local dan Google Drive sudah konsisten, file environment berisi secret sudah di-ignore, dan CI kini mencakup integration test dengan PostgreSQL, lint, type-check, unit test, build, sinkronisasi bundle, pemeriksaan format Go, serta Playwright E2E.

Satu bug aplikasi nyata juga ditemukan ketika memperbaiki E2E distribusi: dialog distribusi hanya membaca opsi mesin/selang dari objek template yang tersemat pada jadwal. API jadwal tidak selalu mengirim objek tersebut. UI kini mengambil daftar package template dan menggunakan `package_template_version_id` sebagai fallback, sehingga opsi alat tetap tersedia.

## Status temuan

| Temuan | Remediasi | Status |
|---|---|---|
| Helper navigasi E2E gagal pada grup sidebar tertutup | Helper bersama memakai pemetaan item-ke-grup, membuka grup melalui tombol berlabel, dan menangani menu profil desktop/mobile | Selesai |
| Alur E2E DCP3 tidak sesuai UI terbaru | Skenario diperbarui untuk alur tiga posisi, validasi penerima historis, empat evidence, dan penyelesaian distribusi | Selesai |
| Assertion audit rapuh saat data melebihi halaman pertama | E2E memfilter `user.updated` sebelum melakukan assertion | Selesai |
| `.env.compose` dapat ikut ter-commit | Ditambahkan ke `.gitignore`; hanya `.env.compose.example` yang dilacak | Selesai |
| Kontrak deployment GDrive tidak konsisten | Override `docker-compose.gdrive.yml`, contoh env, serta seluruh perintah start/redeploy terkait diselaraskan | Selesai |
| Lint tidak mencakup E2E | Script lint mencakup `src` dan `e2e` | Selesai |
| CI tidak menjalankan integration/E2E test | PostgreSQL service dan job Playwright ditambahkan; database CI diselaraskan ke `konkit_test` | Selesai |
| Tidak ada pemeriksaan formatter | CI gagal bila `gofmt -l .` menemukan file Go yang belum diformat | Selesai |
| Bundle tracked dapat tertinggal dari source | CI membangun ulang lalu memeriksa `git diff --exit-code -- ../web/static/app` | Selesai |
| Integration test tidak repeatable | Fixture memakai identitas unik dan assertion diselaraskan dengan kontrak repository saat ini | Selesai |
| Redirect logout test tertinggal | Expected URL diperbarui ke `/login?notice=logged_out` | Selesai |

## Hasil verifikasi lokal

| Pemeriksaan | Hasil |
|---|---|
| `go test -p 1 ./...` dengan PostgreSQL | Lulus |
| `go vet ./...` | Lulus |
| `npm run typecheck` | Lulus |
| `npm run lint` | Lulus, 0 error dan 4 warning baseline |
| Unit/component test frontend | 32 file, 140 test lulus |
| Build produksi frontend | Lulus |
| Playwright E2E lengkap | 17 lulus, 3 dilewati berdasarkan viewport, 0 gagal |
| Compose base | Valid |
| Compose base + GDrive override | Valid |
| Audit dependency produksi npm | 0 vulnerability yang dilaporkan |

Bundle aktif setelah build:

- CSS: `index-BvDmvzLq.css` (117,53 kB)
- JavaScript: `index-Dvymd4jC.js` (746,25 kB; gzip 228,20 kB)

## Pekerjaan lanjutan non-blocking

Hal berikut sengaja tidak digabungkan ke remediasi ini karena membutuhkan perubahan arsitektur atau review terpisah:

1. Pecah bundle utama dengan route-level lazy loading. CI sekarang memberi warning pada chunk JavaScript di atas 500 kB, tetapi belum menjadikannya hard gate.
2. Pecah file hotspot seperti repository administrasi dan registrasi route secara bertahap agar risiko regresinya terkendali.
3. Tinjau dan selesaikan empat warning lint baseline: satu penggunaan `any` dan tiga dependency React Hook.
4. Pasang `govulncheck` dan tambahkan audit dependency terjadwal.
5. Evaluasi strategi session/storage hanya ketika aplikasi benar-benar akan dijalankan dalam beberapa replica.

## Kriteria integrasi

Remediasi lokal siap di-commit. Status akhir baru dapat disebut aktif di branch utama setelah:

1. perubahan di-commit dan di-push;
2. workflow GitHub Actions berjalan hijau pada commit tersebut;
3. bundle hasil build ikut dalam commit yang sama dengan source frontend.
