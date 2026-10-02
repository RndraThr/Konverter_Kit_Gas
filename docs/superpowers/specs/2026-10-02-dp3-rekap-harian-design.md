# PRD DP3 dan Rekapitulasi Harian

**Tanggal:** 2 Oktober 2026  
**Status:** Draft untuk review  
**Pemilik produk:** Konkit  
**Target pembaca:** Agent implementasi berikutnya, engineer backend/frontend, dan reviewer produk

## 1. Ringkasan

Konkit akan menambahkan dua jenis Berita Acara pada ruang kerja Berita Acara:

1. **DP3 (Daftar Penerima Paket Perdana)** sebagai dokumen daftar nominatif sebelum distribusi. Sumbernya adalah penerima pada jadwal terpilih yang berasal dari impor DCP3 maupun input manual.
2. **Rekapitulasi Harian** sebagai dokumen hasil aktual distribusi pada satu tanggal, berdasarkan penerima yang sudah menyelesaikan penyerahan.

Keduanya memakai pipeline dokumen agregat yang sama untuk preview, validasi, snapshot, finalisasi, versioning, audit, unduh, dan sinkronisasi Google Drive. Query sumber data dan renderer PDF tetap terpisah karena aturan bisnis serta tata letaknya berbeda.

Dokumen final harus stabil terhadap perubahan kode barang, template paket, identitas penerima, logo, lokasi, dan penandatangan. Stabilitas dicapai dengan menyimpan snapshot lengkap pada saat finalisasi.

## 2. Latar Belakang

Ruang kerja Berita Acara sudah memiliki tab DP3, BA Perorangan, dan Rekap Harian, tetapi saat ini hanya BA Perorangan yang memiliki alur operasional lengkap. Referensi layout Go untuk DP3 dan Rekap Harian sudah tersedia dan menjadi acuan visual awal.
 
Alur nominatif Konkit mempunyai dua pintu masuk:

- impor workbook DCP3;
- tambah penerima secara manual.

Kedua pintu masuk harus menghasilkan kandidat distribusi dengan semantik yang sama. Penerima belum memiliki nomor bagi ketika baru masuk daftar. Nomor bagi ditetapkan hanya ketika operator menghubungkan kandidat ke slot distribusi pada POS Dokumen.

Perbaikan prasyarat ini sudah diterapkan: penerima manual baru disimpan tanpa `distribution_number`, sedangkan data manual lama yang belum terhubung dikoreksi melalui migrasi 28.

## 3. Istilah

- **DCP3:** fitur sumber data penerima, termasuk impor workbook dan normalisasi data nominatif.
- **DP3:** dokumen PDF “Daftar Penerima Paket Perdana” yang dibentuk dari daftar nominatif pada sebuah jadwal.
- **Kandidat belum ter-mount:** alokasi penerima dengan `distribution_number = NULL` dan belum terhubung ke slot distribusi.
- **Mounting/hubungkan penerima:** proses menghubungkan kandidat ke nomor bagi pada POS Dokumen. Pada titik inilah nomor bagi ditetapkan.
- **Snapshot alokasi:** salinan identitas paket/barang yang melekat pada penerima ketika nominasi dibuat.
- **Snapshot verifikasi:** salinan barang aktual dan nomor seri yang disimpan ketika distribusi diselesaikan.
- **Dokumen aktif:** versi final terbaru untuk kombinasi identitas dokumen yang sama.
- **Versi superseded:** versi final lama yang tetap menjadi riwayat dan tidak lagi menjadi versi utama.

## 4. Tujuan Produk

1. Menghasilkan DP3 langsung dari daftar nominatif tanpa input ulang.
2. Menyatukan perilaku penerima hasil impor dan penerima manual.
3. Menghasilkan Rekap Harian langsung dari transaksi distribusi selesai.
4. Menampilkan barang per penerima, bukan satu nilai barang global untuk seluruh tabel.
5. Menjaga dokumen final tetap identik walaupun kode atau isi template paket kemudian berubah.
6. Mendukung preview, finalisasi, versioning, audit, unduh, dan sinkronisasi Drive dengan pola yang konsisten.
7. Menyediakan fondasi yang dapat dipakai untuk jenis BA kolektif berikutnya tanpa memaksakan satu renderer generik.

