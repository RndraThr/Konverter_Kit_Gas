import {
  ArrowRight, Download, FileSpreadsheet, FileText, ImagePlus, Camera, PenLine, Plus, Printer,
  RefreshCw, Save, ScanBarcode, Signature, Upload, Video,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { ButtonSample, FieldSample } from './GuideExamples';
import type { GuideShotSpec } from './GuideShot';

// Isi panduan pengguna. Setiap tahap dikaitkan ke permission: tahap yang
// bisa dikerjakan pengguna ditandai "Tugas Anda", tahap lain tetap tampil
// (redup) agar pengguna paham posisinya dalam alur. Tambah fitur baru cukup
// dengan menambah entri di sini.

export type GuideTask = {
  /** Nomor yang sama dengan kotak di sketsa halaman (bila ada). */
  n?: number;
  text: ReactNode;
  /** Permission yang dibutuhkan untuk langkah ini; kosong = semua yang bisa membuka halaman. */
  permissions?: string[];
};

export type GuideStep = {
  id: string;
  title: string;
  summary: string;
  /** Siapa yang biasanya mengerjakan, dalam bahasa sehari-hari. */
  doers: string;
  /** Permission untuk mengerjakan tahap ini (salah satu cukup). */
  manage: string[];
  /** Permission untuk membuka halamannya (salah satu cukup). */
  view: string[];
  route: string;
  /** Awalan path halaman yang dibantu tahap ini (untuk tombol "?"). */
  routes: string[];
  /** Screenshot halaman asli bernomor (scripts/guide-screenshots.mjs). */
  shots?: GuideShotSpec[];
  tasks: GuideTask[];
  doneWhen: string;
  tips?: string[];
};

export function hasAny(permissions: string[], required: string[]) {
  return permissions.includes('*') || required.some((permission) => permissions.includes(permission));
}

export const guideIntro = {
  title: 'Konkit untuk apa?',
  body: 'Konkit mencatat seluruh penyaluran paket konverter kit (mesin, konkit/reducer, selang, tabung LPG) kepada petani/nelayan penerima: dari persiapan program dan data penerima, pendistribusian di lapangan beserta bukti foto, dokumentasi kegiatan, sampai Berita Acara dan laporan. Setiap pengguna hanya melihat menu dan kabupaten sesuai role-nya.',
};

export const guideSteps: GuideStep[] = [
  {
    id: 'persiapan',
    title: 'Persiapan program',
    summary: 'Menyiapkan kabupaten, program, zona, template paket & dokumentasi, logo dokumen, dan jadwal distribusi.',
    doers: 'Admin pusat',
    manage: ['programs.manage'],
    view: ['programs.view'],
    route: '/persiapan-program',
    routes: ['/persiapan-program'],
    shots: [{ id: 'persiapan-jadwal', title: 'Persiapan program, tab Jadwal', legend: ['Tab tahapan persiapan: kerjakan dari kiri ke kanan', 'Daftar data pada tab yang dipilih', 'Tombol untuk menambah data baru'] }],
    tasks: [
      { n: 1, text: <>Kerjakan tab dari kiri ke kanan. Mulai dari tab <b>Kabupaten</b>: <ButtonSample icon={<Plus />}>Tambah kabupaten</ButtonSample>, isi nama dan <b>kode dokumen</b> (dipakai di nomor BA).</> },
      { text: <>Tab <b>Program</b>: <ButtonSample icon={<Plus />}>Tambah program</ButtonSample> (kode, nama, tahun anggaran, jenis petani/nelayan). Lalu tab <b>Zona</b> untuk mengelompokkan kabupaten dan No. PO per zona.</> },
      { text: <>Tab <b>Template</b>: <ButtonSample icon={<Plus />}>Tambah paket</ButtonSample> berisi merk/tipe mesin, komponen paket, dan % TKDN; <ButtonSample icon={<Plus />}>Tambah dokumentasi</ButtonSample> berisi daftar foto wajib.</> },
      { text: <>Tab <b>Logo Dokumen</b>: unggah logo yang tampil di kepala Berita Acara.</> },
      { n: 3, text: <>Tab <b>Jadwal</b>: <ButtonSample icon={<Plus />}>Tambah jadwal</ButtonSample> untuk setiap kabupaten (program, tanggal mulai–selesai, template). Jadwal inilah yang dipilih di semua menu lain.</> },
    ],
    doneWhen: 'Setiap kabupaten sasaran punya jadwal aktif dengan template paket dan dokumentasi.',
  },
  {
    id: 'akses',
    title: 'Pengguna & hak akses',
    summary: 'Membuat role untuk setiap tim beserta kabupaten yang boleh diakses, lalu membuat akun pengguna.',
    doers: 'Admin pusat',
    manage: ['roles.manage', 'users.manage'],
    view: ['roles.view', 'users.view'],
    route: '/role',
    routes: ['/role', '/pengguna'],
    shots: [{ id: 'role', title: 'Role & akses, tambah role', legend: ['Akses kabupaten: centang "Akses semua kabupaten" untuk tim pusat, atau pilih kabupaten tim', 'Simpan role'] }],
    tasks: [
      { n: 1, text: <>Menu <b>Role & akses</b>: <ButtonSample icon={<Plus />}>Tambah role</ButtonSample>, centang hak akses sesuai tugas tim, lalu di <b>Akses kabupaten</b> pilih kabupaten tim tersebut (atau <i>Akses semua kabupaten</i> untuk tim pusat). <ButtonSample>Simpan role</ButtonSample></>, permissions: ['roles.manage'] },
      { text: <>Menu <b>Pengguna</b>: <ButtonSample icon={<Plus />}>Tambah pengguna</ButtonSample>, isi nama, username, password awal, dan pilih role. <ButtonSample>Simpan pengguna</ButtonSample></>, permissions: ['users.manage'] },
    ],
    doneWhen: 'Setiap anggota tim bisa login dan hanya melihat kabupaten tugasnya.',
    tips: ['Tim berbeda kabupaten? Buat role terpisah, misalnya "Koordinator Lapangan Wonogiri".', 'Role tanpa kabupaten sama sekali tidak bisa melihat data apa pun.'],
  },
  {
    id: 'dcp3',
    title: 'Import data penerima (DCP3)',
    summary: 'Memasukkan daftar calon penerima dari file Excel DCP3 ke jadwal.',
    doers: 'Operator data',
    manage: ['dcp3.import'],
    view: ['dcp3.view'],
    route: '/dcp3',
    routes: ['/dcp3'],
    shots: [{ id: 'dcp3-jadwal', title: 'DCP3, tahap pertama', legend: ['Empat tahap import, berurutan', 'Pilih jadwal kabupaten tujuan', 'Lanjut ke tahap unggah file'] }],
    tasks: [
      { n: 2, text: <>Tahap <b>Jadwal</b>: pilih jadwal kabupaten, lalu <ButtonSample trailingIcon={<ArrowRight />}>Lanjut ke upload</ButtonSample></> },
      { text: <>Tahap <b>Workbook</b>: pilih file Excel DCP3 dan baris judul kolom, lalu <ButtonSample>Unggah dan baca file</ButtonSample></> },
      { text: <>Tahap <b>Pemetaan</b>: cocokkan kolom file dengan isian sistem (Nama, NIK, No Kartu Petani, Alamat, Desa, Kecamatan, No HP), lalu <ButtonSample trailingIcon={<ArrowRight />}>Periksa data</ButtonSample></> },
      { text: <>Tahap <b>Tinjau</b>: baris bertanda perlu ditinjau tetap bisa diimport; perbaiki yang tidak valid, lalu <ButtonSample>Import 120 data</ButtonSample></> },
    ],
    doneWhen: 'Penerima muncul di Data Penerima dan siap dihubungkan ke nomor bagi di lapangan.',
  },
  {
    id: 'penerima',
    title: 'Periksa data penerima',
    summary: 'Mencari, memeriksa, dan memperbaiki data penerima lintas jadwal.',
    doers: 'Operator data, koordinator',
    manage: ['recipients.manage'],
    view: ['recipients.view'],
    route: '/',
    routes: ['/'],
    shots: [{ id: 'data-penerima', title: 'Data Penerima', legend: ['Pencarian dan filter', 'Daftar penerima (data pribadi disamarkan di contoh ini)', 'Tambah penerima manual'] }],
    tasks: [
      { n: 1, text: <>Gunakan pencarian dan filter program/kabupaten. Klik baris untuk melihat detail penerima.</> },
      { n: 3, text: <>Penerima yang belum ada di DCP3 bisa ditambah manual: <ButtonSample icon={<Plus />}>Tambah penerima</ButtonSample></>, permissions: ['recipients.manage'] },
    ],
    doneWhen: 'NIK, nama, dan alamat penerima sudah benar sebelum hari distribusi.',
  },
  {
    id: 'distribusi',
    title: 'Pendistribusian di lapangan',
    summary: 'Tiga pos berurutan untuk setiap paket (nomor bagi): POS Mesin → POS Dokumen → POS Penyerahan, masing-masing dengan foto bukti.',
    doers: 'Tim lapangan (POS Mesin, POS Dokumen, POS Penyerahan)',
    manage: ['distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan', 'distribution.manage'],
    view: ['distribution.view'],
    route: '/dokumentasi/pendistribusian',
    routes: ['/dokumentasi/pendistribusian'],
    shots: [{ id: 'distribusi-pos', title: 'Pendistribusian, nomor bagi sedang diproses', legend: ['Pilih jadwal distribusi kabupaten', 'Katalog nomor bagi: hijau = selesai, kuning = proses, biru = terbuka, + = buat baru', 'POS Mesin: nomor seri dan foto mesin', 'POS Dokumen: hubungkan penerima, tanggal, data peralatan, foto dokumen'] }],
    tasks: [
      { n: 1, text: <>Pilih <FieldSample>Jadwal distribusi</FieldSample> kabupaten Anda.</> },
      { n: 2, text: <>Di <b>Katalog</b>, pilih nomor bagi (nomor urut paket). Nomor baru dibuat di POS Mesin dengan <ButtonSample>Mulai dokumentasi</ButtonSample></>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
      { n: 3, text: <><b>POS Mesin</b>: scan nomor seri mesin dengan <ButtonSample variant="outline" icon={<ScanBarcode />}>Scan</ButtonSample> atau ketik manual, <ButtonSample>Simpan nomor seri</ButtonSample>, lalu unggah foto mesin & nomor seri dengan <ButtonSample variant="outline" icon={<Camera />}>Ambil Foto</ButtonSample></>, permissions: ['distribution.pos_mesin', 'distribution.manage'] },
      { n: 4, text: <><b>POS Dokumen</b>: ketik awalan NIK, pilih penerima, lalu <ButtonSample>Hubungkan ke Nomor Bagi Ini</ButtonSample>. Isi tanggal (<ButtonSample variant="outline">Simpan tanggal distribusi</ButtonSample>), merk peralatan (<ButtonSample>Simpan data peralatan</ButtonSample>), tanda tangan penerima (opsional, bisa tanda tangan basah setelah dicetak), dan foto KTP/KK/kartu tani.</>, permissions: ['distribution.pos_dokumen', 'distribution.manage'] },
      { text: <><b>POS Penyerahan</b>: setelah dokumen lengkap, unggah foto serah terima lalu <ButtonSample>Selesaikan Distribusi</ButtonSample> dan konfirmasi.</>, permissions: ['distribution.pos_penyerahan', 'distribution.manage'] },
      { text: <>Data yang sudah selesai bisa diperbaiki lewat <ButtonSample variant="outline">Buka revisi POS Dokumen</ButtonSample> (alasan revisi tercatat).</>, permissions: ['distribution.pos_mesin', 'distribution.pos_dokumen', 'distribution.pos_penyerahan'] },
    ],
    doneWhen: 'Nomor bagi berstatus Selesai dengan semua foto wajib Lengkap.',
    tips: ['POS Penyerahan terkunci sampai POS Dokumen selesai.', 'Foto bisa langsung dari kamera HP; unggahan yang gagal bisa dicoba lagi tanpa memilih ulang.'],
  },
  {
    id: 'kegiatan',
    title: 'Dokumentasi kegiatan',
    summary: 'Foto/video Ceremony & Sosialisasi, Pelatihan Teknis, Rakor, Training 10%/100%, dan Unloading barang.',
    doers: 'Tim dokumentasi, koordinator',
    manage: ['activities.manage'],
    view: ['activities.view'],
    route: '/dokumentasi/ceremony-sosialisasi',
    routes: ['/dokumentasi/ceremony-sosialisasi', '/dokumentasi/pelatihan-teknis', '/dokumentasi/rakor', '/dokumentasi/training-10', '/dokumentasi/training-100', '/dokumentasi/unloading'],
    shots: [{ id: 'kegiatan', title: 'Dokumentasi kegiatan', legend: ['Pilih program dan kabupaten', 'Unggah foto/video dari kamera atau galeri'] }],
    tasks: [
      { n: 1, text: <>Buka menu kegiatan di grup <b>Dokumentasi</b>, pilih program dan kabupaten.</> },
      { n: 2, text: <>Unggah dengan <ButtonSample variant="outline" icon={<Camera />}>Ambil Foto</ButtonSample> <ButtonSample variant="outline" icon={<Video />}>Rekam Video</ButtonSample> atau <ButtonSample variant="outline" icon={<ImagePlus />}>Pilih dari Galeri</ButtonSample></>, permissions: ['activities.manage'] },
      { text: <>Hasil tampil di galeri; klik untuk melihat lebih besar.</> },
    ],
    doneWhen: 'Setiap kegiatan di kabupaten punya foto/video pendukung.',
  },
  {
    id: 'berita-acara',
    title: 'Berita Acara',
    summary: 'Membuat, menandatangani, dan memfinalisasi BA: DP3, BA Perorangan, Rekap Harian, Closing, Sosialisasi, Training, Pemeriksaan, Servis Berkala, TKDN.',
    doers: 'Koordinator / admin BA',
    manage: ['bast.manage'],
    view: ['bast.view'],
    route: '/berita-acara',
    routes: ['/berita-acara'],
    shots: [
      { id: 'berita-acara', title: 'Berita Acara, Closing Titik Serah', legend: ['Pilih jadwal kabupaten', 'Pilih jenis BA', 'Tanggal dokumen dan informasi BA', 'Buka pengaturan penandatangan', 'Finalisasi setelah preview benar (aktif bila data lengkap)'] },
      { id: 'penandatangan', title: 'Penandatangan', legend: ['Pilih pihak yang akan diatur', 'Tabel tanda tangan persis seperti cetak: klik, geser, ubah ukuran, putar', 'Nama, NIP, tanda tangan, stempel, ukuran & putar untuk pihak terpilih', 'Simpan semua perubahan'] },
    ],
    tasks: [
      { n: 1, text: <>Pilih jadwal kabupaten, lalu <b>tab jenis BA</b> yang akan dibuat.</> },
      { n: 3, text: <>Isi tanggal dokumen dan informasi yang diminta (mis. Lokasi/Titik Serah). Preview langsung diperbarui.</> },
      { n: 4, text: <>Klik <ButtonSample variant="ghost" icon={<Signature />}>Penandatangan</ButtonSample>: tabel tanda tangan tampil persis seperti cetak. Pilih kolom pihak, isi nama/NIP, <ButtonSample variant="outline" icon={<PenLine />}>Gambar tanda tangan</ButtonSample> langsung di kolomnya, unggah stempel, geser/putar/ubah ukuran, lalu <ButtonSample icon={<Save />}>Simpan</ButtonSample></>, permissions: ['bast.manage'] },
      { text: <>Tanda tangan basah: nyalakan switch <b>Tanda tangan basah</b>, <ButtonSample variant="outline" icon={<Printer />}>Cetak halaman tanda tangan</ButtonSample>, tandatangani & stempel, lalu <ButtonSample icon={<Upload />}>Unggah hasil pindai</ButtonSample> (PDF/JPG/foto).</>, permissions: ['bast.manage'] },
      { n: 5, text: <>Periksa preview, lalu <ButtonSample icon={<RefreshCw />}>Finalisasi & sinkronkan</ButtonSample>. Versi final bisa diunduh dengan <ButtonSample variant="outline" icon={<Download />}>Unduh PDF</ButtonSample></>, permissions: ['bast.manage'] },
    ],
    doneWhen: 'BA sudah difinalisasi: muncul kotak nama file PDF dengan tombol Unduh PDF, dan versinya tercatat.',
    tips: ['Tombol Finalisasi abu-abu? Periksa pesan kuning di panel: data belum lengkap, atau mode tanda tangan basah menunggu pindaian.', 'Data berubah setelah final → versi lama ditandai "Perlu dibuat ulang"; finalisasi lagi untuk versi baru.'],
  },
  {
    id: 'laporan',
    title: 'Laporan & pemantauan',
    summary: 'Memantau progres distribusi per kabupaten di peta dan laporan, lalu mengekspornya.',
    doers: 'Koordinator, pimpinan',
    manage: ['distribution.manage'],
    view: ['distribution.view'],
    route: '/laporan',
    routes: ['/laporan', '/map-distribusi'],
    shots: [{ id: 'laporan', title: 'Laporan', legend: ['Pilih jadwal dan filter status', 'Unduh laporan Excel atau PDF', 'Tabel laporan per penerima (disamarkan di contoh ini)'] }],
    tasks: [
      { n: 1, text: <>Menu <b>Laporan</b>: pilih program, kabupaten, dan jadwal.</> },
      { n: 2, text: <>Unduh dengan <ButtonSample variant="outline" icon={<FileSpreadsheet />} trailingIcon={<Download />}>Export Excel</ButtonSample> atau <ButtonSample variant="outline" icon={<FileText />} trailingIcon={<Download />}>Export PDF</ButtonSample></> },
      { text: <>Menu <b>Map Distribusi</b> menampilkan sebaran penerima per wilayah.</> },
    ],
    doneWhen: 'Laporan berkala terkirim ke pihak terkait.',
  },
];

export const glossary: { term: string; meaning: string }[] = [
  { term: 'DCP3', meaning: 'Daftar Calon Penerima Paket: file Excel berisi data petani/nelayan calon penerima.' },
  { term: 'Jadwal', meaning: 'Rencana distribusi satu program di satu kabupaten; hampir semua menu dimulai dengan memilih jadwal.' },
  { term: 'Nomor bagi', meaning: 'Nomor urut paket yang dibagikan; satu nomor bagi = satu paket = satu penerima.' },
  { term: 'POS Mesin / Dokumen / Penyerahan', meaning: 'Tiga meja kerja di lokasi distribusi, dikerjakan berurutan untuk setiap nomor bagi.' },
  { term: 'BA', meaning: 'Berita Acara: dokumen resmi serah terima dan rekap yang ditandatangani para pihak.' },
  { term: 'Finalisasi', meaning: 'Mengunci BA menjadi versi resmi (PDF) dan menyinkronkannya ke arsip.' },
  { term: 'Tanda tangan basah', meaning: 'Tanda tangan dan stempel asli di kertas; halaman yang sudah ditandatangani dipindai lalu diunggah.' },
  { term: 'TKDN', meaning: 'Tingkat Komponen Dalam Negeri: persentase komponen produksi dalam negeri pada paket.' },
];

/** Tahap yang memuat halaman pathname (untuk tombol "?"). */
export function stepsForPath(pathname: string) {
  return guideSteps.filter((step) => step.routes.some((route) => route === '/' ? pathname === '/' : pathname === route || pathname.startsWith(`${route}/`)));
}
