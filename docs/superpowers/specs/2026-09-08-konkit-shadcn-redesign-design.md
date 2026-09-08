# Konkit Shadcn Redesign Design

## Ringkasan

Redesign seluruh antarmuka Konkit menjadi design system yang konsisten berbasis Shadcn, tanpa mengubah kontrak API, permission, istilah bisnis, atau urutan workflow yang sudah berjalan. Tampilan mempertahankan identitas hijau–emas dan kedua logo, dengan pendekatan visual **Operational Editorial**: korporat, tenang, mudah dipindai, dan efisien untuk pekerjaan data yang padat pada desktop maupun perangkat mobile.

## Tujuan

- Menyatukan seluruh pola UI melalui primitive Shadcn dan token desain bersama.
- Meningkatkan hierarki informasi, keterbacaan, konsistensi, dan aksesibilitas.
- Menjadikan alur operasional kompleks—khususnya DCP3 dan pendistribusian—lebih mudah dipahami.
- Menyediakan pengalaman desktop dan mobile yang sengaja dirancang, bukan sekadar mengecilkan layout desktop.
- Mempertahankan seluruh perilaku bisnis, integrasi backend, RBAC, dan selector aksesibilitas yang digunakan pengujian.

## Batasan Ruang Lingkup

Redesign mencakup login, App Shell, dashboard, persiapan program, DCP3, pendistribusian, laporan, pengguna, role dan akses, pengaturan, kesehatan sistem, riwayat aktivitas, profil, perubahan password, dialog, form, tabel, filter, pagination, serta seluruh loading/error/empty/success state.

Redesign tidak mencakup perubahan endpoint, bentuk payload, query key, schema database, aturan validasi bisnis, permission, proses autentikasi, atau urutan workflow. Fitur baru yang tidak diperlukan untuk menyelesaikan redesign tidak ditambahkan.

## Arah Visual

### Konsep

Operational Editorial memakai struktur halaman yang tegas, ritme spacing yang stabil, permukaan yang tenang, dan aksen warna yang fungsional. Informasi operasional menjadi pusat perhatian; dekorasi tidak boleh bersaing dengan data.

Kartu tidak digunakan untuk membungkus setiap bagian. Card dipakai hanya untuk unit informasi yang memang berdiri sendiri, sementara tabel, toolbar, dan section panjang dapat menggunakan divider serta perubahan permukaan yang lebih ringan.

### Palet inti

- `forest`: `#173D2B` — navigasi, identitas, dan teks berkontras tinggi.
- `leaf`: `#2F7D4A` — tindakan utama, focus state, dan status positif.
- `gold`: `#C49A3A` — aksen terpilih, indikator aktif, dan konteks penting.
- `canvas`: `#F6F7F2` — latar utama yang hangat.
- `surface`: `#FFFFFF` — dialog, table surface, dan area input.
- `line`: `#DDE4DA` — divider dan border netral.

Semantic token untuk destructive, warning, info, muted, ring, input, popover, dan chart diturunkan dari palet ini dengan rasio kontras yang tetap terbaca. Emas tidak menjadi warna tombol utama dan hanya digunakan sebagai aksen selektif.

### Tipografi

Antarmuka menggunakan keluarga sans-serif yang bersih dan tersedia secara lokal atau melalui dependency proyek tanpa ketergantungan runtime eksternal. Hierarki dibangun melalui ukuran, bobot, dan line-height; label dekoratif all-caps tidak digunakan. Nilai NIK, kode dokumen, nomor distribusi, jumlah, dan angka laporan memakai angka tabular.

Panjang teks deskriptif dibatasi agar mudah dipindai. Copy menggunakan bahasa Indonesia aktif, ringkas, dan konsisten dengan istilah bisnis yang sudah ada.

### Bentuk dan gerak

Radius bersifat moderat, bukan pill secara menyeluruh. Bayangan digunakan terutama pada popover, dialog, sheet, dan elemen yang benar-benar mengambang. Interaksi memiliki transisi singkat yang menjelaskan perubahan state; animasi non-esensial menghormati `prefers-reduced-motion`.

## Arsitektur UI

### Fondasi

Tailwind dan konfigurasi Shadcn ditambahkan ke frontend Vite yang ada. CSS variables menjadi sumber token semantic. Utility penggabungan class disediakan melalui `cn()`. Primitive UI disimpan pada `src/components/ui`, sedangkan komponen yang memahami domain Konkit tetap berada pada komponen bersama atau folder feature.

Komponen feature tidak boleh mengulang implementasi button, input, dialog, sheet, select, tabs, dropdown, badge, alert, skeleton, tooltip, atau toast. Primitive Shadcn boleh dibungkus oleh komponen domain bila diperlukan untuk menyederhanakan API penggunaan.

### App Shell

