# Rancangan Zona Program, Struktur Drive, dan BA Perorangan

## 1. Tujuan

Fase ini membangun fondasi Berita Acara (BA) Perorangan Petani dan menata ulang konteks program agar setiap file dapat ditempatkan konsisten berdasarkan jenis program, zona, kabupaten/kota, jenis dokumen, dan tanggal pembagian.

Hasil akhirnya:

- satu `Program` merepresentasikan satu tender dan satu jenis program (`farmer`/Petani atau `fisherman`/Nelayan);
- setiap kabupaten/kota pada program ditempatkan dalam satu zona;
- logo dan identitas dokumen dapat dikonfigurasi per program/tender dan dipublikasikan dalam versi yang tidak berubah;
- satu penerima mempunyai satu data BA Perorangan dengan nomor dokumen yang berasal dari nomor pembagian;
- seluruh BA Perorangan pada kabupaten dan tanggal pembagian yang sama digabung menjadi satu PDF banyak halaman;
- Google Drive menerima satu PDF rangkapan per tanggal langsung di folder `2. BA PERORANGAN`, tanpa subfolder tanggal;
- template, data, nomor, logo, dan PDF yang sudah difinalkan dapat diaudit dan tidak berubah diam-diam.

## 2. Batasan Fase

Fase ini mencakup:

1. struktur zona dalam Persiapan Program;
2. konfigurasi dokumen tender dan logo dinamis;
3. pembentukan path Google Drive yang baru;
4. BA Perorangan Petani berdasarkan contoh yang diberikan;
5. pencatatan BA per penerima;
6. penggabungan PDF per tanggal pembagian;
7. pratinjau, finalisasi, unduh, dan sinkronisasi ke penyimpanan aktif;
8. status sinkronisasi serta audit perubahan.

Fase ini belum mencakup:

- template BA Perorangan Nelayan; fondasi dan pemisahan datanya disiapkan, tetapi isi dokumennya menunggu contoh dari pengguna;
- penyematan gambar tanda tangan atau tanda tangan digital;
- keputusan proses unggah kembali BA bertanda tangan;
- jenis BA lain seperti DP3, Rekap Harian, Closing, Rakorda, dan seterusnya;
- pemindahan otomatis file lama yang sudah berada di Google Drive. File lama tetap dapat dibuka melalui `storage_key`; upload baru memakai struktur baru;
- editor bebas untuk mengubah seluruh layout PDF. Layout Petani dikodekan mengikuti referensi, sementara nilai dan aset yang memang dinamis disimpan sebagai konfigurasi.

Bagian tanda tangan pada PDF Petani tetap dirender sebagai kotak dan label sesuai referensi, tanpa gambar tanda tangan. Nama pihak yang datanya tersedia boleh dicetak; mekanisme penandatanganan akan dirancang pada fase lanjutan.

## 3. Keputusan Arsitektur

Generator merender BA per penerima secara terpisah di memori, kemudian menggabungkannya berdasarkan tanggal pembagian. Pendekatan ini dipilih karena kesalahan pada satu penerima dapat dilaporkan secara spesifik, data BA per penerima tetap dapat ditelusuri, dan artefak di Drive tetap berupa satu rangkapan harian.

Alternatif membuat satu PDF besar secara langsung ditolak karena menyulitkan isolasi kesalahan dan pratinjau penerima. Pembuatan PDF di browser juga ditolak karena hasilnya dapat berbeda antarperangkat dan kurang sesuai untuk proses massal yang harus dapat diulang.

PDF dibentuk di backend agar font, ukuran A4, pagination, urutan halaman, dan hasil unduhan konsisten. Implementasi memakai pustaka PDF Go yang sudah digunakan aplikasi bila mampu memenuhi layout; bila pengujian visual membuktikan kemampuannya tidak cukup, penggantian renderer dibatasi di dalam paket BA dan tidak mengubah kontrak service atau API.

## 4. Model Wilayah Program

### 4.1 Program dan zona

`Program` tetap menjadi induk tender dan tetap mempunyai satu `program_type`. Ditambahkan:

- `program_zones`
  - `id`
  - `program_id`
  - `code`
  - `name`
  - `sort_order`
  - `created_at`, `updated_at`
  - unik pada `(program_id, code)` dan `(program_id, lower(name))`
- `program_regency_assignments`
  - `program_id`
  - `regency_id`
  - `zone_id`
  - `created_at`, `updated_at`
  - primary/unique pada `(program_id, regency_id)`

