import { Download, FileSpreadsheet, FileText, RefreshCw } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const laporan: GuideTopic = {
  slug: 'laporan',
  stepId: 'laporan',
  title: 'Laporan DP3 & Map Distribusi',
  intro: 'Laporan DP3 berisi data penerima yang distribusinya sudah selesai per kabupaten, tanpa status, dan bisa diunduh sebagai Excel atau PDF. Excel-nya juga tersimpan otomatis di folder kabupaten di Drive. Map Distribusi menampilkan setiap nomor bagi sebagai titik di peta, berwarna sesuai milestone penyaluran.',
  manage: ['distribution.manage'],
  view: ['distribution.view'],
  route: '/laporan',
  sections: [
    {
      id: 'laporan',
      title: 'Laporan DP3 per kabupaten',
      summary: 'Pilih jadwal kabupaten. Laporan hanya memuat penerima yang distribusinya sudah selesai, bernomor sama dengan BA DP3.',
      steps: [
        { n: 1, text: <>Buka menu <b>Laporan DP3</b> dan pilih <FieldSample>Jadwal</FieldSample>.</> },
        { n: 2, text: <>Tabel menampilkan identitas, nomor kartu, nomor HP, desa, tanggal dan titik serah, serta mesin tiap penerima. Penerima yang belum selesai diserahkan tidak ikut.</> },
      ],
      fields: [
        { name: 'Penerima selesai', meaning: 'Jumlah penerima yang distribusinya sudah diselesaikan di POS Penyerahan.' },
        { name: 'Hari distribusi / titik serah / desa', meaning: 'Banyaknya tanggal, lokasi penyerahan, dan desa yang tercakup di laporan.' },
        { name: 'Excel di Drive', meaning: 'File Laporan DP3 di folder kabupaten. Diperbarui otomatis tiap 10 menit bila datanya berubah.' },
      ],
      problems: [
        { q: 'Penerima belum muncul?', a: 'Laporan DP3 hanya memuat distribusi yang sudah selesai. Selesaikan nomor bagi di POS Penyerahan terlebih dahulu.' },
        { q: 'Muncul "Pembaruan terakhir gagal"?', a: 'Biasanya karena koneksi ke Drive terputus. Sistem mencoba lagi otomatis; pengguna dengan akses kelola distribusi bisa menekan Perbarui sekarang.' },
      ],
    },
    {
      id: 'ekspor',
      title: 'Unduh Laporan DP3',
      summary: 'Excel memuat data lengkap (termasuk nomor seri converter dan selang); PDF berisi ringkasan kolom utama, siap cetak A4 landscape.',
      steps: [
        { n: 1, text: <>Klik <ButtonSample variant="outline" icon={<FileSpreadsheet />} trailingIcon={<Download />}>Export Excel</ButtonSample> untuk diolah lebih lanjut, atau <ButtonSample variant="outline" icon={<FileText />} trailingIcon={<Download />}>Export PDF</ButtonSample> untuk dikirim atau dicetak.</> },
        { text: <>Perlu file di Drive diperbarui sekarang juga? Klik <ButtonSample variant="ghost" icon={<RefreshCw />}>Perbarui sekarang</ButtonSample> di kartu Excel di Drive.</>, permissions: ['distribution.manage'] },
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