## 5. Bukan Ruang Lingkup

- Implementasi BA Closing Titik Serah, Closing Kabupaten, Rakorda, Sosialisasi, Training, Pemeriksaan, Servis Berkala, atau TKDN.
- Dukungan template Nelayan untuk DP3 dan Rekap Harian pada fase pertama.
- Editor tata letak PDF bebas di UI.
- Pengubahan atau penggantian pipeline BA Perorangan yang sudah aktif.
- Penghapusan riwayat versi dokumen secara otomatis.
- Tanda tangan digital atau QR signature.

## 6. Prinsip Produk

### 6.1 Jadwal adalah batas data

Semua query DP3 dan Rekap Harian wajib menggunakan `schedule_id`. Program dan kabupaten hanya menjadi atribut turunan dan pemeriksaan scope. Query tidak boleh hanya memakai pasangan `program_id + regency_id`, karena satu program dan kabupaten dapat memiliki lebih dari satu jadwal.

### 6.2 DP3 adalah daftar nominatif

DP3 bukan turunan dari distribusi selesai. Penerima hasil impor DCP3 dan input manual harus muncul melalui sumber nominatif yang sama.

### 6.3 Nomor bagi bukan nomor urut input

Menambah penerima tidak boleh menghasilkan nomor bagi. Nomor bagi dibuat atau dipilih pada POS Mesin dan ditetapkan pada alokasi hanya ketika kandidat dihubungkan melalui POS Dokumen.

### 6.4 Data aktual per penerima

Kolom barang pada DP3 dan Rekap Harian diisi per penerima. Renderer tidak boleh memakai satu objek mesin global untuk seluruh baris.

### 6.5 Snapshot mengalahkan lookup kode

Kode barang hanya berfungsi sebagai identitas saat input. PDF tidak boleh bergantung pada lookup kode terhadap template paket terkini. Jika kode diubah atau dihapus, snapshot lama tetap harus dapat dirender dan diunduh.

### 6.6 Renderer khusus per dokumen

Pipeline lifecycle boleh generik, tetapi DP3 dan Rekap Harian memiliki renderer, model render, validasi, dan visual regression test masing-masing.

## 7. Alur Operasional Utama

```text
Impor DCP3 ───────┐
                  ├─> Daftar nominatif jadwal ─> Preview/finalisasi DP3
Tambah manual ────┘                │
                                   └─> Mounting ke nomor bagi
                                             │
                                             └─> Distribusi selesai
                                                       │
                                                       ├─> BA Perorangan
                                                       └─> Rekap Harian
```

### 7.1 Penerima dari impor

1. Operator mengimpor DCP3 ke jadwal.
2. Sistem membuat nominasi dan alokasi tanpa nomor bagi.
3. Sistem menyimpan snapshot paket dari versi template paket jadwal.
4. Jika template hanya memiliki satu varian mesin, sistem memilihnya otomatis. Jika terdapat lebih dari satu varian, impor wajib memetakan varian mesin untuk setiap penerima.
5. Alokasi berstatus `ready` muncul di DP3 dan dapat dicari berdasarkan NIK pada POS Dokumen.

### 7.2 Penerima manual

1. Operator memilih jadwal lalu menambahkan penerima manual.
2. Sistem membuat `people`, identitas sektor, nominasi, dan alokasi.
3. `distribution_number` wajib `NULL`.
4. Form manual memilih varian mesin jika template jadwal mempunyai lebih dari satu varian; pilihan diisi otomatis jika hanya ada satu.
5. Sistem menyimpan snapshot paket dan varian terpilih yang setara dengan hasil impor DCP3, berdasarkan versi template paket jadwal saat penerima dibuat.
6. Penerima muncul di DP3 dan dapat dicari berdasarkan NIK pada POS Dokumen.

### 7.3 Mounting

1. Operator membuat atau memilih nomor bagi pada POS Mesin.
2. Operator mencari kandidat berdasarkan NIK pada jadwal yang sama.
3. Kandidat hanya dapat dipilih jika aktif, belum terhubung, dan belum menerima paket sebelumnya.
4. Dalam satu transaksi database, sistem mengisi `package_allocations.distribution_number`, menghubungkan `distribution_slots.allocation_id`, mengisi `recipient_person_id`, dan mengubah slot menjadi `linked`.

