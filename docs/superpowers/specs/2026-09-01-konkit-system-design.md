# Rancangan Sistem Website Konkit

Versi: 0.1  
Tanggal: 2026-09-01  
Status: Draft awal untuk diskusi dan revisi

Rancangan rinci DCP3, identitas penerima, peralihan, pemeriksaan penerimaan berulang, pendistribusian, dan slot foto dilanjutkan dalam `2026-09-03-dcp3-distribution-documentation-design.md`. Dokumen rinci tersebut menggantikan asumsi awal pada bagian calon penerima, import, dan dokumentasi apabila terdapat perbedaan.

## 1. Gambaran Umum

Konkit adalah singkatan dari konverter kit gas, yaitu alat tambahan yang dipasang pada mesin kendaraan atau mesin alat kerja agar dapat menggunakan bahan bakar gas seperti LPG atau CNG sebagai pengganti bensin atau solar.

Website Konkit dirancang sebagai sistem administrasi dan operasional program Konkit lintas kabupaten. Sistem ini menyimpan seluruh data dalam satu database pusat, sehingga data dari kabupaten yang sudah selesai tetap dapat diakses ketika program berpindah ke kabupaten lain.

Fokus awal sistem adalah aplikasi internal/admin, bukan website publik. Website publik dapat ditambahkan pada tahap berikutnya jika diperlukan.

## 2. Tujuan Sistem

Tujuan utama sistem:

- Mengelola data program Konkit lintas kabupaten dalam satu database pusat.
- Mengatur jadwal pelaksanaan program per kabupaten.
- Memisahkan kebutuhan data antara program Konkit Nelayan dan Konkit Petani.
- Mendukung import data awal calon penerima dari Excel jika tersedia.
- Tetap mendukung input manual jika data awal dari dinas tidak tersedia.
- Menghasilkan dokumen Berita Acara Serah Terima (BAST) per penerima.
- Menyediakan laporan berdasarkan kabupaten, jenis program, periode, dan status.

## 3. Jenis Program

Sistem membagi data program menjadi dua jenis utama.

### 3.1 Konkit Nelayan

Digunakan untuk penerima dari sektor nelayan, misalnya pemilik kapal atau pengguna mesin kapal/perahu.

Data yang kemungkinan diperlukan:

- data penerima nelayan,
- data kapal/perahu,
- data mesin,
- data pemasangan konkit,
- serial number komponen,
- dokumen dan Berita Acara,
- status proses.

Detail final field akan disesuaikan setelah alur nelayan dijelaskan lebih lanjut.

### 3.2 Konkit Petani

Digunakan untuk penerima dari sektor pertanian, misalnya petani, kelompok tani, atau pengguna pompa air dan alat mesin pertanian.

Data yang kemungkinan diperlukan:

- data penerima petani,
- data kelompok tani jika ada,
- data alat atau mesin pertanian,
- data lokasi/lahan jika diperlukan,
- data pemasangan konkit,
- serial number komponen,
- dokumen dan Berita Acara,
- status proses.

Detail final field akan disesuaikan setelah alur petani dijelaskan lebih lanjut.

## 4. Role Pengguna

Untuk rancangan awal, role utama adalah Super Admin.

Super Admin memiliki akses penuh untuk mengatur hal krusial, seperti:

- master kabupaten,
- kode kabupaten 3 huruf,
- jadwal program per kabupaten,
- jenis program yang berjalan,
- setting template dokumen,
- format penomoran BAST,
- master komponen paket,
- import data calon penerima,
- laporan dan arsip data.

Role lain seperti Admin Kabupaten, Petugas Lapangan, atau Viewer dapat ditambahkan pada fase berikutnya jika diperlukan.

## 5. Konsep Kabupaten Dan Jadwal Program

Kabupaten tidak dibuat sebagai sistem terpisah. Kabupaten menjadi master wilayah dan jadwal kerja di dalam satu sistem pusat.

Setiap kabupaten memiliki data dasar:

- nama kabupaten,
- kode kabupaten 3 huruf,
- provinsi,
- status aktif/nonaktif,
- catatan.

Kode kabupaten 3 huruf digunakan untuk kebutuhan dokumen, termasuk penomoran BAST.

Contoh:

```text
Nama Kabupaten: Wonogiri
Kode Dokumen: WJO
Provinsi: Jawa Tengah
```

Program berjalan melalui jadwal kegiatan.

Contoh jadwal:

```text
Kabupaten: Wonogiri
Kode Kabupaten: WJO
Jenis Program: Petani
Periode: 1-10 Desember 2024
Status: Aktif
```

