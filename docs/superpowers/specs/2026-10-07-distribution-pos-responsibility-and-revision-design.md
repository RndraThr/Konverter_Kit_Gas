# Rancangan Pemisahan Tanggung Jawab POS dan Revisi Data Distribusi

Versi: 0.1  
Tanggal: 2026-10-07  
Status: Disetujui untuk implementation plan pada 2026-10-07

## 1. Latar Belakang

Alur lapangan menjalankan POS Mesin lebih dahulu, tetapi data mesin, converter, dan selang baru lebih tepat dilengkapi bersama data penerima di POS Dokumen. POS Mesin karena itu perlu disederhanakan menjadi tempat menentukan tanggal distribusi dan mengumpulkan dokumentasi tahap mesin.

Sistem juga harus tetap fleksibel: data yang sudah disimpan, termasuk slot yang sudah selesai, dapat diperbaiki oleh petugas POS yang bertanggung jawab. Perubahan setelah selesai tidak boleh diam-diam membuat snapshot verifikasi atau Berita Acara (BA) lama terlihat seolah masih mutakhir.

## 2. Tujuan

- POS Mesin hanya mengelola tanggal distribusi dan dokumentasi tahap `mesin`.
- POS Dokumen mengelola penerima, data mesin/converter/selang, dan dokumentasi tahap `dokumen`.
- POS Penyerahan mengelola dokumentasi tahap `penyerahan` dan menyelesaikan slot.
- Setiap POS dapat memperbaiki data miliknya.
- Slot yang sudah selesai dapat dibuka sebagai revisi dengan alasan dan audit yang jelas.
- Struktur Google Drive tetap berbasis tanggal distribusi dan memakai ulang folder yang sudah ada.
- Snapshot dan BA yang terdampak revisi ditandai perlu dibuat ulang.

## 3. Alur Utama

### 3.1 POS Mesin

Saat petugas memilih nomor slot yang belum pernah digunakan:

1. Petugas mengisi `distribution_date`.
2. Sistem membuat slot berstatus `open` tanpa mewajibkan data peralatan.
3. Petugas mengunggah dokumentasi yang template-nya memiliki `stage='mesin'`.

Form POS Mesin tidak lagi menampilkan:

- opsi/jenis mesin;
- nomor seri mesin;
- opsi converter;
- nomor seri converter;
- opsi selang;
- nomor seri selang.

Tanggal tetap berada di POS Mesin karena tahap ini berjalan lebih dahulu dan tanggal menentukan lokasi folder Drive.

### 3.2 POS Dokumen

POS Dokumen memuat dua kelompok data:

1. **Data penerima**: pencarian/pengaitan penerima serta data yang saat ini dapat diperbarui ketika penerima dikaitkan, seperti alamat, desa, kecamatan, telepon, dan nomor identitas sektor.
2. **Data peralatan**: opsi/jenis dan nomor seri mesin, converter, serta selang. Nomor seri selang tetap boleh berisi `-` sesuai kebutuhan lapangan.

POS Dokumen juga mengelola dokumentasi dengan `stage='dokumen'`.

Pengaitan pertama mengubah status `open` menjadi `linked`. Setelah penerima sudah terkait, POS Dokumen menyediakan aksi eksplisit **Edit data penerima** dan **Ganti penerima**:

- edit data penerima memperbarui field milik orang/alokasi yang sedang terkait;
- ganti penerima dilakukan atomik: alokasi lama dikembalikan ke kondisi sebelum dikaitkan, kandidat baru divalidasi belum digunakan, lalu slot dikaitkan ke alokasi baru;
- NIK tidak diedit sebagai teks bebas. Perubahan NIK dilakukan melalui aksi Ganti penerima agar integritas DCP3/alokasi tetap terjaga.

### 3.3 POS Penyerahan

POS Penyerahan mengelola dokumentasi dengan `stage='penyerahan'`. Penyelesaian slot hanya berhasil jika:

- penerima sudah terkait;
- seluruh field peralatan yang diwajibkan valid;
- seluruh dokumentasi wajib lintas ketiga tahap memenuhi `min_files`.

Saat selesai, sistem membangun ulang `verification_snapshot_json`, mengubah slot menjadi `completed`, dan mengubah alokasi menjadi `distributed`.

## 4. Aturan Edit dan Revisi

### 4.1 Slot belum selesai

Selama status slot `open` atau `linked`, setiap POS dapat mengubah domainnya sendiri tanpa membuka revisi:

- POS Mesin: tanggal dan media tahap mesin;
- POS Dokumen: penerima, peralatan, dan media tahap dokumen;
- POS Penyerahan: media tahap penyerahan dan aksi penyelesaian.

Data peralatan tidak lagi dikunci setelah media tahap mesin diunggah.

### 4.2 Slot sudah selesai

