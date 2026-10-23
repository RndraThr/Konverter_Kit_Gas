# Rancangan Kamera Watermark untuk Web

Versi: 1.0
Tanggal: 2026-10-23
Status: Menunggu review pengguna

## 1. Maksud dan alasan

Petugas lapangan Konkit saat ini memakai dua jalur: app mobile Flutter (`Konverter_Kit_Gas_Mobile`) dan web (`frontend/`). App mobile sudah punya kamera watermark, mengirim lokasi GPS, dan memperkecil foto sebelum upload; web belum. Tujuannya **kesetaraan dan fleksibilitas**: petugas yang memakai browser (HP tanpa app, laptop/tablet di gudang) menghasilkan foto dokumentasi dengan standar yang sama dengan app mobile.

Acuan perilaku: implementasi mobile di `D:\KSM\Deployment\Konverter_Kit_Gas_Mobile\lib\features\camera\` (`watermark_camera_page.dart`, `photo_compose.dart`, `widgets/watermark_widgets.dart`, `widgets/draggable_element.dart`), `lib\data\camera_settings.dart`, `lib\data\media_policy.dart`, `lib\data\location.dart`, dan `lib\features\settings\watermark_settings_page.dart`. Bila rancangan ini diam tentang suatu detail, ikuti perilaku mobile.

## 2. Tujuan

1. **Kamera watermark di browser** — pratinjau kamera langsung dengan overlay, lalu foto disusun ulang di canvas dengan watermark "terbakar" ke dalam gambar sebelum upload.
2. **Isi watermark** (setara mobile):
   - **Logo**, dengan urutan sumber: (a) logo kustom pilihan petugas, (b) logo tender program aktif dari `GET /api/v1/bast/branding?program_id=…`, (c) banner bawaan (aset logo KSM kiri + logo Ergas kanan, sama seperti mobile — ambil berkas asetnya dari `Konverter_Kit_Gas_Mobile/assets/`).
   - **Blok waktu**: tanggal dan jam (berjalan live), alamat hasil reverse geocoding, dan koordinat `Lat: x.xxxxx, Lng: y.yyyyy` (5 desimal), dengan garis aksen di sisi luar.
3. **Pengaturan watermark**, disimpan per perangkat:
   - Tampilkan/sembunyikan Logo dan Blok waktu masing-masing.
   - Logo dan Blok waktu bisa digeser (drag) dan diubah ukurannya (pinch di layar sentuh; slider/tombol di desktop), skala 0,4–3,0.
   - Sisi blok waktu (kiri/kanan), warna teks dan warna aksen dari 5 pilihan mobile (Kuning `#FFEB3B` bawaan teks, Putih `#FFFFFF`, Oranye `#FF6B35` bawaan aksen, Hijau `#4ADE80`, Biru `#60A5FA`), ukuran teks (Kecil 0,85 / Sedang 1,0 / Besar 1,2), skala logo 0,5–1,0, ganti logo kustom / kembalikan ke bawaan, tombol "Bawaan" untuk reset semua.
   - Pratinjau live di halaman pengaturan.
4. **Backdate**: tanggal yang tercetak di watermark bisa digeser ke hari lain (jam tetap berjalan). **Waktu pengambilan asli tetap yang dikirim ke server** (`captured_at`), persis seperti mobile.
5. **Lokasi GPS**: saat memotret, ambil koordinat dari browser (`navigator.geolocation`, akurasi tinggi, batas waktu 8 detik, boleh memakai posisi terakhir bila < 5 menit). Untuk foto slot distribusi, kirim `latitude`/`longitude`/`captured_at` ke `POST /api/v1/distribution/slots/{documentationSlotId}/media` — backend **sudah** menerima ketiga field ini (`internal/api/distribution_routes.go:473-505`). Bila slot `require_location` aktif dan lokasi tidak didapat, upload diblokir dengan pesan jelas (backend juga sudah menolak lewat `ErrMediaLocationRequired`).
6. **Perkecil foto sebelum upload**: pilihan kualitas setara mobile — Hemat (sisi terpanjang 1280 px), Standar (1920 px, bawaan), Tinggi (2560 px), Asli (tanpa resize) — JPEG kualitas 92 (95 untuk Asli). Berlaku untuk foto dari kamera watermark **dan** foto dari galeri.
7. **Titik pakai**:
   - Slot dokumentasi distribusi (`frontend/src/features/distribution/DocumentationSlot.tsx`) — tombol "Buka kamera" membuka kamera watermark.
   - Dokumentasi Kegiatan (`frontend/src/features/activities/ActivityDocumentationPage.tsx`) — tombol "Ambil Foto" membuka kamera watermark.
   - Halaman pengaturan watermark baru, bisa dibuka dari halaman Profil dan dari tombol di layar kamera.

## 3. Non-tujuan

