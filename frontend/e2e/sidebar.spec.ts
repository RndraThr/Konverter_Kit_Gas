import { expect, test, type Page } from '@playwright/test';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
}

test.beforeEach(async ({ page }) => {
  await login(page);
});

test('desktop sidebar collapses without moving its toggle', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop');

  const sidebar = page.getByLabel('Sidebar utama');
  const minimize = page.getByRole('button', { name: 'Minimalkan sidebar' });
  await expect(sidebar).toHaveAttribute('id', 'primary-sidebar');
  await expect(minimize).toHaveAttribute('aria-controls', 'primary-sidebar');

  const [expandedSidebarBox, initialToggleBox] = await Promise.all([sidebar.boundingBox(), minimize.boundingBox()]);
  expect(expandedSidebarBox?.width).toBe(264);
  expect(initialToggleBox?.width).toBe(44);
  expect(initialToggleBox?.height).toBe(44);
  await page.screenshot({ path: testInfo.outputPath('sidebar-expanded.png'), fullPage: true });

  await minimize.click();
  const maximize = page.getByRole('button', { name: 'Maksimalkan sidebar' });
  await expect(sidebar).toHaveAttribute('data-state', 'collapsed');
  await expect(maximize).toHaveAttribute('aria-controls', 'primary-sidebar');
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBe(76);
  await expect.poll(async () => {
    const box = await maximize.boundingBox();
    return Math.abs((box?.y ?? 0) - (initialToggleBox?.y ?? 0));
  }).toBeLessThanOrEqual(1);
  const [compactLogoBox, maximizeBox] = await Promise.all([sidebar.getByAltText('Ergas').boundingBox(), maximize.boundingBox()]);
  expect((compactLogoBox?.x ?? 0) + (compactLogoBox?.width ?? 0)).toBeLessThanOrEqual((maximizeBox?.x ?? 0) - 2);

  const recipientLabel = sidebar.getByRole('link', { name: 'Data Penerima' }).getByText('Data Penerima');
  await expect.poll(() => recipientLabel.evaluate((element) => getComputedStyle(element).opacity)).toBe('0');
  const groupBorders = await sidebar.getByRole('navigation', { name: 'Navigasi utama' }).locator('section').evaluateAll((sections) => sections.slice(1).map((section) => getComputedStyle(section).borderTopStyle));
  expect(groupBorders.every((style) => style === 'solid')).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('sidebar-collapsed.png'), fullPage: true });

  await page.reload();
  await expect(page.getByRole('button', { name: 'Maksimalkan sidebar' })).toBeVisible();
});

test('tablet uses drawer navigation instead of the fixed sidebar', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop');
  await page.setViewportSize({ width: 1023, height: 900 });

  await expect(page.getByLabel('Sidebar utama')).toBeHidden();
  const trigger = page.getByRole('button', { name: 'Buka navigasi' });
  await expect(trigger).toBeVisible();
  await trigger.click();
  const drawer = page.getByRole('dialog', { name: 'Navigasi utama' });
  await expect(drawer).toBeVisible();
  await drawer.getByRole('button', { name: 'Tutup navigasi' }).click();

  await page.setViewportSize({ width: 1024, height: 900 });
  await expect(page.getByLabel('Sidebar utama')).toBeVisible();
  await expect(trigger).toBeHidden();
});

test('mobile navigation has an explicit close action and restores focus', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'mobile');

  const trigger = page.getByRole('button', { name: 'Buka navigasi' });
  await trigger.click();
  const navigation = page.getByRole('dialog', { name: 'Navigasi utama' });
  const close = navigation.getByRole('button', { name: 'Tutup navigasi' });
  await expect(close).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('sidebar-mobile-open.png'), fullPage: true });

  await close.click();
  await expect(navigation).not.toBeVisible();
  await expect(trigger).toBeFocused();
});
