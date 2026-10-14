import { Pencil, Plus, Save, Upload } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

// Tutorial rinci menu Persiapan program. Nomor langkah (n) sama dengan nomor
// pada screenshot bagian tersebut.

export const persiapanProgram: GuideTopic = {
  slug: 'persiapan-program',
  stepId: 'persiapan',
  title: 'Persiapan program',
  intro: 'Persiapan program adalah fondasi seluruh sistem: kabupaten, program bantuan, zona, template paket dan dokumentasi, logo dokumen, lalu jadwal distribusi per kabupaten. Semua menu lain (DCP3, distribusi, Berita Acara, laporan) dimulai dengan memilih jadwal yang dibuat di sini.',
  manage: ['programs.manage'],
  view: ['programs.view'],
  route: '/persiapan-program',
  sections: [
    {
      id: 'urutan',
      title: 'Urutan pengerjaan',
      summary: 'Menu ini terdiri dari enam tab. Kerjakan dari kiri ke kanan, karena setiap tab memakai data dari tab sebelumnya.',
      shots: [{ id: 'pp-tabs', title: 'Tab Persiapan program', legend: ['Kabupaten: daftar wilayah dan kode dokumen', 'Program: program bantuan per tahun anggaran', 'Zona: kelompok kabupaten dan No. PO', 'Template: isi paket dan foto wajib', 'Logo Dokumen: logo kepala Berita Acara', 'Jadwal: program × kabupaten × template'] }],
      steps: [
        { n: 1, text: <>Pastikan semua kabupaten sasaran ada dan berstatus aktif.</> },
        { n: 2, text: <>Buat program bantuan (Petani atau Nelayan) untuk tahun anggaran berjalan.</> },
        { n: 3, text: <>Kelompokkan kabupaten ke zona dan isi No. PO bila sudah ada.</> },
        { n: 4, text: <>Siapkan template paket dan template dokumentasi, lalu <b>terbitkan</b>.</> },
        { n: 5, text: <>Unggah logo untuk kepala Berita Acara.</> },
        { n: 6, text: <>Terakhir, buat jadwal untuk setiap kabupaten.</> },
      ],
      doneWhen: 'Setiap kabupaten sasaran punya jadwal berstatus Aktif.',
    },
    {
      id: 'kabupaten',
      title: 'Kabupaten',
      summary: 'Daftar wilayah operasional. Kode dokumen tiga huruf dipakai di nomor Berita Acara dan nama folder.',
      shots: [
        { id: 'pp-kabupaten', title: 'Tab Kabupaten', legend: ['Cari kabupaten', 'Saring status (aktif/nonaktif)', 'Daftar kabupaten beserta provinsi, kode dokumen, dan status', 'Tambah kabupaten baru', 'Ubah data kabupaten'] },
        { id: 'pp-kabupaten-modal', title: 'Modal Tambah kabupaten', legend: ['Provinsi', 'Nama kabupaten/kota', 'Kode dokumen tiga huruf', 'Aktif/nonaktif', 'Simpan'] },
      ],
      steps: [
        { n: 4, text: <>Klik <ButtonSample icon={<Plus />}>Tambah kabupaten</ButtonSample>.</>, permissions: ['programs.manage'] },
        { text: <>Pilih provinsi, lalu kabupaten/kota, dan isi kode dokumen. Biarkan <i>Kabupaten aktif</i> tercentang, lalu <ButtonSample>Simpan</ButtonSample>.</>, permissions: ['programs.manage'] },
        { n: 5, text: <>Untuk mengubah, klik ikon <ButtonSample variant="ghost" icon={<Pencil />}>Edit</ButtonSample> di baris kabupaten.</>, permissions: ['programs.manage'] },
      ],
      fields: [
        { name: 'Provinsi', meaning: 'Provinsi tempat kabupaten berada.', example: 'JAWA TENGAH', required: true },
        { name: 'Kabupaten', meaning: 'Nama kabupaten/kota sesuai daftar wilayah resmi.', example: 'KAB. WONOGIRI', required: true },
        { name: 'Kode dokumen', meaning: 'Tepat tiga huruf, unik untuk setiap kabupaten. Dipakai di nomor BA, misalnya BA-…/WNG/2026.', example: 'WNG', required: true },
        { name: 'Catatan', meaning: 'Keterangan internal, tidak tercetak.', required: false },
        { name: 'Kabupaten aktif', meaning: 'Hanya kabupaten aktif yang bisa dipilih saat membuat jadwal.', required: false },
      ],
      problems: [
        { q: 'Kode dokumen ditolak?', a: 'Kode harus tepat tiga huruf dan belum dipakai kabupaten lain.' },
        { q: 'Kabupaten tidak muncul saat membuat jadwal?', a: 'Pastikan kabupaten berstatus aktif.' },
      ],
    },
    {
      id: 'program',
      title: 'Program',
      summary: 'Program bantuan per tahun anggaran. Program Petani dan Nelayan dipisah karena paket dan dokumennya berbeda.',
      shots: [
        { id: 'pp-program', title: 'Tab Program', legend: ['Daftar program', 'Tambah program', 'Ubah program'] },
        { id: 'pp-program-modal', title: 'Modal Edit program', legend: ['Kode program', 'Nama program', 'Jenis penerima: Petani/Nelayan', 'Tahun anggaran', 'Status program', 'Simpan'] },
      ],
      steps: [
        { n: 2, text: <>Klik <ButtonSample icon={<Plus />}>Tambah program</ButtonSample>, isi kode, nama, jenis penerima, dan tahun anggaran.</>, permissions: ['programs.manage'] },
        { text: <>Ubah status menjadi <b>Aktif</b> saat program mulai berjalan, lalu <ButtonSample>Simpan</ButtonSample>.</>, permissions: ['programs.manage'] },
      ],
      fields: [
        { name: 'Kode program', meaning: 'Kode singkat dan unik.', example: 'KONKIT-2026', required: true },
        { name: 'Nama program', meaning: 'Nama resmi program, tampil di dokumen.', example: 'PEMBAGIAN KONVERTER KIT 2026', required: true },
        { name: 'Jenis penerima', meaning: 'Petani atau Nelayan. Menentukan template yang bisa dipakai dan jenis BA. Tidak bisa dicampur dalam satu jadwal.', example: 'Petani', required: true },
        { name: 'Tahun anggaran', meaning: 'Tahun anggaran program.', example: '2026', required: true },
        { name: 'Status', meaning: 'Draf (disiapkan), Aktif (berjalan), Selesai, atau Arsip.', example: 'Aktif', required: true },
      ],
    },
    {
      id: 'zona',
      title: 'Zona',
      summary: 'Mengelompokkan kabupaten per zona tender. Zona dipakai untuk struktur folder dokumen (Drive) dan No. PO di BA Pemeriksaan.',
      shots: [{ id: 'pp-zona', title: 'Tab Zona', legend: ['Pilih program', 'Tambah zona', 'Papan zona: seret kabupaten antar zona', 'No. PO per zona untuk BA Pemeriksaan'] }],
      steps: [
        { n: 1, text: <>Pilih program, lalu <ButtonSample icon={<Plus />}>Tambah zona</ButtonSample> sebanyak zona tender.</>, permissions: ['programs.manage'] },
        { n: 3, text: <>Seret kabupaten dari kolom <i>Belum ada zona</i> ke zona yang benar. Untuk memindahkan banyak sekaligus, centang beberapa kabupaten lalu pilih <FieldSample>Pindahkan ke…</FieldSample>. Ikon pensil mengubah nama zona; seret judul zona untuk mengubah urutan.</>, permissions: ['programs.manage'] },
        { n: 4, text: <>Isi <b>No. PO</b> per barang untuk setiap zona, lalu <ButtonSample icon={<Save />}>Simpan No. PO</ButtonSample>.</>, permissions: ['programs.manage'] },
      ],
      problems: [
        { q: 'Unggahan dokumen kabupaten tertahan?', a: 'Kabupaten belum masuk zona. Pindahkan ke zona agar unggahan diteruskan ke folder yang benar.' },
        { q: 'Tabel No. PO kosong?', a: 'No. PO mengikuti merk dari template paket jadwal di zona itu. Buat jadwal dengan template paket terlebih dahulu.' },
      ],
    },
    {
      id: 'template-paket',
      title: 'Template paket',
      summary: 'Isi baku paket bantuan: pilihan mesin, selang, konkit/reducer, dan komponen beserta % TKDN. Nilai ini disalin ke setiap penerima saat import DCP3.',
      shots: [
        { id: 'pp-template', title: 'Tab Template', legend: ['Daftar template paket (versi & status)', 'Tambah template paket', 'Daftar template dokumentasi', 'Tambah template dokumentasi'] },
        { id: 'pp-paket-identitas', title: 'Template paket: identitas', legend: ['Kode template', 'Nama template', 'Jenis program', 'Status versi'] },
        { id: 'pp-paket-mesin', title: 'Template paket: opsi mesin', legend: ['Merk mesin', 'Tipe mesin', 'Daya', 'Jenis BBM', '% TKDN mesin', 'Tambah pilihan mesin lain'] },
        { id: 'pp-paket-selang', title: 'Template paket: opsi selang', legend: ['Merk selang hisap', 'Spesifikasi selang hisap', 'Merk selang buang', 'Spesifikasi selang buang', '% TKDN selang'] },
        { id: 'pp-paket-konkit', title: 'Template paket: opsi konkit/reducer', legend: ['Merk konkit/reducer', '% TKDN konkit', 'Tambah pilihan konkit lain'] },
        { id: 'pp-paket-komponen', title: 'Template paket: komponen', legend: ['Nama komponen', 'Jumlah (dan satuan di sebelahnya)', 'Merk komponen', '% TKDN komponen', 'Sembunyikan dari checklist BA serah terima'] },
      ],
      steps: [
        { n: 2, text: <>Di tab <b>Template</b>, klik <ButtonSample icon={<Plus />}>Tambah paket</ButtonSample>, atau ikon edit untuk mengubah template yang ada.</>, permissions: ['programs.manage'] },
        { text: <><b>Opsi mesin, selang, konkit:</b> setiap opsi adalah satu pilihan yang bisa dipilih tim lapangan untuk penerima. Klik <ButtonSample variant="outline" icon={<Plus />}>Tambah opsi mesin</ButtonSample> bila ada lebih dari satu merk/tipe.</>, permissions: ['programs.manage'] },
        { text: <><b>Komponen paket:</b> aksesoris dan kelengkapan (tabung LPG, regulator, buku manual, oli, braket, …) beserta jumlah, satuan, merk, dan % TKDN. Centang <i>Sembunyikan dari checklist BA serah terima</i> untuk komponen yang hanya dihitung di TKDN.</>, permissions: ['programs.manage'] },
        { text: <>Simpan sebagai <b>Draf</b> untuk diperiksa, lalu ubah status ke <b>Terbit</b>. Hanya template terbit yang bisa dipilih di jadwal.</>, permissions: ['programs.manage'] },
      ],
      fields: [
        { name: 'Kode / Nama template', meaning: 'Identitas template; kode singkat dan unik.', example: 'KONKIT-2026 / PAKET KONVERTER KIT 2026', required: true },
        { name: 'Jenis program', meaning: 'Petani atau Nelayan; template hanya bisa dipakai jadwal dengan jenis yang sama.', example: 'Petani', required: true },
        { name: 'Status', meaning: 'Draf (masih disusun), Terbit (dipakai jadwal), Diarsipkan (tidak dipakai lagi). Mengubah template terbit membuat versi draf baru, jadi jadwal lama tetap memakai versi lama.', example: 'Terbit', required: true },
        { name: 'Opsi mesin', meaning: 'Merk, tipe, daya, jenis BBM, dan % TKDN mesin.', example: 'Shark · SPWP 80-30/3" · 5,5 HP · Bensin · 61,42%', required: true },
        { name: 'Opsi selang', meaning: 'Pasangan selang hisap dan selang buang yang diserahkan bersama, beserta spesifikasi dan % TKDN.', example: 'Triliunhose 6 m / Yamakoyo 10 m', required: true },
        { name: 'Opsi konkit/reducer', meaning: 'Merk konkit/reducer dan % TKDN.', example: 'Ergas · 93,27%', required: true },
        { name: 'Komponen paket', meaning: 'Kelengkapan lain: nama, jumlah, satuan, merk, % TKDN. Muncul di checklist BA serah terima kecuali disembunyikan.', example: 'Tabung LPG 3 kg · 1 Tabung · Pertamina · 58,47%', required: false },
      ],
      problems: [
        { q: 'Template tidak muncul saat membuat jadwal?', a: 'Status template masih Draf, atau jenis programnya berbeda dengan program jadwal.' },
        { q: 'Mengubah template tapi jadwal tidak berubah?', a: 'Perubahan pada template terbit membuat versi draf baru. Terbitkan versi baru lalu pilih versi itu di jadwal.' },
      ],
    },
    {
      id: 'template-dokumentasi',
      title: 'Template dokumentasi',
      summary: 'Daftar foto/video bukti yang wajib diambil di setiap POS saat distribusi.',
      shots: [{ id: 'pp-dokumentasi-slot', title: 'Template dokumentasi: slot foto', legend: ['Judul slot (tampil di aplikasi lapangan)', 'Jenis media: foto, video, atau keduanya', 'Sumber: kamera, galeri, atau keduanya', 'POS tempat foto diambil', 'Jumlah minimal file', 'Wajib diisi sebelum lanjut', 'Tambah slot baru'] }],
      steps: [
        { text: <>Klik <ButtonSample icon={<Plus />}>Tambah dokumentasi</ButtonSample> di tab Template, isi identitas seperti template paket.</>, permissions: ['programs.manage'] },
        { n: 7, text: <>Tambah satu slot untuk setiap foto bukti dengan <ButtonSample variant="outline" icon={<Plus />}>Tambah slot</ButtonSample>.</>, permissions: ['programs.manage'] },
        { n: 4, text: <>Pilih POS yang tepat: foto mesin & nomor seri di <b>POS Mesin</b>, foto KTP/KK/kartu tani di <b>POS Dokumen</b>, foto serah terima di <b>POS Penyerahan</b>.</>, permissions: ['programs.manage'] },
        { text: <>Terbitkan template agar bisa dipilih di jadwal.</>, permissions: ['programs.manage'] },
      ],
      fields: [
        { name: 'Kode slot', meaning: 'Kode unik per slot (huruf kecil, garis bawah).', example: 'machine_serial', required: true },
        { name: 'Judul', meaning: 'Nama foto yang dilihat tim lapangan.', example: 'Foto Nomor Seri Mesin Baru dan Nomor Urut', required: true },
        { name: 'Jenis media', meaning: 'Foto saja, Video saja, atau Foto & video.', example: 'Foto saja', required: true },
        { name: 'Sumber', meaning: 'Kamera (langsung memotret), Galeri, atau keduanya.', example: 'Kamera & galeri', required: true },
        { name: 'Pos', meaning: 'POS tempat slot ini diisi.', example: 'POS Mesin', required: true },
        { name: 'Minimal', meaning: 'Jumlah file minimal.', example: '1', required: true },
        { name: 'Wajib', meaning: 'Distribusi tidak bisa diselesaikan sebelum slot wajib lengkap.', required: false },
      ],
    },
    {
      id: 'logo',
      title: 'Logo Dokumen',
      summary: 'Logo yang tercetak di kepala semua Berita Acara dalam satu program.',
      shots: [{ id: 'pp-logo', title: 'Tab Logo Dokumen', legend: ['Pilih program', 'Unggah logo baru', 'Daftar logo yang tercetak'] }],
      steps: [
        { n: 1, text: <>Pilih program.</> },
        { n: 2, text: <>Klik <ButtonSample variant="outline" icon={<Upload />}>Unggah logo</ButtonSample> dan pilih file gambar (PNG transparan paling rapi).</>, permissions: ['programs.manage'] },
        { n: 3, text: <>Logo berlaku untuk semua jadwal, zona, dan kabupaten dalam program itu.</> },
      ],
      problems: [{ q: 'Preview BA menolak karena logo?', a: 'Minimal satu logo aktif harus ada di program jadwal.' }],
    },
    {
      id: 'jadwal',
      title: 'Jadwal',
      summary: 'Jadwal menggabungkan program, kabupaten, template paket, dan template dokumentasi. Satu jadwal untuk setiap kabupaten.',
      shots: [
        { id: 'pp-jadwal', title: 'Tab Jadwal', legend: ['Cari jadwal', 'Daftar jadwal (program, kabupaten, periode, template, status)', 'Tambah jadwal', 'Ubah jadwal'] },
        { id: 'pp-jadwal-modal', title: 'Modal Edit jadwal', legend: ['Nama jadwal', 'Program', 'Kabupaten', 'Template paket (hanya yang terbit)', 'Template dokumentasi (hanya yang terbit)', 'Status jadwal', 'Tanggal mulai', 'Tanggal selesai', 'Kuota nomor bagi', 'Nama konsultan pengawas', 'Simpan'] },
      ],
      steps: [
        { n: 3, text: <>Klik <ButtonSample icon={<Plus />}>Tambah jadwal</ButtonSample>.</>, permissions: ['programs.manage'] },
        { text: <>Isi nama jadwal, pilih program dan kabupaten, lalu template paket dan dokumentasi (daftar otomatis disaring sesuai jenis program).</>, permissions: ['programs.manage'] },
        { text: <>Isi tanggal mulai–selesai dan kuota nomor bagi, ubah status ke <b>Aktif</b>, lalu <ButtonSample>Simpan</ButtonSample>.</>, permissions: ['programs.manage'] },
      ],
      fields: [
        { name: 'Nama jadwal', meaning: 'Nama yang tampil di semua pilihan jadwal.', example: 'KAB. WONOGIRI - KONVERTER KIT 2026', required: true },
        { name: 'Program / Kabupaten', meaning: 'Program bantuan dan kabupaten pelaksana.', example: 'PEMBAGIAN KONVERTER KIT 2026 / WNG - KAB. WONOGIRI', required: true },
        { name: 'Template paket / dokumentasi', meaning: 'Versi template terbit yang dipakai jadwal ini.', example: 'PAKET KONVERTER KIT 2026 v1', required: true },
        { name: 'Status', meaning: 'Draf, Aktif (bisa dipakai DCP3 dan distribusi), Selesai, Dibatalkan.', example: 'Aktif', required: true },
        { name: 'Mulai / Selesai', meaning: 'Rentang tanggal distribusi.', example: '24/09/2026 – 08/11/2026', required: true },
        { name: 'Kuota slot', meaning: 'Jumlah nomor bagi maksimal (jumlah paket). Kosongkan bila tanpa batas.', example: '320', required: false },
        { name: 'Konsultan pengawas', meaning: 'Nama konsultan pengawas jadwal ini.', required: false },
      ],
      problems: [
        { q: 'Jadwal tidak muncul di DCP3 atau distribusi?', a: 'Status jadwal belum Aktif, atau kabupaten jadwal di luar akses role Anda.' },
        { q: 'Template yang dicari tidak ada di pilihan?', a: 'Template belum Terbit atau jenis programnya berbeda.' },
      ],
      doneWhen: 'Jadwal aktif tampil di DCP3, Pendistribusian, dan Berita Acara.',
    },
  ],
};
