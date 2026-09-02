import { expect, test } from '@playwright/test';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

test.beforeEach(async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
});

test('administration foundation journey', async ({ page }, testInfo) => {
  const displayName = `Petugas E2E ${testInfo.project.name}`;
  await expect(page.getByRole('heading', { name: 'Ringkasan program' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('dashboard.png'), fullPage: true });

  for (const [link, heading] of [
    ['Profil saya', 'Profil saya'], ['Pengguna', 'Pengguna'], ['Role & akses', 'Role & akses'],
    ['Pengaturan', 'Pengaturan sistem'], ['Kesehatan sistem', 'Kesehatan sistem'], ['Riwayat aktivitas', 'Riwayat aktivitas'],
  ]) {
    if (testInfo.project.name === 'mobile') {
      await page.getByRole('button', { name: 'Buka navigasi' }).click();
    }
    await page.getByRole('link', { name: link, exact: true }).click();
    await expect(page.getByRole('heading', { name: heading })).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`${link.toLowerCase().replaceAll(' ', '-')}.png`), fullPage: true });
  }

  if (testInfo.project.name === 'mobile') await page.getByRole('button', { name: 'Buka navigasi' }).click();
  await page.getByRole('link', { name: 'Pengguna', exact: true }).click();
  await page.getByRole('button', { name: 'Tambah pengguna' }).click();
  await page.getByLabel('Nama lengkap').fill(displayName);
  await page.getByLabel('Username').fill(`petugas.e2e.${testInfo.project.name}`);
  await page.getByLabel('Email').fill(`petugas.${testInfo.project.name}@konkit.test`);
  await page.getByLabel('Password awal').fill('Petugas-E2E-Password');
  await page.getByRole('checkbox', { name: 'Super Admin' }).check();
  await page.getByRole('button', { name: 'Simpan pengguna' }).click();
  await expect(page.getByText(displayName)).toBeVisible();
  await page.getByRole('button', { name: `Edit ${displayName}` }).click();
  await page.getByRole('checkbox', { name: 'Pengguna aktif' }).uncheck();
  await page.getByRole('button', { name: 'Simpan pengguna' }).click();
  await expect(page.getByText('Nonaktif')).toBeVisible();

  if (testInfo.project.name === 'mobile') await page.getByRole('button', { name: 'Buka navigasi' }).click();
  await page.getByRole('link', { name: 'Riwayat aktivitas' }).click();
  await expect(page.getByText(/user\.(created|updated)/).first()).toBeVisible();

  await page.getByRole('button', { name: 'Menu akun' }).click();
  await page.getByRole('button', { name: 'Keluar' }).click();
  await expect(page).toHaveURL(/\/login/);
  await page.goto('/dashboard');
  await expect(page).toHaveURL(/\/login/);
});