### 7.4 Distribusi selesai

Ketika penyerahan diselesaikan, sistem menyimpan snapshot verifikasi barang aktual, termasuk kode pilihan, label/merek/tipe/spesifikasi, dan nomor seri. Rekap Harian dan BA Perorangan mengutamakan snapshot ini.

## 8. Aturan Data DP3

### 8.1 Cakupan penerima

DP3 mengambil penerima aktif pada `schedule_id` terpilih dengan aturan:

- termasuk alokasi berstatus `ready`;
- tetap termasuk alokasi berstatus `distributed`, agar DP3 yang dibuat setelah kegiatan masih mencerminkan daftar penerima sah;
- tidak termasuk `candidate` yang belum lolos validasi;
- tidak termasuk `needs_review`, `replaced`, atau `cancelled`;
- tidak bergantung pada ada/tidaknya `distribution_number`;
- urutan utama: `distribution_number` yang sudah tersedia secara menaik;
- kandidat belum ter-mount ditempatkan setelah penerima bernomor, lalu diurutkan berdasarkan waktu pembuatan dan ID agar deterministik.

Sebelum finalisasi, UI harus menampilkan jumlah penerima bernomor dan belum bernomor. Belum adanya nomor bagi tidak menghalangi finalisasi DP3 karena dokumen referensi tidak memiliki kolom nomor bagi.

### 8.2 Tanggal dokumen

- Pengguna memilih tanggal dokumen DP3.
- Nilai awal adalah `program_schedules.start_date`.
- Tanggal disimpan sebagai tanggal lokal tanpa komponen waktu.
- Hari dan nama bulan pada PDF menggunakan Bahasa Indonesia.

### 8.3 Data identitas

Setiap baris memerlukan:

- nama lengkap;
- alamat yang dapat terdiri dari alamat, desa/kelurahan, kecamatan, dan kabupaten;
- NIK.

Finalisasi diblokir bila nama, NIK, atau seluruh komponen alamat kosong. Desa/kelurahan atau kecamatan yang belum tersedia ditampilkan sebagai peringatan tetapi tidak memblokir selama terdapat alamat yang dapat dicetak. Alamat tidak boleh dirender sebagai teks `null` atau separator kosong.

### 8.4 Data barang

Setiap baris DP3 memerlukan:

- merek mesin;
- tipe mesin;
- daya;
- jenis BBM.

Sumber data menggunakan urutan prioritas:

1. snapshot verifikasi distribusi jika penerima sudah selesai distribusi;
2. snapshot paket/alokasi saat nominasi dibuat;
3. tidak melakukan fallback diam-diam ke template paket terkini.

Jika snapshot lama belum memiliki `power` atau `fuel_type`, preview menampilkan validasi data tidak lengkap dan finalisasi diblokir. Template paket Petani harus memperluas setiap `machine_option` dengan field `power` dan `fuel_type`. Saat snapshot dibuat, label tersebut disalin bersama kodenya.

Snapshot alokasi menyimpan satu `selected_machine` per penerima, bukan seluruh daftar opsi sebagai sumber render. Field minimalnya adalah `code`, `brand`, `type`, `power`, dan `fuel_type`. Kode boleh berubah pada template baru tanpa mengubah label snapshot lama. Jika template mempunyai beberapa varian dan penerima belum memiliki pilihan, penerima ditandai `allocation_snapshot_incomplete` dan DP3 tidak dapat difinalisasi.

### 8.5 Identitas dokumen

Kunci logis DP3 adalah:

`schedule_id + document_type(dp3) + document_date`

Perubahan daftar penerima atau metadata setelah finalisasi menghasilkan versi baru untuk kunci logis yang sama.

## 9. Aturan Data Rekapitulasi Harian

### 9.1 Cakupan penerima

Rekap Harian mengambil `distribution_slots` yang:

- termasuk `schedule_id` terpilih;
- berstatus `completed`;
- memiliki `distributed_at` pada tanggal lokal terpilih dalam zona `Asia/Jakarta`.

