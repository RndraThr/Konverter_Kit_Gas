# Desain Normalisasi Huruf Kapital Data Bisnis

**Tanggal:** 2 Oktober 2026  
**Status:** Menunggu persetujuan implementasi  
**Cakupan:** Seluruh modul Konkit yang menerima atau mencetak data bisnis

## 1. Tujuan

Menyeragamkan data bisnis baru agar menggunakan huruf kapital sejak pengguna mengetik, tetap tersimpan kapital ketika data masuk lewat API atau impor, dan selalu dicetak kapital pada PDF. Data lama di database tidak dimigrasikan.

## 2. Prinsip

Normalisasi dilakukan berlapis:

1. **Frontend:** field bisnis berubah menjadi kapital saat pengguna mengetik.
2. **Backend:** create, update, dan impor menormalisasi ulang field bisnis sebelum validasi dan penyimpanan.
3. **PDF:** renderer mengapitalisasi nilai bisnis saat render agar data lama tetap tercetak seragam.

Frontend bukan sumber kebenaran. Backend tetap menjadi batas kanonikal agar request langsung, klien lama, dan impor tidak dapat melewati aturan.

## 3. Cakupan Field

### 3.1 Dikapitalisasi

- Nama orang dan nama penerima.
- Nama perusahaan, instansi, dinas, pelaksana, pengawas, dan perwakilan.
- Alamat, desa/kelurahan, kecamatan, kabupaten/kota, provinsi, zona, serta lokasi/titik serah.
- Nama program, nama jadwal, judul/label bisnis, dan label komponen paket.
- Merek mesin/selang/konkit, tipe atau spesifikasi alat, daya, dan jenis BBM.
- Nomor seri alat yang dapat mengandung huruf.
- Nomor kartu sektor atau identitas sektor yang dapat mengandung huruf.
- Nilai teks bisnis dari impor DCP3 setelah parsing.
- Nilai bisnis yang ditampilkan pada PDF DP3, Rekapitulasi Harian, dan BA Perorangan.

Kapitalisasi menggunakan operasi Unicode. Spasi tepi tetap dipangkas oleh normalisasi domain yang sudah ada.

### 3.2 Tidak Dikapitalisasi

- Email, password, username, token, dan secret.
- URL, path, nama file, MIME type, storage key, checksum, serta metadata teknis.
- UUID dan identifier internal.
- Kode template, key konfigurasi, `machine_option.code`, kode komponen, kode slot, dan enum/status internal.
- Tanggal, angka, NIK, nomor telepon, serta nilai numerik lain. Karakter nonhuruf dibiarkan apa adanya.
- Isi bebas yang secara eksplisit merupakan data teknis case-sensitive.

## 4. Arsitektur

### 4.1 Frontend

Disediakan helper tunggal untuk normalisasi input bisnis, misalnya `uppercaseBusinessText(value)`. Form yang berada dalam cakupan memanggil helper pada `onChange`, sehingga nilai state dan tampilan input langsung kapital.

Komponen input generik tidak mengapitalisasi semua nilai secara default. Kapitalisasi harus dinyatakan oleh form atau prop khusus agar email, password, URL, dan kode internal tidak rusak.

Area awal yang wajib dicakup:

- Data Penerima.
- Persiapan Program: kabupaten, program, zona, jadwal, dan nilai tampilan template paket.
- DCP3: hasil normalisasi baris impor.
- Pendistribusian: nomor seri dan teks bisnis yang dapat diedit.
- Pengaturan Berita Acara per jadwal.
- Field bisnis lain yang ditemukan melalui audit form.

### 4.2 Backend

Normalisasi ditempatkan di boundary domain masing-masing input, bukan middleware JSON global. Setiap service atau model input hanya mengapitalisasi field bisnis yang dimilikinya sebelum validasi dan repository dipanggil.

Untuk impor DCP3, kapitalisasi dilakukan pada hasil `NormalizeRow`, bukan pada header, nama kolom mapping, atau source snapshot mentah. Source snapshot tetap menyimpan nilai asli untuk audit, sedangkan nilai normalized dan data penerima menggunakan kapital.

Template key dan option code tidak diubah. Hanya label serta atribut tampilan seperti brand, type, power, fuel type, dan specification yang dikapitalisasi.

### 4.3 PDF

Renderer memakai helper presentasi bersama untuk mengapitalisasi seluruh nilai bisnis yang dicetak. Snapshot dokumen tetap menyimpan nilai hasil domain saat finalisasi; renderer tidak mengubah database.

Lapisan ini memastikan record lama yang masih bercampur huruf kecil tetap tampil kapital pada:

- BA Perorangan.
- DP3.
- Rekapitulasi Harian.

Nama file dan storage key tidak ikut diubah di luar format penamaan yang sudah ada.

## 5. Perilaku Data Lama

- Tidak ada migration massal `UPDATE ... UPPER(...)`.
- Record lama tetap seperti sekarang di database.
- Jika record lama diedit dan disimpan, field bisnis yang termasuk request tersebut menjadi kapital.
- PDF mengapitalisasi record lama hanya pada output render.

## 6. Kompatibilitas dan Risiko

- Pencarian tetap case-insensitive seperti perilaku yang sudah ada.
- Kode internal tidak boleh berubah karena digunakan sebagai key relasi dan snapshot.
- Source snapshot impor dipertahankan asli agar audit tidak kehilangan bentuk sumber.
- Kapitalisasi tidak boleh mengubah panjang atau isi angka identitas.
- Perubahan frontend dan backend harus dirilis bersama agar pengalaman mengetik sesuai dengan data yang tersimpan.

## 7. Pengujian

Implementasi mengikuti TDD dan minimal mencakup:

1. Test helper frontend untuk Unicode, angka, dan spasi.
2. Test form representatif yang membuktikan input langsung tampil kapital.
3. Test backend setiap boundary input yang berubah agar field bisnis tersimpan kapital dan field teknis tetap identik.
4. Test impor DCP3: normalized value kapital, source snapshot tetap asli, dan mapping header tidak berubah.
5. Test renderer untuk memastikan data lama bercampur case dicetak kapital pada ketiga jenis PDF.
6. Full backend test, frontend test, typecheck, lint, build, dan pemeriksaan visual PDF.

## 8. Kriteria Penerimaan

- Pengguna melihat huruf kapital langsung ketika mengetik pada semua field bisnis yang dicakup.
- Request API dengan huruf kecil menghasilkan data bisnis tersimpan kapital.
- Impor DCP3 menghasilkan data normalized kapital tanpa merusak raw source atau mapping.
- Email, password, URL, file/storage key, UUID, dan kode internal tidak berubah.
- Data lama tidak dimutasi secara massal.
- Semua PDF BA menampilkan nilai bisnis dalam huruf kapital, termasuk nilai dari record lama.
- Tidak ada regresi pada pencarian, mounting penerima, pemilihan varian mesin, upload media, dan versioning dokumen.

