import { Download, FileSpreadsheet, FileText } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const laporan: GuideTopic = {
  slug: 'laporan',
  stepId: 'laporan',
  title: 'Laporan & Map Distribusi',
  intro: 'Laporan menampilkan progres alokasi, distribusi, dan dokumentasi per jadwal dan bisa diekspor ke Excel/PDF. Map Distribusi menampilkan sebaran penerima, progres, dan kelengkapan bukti per kabupaten di peta Indonesia.',
  manage: ['distribution.manage'],
  view: ['distribution.view'],
  route: '/laporan',
  sections: [
    {
      id: 'laporan',
      title: 'Laporan per jadwal',
      summary: 'Pilih jadwal, lalu persempit dengan status.',
      shots: [{ id: 'lp-filter', title: 'Filter dan ringkasan laporan', legend: ['Jadwal (kabupaten)', 'Status alokasi', 'Status distribusi', 'Kelengkapan dokumentasi', 'Ringkasan angka sesuai filter'] }],
      steps: [
        { n: 1, text: <>Buka menu <b>Laporan</b> dan pilih <FieldSample>Jadwal</FieldSample>.</> },
        { n: 2, text: <>Saring berdasarkan status alokasi, status distribusi, atau dokumentasi. Ringkasan dan tabel mengikuti filter.</> },
      ],
      fields: [
        { name: 'Alokasi candidate / ready / needs review / distributed', meaning: 'Status penerima: kandidat, siap, perlu ditinjau, sudah distribusi.' },
        { name: 'Alokasi replaced / cancelled', meaning: 'Penerima digantikan atau dibatalkan.' },
        { name: 'Distribusi draft / completed / cancelled', meaning: 'Nomor bagi masih berjalan, selesai diserahkan, atau dibatalkan.' },
        { name: 'Dokumentasi belum lengkap', meaning: 'Penerima yang foto wajibnya belum lengkap.' },
      ],
    },
    {
      id: 'ekspor',
      title: 'Ekspor laporan',
      summary: 'Unduh laporan sesuai filter yang sedang aktif.',
      shots: [{ id: 'lp-export', title: 'Ekspor dan tabel laporan (disamarkan)', legend: ['Unduh Excel', 'Unduh PDF', 'Tabel per penerima'] }],
      steps: [
        { n: 1, text: <>Klik <ButtonSample variant="outline" icon={<FileSpreadsheet />} trailingIcon={<Download />}>Export Excel</ButtonSample> untuk diolah lebih lanjut, atau <ButtonSample variant="outline" icon={<FileText />} trailingIcon={<Download />}>Export PDF</ButtonSample> untuk dikirim.</> },
      ],
    },
    {
      id: 'peta',
      title: 'Map Distribusi',
      summary: 'Peta kabupaten berwarna sesuai angka yang dipilih. Filternya sama dengan Data Penerima.',
      shots: [
        { id: 'mp-peta', title: 'Map Distribusi', legend: ['Pilih tampilan: sebaran penerima, progres distribusi, atau kelengkapan bukti', 'Peta: klik kabupaten untuk melihat rincian; tombol + / − / reset untuk zoom', 'Filter kabupaten, program, jadwal, bukti, status', 'Legenda warna dan total angka'] },
        { id: 'mp-peringkat', title: 'Peringkat wilayah', legend: ['Baris kabupaten: klik untuk memilih di peta', 'Jumlah penerima', 'Jumlah bukti lengkap'] },
      ],
      steps: [
        { n: 1, text: <>Pilih tampilan peta di atas peta.</> },
        { n: 2, text: <>Klik kabupaten di peta atau di tabel peringkat untuk melihat rinciannya.</> },
        { n: 3, text: <>Gunakan filter untuk satu program atau jadwal tertentu.</> },
      ],
      doneWhen: 'Laporan berkala terkirim dan progres tiap kabupaten terpantau.',
    },
  ],
};