Satu kabupaten/kota hanya dapat berada di satu zona dalam program yang sama. Kabupaten yang sama boleh masuk zona berbeda pada program/tender lain.

`program_schedules` tetap menyimpan `program_id` dan `regency_id`. Pasangan tersebut harus mempunyai `program_regency_assignments`, sehingga jadwal selalu dapat diturunkan ke satu zona tanpa menyimpan nama zona berulang-ulang.

### 4.2 Migrasi data yang sudah ada

Untuk setiap program lama, migrasi membuat zona aman bernama `ZONA BELUM DIATUR`, lalu mengaitkan seluruh kabupaten yang sudah mempunyai jadwal ke zona tersebut. Dengan demikian tidak ada jadwal atau media lama yang hilang. Administrator kemudian memindahkan kabupaten ke zona sebenarnya melalui Persiapan Program.

Program tidak boleh dipakai untuk upload baru atau finalisasi BA sampai seluruh kabupaten aktifnya keluar dari `ZONA BELUM DIATUR`.

## 5. Konfigurasi Dokumen Tender dan Logo

Konfigurasi dibuat per program karena satu program merepresentasikan satu tender. Konfigurasi memakai versi:

- `program_document_profile_versions`
  - `id`, `program_id`, `version`
  - judul, subjudul, uraian pengadaan, kode seri dokumen, dan konfigurasi teks lain
  - `status`: `draft`, `published`, `retired`
  - `published_at`, `created_at`, `updated_at`
  - unik pada `(program_id, version)`
- `program_document_logo_assets`
  - `id`, `profile_version_id`
  - `slot_code`, `storage_key`, `mime_type`, `checksum`
  - `sort_order`, `max_width_mm`, `max_height_mm`, `is_visible`

Administrator dapat mengunggah, mengurutkan, menyembunyikan, dan mengganti logo pada versi draf. Format aset yang diterima adalah PNG transparan atau JPEG. Renderer menempatkan setiap logo dalam kotak ukuran tetap dengan perilaku setara `contain`, sehingga rasio logo tidak terdistorsi.

Versi yang sudah `published` tidak dapat diubah. Perubahan logo atau teks membuat versi baru. BA yang difinalkan menyimpan ID versi profil dan snapshot nilai yang digunakan, sehingga dokumen lama tidak ikut berubah.

## 6. Struktur Google Drive

Root aktual tetap berasal dari `GDRIVE_ROOT_FOLDER_ID`. Di bawahnya, folder dibuat secara idempoten dengan struktur:

```text
{ROOT}/
  PETANI | NELAYAN/
    {NAMA ZONA}/
      {NAMA KABUPATEN/KOTA}/
        BERITA ACARA (BA)/
          1. DP3/
          2. BA PERORANGAN/
          3. REKAP HARIAN/
          4. CLOSING TITIK SERAH/
          5. CLOSING KABUPATEN/
          6. RAKORDA/
          7. SOSIALISASI/
          8. TRAINING 10%/
          9. TRAINING 100%/
          10. BA PEMERIKSAAN/
          11. SERVIS BERKALA/
          12. TKDN/
        DOKUMEN PENDUKUNG/
        DOKUMENTASI (FOTO)/
          {JENIS KEGIATAN}/
```

`2. BA PERORANGAN` langsung berisi file PDF. Tidak dibuat subfolder berdasarkan tanggal.

Satu pembentuk path terpusat menerima `program_type`, zona, kabupaten/kota, kategori, dan jenis dokumen. Semua fitur baru wajib memakai pembentuk path tersebut. Modul dokumentasi kegiatan yang saat ini hanya mengetahui kabupaten ditambah konteks program; upload baru ditolak bila program atau assignment zona tidak valid.

Nama folder melalui sanitasi terpusat: spasi dirapikan, karakter pemisah path dilarang, tetapi nama tampil tetap berupa huruf normal dan bukan slug. Cache folder tetap menggunakan path key terkanonisasi dan tetap menyimpan Drive folder ID.

## 7. Penomoran BA Perorangan

Format awal Petani mengikuti:

```text
{NOMOR_PEMBAGIAN_PADDED}/{TOTAL_FINAL_KABUPATEN}/{SERI}-{KODE_KABUPATEN}/{BULAN_ROMAWI}/{TAHUN}
```

Contoh:

```text
0102/1578/KSM-KKT-WJO/XII/2024
```

Aturannya:

