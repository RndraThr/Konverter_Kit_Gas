# Sisa Backlog Konkit (per 22 November 2026)

Dokumen ini menjadi titik awal untuk mengerjakan sisa backlog di sesi Claude Code CLI yang baru. Setiap item memuat empat hal:

- kondisi kode saat ini, dengan path file;
- keputusan yang harus ditanyakan ke pemilik proyek **sebelum** coding;
- langkah kerja;
- definisi selesai.

Di bagian akhir tersedia prompt siap tempel.

Cara pakai:

```
claude
> Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md, lalu kerjakan item <nomor>.
```

Kerjakan satu item per sesi dan per PR.

---

## 0. Aturan kerja (berlaku untuk semua item)

- **Bahasa.** Balasan ke pengguna memakai Bahasa Indonesia. Commit, deskripsi PR, dan komentar kode mengikuti gaya repo, yaitu bahasa Inggris untuk commit/PR dan campuran sesuai file untuk komentar.
- **Fokus web.** Pekerjaan di app mobile (Flutter) sedang ditunda. Jangan ubah kontrak API yang dipakai mobile tanpa konfirmasi. Contohnya `GET /api/v1/reports/schedule/{id}/summary` (`total_allocations`) dan `GET /api/v1/documentation-examples`.
- **Dev server.** Jangan mematikan atau me-restart proses di port `:8080`, karena itu sesi dev milik pengguna. Untuk uji end-to-end, jalankan server sendiri di port lain, misalnya `:8099`, dengan `DATABASE_URL` diarahkan ke `konkit_test`.
- **Database tes.** Integration test memakai `TEST_DATABASE_URL`, yaitu `DATABASE_URL` dari `.env` dengan nama DB diganti `konkit_test`. Test menjalankan `goose.Up` sendiri.
  - Jalankan dengan `-p 1` supaya fixture antar paket tidak bentrok.
  - `administration`, `bast`, dan `geo` bisa gagal di lokal karena sisa data lama di `konkit_test` (misalnya akun `driver-admin`). Itu bukan regresi. CI selalu memakai DB baru.
- **Sebelum PR, jalankan semua test**, bukan hanya paket yang diubah:
  - `go vet ./...` dan `go test -p 1 ./...` (dengan `TEST_DATABASE_URL`);
  - di `frontend/`: `npx vitest run`, `npm run typecheck`, `npx eslint src`.

  Pelajaran dari PR #89: ada test di `internal/api` yang ikut terdampak perubahan di `internal/reports`.
