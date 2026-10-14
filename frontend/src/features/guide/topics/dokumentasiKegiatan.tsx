import { Camera, ImagePlus, Trash2, Video } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const dokumentasiKegiatan: GuideTopic = {
  slug: 'dokumentasi-kegiatan',
  stepId: 'kegiatan',
  title: 'Dokumentasi kegiatan',
  intro: 'Foto dan video kegiatan di luar serah terima per penerima: Ceremony & Sosialisasi, Pelatihan Teknis, Rakor, Training 10%, Training 100%, dan Unloading barang. Dokumentasi dicatat per program dan kabupaten, tidak terikat jadwal, dan disimpan ke folder dokumen kabupaten.',
  manage: ['activities.manage'],
  view: ['activities.view'],
  route: '/dokumentasi/ceremony-sosialisasi',
  sections: [
    {
      id: 'unggah',
      title: 'Pilih kegiatan dan unggah',
      summary: 'Setiap jenis kegiatan punya menu sendiri di grup Dokumentasi pada menu samping.',
      shots: [{ id: 'kg-unggah', title: 'Ceremony & Sosialisasi', legend: ['Program / tender', 'Kabupaten / kota', 'Ambil foto langsung dengan kamera', 'Rekam video', 'Pilih foto/video dari galeri'] }],
      steps: [
        { text: <>Buka menu kegiatan di grup <b>Dokumentasi</b>, misalnya <b>Ceremony & Sosialisasi</b>.</> },
        { n: 1, text: <>Pilih <FieldSample>Program / Tender</FieldSample> lalu <FieldSample>Kabupaten / Kota</FieldSample>.</> },
        { n: 3, text: <>Unggah dengan <ButtonSample variant="outline" icon={<Camera />}>Ambil Foto</ButtonSample>, <ButtonSample variant="outline" icon={<Video />}>Rekam Video</ButtonSample>, atau <ButtonSample variant="outline" icon={<ImagePlus />}>Pilih dari Galeri</ButtonSample>. Di komputer, file juga bisa diseret ke kotak unggah.</>, permissions: ['activities.manage'] },
      ],
      problems: [
        { q: 'Unggahan tertahan atau gagal?', a: 'Pastikan kabupaten sudah masuk zona di Persiapan program, lalu coba lagi. Unggahan video besar butuh sinyal stabil.' },
      ],
    },
    {
      id: 'galeri',
      title: 'Galeri dokumentasi',
      summary: 'Semua foto/video kabupaten yang dipilih, terbaru di depan. Nama file dibuat otomatis: kode kabupaten, kegiatan, dan waktu.',
      shots: [
        { id: 'kg-galeri', title: 'Galeri (foto contoh)', legend: ['Jumlah dokumentasi', 'Klik untuk melihat lebih besar', 'Hapus dokumentasi'] },
        { id: 'kg-pratinjau', title: 'Pratinjau foto', legend: ['Foto ukuran besar', 'Perkecil/perbesar', 'Tutup pratinjau'] },
      ],
      steps: [
        { n: 2, text: <>Klik foto untuk membuka pratinjau; geser dengan panah kanan/kiri untuk foto berikutnya.</> },
        { n: 3, text: <>Klik <ButtonSample variant="ghost" icon={<Trash2 />}>Hapus</ButtonSample> untuk menghapus foto yang salah.</>, permissions: ['activities.manage'] },
      ],
    },
    {
      id: 'unloading',
      title: 'Unloading barang',
      summary: 'Unloading mendokumentasikan kedatangan barang per jenis.',
      shots: [{ id: 'kg-unloading', title: 'Unloading', legend: ['Jenis barang: Konkit, Mesin Pompa, Oli, Selang, Tabung Gas', 'Program / tender', 'Kabupaten / kota'] }],
      steps: [
        { n: 1, text: <>Pilih tab jenis barang terlebih dahulu, lalu program dan kabupaten seperti kegiatan lain.</> },
      ],
      doneWhen: 'Setiap kegiatan di kabupaten punya foto/video pendukung untuk laporan dan Berita Acara kegiatan.',
    },
  ],
};