- **Tidak** menyentuh scan Dokumen Pendukung (`DocumentScanner.tsx`) maupun scan RAKORDA (`RakordaScanEditor.tsx`) — itu pemindaian dokumen, bukan foto lapangan.
- **Tidak** menambah kolom lokasi ke `activity_media`. Foto kegiatan tetap memuat watermark (termasuk alamat dan koordinat tercetak di gambar), tapi koordinatnya tidak disimpan sebagai data terpisah — sama seperti mobile, yang juga tidak mengirim koordinat untuk foto kegiatan.
- **Tidak** mengubah perekaman video (tetap input file bawaan, tanpa watermark), sama seperti mobile.
- **Tidak** ada mode offline / antrean upload di web — itu proyek terpisah.
- **Tidak** ada rotasi elemen watermark (mobile juga mematikannya secara bawaan).
- **Tidak** membuat izin (permission) baru: kamera watermark dan backdate tersedia untuk siapa pun yang sudah bisa mengupload foto di halaman tersebut, sama seperti mobile.

## 4. Anti-tujuan (gagal walau "jalan")

- Foto yang ter-upload **berbeda** dari pratinjau yang dilihat petugas (posisi/ukuran watermark bergeser). Hasil akhir harus sama dengan pratinjau.
- Backdate ikut mengubah `captured_at` yang dikirim ke server — jejak waktu asli tidak boleh hilang.
- Kamera watermark menggantikan pilihan galeri. Galeri tetap tersedia sesuai `input_source` slot (foto galeri: tanpa watermark, tanpa koordinat, tapi tetap diperkecil).
- Kegagalan alamat/GPS membuat foto tidak bisa diambil (kecuali slot yang mewajibkan lokasi). Alamat gagal → tulis "Alamat tidak tersedia"; GPS gagal → blok koordinat dikosongkan, foto tetap bisa diambil.

## 5. Keputusan yang sudah diambil

**Dari pengguna:**
- Fitur ini meniru mobile demi kesetaraan dan fleksibilitas, bukan untuk alur kerja baru.
- Backdate **ikut** dibawa ke web.

**Keputusan saya (bisa dikoreksi saat review):**
- **Reverse geocoding lewat server Konkit**, bukan langsung dari browser: endpoint baru `GET /api/v1/geo/reverse?lat=&lng=` yang memanggil OpenStreetMap Nominatim, karena browser tidak punya geocoder bawaan seperti HP. Server mengirim `User-Agent` yang mengidentifikasi Konkit (syarat Nominatim), membatasi ke ≤ 1 permintaan/detik secara global, menyimpan cache di memori berdasarkan koordinat yang dibulatkan ke 4 desimal (~11 m), memakai `accept-language=id`, dan menyusun alamat dari komponen yang setara mobile (jalan, kelurahan/desa, kecamatan, kabupaten/kota, provinsi, kode pos), tanpa duplikat, dipisah koma. Endpoint memerlukan login dan salah satu izin lapangan yang sudah ada (`fieldReadPermissions` di `internal/api/program_routes.go:157`). Klien hanya meminta ulang alamat bila posisi bergeser > 15 m, seperti mobile.
- **Pengaturan disimpan di `localStorage` per perangkat**, seperti mobile memakai prefs lokal; logo kustom disimpan sebagai data URL di `localStorage` (bila terlalu besar, perkecil dulu ke sisi terpanjang 512 px PNG).
- **Kamera watermark sebagai komponen lazy-loaded** (pola yang sama dengan `BarcodeScanner.tsx` dan `DocumentScanner.tsx`), memakai `getUserMedia` dengan preferensi kamera belakang dan resolusi 1920×1080, plus pilihan ganti kamera bila ada lebih dari satu.

## 6. Batasan

- Tanpa dependency npm/Go baru; Canvas 2D, `getUserMedia`, `navigator.geolocation` semuanya API browser standar; Go stdlib `net/http` untuk Nominatim.
- Teks UI dalam Bahasa Indonesia.
- Bundle `web/static/app` **tidak** di-commit dari Windows (CI memverifikasi byte bundle; build ulang dilakukan di Linux saat rilis).
- Gaya kode dan komponen mengikuti pola yang ada (shadcn-style `components/ui`, TanStack Query, Vitest, pola error Indonesia di `writeServiceError`).
- `navigator.geolocation` dan `getUserMedia` hanya jalan di konteks aman (HTTPS atau `localhost`) — produksi sudah HTTPS.

## 7. Yang diserahkan ke pembangun

- Struktur file/komponen, nama hook, dan pembagian modul di frontend.
- Cara menyesuaikan pinch-zoom dan drag untuk mouse/desktop, selama hasilnya sama dengan pratinjau.
- Tata letak persis layar kamera dan halaman pengaturan (ikuti mobile sebagai acuan visual), dan di mana tautan "Pengaturan watermark" diletakkan di Profil.
- Format persis tampilan tanggal/jam di watermark — ikuti `watermark_widgets.dart` mobile.
- Strategi cache dan batas laju Nominatim di server, selama memenuhi kebijakan penggunaan Nominatim.
- Pengujian: unit test untuk penyusunan gambar (posisi, skala, sembunyi/tampil), resize, backdate vs `captured_at`, penanganan GPS/alamat gagal, dan endpoint geocoding (dengan server Nominatim palsu); uji komponen untuk alur kamera → tinjau → pakai.
