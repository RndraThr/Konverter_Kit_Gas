import { expect, test } from '@playwright/test';

test('uses the photo carousel as the mobile login background', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'mobile', 'Mobile-only responsive contract');

  await page.goto('/login');

  const viewport = page.viewportSize();
  const photoPanel = page.getByRole('region', { name: 'Cerita lapangan Konkit' });
  const formPanel = page.getByRole('region', { name: 'Form login' });
  const formSurface = formPanel.locator(':scope > div');
  const [photoBox, formBox] = await Promise.all([photoPanel.boundingBox(), formPanel.boundingBox()]);
  const surfaceColor = await formSurface.evaluate((element) => getComputedStyle(element).backgroundColor);
  const alpha = Number(surfaceColor.match(/[\d.]+(?=\)$)/)?.[0] ?? 1);

  expect(viewport).not.toBeNull();
  expect(photoBox).not.toBeNull();
  expect(formBox).not.toBeNull();
  expect(photoBox?.x).toBe(0);
  expect(photoBox?.y).toBe(0);
  expect(photoBox?.width).toBe(viewport?.width);
  expect(photoBox?.height).toBe(viewport?.height);
  expect(formBox?.y).toBeLessThan((photoBox?.y ?? 0) + (photoBox?.height ?? 0));
  expect(alpha).toBeGreaterThanOrEqual(0.5);
  expect(alpha).toBeLessThanOrEqual(0.65);
  await expect(page.getByRole('form', { name: 'Masuk ke dashboard' })).toBeVisible();
});
