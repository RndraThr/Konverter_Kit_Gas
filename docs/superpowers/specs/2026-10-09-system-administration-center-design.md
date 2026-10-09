# Pusat Administrasi Sistem

Tanggal: 9 Oktober 2026  
Status: Menunggu review pengguna

## Tujuan

Meningkatkan tiga halaman sistem—Kesehatan Sistem, Pengaturan Sistem, dan Riwayat Aktivitas—menjadi pusat administrasi operasional yang mudah dipahami pada desktop maupun mobile. Implementasi harus memakai data dan izin yang sudah ada, menambahkan diagnosis yang relevan tanpa tindakan berbahaya, dan mempertahankan audit sebagai data read-only.

## Batasan

- Tidak menambah tabel database baru.
- Tidak menyediakan tindakan untuk menghapus atau mengubah audit.
- Tidak menyediakan kontrol untuk menghentikan worker, mengubah database, atau menghapus antrean media.
- Tidak menampilkan credential, token, storage key, atau database URL.
- IP address dan user-agent hanya muncul di panel detail audit, bukan di daftar utama.
- Hak akses yang sudah ada tetap digunakan: `health.view`, `settings.view`, `settings.manage`, dan `audit.view`.
- Bahasa antarmuka tetap Bahasa Indonesia dan identitas visual hijau Ergas/KSM dipertahankan.

## Arsitektur

Perubahan menggunakan modul yang sudah ada:

- `internal/health` diperluas dengan statistik operasional read-only dari PostgreSQL dan informasi konfigurasi storage yang aman.
- `internal/settings` tetap memakai registry lima pengaturan yang sudah tersedia. Respons daftar diperkaya dengan nama pengguna terakhir yang memperbarui bila tersedia.
- `internal/audit` memperluas filter query tanpa mengubah skema tabel.
- Endpoint sistem yang sudah ada tetap dipertahankan agar URL dan permission tidak berubah.
- Frontend tetap menggunakan React Query, komponen UI yang sudah tersedia, dan parameter URL untuk filter audit.

Tidak ada dependency frontend atau backend baru.

## Kesehatan Sistem

### Data backend

Laporan kesehatan tetap mengandung status keseluruhan, database, versi migrasi, environment, versi aplikasi, uptime, dan waktu pemeriksaan. Laporan ditambah dengan:

- backend penyimpanan aktif (`local` atau `gdrive`) sebagai informasi non-rahasia;
- status worker pemindahan media (`active` atau `not_applicable`);
- statistik job pemindahan media: queued, processing, dan retry;
- jumlah media gagal dengan `storage_state = move_failed`.

Statistik operasional diambil melalui probe PostgreSQL dengan timeout yang sama seperti pemeriksaan kesehatan. Kegagalan membaca statistik menjadikan status `degraded`, bukan membocorkan error mentah. Kegagalan ping database tetap menghasilkan `unhealthy`.

### Tampilan

- Status keseluruhan dominan di bagian atas, lengkap dengan waktu pemeriksaan dan tombol “Periksa ulang”.
- Metrik utama: database, latensi, migrasi, uptime, dan storage.
- Panel antrean media menampilkan empat status job dan jumlah media gagal.
- Status menggunakan ikon, teks, dan warna sekaligus.
- Refresh otomatis tetap 60 detik dan indikator refresh tidak menggeser layout.
- Pada mobile, metrik menjadi daftar satu kolom tanpa horizontal scroll.

## Pengaturan Sistem

### Data backend

Registry tetap membatasi konfigurasi pada:

- `application_name`;
- `organization_name`;
- `timezone`;
- `date_format`;
- `locale`.

Repository daftar melakukan left join ke tabel pengguna agar dapat mengembalikan `updated_by_name`. Field `updated_by` tetap tersedia untuk kompatibilitas. Update tetap atomik dan menghasilkan satu event `settings.updated` yang memuat nilai sebelum dan sesudah perubahan.

Frontend mengirim hanya nilai yang benar-benar berubah. Validasi dan normalisasi utama tetap berada di service backend.

### Tampilan dan perilaku

- Form dibagi menjadi bagian “Identitas” dan “Regional”.
- Desktop menggunakan area form dan panel pratinjau berdampingan; mobile menumpuk keduanya.
- Pratinjau menunjukkan nama aplikasi, organisasi, serta contoh tanggal/waktu berdasarkan timezone dan format pilihan.
- Halaman mendeteksi dirty state dengan membandingkan draft dan nilai server.
- Tombol “Simpan perubahan” hanya aktif ketika draft valid, berubah, dan mutation tidak berjalan.
- Tombol “Batalkan perubahan” mengembalikan seluruh draft ke nilai server.
- Ringkasan pembaruan terakhir menampilkan waktu dan nama pelaku bila tersedia.
- Pengguna read-only melihat nilai yang sama dalam kontrol disabled dan pesan bahwa perubahan memerlukan izin pengelolaan.
- Navigasi keluar halaman tidak diberi blocker global; dirty state selalu terlihat melalui bilah aksi agar tidak menambah mekanisme routing baru.