- **Migrasi.** Migrasi baru ditaruh di `internal/database/migrations/NNNNN_nama.sql` (goose, ter-embed). Nomor terakhir saat dokumen ini ditulis adalah `00110`. Jangan menjalankan migrasi ke VPS. Cukup beri tahu pengguna perintahnya.
- **Skema bersih.** Kolom DB yang tidak terpakai langsung dihapus saat mengerjakan fitur terkait.
- **Rahasia.** Jangan pernah mencetak token, password, atau isi `.env`.
- **GitHub.** `gh` tidak terpasang. PR dibuat lewat GitHub API. Token diambil dari `git credential fill` di dalam skrip Python yang dijalankan dengan `python -I`, dan tidak pernah di-echo.
- **Atribusi.**
  - Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Deskripsi PR diakhiri `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- **Branch.** Buat branch baru dari `origin/main` terbaru untuk setiap item. Jangan commit langsung ke `main`.
- **Panduan.** Setiap perubahan perilaku yang terlihat pengguna juga memperbarui panduan di `frontend/src/features/guide/topics/*.tsx`, termasuk FAQ-nya bila perlu.

---

## Item 6. Label dan notifikasi kelengkapan dokumentasi

**Tujuan.** Petugas dan tim pusat langsung tahu nomor bagi mana yang dokumentasinya belum lengkap, tanpa membuka slot satu per satu.

**Kondisi kode saat ini**
- `GET /api/v1/distribution/slots/catalog?schedule_id=` mengembalikan `SlotCatalogEntry` (`internal/distribution/models.go`) dengan field `slot_number`, `status`, `documentation_complete`, `needs_recompletion`, dan `last_activity_at`.
- `frontend/src/features/distribution/SlotCatalogGrid.tsx` sudah menandai slot `completed` yang dokumennya berkurang dengan ikon peringatan dan label "(dokumen berkurang)". Slot `linked` yang belum lengkap belum dibedakan.
- `DocumentationSlot.tsx` menampilkan "Lengkap", "Belum lengkap", atau "Belum diisi" per slot dokumentasi, tapi hanya di dalam detail nomor bagi.
- Notifikasi:
  - `internal/notifications` sudah punya inbox, push FCM, dan kategori seperti `distribution_revision`, yang dipakai monitoring saat meminta revisi.
  - Lonceng notifikasi ada di `frontend/src/features/finance/NotificationBell.tsx`.
- Monitoring pusat (`internal/monitoring`, halaman `frontend/src/features/monitoring`) sudah punya antrean cek foto per kabupaten.

**Keputusan yang harus ditanyakan ke pengguna dulu**
1. Bentuknya: (a) hanya label/badge dan filter di katalog nomor bagi, (b) ditambah notifikasi, atau (c) keduanya. Usulan: mulai dari (a). Notifikasi menyusul.
2. Arti "lengkap": semua slot dokumentasi **wajib** sudah memenuhi `min_files`. Ini sama dengan `documentation_complete` saat ini. Perlu dikonfirmasi apakah slot opsional ikut dihitung.
3. Bila memilih notifikasi:
   - siapa penerimanya (ketua lapangan kabupaten lewat `monitoring_assignments` kind `leader`, petugas yang menyelesaikan slot, atau keduanya);
   - kapan dikirim (misalnya H+1 setelah `distribution_date` dan masih belum lengkap);
   - seberapa sering (sekali per slot, atau ringkasan harian per kabupaten).

**Langkah (opsi a, usulan)**
1. Backend: tambahkan `missing_documents int` (jumlah slot dokumentasi wajib yang belum memenuhi `min_files`) ke `SlotCatalogEntry`. Hitung di query katalog yang sama, tanpa query tambahan per slot.
2. Frontend `SlotCatalogGrid`:
   - slot `linked` atau `completed` yang `missing_documents > 0` mendapat penanda (titik atau angka kecil);
   - `aria-label` dan `title`-nya menyebut "kurang N dokumen".
3. Di atas katalog, tambahkan filter atau toggle "Belum lengkap saja", plus ringkasan "X dari Y nomor bagi dokumennya belum lengkap".
4. Pastikan mobile tidak rusak. Field baru bersifat tambahan, jadi aman untuk JSON.
5. Test:
   - integration test katalog di `internal/distribution`: hitungan `missing_documents` untuk slot yang lengkap, kurang satu, dan opsional kosong;
   - vitest `SlotCatalogGrid` dan halaman distribusi: penanda dan filter.
6. Perbarui panduan `frontend/src/features/guide/topics/pendistribusian.tsx`.

**Langkah tambahan (opsi b, bila disetujui)**
- Tambahkan kategori `distribution_incomplete` di `internal/notifications` beserta daftar `Categories`, supaya bisa di-mute.
- Buat job periodik di server, mengikuti pola `reports.DriveSyncInterval`, yang mengirim notifikasi sesuai aturan yang disepakati. Simpan jejaknya supaya tidak mengirim dobel.

**Selesai bila** katalog menunjukkan slot yang belum lengkap, filter berfungsi, semua test lulus, dan panduan diperbarui.

---

## Item 1. Dashboard grafik khusus super admin

**Tujuan.** Satu halaman ringkasan volume pekerjaan per program untuk super admin.

**Kondisi kode saat ini**
- `frontend/src/features/dashboard/DashboardPage.tsx` saat ini berisi **Data Penerima**, bukan dashboard grafik.
- `internal/monitoring` (PR #84) sudah menghitung per kabupaten:
  - `Overview(programID, date, scope)` dengan `Quota`, `Completed`, `Pending`, dan `Approved`;
  - hari dan tahap jadwal (`schedule_phase_days`) serta Rakornas.

  Dashboard sebaiknya **memakai ulang** query ini, bukan menghitung ulang.
- Sumber data lain:
  - `distribution_slots` (status, `distribution_date`, `completed_at`);
  - `documentation_slots` + `media_files` (foto);
  - tabel BA di `internal/bast` (status tanda tangan);
  - `activity_media` (dokumentasi kegiatan).
- Rute ada di `frontend/src/app/routes.tsx`, menu di `frontend/src/app/AppShell.tsx`. Akses diatur lewat permission di migrasi.

**Rancangan yang pernah diajukan (BELUM disetujui, wajib dikonfirmasi dulu)**
1. Kartu volume keseluruhan: target (kuota), penerima tertaut, diserahkan, bukti lengkap, BA ditandatangani, masing-masing dengan persentase.
2. Bar per kabupaten: target vs progres per tahap.
3. Bar per hari: jumlah penyerahan per tanggal plus garis kumulatif.
4. Bar dokumentasi kegiatan: jumlah foto per jenis.
5. Filter program, hanya terlihat oleh super admin.

**Keputusan yang harus ditanyakan dulu**
- Grafik mana yang dipakai dan apa yang ditambah atau dibuang.
- Letaknya: halaman baru atau tab di Monitoring.
- Siapa yang boleh melihat: hanya super admin, atau juga role `admin` (tim pusat) yang sudah punya `monitoring.view`.

**Langkah (setelah disetujui)**
1. Backend:
   - endpoint `GET /api/v1/dashboard/summary?program_id=`;
   - permission baru, misalnya `dashboard.view`, lewat migrasi dan diberikan ke `super_admin`;
   - query agregat dengan scope kabupaten;
   - integration test dengan fixture kecil.
2. Frontend: pakai library chart yang sudah ada di `frontend/package.json` bila ada. Bila belum ada, tanyakan dulu sebelum menambah dependensi. Ikuti design system di `design-system/konkit/MASTER.md`.
3. Siapkan state kosong, loading, dan error. Pastikan tampil baik di lebar ponsel.
4. Test vitest untuk render dan izin. Perbarui panduan.

**Selesai bila** isi grafik sesuai yang disetujui, akses sesuai izin, dan semua test lulus.

---

## Item 2–4. Laporan Akhir (PPT): layout, kalimat belum terisi, tanggal kegiatan

Ketiganya dikerjakan dalam satu scope, sesuai permintaan pengguna 2026-11-16.

**Kondisi kode saat ini**
- Backend ada di `internal/finalreport`:
  - `service.go` menyusun nilai placeholder;
  - `pptx.go` mengisi template;
  - `preview.go` menyediakan `orBlank`, yang mengganti nilai kosong dengan penanda `blank`;
  - `repository.go` mengambil data sumber.
- Template: `internal/finalreport/template/final_report_template.pptx`, dengan daftar placeholder di `template/placeholders.go`.
- Frontend: `frontend/src/features/finalreport/FinalReportPage.tsx` dan `FinalReportPresent.tsx`.
- Tanggal yang dipakai sekarang, dari `repository.go` `GetPreviewSourceData`:
  - `rakordaDate` dan `sosialisasiDate` diambil dari `bast_rakorda_uploads.event_date`;
  - `distributionStart` dan `distributionEnd` diambil dari `min/max(distribution_slots.completed_at)`, **bukan** dari Berita Acara;
  - tanggal pengiriman (selang, mesin, tabung gas) diisi manual di `final_report_settings`.

**Bahan yang harus diminta ke pengguna dulu**
- **Contoh PPT final** yang dianggap benar, atau catatan per slide tentang apa yang salah di layout saat ini.
- Daftar kalimat yang tampil "belum diisi", atau izin untuk membuat PPT dari data uji lalu mendaftar semua placeholder yang masih `blank`.
- Konfirmasi "tanggal kegiatan dari Berita Acara saja": BA yang mana (BA Perorangan, rekap harian, atau penutupan kabupaten/titik serah di `internal/bast`) dan field tanggal yang mana. Juga apa yang ditampilkan bila BA belum ada: kosong dengan tanda, atau fallback.

**Langkah**
1. Buat PPT dari data uji di `konkit_test`, render, lalu catat semua placeholder yang masih `blank` beserta penyebabnya: data memang belum ada, salah sumber, atau salah nama placeholder.
2. Ganti sumber tanggal distribusi ke tabel BA yang disepakati. Tambahkan test di `repository_integration_test.go` dan `service_test.go`.
3. Rapikan layout sesuai contoh PPT. Bila template `.pptx` diubah, jelaskan di PR slide mana yang berubah. Bila memungkinkan, sertakan screenshot render.
4. Perbarui `pptx_test.go` untuk placeholder yang berubah.

**Selesai bila** PPT dari data lengkap tidak menyisakan penanda kosong, tanggal mengikuti BA, dan layout cocok dengan contoh.

---

## Item 13. WebSocket `/api/v1/realtime` gagal di produksi

**Gejala.** Di browser muncul `WebSocket connection to 'wss://konkit.ptkiansantang.com/api/v1/realtime' failed`.

**Yang sudah diketahui**
- Di lokal, handshake mengembalikan `101 Switching Protocols`. Tanpa header `Upgrade`, server mengembalikan `426`. Jadi aplikasinya benar.
- Contoh konfigurasi nginx di `README.md` (bagian "Varian: VPS bersama dengan nginx yang sudah ada") **sudah** punya `proxy_http_version 1.1`, `Upgrade`, dan `Connection 'upgrade'` di `location /`. Kemungkinan konfigurasi di VPS berbeda dari README, atau ada lapisan lain di depan nginx.

**Langkah diagnosis.** Claude tidak punya akses SSH ke VPS, jadi berikan perintah-perintah ini ke pengguna:
1. Lihat konfigurasi yang aktif:
   ```bash
   sudo nginx -T | sed -n '/server_name konkit.ptkiansantang.com/,/^}/p'
   ```
   Pastikan blok `location /` (atau `location /api/v1/realtime`) memiliki:
   ```nginx
   proxy_http_version 1.1;
   proxy_set_header Upgrade $http_upgrade;
   proxy_set_header Connection "upgrade";
   proxy_read_timeout 1h;
   ```
   Periksa juga blok `listen 443` buatan certbot. Kadang certbot membuat blok terpisah yang tidak ikut berisi header ini.
2. Uji langsung ke container, melewati nginx:
   ```bash
   curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Version: 13" \
     -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" http://localhost:8090/api/v1/realtime
   ```
   Hasil `401` (perlu sesi) atau `101` berarti aplikasi sehat. Uji lagi lewat domain untuk membandingkan.
3. Bila ada Cloudflare atau proxy lain di depan, pastikan WebSocket diaktifkan di sana.
4. Setelah mengubah konfigurasi, jalankan `sudo nginx -t` lalu `sudo systemctl reload nginx`.

**Selesai bila** halaman yang memakai realtime (misalnya Monitoring) tidak lagi menampilkan error WebSocket di console. Bila penyebabnya ternyata perbedaan konfigurasi, perbarui juga README.

---

## Item 11b. Upload ulang file DCP3 yang mirip

**Kebutuhan.** File DCP3 yang sudah diperbaiki, dengan sebagian besar baris sama dan satu atau dua baris berubah, harus bisa diimport ulang. Baris dengan NIK yang sudah ada cukup diperbarui.

**Kondisi kode.** Penolakan "file sama" hanya berdasarkan checksum byte file (`UNIQUE (schedule_id, file_checksum)` di `dcp3_import_batches`, error `ErrDuplicateImport` di `internal/dcp3`). File yang isinya berubah seharusnya sudah lolos, dan barisnya dicocokkan lewat NIK (`matchOrCreatePerson` di `internal/dcp3/repository.go`). Pengecualiannya penerima yang distribusinya sudah berjalan. Sejak PR #86 juga ada fitur hapus import yang salah (`internal/dcp3/removal.go`).

**Langkah**
1. Minta pengguna mencoba dengan file asli. Bila masih ditolak, minta pesan error persisnya.
2. Bila belum ada, tambahkan integration test yang menjalankan import file A, lalu import file A' (satu baris diubah, satu baris ditambah). Test memastikan baris yang diubah diperbarui, baris baru masuk, dan tidak ada orang ganda.
3. Bila ditemukan bug, perbaiki di `internal/dcp3` dan tampilkan ringkasan "diperbarui / baru / dilewati" di hasil import.

**Selesai bila** import ulang file yang diperbaiki berhasil dan hasilnya sesuai, dibuktikan dengan test.

---

## Item kecil

1. **Screenshot panduan Laporan DP3.** Jalankan `frontend/scripts/guide-screenshots.mjs` (lihat isi skrip untuk prasyarat server dan fixture) supaya screenshot menampilkan kolom **Status** (DCP3/Pengganti) yang baru, dari PR #89.
2. **Deploy ke VPS.** Ini dijalankan oleh pengguna, bukan Claude.
   - Migrasi `00097`–`00110` belum dijalankan di VPS. `ENTRYPOINT` container menjalankan migrasi saat `app` start ulang (lihat README). Bila perlu manual, gunakan `go run ./cmd/migrate up`.
   - Setelah deploy, unggah foto contoh dokumentasi di **Pengaturan → Contoh foto dokumentasi** (super admin).
3. **Mobile (ditunda, jangan dikerjakan tanpa konfirmasi).**
   - Tombol "+ Surat Kuasa" di bawah slot KTP (slot `power_of_attorney`, dari PR #79).
   - Tombol contoh foto (memakai `GET /api/v1/documentation-examples`). Ini kabarnya sudah dibuat rekan pengguna, jadi cek dulu sebelum membuat.

---

## Prompt siap tempel

Item 6:
```
Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md bagian "Item 6" dan "0. Aturan kerja".
Tanyakan dulu keputusan yang tercantum (bentuk label/notifikasi, arti "lengkap"), lalu kerjakan
sesuai jawaban saya di branch baru dari origin/main, lengkap dengan test dan panduan, dan buat PR.
```

Item 1:
```
Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md bagian "Item 1" dan "0. Aturan kerja".
Tunjukkan rancangan isi dashboard dan minta persetujuan saya sebelum menulis kode.
Pakai ulang perhitungan internal/monitoring bila cocok.
```

Item 2–4:
```
Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md bagian "Item 2–4" dan "0. Aturan kerja".
Saya lampirkan contoh PPT di <path>. Mulai dengan membuat PPT dari data uji dan mendaftar semua
placeholder yang masih kosong, lalu tanyakan sumber tanggal BA sebelum mengubah kode.
```

Item 13:
```
Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md bagian "Item 13".
Pandu saya mengecek konfigurasi nginx di VPS langkah demi langkah; saya yang menjalankan perintahnya.
```

Item 11b:
```
Baca docs/superpowers/plans/2026-11-22-sisa-backlog.md bagian "Item 11b" dan "0. Aturan kerja".
Tambahkan integration test import ulang file DCP3 yang sudah diperbaiki, lalu laporkan hasilnya.
```
