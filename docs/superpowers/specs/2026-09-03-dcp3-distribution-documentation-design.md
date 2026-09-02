# Rancangan DCP3, Pendistribusian, dan Dokumentasi Konkit

Versi: 0.1  
Tanggal: 2026-09-03  
Status: Disetujui untuk implementasi prototipe

## 1. Tujuan

Fase ini membangun alur operasional dari penerimaan Data Calon Penerima Paket Perdana (DCP3), pencarian calon di lokasi pembagian, verifikasi kelayakan lintas kabupaten, peralihan penerima, dokumentasi foto, hingga data siap dipakai oleh BAST dan DP3.

Seluruh data berada dalam satu PostgreSQL. Kabupaten, jadwal, dan batch DCP3 menjadi konteks data, bukan database terpisah.

## 2. Prinsip Utama

- DCP3 adalah daftar pencalonan, bukan bukti bahwa paket sudah diterima.
- Identitas orang disimpan terpusat agar riwayat lintas kabupaten dapat ditemukan.
- Slot paket berasal dari DCP3 dan tetap memiliki calon awal walaupun penerima diganti.
- Penerima aktual dicatat terpisah dari calon awal.
- Pemeriksaan duplikasi dilakukan saat import, saat pencarian lapangan, dan secara final saat konfirmasi distribusi.
- Template dan nilainya disimpan di database serta memiliki versi.
- Dokumen yang diterbitkan memakai snapshot agar arsip lama tidak berubah.
- Foto dan PDF berada di file/object storage; database menyimpan metadata, status, dan lokasi file.
- Semua pengecualian dan perubahan sensitif masuk audit log.

## 3. Batas Modul

Fase operasional dibagi menjadi modul berikut:

1. Program, jadwal, dan kabupaten.
2. Template paket dan template dokumen.
3. Import dan batch DCP3.
4. Identitas orang dan profil sektor.
5. Slot alokasi paket.
6. Pendistribusian dan verifikasi penerima.
7. Peralihan penerima.
8. Template slot dokumentasi dan unggahan foto.
9. Kelayakan penerimaan berulang dan persetujuan pusat.
10. Dataset dokumen untuk BAST dan DP3.

## 4. Template dan Snapshot

### 4.1 Template Paket

Template paket menyimpan nilai tipikal yang berlaku untuk banyak penerima:

- jenis program Petani atau Nelayan,
- merek dan tipe mesin,
- merek konverter,
- merek regulator/reducer,
- spesifikasi selang,
- daftar komponen dan aksesori,
- jumlah dan satuan,
- dokumen garansi dan buku manual.

Template mempunyai status `draft`, `published`, atau `retired`. Template yang sudah dipakai tidak diedit secara destruktif; perubahan membuat versi baru.

### 4.2 Template Dokumen

Template dokumen menyimpan judul, kalimat baku, logo, struktur bagian, token penomoran, pihak penandatangan, dan pemetaan field. BAST dan DP3 merupakan jenis template berbeda, tetapi dapat membaca dataset operasional yang sama.

Format DP3 belum ditentukan. Sistem tidak akan menebak layout DP3, tetapi data sumbernya disimpan secara terstruktur agar dapat dipetakan setelah contoh dokumen tersedia.

### 4.3 Penetapan ke Jadwal

Jadwal kabupaten memilih versi template paket, BAST, kebutuhan foto, dan kebijakan penerimaan. Jadwal dapat memiliki override terbatas. Saat distribusi atau dokumen diterbitkan, nilai efektif disalin menjadi snapshot.

## 5. DCP3 dan Nomor Pembagian

Setiap file yang diterima dicatat sebagai `DCP3 batch` dengan:

- jadwal dan kabupaten,
- nama file asli,
- sumber/pengirim,
- tanggal diterima dan diimport,
- mapping kolom,
- jumlah baris valid, peringatan, dan gagal,
- checksum file untuk mendeteksi import ulang,
- pengguna yang melakukan import.

Sistem menyimpan tiga nomor berbeda:

- `source_row_number`: posisi baris Excel.
- `source_sequence_number`: nomor urut yang tertulis pada DCP3.
- `distribution_number`: nomor pembagian operasional yang unik dalam satu jadwal.

Pemisahan ini memungkinkan DCP3 datang beberapa kali dan setiap file dapat kembali mulai dari nomor `1` tanpa merusak nomor sumber. Sebelum import disimpan, sistem menampilkan usulan nomor pembagian dan konflik nomor yang ditemukan.

## 6. Identitas Orang

Identitas umum disimpan pada satu entitas `person`:

- nama lengkap,
- NIK,
- alamat dan wilayah,
- nomor telepon,
- tanggal lahir jika tersedia,
- catatan verifikasi.

Identitas sektor disimpan terpisah:

- `farmer_card_number` untuk Petani.
- `kusuka_number` untuk Nelayan.

NIK menjadi pencocokan terkuat. Nomor Kartu Petani dan KUSUKA menjadi pencocokan tambahan. Nomor telepon dan nama hanya menghasilkan kandidat kemiripan, bukan keputusan identitas otomatis.

Jika NIK cocok tetapi nomor kartu berbeda, atau nomor kartu sudah terhubung ke orang lain, baris masuk status `needs_review`. Sistem tidak menggabungkan orang secara otomatis pada konflik tersebut.

## 7. Pencalonan dan Slot Alokasi

Kemunculan seseorang dalam DCP3 disimpan sebagai `candidate nomination`. Hak paket disimpan sebagai `package allocation` yang berisi:

- batch dan baris sumber,
- calon awal,
- nomor pembagian,
- template paket efektif,
- status alokasi,
- penerima aktual jika sudah ditentukan.

Status awal alokasi:

- `candidate`: belum diverifikasi lapangan,
- `ready`: data cukup dan layak diproses,
- `needs_review`: ada konflik atau data kurang,
- `distributed`: distribusi selesai,
- `replaced`: paket diterima pengganti,
- `cancelled`: alokasi dibatalkan.

Calon awal tidak pernah ditimpa oleh data pengganti.

## 8. Pemeriksaan Kelayakan

Pemeriksaan dilakukan pada tiga tahap:

1. Import DCP3 memberi tanda awal terhadap identitas dan penerimaan sebelumnya.
2. Pemilihan penerima di halaman Pendistribusian mengambil kondisi terbaru lintas kabupaten.
3. Konfirmasi distribusi mengulang pemeriksaan di dalam transaksi database dan mengunci identitas/alokasi terkait.

Pemeriksaan final mencegah dua petugas mengonfirmasi orang atau slot yang sama secara bersamaan.

Hasil kelayakan:

- `eligible`: dapat diproses,
- `incomplete`: data wajib belum lengkap,
- `previously_received`: pernah menerima dan diblokir,
- `approval_required`: memerlukan persetujuan pusat,
- `override_approved`: pengecualian masih berlaku,
- `identity_conflict`: identitas perlu ditinjau.

Kebijakan penerimaan dapat diatur menjadi larangan menyeluruh, per jenis program, per kategori paket, menggunakan tenggat waktu, atau memerlukan persetujuan pusat. Sampai kebijakan final ditentukan, default sistem adalah memblokir penerimaan kedua dan meminta persetujuan pusat.

## 9. Halaman Dokumentasi - Pendistribusian

Halaman bekerja dalam konteks satu jadwal kabupaten. Kolom pencarian menerima:

- nomor pembagian,
- nama,
- NIK,
- Nomor Kartu Petani,
- Nomor KUSUKA.

Pencarian nama berjalan setelah minimal dua karakter dengan debounce. Hasil nomor pembagian atau identitas eksak diprioritaskan. Dropdown menampilkan nomor pembagian, nama, NIK tersamarkan, desa/kecamatan, jenis program, dan label status.

Label ringkas:

- hijau: siap,
- kuning: data belum lengkap/perlu verifikasi,
- merah: sudah menerima atau diblokir,
- biru: pengecualian disetujui,
- abu-abu: batal atau digantikan.

Setelah dipilih, layar menampilkan:

- data DCP3 untuk diperiksa,
- field kosong yang harus dilengkapi,
- penanda field yang diubah dari DCP3,
- hasil pemeriksaan identitas,
- riwayat penerimaan dalam panel yang dapat dibuka,
- slot foto sesuai template,
- aksi simpan draft atau konfirmasi distribusi.

Perubahan NIK atau nomor kartu membutuhkan alasan dan dicatat sebagai nilai sebelum/sesudah dalam audit log.

## 10. Peralihan Penerima

Petugas memilih aksi `Proses penerima pengganti`, bukan mengganti nama calon secara langsung. Peralihan menyimpan:

- alokasi dan calon awal,
- penerima aktual,
- alasan dan catatan,
- tanggal peralihan,
- pengusul dan pemberi persetujuan,
- lampiran pendukung,
- hasil pemeriksaan kelayakan pengganti.

Nomor pembagian tetap milik slot alokasi. BAST dan DP3 dapat menampilkan calon awal serta penerima aktual sesuai kebutuhan dokumen.

## 11. Penerimaan Berulang dan Override

Riwayat penerimaan ditampilkan lintas kabupaten dan program. Penerimaan kedua tidak dapat dikonfirmasi tanpa aturan yang mengizinkan atau override pusat yang masih berlaku.

Override menyimpan:

- orang dan program tujuan,
- penerimaan sebelumnya,
- alasan,
- pemberi persetujuan,
- tanggal dan masa berlaku,
- lampiran,
- waktu digunakan.

