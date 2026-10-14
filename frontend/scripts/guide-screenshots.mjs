// Membuat screenshot tutorial untuk halaman Panduan beserta posisi kotak
// sorot bernomor. Jalankan ulang bila tampilan halaman berubah:
//
//   GUIDE_BASE_URL=http://localhost:8080 GUIDE_USER=admin GUIDE_PASSWORD=... \
//     node scripts/guide-screenshots.mjs
//
// Hasil: ../web/static/images/guide/<id>.jpg dan
// src/features/guide/guideShots.json. Angka panjang (NIK, nomor HP, nomor
// seri) dan nama penerima disamarkan sebelum difoto.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const imageDir = resolve(root, '../web/static/images/guide');
const manifestPath = resolve(root, 'src/features/guide/guideShots.json');
const base = process.env.GUIDE_BASE_URL ?? 'http://localhost:8080';
const user = process.env.GUIDE_USER;
const password = process.env.GUIDE_PASSWORD;
const schedule = new RegExp(process.env.GUIDE_REGENCY ?? 'WONOGIRI', 'i');
if (!user || !password) throw new Error('Set GUIDE_USER dan GUIDE_PASSWORD.');

// Menyamarkan data pribadi: angka 8+ digit diganti titik, dan elemen yang
// ditandai (nama/alamat penerima) diburamkan.
async function mask(page, blurSelectors = []) {
  await page.evaluate((selectors) => {
    const walker = document.createTreeWalker(document.querySelector('main') ?? document.body, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      node.nodeValue = node.nodeValue.replace(/\d{8,}/g, (digits) => digits.slice(0, 4) + '•'.repeat(digits.length - 4));
    }
    for (const input of document.querySelectorAll('main input:not([type=file])')) {
      if (/\d{8,}/.test(input.value)) input.value = input.value.replace(/\d{8,}/g, (digits) => digits.slice(0, 4) + '•'.repeat(digits.length - 4));
    }
    const style = document.createElement('style');
    // Foto (wajah, KTP) diburamkan lebih kuat daripada teks.
    const images = selectors.filter((selector) => / img$/.test(selector));
    const others = selectors.filter((selector) => !/ img$/.test(selector));
    style.textContent = `${others.join(',') || '.__none'} { filter: blur(4px) !important; } ${images.join(',') || '.__none'} { filter: blur(12px) !important; }`;
    document.head.appendChild(style);
  }, blurSelectors);
}

async function choose(page, combobox, option) {
  await combobox.click();
  await page.getByRole('option', { name: option }).first().click();
}

const shots = [
  {
    id: 'persiapan-jadwal',
    maxHeight: 560,
    path: 'persiapan-program',
    async setup(page) { await page.getByRole('tab', { name: /Jadwal/ }).click(); },
    marks: (page) => [
      [1, page.getByRole('tablist').first()],
      [2, page.getByRole('main').getByRole('table').first()],
      [3, page.getByRole('button', { name: 'Tambah jadwal' })],
    ],
  },
  {
    id: 'dcp3-jadwal',
    path: 'dcp3',
    marks: (page) => [
      [1, page.getByLabel('Tahapan import DCP3')],
      [2, page.getByRole('combobox').first()],
      [3, page.getByRole('button', { name: 'Lanjut ke upload' })],
    ],
  },
  {
    id: 'data-penerima',
    maxHeight: 720,
    path: '',
    blur: ['main table tbody td'],
    async setup(page) {
      const close = page.getByRole('button', { name: 'Tutup panduan awal' });
      if (await close.count()) await close.click();
    },
    marks: (page) => [
      [1, page.getByRole('combobox', { name: /program/i }).first().locator('xpath=ancestor::section[1]')],
      [2, page.getByRole('main').getByRole('table').first()],
      [3, page.getByRole('button', { name: 'Tambah penerima' })],
    ],
  },
  {
    id: 'distribusi-pos',
    path: 'dokumentasi/pendistribusian',
    blur: ['main input[value]:not([type=date])', 'main [data-guide-blur]', 'main img', 'main button[aria-label^="Lihat "]'],
    async setup(page) {
      await choose(page, page.getByRole('combobox').first(), schedule);
      await page.getByRole('button', { name: /^Nomor \d+ - Proses$/ }).first().click();
      await page.waitForTimeout(2500);
      // Judul POS Dokumen berisi nama penerima.
      await page.evaluate(() => {
        for (const badge of document.querySelectorAll('main *')) {
          if (badge.children.length === 0 && badge.textContent.trim() === 'POS Dokumen') {
            const section = badge.closest('section') ?? badge.parentElement?.parentElement;
            section?.querySelector('h2, h3, h4')?.setAttribute('data-guide-blur', '');
          }
        }
      });
    },
    marks: (page) => [
      [1, page.getByLabel('Pilih jadwal distribusi')],
      [2, page.getByRole('complementary', { name: 'Katalog nomor bagi' })],
      [3, page.getByText('POS Mesin', { exact: true }).first().locator('xpath=ancestor::section[1]')],
      [4, page.getByText('POS Dokumen', { exact: true }).first().locator('xpath=ancestor::*[2]')],
    ],
  },
  {
    id: 'kegiatan',
    path: 'dokumentasi/ceremony-sosialisasi',
    async setup(page) {
      await choose(page, page.getByRole('combobox').first(), /KONVERTER/i);
      await choose(page, page.getByRole('combobox').nth(1), schedule);
      await page.waitForTimeout(1500);
    },
    marks: (page) => [
      [1, page.getByRole('combobox').first()],
      [1, page.getByRole('combobox').nth(1)],
      [2, page.getByLabel('Ambil Foto').first().locator('xpath=ancestor::*[self::div or self::section][2]')],
    ],
  },
  {
    id: 'berita-acara',
    path: 'berita-acara?tab=closing-titik-serah',
    async setup(page) {
      await choose(page, page.getByRole('combobox').first(), schedule);
      await page.getByRole('tab', { name: 'Closing Titik Serah' }).click();
      await page.waitForTimeout(3000);
    },
    marks: (page) => [
      [1, page.getByRole('combobox').first()],
      [2, page.getByRole('tablist', { name: 'Jenis berita acara' })],
      [3, page.getByLabel('Tanggal closing')],
      [4, page.getByRole('button', { name: 'Penandatangan', exact: true })],
      [5, page.getByRole('button', { name: /Finalisasi & sinkronkan/ })],
    ],
  },
  {
    id: 'penandatangan',
    path: 'berita-acara?tab=closing-titik-serah',
    async setup(page) {
      await choose(page, page.getByRole('combobox').first(), schedule);
      await page.getByRole('tab', { name: 'Closing Titik Serah' }).click();
      await page.getByRole('button', { name: 'Penandatangan', exact: true }).click();
      await page.getByRole('group', { name: /^Kolom / }).first().waitFor({ timeout: 60000 });
      await page.waitForTimeout(2500);
      // Nama dan NIP pada tabel adalah gambar (render PDF): buramkan barisnya.
      await page.evaluate(() => {
        for (const column of document.querySelectorAll('[role="group"][aria-label^="Kolom "]')) {
          const rect = column.getBoundingClientRect();
          const cover = document.createElement('div');
          Object.assign(cover.style, {
            position: 'absolute', left: `${rect.left + window.scrollX}px`, top: `${rect.bottom + window.scrollY}px`,
            width: `${rect.width}px`, height: `${rect.height * 0.55}px`, backdropFilter: 'blur(5px)', zIndex: 50,
          });
          document.body.appendChild(cover);
        }
      });
    },
    blur: ['main section[aria-label] input:not([type=range]):not([type=number]):not([type=checkbox])'],
    marks: (page) => [
      [1, page.getByRole('tablist', { name: 'Pihak penandatangan' })],
      [2, page.getByRole('group', { name: /^Kolom / }).first().locator('xpath=ancestor::div[contains(@class,"overflow-auto")][1]')],
      [3, page.getByRole('main').getByRole('region', { name: /Dinas|Pelaksana|Konsultan|Pertamina/ }).first()],
      [4, page.getByRole('button', { name: 'Simpan', exact: true })],
    ],
  },
  {
    id: 'laporan',
    maxHeight: 720,
    path: 'laporan',
    blur: ['main table tbody td'],
    async setup(page) {
      await choose(page, page.getByRole('combobox').first(), schedule);
      await page.waitForTimeout(2500);
    },
    marks: (page) => [
      [1, page.getByLabel('Filter laporan')],
      [2, page.getByLabel('Ekspor laporan')],
      [3, page.getByRole('main').getByRole('table').first()],
    ],
  },
  {
    id: 'role',
    path: 'role',
    async setup(page) {
      // Layar tinggi agar dialog tampil utuh tanpa scroll.
      await page.setViewportSize({ width: 1440, height: 2600 });
      await page.getByRole('button', { name: 'Tambah role' }).click();
      await page.getByRole('dialog').waitFor();
      // Contoh tim pusat: semua kabupaten (daftar kabupaten disembunyikan).
      await page.getByRole('dialog').getByText('Akses semua kabupaten').click();
    },
    dialog: true,
    marks: (page) => [
      [1, page.getByRole('dialog').getByText('Akses kabupaten').locator('xpath=ancestor::fieldset[1]')],
      [2, page.getByRole('dialog').getByRole('button', { name: 'Simpan role' })],
    ],
  },
];