- `0102` berasal dari `distribution_slots.slot_number` dengan padding jadwal;
- `1578` adalah total pembagian final kabupaten pada program/tender tersebut;
- urutan dimulai dari kabupaten masing-masing, bukan global tender;
- kode kabupaten berasal dari `regencies.document_code`;
- bulan dan tahun berasal dari tanggal pembagian dalam timezone aplikasi;
- nomor yang sudah masuk BA final tidak dapat digunakan ulang untuk penerima lain.

Total final disimpan sebagai nilai terkunci pada konteks program-kabupaten. Nilai awalnya diambil dari `program_schedules.slot_quota`, kemudian aksi `Kunci Daftar BA` memvalidasi bahwa nomor pembagian berada dalam rentang `1..total`, tidak ganda, dan konfigurasi wilayah serta dokumen sudah lengkap. Nilai total tidak dihitung ulang saat PDF lama dibuka.

Perubahan total setelah ada BA final tidak diperbolehkan. Koreksi administratif harus melalui pembatalan/revisi yang tercatat di audit dan tidak menulis ulang dokumen final lama secara diam-diam.

## 8. Model BA dan Rangkapan Harian

### 8.1 BA per penerima

`bast_individual_documents` menyimpan satu BA untuk satu `distribution_slot`:

- identitas program, zona, kabupaten, jadwal, dan distribution slot;
- jenis program dan jenis dokumen (`individual_handover`);
- nomor pembagian, total final kabupaten, dan nomor dokumen lengkap;
- tanggal pembagian lokal;
- versi profil dokumen dan versi template;
- `snapshot_json` berisi seluruh nilai yang dicetak;
- status `draft`, `final`, `cancelled`, atau `superseded`;
- nomor revisi;
- audit waktu dan pengguna pembuat/finalisasi.

Snapshot memuat sekurang-kurangnya:

- data penerima: nama, alamat, kabupaten/kota, NIK, kartu Petani/KUSUKA, dan telepon;
- mesin: merek, tipe, dan serial number;
- selang: merek, spesifikasi, serial number;
- konkit/reducer: merek dan serial number;
- komponen paket, jumlah, satuan, dan status kelengkapan;
- pelaksana distribusi dan pengawas yang tersedia;
- nomor dokumen serta tanggal;
- teks tender dan referensi aset logo yang dipakai.

Snapshot dibentuk dari tabel operasional ketika BA difinalkan. Membuka ulang BA final tidak mengambil nilai terbaru dari tabel penerima atau paket.

### 8.2 Rangkapan per tanggal

`bast_daily_bundles` menyimpan satu rangkapan untuk kombinasi:

```text
program + kabupaten + tanggal pembagian lokal + jenis dokumen
```

Data yang disimpan meliputi status, nama file, jumlah BA, jumlah halaman, checksum, `storage_key`, versi rangkapan, waktu sinkronisasi, error terakhir, dan pengguna pemicu.

Nama file:

```text
{HARI}, {DD} {NAMA BULAN} {YYYY}.pdf
```

Contoh:

```text
SELASA, 10 DESEMBER 2024.pdf
```

Hari dan bulan menggunakan bahasa Indonesia dan timezone aplikasi.

## 9. Alur Generate dan Sinkronisasi

1. Pengguna memilih program, kabupaten, dan tanggal pembagian di tab `BA Perorangan`.
2. Backend mengambil seluruh distribution slot berstatus `completed` dengan tanggal lokal tersebut dan memeriksa akses kabupaten pengguna.
3. Data diurutkan berdasarkan `slot_number` menaik.
4. Untuk setiap penerima, backend memvalidasi field wajib, membentuk atau memakai snapshot final, lalu merender BA A4. Setiap BA selalu dimulai pada halaman baru; satu BA boleh memanjang ke lebih dari satu halaman.
5. Seluruh hasil digabung menjadi satu PDF. Lima puluh pembagian menghasilkan satu PDF berisi lima puluh BA dan dapat mempunyai lebih dari lima puluh halaman.
6. Pratinjau/unduh dapat dilakukan tanpa sinkronisasi Drive.
7. Pada aksi finalisasi dan sinkronisasi, backend menghitung checksum dan mengunggah file baru ke path `.../BERITA ACARA (BA)/2. BA PERORANGAN`.
8. Setelah upload berhasil, transaksi database mengganti referensi rangkapan aktif ke Drive file ID baru dan mencatat audit.
9. File versi lama baru dihapus setelah referensi baru berhasil disimpan. Jika penghapusan lama gagal, rangkapan baru tetap aktif dan pembersihan dicatat untuk retry; akses pengguna tidak kembali ke file lama.