Urutan baris menggunakan `slot_number` menaik, kemudian ID sebagai tie-breaker deterministik.

### 9.2 Tanggal dokumen

Tanggal Rekap Harian berasal dari tanggal lokal `distributed_at`. Pengguna memilihnya dari daftar tanggal yang memiliki distribusi selesai; tidak ada input tanggal bebas.

### 9.3 Data baris

Setiap baris berisi:

- nomor urut baris;
- nama penerima;
- nomor kartu petani;
- merek mesin;
- tipe mesin;
- nomor seri mesin.

Identitas mesin dan nomor seri wajib berasal dari snapshot verifikasi distribusi. Finalisasi diblokir jika snapshot tersebut tidak lengkap. Lookup ke kode template terkini hanya boleh dipakai sebagai mekanisme migrasi legacy yang eksplisit dan harus menghasilkan snapshot sebelum dokumen dapat difinalisasi.

### 9.4 Grand Total per varian

Varian mesin didefinisikan oleh kombinasi normalisasi:

`machine_brand + machine_type + power + fuel_type`

PDF menampilkan satu baris ringkasan untuk setiap varian dengan:

- label `Varian 1`, `Varian 2`, dan seterusnya;
- deskripsi merek dan tipe;
- jumlah penerima dalam satuan `Set`.

Urutan varian mengikuti kemunculan pertama pada tabel agar stabil. Jumlah seluruh varian wajib sama dengan jumlah baris penerima.

### 9.5 Identitas dokumen

Kunci logis Rekap Harian adalah:

`schedule_id + document_type(daily_recap) + local_date`

## 10. Konfigurasi Jadwal

Setiap jadwal Petani yang akan memakai DP3 atau Rekap Harian memiliki konfigurasi BA berikut:

| Field | Wajib | Dipakai oleh |
|---|---:|---|
| Lokasi/Titik Serah | Ya | DP3, Rekap Harian |
| Nama perusahaan Konsultan Distribusi | Ya | Header DP3, header Rekap Harian |
| Nama Dinas Pertanian | Ya | DP3, Rekap Harian |
| NIP Dinas Pertanian | Ya | DP3, Rekap Harian |
| Nama Pelaksana Pemasangan dan Pendistribusian | Ya | DP3, Rekap Harian |
| Nama Konsultan Pengawas | Ya | DP3, Rekap Harian |
| Nama perwakilan PT Pertamina Patra Niaga | Ya untuk Rekap | Rekap Harian |

Konfigurasi disimpan per `schedule_id`, bukan sebagai profil dokumen global. Nama perusahaan Konsultan Distribusi berbeda dari nama individu Konsultan Pengawas. Nilai awal nama perusahaan dapat memakai `PT Kian Santang Muliatama Tbk.` sesuai referensi, tetapi tetap dapat dikonfigurasi per jadwal. Field `program_schedules.supervisor_name` yang sudah ada dimigrasikan menjadi nilai awal Konsultan Pengawas agar tidak terjadi input ulang.

Perubahan konfigurasi memengaruhi preview dan finalisasi berikutnya, tetapi tidak mengubah snapshot atau PDF yang sudah final.

## 11. Sumber Branding

- Tahun anggaran berasal dari `programs.fiscal_year`.
- Logo berasal dari `program_ba_logo_assets` aktif milik program, diurutkan berdasarkan `sort_order`.
- DP3 dan Rekap Harian memakai branding program yang sama dengan BA Perorangan.
- Finalisasi diblokir jika tidak ada logo aktif.
- Snapshot dokumen menyimpan ID aset, storage key, MIME type, urutan, dan batas dimensi logo yang dipakai.

## 12. Spesifikasi PDF DP3

Referensi awal adalah renderer Go DP3 yang diberikan pengguna.

- Ukuran A4 landscape.
- Kepala halaman memuat logo program, judul `DAFTAR PENERIMA PAKET PERDANA`, label `(FORM DP3)`, dan uraian program.
- Metadata memuat Hari/Tanggal, Lokasi/Titik Serah, Kabupaten/Kota, dan Konsultan Distribusi.
- Kolom tabel:
  - No;
  - Nama Petani;
  - Alamat;
  - No. KTP;
  - Data Mesin: Merek, Tipe, Daya, Jenis BBM;
  - Paraf.