// ---- Tutorial Persiapan program ----
async function openSetupTab(page, tab) {
  await page.getByRole('tab', { name: new RegExp(`^${tab}`) }).click();
  await page.waitForTimeout(1200);
}
async function openDialog(page, tab, button) {
  await openSetupTab(page, tab);
  await page.getByRole('button', { name: button }).first().click();
  await page.getByRole('dialog').waitFor();
  await page.waitForTimeout(800);
}
const dialog = (page) => page.getByRole('dialog');
const formSection = (page, title) => dialog(page).locator('section.setupFormSection').filter({ has: page.getByRole('heading', { name: title, exact: true }) });

shots.push(
  {
    id: 'pp-tabs',
    path: 'persiapan-program',
    clip: (page) => page.getByRole('tablist').first(),
    marks: (page) => ['Kabupaten', 'Program', 'Zona', 'Template', 'Logo Dokumen', 'Jadwal'].map((tab, index) => [index + 1, page.getByRole('tab', { name: new RegExp(`^${tab}`) })]),
  },
  {
    id: 'pp-kabupaten',
    path: 'persiapan-program',
    maxHeight: 520,
    async setup(page) { await openSetupTab(page, 'Kabupaten'); },
    marks: (page) => [
      [1, page.getByRole('main').getByPlaceholder(/Cari/).first()],
      [2, page.getByRole('main').getByRole('combobox').first()],
      [3, page.getByRole('main').getByRole('table').first()],
      [4, page.getByRole('button', { name: 'Tambah kabupaten' })],
      [5, page.getByRole('button', { name: /^Edit KAB\./ }).first()],
    ],
  },
  {
    id: 'pp-kabupaten-modal',
    path: 'persiapan-program',
    async setup(page) { await openDialog(page, 'Kabupaten', 'Tambah kabupaten'); },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Provinsi', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Kabupaten', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('Kode dokumen', { exact: true }).locator('xpath=..')],
      [4, dialog(page).getByText('Kabupaten aktif')],
      [5, dialog(page).getByRole('button', { name: 'Simpan' })],
    ],
  },
  {
    id: 'pp-program',
    path: 'persiapan-program',
    maxHeight: 420,
    async setup(page) { await openSetupTab(page, 'Program'); },
    marks: (page) => [
      [1, page.getByRole('main').getByRole('table').first()],
      [2, page.getByRole('button', { name: 'Tambah program' })],
      [3, page.getByRole('button', { name: /^Edit / }).first()],
    ],
  },
  {
    id: 'pp-program-modal',
    path: 'persiapan-program',
    async setup(page) { await openDialog(page, 'Program', /^Edit /); },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Kode program', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Nama program', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('Jenis penerima', { exact: true }).locator('xpath=..')],
      [4, dialog(page).getByText('Tahun anggaran', { exact: true }).locator('xpath=..')],
      [5, dialog(page).getByText('Status', { exact: true }).locator('xpath=..')],
      [6, dialog(page).getByRole('button', { name: 'Simpan' })],
    ],
  },
  {
    id: 'pp-zona',
    path: 'persiapan-program',
    maxHeight: 900,
    async setup(page) { await openSetupTab(page, 'Zona'); },
    marks: (page) => [
      [1, page.getByRole('main').getByRole('combobox').first()],
      [2, page.getByRole('button', { name: 'Tambah zona' })],
      [3, page.getByLabel('Papan zona')],
      [4, page.getByLabel('No. PO per zona')],
    ],
  },
  {
    id: 'pp-template',
    path: 'persiapan-program',
    maxHeight: 900,
    async setup(page) { await openSetupTab(page, 'Template'); },
    marks: (page) => [
      [1, page.getByRole('table', { name: 'Daftar template paket' })],
      [2, page.getByRole('button', { name: 'Tambah paket' })],
      [3, page.getByRole('table', { name: 'Daftar template dokumentasi' })],
      [4, page.getByRole('button', { name: 'Tambah dokumentasi' })],
    ],
  },
  {
    id: 'pp-paket-identitas',
    path: 'persiapan-program',
    viewportHeight: 2600,
    async setup(page) { await openDialog(page, 'Template', /^Edit PAKET/); },
    clip: (page) => formSection(page, 'Identitas template'),
    marks: (page) => [
      [1, formSection(page, 'Identitas template').getByText('Kode template', { exact: true }).locator('xpath=..')],
      [2, formSection(page, 'Identitas template').getByText('Nama template', { exact: true }).locator('xpath=..')],
      [3, formSection(page, 'Identitas template').getByText('Jenis program', { exact: true }).locator('xpath=..')],
      [4, formSection(page, 'Identitas template').getByText('Status', { exact: true }).locator('xpath=..')],
    ],
  },
  {
    id: 'pp-paket-mesin',
    path: 'persiapan-program',
    viewportHeight: 2600,
    async setup(page) { await openDialog(page, 'Template', /^Edit PAKET/); },
    clip: (page) => formSection(page, 'Opsi mesin'),
    marks: (page) => [
      [1, formSection(page, 'Opsi mesin').getByLabel('Merk mesin 1')],
      [2, formSection(page, 'Opsi mesin').getByLabel('Tipe mesin 1')],
      [3, formSection(page, 'Opsi mesin').getByLabel('Daya mesin 1')],
      [4, formSection(page, 'Opsi mesin').getByLabel('Jenis BBM 1')],
      [5, formSection(page, 'Opsi mesin').getByLabel('TKDN mesin 1 (%)')],
      [6, formSection(page, 'Opsi mesin').getByRole('button', { name: 'Tambah opsi mesin' })],
    ],
  },
  {
    id: 'pp-paket-selang',
    path: 'persiapan-program',
    viewportHeight: 2600,
    async setup(page) { await openDialog(page, 'Template', /^Edit PAKET/); },
    clip: (page) => formSection(page, 'Opsi selang'),
    marks: (page) => [
      [1, dialog(page).getByLabel('Merk selang hisap 1')],
      [2, dialog(page).getByLabel('Spesifikasi selang hisap 1')],
      [3, dialog(page).getByLabel('Merk selang buang 1')],
      [4, dialog(page).getByLabel('Spesifikasi selang buang 1')],
      [5, dialog(page).getByLabel('TKDN selang hisap 1 (%)')],
    ],
  },
  {
    id: 'pp-paket-konkit',
    path: 'persiapan-program',
    viewportHeight: 2600,
    async setup(page) { await openDialog(page, 'Template', /^Edit PAKET/); },
    clip: (page) => formSection(page, 'Opsi konkit/reducer'),
    marks: (page) => [
      [1, dialog(page).getByLabel('Merk konkit 1')],
      [2, dialog(page).getByLabel('TKDN konkit 1 (%)')],
      [3, dialog(page).getByRole('button', { name: 'Tambah opsi konkit' })],
    ],
  },
  {
    id: 'pp-paket-komponen',
    path: 'persiapan-program',
    viewportHeight: 2600,
    maxHeight: 420,
    async setup(page) { await openDialog(page, 'Template', /^Edit PAKET/); },
    clip: (page) => formSection(page, 'Komponen paket'),
    marks: (page) => [
      [1, dialog(page).getByLabel('Nama komponen 1', { exact: true })],
      [2, dialog(page).getByLabel('Jumlah komponen 1', { exact: true })],
      [3, dialog(page).getByLabel('Merk komponen 1', { exact: true })],
      [4, dialog(page).getByLabel('TKDN komponen 1 (%)')],
      [5, dialog(page).getByText('Sembunyikan dari checklist BA serah terima').first()],
    ],
  },
  {
    id: 'pp-dokumentasi-slot',
    path: 'persiapan-program',
    viewportHeight: 2600,
    maxHeight: 460,
    async setup(page) { await openDialog(page, 'Template', /^Edit Dokumentasi Distribusi Petani/); },
    clip: (page) => formSection(page, 'Slot dokumentasi'),
    marks: (page) => {
      const slot = formSection(page, 'Slot dokumentasi');
      return [
        [1, slot.getByText('Judul', { exact: true }).first().locator('xpath=..')],
        [2, slot.getByText('Jenis media', { exact: true }).first().locator('xpath=..')],
        [3, slot.getByText('Sumber', { exact: true }).first().locator('xpath=..')],
        [4, slot.getByText('Pos', { exact: true }).first().locator('xpath=..')],
        [5, slot.getByText('Minimal', { exact: true }).first().locator('xpath=..')],
        [6, slot.getByText('Wajib', { exact: true }).first()],
        [7, slot.getByRole('button', { name: 'Tambah slot' })],
      ];
    },
  },
  {
    id: 'pp-logo',
    path: 'persiapan-program',
    maxHeight: 700,
    async setup(page) { await openSetupTab(page, 'Logo'); },
    marks: (page) => [
      [1, page.getByRole('main').getByRole('combobox').first()],
      [2, page.getByRole('main').getByText('Unggah logo').first()],
      [3, page.getByRole('main').getByText(/^logo-.*\.png$/).first().locator('xpath=ancestor::li[1] | ancestor::article[1]').first()],
    ],
  },
  {
    id: 'pp-jadwal',
    path: 'persiapan-program',
    maxHeight: 520,
    async setup(page) { await openSetupTab(page, 'Jadwal'); },
    marks: (page) => [
      [1, page.getByRole('main').getByPlaceholder(/Cari/).first()],
      [2, page.getByRole('main').getByRole('table').first()],
      [3, page.getByRole('button', { name: 'Tambah jadwal' })],
      [4, page.getByRole('button', { name: /^Edit KAB\./ }).first()],
    ],
  },
  {
    id: 'pp-jadwal-modal',
    path: 'persiapan-program',
    viewportHeight: 2000,
    maxHeight: 1300,
    async setup(page) { await openDialog(page, 'Jadwal', /^Edit KAB\. WONOGIRI/); },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Nama jadwal', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Program', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('Kabupaten', { exact: true }).locator('xpath=..')],
      [4, dialog(page).getByText('Template paket', { exact: true }).locator('xpath=..')],
      [5, dialog(page).getByText('Template dokumentasi', { exact: true }).locator('xpath=..')],
      [6, dialog(page).getByText('Status', { exact: true }).locator('xpath=..')],
      [7, dialog(page).getByText('Mulai', { exact: true }).locator('xpath=..')],
      [8, dialog(page).getByText('Selesai', { exact: true }).locator('xpath=..')],
      [9, dialog(page).getByText('Kuota slot (kosongkan jika tanpa batas)').locator('xpath=..')],
      [10, dialog(page).getByText('Konsultan pengawas', { exact: true }).locator('xpath=..')],
      [11, dialog(page).getByRole('button', { name: 'Simpan' })],
    ],
  },
);