Desktop memakai sidebar tetap selebar sekitar 264 px, topbar ringkas yang sticky, dan area konten dengan batas lebar sekitar 1440 px. Sidebar mengelompokkan navigasi berdasarkan fungsi dan mempertahankan penyaringan berdasarkan permission.

Pada tablet dan mobile, sidebar diganti menjadi Sheet. Header tetap terlihat dan menyediakan judul halaman, tombol navigasi, serta menu akun. Fokus dikembalikan dengan benar setelah Sheet atau menu ditutup.

Struktur dasar halaman konsisten:

1. Judul dan deskripsi.
2. Konteks, filter, atau tindakan utama.
3. Area kerja atau data utama.
4. Tindakan lanjutan dan feedback state.

### Primitive dan pola bersama

- Button: default, secondary, outline, ghost, destructive, icon, loading, dan disabled.
- Form: Label, Input, Textarea, Checkbox, Select, field description, field error, dan grouping.
- Overlay: Dialog untuk form terfokus, Sheet untuk navigasi/filter mobile, Dropdown Menu untuk tindakan ringkas, Alert Dialog untuk tindakan destruktif.
- Data display: Table, Badge, Card, Separator, Tooltip, Skeleton, Alert, dan Pagination.
- Navigation: Tabs, Breadcrumb bila konteks berlapis membutuhkannya, dan Stepper domain untuk DCP3.
- Feedback: toast untuk hasil tindakan singkat; Alert untuk masalah yang perlu dibaca atau ditindaklanjuti; empty state selalu menjelaskan langkah berikutnya.

## Rancangan Halaman

### Login

Foto lapangan dan kedua logo dipertahankan. Desktop memakai komposisi dua panel dengan fotografi sebagai elemen karakter utama dan form sebagai area tenang. Mobile menampilkan foto lebih ringkas di atas form. Input, checkbox, error state, loading button, focus ring, dan spacing mengikuti design system dashboard.

### Dashboard

Dashboard menampilkan ringkasan pengguna, status operasional, aktivitas terbaru, dan akses cepat ke pekerjaan utama. Metrik disusun sebagai unit yang mudah dibandingkan tanpa menjadikan semua konten kartu dekoratif. Status sistem memiliki indikator yang jelas dan tidak bergantung pada warna saja.

### Persiapan Program

Kabupaten, program, jadwal, template paket, dan template dokumentasi menggunakan Tabs responsif serta pola section yang sama. Toolbar, dialog, field berulang, kode dokumen, status publikasi, dan tindakan baris distandarkan. Tab dapat digulir secara horizontal pada layar sempit tanpa menyembunyikan label aktif.

### DCP3

Empat tahap impor ditampilkan sebagai Stepper yang menunjukkan tahap aktif, selesai, dan akan datang. Konteks jadwal tetap terlihat selama proses. Dropzone, pemilihan baris header, pemetaan kolom, preview validasi, dan hasil impor memiliki feedback yang eksplisit. Desktop mengutamakan visibilitas data; mobile menumpuk kontrol dan menjaga tombol kembali/lanjut mudah dijangkau.

### Pendistribusian

Pencarian penerima menjadi fokus awal. Setelah penerima dipilih, halaman memisahkan identitas, kelayakan, verifikasi, detail perlengkapan, sumber data, dokumentasi, dan finalisasi ke dalam section yang mudah dipindai. Status blokir penerimaan ulang dan kelengkapan wajib tidak hanya ditunjukkan lewat warna.

Dokumentasi foto menggunakan grid responsif, preview yang stabil, dan kontrol kamera/galeri yang cukup besar untuk sentuhan. Mobile menampilkan satu alur vertikal dan tombol tindakan dengan lebar penuh bila ruang terbatas.

### Laporan dan administrasi

Laporan, pengguna, role, audit, pengaturan, dan kesehatan sistem memakai bahasa visual yang sama untuk filter, tabel, pagination, badge, dialog, dan feedback. Tabel sederhana tetap menjadi tabel dengan horizontal scroll pada mobile. Data yang membutuhkan tindakan per item dapat memakai ringkasan baris responsif selama makna dan kontrolnya tetap setara.

### Profil

Profil dan perubahan password memakai form dengan lebar baca yang terkendali. Metadata akun, permission, bantuan field, validation state, dan tindakan simpan memiliki hierarki yang jelas.

## Perilaku Responsif

### Desktop

- Target validasi utama: `1366×768`.
- Sidebar sekitar 264 px; konten menggunakan ruang tersisa dengan batas lebar sekitar 1440 px.
- Toolbar dapat menyimpan filter dan tindakan pada satu baris selama ruang mencukupi.
- Tabel mempertahankan kepadatan yang sesuai untuk pekerjaan administratif.

### Tablet

- Sidebar berubah menjadi Sheet.
- Grid dua atau tiga kolom berangsur menjadi dua atau satu kolom berdasarkan kebutuhan konten.
- Toolbar boleh membungkus dan filter sekunder dapat ditempatkan dalam Sheet.

### Mobile

