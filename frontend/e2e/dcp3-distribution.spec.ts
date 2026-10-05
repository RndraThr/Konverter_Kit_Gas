import { expect, test, type Locator, type Page } from '@playwright/test';
import { resolve } from 'node:path';
import { navigateDashboard } from './navigation';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

async function expectNoHorizontalOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
}

async function chooseOption(trigger: Locator, optionName: string | RegExp) {
  await trigger.click();
  await trigger.page().getByRole('option', { name: optionName, exact: true }).click();
}

test.beforeEach(async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page.getByText('Login berhasil. Mengarahkan ke dashboard…')).toBeVisible();
  await expect(page).toHaveURL(/\/dashboard/);
});

test('DCP3 to completed package distribution', async ({ page }, testInfo) => {
	test.setTimeout(120_000);
  const project = testInfo.project.name;
  const mobile = project === 'mobile';
  const schedule = `E2E Wajo ${project}`;
  const cleanName = `Penerima Bersih E2E ${project}`;
  const storedCleanName = cleanName.toUpperCase();
  const cleanNIK = project === 'desktop' ? '9100000000000001' : '9100000000000002';

  await navigateDashboard(page, 'Persiapan program', mobile);
  await page.getByRole('tab', { name: 'Jadwal' }).click();
  await expect(page.getByText(schedule, { exact: true })).toBeVisible();
  await expect.poll(() => page.getByRole('tablist').evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(mobile);
  await expectNoHorizontalOverflow(page);

  await navigateDashboard(page, 'DCP3', mobile);
  await chooseOption(page.getByRole('combobox', { name: 'Jadwal distribusi' }), `Wajo E2E - ${schedule}`);
  await page.getByRole('button', { name: /Lanjut ke upload/ }).click();
  await page.getByLabel('Pilih file DCP3').setInputFiles(resolve(process.cwd(), '..', '.cache', 'e2e', `dcp3-${project}.xlsx`));
  await page.getByRole('button', { name: 'Unggah dan baca file' }).click();
  await expect(page.getByRole('heading', { name: 'Cocokkan kolom Excel' })).toBeVisible();
  for (const [field, column] of [
    ['Nomor urut DCP3', 'No'], ['Nama lengkap', 'Nama'], ['NIK', 'NIK'],
    ['Nomor kartu petani', 'No Kartu Petani'], ['Alamat', 'Alamat'],
    ['Desa / kelurahan', 'Desa'], ['Kecamatan', 'Kecamatan'], ['Nomor telepon', 'No HP'],
	]) await chooseOption(page.getByRole('combobox', { name: new RegExp(`^${field}`) }), column);
  await page.getByRole('button', { name: /Periksa data/ }).click();
  await expect(page.getByText(cleanName, { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Import 2 data' }).click();
  await expect(page.getByRole('heading', { name: '2 data selesai diproses' })).toBeVisible();

  await navigateDashboard(page, 'Pendistribusian', mobile);
  await chooseOption(page.getByRole('combobox', { name: 'Jadwal distribusi' }), `Wajo E2E / ${schedule}`);
  await page.getByRole('button', { name: 'Buat Nomor 1' }).click();
  await chooseOption(page.getByLabel('Merk/Tipe Mesin'), /SHARK/);
  await page.getByRole('textbox', { name: 'Serial Number Mesin', exact: true }).fill(`MESIN-${project}`);
  await chooseOption(page.getByLabel('Merk Konkit/Reducer'), /ERGAS/);
  await page.getByRole('textbox', { name: 'Serial Number Konkit/Reducer', exact: true }).fill(`KONKIT-${project}`);
  await chooseOption(page.getByLabel('Merk/Spesifikasi Selang'), /TRILIUNHOSE/);
  await expect(page.getByRole('textbox', { name: 'Serial Number Selang', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Simpan Nomor Bagi' }).click();
  await expect(page.getByLabel('POS Mesin')).toBeVisible();

  const recipientNIK = page.getByLabel('NIK Penerima');
  await recipientNIK.fill('9100000000000099');
  await page.getByRole('option', { name: '9100000000000099 Penerima Riwayat E2E' }).click();
  await expect(page.getByLabel('Nama')).toHaveValue('Penerima Riwayat E2E');
  await page.getByRole('button', { name: 'Hubungkan ke Nomor Bagi Ini' }).click();
  await expect(page.getByText('Penerima sudah pernah menerima paket sebelumnya')).toBeVisible();

  await recipientNIK.fill(cleanNIK);
  await page.getByRole('option', { name: `${cleanNIK} ${storedCleanName}` }).click();
  await expect(page.getByLabel('Nama')).toHaveValue(storedCleanName);
  await page.getByLabel('Nomor telepon').fill('081234567890');
  await page.getByRole('button', { name: 'Hubungkan ke Nomor Bagi Ini' }).click();
  const recipientHeading = page.getByRole('heading', { name: storedCleanName });
  await expect(recipientHeading).toBeVisible();
  await expect.poll(() => recipientHeading.evaluate((element) => element.clientHeight >= element.scrollHeight)).toBe(true);

  const imagePath = resolve(process.cwd(), '..', 'web', 'static', 'images', 'konkit-aceh-recipient.jpg');
	const requiredPhotoLabels = [
		'Foto Mesin Baru dan Nomor Urut',
		'Foto Nomor Seri Mesin Baru dan Nomor Urut',
		'Foto KTP dan Nomor Urut',
		'Foto KK dan Nomor Urut',
		'Foto Kartu Tani atau Surat Keterangan dan Nomor Urut',
		'Foto Surat Keterangan Luas Lahan dan Nomor Urut',
		'Foto Penerima dengan KTP dan Nomor Urut',
		'Foto Penerima dengan Paket Distribusi beserta Toolkit, Manual Book, Kartu Garansi, dan Nomor Urut (di depan banner backdrop)',
		'Foto Penerima dengan Teknisi dan Nomor Urut (di depan banner backdrop)',
		'Foto Penerima untuk Training dan Nomor Urut (di depan banner pelatihan teknis)',
		'Foto Penerima dengan Mesin Lama dan Nomor Urut',
	];
	for (const label of requiredPhotoLabels) {
		const slot = page.locator('article').filter({ has: page.getByRole('heading', { name: label, exact: true }) });
		const uploadResponse = page.waitForResponse((response) => response.request().method() === 'POST' && /\/api\/v1\/distribution\/slots\/.+\/media$/.test(response.url()));
		await slot.getByLabel('Pilih galeri').setInputFiles(imagePath);
		const response = await uploadResponse;
		expect(response.ok(), `${response.status()} ${await response.text()}`).toBe(true);
		await expect(slot.getByText('Lengkap', { exact: true })).toBeVisible();
  }
	const firstPhotoSlot = page.locator('article').filter({ has: page.getByRole('heading', { name: requiredPhotoLabels[0], exact: true }) });
	await firstPhotoSlot.getByRole('button', { name: /^Lihat / }).click();
	await expect(page.getByRole('dialog', { name: /^Preview / })).toBeVisible();
	await expect(page.getByLabel('Tingkat zoom foto')).toHaveValue('100');
	await page.getByRole('button', { name: 'Perbesar foto' }).click();
	await expect(page.getByLabel('Tingkat zoom foto')).toHaveValue('125');
	await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Selesaikan Distribusi' })).toBeEnabled();
  await page.getByRole('button', { name: 'Selesaikan Distribusi' }).click();
  await expect(page.getByRole('alertdialog', { name: 'Konfirmasi distribusi' })).toContainText('Aksi ini tidak dapat dibatalkan');
  await page.getByRole('button', { name: 'Konfirmasi penyerahan' }).click();
	await expect(page.getByText('Distribusi selesai', { exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page);
	await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath('distribution-completed.png'), fullPage: true });
  await page.screenshot({ path: testInfo.outputPath(`konkit-${project}-final.png`), fullPage: true });
});