// ---- Tutorial Pengguna & hak akses ----
async function openMenu(page, button) {
  await page.getByRole('button', { name: button }).click();
  await page.getByRole('menuitem').first().waitFor();
  await page.waitForTimeout(400);
}
shots.push(
  {
    id: 'ak-role-list',
    path: 'role',
    maxHeight: 620,
    marks: (page) => [
      [1, page.getByRole('main').getByRole('table').first()],
      [2, page.getByRole('button', { name: 'Tambah role' })],
      [3, page.getByRole('button', { name: 'Aksi POS Mesin' })],
    ],
  },
  {
    id: 'ak-role-menu',
    path: 'role',
    dialog: true,
    async setup(page) { await openMenu(page, 'Aksi POS Mesin'); },
    marks: (page) => [
      [1, page.getByRole('menuitem', { name: /^Edit / })],
      [2, page.getByRole('menuitem', { name: /^Hapus / })],
    ],
  },
  {
    id: 'ak-role-dialog',
    path: 'role',
    viewportHeight: 3200,
    maxHeight: 820,
    async setup(page) {
      await openMenu(page, 'Aksi POS Mesin');
      await page.getByRole('menuitem', { name: /^Edit / }).click();
      await dialog(page).waitFor();
      await page.waitForTimeout(600);
    },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Kode role', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Nama role', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('Deskripsi', { exact: true }).locator('xpath=..')],
      [4, dialog(page).locator('fieldset').first()],
    ],
  },
  {
    id: 'ak-role-regency',
    path: 'role',
    viewportHeight: 3200,
    maxHeight: 420,
    async setup(page) {
      await openMenu(page, 'Aksi POS Mesin');
      await page.getByRole('menuitem', { name: /^Edit / }).click();
      await dialog(page).waitFor();
      await page.waitForTimeout(600);
    },
    clip: (page) => dialog(page).getByText('Akses kabupaten', { exact: true }).locator('xpath=ancestor::fieldset[1]'),
    marks: (page) => [
      [1, dialog(page).getByText('Akses semua kabupaten')],
      [2, dialog(page).getByText('Akses kabupaten', { exact: true }).locator('xpath=ancestor::fieldset[1]').locator('label').nth(1)],
    ],
  },
  {
    id: 'ak-users',
    path: 'pengguna',
    maxHeight: 620,
    marks: (page) => [
      [1, page.getByLabel('Cari pengguna')],
      [2, page.getByLabel('Filter status')],
      [3, page.getByLabel('Filter role')],
      [4, page.getByRole('main').getByRole('table').first()],
      [5, page.getByRole('button', { name: 'Tambah pengguna' })],
      [6, page.getByRole('button', { name: /^Aksi / }).first()],
    ],
  },
  {
    id: 'ak-user-menu',
    path: 'pengguna',
    dialog: true,
    async setup(page) { await openMenu(page, 'Aksi UJI POS MESIN'); },
    marks: (page) => [
      [1, page.getByRole('menuitem', { name: 'Edit pengguna' })],
      [2, page.getByRole('menuitem', { name: 'Atur ulang password' })],
    ],
  },
  {
    id: 'ak-user-dialog',
    path: 'pengguna',
    viewportHeight: 1800,
    async setup(page) { await page.getByRole('button', { name: 'Tambah pengguna' }).click(); await dialog(page).waitFor(); await page.waitForTimeout(600); },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Nama lengkap', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Username', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('Email', { exact: true }).locator('xpath=..')],
      [4, dialog(page).getByText('Password awal', { exact: true }).locator('xpath=..')],
      [5, dialog(page).locator('fieldset').first()],
      [6, dialog(page).getByText('Pengguna aktif')],
      [7, dialog(page).getByRole('button', { name: 'Simpan pengguna' })],
    ],
  },
  {
    id: 'ak-reset-password',
    path: 'pengguna',
    async setup(page) {
      await openMenu(page, 'Aksi UJI POS MESIN');
      await page.getByRole('menuitem', { name: 'Atur ulang password' }).click();
      await dialog(page).waitFor();
      await page.waitForTimeout(600);
    },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Password baru', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Konfirmasi password', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByRole('button', { name: 'Simpan password' })],
    ],
  },
);