- Target validasi utama: `375×812`.
- Header sticky, navigasi melalui Sheet, dan konten memakai padding ringkas.
- Target sentuh minimal 44 px untuk kontrol utama.
- Tindakan primer dapat memenuhi lebar container.
- Form multi-kolom menjadi satu kolom.
- Dialog besar berubah menjadi layout yang tetap dapat digulir atau Sheet bila lebih sesuai.
- Tabel kompleks menyediakan horizontal scroll dengan petunjuk visual, atau representasi ringkas jika tindakan baris lebih penting daripada perbandingan kolom.
- Tidak boleh ada overflow halaman horizontal yang tidak disengaja.

## Aksesibilitas

- Elemen interaktif menggunakan primitive semantik dan dapat dioperasikan dengan keyboard.
- Focus ring terlihat pada seluruh tema dan permukaan.
- Dialog, Sheet, Dropdown, dan Alert Dialog memiliki focus management yang benar.
- Status tidak disampaikan melalui warna saja; teks atau ikon dengan label turut digunakan.
- Label form terhubung dengan input, error dikaitkan melalui atribut ARIA, dan loading state diumumkan bila relevan.
- Kontras teks dan kontrol memenuhi standar antarmuka yang dapat dibaca.
- Animasi tidak esensial dinonaktifkan pada reduced motion.

## Data Flow dan Kompatibilitas

React Query, fungsi API, query key, route, permission guard, CSRF, dan form action autentikasi dipertahankan. Redesign mengubah presentasi dan komposisi komponen, bukan aturan data.

Selector berbasis role dan accessible name yang digunakan Testing Library serta Playwright dipertahankan sejauh copy tidak perlu diperjelas. Jika struktur DOM berubah, hasil interaksi dan nama kontrol tetap setara sehingga alur E2E desktop dan mobile terus berlaku.

## Error, Loading, dan Empty State

- Loading awal shell menggunakan skeleton atau status terpusat yang informatif.
- Loading lokal mempertahankan layout agar konten tidak meloncat secara berlebihan.
- Error menjelaskan apa yang gagal dan tindakan yang dapat dilakukan, seperti mencoba kembali.
- Empty state membedakan antara belum ada data, hasil filter kosong, dan akses yang tidak tersedia.
- Success feedback tidak menghalangi pengguna melanjutkan pekerjaan.
- Disabled state selalu memiliki alasan yang dapat dipahami dari konteks atau bantuan teks.

## Strategi Migrasi

1. Tambahkan Tailwind, Shadcn, token, dan utility bersama.
2. Bangun primitive UI serta App Shell baru.
3. Migrasikan komponen bersama: form, dialog, badge, table, pagination, loading, error, dan empty state.
4. Migrasikan kelompok halaman dalam urutan: autentikasi, dashboard, administrasi, persiapan program, DCP3, pendistribusian, dan laporan.
5. Hapus CSS lama hanya setelah seluruh pemakai telah dipindahkan dan verifikasi menunjukkan tidak ada regresi.

Migrasi tidak berjalan sebagai campuran visual permanen. Setiap tahap boleh sementara memakai adapter class, tetapi hasil akhir harus menggunakan token dan primitive bersama secara konsisten.

## Pengujian dan Kriteria Penerimaan

- Seluruh unit/component test yang ada tetap lulus.
- Build produksi Vite berhasil tanpa warning atau import yang hilang.
- E2E administrasi serta DCP3–pendistribusian lulus pada desktop `1366×768` dan mobile `375×812`.
- Route dan item navigasi tetap mengikuti permission.
- Login, logout, dropdown akun, mobile navigation, dialog, tabs, filter, pagination, upload, pemetaan, preview, penyimpanan draft, upload media, dan finalisasi dapat digunakan dengan keyboard dan pointer.
- Tidak ada overflow horizontal halaman yang tidak disengaja pada viewport target.
- Loading, error, empty, disabled, success, focus, dan reduced-motion diperiksa pada pola bersama.
- Screenshot desktop dan mobile ditinjau untuk konsistensi alignment, spacing, hierarchy, wrapping, dan touch target.

## Risiko dan Mitigasi

- **Regresi perilaku akibat perubahan struktur DOM:** pertahankan handler, API, dan accessible name; migrasikan per kelompok dengan test berjalan.
- **Campuran gaya lama dan baru:** pusatkan token dan primitive lebih awal, kemudian hapus selector lama setelah semua pemakai berpindah.
- **Tabel tidak nyaman di mobile:** tentukan per halaman apakah perbandingan kolom atau tindakan per item yang lebih penting; gunakan scroll atau ringkasan secara sengaja.
- **Bundle bertambah:** pilih hanya primitive Shadcn yang benar-benar digunakan dan hindari library visual tambahan yang tumpang tindih.
- **Dependency lokal tidak lengkap:** lakukan instalasi deterministik melalui lockfile sebelum baseline dan verifikasi akhir.