- Tinggi baris menyesuaikan teks terpanjang tanpa memotong konten.
- Header logo, judul, metadata, dan kepala tabel diulang pada halaman lanjutan.
- Minimal lima baris visual; baris kosong hanya untuk menjaga layout dan tidak dihitung sebagai penerima.
- Halaman tanda tangan terpisah memiliki tiga kolom:
  - Dinas Pertanian daerah;
  - Pelaksana Pemasangan dan Pendistribusian;
  - Konsultan Pengawas.
- Nama dan NIP tidak boleh terpotong atau keluar batas.

## 13. Spesifikasi PDF Rekapitulasi Harian

Referensi awal adalah renderer Go Rekap Harian yang diberikan pengguna.

- Ukuran A4 portrait.
- Kepala halaman memuat logo program, judul `BERITA ACARA SERAH TERIMA`, label `(FORM REKAPITULASI PENERIMA PAKET)`, dan uraian program.
- Metadata memuat Hari/Tanggal, Lokasi/Titik Serah, dan Konsultan Distribusi.
- Kolom tabel:
  - No;
  - Nama Petani;
  - No. Kartu Petani;
  - Merek Mesin;
  - Tipe Mesin;
  - No. Seri Mesin.
- Tinggi baris dinamis dan header diulang pada halaman lanjutan.
- Bagian Grand Total mendukung lebih dari satu varian sebagaimana aturan pada bagian 9.4.
- Halaman tanda tangan terpisah memiliki empat kolom:
  - Dinas Pertanian daerah;
  - Pelaksana Pemasangan dan Pendistribusian;
  - Konsultan Pengawas;
  - PT Pertamina Patra Niaga.

## 14. Aturan Tipografi dan Layout Bersama

- Semua teks menggunakan Unicode yang valid; karakter mojibake seperti `â€` dilarang.
- Tanda inci ditulis konsisten sebagai `3”` atau sesuai konten resmi yang disetujui.
- Teks dinamis wajib melalui normalisasi whitespace tanpa mengubah identitas formal.
- Tidak boleh ada teks yang bertabrakan, terpotong, keluar halaman, atau menghasilkan halaman kosong yang tidak disengaja.
- Logo mempertahankan rasio aspek.
- Renderer harus deterministik: input snapshot yang sama menghasilkan checksum PDF yang sama, kecuali metadata internal library PDF yang memang harus dinormalisasi.

## 15. Arsitektur Dokumen Agregat

### 15.1 Batas modul

Tetap gunakan modul `internal/bast`, dengan pemisahan berikut:

- lifecycle bersama untuk preview/finalisasi/versioning/storage;
- query builder/source loader khusus DP3;
- query builder/source loader khusus Rekap Harian;
- model snapshot khusus setiap document type;
- renderer khusus setiap document type;
- satu kontrak penyimpanan dokumen agregat.

BA Perorangan tidak dipaksa masuk ke pipeline baru pada fase ini.

### 15.2 Tabel dokumen agregat

Tambahkan tabel `bast_aggregate_documents` dengan field minimal:

- `id uuid`;
- `schedule_id uuid`;
- `program_id uuid`;
- `regency_id uuid`;
- `document_type text` (`dp3` atau `daily_recap` pada fase ini);
- `document_date date`;
- `filename text`;
- `recipient_count integer`;
- `page_count integer`;
- `version integer`;
- `status text` (`active` atau `superseded`);
- `checksum text`;
- `storage_key text`;
- `snapshot_json jsonb`;
- `last_error text`;
- `finalized_by uuid`;
- `finalized_at`, `created_at`, `updated_at`.

Constraint penting:

- satu versi `active` untuk `schedule_id + document_type + document_date`;
- nomor versi unik untuk kunci logis yang sama;
- checksum wajib 64 karakter heksadesimal;
- storage key harus `text`, karena ID Google Drive bukan UUID.

### 15.3 Snapshot

Snapshot DP3 minimal berisi:

