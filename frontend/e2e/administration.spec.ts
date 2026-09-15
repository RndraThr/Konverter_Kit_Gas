import { expect, test, type Page } from '@playwright/test';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

async function navigate(page: Page, label: string, mobile: boolean) {
  if (mobile) await page.getByRole('button', { name: 'Buka navigasi' }).click();
  await page.getByRole('link', { name: label, exact: true }).click();
  // The mobile Sheet closes with a 150ms opacity transition; screenshotting
  // before it settles captures the drawer ghosted over the page content.
  if (mobile) await expect(page.locator('[data-slot="sheet-overlay"]')).not.toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
});

test('administration foundation journey', async ({ page }, testInfo) => {
  const displayName = `Petugas E2E ${testInfo.project.name}`;
  await expect(page.getByRole('heading', { name: 'Data Penerima', exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('data-penerima.png'), fullPage: true });

  if (testInfo.project.name === 'desktop') {
    const sidebar = page.getByLabel('Sidebar utama');
    const toggle = page.getByRole('button', { name: 'Minimalkan sidebar' });
    const ergasLogo = sidebar.getByAltText('Ergas');
    const ksmLogo = sidebar.getByAltText('PT Kian Santang Mulitama Tbk');
    const [sidebarBox, toggleBox, ergasBox, ksmBox] = await Promise.all([
      sidebar.boundingBox(), toggle.boundingBox(), ergasLogo.boundingBox(), ksmLogo.boundingBox(),
    ]);

    expect(toggleBox?.width).toBe(44);
    expect(toggleBox?.height).toBe(44);
    expect(Math.abs((toggleBox?.x ?? 0) + 22 - ((sidebarBox?.x ?? 0) + (sidebarBox?.width ?? 0)))).toBeLessThanOrEqual(1);
    expect(toggleBox?.y).toBeLessThan(40);
    expect(ergasBox?.width).toBeGreaterThanOrEqual(76);
    expect(ksmBox?.width).toBeGreaterThanOrEqual(92);

    await toggle.click();
    await expect(page.getByLabel('Sidebar utama')).toHaveAttribute('data-state', 'collapsed');
    await expect(page.getByRole('button', { name: 'Maksimalkan sidebar' })).toBeVisible();
    await expect.poll(async () => (await page.getByLabel('Sidebar utama').boundingBox())?.width).toBe(76);
    await expect.poll(async () => (await page.getByLabel('Sidebar utama').getByAltText('Ergas').boundingBox())?.width).toBeGreaterThanOrEqual(48);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath('sidebar-minimized.png'), fullPage: true });
    await page.reload();
    await expect(page.getByRole('button', { name: 'Maksimalkan sidebar' })).toBeVisible();
    await page.getByRole('button', { name: 'Maksimalkan sidebar' }).click();
    await expect(page.getByRole('button', { name: 'Minimalkan sidebar' })).toBeVisible();
  }

  await navigate(page, 'Map Distribusi', testInfo.project.name === 'mobile');
  await expect(page.getByRole('heading', { name: 'Map Distribusi', exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('map-distribusi.png'), fullPage: true });

  await navigate(page, 'Profil saya', testInfo.project.name === 'mobile');
  await expect(page.getByRole('heading', { name: 'Profil saya', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Informasi profil', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Keamanan akun', exact: true })).toBeVisible();
  await expect(page.getByLabel('Password saat ini')).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('profil-saya.png'), fullPage: true });

  for (const [link, heading] of [
    ['Pengguna', 'Pengguna'], ['Role & akses', 'Role & akses'],
    ['Pengaturan', 'Pengaturan sistem'], ['Kesehatan sistem', 'Kesehatan sistem'], ['Riwayat aktivitas', 'Riwayat aktivitas'],
  ]) {
    await navigate(page, link, testInfo.project.name === 'mobile');
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`${link.toLowerCase().replaceAll(' ', '-')}.png`), fullPage: true });
  }

  await navigate(page, 'Pengguna', testInfo.project.name === 'mobile');
  await page.getByRole('button', { name: 'Tambah pengguna' }).click();
  await page.getByLabel('Nama lengkap').fill(displayName);
  await page.getByLabel('Username').fill(`petugas.e2e.${testInfo.project.name}`);
  await page.getByLabel('Email').fill(`petugas.${testInfo.project.name}@konkit.test`);
  await page.getByLabel('Password awal').fill('Petugas-E2E-Password');
  await page.getByRole('checkbox', { name: 'Super Admin' }).check();
  await page.getByRole('button', { name: 'Simpan pengguna' }).click();
  await expect(page.getByText(displayName)).toBeVisible();
  await page.getByRole('button', { name: `Aksi ${displayName}` }).click();
  await page.getByRole('menuitem', { name: 'Edit pengguna' }).click();
  await page.getByRole('checkbox', { name: 'Pengguna aktif' }).uncheck();
  await page.getByRole('button', { name: 'Simpan pengguna' }).click();
  await page.getByRole('alertdialog', { name: `Nonaktifkan ${displayName}?` }).getByRole('button', { name: 'Nonaktifkan pengguna' }).click();
  await expect(page.getByRole('row').filter({ hasText: displayName }).getByText('Nonaktif')).toBeVisible();

  await navigate(page, 'Riwayat aktivitas', testInfo.project.name === 'mobile');
  await expect(page.getByText(/user\.(created|updated)/).first()).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath(`konkit-${testInfo.project.name}-final.png`), fullPage: true });

  await page.getByRole('button', { name: 'Menu akun' }).click();
  await page.getByRole('menuitem', { name: 'Keluar' }).click();
  await expect(page).toHaveURL(/\/login/);
  await page.goto('/dashboard');
  await expect(page).toHaveURL(/\/login/);
});
