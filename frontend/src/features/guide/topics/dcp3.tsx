import { ArrowRight } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const dcp3: GuideTopic = {
  slug: 'dcp3',
  stepId: 'dcp3',
  title: 'Import DCP3',
  intro: 'DCP3 (Daftar Calon Penerima Paket) adalah file Excel daftar petani/nelayan calon penerima. Import memasukkan isinya ke jadwal kabupaten, lengkap dengan isi paket dari template jadwal. Prosesnya empat tahap: Jadwal → Workbook → Pemetaan → Tinjau.',
  manage: ['dcp3.import'],
  view: ['dcp3.view'],
  route: '/dcp3',
  sections: [
    {
      id: 'jadwal',
      title: 'Tahap 1: Pilih jadwal',
      summary: 'Data penerima selalu masuk ke satu jadwal (satu kabupaten).',
      shots: [{ id: 'dcp3-1-jadwal', title: 'DCP3, tahap Jadwal', legend: ['Empat tahap import', 'Pilih jadwal kabupaten tujuan', 'Lanjut ke tahap unggah file'] }],
      steps: [
        { n: 2, text: <>Pilih <FieldSample>Jadwal</FieldSample> kabupaten tujuan. Hanya jadwal berstatus Aktif di kabupaten akses Anda yang muncul.</> },
        { n: 3, text: <>Klik <ButtonSample trailingIcon={<ArrowRight />}>Lanjut ke upload</ButtonSample>.</>, permissions: ['dcp3.import'] },
      ],
    },
    {
      id: 'workbook',
      title: 'Tahap 2: Unggah workbook',
      summary: 'Unggah file Excel (.xlsx) DCP3. Batas 5.000 baris dan 10 MB per file.',
      shots: [{ id: 'dcp3-2-workbook', title: 'DCP3, tahap Workbook', legend: ['Pilih atau seret file .xlsx ke sini', 'Unggah dan baca isi file'] }],
      steps: [
        { n: 1, text: <>Klik area unggah atau seret file .xlsx ke dalamnya.</>, permissions: ['dcp3.import'] },
        { n: 2, text: <>Klik <ButtonSample>Unggah dan baca file</ButtonSample>. Baris pertama dianggap baris judul kolom.</>, permissions: ['dcp3.import'] },
      ],
      problems: [
        { q: 'Judul kolom tidak berada di baris pertama (ada kop/judul laporan di atasnya)?', a: <>Sistem menampilkan beberapa baris awal file. Pilih <b>Baris</b> yang berisi judul kolom, lalu klik <ButtonSample>Coba lagi dengan baris ini</ButtonSample>.</> },
        { q: '"File DCP3 ini sudah pernah diunggah pada jadwal yang sama"?', a: 'File yang persis sama sudah pernah diunggah ke jadwal ini. Periksa riwayat import; bila memang ada data baru, gunakan file yang sudah diperbarui.' },
        { q: 'File ditolak karena terlalu besar?', a: 'Pecah file menjadi beberapa bagian (maksimal 5.000 baris, 10 MB).' },
      ],
    },
    {
      id: 'pemetaan',
      title: 'Tahap 3: Cocokkan kolom',
      summary: 'Setiap file DCP3 bisa punya judul kolom berbeda. Di tahap ini Anda menunjukkan kolom mana yang berisi nama, NIK, dan seterusnya.',
      shots: [{ id: 'dcp3-3-pemetaan', title: 'DCP3, tahap Pemetaan', legend: ['Nama lengkap (wajib)', 'NIK', 'Nomor kartu petani', 'Lanjut ke pemeriksaan data'] }],
      steps: [
        { n: 1, text: <>Untuk setiap isian, pilih kolom Excel yang sesuai. Isian bertanda <b>*</b> wajib dipetakan.</>, permissions: ['dcp3.import'] },
        { n: 4, text: <>Klik <ButtonSample trailingIcon={<ArrowRight />}>Periksa data</ButtonSample>.</>, permissions: ['dcp3.import'] },
      ],
      fields: [
        { name: 'Nama lengkap *', meaning: 'Nama penerima sesuai KTP.', example: 'Kolom "Nama"', required: true },
        { name: 'NIK', meaning: 'NIK 16 digit; dipakai mencocokkan penerima di POS Dokumen dan mencegah penerima ganda.', example: 'Kolom "NIK"', required: false },
        { name: 'Nomor kartu petani', meaning: 'Nomor kartu tani/KUSUKA.', example: 'Kolom "No Kartu Petani"', required: false },
        { name: 'Alamat / Desa / Kecamatan', meaning: 'Alamat penerima; desa dan kecamatan dipakai di rekap dan laporan.', example: 'Kolom "Alamat", "Desa", "Kecamatan"', required: false },
        { name: 'Nomor telepon', meaning: 'Nomor HP penerima.', example: 'Kolom "No HP"', required: false },
      ],
    },
    {
      id: 'tinjau',
      title: 'Tahap 4: Tinjau dan import',
      summary: 'Sistem memeriksa setiap baris sebelum data disimpan.',
      shots: [{ id: 'dcp3-4-tinjau', title: 'DCP3, tahap Tinjau', legend: ['Ringkasan: valid, peringatan, konflik', 'Pratinjau baris dan status awal', 'Import semua baris'] }],
      steps: [
        { n: 1, text: <>Periksa ringkasan. <b>Valid</b> siap diimport, <b>Peringatan</b> tetap bisa diimport tetapi perlu ditinjau, <b>Konflik</b> perlu diperbaiki (mis. NIK sudah terdaftar di penerima lain).</> },
        { n: 3, text: <>Klik <ButtonSample>Import 120 data</ButtonSample> (angka sesuai jumlah baris).</>, permissions: ['dcp3.import'] },
        { text: <>Setelah selesai, penerima muncul di <b>Data Penerima</b> dan siap dihubungkan ke nomor bagi di POS Dokumen.</> },
      ],
      problems: [
        { q: 'Banyak baris berstatus Peringatan?', a: 'Biasanya data opsional kosong (NIK, kartu tani, HP). Lengkapi di Data Penerima atau saat POS Dokumen.' },
        { q: 'Salah memetakan kolom?', a: 'Klik Kembali ke tahap Pemetaan sebelum menekan Import.' },
      ],
      doneWhen: 'Jumlah penerima di Data Penerima untuk jadwal ini sesuai jumlah baris DCP3.',
    },
  ],
};