// ---- Tutorial DCP3 ----
// Server menolak file yang sama dua kali pada satu jadwal, jadi setiap run
// mengunggah salinan contoh (data fiktif) dengan komentar zip berbeda: isi
// workbook sama, checksum berbeda. Ini hanya membuat pratinjau draf, tidak
// meng-import data.
async function uniqueWorkbook() {
  const source = await readFile(resolve(root, 'scripts/fixtures/dcp3-contoh.xlsx'));
  const end = source.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  const comment = Buffer.from(`panduan ${Date.now()} ${Math.random()}`);
  const header = Buffer.from(source.subarray(0, end + 22));
  header.writeUInt16LE(comment.length, end + 20);
  const file = resolve(tmpdir(), `dcp3-contoh-${Date.now()}.xlsx`);
  await writeFile(file, Buffer.concat([header, comment]));
  return file;
}
async function dcp3ToStep(page, step) {
  await choose(page, page.getByRole('combobox').first(), schedule);
  if (step < 2) return;
  await page.getByRole('button', { name: 'Lanjut ke upload' }).click();
  await page.getByLabel('Pilih file DCP3').setInputFiles(await uniqueWorkbook());
  await page.waitForTimeout(1200);
  if (step < 3) return;
  await page.getByRole('button', { name: 'Unggah dan baca file' }).click();
  await page.getByRole('heading', { name: 'Cocokkan kolom Excel' }).waitFor({ timeout: 30000 });
  const columns = ['Nama', 'NIK', 'No Kartu Petani', 'Alamat', 'Desa', 'Kecamatan', 'No HP'];
  for (let index = 0; index < columns.length; index++) {
    await page.getByRole('main').getByRole('combobox').nth(index).click();
    await page.getByRole('option', { name: columns[index], exact: true }).click();
  }
  if (step < 4) return;
  await page.getByRole('button', { name: 'Periksa data' }).click();
  await page.waitForTimeout(1500);
}
shots.push(
  {
    id: 'dcp3-1-jadwal',
    path: 'dcp3',
    async setup(page) { await dcp3ToStep(page, 1); },
    marks: (page) => [
      [1, page.getByLabel('Tahapan import DCP3')],
      [2, page.getByRole('main').getByRole('combobox').first()],
      [3, page.getByRole('button', { name: 'Lanjut ke upload' })],
    ],
  },
  {
    id: 'dcp3-2-workbook',
    path: 'dcp3',
    async setup(page) { await dcp3ToStep(page, 2); },
    marks: (page) => [
      [1, page.getByLabel('Pilih file DCP3').locator('xpath=ancestor::label[1] | ..').first()],
      [2, page.getByRole('button', { name: 'Unggah dan baca file' })],
    ],
  },
  {
    id: 'dcp3-3-pemetaan',
    path: 'dcp3',
    async setup(page) { await dcp3ToStep(page, 3); },
    marks: (page) => [
      [1, page.getByText('Nama lengkap*').locator('xpath=..')],
      [2, page.getByRole('main').locator('label', { hasText: /^NIK$/ }).locator('xpath=..')],
      [3, page.getByText('Nomor kartu petani', { exact: true }).locator('xpath=..')],
      [4, page.getByRole('button', { name: 'Periksa data' })],
    ],
  },
  {
    id: 'dcp3-4-tinjau',
    path: 'dcp3',
    maxHeight: 900,
    async setup(page) { await dcp3ToStep(page, 4); },
    marks: (page) => [
      [1, page.getByLabel('Ringkasan pratinjau')],
      [2, page.getByRole('main').getByRole('table').first()],
      [3, page.getByRole('button', { name: /^Import \d+ data$/ })],
    ],
  },
);