Sinkronisasi ulang dengan input dan checksum yang sama menjadi no-op. Bila snapshot final berubah melalui prosedur revisi, versi rangkapan bertambah dan PDF dibangun ulang. Drive tidak boleh berisi dua file aktif dengan nama dan tanggal yang sama dari sistem.

## 10. Layout BA Perorangan Petani

Layout mengikuti referensi A4 yang diberikan:

1. baris logo dinamis;
2. judul `BERITA ACARA SERAH TERIMA (FORM PENERIMA PAKET)`;
3. uraian tender;
4. nomor BAST dan tanggal;
5. data penerima;
6. tabel mesin;
7. tabel selang hisap dan selang buang;
8. tabel konkit/reducer;
9. tabel komponen paket, aksesori, dan kelengkapan;
10. pernyataan penerimaan;
11. tiga blok tanda tangan tanpa gambar tanda tangan pada fase ini.

Renderer memakai margin, ukuran font minimum, lebar kolom, dan wrapping yang tetap. Baris komponen boleh melanjutkan halaman. Header identitas dokumen diulang bila tabel berlanjut agar halaman lanjutan tetap dapat dikenali. Penerima berikutnya tidak boleh dimulai pada sisa ruang BA sebelumnya.

## 11. Antarmuka Pengguna

### 11.1 Persiapan Program

Program memperoleh bagian `Zona & Kabupaten`:

- daftar zona;
- tambah, ubah nama, urutkan, dan arsipkan zona;
- penempatan kabupaten/kota ke zona;
- indikator kabupaten yang belum diatur;
- validasi bahwa kabupaten aktif hanya berada di satu zona.

Program juga memperoleh bagian `Konfigurasi Dokumen Tender`:

- teks identitas tender;
- konfigurasi seri/format nomor;
- upload, urutan, ukuran relatif, visibilitas, dan pratinjau logo;
- simpan draf dan publikasi versi;
- pratinjau layout Petani menggunakan data contoh.

### 11.2 BA Perorangan

Tab `BA Perorangan` memakai konteks program yang dipilih dan menampilkan:

- tipe Petani/Nelayan otomatis;
- filter zona, kabupaten, dan tanggal;
- ringkasan total final kabupaten;
- daftar tanggal pembagian beserta jumlah penerima, status validasi, status rangkapan, dan status sinkronisasi;
- detail penerima pada tanggal terpilih, diurutkan berdasarkan nomor pembagian;
- aksi pratinjau, unduh, finalisasi, sinkronkan ulang, dan buka hasil tersimpan.

Untuk program Nelayan, halaman dan struktur datanya tetap terpisah. Sampai template Nelayan tersedia, UI menampilkan status `Template BA Perorangan Nelayan belum dikonfigurasi` dan tidak memakai template Petani sebagai fallback.

## 12. API dan Izin

Izin `bast.view` tetap dipakai untuk membaca. Ditambahkan:

- `bast.manage` untuk membuat draf, finalisasi, revisi, dan sinkronisasi;
- pengelolaan zona dan profil dokumen tetap berada di `programs.manage`.

Semua endpoint menerapkan `RegencyScope`. Pengguna tidak dapat memperoleh daftar, PDF, atau status sinkronisasi kabupaten di luar cakupannya walaupun mengetahui ID objek.

Kontrak endpoint dikelompokkan di bawah:

```text
/api/v1/program-setup/programs/{programID}/zones
/api/v1/program-setup/programs/{programID}/document-profiles
/api/v1/bast/individual
/api/v1/bast/individual/dates
/api/v1/bast/individual/bundles
```

Endpoint content melakukan streaming melalui backend storage; file Drive tidak dibuat publik.

## 13. Penanganan Kesalahan dan Konsistensi