Override tidak menghapus peringatan riwayat dan hanya berlaku pada cakupan yang disetujui.

## 12. Template Dokumentasi

Super Admin dapat membuat versi kebutuhan dokumentasi per jenis program atau jadwal. Setiap slot memiliki:

- kode stabil,
- judul yang dapat diubah,
- tahap proses,
- wajib atau opsional,
- jumlah minimal dan maksimal,
- sumber kamera, galeri, atau keduanya,
- kebutuhan koordinat dan waktu pengambilan,
- instruksi internal.

Contoh slot:

- penerima dan paket,
- identitas penerima,
- serial number mesin,
- kelengkapan paket,
- proses pemasangan,
- BAST bertanda tangan.

Status slot adalah `missing`, `local_draft`, `uploading`, `complete`, atau `rejected`. Daftar penerima menampilkan indikator ringkas merah/hijau per slot dan panel detail untuk melihat file atau alasan penolakan.

Distribusi dapat disimpan sebagai draft, tetapi hanya dapat diselesaikan jika semua slot wajib lengkap dan pemeriksaan final berhasil.

## 13. Penyimpanan Media

Database menyimpan:

- pemilik dan jenis slot,
- lokasi file,
- nama dan tipe file,
- ukuran dan checksum,
- waktu pengambilan,
- koordinat jika diwajibkan,
- sumber kamera/galeri,
- status review,
- pengunggah dan waktu upload.

File asli tidak disimpan sebagai byte besar di PostgreSQL. Local development memakai folder storage lokal; deployment Docker dapat memakai volume dan kemudian dipindahkan ke object storage tanpa mengubah kontrak data.

## 14. Dukungan Offline

Pendistribusian disiapkan untuk mode PWA bertahap. Data jadwal, calon yang ditugaskan, template slot, dan draft foto dapat disimpan di IndexedDB. Setiap perubahan mendapat ID dari perangkat dan status sinkronisasi.

Konfirmasi final tetap membutuhkan pemeriksaan server. Saat offline, petugas hanya dapat menyimpan draft; status distribusi belum menjadi final sampai sinkronisasi dan pemeriksaan kelayakan berhasil.

## 15. Struktur Data Konseptual

Entitas utama:

- `programs`
- `program_schedules`
- `regencies`
- `package_template_versions`
- `document_template_versions`
- `documentation_template_versions`
- `eligibility_policies`
- `dcp3_import_batches`
- `dcp3_import_rows`
- `people`
- `person_sector_identifiers`
- `candidate_nominations`
- `package_allocations`
- `distribution_records`
- `recipient_replacements`
- `eligibility_checks`
- `eligibility_overrides`
- `documentation_slots`
- `media_files`
- `document_snapshots`

Relasi detail akan ditetapkan pada implementation plan setelah spesifikasi ini disetujui.

## 16. Keamanan dan Audit

- NIK disamarkan pada hasil pencarian dan hanya ditampilkan penuh kepada role yang berwenang.
- Pencarian identitas diberi rate limit dan audit untuk aktivitas sensitif.
- Media tidak dapat diakses hanya dengan menebak URL.
- Aksi distribusi final, peralihan, override, perubahan identitas, perubahan template, dan penghapusan media dicatat.
- Catatan distribusi final tidak dihapus; koreksi dilakukan melalui pembatalan atau revisi yang berjejak.

## 17. Tahapan Implementasi

Fase ini terlalu besar untuk satu perubahan sekaligus. Urutan implementasi:

1. Program, kabupaten, jadwal, dan template dasar.
2. Identitas orang, batch DCP3, preview, mapping, dan import.
3. Pencarian realtime dan halaman verifikasi pendistribusian.
4. Riwayat penerimaan, eligibility check, dan penguncian transaksi.
5. Peralihan penerima dan persetujuan pusat.
6. Template slot foto, kamera/galeri, dan indikator kelengkapan.
7. Snapshot BAST dan dataset awal DP3.
8. Draft offline dan sinkronisasi.

Hasil pertama yang dapat diuji sebaiknya mencakup langkah 1 sampai 3 menggunakan data contoh, agar alur DCP3 dan pendistribusian dapat dievaluasi sebelum aturan dokumen dan offline diperluas.

## 18. Keputusan Sementara

- Default penerimaan kedua adalah diblokir dan membutuhkan persetujuan pusat.
- NIK adalah identitas utama; Kartu Petani dan KUSUKA adalah identitas sektor tambahan.
- Nomor sumber DCP3 dipisahkan dari nomor pembagian operasional.
- DP3 memakai data terstruktur dan snapshot, bukan mengekstrak ulang data dari PDF.
- Detail aturan penerimaan lintas Petani/Nelayan akan dikonfirmasi setelah prototipe alur dapat dicoba.