// ---- Tutorial Data Penerima ----
async function closeStartCard(page) {
  const close = page.getByRole('button', { name: 'Tutup panduan awal' });
  if (await close.count()) await close.click();
}
const filterCard = (page) => page.getByLabel('Cari penerima').locator('xpath=ancestor::section[1]');
shots.push(
  {
    id: 'pn-filter',
    path: '',
    async setup(page) { await closeStartCard(page); },
    clip: filterCard,
    marks: (page) => [
      [1, page.getByLabel('Cari penerima')],
      [2, page.getByRole('combobox', { name: /Program/ }).first()],
      [3, page.getByRole('combobox', { name: /Kabupaten/ }).first()],
      [4, page.getByRole('combobox', { name: /Jadwal/ }).first()],
      [5, page.getByRole('combobox', { name: /Dokumentasi/ }).first()],
      [6, page.getByRole('button', { name: 'Reset filter' })],
    ],
  },
  {
    id: 'pn-stats',
    path: '',
    async setup(page) { await closeStartCard(page); },
    clip: (page) => page.getByLabel('Statistik penerima'),
    marks: (page) => [
      [1, page.getByLabel('Statistik penerima').locator(':scope > *').nth(0)],
      [2, page.getByLabel('Statistik penerima').locator(':scope > *').nth(1)],
      [3, page.getByLabel('Statistik penerima').locator(':scope > *').nth(2)],
      [4, page.getByLabel('Statistik penerima').locator(':scope > *').nth(3)],
      [5, page.getByLabel('Statistik penerima').locator(':scope > *').nth(4)],
    ],
  },
  {
    id: 'pn-table',
    path: '',
    viewportWidth: 1920,
    maxHeight: 520,
    blur: ['main table tbody td:not(:last-child)'],
    async setup(page) { await closeStartCard(page); },
    clip: (page) => page.getByRole('main').getByRole('table').first(),
    marks: (page) => {
      const header = page.getByRole('main').getByRole('table').first().locator('thead th');
      return [
        [1, header.nth(0)], [2, header.nth(1)], [3, header.nth(2)], [4, header.nth(3)], [5, header.nth(4)], [6, header.nth(5)],
        [7, page.getByRole('button', { name: 'Edit', exact: true }).first().locator('xpath=ancestor::td[1]')],
      ];
    },
  },
  {
    id: 'pn-dialog',
    path: '',
    async setup(page) { await page.getByRole('button', { name: 'Tambah penerima' }).click(); await dialog(page).waitFor(); await page.waitForTimeout(500); },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Jadwal', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByText('Nama lengkap', { exact: true }).locator('xpath=..')],
      [3, dialog(page).getByText('NIK', { exact: true }).locator('xpath=..')],
      [4, dialog(page).getByText('Nomor kartu petani', { exact: true }).locator('xpath=..')],
      [5, dialog(page).getByRole('button', { name: 'Simpan' })],
    ],
  },
  {
    id: 'pn-cancel',
    path: '',
    blur: ['[role=alertdialog] h2'],
    async setup(page) { await page.getByRole('button', { name: 'Batalkan', exact: true }).first().click(); await page.getByRole('alertdialog').waitFor(); await page.waitForTimeout(500); },
    clip: (page) => page.getByRole('alertdialog'),
    marks: (page) => [
      [1, page.getByRole('alertdialog').locator('p').first()],
      [2, page.getByRole('alertdialog').getByRole('button', { name: 'Batalkan penerima' })],
    ],
  },
);