## Riwayat Aktivitas

### Kontrak filter

Endpoint daftar audit mempertahankan filter lama dan menambah:

- `query`: pencarian aksi, jenis objek, ID objek, nama, username, atau email pelaku;
- `actor`: pencarian nama, username, atau email pelaku;
- `date_from`: tanggal/waktu awal inklusif;
- `date_to`: tanggal/waktu akhir inklusif;
- `page_size`: 10, 20, 50, atau 100.

Filter `action`, `resource_type`, dan `actor_user_id` tetap didukung. Tanggal tidak valid menghasilkan respons validasi, bukan error database. Query memakai parameter SQL dan tidak menyusun input pengguna ke dalam SQL mentah.

Respons page ditambah ringkasan sesuai filter aktif:

- total hasil;
- jumlah aktivitas hari ini;
- jumlah aktivitas oleh sistem tanpa aktor pengguna.

Tidak diperlukan endpoint opsi filter baru pada tahap ini; filter aksi dan jenis objek tetap berupa input agar mendukung event baru tanpa perubahan frontend.

### Tampilan dan perilaku

- Filter utama: pencarian umum dan tombol “Terapkan”.
- Filter lanjutan: aksi, jenis objek, pelaku, rentang tanggal, dan jumlah per halaman.
- Filter aktif tampil sebagai chip yang dapat dihapus satu per satu, disertai “Reset semua”.
- Aksi umum diterjemahkan menjadi label manusia, tetapi kode aksi asli tetap ditampilkan sebagai informasi sekunder.
- Desktop memakai tabel ringkas: waktu, pelaku, aktivitas, objek, dan aksi detail.
- Mobile memakai kartu vertikal; tabel desktop tidak dirender sebagai permukaan utama pada breakpoint mobile.
- Tombol detail membuka Sheet dari kanan di desktop dan tetap memenuhi lebar aman pada mobile.
- Panel detail menampilkan waktu lengkap, pelaku, kode aksi, resource, ID resource, IP address, user-agent, metadata terstruktur, dan JSON mentah yang dapat diperluas.
- IP dan user-agent tidak tampil di daftar utama.
- Pagination berada di bawah daftar dan menampilkan “Halaman X dari Y”.
- Empty state menjelaskan filter mana yang dapat diubah.

## Bahasa visual

- Menggunakan token tema yang sudah ada; tidak menambah warna hex ad-hoc kecuali status semantik yang sudah tersedia.
- Hijau digunakan untuk sehat/aktif, amber untuk perhatian/retry, dan destructive untuk gagal.
- Hierarki halaman dibentuk dengan garis pemisah, permukaan tenang, tipografi, dan ruang; tidak semua informasi dibungkus kartu identik.
- Ikon tetap dari Lucide agar konsisten dengan aplikasi.
- Tombol dan kontrol interaktif memiliki tinggi minimal 44px.
- Ikon dekoratif diberi `aria-hidden`.
- Fokus keyboard terlihat dan urutan fokus mengikuti urutan visual.
- Transisi menghormati `prefers-reduced-motion`.
- Layout diverifikasi pada lebar 375px dan desktop tanpa horizontal scroll pada konten utama.

## Penanganan status dan error

- Loading, error, empty, dan retry memakai `DataState` yang sudah ada.
- Error health tidak menampilkan error internal atau credential.
- Error penyimpanan settings mempertahankan draft pengguna.
- Error audit mempertahankan parameter filter di URL.
- Refresh health dan refetch audit tidak mengosongkan data lama saat data sebelumnya masih tersedia.

## Pengujian

### Backend

- Health report sehat, degraded, dan unhealthy.
- Statistik antrean media dipetakan dengan benar dan tidak membocorkan error mentah.
- Nama updater settings dikembalikan dan update tetap atomik serta audited.
- Audit search, actor, rentang tanggal, pagination, kompatibilitas `actor_user_id`, dan validasi tanggal.
- Integration test dijalankan pada `konkit_test` dengan migrasi lengkap.

### Frontend

- Health menampilkan status, metrik, antrean, refresh, serta layout semantik.
- Settings mendeteksi perubahan, membatalkan draft, menyimpan hanya nilai berubah, menampilkan pratinjau, dan menangani mode read-only.
- Audit menerapkan parameter URL, menghapus chip filter, merender tabel desktop dan kartu mobile, membuka detail, serta tidak menampilkan IP/user-agent di daftar.
- Typecheck, seluruh Vitest, seluruh Go test, `go vet`, build produksi, dan `git diff --check` harus lulus.

## Di luar cakupan

- Grafik histori kesehatan dan penyimpanan metrik time-series.
- Notifikasi insiden melalui email atau WhatsApp.
- Tindakan restart worker atau retry massal dari halaman health.
- Ekspor audit ke Excel/PDF.
- Penghapusan atau retensi otomatis audit.
- Penambahan setting baru yang belum dikonsumsi modul lain.