- identitas jadwal/program/kabupaten;
- tanggal dokumen;
- tahun anggaran dan logo;
- lokasi/titik serah;
- penandatangan;
- daftar penerima terurut;
- identitas dan barang setiap penerima;
- kode serta label varian terpilih setiap penerima;
- sumber barang (`allocation_snapshot` atau `verification_snapshot`).

Snapshot Rekap Harian minimal berisi:

- seluruh metadata bersama;
- tanggal distribusi lokal;
- daftar penerima dan snapshot mesin aktual;
- daftar varian dan total tiap varian;
- grand total penerima.

Snapshot disimpan sebelum render final. Renderer final hanya membaca snapshot, bukan tabel operasional.

### 15.4 Versioning dan idempotensi

1. Backend membangun snapshot kandidat dan checksum konten.
2. Jika versi aktif mempunyai checksum sama, finalisasi mengembalikan versi aktif tanpa upload ulang.
3. Jika checksum berubah, backend membuat PDF dan mengunggah file baru.
4. Aktivasi versi baru dan supersede versi lama dilakukan dalam transaksi dengan advisory lock berdasarkan kunci logis.
5. Jika aktivasi database gagal, file baru dibersihkan dari storage.
6. Versi superseded dan file PDF-nya tetap dipertahankan untuk audit; UI utama hanya menampilkan versi aktif.

## 16. Google Drive dan Nama File

Path penyimpanan:

```text
{ROOT}/PETANI/{ZONE}/{REGENCY}/BERITA ACARA (BA)/1. DP3/
{ROOT}/PETANI/{ZONE}/{REGENCY}/BERITA ACARA (BA)/3. REKAP HARIAN/
```

Tidak dibuat subfolder tanggal. Nama file harus aman untuk Drive, mudah dicari, dan membedakan versi:

- `DP3 - {KABUPATEN} - {YYYY-MM-DD} - V{VERSION}.pdf`
- `REKAP HARIAN - {KABUPATEN} - {YYYY-MM-DD} - V{VERSION}.pdf`

Folder dibuat melalui abstraksi `media.Storage` dan cache folder yang sudah ada. `storage_key` selalu diperlakukan sebagai opaque string.

## 17. API

Gunakan endpoint eksplisit per jenis dokumen dengan implementasi lifecycle bersama.

### 17.1 Konfigurasi jadwal

- `GET /api/v1/bast/schedules/{schedule_id}/settings`
- `PUT /api/v1/bast/schedules/{schedule_id}/settings`

### 17.2 DP3

- `GET /api/v1/bast/dp3/summary?schedule_id=...`
- `GET /api/v1/bast/dp3/recipients?schedule_id=...`
- `POST /api/v1/bast/dp3/preview`
- `POST /api/v1/bast/dp3/finalize`
- `GET /api/v1/bast/dp3/documents?schedule_id=...&date=...`
- `GET /api/v1/bast/dp3/documents/{id}/content`

Body preview/finalize:

```json
{
  "schedule_id": "uuid",
  "document_date": "2026-10-02"
}
```

### 17.3 Rekap Harian

- `GET /api/v1/bast/daily-recap/dates?schedule_id=...`
- `GET /api/v1/bast/daily-recap/recipients?schedule_id=...&date=...`
- `POST /api/v1/bast/daily-recap/preview`
- `POST /api/v1/bast/daily-recap/finalize`
- `GET /api/v1/bast/daily-recap/documents?schedule_id=...&date=...`
- `GET /api/v1/bast/daily-recap/documents/{id}/content`

Body preview/finalize:

```json
{
  "schedule_id": "uuid",
  "local_date": "2026-10-02"
}
```

### 17.4 Permission

- Membaca ringkasan, penerima, preview, versi, dan konten memerlukan `bast.view`.
- Mengubah konfigurasi dan finalisasi memerlukan `bast.manage`.
- Seluruh endpoint menerapkan regency scope dari jadwal.
- Resource di luar scope dikembalikan sebagai `404`.

## 18. Validasi dan Error Produk

Status validasi yang harus dapat dibedakan UI:

- `zone_not_configured`;
- `ba_logo_required`;
- `handover_location_required`;
- `signatory_required`;
- `no_recipients`;
- `recipient_identity_incomplete`;
- `allocation_snapshot_incomplete`;
- `verification_snapshot_incomplete`;
- `machine_power_required`;
- `machine_fuel_required`;
- `document_conflict`.