// ---- Tutorial Pendistribusian ----
// Foto bukti (KTP, KK, kartu tani), tanda tangan, nama file foto (berisi nama
// penerima), isian identitas, dan judul POS Dokumen (nama penerima) diburamkan.
const distributionBlur = ['main img', 'main button[aria-label^="Lihat "]', 'main [data-guide-blur]', 'main input[value]:not([type=date]):not([type=file])'];
async function openSlot(page, slot) {
  await choose(page, page.getByRole('combobox').first(), schedule);
  await page.waitForTimeout(1200);
  if (!slot) return;
  await page.getByRole('button', { name: slot, exact: true }).click();
  await page.waitForTimeout(2500);
  await page.evaluate(() => {
    for (const badge of document.querySelectorAll('main *')) {
      if (badge.children.length === 0 && badge.textContent.trim() === 'POS Dokumen') {
        badge.closest('section')?.querySelector('h3')?.setAttribute('data-guide-blur', '');
      }
    }
  });
}
const posSection = (page, label) => page.getByText(label, { exact: true }).first().locator('xpath=ancestor::section[1]');
const subSection = (page, pos, title) => posSection(page, pos).locator('section, [class*="rounded-lg"]').filter({ has: page.getByRole('heading', { name: title, exact: true }) }).first();
shots.push(
  {
    id: 'ds-katalog',
    path: 'dokumentasi/pendistribusian',
    maxHeight: 760,
    blur: distributionBlur,
    async setup(page) { await openSlot(page); },
    marks: (page) => [
      [1, page.getByLabel('Pilih jadwal distribusi')],
      [2, page.getByRole('complementary', { name: 'Katalog nomor bagi' }).getByText(/^\d+ \/ \d+$/)],
      [3, page.getByLabel('Keterangan status')],
      [4, page.getByRole('button', { name: /^Nomor 1 - / })],
      [5, page.getByRole('button', { name: /^Buat Nomor / }).first()],
      [6, page.getByText('Pilih nomor bagi untuk mulai bekerja')],
    ],
  },
  {
    id: 'ds-buat',
    path: 'dokumentasi/pendistribusian',
    maxHeight: 640,
    blur: distributionBlur,
    async setup(page) {
      await openSlot(page);
      await page.getByRole('button', { name: /^Buat Nomor / }).first().click();
      await page.waitForTimeout(1200);
    },
    marks: (page) => [
      [1, page.getByRole('button', { name: /^Buat Nomor / }).first()],
      [2, posSection(page, 'POS Mesin')],
      [3, page.getByRole('button', { name: 'Mulai dokumentasi' })],
    ],
  },
  {
    id: 'ds-mesin',
    path: 'dokumentasi/pendistribusian',
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 15 - Terbuka'); },
    clip: (page) => posSection(page, 'POS Mesin'),
    marks: (page) => [
      [1, posSection(page, 'POS Mesin').getByLabel('Serial Number Mesin', { exact: true })],
      [2, posSection(page, 'POS Mesin').getByRole('button', { name: 'Scan Serial Number Mesin' })],
      [3, posSection(page, 'POS Mesin').getByRole('button', { name: 'Simpan nomor seri' })],
      [4, posSection(page, 'POS Mesin').getByText('Buka kamera').first()],
      [5, posSection(page, 'POS Mesin').getByText('Pilih galeri').first()],
    ],
  },
  {
    id: 'ds-dokumen-hubungkan',
    path: 'dokumentasi/pendistribusian',
    maxHeight: 900,
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 15 - Terbuka'); },
    clip: (page) => posSection(page, 'POS Dokumen'),
    marks: (page) => [
      [1, posSection(page, 'POS Dokumen').getByLabel('Tanggal distribusi')],
      [2, posSection(page, 'POS Dokumen').getByRole('button', { name: 'Simpan tanggal distribusi' })],
      [3, posSection(page, 'POS Dokumen').getByLabel('NIK Penerima')],
      [4, posSection(page, 'POS Dokumen').getByText('Data peralatan', { exact: true }).locator('xpath=..')],
    ],
  },
  {
    id: 'ds-dokumen-penerima',
    path: 'dokumentasi/pendistribusian',
    maxHeight: 900,
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 14 - Proses'); },
    clip: (page) => posSection(page, 'POS Dokumen').getByRole('heading', { name: 'Penerima', exact: true }).locator('xpath=..'),
    marks: (page) => [
      [1, posSection(page, 'POS Dokumen').getByText('NIK terpasang', { exact: true }).locator('xpath=..')],
      [2, posSection(page, 'POS Dokumen').getByText('Nomor telepon', { exact: true }).locator('xpath=..')],
      [3, posSection(page, 'POS Dokumen').getByRole('button', { name: 'Simpan data penerima' })],
      [4, posSection(page, 'POS Dokumen').getByRole('button', { name: 'Ganti penerima' })],
    ],
  },
  {
    id: 'ds-dokumen-ttd',
    path: 'dokumentasi/pendistribusian',
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 14 - Proses'); },
    clip: (page) => posSection(page, 'POS Dokumen').getByRole('heading', { name: 'Tanda tangan penerima', exact: true }).locator('xpath=ancestor::section[1]'),
    marks: (page) => [
      [1, posSection(page, 'POS Dokumen').getByText(/^Opsional\./)],
      [2, posSection(page, 'POS Dokumen').getByRole('button', { name: 'Tanda tangan ulang' })],
    ],
  },
  {
    id: 'ds-dokumen-peralatan',
    path: 'dokumentasi/pendistribusian',
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 14 - Proses'); },
    clip: (page) => posSection(page, 'POS Dokumen').getByRole('heading', { name: 'Data peralatan', exact: true }).locator('xpath=ancestor::section[1]'),
    marks: (page) => [
      [1, posSection(page, 'POS Dokumen').getByLabel('Merk/Tipe Mesin', { exact: true })],
      [2, posSection(page, 'POS Dokumen').getByLabel('Merk Konkit/Reducer', { exact: true })],
      [3, posSection(page, 'POS Dokumen').getByLabel('Merk/Spesifikasi Selang', { exact: true })],
      [4, posSection(page, 'POS Dokumen').getByRole('button', { name: 'Simpan data peralatan' })],
    ],
  },
  {
    id: 'ds-penyerahan-terkunci',
    path: 'dokumentasi/pendistribusian',
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 15 - Terbuka'); },
    clip: (page) => page.getByRole('heading', { name: 'Menunggu dokumen selesai' }).locator('xpath=ancestor::section[1]'),
    marks: (page) => [[1, page.getByRole('heading', { name: 'Menunggu dokumen selesai' })]],
  },
  {
    id: 'ds-selesai',
    path: 'dokumentasi/pendistribusian',
    maxHeight: 700,
    blur: distributionBlur,
    async setup(page) { await openSlot(page, 'Nomor 1 - Selesai'); },
    clip: (page) => posSection(page, 'POS Penyerahan'),
    marks: (page) => [
      [1, page.getByText('Distribusi selesai')],
      [2, page.getByRole('button', { name: 'Buka revisi POS Penyerahan' })],
    ],
  },
  {
    id: 'ds-revisi',
    path: 'dokumentasi/pendistribusian',
    blur: distributionBlur,
    async setup(page) {
      await openSlot(page, 'Nomor 1 - Selesai');
      await page.getByRole('button', { name: 'Buka revisi POS Dokumen' }).click();
      await dialog(page).waitFor();
      await page.waitForTimeout(500);
    },
    clip: dialog,
    marks: (page) => [
      [1, dialog(page).getByText('Alasan revisi', { exact: true }).locator('xpath=..')],
      [2, dialog(page).getByRole('button', { name: 'Buka revisi' })],
    ],
  },
);

// ---- Tutorial Dokumentasi kegiatan ----
// Contoh galeri memakai foto dummy (gambar publik website) yang diunggah ke
// Ceremony & Sosialisasi kabupaten contoh.
async function openActivity(page) {
  await choose(page, page.getByRole('combobox').first(), /KONVERTER/i);
  await choose(page, page.getByRole('combobox').nth(1), schedule);
  await page.waitForTimeout(2500);
}
shots.push(
  {
    id: 'kg-unggah',
    path: 'dokumentasi/ceremony-sosialisasi',
    async setup(page) { await openActivity(page); },
    marks: (page) => [
      [1, page.getByRole('combobox').first()],
      [2, page.getByRole('combobox').nth(1)],
      [3, page.getByLabel('Ambil Foto').locator('xpath=..')],
      [4, page.getByLabel('Rekam Video').locator('xpath=..')],
      [5, page.getByLabel('Pilih dari Galeri').locator('xpath=..')],
    ],
  },
  {
    id: 'kg-galeri',
    path: 'dokumentasi/ceremony-sosialisasi',
    maxHeight: 760,
    async setup(page) { await openActivity(page); },
    marks: (page) => [
      [1, page.getByText(/^\d+ dokumentasi$/)],
      [2, page.getByRole('button', { name: /^Lihat / }).first()],
      [3, page.getByRole('button', { name: /^Hapus / }).first()],
    ],
  },
  {
    id: 'kg-pratinjau',
    path: 'dokumentasi/ceremony-sosialisasi',
    dialog: true,
    async setup(page) {
      await openActivity(page);
      await page.getByRole('button', { name: /^Lihat / }).first().click();
      await dialog(page).waitFor();
      await page.waitForTimeout(1500);
    },
    marks: (page) => [
      [1, dialog(page).locator('img').first()],
      [2, dialog(page).getByRole('button', { name: /Perbesar|Zoom|Perkecil/ }).first()],
      [3, dialog(page).getByRole('button', { name: /Tutup|Close/ }).first()],
    ],
  },
  {
    id: 'kg-unloading',
    path: 'dokumentasi/unloading',
    marks: (page) => [
      [1, page.getByRole('tablist').first()],
      [2, page.getByRole('combobox').first()],
      [3, page.getByRole('combobox').nth(1)],
    ],
  },
);

