import { Download, FileUp, ListChecks, PenLine, Printer, RefreshCw, Save, ScanLine, Signature, Upload } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const beritaAcara: GuideTopic = {
  slug: 'berita-acara',
  stepId: 'berita-acara',
  title: 'Berita Acara',
  intro: 'Semua Berita Acara (BA) dibuat otomatis dari data distribusi dan persiapan program. Alurnya sama untuk hampir semua jenis: pilih jadwal dan jenis BA, lengkapi isian, atur penandatangan, periksa preview, lalu finalisasi. PDF final tersimpan sebagai versi dan disinkronkan ke folder dokumen kabupaten. Preview pada contoh di halaman ini diburamkan karena memuat data penerima.',
  manage: ['bast.manage'],
  view: ['bast.view'],
  route: '/berita-acara',
  sections: [
    {
      id: 'jenis',
      title: 'Pilih jadwal dan jenis BA',
      summary: 'Setiap tab adalah satu jenis BA untuk jadwal (kabupaten) yang dipilih.',
      shots: [{ id: 'ba-pilih', title: 'Berita Acara', legend: ['Pilih jadwal kabupaten', 'Tab jenis BA'] }],
      steps: [
        { n: 1, text: <>Pilih <FieldSample>Jadwal program</FieldSample>, lalu tab jenis BA.</> },
        { text: <>Di HP, jenis BA dipilih dari daftar pilihan di atas, bukan tab.</> },
      ],
      fields: [
        { name: 'DP3', meaning: 'Daftar penerima paket perdana satu jadwal.', example: 'Setelah semua nomor bagi terhubung' },
        { name: 'BA Perorangan', meaning: 'BA serah terima per penerima, dibuat per tanggal distribusi (satu PDF berisi semua penerima hari itu).', example: 'Setiap hari distribusi' },
        { name: 'Rekap Harian', meaning: 'Rekap penerima per tanggal distribusi.', example: 'Setiap hari distribusi' },
        { name: 'Closing Titik Serah / Kabupaten', meaning: 'Rekap akhir per titik serah dan per kabupaten.', example: 'Setelah distribusi selesai' },
        { name: 'Rakorda, Sosialisasi, Training 10%/100%', meaning: 'Daftar hadir kegiatan: dicetak kosong, diisi di lokasi, lalu hasilnya diunggah.', example: 'Saat kegiatan' },
        { name: 'BA Pemeriksaan', meaning: 'BA pemeriksaan barang, satu PDF per jenis barang (form).', example: 'Saat barang diperiksa' },
        { name: 'Servis Berkala', meaning: 'BA servis berkala ke-1 dan ke-2.', example: 'Sesuai jadwal servis' },
        { name: 'TKDN', meaning: 'Rekap % TKDN barang dan TKDN gabungan.', example: 'Akhir program' },
        { name: 'Gas LPG', meaning: 'BA serah terima tabung gas elpiji per penerimaan, lalu digabung menjadi satu BA.', example: 'Setiap LPG datang' },
      ],
    },
    {
      id: 'isian-preview',
      title: 'Isian dokumen dan preview',
      summary: 'Isian di atas preview tersimpan otomatis setelah kolom ditinggalkan (atau Enter), lalu preview diperbarui.',
      shots: [{ id: 'ba-dp3', title: 'BA DP3', legend: ['Tanggal dokumen', 'Lokasi / titik serah', 'Konsultan distribusi (kosong = nama organisasi)', 'Pengaturan penandatangan', 'Finalisasi (aktif bila data lengkap)', 'Langkah tanda tangan basah (bila mode aktif)'] }],
      steps: [
        { n: 1, text: <>Isi tanggal dokumen dan informasi yang diminta. Ringkasan di kiri menampilkan pesan kuning bila ada data yang belum lengkap.</>, permissions: ['bast.manage'] },
        { text: <>Periksa preview PDF di bawahnya. Preview tetap tampil walau data belum lengkap; isian yang belum ada dibiarkan kosong.</> },
      ],
      problems: [
        { q: 'Tombol Finalisasi abu-abu?', a: 'Baca pesan kuning di panel ringkasan, misalnya "Lokasi/titik serah belum diisi", atau mode tanda tangan basah sedang menunggu pindaian.' },
        { q: 'Preview gagal dimuat?', a: 'Periksa logo dokumen program dan zona kabupaten di Persiapan program.' },
      ],
    },
    {
      id: 'penandatangan',
      title: 'Penandatangan',
      summary: 'Tabel tanda tangan ditampilkan persis seperti cetak. Tanda tangan digital dan stempel bisa diletakkan, diubah ukurannya, dan diputar langsung di tabel.',
      shots: [
        { id: 'ba-penandatangan-switch', title: 'Pengaturan cetak', legend: ['Atur khusus: pengaturan hanya untuk BA ini (mati = global jadwal)', 'Mode tanda tangan basah'] },
        { id: 'penandatangan', title: 'Editor tabel tanda tangan', legend: ['Pilih pihak yang akan diatur', 'Tabel tanda tangan: klik, geser, ubah ukuran, putar', 'Nama, NIP, tanda tangan, stempel, ukuran & putar', 'Simpan semua perubahan'] },
      ],
      steps: [
        { text: <>Klik <ButtonSample variant="ghost" icon={<Signature />}>Penandatangan</ButtonSample> di panel BA.</>, permissions: ['bast.manage'] },
        { text: <>Pilih pihak (tab atau klik kolomnya), isi nama/NIP, klik <ButtonSample variant="outline" icon={<PenLine />}>Gambar tanda tangan</ButtonSample> untuk mencoret langsung di kolom, atau unggah stempel.</>, permissions: ['bast.manage'] },
        { text: <>Geser, tarik sudut, atau putar tanda tangan/stempel; tombol panah menggeser sedikit demi sedikit. Klik <ButtonSample icon={<Save />}>Simpan</ButtonSample>.</>, permissions: ['bast.manage'] },
        { n: 1, text: <>Pengaturan cetak dan posisi berlaku global untuk jadwal; nyalakan <b>Atur khusus</b> bila BA ini perlu pengaturan berbeda. Nama, tanda tangan, dan stempel selalu dipakai bersama di semua BA kabupaten.</>, permissions: ['bast.manage'] },
      ],
      problems: [{ q: 'Kolom tanda tangan kosong di PDF?', a: 'Centang "Cetak tanda tangan digital" atau "Tambah stempel" untuk pihak tersebut. Kolom yang kosong memang disediakan untuk tanda tangan basah.' }],
    },
    {
      id: 'tanda-tangan-basah',
      title: 'Tanda tangan basah',
      summary: 'Untuk BA yang harus ditandatangani dan distempel asli: cetak halaman tanda tangan, tandatangani, lalu unggah hasil pindainya. Pindaian menggantikan halaman tanda tangan di PDF final.',
      shots: [{ id: 'ba-basah', title: 'Langkah tanda tangan basah', legend: ['Kode dokumen untuk mencocokkan pindaian', 'Status: belum diunggah / sudah ditandatangani / perlu ditandatangani ulang', 'Cetak halaman tanda tangan', 'Unggah hasil pindai (PDF, JPG, PNG, atau foto)'] }],
      steps: [
        { n: 2, text: <>Nyalakan switch <b>Tanda tangan basah</b> di panel Penandatangan (tersedia untuk DP3, Rekap Harian, Closing, Servis Berkala, TKDN).</>, permissions: ['bast.manage'] },
        { n: 3, text: <>Klik <ButtonSample variant="outline" icon={<Printer />}>Cetak halaman tanda tangan</ButtonSample>, cetak, minta tanda tangan dan stempel.</> },
        { n: 4, text: <>Pindai atau foto halamannya, lalu <ButtonSample icon={<FileUp />}>Unggah hasil pindai</ButtonSample>. Status berubah menjadi <b>Sudah ditandatangani</b> dan Finalisasi terbuka.</>, permissions: ['bast.manage'] },
      ],
      problems: [
        { q: 'Status "Perlu ditandatangani ulang"?', a: 'Isi BA berubah setelah halaman dipindai. Cetak ulang halaman tanda tangan, tandatangani, lalu unggah lagi.' },
        { q: 'Unggahan pindaian gagal karena terlalu besar?', a: 'Gunakan file di bawah 8 MB; foto dari HP biasanya cukup kecil setelah diproses otomatis.' },
      ],
    },
    {
      id: 'finalisasi',
      title: 'Finalisasi dan riwayat versi',
      summary: 'Finalisasi mengunci BA menjadi PDF resmi, menyimpannya sebagai versi, dan menyinkronkannya ke folder dokumen.',
      shots: [{ id: 'ba-perorangan', title: 'BA Perorangan setelah finalisasi', legend: ['Daftar tanggal: Siap / Tersinkron / Perlu konfigurasi', 'Unduh PDF versi aktif', 'Finalisasi & sinkronkan'] }],
      steps: [
        { n: 3, text: <>Setelah preview benar, klik <ButtonSample icon={<RefreshCw />}>Finalisasi & sinkronkan</ButtonSample>.</>, permissions: ['bast.manage'] },
        { n: 2, text: <>Versi aktif tampil dengan nama file, jumlah halaman, dan <ButtonSample variant="outline" icon={<Download />}>Unduh PDF</ButtonSample>.</> },
        { text: <>Bila data berubah setelah final, versi lama ditandai <b>Perlu dibuat ulang</b>; finalisasi lagi untuk membuat versi baru. Versi lama tetap bisa diunduh di Riwayat versi.</> },
      ],
      doneWhen: 'Versi aktif BA tersedia untuk diunduh dan status tanggalnya Tersinkron.',
    },
    {
      id: 'per-tanggal',
      title: 'BA Perorangan dan Rekap Harian',
      summary: 'Kedua BA ini dibuat per tanggal distribusi. Daftar tanggal di kiri muncul otomatis dari distribusi yang sudah selesai.',
      steps: [
        { text: <>Pilih tanggal di daftar kiri. <b>Siap</b>: bisa difinalisasi; <b>Tersinkron</b>: sudah final; <b>Perlu konfigurasi</b>: ada data yang kurang (baca pesan kuning).</> },
        { text: <>BA Perorangan berisi semua penerima tanggal itu dalam satu PDF; tanda tangan penerima diambil dari POS Dokumen (bila kosong, kolom dibiarkan untuk tanda tangan basah).</> },
      ],
    },
    {
      id: 'daftar-hadir',
      title: 'Daftar hadir: Rakorda, Sosialisasi, Training',
      summary: 'Daftar hadir dicetak kosong, diisi peserta di lokasi, lalu hasil terisinya diunggah sebagai arsip.',
      shots: [{ id: 'ba-sosialisasi', title: 'Sosialisasi', legend: ['Lokasi kegiatan (dicetak di header)', 'Jumlah baris peserta', 'Tanggal kegiatan', 'Cetak daftar hadir kosong', 'Unggah hasil terisi (PDF/JPG/PNG)', 'Pindai dari foto (rapikan foto halaman)'] }],
      steps: [
        { n: 1, text: <>Isi lokasi dan jumlah baris, lalu <ButtonSample variant="outline" icon={<Save />}>Simpan pengaturan</ButtonSample>.</>, permissions: ['bast.manage'] },
        { n: 4, text: <>Pilih tanggal kegiatan, klik <ButtonSample variant="outline" icon={<Printer />}>Cetak</ButtonSample> atau unduh PDF kosong.</> },
        { n: 5, text: <>Setelah diisi dan ditandatangani, klik <ButtonSample variant="outline" icon={<Upload />}>Unggah berkas</ButtonSample>, atau <ButtonSample variant="outline" icon={<ScanLine />}>Pindai dari foto</ButtonSample> untuk memotret dan merapikan halaman langsung dari HP.</>, permissions: ['bast.manage'] },
      ],
    },
    {
      id: 'pemeriksaan',
      title: 'BA Pemeriksaan',
      summary: 'Satu PDF per form barang (Mesin Pompa, Konverter Kit, Regulator, Selang, Oli, Tabung Gas). No. PO diambil dari Zona di Persiapan program.',
      shots: [{ id: 'ba-pemeriksaan', title: 'BA Pemeriksaan', legend: ['Daftar form barang (klik untuk melihat)', 'Atur daftar form', 'Tanggal pemeriksaan', 'Pemasok (kosong = nama organisasi)', 'Finalisasi semua form sekaligus'] }],
      steps: [
        { n: 2, text: <>Bila perlu, ubah susunan form dengan <ButtonSample variant="outline" icon={<ListChecks />}>Atur daftar form</ButtonSample> (berlaku untuk seluruh program).</>, permissions: ['bast.manage'] },
        { n: 3, text: <>Isi tanggal pemeriksaan dan pemasok, periksa setiap form, lalu <ButtonSample icon={<RefreshCw />}>Finalisasi semua form</ButtonSample>.</>, permissions: ['bast.manage'] },
      ],
    },
    {
      id: 'servis',
      title: 'Servis Berkala',
      summary: 'BA servis berkala memakai rentang tanggal servis ke-1 dan ke-2.',
      shots: [{ id: 'ba-servis', title: 'Servis Berkala', legend: ['Rentang tanggal servis ke-1', 'Rentang tanggal servis ke-2', 'Simpan jadwal servis', 'Tanggal dokumen', 'Perusahaan pelaksana'] }],
      steps: [
        { n: 1, text: <>Isi tanggal mulai–selesai servis ke-1 dan ke-2, lalu <ButtonSample variant="outline">Simpan jadwal</ButtonSample>.</>, permissions: ['bast.manage'] },
        { n: 4, text: <>Isi tanggal dokumen dan perusahaan pelaksana, lalu finalisasi.</>, permissions: ['bast.manage'] },
      ],
      problems: [{ q: 'Finalisasi ditolak?', a: 'Tanggal servis ke-1 dan ke-2 harus diisi dan disimpan terlebih dahulu.' }],
    },
    {
      id: 'tkdn',
      title: 'TKDN',
      summary: 'Rekap % TKDN per barang (dari template paket jadwal) dan TKDN gabungan program.',
      shots: [{ id: 'ba-tkdn', title: 'TKDN', legend: ['Tanggal dokumen', 'TKDN gabungan (%)', 'Simpan TKDN gabungan', 'Nama penyedia', 'Susunan baris TKDN'] }],
      steps: [
        { n: 2, text: <>Isi TKDN gabungan lalu <ButtonSample variant="outline" icon={<Save />}>Simpan</ButtonSample>. Nilai ini berlaku untuk semua kabupaten program.</>, permissions: ['bast.manage'] },
        { n: 5, text: <>Atur urutan dan pengelompokan baris di <b>Susunan TKDN</b>. Merk dan % TKDN per barang diubah di Template Paket.</>, permissions: ['bast.manage'] },
      ],
      problems: [{ q: 'Finalisasi ditolak?', a: 'TKDN gabungan harus diisi dan disimpan.' }],
    },
    {
      id: 'gas-lpg',
      title: 'Gas LPG',
      summary: 'LPG satu kabupaten biasanya datang beberapa kali. Tiap penerimaan dibuat satu BA per tanggal, lalu semua penerimaan digabung menjadi satu BA Gas LPG.',
      steps: [
        { n: 1, text: <>Isi tanggal penerimaan, lokasi gudang, alamat, PIC, jumlah tabung, dan keterangan. Nama penerima dan petugas PT Pertamina Patra Niaga boleh dikosongkan untuk memakai nama di Penandatangan. Periksa preview di sebelahnya.</>, permissions: ['bast.manage'] },
        { text: <>Tanda tangan digital, stempel, dan mode tanda tangan basah diatur lewat <ButtonSample variant="outline" icon={<Signature />}>Penandatangan</ButtonSample>, sama seperti BA lain. Pada mode basah, cetak halaman tanda tangan, tandatangani, lalu unggah pindaiannya sebelum finalisasi.</> },
        { n: 2, text: <>Klik <ButtonSample icon={<RefreshCw />}>Finalisasi penerimaan</ButtonSample>. Untuk kedatangan berikutnya, ganti tanggal penerimaan lalu ulangi.</>, permissions: ['bast.manage'] },
        { n: 3, text: <>Setelah semua penerimaan final, klik <ButtonSample icon={<RefreshCw />}>Gabungkan & finalisasi</ButtonSample> di bagian daftar penerimaan. Hasilnya satu PDF berisi semua penerimaan urut tanggal.</>, permissions: ['bast.manage'] },
        { text: <>Bila ada penerimaan baru atau yang diubah, status gabungan menjadi <b>Perlu digabung ulang</b>; gabungkan lagi agar versinya diperbarui.</> },
      ],
      problems: [
        { q: 'Gabungan tidak bisa difinalisasi saat mode tanda tangan basah aktif?', a: 'Setiap penerimaan harus memakai halaman bertanda tangan basah. Buka penerimaan tanpa badge TTD basah, unggah pindaiannya, finalisasi ulang, lalu gabungkan.' },
        { q: 'Salah memasukkan tanggal penerimaan?', a: 'Klik Batalkan pada penerimaan itu di daftar, lalu buat ulang pada tanggal yang benar dan gabungkan ulang.' },
        { q: 'Finalisasi ditolak?', a: 'Semua isian wajib diisi dan jumlah tabung minimal 1; keterangan boleh kosong.' },
      ],
    },
  ],
};