Sebelum mengubah atau menghapus data pada slot `completed`, petugas harus menekan **Buka revisi** dan mengisi alasan. Endpoint revisi menerima tahap asal (`mesin`, `dokumen`, atau `penyerahan`) dan hanya dapat dipanggil oleh petugas yang memiliki permission POS tersebut.

Pembukaan revisi dilakukan dalam satu transaksi dan:

- mengubah status slot dari `completed` menjadi `linked`;
- mengubah alokasi dari `distributed` menjadi `ready`;
- mengosongkan `distributed_at`, `distributed_by`, dan `completed_at`;
- mengosongkan snapshot verifikasi final yang harus dihitung ulang;
- mengaktifkan penanda `needs_recompletion`;
- menyimpan `reopened_at`, `reopened_by`, `reopened_stage`, dan `revision_reason`;
- mencatat audit event `distribution.slot_reopened`.

Setelah dibuka, petugas dapat mengubah domain POS-nya seperti biasa. Slot menampilkan badge **Perlu diselesaikan ulang** sampai POS Penyerahan menyelesaikannya kembali. Penyelesaian ulang membangun snapshot terbaru, mengubah alokasi menjadi `distributed`, mengembalikan status `completed`, dan mematikan `needs_recompletion`.

Pembukaan revisi bersifat global untuk slot. Artinya, setelah salah satu POS membukanya, POS lain tidak perlu membuka revisi kedua untuk memperbaiki domainnya sebelum slot diselesaikan ulang. Audit setiap perubahan tetap mengikuti mekanisme audit yang ada.

## 5. Aturan Tanggal dan Google Drive

### 5.1 Penguncian tanggal

Tanggal dapat diedit selama slot belum memiliki media yang diterima pada tahap mana pun. Begitu ada satu media, tanggal dikunci untuk mencegah data database menunjuk tanggal yang berbeda dari folder Drive.

Jika tanggal salah setelah upload:

1. jika slot sudah selesai, buka revisi lebih dahulu;
2. hapus seluruh media slot dari semua tahap;
3. ubah tanggal;
4. upload ulang media ke tanggal yang benar.

Petugas hanya dapat menghapus media pada tahap POS miliknya. Karena itu, pengosongan seluruh media dapat membutuhkan kerja sama ketiga POS atau pengguna koordinator yang memiliki semua permission POS.

### 5.2 Pemakaian ulang folder

Path tetap mengikuti pola:

`{jenis program}/{zona}/{kabupaten}/DOKUMENTASI (FOTO)/PENDISTRIBUSIAN/{TANGGAL}/{NOMOR SLOT}`

Resolver folder wajib idempotent:

- jika folder tanggal sudah ada, folder tersebut dipakai ulang;
- jika subfolder nomor slot sudah ada di bawah tanggal itu, subfolder tersebut dipakai ulang;
- foto/video baru langsung dimasukkan ke subfolder slot yang sama;
- folder baru hanya dibuat jika segmen yang dibutuhkan memang belum ada.

Folder lama yang menjadi kosong setelah semua medianya dihapus tidak dihapus otomatis. Ini menghindari operasi destruktif terhadap folder yang mungkin berisi file audit atau file yang dibuat manual.

## 6. Permission dan Keamanan Backend

Validasi tidak boleh hanya bergantung pada komponen frontend. Backend menerapkan matriks berikut:

| Operasi | Permission wajib |
|---|---|
| Buat slot, ubah tanggal, media `mesin`, buka revisi dari POS Mesin | `distribution.pos_mesin` |
| Kaitkan/edit/ganti penerima, ubah peralatan, media `dokumen`, buka revisi dari POS Dokumen | `distribution.pos_dokumen` |
| Media `penyerahan`, selesaikan slot, buka revisi dari POS Penyerahan | `distribution.pos_penyerahan` |

Untuk media distribusi, `documentation.manage` saja tidak cukup untuk melewati batas tahap. Endpoint upload/delete juga memverifikasi permission POS yang sesuai dengan `documentation_slots.stage`. Koordinator atau super admin dapat mengelola semua tahap karena memiliki seluruh permission POS.

Permintaan dengan tahap yang tidak cocok, slot di luar cakupan kabupaten pengguna, atau revisi tanpa alasan ditolak oleh backend.

## 7. Dampak terhadap Snapshot dan Berita Acara

Dokumen final yang sudah pernah dibuat tetap disimpan sebagai riwayat dan tidak ditimpa atau dihapus. Ketika slot selesai dibuka untuk revisi:

