import { Camera, ImagePlus, Pencil, ScanBarcode } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const pendistribusian: GuideTopic = {
  slug: 'pendistribusian',
  stepId: 'distribusi',
  title: 'Pendistribusian',
  intro: 'Setiap paket punya satu nomor bagi. Di lokasi distribusi, nomor bagi dikerjakan berurutan di tiga meja: POS Mesin (catat mesin & foto), POS Dokumen (hubungkan penerima, data peralatan, foto dokumen), lalu POS Penyerahan (serah terima). Setiap POS hanya bisa dikerjakan pemegang aksesnya. Contoh di halaman ini disamarkan: foto, tanda tangan, dan identitas penerima diburamkan.',
  manage: ['distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan', 'distribution.manage'],
  view: ['distribution.view'],
  route: '/dokumentasi/pendistribusian',
  sections: [
    {
      id: 'katalog',
      title: 'Pilih jadwal dan nomor bagi',
      summary: 'Katalog menampilkan semua nomor bagi jadwal beserta statusnya.',
      shots: [{ id: 'ds-katalog', title: 'Katalog nomor bagi', legend: ['Pilih jadwal distribusi', 'Jumlah nomor bagi terpakai dari kuota', 'Arti warna: Terbuka, Proses, Selesai', 'Nomor bagi yang sudah dibuat', 'Nomor berikutnya yang bisa dibuat (+)', 'Area kerja POS setelah nomor dipilih'] }],
      steps: [
        { n: 1, text: <>Pilih <FieldSample>Jadwal distribusi</FieldSample> kabupaten.</> },
        { n: 4, text: <>Klik nomor bagi untuk membukanya. <b>Biru/Terbuka</b>: belum dihubungkan ke penerima; <b>Kuning/Proses</b>: sedang dikerjakan; <b>Hijau/Selesai</b>: sudah diserahkan.</> },
        { n: 5, text: <>Kotak bertanda <b>+</b> adalah nomor baru yang bisa dibuat oleh POS Mesin.</> },
      ],
    },
    {
      id: 'buat-nomor',
      title: 'Membuat nomor bagi',
      summary: 'Nomor bagi dibuat di POS Mesin saat unit mesin mulai didata.',
      shots: [{ id: 'ds-buat', title: 'Buat nomor bagi', legend: ['Nomor baru yang dipilih', 'Formulir POS Mesin untuk nomor ini', 'Simpan dan mulai dokumentasi mesin'] }],
      steps: [
        { n: 1, text: <>Klik kotak <b>+</b> nomor berikutnya.</>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
        { n: 3, text: <>Klik <ButtonSample>Mulai dokumentasi</ButtonSample>. Tanggal, penerima, dan data peralatan dilengkapi kemudian di POS Dokumen.</>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
      ],
      problems: [{ q: 'Tidak bisa membuat nomor baru?', a: 'Kuota nomor bagi jadwal sudah habis, atau akun Anda tidak memiliki akses POS Mesin.' }],
    },
    {
      id: 'pos-mesin',
      title: 'POS Mesin',
      summary: 'Mencatat nomor seri dari kartu di dalam box dan memotret mesin.',
      shots: [{ id: 'ds-mesin', title: 'POS Mesin', legend: ['Nomor seri mesin', 'Scan barcode dengan kamera', 'Simpan nomor seri', 'Ambil foto langsung dengan kamera', 'Pilih foto dari galeri'] }],
      steps: [
        { n: 2, text: <>Scan barcode kartu di dalam box dengan <ButtonSample variant="outline" icon={<ScanBarcode />}>Scan</ButtonSample>, atau ketik nomor seri mesin dan konkit/reducer.</>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
        { n: 3, text: <>Klik <ButtonSample>Simpan nomor seri</ButtonSample>. Bila kartu belum ada, nomor seri bisa dilengkapi di POS Dokumen.</>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
        { n: 4, text: <>Isi setiap slot foto wajib dengan <ButtonSample variant="outline" icon={<Camera />}>Buka kamera</ButtonSample> atau <ButtonSample variant="outline" icon={<ImagePlus />}>Pilih galeri</ButtonSample>. Slot berubah menjadi <b>Lengkap</b> setelah tersimpan.</>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
      ],
      problems: [{ q: 'Unggahan foto gagal?', a: 'Periksa sinyal, lalu tekan Coba lagi pada foto yang gagal; foto tidak perlu dipilih ulang.' }],
    },
    {
      id: 'pos-dokumen-hubungkan',
      title: 'POS Dokumen: hubungkan penerima',
      summary: 'Menghubungkan nomor bagi ke penerima dari data DCP3 melalui NIK.',
      shots: [{ id: 'ds-dokumen-hubungkan', title: 'POS Dokumen, nomor bagi belum terhubung', legend: ['Tanggal distribusi', 'Simpan tanggal', 'Ketik minimal 4 digit NIK penerima', 'Data peralatan paket'] }],
      steps: [
        { n: 1, text: <>Isi tanggal distribusi lalu <ButtonSample variant="outline">Simpan tanggal distribusi</ButtonSample>.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
        { n: 3, text: <>Ketik minimal 4 digit NIK dari KTP penerima, pilih nama yang muncul, lalu <ButtonSample>Hubungkan ke Nomor Bagi Ini</ButtonSample>.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
      ],
      problems: [
        { q: 'NIK tidak ditemukan?', a: 'Penerima belum ada di DCP3/Data Penerima jadwal ini, sudah terhubung ke nomor bagi lain, atau dibatalkan. Tambahkan di Data Penerima bila memang berhak.' },
        { q: 'NIK masih terpasang di nomor bagi lain?', a: 'Sistem menolak penerima ganda. Periksa nomor bagi lama; bila salah, buka revisi di nomor tersebut.' },
      ],
    },
    {
      id: 'pos-dokumen-penerima',
      title: 'POS Dokumen: data penerima & tanda tangan',
      summary: 'Setelah terhubung, lengkapi identitas dan (opsional) tanda tangan penerima.',
      shots: [
        { id: 'ds-dokumen-penerima', title: 'Data penerima (disamarkan)', legend: ['NIK terpasang', 'Nomor telepon dan data lain dapat dilengkapi', 'Simpan perubahan data', 'Ganti penerima bila salah orang'] },
        { id: 'ds-dokumen-ttd', title: 'Tanda tangan penerima (disamarkan)', legend: ['Tanda tangan opsional: bila kosong, ditandatangani basah setelah BA dicetak', 'Ulangi tanda tangan'] },
      ],
      steps: [
        { n: 2, text: <>Periksa dan lengkapi nomor kartu, telepon, dan alamat, lalu <ButtonSample>Simpan data penerima</ButtonSample>.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
        { n: 4, text: <>Salah menghubungkan penerima? Klik <ButtonSample variant="outline">Ganti penerima</ButtonSample> dan isi alasannya; riwayat penggantian tersimpan.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
        { text: <>Minta penerima menandatangani di layar (opsional). Tanda tangan dipakai di BA Perorangan dan daftar hadir Training.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
      ],
    },
    {
      id: 'pos-dokumen-peralatan',
      title: 'POS Dokumen: data peralatan & foto dokumen',
      summary: 'Memilih merk peralatan yang benar-benar diterima dan memotret dokumen penerima.',
      shots: [{ id: 'ds-dokumen-peralatan', title: 'Data peralatan', legend: ['Merk/tipe mesin', 'Merk konkit/reducer', 'Merk/spesifikasi selang', 'Simpan data peralatan'] }],
      steps: [
        { n: 1, text: <>Pilih merk/tipe sesuai barang yang diserahkan (pilihan berasal dari template paket jadwal), lengkapi nomor seri bila belum, lalu <ButtonSample>Simpan data peralatan</ButtonSample>.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
        { text: <>Isi foto wajib: KTP, KK, kartu tani/surat keterangan, surat keterangan luas lahan, dan foto penerima dengan KTP. Foto bertanda <b>Wajib</b> harus lengkap sebelum POS Penyerahan terbuka.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
      ],
    },
    {
      id: 'pos-penyerahan',
      title: 'POS Penyerahan',
      summary: 'Serah terima paket ke penerima. Terbuka setelah POS Dokumen lengkap.',
      shots: [
        { id: 'ds-penyerahan-terkunci', title: 'POS Penyerahan terkunci', legend: ['Menunggu POS Dokumen lengkap'] },
        { id: 'ds-selesai', title: 'Distribusi selesai (foto disamarkan)', legend: ['Status selesai dan tanggal serah terima', 'Buka revisi bila ada data yang perlu diperbaiki'] },
      ],
      steps: [
        { n: 1, text: <>Selama dokumen belum lengkap, POS Penyerahan menampilkan <i>Menunggu dokumen selesai</i>.</> },
        { text: <>Setelah terbuka: unggah foto serah terima (penerima dengan paket, dengan teknisi, untuk training, dengan mesin lama), lalu <ButtonSample>Selesaikan Distribusi</ButtonSample>.</>, permissions: ['distribution.pos_penyerahan', 'distribution.manage'] },
        { text: <>Periksa kembali, lalu <ButtonSample>Konfirmasi Penyerahan</ButtonSample>. Aksi ini tidak dapat dibatalkan dari halaman ini.</>, permissions: ['distribution.pos_penyerahan', 'distribution.manage'] },
      ],
      doneWhen: 'Nomor bagi berwarna hijau (Selesai) dan semua foto wajib Lengkap.',
    },
    {
      id: 'revisi',
      title: 'Revisi data yang sudah selesai',
      summary: 'Data POS yang sudah selesai dikunci. Untuk mengubahnya, buka revisi dengan alasan.',
      shots: [{ id: 'ds-revisi', title: 'Buka revisi', legend: ['Alasan revisi (tercatat di riwayat)', 'Buka revisi'] }],
      steps: [
        { text: <>Klik <ButtonSample variant="outline" icon={<Pencil />}>Buka revisi POS Dokumen</ButtonSample> (atau POS Mesin/Penyerahan) pada nomor bagi.</>, permissions: ['distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan'] },
        { n: 1, text: <>Isi alasan, lalu <ButtonSample>Buka revisi</ButtonSample>. Setelah diperbaiki, nomor bagi harus <b>diselesaikan ulang</b> di POS Penyerahan.</>, permissions: ['distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan'] },
      ],
      problems: [{ q: 'Tidak ada tombol revisi?', a: 'Tombol revisi setiap POS hanya tampil untuk pemegang akses POS tersebut, misalnya revisi POS Dokumen untuk akses POS Dokumen.' }],
    },
  ],
};