// ---- Tutorial Berita Acara ----
// Preview PDF memuat nama/NIK penerima, jadi diburamkan.
const baBlur = ['main iframe', 'main img', 'main table tbody td'];
async function openBA(page, tab) {
  await choose(page, page.getByRole('combobox').first(), schedule);
  await page.waitForTimeout(1200);
  if (tab) await page.getByRole('tab', { name: tab, exact: true }).click();
  await page.waitForTimeout(3000);
}
const wetBar = (page) => page.getByText('Tanda tangan basah', { exact: true }).locator('xpath=ancestor::div[contains(@class,"rounded-lg")][1]');
shots.push(
  {
    id: 'ba-pilih',
    path: 'berita-acara',
    blur: baBlur,
    async setup(page) { await openBA(page); },
    marks: (page) => [
      [1, page.getByRole('combobox').first()],
      [2, page.getByRole('tablist', { name: 'Jenis berita acara' })],
    ],
  },
  {
    id: 'ba-dp3',
    path: 'berita-acara',
    maxHeight: 620,
    blur: baBlur,
    async setup(page) { await openBA(page, 'DP3'); },
    marks: (page) => [
      [1, page.getByLabel('Tanggal dokumen')],
      [2, page.getByLabel('Lokasi / Titik Serah')],
      [3, page.getByLabel('Konsultan Distribusi')],
      [4, page.getByRole('button', { name: 'Penandatangan', exact: true })],
      [5, page.getByRole('button', { name: /Finalisasi & sinkronkan/ })],
      [6, wetBar(page)],
    ],
  },
  {
    id: 'ba-basah',
    path: 'berita-acara',
    blur: baBlur,
    async setup(page) { await openBA(page, 'DP3'); },
    clip: wetBar,
    marks: (page) => [
      [1, wetBar(page).getByText(/^Kode /)],
      [2, wetBar(page).getByText(/^(Belum diunggah|Sudah ditandatangani|Perlu ditandatangani ulang)$/)],
      [3, wetBar(page).getByRole('button', { name: /Cetak halaman tanda tangan/ })],
      [4, wetBar(page).getByText(/Unggah (hasil pindai|ulang)/).first()],
    ],
  },
  {
    id: 'ba-perorangan',
    path: 'berita-acara',
    maxHeight: 620,
    blur: baBlur,
    async setup(page) { await openBA(page, 'BA Perorangan'); await page.getByRole('button', { name: /^05 Oktober/ }).first().click().catch(() => {}); await page.waitForTimeout(2500); },
    marks: (page) => [
      [1, page.getByRole('button', { name: /penerima/ }).first().locator('xpath=..')],
      [2, page.getByRole('button', { name: /Unduh PDF/ }).first()],
      [3, page.getByRole('button', { name: /Finalisasi & sinkronkan/ })],
      [4, page.getByRole('button', { name: /^Lihat \d+ penerima$/ })],
    ],
  },
  {
    id: 'ba-sosialisasi',
    path: 'berita-acara',
    maxHeight: 1000,
    blur: baBlur,
    async setup(page) { await openBA(page, 'Sosialisasi'); },
    marks: (page) => [
      [1, page.getByLabel('Lokasi Sosialisasi')],
      [2, page.getByLabel('Jumlah baris')],
      [3, page.getByLabel('Tanggal kegiatan')],
      [4, page.getByRole('button', { name: 'Cetak', exact: true })],
      [5, page.getByRole('button', { name: 'Unggah berkas' })],
      [6, page.getByRole('button', { name: 'Pindai dari foto' })],
    ],
  },
  {
    id: 'ba-pemeriksaan',
    path: 'berita-acara',
    maxHeight: 760,
    blur: baBlur,
    async setup(page) { await openBA(page, 'BA Pemeriksaan'); },
    marks: (page) => [
      [1, page.getByRole('button', { name: /^Lihat Mesin Pompa/ }).locator('xpath=..')],
      [2, page.getByRole('button', { name: 'Atur daftar form' })],
      [3, page.getByLabel('Tanggal pemeriksaan')],
      [4, page.getByLabel('Pemasok')],
      [5, page.getByRole('button', { name: /Finalisasi semua form/ })],
    ],
  },
  {
    id: 'ba-servis',
    path: 'berita-acara',
    maxHeight: 760,
    blur: baBlur,
    async setup(page) { await openBA(page, 'Servis Berkala'); },
    marks: (page) => [
      [1, page.getByText('Servis ke-1', { exact: true }).locator('xpath=ancestor::fieldset[1]')],
      [2, page.getByText('Servis ke-2', { exact: true }).locator('xpath=ancestor::fieldset[1]')],
      [3, page.getByRole('button', { name: 'Simpan jadwal' })],
      [4, page.getByLabel('Tanggal dokumen')],
      [5, page.getByLabel('Perusahaan Pelaksana')],
    ],
  },
  {
    id: 'ba-tkdn',
    path: 'berita-acara',
    maxHeight: 760,
    blur: baBlur,
    async setup(page) { await openBA(page, 'TKDN'); },
    marks: (page) => [
      [1, page.getByLabel('Tanggal dokumen')],
      [2, page.getByLabel('TKDN gabungan (%)')],
      [3, page.getByRole('button', { name: 'Simpan', exact: true })],
      [4, page.getByLabel('Nama Penyedia')],
      [5, page.getByRole('button', { name: 'Susunan TKDN' })],
    ],
  },
  {
    id: 'ba-penandatangan-switch',
    path: 'berita-acara',
    blur: baBlur,
    async setup(page) {
      await openBA(page, 'DP3');
      await page.getByRole('button', { name: 'Penandatangan', exact: true }).click();
      await page.getByRole('switch').first().waitFor();
      await page.waitForTimeout(1500);
    },
    clip: (page) => page.getByRole('switch').first().locator('xpath=ancestor::div[contains(@class,"grid")][1]'),
    marks: (page) => [
      [1, page.getByRole('switch', { name: /^Atur khusus/ })],
      [2, page.getByRole('switch', { name: 'Tanda tangan basah' })],
    ],
  },
);

