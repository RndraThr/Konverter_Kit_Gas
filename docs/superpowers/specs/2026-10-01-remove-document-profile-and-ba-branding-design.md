# Penghapusan Profil Dokumen dan Branding Berita Acara

## Ringkasan keputusan

Konsep **Profil Dokumen Tender** dihapus sepenuhnya. Sistem tidak lagi menyediakan judul, subjudul, deskripsi pengadaan, seri dokumen, atau versi profil yang berlaku umum untuk semua jenis Berita Acara.

Setiap jenis Berita Acara memiliki renderer, teks baku, struktur data, dan tata letaknya sendiri berdasarkan dokumen referensi yang disetujui. Hanya logo tender dan tahun program yang bersifat dinamis serta dapat digunakan bersama oleh seluruh jenis Berita Acara dalam satu program.

BA Perorangan Petani menggunakan PDF `006. BAST - Penerima Paket (Perorangan).pdf` sebagai otoritas visual. Jenis BA lain akan dirancang terpisah ketika referensinya diberikan.

## Tujuan

- Menghapus asumsi bahwa seluruh jenis BA mempunyai identitas teks dan layout yang sama.
- Menempatkan pengaturan yang memang berkaitan dengan BA di fitur **Berita Acara**, bukan di Persiapan Program.
- Menjaga logo tetap dinamis per tender tanpa membuat profil dokumen generik.
- Menjadikan setiap renderer BA mandiri agar perubahan pada satu jenis BA tidak mengubah jenis BA lain.
- Menjaga dokumen final yang sudah dibuat tetap dapat dibuka dan disinkronkan tanpa bergantung pada konfigurasi yang dapat berubah.

## Di luar cakupan

- Implementasi desain BA Sosialisasi, Training, Closing, dan jenis lain yang referensinya belum diberikan.
- Finalisasi mekanisme serta isi tanda tangan.
- Editor layout bebas atau pembuat template dokumen berbasis drag-and-drop.
- Logo berbeda untuk setiap jenis BA. Logo berlaku bersama dalam satu tender.

## Model konsep baru

### Branding BA per tender

Satu program memiliki kumpulan aset logo BA yang terurut. Aset ini hanya menyimpan informasi yang dibutuhkan untuk merender logo:

- program pemilik;
- kode slot stabil;
- lokasi berkas dan metadata berkas;
- urutan;
- batas lebar dan tinggi;
- status tampil atau tersembunyi.

Tidak ada judul, subjudul, deskripsi pengadaan, seri dokumen, status draf/terbit, atau versioning profil.

Pengaturan logo ditempatkan di ruang kerja **Berita Acara**. Pengguna dengan `bast.view` dapat melihat konfigurasi, sedangkan pengguna dengan `bast.manage` dapat mengunggah, mengurutkan, menampilkan, dan menyembunyikan logo.

### Template per jenis BA

Teks dan layout merupakan kode renderer milik masing-masing jenis BA. Renderer tidak membaca teks umum dari database.

Untuk BA Perorangan Petani:

- judul, subjudul, deskripsi, label, paragraf pernyataan, dan kepala tabel mengikuti PDF referensi;
- tahun di dalam deskripsi berasal dari `programs.fiscal_year`;
- susunan logo berasal dari branding BA program;
- nomor dokumen tetap dibangun dari urutan pembagian, total kabupaten, seri baku `KSM-KKT`, kode kabupaten, bulan Romawi, dan tahun distribusi;
- tanggal berasal dari tanggal distribusi setempat;
- data penerima berasal dari penerima pada slot distribusi;
- data mesin, selang, dan konkit/reducer berasal dari pilihan Template Paket dan nomor seri aktual pada slot distribusi;
- tabel **Komponen Paket, Aksesoris & Kelengkapan** berasal dari `components` pada versi Template Paket yang digunakan jadwal;
- tanda tangan mengikuti struktur referensi, tetapi detail perilakunya tetap ditunda sampai keputusan pengguna tersedia.

Template Paket KONKIT-2026 perlu diselaraskan dengan referensi. Khususnya, referensi menggunakan satu baris `Selang, Clamp & Aksesorisnya`, bukan dua baris komponen `Selang Hisap` dan `Selang Buang`.

## Perubahan antarmuka

### Persiapan Program

- Hapus tab **Profil Dokumen**.
- Hapus `DocumentProfilePanel`, tipe frontend terkait, query, mutasi, dan tesnya.
- Persiapan Program tetap berisi kabupaten, program, zona, jadwal, dan Template Paket/Dokumentasi.

### Berita Acara

- Tambahkan area **Logo Tender** pada fitur Berita Acara.
- Program tetap dipilih melalui konteks program yang sudah digunakan halaman BA.
- Daftar logo memperlihatkan pratinjau, urutan, serta status tampil.
- Aksi pengelolaan logo hanya muncul untuk `bast.manage`.
- Tab jenis BA tetap terpisah. Pengaturan logo tidak menjadi tab jenis BA dan tidak membawa pengaturan teks/layout umum.

## API

Hapus seluruh endpoint `/api/v1/program-setup/programs/{programID}/document-profiles...` dan kontrak service/repository Profil Dokumen.

Sediakan endpoint branding di domain BA:

- `GET /api/v1/bast/branding?program_id={programID}`;
- `POST /api/v1/bast/branding/logos` untuk unggah logo;
- `PATCH /api/v1/bast/branding/logos/{logoID}` untuk urutan, ukuran, dan visibilitas.