Status jadwal program:

- rencana,
- aktif,
- selesai,
- arsip,
- dibatalkan.

## 6. Import Data Awal Dari Excel

Dalam beberapa kegiatan, dinas dapat memberikan data awal berupa Excel daftar calon penerima. Namun kondisi ini tidak selalu ada, sehingga sistem harus mendukung dua jalur:

- import Excel calon penerima,
- input manual calon penerima.

### 6.1 Alur Import Excel

Alur yang disarankan:

1. Super Admin memilih jadwal kabupaten dan jenis program.
2. Super Admin upload file Excel dari dinas.
3. Sistem menampilkan preview data.
4. Super Admin melakukan mapping kolom Excel ke field sistem.
5. Sistem melakukan validasi dasar.
6. Data disimpan sebagai Calon Penerima.
7. Sistem menyimpan riwayat import.

### 6.2 Mapping Kolom

Karena format Excel dari tiap kabupaten dapat berbeda, sistem tidak boleh terlalu kaku. Super Admin perlu bisa memilih kolom mana yang sesuai dengan field sistem.

Contoh mapping:

- kolom nama ke field nama penerima,
- kolom NIK ke field No. KTP,
- kolom alamat ke field alamat,
- kolom desa ke field desa,
- kolom kecamatan ke field kecamatan,
- kolom nomor HP ke field nomor HP,
- kolom nomor kartu petani/nelayan ke field kartu identitas sektor.

### 6.3 Nomor Urut Excel

Data Excel biasanya memiliki nomor urut di kolom paling kiri, misalnya 1 sampai seterusnya. Nomor ini menjadi patokan nomor urut BAST.

Contoh:

```text
Nomor urut Excel: 2
Kode Kabupaten: WJO
Format BAST: {urut}/1578/KSM-KKT-{kode_kabupaten}/XII/2024
Hasil BAST: 0002/1578/KSM-KKT-WJO/XII/2024
```

Penggunaan padding seperti `0001` atau angka biasa seperti `1` dapat diatur di settings.

### 6.4 Validasi Import

Validasi awal:

- nomor urut kosong,
- nomor urut duplikat,
- nama kosong,
- NIK duplikat jika tersedia,
- format nomor HP tidak wajar,
- baris kosong,
- data yang sudah pernah diimport di jadwal yang sama.

Data yang salah tidak langsung ditolak seluruhnya. Sistem sebaiknya memberi daftar baris bermasalah agar Super Admin dapat memperbaiki atau melewati baris tertentu.

## 7. Status Calon Penerima

Data calon penerima tidak otomatis menjadi penerima final. Status diperlukan agar proses lapangan tetap fleksibel.

Status awal:

- calon penerima,
- valid,
- tidak valid,
- pending,
- sudah dipasang,
- batal,
- selesai BAST.

Status ini dapat dikembangkan sesuai alur operasional sebenarnya.

## 8. Modul BAST Generator

BAST Generator adalah modul untuk membuat dokumen Berita Acara Serah Terima per penerima.

Prinsip utama:

- data tipikal dimasukkan ke settings,
- data penerima ditarik dari Excel atau input manual,
- data kegiatan ditarik dari jadwal kabupaten,
- data yang belum ada dilengkapi melalui form,
- sistem menghasilkan preview dan PDF BAST.

### 8.1 Data Dari Settings

Data yang dapat disiapkan oleh Super Admin:

- judul dokumen,
- subjudul dokumen,
- nama program/kegiatan,
- tahun anggaran,
- logo instansi/perusahaan,
- nama vendor/pelaksana,
- format nomor BAST,
- kode kegiatan/program,
- format bulan, termasuk bulan romawi,
- daftar komponen paket,
- merk mesin default,
- tipe mesin default,
- merk selang,
- spesifikasi selang,
- merk regulator/reducer,
- daftar checklist,
- nama jabatan penandatangan,
- nama petugas/pelaksana/pengawas default.

Settings ini dapat dibuat per jenis program dan per jadwal kegiatan, karena format Petani dan Nelayan kemungkinan berbeda.

### 8.2 Data Auto-Fill

Data yang dapat otomatis terisi:

- No. BAST dari nomor urut Excel atau nomor manual,
- tanggal default dari tanggal hari ini atau tanggal kegiatan,
- kota/kabupaten dari jadwal kegiatan,
- kode kabupaten dari master kabupaten,
- nama kegiatan dari settings,
- tahun anggaran dari settings,
- daftar komponen paket dari settings,
- nama pelaksana/pengawas default dari settings.