- BA perorangan aktif untuk slot tersebut ditandai stale/superseded;
- bundle harian pada tanggal lama yang memuat BA tersebut ditandai perlu regenerasi;
- dokumen agregat aktif yang mengambil data jadwal/tanggal terkait (DP3, rekap harian, closing titik serah, dan closing kabupaten sesuai cakupannya) ditandai stale;
- preview berikutnya membaca data live dan hanya memasukkan slot yang kembali `completed`;
- finalisasi setelah slot diselesaikan ulang membuat versi dokumen baru, sementara versi lama tetap dapat diunduh sebagai riwayat.

Penandaan invalidasi dilakukan saat revisi dibuka, menggunakan tanggal lama sebelum metadata penyelesaian dikosongkan. Jika tanggal kemudian diubah, finalisasi baru menggunakan tanggal baru.

Implementasi boleh memakai status tambahan `stale` atau metadata `stale_at/stale_reason`, menyesuaikan tabel dokumen terkait. Pilihan konkretnya ditetapkan dalam implementation plan setelah query semua jenis BA dipetakan, tetapi perilaku pengguna di atas wajib dipertahankan.

## 8. Perubahan API dan Data

Perubahan minimum yang direncanakan:

- `CreateSlotInput` hanya mewajibkan tanggal; input peralatan dihapus dari form pembuatan tetapi backend dapat mempertahankan kompatibilitas sementara bila diperlukan.
- endpoint update peralatan berpindah otorisasi ke `distribution.pos_dokumen` dan tidak lagi memakai aturan `ErrEquipmentLocked` berbasis media mesin;
- endpoint baru untuk edit data penerima yang sudah terkait;
- endpoint baru untuk mengganti penerima secara atomik;
- endpoint baru `POST /api/v1/distribution/slots/{slot_number}/reopen` dengan `schedule_id`, `stage`, dan `reason`;
- respons slot memuat `needs_recompletion` serta metadata revisi yang diperlukan UI;
- migrasi menambah metadata revisi pada `distribution_slots` dan dukungan stale pada dokumen BA;
- endpoint media memvalidasi stage permission di server.

Semua operasi mutasi memakai row lock/transaksi yang sesuai agar dua petugas tidak dapat membuka revisi, mengganti penerima, atau menyelesaikan slot secara bersamaan dengan hasil parsial.

## 9. UX yang Diharapkan

- POS Mesin menampilkan tanggal di bagian atas dan kartu upload tahap mesin di bawahnya.
- POS Dokumen menampilkan data penerima, data peralatan, lalu kartu upload tahap dokumen.
- POS Penyerahan menampilkan kartu upload tahap penyerahan dan tombol selesai.
- Slot revisi menggunakan badge/peringatan yang mudah terlihat: **Perlu diselesaikan ulang** beserta alasan, POS pembuka, dan waktu revisi.
- Pada slot selesai, kontrol edit diganti tombol **Buka revisi**. Setelah alasan tersimpan, kontrol edit aktif kembali.
- Jika tanggal terkunci, UI menjelaskan bahwa seluruh media harus dihapus terlebih dahulu; bukan sekadar menonaktifkan input tanpa alasan.

## 10. Acceptance Criteria

1. Slot baru dapat dibuat di POS Mesin hanya dengan tanggal.
2. Peralatan diisi dan dapat diedit di POS Dokumen, termasuk setelah media mesin ada.
3. Media setiap tahap hanya dapat dikelola oleh permission POS yang sesuai.
4. Folder tanggal dan nomor slot yang sudah ada digunakan ulang saat upload berikutnya.
5. Tanggal tidak dapat berubah selama masih ada media pada salah satu tahap.
6. Slot selesai hanya bisa diedit setelah dibuka sebagai revisi dengan alasan.
7. Setiap POS dapat membuka revisi untuk domainnya sendiri.
8. Slot revisi kembali ke alur penyelesaian dan tidak dianggap terdistribusi sampai diselesaikan ulang.
9. Penyelesaian ulang menghasilkan snapshot terbaru.
10. Dokumen BA lama tetap tersimpan sebagai riwayat tetapi ditandai tidak mutakhir, dan versi baru dapat dibuat setelah penyelesaian ulang.
11. Semua aturan kritis di atas diuji di service/repository/API dan perilaku utama diuji di frontend.

## 11. Di Luar Cakupan

- memindahkan file Drive otomatis ketika tanggal berubah;
- menghapus folder Drive kosong;
- mengedit NIK penerima secara bebas tanpa melalui pergantian kandidat/alokasi;
- mengubah konfigurasi template dokumentasi tiap program secara otomatis;
- memperbaiki error pengaturan BA/DP3 409/500 yang telah didiagnosis sebelumnya (pekerjaan terpisah).

## 12. Langkah Berikutnya

Setelah spesifikasi ini disetujui pengguna, lanjut ke `superpowers:writing-plans` untuk memetakan migrasi, backend, frontend, invalidasi BA, pengujian, dan deployment secara rinci. Implementasi dimulai setelah implementation plan disetujui.