// ---- Tutorial Laporan & Map ----
shots.push(
  {
    id: 'lp-filter',
    path: 'laporan',
    blur: ['main table tbody td'],
    async setup(page) { await choose(page, page.getByRole('combobox').first(), schedule); await page.waitForTimeout(2500); },
    marks: (page) => [
      [1, page.getByRole('combobox').nth(0)],
      [2, page.getByRole('combobox').nth(1)],
      [3, page.getByRole('combobox').nth(2)],
      [4, page.getByRole('combobox').nth(3)],
      [5, page.getByLabel('Ringkasan laporan')],
    ],
  },
  {
    id: 'lp-export',
    path: 'laporan',
    maxHeight: 560,
    blur: ['main table tbody td'],
    async setup(page) { await choose(page, page.getByRole('combobox').first(), schedule); await page.waitForTimeout(2500); },
    marks: (page) => [
      [1, page.getByRole('link', { name: /Export Excel/ })],
      [2, page.getByRole('link', { name: /Export PDF/ })],
      [3, page.getByRole('main').getByRole('table').first()],
    ],
  },
  {
    id: 'mp-peta',
    path: 'map-distribusi',
    maxHeight: 900,
    async setup(page) { await page.waitForTimeout(3000); },
    marks: (page) => [
      [1, page.getByRole('button', { name: 'Sebaran penerima' }).locator('xpath=..')],
      [2, page.getByRole('heading', { name: 'Area peta' }).locator('xpath=ancestor::section[1]')],
      [3, page.getByRole('heading', { name: 'Kontrol peta' }).locator('xpath=ancestor::section[1]')],
      [4, page.getByRole('heading', { name: 'Legenda' }).locator('xpath=ancestor::section[1]')],
    ],
  },
  {
    id: 'mp-peringkat',
    path: 'map-distribusi',
    maxHeight: 560,
    async setup(page) { await page.waitForTimeout(3000); },
    clip: (page) => page.getByRole('heading', { name: /^Peringkat wilayah/ }).locator('xpath=ancestor::section[1]'),
    marks: (page) => [
      [1, page.getByRole('button', { name: /^Pilih KAB\./ }).first().locator('xpath=ancestor::tr[1]')],
      [2, page.getByRole('main').getByRole('table').first().locator('thead th').nth(3)],
      [3, page.getByRole('main').getByRole('table').first().locator('thead th').nth(5)],
    ],
  },
);

const only = process.argv.slice(2);
const browser = await chromium.launch({ channel: process.env.GUIDE_CHANNEL ?? 'chrome' });
const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1.5 });
await context.request.get(`${base}/login`);
const login = await context.request.post(`${base}/login`, { form: { identity: user, password }, maxRedirects: 0 });
if (login.status() !== 303) throw new Error(`Login gagal (${login.status()})`);
await mkdir(imageDir, { recursive: true });

let manifest = {};
try { manifest = JSON.parse(await (await import('node:fs/promises')).readFile(manifestPath, 'utf8')); } catch { /* manifest baru */ }

for (const shot of shots) {
  if (only.length && !only.includes(shot.id)) continue;
  const page = await context.newPage();
  if (shot.viewportHeight || shot.viewportWidth) await page.setViewportSize({ width: shot.viewportWidth ?? 1440, height: shot.viewportHeight ?? 900 });
  await page.goto(`${base}/dashboard/${shot.path}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(1500);
  if (shot.setup) await shot.setup(page);
  await page.waitForTimeout(800);
  await mask(page, shot.blur);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(300);

  const boxes = [];
  for (const [n, locator] of shot.marks(page)) {
    const box = await locator.boundingBox().catch(() => null);
    if (!box) { console.warn(`! ${shot.id}: penanda ${n} tidak ditemukan`); continue; }
    const scroll = await page.evaluate(() => ({ x: window.scrollX, y: window.scrollY }));
    boxes.push({ n, x: box.x + scroll.x, y: box.y + scroll.y, w: box.width, h: box.height });
  }
  if (!boxes.length) { console.warn(`! ${shot.id}: dilewati`); await page.close(); continue; }

  // Potong ke area yang memuat semua penanda (selebar konten utama).
  if (process.env.GUIDE_DEBUG) console.log(JSON.stringify(boxes));
  // clip: potong tepat ke satu elemen (mis. satu bagian modal).
  if (shot.clip) {
    const box = await shot.clip(page).boundingBox();
    if (!box) { console.warn(`! ${shot.id}: area tidak ditemukan`); await page.close(); continue; }
    const edge = 10;
    const clip = { x: Math.max(0, box.x - edge), y: Math.max(0, box.y - edge), width: box.width + edge * 2, height: Math.min(shot.maxHeight ?? 1000, box.height + edge * 2) };
    await capture(page, shot, clip, boxes);
    continue;
  }
  // Dialog: potong dari layar penuh (isi dialog bisa lebih panjang dari kotaknya).
  const area = shot.dialog
    ? { x: Math.min(...boxes.map((b) => b.x)) - 40, y: 0, width: Math.max(...boxes.map((b) => b.x + b.w)) - Math.min(...boxes.map((b) => b.x)) + 80, height: Math.max(...boxes.map((b) => b.y + b.h)) + 40 }
    : await page.getByRole('main').boundingBox();
  const pad = 32;
  const top = Math.max(area.y, Math.min(...boxes.map((b) => b.y)) - pad);
  const bottom = Math.min(area.y + Math.max(area.height, 1), Math.max(...boxes.map((b) => b.y + b.h)) + pad);
  const clip = { x: area.x, y: top, width: area.width, height: Math.max(120, Math.min(shot.maxHeight ?? 1000, bottom - top)) };
  await capture(page, shot, clip, boxes);
}

async function capture(page, shot, clip, boxes) {
  const file = `${shot.id}.jpg`;
  await page.screenshot({ path: resolve(imageDir, file), clip, fullPage: true, type: 'jpeg', quality: 82 });
  // Penanda dipotong ke batas gambar (mis. tabel yang lebih lebar dari layar).
  const percent = (value, size) => +((value / size) * 100).toFixed(2);
  const marks = boxes.map((b) => {
    const left = Math.max(b.x, clip.x), right = Math.min(b.x + b.w, clip.x + clip.width);
    const topEdge = Math.max(b.y, clip.y), bottomEdge = Math.min(b.y + b.h, clip.y + clip.height);
    return { n: b.n, x: percent(left - clip.x, clip.width), y: percent(topEdge - clip.y, clip.height), w: percent(right - left, clip.width), h: percent(bottomEdge - topEdge, clip.height) };
  }).filter((mark) => mark.w > 0 && mark.h > 0);
  manifest[shot.id] = {
    src: `/static/images/guide/${file}`,
    width: Math.round(clip.width),
    height: Math.round(clip.height),
    marks,
  };
  // Simpan setiap selesai satu gambar agar error di gambar berikutnya tidak menghapusnya.
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}
`);
  console.log(`✓ ${shot.id} (${marks.length} penanda)`);
  await page.close();
}

await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
await browser.close();