Tanggal tetap dapat diubah manual melalui date picker jika tanggal dokumen berbeda dari default.

### 8.3 Data Yang Dilengkapi Per Penerima

Data yang mungkin tetap perlu diinput atau dilengkapi:

- nama penerima,
- alamat,
- No. KTP,
- No. Kartu Petani/Nelayan,
- No. HP,
- merk mesin jika berbeda,
- tipe mesin jika berbeda,
- serial number mesin,
- serial number selang,
- serial number regulator/reducer,
- checklist kelengkapan,
- nama penerima untuk tanda tangan,
- catatan dokumen.

Sebagian data di atas bisa berasal dari Excel. Jika sudah ada, form hanya menampilkan data tersebut untuk dicek atau diedit.

### 8.4 Output BAST

Output yang disarankan:

- preview BAST sebelum cetak,
- download PDF per penerima,
- cetak satuan,
- cetak massal berdasarkan jadwal/kabupaten,
- arsip PDF per penerima,
- riwayat perubahan data BAST.

## 9. Modul Sistem

Modul awal yang disarankan:

### 9.1 Dashboard

Menampilkan ringkasan:

- total kabupaten,
- program aktif,
- jumlah calon penerima,
- jumlah penerima valid,
- jumlah pemasangan,
- jumlah BAST selesai,
- progres per kabupaten,
- progres per jenis program.

### 9.2 Master Kabupaten

Mengelola:

- nama kabupaten,
- kode kabupaten 3 huruf,
- provinsi,
- status,
- catatan.

### 9.3 Jadwal Program

Mengelola:

- kabupaten,
- jenis program,
- tanggal mulai,
- tanggal selesai,
- status kegiatan,
- tahun anggaran,
- setting dokumen terkait,
- catatan.

### 9.4 Program Petani

Mengelola data program petani:

- calon penerima petani,
- data alat/mesin pertanian,
- data pemasangan,
- serial number komponen,
- status penerima,
- dokumen BAST.

### 9.5 Program Nelayan

Mengelola data program nelayan:

- calon penerima nelayan,
- data kapal/perahu,
- data mesin,
- data pemasangan,
- serial number komponen,
- status penerima,
- dokumen BAST.

### 9.6 Import Excel

Mengelola:

- upload Excel,
- preview data,
- mapping kolom,
- validasi,
- simpan hasil import,
- riwayat import.

### 9.7 Persiapan Dokumen

Mengelola:

- template BAST Petani,
- template BAST Nelayan,
- format nomor dokumen,
- daftar komponen paket,
- data pihak pelaksana/pengawas,
- logo dan identitas dokumen.

### 9.8 Laporan

Filter laporan:

- kabupaten,
- jenis program,
- periode,
- status penerima,
- status pemasangan,
- status BAST.

Output:

- tampilan tabel,
- export Excel,
- export PDF ringkasan jika dibutuhkan.

## 10. Struktur Database Awal

Struktur awal yang disarankan:

- `regencies`
- `program_schedules`
- `beneficiaries`
- `farmer_profiles`
- `fisherman_profiles`
- `machines`
- `installations`
- `konkit_components`
- `document_templates`
- `document_template_items`
- `bast_documents`
- `import_batches`
- `import_rows`
- `system_settings`
- `activity_logs`

### 10.1 `regencies`

Menyimpan master kabupaten.

Field awal:

- id,
- name,
- document_code,
- province,
- is_active,
- notes.

### 10.2 `program_schedules`

Menyimpan jadwal kegiatan.

Field awal:

- id,
- regency_id,
- program_type,
- start_date,
- end_date,
- status,
- fiscal_year,
- document_template_id,
- notes.

`program_type` berisi `farmer` atau `fisherman`.

### 10.3 `beneficiaries`

Menyimpan data umum calon/penerima.

Field awal:

- id,
- program_schedule_id,
- import_batch_id,
- sequence_number,
- bast_number,
- name,
- address,
- village,
- district,
- identity_number,
- sector_card_number,
- phone_number,
- status,
- notes.

### 10.4 `farmer_profiles`

Menyimpan data khusus petani.

Field awal:

- id,
- beneficiary_id,
- farmer_group_name,
- agricultural_tool_type,
- land_location,
- notes.

### 10.5 `fisherman_profiles`

Menyimpan data khusus nelayan.

Field awal:

- id,
- beneficiary_id,
- boat_name,
- boat_registration_number,
- fishing_card_number,
- notes.

### 10.6 `machines`

Menyimpan data mesin penerima.

Field awal:

- id,
- beneficiary_id,
- machine_brand,
- machine_type,
- machine_serial_number,
- fuel_type_before_conversion,
- notes.

