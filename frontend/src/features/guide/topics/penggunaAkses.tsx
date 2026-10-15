import { Ellipsis, Plus } from 'lucide-react';
import { ButtonSample, FieldSample } from '../GuideExamples';
import type { GuideTopic } from '../guideTopics';

export const penggunaAkses: GuideTopic = {
  slug: 'pengguna-akses',
  stepId: 'akses',
  title: 'Pengguna & hak akses',
  intro: 'Setiap pengguna mendapat satu atau beberapa role. Role menentukan dua hal: menu apa yang bisa dibuka dan diubah (hak akses), serta kabupaten mana yang datanya terlihat (akses kabupaten). Buat role dulu, baru pengguna.',
  manage: ['roles.manage', 'users.manage'],
  view: ['roles.view', 'users.view'],
  route: '/role',
  sections: [
    {
      id: 'role',
      title: 'Daftar role',
      summary: 'Role & akses menampilkan semua role beserta jumlah hak akses dan penggunanya. Role bawaan tim lapangan sudah tersedia: POS Mesin, POS Dokumen, POS Penyerahan, Tim Lapangan, Dokumentasi Kegiatan, dan Koordinator Lapangan.',
      shots: [
        { id: 'ak-role-list', title: 'Role & akses', legend: ['Daftar role: hak akses, jumlah pengguna, jenis', 'Tambah role baru', 'Menu aksi role'] },
        { id: 'ak-role-menu', title: 'Menu aksi role', legend: ['Ubah role', 'Hapus role (tidak bisa untuk role yang masih dipakai)'] },
      ],
      steps: [
        { n: 2, text: <>Klik <ButtonSample icon={<Plus />}>Tambah role</ButtonSample> untuk membuat role baru.</>, permissions: ['roles.manage'] },
        { n: 3, text: <>Untuk mengubah role, klik <ButtonSample variant="ghost" icon={<Ellipsis />}>Aksi</ButtonSample> lalu <b>Edit</b>.</>, permissions: ['roles.manage'] },
      ],
      problems: [{ q: 'Role tidak bisa dihapus?', a: 'Role sistem atau role yang masih dipakai pengguna tidak bisa dihapus. Pindahkan penggunanya ke role lain terlebih dahulu.' }],
    },
    {
      id: 'hak-akses',
      title: 'Hak akses role',
      summary: 'Centang hak akses sesuai tanggung jawab tim. "Lihat" hanya membuka menu; "Kelola" bisa menambah dan mengubah data.',
      shots: [{ id: 'ak-role-dialog', title: 'Modal Edit role', legend: ['Kode role (tidak bisa diubah setelah dibuat)', 'Nama role', 'Deskripsi tugas', 'Daftar hak akses per kelompok menu'] }],
      steps: [
        { n: 1, text: <>Isi kode dan nama role, misalnya <FieldSample>koordinator_wonogiri</FieldSample> dan <FieldSample>Koordinator Lapangan Wonogiri</FieldSample>.</>, permissions: ['roles.manage'] },
        { n: 4, text: <>Centang hak akses. Contoh tim POS Dokumen: <i>Lihat Dashboard</i>, <i>Lihat Pendistribusian</i>, <i>POS Dokumen</i>, <i>Kelola Dokumentasi</i>.</>, permissions: ['roles.manage'] },
      ],
      fields: [
        { name: 'Lihat … / Kelola …', meaning: 'Pasangan hak akses per menu. Kelola mencakup menambah, mengubah, dan menghapus.', example: 'Lihat Berita Acara, Kelola Berita Acara' },
        { name: 'POS Mesin / Dokumen / Penyerahan', meaning: 'Hak mengerjakan masing-masing POS di halaman Pendistribusian.', example: 'POS Mesin' },
        { name: 'Kelola Pendistribusian', meaning: 'Melengkapi data dan mengonfirmasi distribusi; biasanya untuk koordinator bersama ketiga akses POS.', example: 'Koordinator' },
        { name: 'Import DCP3', meaning: 'Mengunggah file DCP3 menjadi data penerima.', example: 'Operator data' },
      ],
    },
    {
      id: 'akses-kabupaten',
      title: 'Akses kabupaten',
      summary: 'Bagian paling bawah modal role menentukan kabupaten yang datanya terlihat oleh pemegang role ini.',
      shots: [{ id: 'ak-role-regency', title: 'Akses kabupaten', legend: ['Akses semua kabupaten: untuk tim pusat', 'Atau centang kabupaten tugas tim'] }],
      steps: [
        { n: 1, text: <>Tim pusat: centang <b>Akses semua kabupaten</b>.</>, permissions: ['roles.manage'] },
        { n: 2, text: <>Tim daerah: centang hanya kabupaten tugasnya, lalu <ButtonSample>Simpan role</ButtonSample>.</>, permissions: ['roles.manage'] },
      ],
      problems: [
        { q: 'Pengguna tidak melihat data apa pun?', a: 'Role-nya belum diberi kabupaten. Role tanpa kabupaten sama sekali tidak melihat data kabupaten mana pun.' },
        { q: 'Dua tim beda kabupaten memakai role yang sama?', a: 'Buat role terpisah per tim, misalnya "Koordinator Lapangan Wonogiri" dan "Koordinator Lapangan Blora", dengan hak akses sama tetapi kabupaten berbeda.' },
      ],
    },
    {
      id: 'pengguna',
      title: 'Daftar pengguna',
      summary: 'Pengguna adalah akun login. Satu pengguna bisa memegang beberapa role; aksesnya gabungan dari semua role tersebut.',
      shots: [
        { id: 'ak-users', title: 'Pengguna', legend: ['Cari nama, username, atau email', 'Saring status aktif/nonaktif', 'Saring berdasarkan role', 'Daftar pengguna', 'Tambah pengguna', 'Menu aksi pengguna'] },
        { id: 'ak-user-menu', title: 'Menu aksi pengguna', legend: ['Ubah data dan role pengguna', 'Atur ulang password'] },
      ],
      steps: [
        { n: 5, text: <>Klik <ButtonSample icon={<Plus />}>Tambah pengguna</ButtonSample>.</>, permissions: ['users.manage'] },
        { n: 6, text: <>Untuk mengubah role atau menonaktifkan akun, buka menu aksi lalu <b>Edit pengguna</b>.</>, permissions: ['users.manage'] },
      ],
    },
    {
      id: 'tambah-pengguna',
      title: 'Tambah pengguna',
      summary: 'Isi identitas, password awal, dan role.',
      shots: [{ id: 'ak-user-dialog', title: 'Modal Tambah pengguna', legend: ['Nama lengkap', 'Username untuk login', 'Email', 'Password awal (minimal 12 karakter)', 'Role yang dipegang (boleh lebih dari satu)', 'Pengguna aktif', 'Simpan pengguna'] }],
      steps: [
        { n: 1, text: <>Isi nama lengkap, username, email, dan password awal.</>, permissions: ['users.manage'] },
        { n: 5, text: <>Centang role pengguna, pastikan <i>Pengguna aktif</i> tercentang, lalu <ButtonSample>Simpan pengguna</ButtonSample>.</>, permissions: ['users.manage'] },
        { text: <>Berikan username dan password awal ke pengguna. Pengguna bisa mengganti password sendiri di <b>Profil saya</b>.</> },
      ],
      fields: [
        { name: 'Nama lengkap', meaning: 'Nama yang tampil di aktivitas dan dokumen.', example: 'BUDI SANTOSO', required: true },
        { name: 'Username', meaning: 'Dipakai untuk login; unik.', example: 'budi.wonogiri', required: true },
        { name: 'Email', meaning: 'Email pengguna; bisa juga dipakai untuk login.', example: 'budi@contoh.id', required: true },
        { name: 'Password awal', meaning: 'Minimal 12 karakter.', required: true },
        { name: 'Role', meaning: 'Satu atau beberapa role.', example: 'POS Dokumen', required: true },
        { name: 'Pengguna aktif', meaning: 'Akun nonaktif tidak bisa login.', required: false },
      ],
    },
    {
      id: 'reset-password',
      title: 'Atur ulang password',
      summary: 'Bila pengguna lupa password, admin menetapkan password baru.',
      shots: [{ id: 'ak-reset-password', title: 'Modal Reset password', legend: ['Password baru', 'Ulangi password baru', 'Simpan password'] }],
      steps: [
        { text: <>Buka menu aksi pengguna, pilih <b>Atur ulang password</b>.</>, permissions: ['users.manage'] },
        { n: 1, text: <>Isi password baru dua kali, lalu <ButtonSample>Simpan password</ButtonSample>.</>, permissions: ['users.manage'] },
      ],
      doneWhen: 'Pengguna bisa login dan hanya melihat menu serta kabupaten sesuai role-nya.',
    },
  ],
};