Error storage atau database tetap menghasilkan `500`, tetapi harus dicatat pada log server dengan correlation/request ID tanpa membocorkan token, credential, atau connection string ke browser.

## 19. UI/UX

### 19.1 Konteks bersama

Pilihan jadwal di halaman Berita Acara tetap menjadi konteks utama. Tab tidak menyimpan pilihan jadwal sendiri. Hanya jadwal aktif Petani yang didukung pada fase pertama.

### 19.2 Tab DP3

Tampilkan:

- tanggal dokumen dengan default tanggal mulai jadwal;
- jumlah total nominatif;
- jumlah sudah memiliki nomor bagi;
- jumlah belum ter-mount;
- status kelengkapan konfigurasi dan data barang;
- tabel pratinjau penerima;
- tombol Preview PDF;
- tombol Finalisasi & Sinkronkan bagi pengguna `bast.manage`;
- kartu versi aktif dan riwayat versi.

Tabel pratinjau minimal menampilkan nama, NIK, status mounting, merek/tipe, daya, BBM, dan sumber snapshot.

### 19.3 Tab Rekap Harian

Ikuti pola BA Perorangan:

- daftar tanggal distribusi di sisi kiri;
- jumlah penerima dan status dokumen per tanggal;
- detail tanggal terpilih;
- tabel penerima dan varian;
- preview, finalisasi, unduh, dan riwayat versi.

### 19.4 Empty dan error state

- DP3 kosong: jelaskan bahwa penerima perlu diimpor melalui DCP3 atau ditambahkan manual.
- Rekap kosong: jelaskan bahwa belum ada distribusi selesai pada tanggal/jadwal tersebut.
- Validasi konfigurasi harus menyebut field yang perlu dilengkapi dan memberikan tautan menuju pengaturan jadwal jika pengguna memiliki permission.
- Kegagalan preview/finalisasi tidak boleh menghilangkan pilihan tanggal atau tabel yang sudah dimuat.

## 20. Konsistensi dan Transaksi

- Semua list dan render memakai urutan deterministik.
- Snapshot dibangun dalam pembacaan konsisten agar jumlah dan detail baris berasal dari keadaan yang sama.
- Finalisasi memakai advisory lock untuk mencegah dua versi aktif.
- Jika data berubah antara preview dan finalisasi, finalisasi membangun ulang snapshot dan checksum; UI menampilkan hasil final sebenarnya.
- Dokumen final tidak diperbarui in-place.
- Perubahan master penerima atau template paket setelah finalisasi tidak memodifikasi dokumen lama.

## 21. Audit

Catat event minimal:

- `bast.schedule_settings_updated`;
- `bast.dp3_finalized`;
- `bast.daily_recap_finalized`;
- `bast.aggregate_document_downloaded` bila kebijakan audit unduhan diaktifkan.

Metadata finalisasi memuat `schedule_id`, `document_date`, `recipient_count`, `version`, dan checksum, tanpa menyimpan NIK lengkap pada audit log.

## 22. Strategi Pengujian

### 22.1 Unit test

- filter status penerima DP3;
- urutan penerima bernomor dan belum ter-mount;
- prioritas snapshot verifikasi terhadap snapshot alokasi;
- perubahan kode template tidak memengaruhi snapshot;
- pengelompokan varian Rekap Harian;
- format tanggal Indonesia;
- sanitasi nama file;
- checksum dan idempotensi.

### 22.2 Repository integration test

- penerima impor dan manual sama-sama masuk DP3;
- penerima manual tidak mendapat nomor bagi sebelum mounting;
- mounting menetapkan nomor bagi secara atomik;
- query terisolasi berdasarkan `schedule_id`;
- batas tanggal memakai `Asia/Jakarta`;
- scope kabupaten diterapkan;
- opaque Google Drive storage key tersimpan sebagai text;
- hanya satu dokumen aktif per kunci logis.

### 22.3 Renderer test