### 10.7 `installations`

Menyimpan proses pemasangan.

Field awal:

- id,
- beneficiary_id,
- installation_date,
- status,
- installer_name,
- supervisor_name,
- notes.

### 10.8 `bast_documents`

Menyimpan data BAST per penerima.

Field awal:

- id,
- beneficiary_id,
- document_template_id,
- bast_number,
- bast_date,
- regency_name_snapshot,
- regency_code_snapshot,
- generated_pdf_path,
- status,
- notes.

Snapshot digunakan agar dokumen lama tetap konsisten meskipun master data berubah.

### 10.9 `import_batches`

Menyimpan riwayat import Excel.

Field awal:

- id,
- program_schedule_id,
- original_filename,
- imported_by,
- imported_at,
- total_rows,
- valid_rows,
- invalid_rows,
- status,
- notes.

### 10.10 `import_rows`

Menyimpan detail hasil pembacaan Excel.

Field awal:

- id,
- import_batch_id,
- row_number,
- raw_data_json,
- validation_status,
- validation_message.

## 11. Format Nomor BAST

Format nomor BAST dibuat dinamis melalui settings.

Contoh token:

```text
{urut}
{urut_padded}
{kode_kabupaten}
{kode_program}
{bulan}
{bulan_romawi}
{tahun}
{vendor}
{jenis_program}
```

Contoh format:

```text
{urut_padded}/1578/KSM-KKT-{kode_kabupaten}/{bulan_romawi}/{tahun}
```

Contoh hasil:

```text
0002/1578/KSM-KKT-WJO/XII/2024
```

Token dapat dikembangkan setelah format dokumen final diketahui.

## 12. Tech Stack Rekomendasi

Tech stack yang disarankan:

- Backend: Laravel
- Database: MySQL atau MariaDB
- Admin UI: Filament
- Frontend: Blade + Tailwind CSS
- Import Excel: Laravel Excel
- PDF: DomPDF, Browsershot, atau renderer HTML-to-PDF lain sesuai hasil uji dokumen
- Storage: local storage server pada tahap awal
- Deployment: VPS ringan atau shared hosting yang mendukung Laravel

Alasan:

- ringan untuk sistem administrasi,
- mudah dikembangkan jangka panjang,
- banyak developer Indonesia familiar dengan Laravel,
- cocok untuk desktop dan mobile,
- Filament mempercepat pembuatan dashboard admin,
- biaya hosting relatif terkendali.

## 13. Desain Tampilan

Prinsip tampilan:

- responsive untuk desktop dan mobile,
- fokus sebagai aplikasi kerja/admin,
- navigasi sederhana,
- tabel mudah difilter,
- form tidak terlalu panjang dalam satu layar,
- data auto-fill ditampilkan jelas tetapi tetap bisa diedit bila diperlukan,
- tombol cetak/download dokumen mudah ditemukan.

Menu awal:

- Dashboard
- Kabupaten
- Jadwal Program
- Program Petani
- Program Nelayan
- Import Excel
- BAST Generator
- Laporan
- System Settings

## 14. Roadmap Pengembangan

### Fase 1: Fondasi Admin

- login Super Admin,
- dashboard awal,
- master kabupaten,
- kode kabupaten 3 huruf,
- jadwal program,
- jenis program Petani/Nelayan.

### Fase 2: Data Penerima Dan Import Excel

- input manual calon penerima,
- import Excel,
- preview dan mapping kolom,
- validasi data,
- status calon penerima.

### Fase 3: BAST Generator

- settings template BAST,
- format nomor BAST,
- auto-fill data,
- preview dokumen,
- export/cetak PDF.

### Fase 4: Data Teknis Dan Pemasangan

- data mesin,
- serial number komponen,
- status pemasangan,
- checklist kelengkapan,
- arsip dokumen.

### Fase 5: Laporan

- filter laporan,
- export Excel,
- rekap per kabupaten,
- rekap per jenis program,
- rekap status pemasangan dan BAST.

## 15. Catatan Untuk Diskusi Berikutnya

Hal yang masih perlu dijelaskan pada sesi berikutnya:

- detail field khusus Konkit Nelayan,
- detail field khusus Konkit Petani,
- format final BAST Petani,
- format final BAST Nelayan,
- apakah tanda tangan akan tetap manual atau digital,
- apakah dokumen perlu dicetak kosong sebagian atau selalu penuh dari sistem,
- format Excel yang paling sering diberikan dinas,
- status proses yang benar sesuai operasional lapangan,
- kebutuhan role selain Super Admin.