Semua endpoint memvalidasi bahwa program tersedia. Operasi baca membutuhkan `bast.view`; operasi perubahan membutuhkan `bast.manage`. Unggahan tetap dibatasi pada PNG/JPEG maksimal 10 MiB, dan seluruh perubahan dicatat dalam audit log.

## Migrasi database

Migration baru menggunakan nomor berikutnya yang tersedia dan melakukan perubahan secara transaksional:

1. Buat `program_ba_logo_assets` yang berelasi langsung ke `programs`.
2. Salin logo dari profil terbaru yang relevan untuk setiap program, mempertahankan storage key, urutan, ukuran, visibilitas, checksum, dan metadata berkas.
3. Validasi bahwa setiap aset sumber tersalin tepat satu kali sebelum data lama dihapus.
4. Lepaskan ketergantungan dokumen BA final pada `profile_version_id`. Dokumen final tetap menggunakan `snapshot_json` sebagai sumber historis.
5. Hapus `program_document_logo_assets` dan `program_document_profile_versions` beserta trigger serta indeksnya.

Migration tidak menghapus berkas logo fisik yang storage key-nya dipindahkan ke tabel baru. Jika tidak ada profil lama, program memulai dengan daftar logo kosong dan pembuatan BA ditahan sampai minimal satu logo aktif tersedia.

Migration lama tidak diubah karena database yang sudah terpasang harus tetap memiliki riwayat migration yang valid. Konsep lama dihapus melalui migration maju.

## Snapshot dan kompatibilitas dokumen final

BA final harus tetap immutable. Snapshot baru tidak menggunakan nama `ProfileSnapshot`; struktur tersebut diganti menjadi identitas render BA yang berisi nilai hasil render seperti tahun program, logo, data penerima, peralatan, komponen, dan tanda tangan.

Dokumen lama yang sudah mempunyai `snapshot_json` tetap dapat dibuka. Decoder harus menerima snapshot lama selama masa migrasi, kemudian memetakannya ke model baca internal tanpa kembali mengaktifkan konsep Profil Dokumen.

Pembuatan atau finalisasi BA baru tidak boleh bergantung pada tabel Profil Dokumen. Validasi `ErrProfileNotPublished` dihapus dan diganti dengan kesalahan konfigurasi logo BA apabila tidak ada logo aktif.

## Alur data BA Perorangan

1. Pengguna memilih program, kabupaten, dan tanggal pada BA Perorangan.
2. Service memastikan zona, kuota final kabupaten, serta logo BA aktif tersedia.
3. Service mengambil penerima yang selesai dibagikan pada tanggal tersebut.
4. Untuk setiap penerima, service mengambil Template Paket versi jadwal dan nilai peralatan aktual.
5. Service membangun nomor dokumen dan snapshot immutable.
6. Renderer BA Perorangan Petani merender satu penerima per halaman sesuai PDF referensi.
7. Halaman digabung menjadi satu PDF harian dan disinkronkan ke Drive dengan alur yang sudah ada.

## Penanganan kesalahan

- Tidak ada logo aktif: tampilkan status konfigurasi belum lengkap pada fitur BA dan cegah finalisasi.
- Template Paket tidak mempunyai komponen: cegah finalisasi karena tabel komponen tidak dapat dibuat sesuai dokumen resmi.
- Opsi peralatan pada slot tidak ditemukan pada Template Paket: laporkan data pembagian tidak konsisten dan cegah finalisasi.
- Snapshot lama: tetap dapat dibaca; kegagalan decoding dilaporkan sebagai dokumen historis tidak valid tanpa mengubah data.
- Kegagalan migrasi aset logo: rollback seluruh migration, termasuk penghapusan tabel lama.

## Strategi pengujian

### Database dan repository

- Migration menyalin seluruh logo lama ke program yang benar tanpa kehilangan storage key.
- Migration melepaskan referensi profil dari dokumen final dan menghapus tabel Profil Dokumen.
- Repository branding membatasi logo berdasarkan program dan menjaga urutan.

### API

- Endpoint Profil Dokumen tidak lagi terdaftar.
- Pengguna `bast.view` dapat membaca branding tetapi tidak dapat mengubahnya.
- Pengguna `bast.manage` dapat mengunggah dan mengatur logo.
- Validasi tipe, ukuran berkas, kepemilikan program, dan audit tetap berlaku.

### Frontend

- Tab Profil Dokumen tidak tampil di Persiapan Program.
- Area Logo Tender tampil di Berita Acara.
- Kontrol pengubahan hanya tersedia untuk `bast.manage`.
- Program tanpa logo menampilkan keadaan kosong dan petunjuk konfigurasi, bukan crash.

### BA Perorangan

- Golden/render tests membandingkan struktur halaman dengan PDF referensi.
- Tahun pada teks mengikuti `programs.fiscal_year`.
- Komponen mengikuti urutan, jumlah, dan satuan pada Template Paket.
- Satu penerima menghasilkan satu halaman; bundle harian mempertahankan urutan nomor dokumen.
- Dokumen lama tetap dapat dibuka dari snapshot sebelum migrasi.

## Kriteria penerimaan

- Tidak ada UI, endpoint, service, model aktif, atau tabel database bernama Profil Dokumen.
- Hanya logo tender dan tahun program yang menjadi konfigurasi bersama lintas jenis BA.
- Pengaturan logo berada di fitur Berita Acara.
- BA Perorangan Petani mengikuti tata letak PDF referensi dan mengambil komponen dari Template Paket.
- BA final lama tidak rusak dan BA baru tidak bergantung pada profil generik.
- Seluruh tes backend dan frontend lulus, bundle produksi dibangun ulang, migration diterapkan, dan server lokal menyajikan bundle baru.
