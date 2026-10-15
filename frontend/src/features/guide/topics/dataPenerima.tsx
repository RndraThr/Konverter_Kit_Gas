import { Ban, Pencil, Plus, Undo2 } from 'lucide-react';
import { ButtonSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const dataPenerima: GuideTopic = {
  slug: 'data-penerima',
  stepId: 'penerima',
  title: 'Data Penerima',
  intro: 'Pusat data semua penerima lintas program, jadwal, dan kabupaten (sesuai akses Anda). Dipakai untuk mencari penerima, memeriksa kelengkapan dokumentasi, memperbaiki identitas, serta menambah, membatalkan, atau memulihkan penerima.',
  manage: ['recipients.manage'],
  view: ['recipients.view'],
  route: '/',
  sections: [
    {
      id: 'ringkasan',
      title: 'Ringkasan angka',
      summary: 'Kartu di atas menghitung penerima sesuai filter yang aktif.',
      shots: [{ id: 'pn-stats', title: 'Statistik penerima', legend: ['Total hasil sesuai filter', 'Siap / menunggu distribusi', 'Sudah didistribusikan', 'Dokumentasi (evidence) lengkap', 'Dokumentasi perlu dilengkapi'] }],
      steps: [{ n: 5, text: <>Angka <b>Evidence perlu dilengkapi</b> menunjukkan penerima yang foto wajibnya belum lengkap; saring dengan filter Dokumentasi untuk melihat daftarnya.</> }],
    },
    {
      id: 'filter',
      title: 'Cari dan saring',
      summary: 'Gabungkan pencarian dan filter untuk menemukan penerima.',
      shots: [{ id: 'pn-filter', title: 'Filter Data Penerima', legend: ['Cari nama, NIK, atau nomor kartu', 'Program', 'Kabupaten', 'Jadwal', 'Kelengkapan dokumentasi', 'Reset semua filter'] }],
      steps: [
        { n: 1, text: <>Ketik nama, NIK, atau nomor kartu di kolom pencarian.</> },
        { n: 2, text: <>Persempit dengan program, zona, kabupaten, jadwal, kecamatan, status alokasi, dokumentasi, dan status distribusi. Filter aktif tampil sebagai chip yang bisa dihapus satu per satu.</> },
        { n: 6, text: <>Klik <ButtonSample variant="ghost" icon={<Undo2 />}>Reset filter</ButtonSample> untuk kembali ke semua data.</> },
      ],
      fields: [
        { name: 'Status alokasi', meaning: 'Aktif (default: Kandidat, Siap, Perlu ditinjau, Sudah distribusi), Dibatalkan, atau Digantikan. Penerima dibatalkan/digantikan tidak dihitung di statistik.', example: 'Aktif' },
        { name: 'Dokumentasi', meaning: 'Kelengkapan foto wajib: lengkap atau perlu dilengkapi.', example: 'Perlu dilengkapi' },
        { name: 'Status distribusi', meaning: 'Draft (POS masih berjalan), Selesai (sudah diserahkan), atau Dibatalkan.', example: 'Selesai' },
      ],
    },
    {
      id: 'tabel',
      title: 'Membaca tabel',
      summary: 'Setiap baris adalah satu penerima pada satu jadwal.',
      shots: [{ id: 'pn-table', title: 'Tabel penerima (data disamarkan)', legend: ['Nomor bagi', 'Identitas: nama, NIK, nomor kartu', 'Wilayah: zona, kabupaten, desa/kecamatan', 'Program dan jadwal', 'Dokumentasi: jumlah foto wajib terpenuhi', 'Status alokasi dan distribusi', 'Aksi: edit, batalkan, atau pulihkan'] }],
      steps: [
        { text: <>Klik judul kolom bertanda panah untuk mengurutkan.</> },
        { n: 5, text: <>Klik sel Dokumentasi untuk melihat rincian foto mana yang sudah dan belum ada.</> },
        { text: <>Penerima yang menggantikan/digantikan penerima lain diberi keterangan berwarna kuning beserta alasannya.</> },
      ],
    },
    {
      id: 'tambah-edit',
      title: 'Tambah dan ubah penerima',
      summary: 'Penerima biasanya masuk lewat import DCP3. Tambah manual hanya untuk penerima yang belum ada di DCP3.',
      shots: [{ id: 'pn-dialog', title: 'Modal Tambah penerima', legend: ['Jadwal (kabupaten) penerima', 'Nama lengkap', 'NIK 16 digit (boleh kosong)', 'Nomor kartu petani', 'Simpan'] }],
      steps: [
        { text: <>Klik <ButtonSample icon={<Plus />}>Tambah penerima</ButtonSample> di kanan atas.</>, permissions: ['recipients.manage'] },
        { n: 1, text: <>Pilih jadwal, isi identitas, lalu <ButtonSample>Simpan</ButtonSample>.</>, permissions: ['recipients.manage'] },
        { text: <>Untuk memperbaiki data, klik <ButtonSample variant="ghost" icon={<Pencil />}>Edit</ButtonSample> di kolom Aksi.</>, permissions: ['recipients.manage'] },
      ],
      fields: [
        { name: 'Jadwal', meaning: 'Jadwal (kabupaten) tempat penerima didistribusikan.', example: 'KAB. WONOGIRI - KONVERTER KIT 2026', required: true },
        { name: 'Nama lengkap', meaning: 'Sesuai KTP.', example: 'PETANI CONTOH', required: true },
        { name: 'NIK', meaning: '16 digit; boleh dikosongkan bila belum ada, dilengkapi di POS Dokumen.', example: '3312…', required: false },
        { name: 'Nomor kartu, alamat, desa, kecamatan, telepon', meaning: 'Data pelengkap untuk BA dan laporan.', required: false },
      ],
    },
    {
      id: 'batalkan',
      title: 'Batalkan dan pulihkan',
      summary: 'Penerima yang batal menerima (mis. meninggal, pindah) dibatalkan, bukan dihapus.',
      shots: [{ id: 'pn-cancel', title: 'Konfirmasi batalkan penerima', legend: ['Penjelasan: disembunyikan dari daftar aktif, tetap tersimpan untuk audit', 'Konfirmasi pembatalan'] }],
      steps: [
        { text: <>Klik <ButtonSample variant="ghost" icon={<Ban />}>Batalkan</ButtonSample> di kolom Aksi, lalu konfirmasi.</>, permissions: ['recipients.manage'] },
        { text: <>Untuk mengembalikan, ubah filter Status alokasi ke <b>Dibatalkan</b>, lalu klik <ButtonSample variant="ghost" icon={<Undo2 />}>Pulihkan</ButtonSample>.</>, permissions: ['recipients.manage'] },
        { text: <>Penggantian penerima pada nomor bagi yang sudah berjalan dilakukan di POS Dokumen (tombol <b>Ganti penerima</b>), bukan di sini.</> },
      ],
      problems: [{ q: 'Penerima tidak ditemukan?', a: 'Periksa filter Status alokasi (default hanya Aktif) dan pastikan kabupatennya termasuk akses role Anda.' }],
      doneWhen: 'Identitas penerima lengkap dan benar sebelum hari distribusi.',
    },
  ],
};