- golden PDF atau text extraction untuk judul, metadata, baris, grand total, dan penandatangan;
- page count untuk data pendek dan panjang;
- repeat header pada multi-page;
- baris dengan nama/alamat panjang;
- beberapa varian mesin;
- karakter Indonesia dan simbol inci tanpa mojibake;
- logo dengan rasio berbeda.

### 22.4 Frontend test

- jadwal diteruskan sebagai `schedule_id` ke seluruh request;
- tanggal default DP3;
- daftar tanggal Rekap Harian;
- permission tombol finalisasi;
- rendering status validasi;
- preview membuka blob PDF;
- unduh versi aktif dan versi lama;
- state tidak hilang setelah mutation error.

### 22.5 End-to-end

1. Tambah penerima manual.
2. Pastikan nomor bagi kosong dan penerima muncul di DP3.
3. Preview/finalisasi DP3.
4. Buat nomor bagi dan hubungkan penerima melalui NIK.
5. Isi barang, unggah dokumentasi, dan selesaikan distribusi.
6. Pastikan penerima muncul pada Rekap Harian tanggal yang benar.
7. Preview/finalisasi Rekap Harian.
8. Ubah kode pada template paket.
9. Pastikan dokumen final lama tetap identik dan versi baru memakai snapshot yang benar.

## 23. Acceptance Criteria

### 23.1 DP3

- Penerima hasil impor dan manual muncul pada jadwal yang tepat.
- Input manual tidak membuat nomor bagi.
- Penerima dapat dicari melalui NIK saat mounting.
- DP3 dapat difinalisasi walaupun sebagian penerima belum memiliki nomor bagi.
- Setiap baris menampilkan identitas dan mesin penerima tersebut.
- Perubahan kode/template setelah finalisasi tidak merusak PDF lama.
- PDF mengikuti layout referensi dan lolos pemeriksaan visual.
- File tersimpan di folder `1. DP3` dengan versi yang jelas.

### 23.2 Rekap Harian

- Hanya distribusi selesai pada jadwal dan tanggal lokal terpilih yang masuk.
- Mesin dan nomor seri berasal dari snapshot aktual setiap penerima.
- Grand Total per varian benar dan totalnya sama dengan jumlah penerima.
- PDF mengikuti layout referensi dan lolos pemeriksaan visual.
- File tersimpan di folder `3. REKAP HARIAN`.

### 23.3 Lifecycle bersama

- Preview tidak mengubah database atau Drive.
- Finalisasi identik bersifat idempotent.
- Perubahan isi menghasilkan versi baru tanpa menimpa riwayat.
- Hanya satu versi aktif per kunci logis.
- Seluruh operasi menghormati permission dan regency scope.

## 24. Urutan Implementasi yang Direkomendasikan

1. Lengkapi snapshot paket penerima manual dan field mesin `power`/`fuel_type`.
2. Tambahkan konfigurasi BA per jadwal beserta UI-nya.
3. Tambahkan schema dan repository dokumen agregat.
4. Implementasikan source loader, snapshot, renderer, API, dan UI DP3.
5. Implementasikan source loader, snapshot, renderer, API, dan UI Rekap Harian.
6. Tambahkan versioning, Drive, audit, dan concurrency control.
7. Jalankan visual QA PDF serta end-to-end alur lengkap.

Urutan ini bukan implementation plan terperinci. Agent berikutnya wajib membuat rencana implementasi berbasis PRD yang sudah disetujui sebelum mengubah kode.

## 25. Keputusan yang Sudah Dikunci

- DP3 bersumber dari daftar nominatif, bukan distribusi selesai.
- Impor DCP3 dan tambah manual menghasilkan kandidat dengan semantik sama.
- Nomor bagi ditetapkan ketika mounting, bukan ketika input penerima.
- Rekap Harian bersumber dari distribusi selesai per tanggal lokal.
- Barang ditampilkan per penerima dan menggunakan snapshot.
- Grand Total Rekap dihitung per varian aktual.
- Lokasi/titik serah dan penandatangan dikonfigurasi per jadwal.
- Tanggal DP3 dipilih pengguna dengan default tanggal mulai jadwal.
- Pipeline lifecycle agregat digunakan bersama, sementara renderer tetap khusus per dokumen.
- Fase pertama hanya mendukung program Petani.
