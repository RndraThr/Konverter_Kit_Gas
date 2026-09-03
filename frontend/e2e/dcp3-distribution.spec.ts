import { expect, test, type Page } from '@playwright/test';
import { resolve } from 'node:path';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

async function navigate(page: Page, label: string, mobile: boolean) {
  if (mobile) await page.getByRole('button', { name: 'Buka navigasi' }).click();
  await page.getByRole('link', { name: label, exact: true }).click();
}

async function expectNoHorizontalOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
}

test.beforeEach(async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
});

test('DCP3 to completed package distribution', async ({ page }, testInfo) => {
  const project = testInfo.project.name;
  const mobile = project === 'mobile';
  const schedule = `E2E Wajo ${project}`;
  const cleanName = `Penerima Bersih E2E ${project}`;
  const cleanNIK = project === 'desktop' ? '9100000000000001' : '9100000000000002';

  await navigate(page, 'Persiapan program', mobile);
  await page.getByRole('tab', { name: 'Jadwal' }).click();
  await expect(page.getByText(schedule, { exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page);

  await navigate(page, 'DCP3', mobile);
  await page.getByLabel('Jadwal distribusi').selectOption({ label: `Wajo E2E - ${schedule}` });
  await page.getByRole('button', { name: /Lanjut ke upload/ }).click();
  await page.getByLabel('Pilih file DCP3').setInputFiles(resolve(process.cwd(), '..', '.cache', 'e2e', `dcp3-${project}.xlsx`));
  await page.getByRole('button', { name: 'Unggah dan baca file' }).click();
  await expect(page.getByRole('heading', { name: 'Cocokkan kolom Excel' })).toBeVisible();
  for (const [field, column] of [
    ['Nomor urut DCP3', 'No'], ['Nama lengkap', 'Nama'], ['NIK', 'NIK'],
    ['Nomor kartu petani', 'No Kartu Petani'], ['Alamat', 'Alamat'],
    ['Desa / kelurahan', 'Desa'], ['Kecamatan', 'Kecamatan'], ['Nomor telepon', 'No HP'],
	]) await page.locator('label').filter({ hasText: new RegExp(`^${field}`) }).getByRole('combobox').selectOption(column);
  await page.getByRole('button', { name: /Periksa data/ }).click();
  await expect(page.getByText(cleanName, { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Import 2 data' }).click();
  await expect(page.getByRole('heading', { name: '2 data selesai diproses' })).toBeVisible();

  await navigate(page, 'Pendistribusian', mobile);
  await page.getByLabel('Jadwal distribusi').selectOption({ label: `Wajo E2E / ${schedule}` });
  const search = page.getByRole('combobox', { name: 'Cari penerima' });

  await search.fill('1');
  await expect(page.getByRole('option', { name: new RegExp(cleanName) })).toBeVisible();
  await search.fill(cleanName);
  const cleanResult = page.getByRole('option', { name: new RegExp(cleanName) });
  await expect(cleanResult).toBeVisible();
  await expect(cleanResult).not.toContainText(cleanNIK);
  await search.fill(cleanNIK);
  await expect(cleanResult).toBeVisible();

  await search.fill('2');
  const blockedResult = page.getByRole('option', { name: /Penerima Riwayat E2E/ });
  await expect(blockedResult.getByText('Pernah menerima')).toBeVisible();
  await blockedResult.click();
  await expect(page.getByText('Penerimaan ulang diblokir')).toBeVisible();
  await page.getByText('Riwayat penerimaan (1)').click();
	await expect(page.getByText('Program Petani E2E 2026', { exact: true })).toBeVisible();

  await search.fill('1');
  await page.getByRole('option', { name: new RegExp(cleanName) }).click();
	const recipientHeading = page.getByRole('heading', { name: cleanName });
	await expect(recipientHeading).toBeVisible();
	await expect.poll(() => recipientHeading.evaluate((element) => element.clientHeight >= element.scrollHeight)).toBe(true);
  await page.getByLabel('Nomor telepon').fill('081234567890');
  await page.getByRole('button', { name: 'Simpan draft' }).click();
  await expect(page.getByText('Draft penerima tersimpan.')).toBeVisible();

  const imagePath = resolve(process.cwd(), '..', 'web', 'static', 'images', 'konkit-aceh-recipient.jpg');
	for (const label of ['Penerima dan paket', 'Serial number mesin', 'Kelengkapan paket', 'BAST bertanda tangan']) {
		const slot = page.locator('article').filter({ has: page.getByRole('heading', { name: label, exact: true }) });
		await slot.getByLabel('Pilih galeri').setInputFiles(imagePath);
		await expect(slot.getByText('Lengkap', { exact: true })).toBeVisible();
  }
  await expect(page.getByRole('button', { name: 'Selesaikan distribusi' })).toBeEnabled();
  await page.getByRole('button', { name: 'Selesaikan distribusi' }).click();
  await expect(page.getByRole('dialog', { name: 'Konfirmasi distribusi' })).toContainText('ERGAS');
  await page.getByRole('button', { name: 'Konfirmasi penyerahan' }).click();
  await expect(page.getByText('Distribusi berhasil diselesaikan.')).toBeVisible();
	await expect(page.getByText('Distribusi selesai', { exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page);
	await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath('distribution-completed.png'), fullPage: true });
});