- Kabupaten tanpa zona sebenarnya: upload/finalisasi ditolak dengan pesan untuk melengkapi Persiapan Program.
- Profil dokumen belum dipublikasikan: BA hanya dapat dilihat sebagai konfigurasi belum lengkap, tidak dapat difinalkan.
- Total final belum dikunci atau nomor di luar rentang: finalisasi ditolak.
- Data penerima/paket wajib belum lengkap: tanggal ditandai gagal validasi dan menampilkan penerima serta field yang bermasalah.
- Tidak ada distribusi selesai pada tanggal: generator tidak membuat PDF kosong.
- Upload Drive gagal: metadata rangkapan aktif lama tidak berubah; error disimpan dan dapat dicoba kembali.
- Cache folder Drive stale: cache dibuang dan path di-resolve ulang memakai mekanisme storage yang ada.
- Dua finalisasi bersamaan untuk tanggal yang sama: constraint unik dan penguncian transaksi memastikan hanya satu versi aktif.
- Perubahan timezone tidak menulis ulang tanggal dokumen final; tanggal lokal disimpan dalam snapshot.

## 14. Migrasi dan Kompatibilitas

- Migrasi database bersifat additive dan tidak menghapus tabel atau data program lama.
- Seluruh jadwal lama memperoleh assignment ke `ZONA BELUM DIATUR`.
- `activity_media` memperoleh konteks program untuk upload baru. Baris lama tetap dapat dibaca; bila program lama dapat ditentukan secara tunggal, migrasi mengisinya, selain itu baris ditandai legacy tanpa dipindahkan.
- Dokumentasi distribusi yang terikat schedule dapat memakai program, zona, dan kabupaten dari rantai relasinya untuk upload baru.
- Folder lama tidak dipindah atau dihapus otomatis. Perubahan lokasi fisik file lama memerlukan operasi migrasi Drive terpisah dan persetujuan eksplisit.
- Struktur folder baru dibuat saat sinkronisasi/upload pertama atau melalui aksi administratif `Siapkan Struktur Drive`.

## 15. Pengujian

### 15.1 Database dan service

- constraint zona per program dan satu assignment per kabupaten;
- backfill zona aman untuk jadwal lama;
- validasi total final dan rentang nomor pembagian;
- nomor dokumen per kabupaten, padding, bulan Romawi, dan timezone;
- snapshot final tidak berubah saat data sumber diedit;
- pengelompokan tanggal lokal dan urutan `slot_number`;
- Petani dan Nelayan tidak berbagi template atau data;
- regency scope pada seluruh query dan endpoint;
- konkurensi finalisasi rangkapan.

### 15.2 PDF

- output mempunyai header PDF valid;
- setiap penerima dimulai pada halaman baru;
- urutan nomor benar;
- 50 penerima menjadi satu rangkapan;
- tabel panjang membuat halaman lanjutan tanpa memotong teks;
- logo dengan rasio berbeda tidak terdistorsi;
- golden/rendered-page visual test untuk layout Petani pada data pendek dan panjang.

### 15.3 Storage dan Drive

- path Petani/Nelayan, zona, kabupaten, tiga folder utama, dan 12 folder BA;
- `2. BA PERORANGAN` berisi PDF langsung tanpa subfolder tanggal;
- nama file hari/tanggal Indonesia;
- sinkronisasi checksum sama bersifat no-op;
- upload baru gagal tidak mengganti file aktif;
- swap berhasil lalu cleanup file lama;
- cache folder stale di-resolve ulang;
- local storage tetap berfungsi untuk lingkungan tanpa Drive.

### 15.4 Frontend

- pengelolaan zona dan assignment kabupaten;
- konfigurasi serta urutan logo;
- filter zona/kabupaten/tanggal;
- daftar status validasi dan sinkronisasi;
- pratinjau, finalisasi, unduh, retry;
- pesan template Nelayan belum tersedia;
- aksesibilitas kontrol, loading, empty, error, dan responsive state.

## 16. Kriteria Selesai

Fitur dinyatakan selesai bila:

1. zona dapat disusun pada program dan setiap jadwal aktif dapat diturunkan ke zona;
2. upload baru mengikuti hierarchy `PETANI/NELAYAN → ZONA → KABUPATEN → kategori`;
3. profil tender dan logo dapat dipublikasikan dalam versi immutable;
4. BA Perorangan Petani mengambil data distribusi final dan menghasilkan nomor `urutan/total` per kabupaten;
5. seluruh BA pada tanggal yang sama tergabung dalam satu PDF bernama hari dan tanggal;
6. PDF berada langsung di `2. BA PERORANGAN` tanpa subfolder tanggal;
7. sinkronisasi ulang aman, dapat diaudit, dan tidak menghasilkan dua file aktif;
8. data Nelayan tetap terpisah dan tidak memakai template Petani;
9. test backend, frontend, migrasi, PDF, dan storage lulus.
