import { Download, FileSpreadsheet, FileText } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const laporan: GuideTopic = {
  slug: 'laporan',
  stepId: 'laporan',
  title: 'Laporan & Map Distribusi',
  intro: 'Laporan menampilkan progres alokasi, distribusi, dan dokumentasi per jadwal dan bisa diekspor ke Excel/PDF. Map Distribusi menampilkan setiap nomor bagi sebagai titik di peta, berwarna sesuai milestone penyaluran.',
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
      summary: 'Peta interaktif tempat setiap nomor bagi tampil sebagai titik berwarna sesuai milestone. Filternya sama dengan Data Penerima.',
      steps: [
        { n: 1, text: <>Buka menu <b>Map Distribusi</b>. Peta langsung memperbesar ke titik di cakupan kabupaten Anda. Gunakan kotak pencarian untuk kabupaten atau nomor bagi.</> },
        { n: 2, text: <>Pilih jenis peta: <b>Biasa</b> (jalan dan tempat), <b>Satelit</b> (citra udara), atau <b>Hybrid</b> (citra udara dengan nama jalan dan wilayah). Titik dan batas zona tidak hilang saat jenis peta diganti.</> },
        { n: 3, text: <>Klik titik untuk membuka rincian: tanggal tiap milestone, penerima, mesin, foto bukti, dan koordinat. <b>Buka nomor bagi</b> membawa Anda ke halaman Pendistribusian; <b>Arahkan ke lokasi</b> membuka rute di Google Maps. Di HP, rincian muncul sebagai panel di bagian bawah layar.</> },
        { n: 4, text: <>Klik angka berwarna (titik serah) untuk melihat daftar penerima yang diserahkan di tempat yang sama, lalu pilih salah satu nomor bagi.</> },
        { n: 5, text: <>Pilih milestone di bawah peta untuk menyaring titik, atau putar <b>timeline</b> untuk melihat perkembangan hari demi hari.</> },
      ],
      fields: [
        { name: 'Warna titik (milestone)', meaning: <>Dari terang ke gelap: M1 <b>Terjadwal</b>, M2 <b>Penerima tertaut</b>, M3 <b>Diserahkan</b>, M4 <b>Bukti lengkap</b>, M5 <b>BA ditandatangani</b>. Semakin gelap, semakin lengkap.</> },
        { name: 'Perlu ditinjau', meaning: 'Titik dengan lingkar oranye: koordinat foto berada di luar batas kabupaten jadwalnya (toleransi 500 m), atau lebih dari 1 km dari titik serahnya. Ini hanya penanda untuk dicek, tidak menghalangi distribusi.' },
        { name: 'Titik serah', meaning: 'Beberapa penerima yang diserahkan di satu tempat (misalnya balai desa) digabung menjadi satu penanda berangka. Warnanya mengikuti milestone terendah di tempat itu, agar pekerjaan yang belum selesai tetap terlihat.' },
        { name: 'N belum ada lokasi', meaning: 'Nomor bagi yang belum punya foto berkoordinat GPS tidak muncul di peta, tetapi dihitung. Klik tombolnya untuk melihat daftar nomor bagi tersebut; lengkapi dengan mengunggah foto yang menyimpan lokasi.' },
        { name: 'Mode ringan', meaning: 'Peta sederhana tanpa peta dasar, hemat kuota dan sinyal. Aktif otomatis bila peta dasar gagal dimuat (pesan "Peta ringan dipakai karena peta dasar gagal dimuat"), atau pilih sendiri lewat tombol Mode ringan. Gunakan "Kembali ke peta interaktif" bila sinyal sudah baik.' },
      ],
      problems: [
        { q: 'Peta kosong atau lambat?', a: 'Sinyal lemah. Peta pindah ke mode ringan sendiri setelah 8 detik; tabel progres di bawah peta tetap menampilkan data yang sama.' },
        { q: 'Nomor bagi tidak muncul di peta?', a: 'Kemungkinan belum ada foto berkoordinat GPS yang diterima. Cek daftar pada tombol "belum ada lokasi".' },
      ],
      doneWhen: 'Laporan berkala terkirim dan progres tiap kabupaten terpantau.',
    },
  ],
};
