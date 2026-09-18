import { expect, test, type Locator, type Page } from '@playwright/test';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
}

async function expectFieldsContained(dialog: Locator) {
  const overflowingValues = await dialog.locator('[data-slot="select-value"]').evaluateAll((values) => values
    .filter((value) => value.scrollWidth > value.clientWidth + 1)
    .map((value) => value.textContent?.trim() ?? ''));
  expect(overflowingValues, 'Nilai select tidak boleh meluber ke kolom di sebelahnya').toEqual([]);

  const fieldEscapes = await dialog.locator('input:not([aria-hidden="true"]), [data-slot="select-trigger"]').evaluateAll((fields) => fields
    .filter((field) => {
      const parent = field.closest('.setupFormSectionFields, .setupDialogBody');
      if (!parent) return false;
      const fieldBox = field.getBoundingClientRect();
      const parentBox = parent.getBoundingClientRect();
      return fieldBox.left < parentBox.left - 1 || fieldBox.right > parentBox.right + 1;
    })
    .map((field) => field.getAttribute('aria-labelledby') ?? field.getAttribute('name') ?? field.tagName));
  expect(fieldEscapes, 'Semua kontrol harus tetap berada di dalam area form').toEqual([]);
}

async function discardEditedDialog(page: Page, dialog: Locator) {
  await dialog.getByRole('button', { name: 'Tutup' }).click();
  const confirmation = page.getByRole('alertdialog', { name: 'Buang perubahan?' });
  await expect(confirmation).toBeVisible();
  await confirmation.getByRole('button', { name: 'Buang perubahan' }).click();
  await expect(confirmation).toBeHidden();
}

test('all edit dialogs remain editable and contained on a compact web viewport', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 768, height: 900 });
  await login(page);
  await page.goto('/dashboard/persiapan-program');

  await page.getByRole('region', { name: 'Kabupaten operasional' }).getByRole('button', { name: /^Edit / }).first().click();
  let dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect(dialog.getByRole('textbox', { name: 'Kabupaten', exact: true })).toBeEditable();
  await dialog.getByRole('textbox', { name: 'Kabupaten', exact: true }).fill('WAJO E2E UI');
  await expect(dialog.getByRole('textbox', { name: 'Kabupaten', exact: true })).toHaveValue('WAJO E2E UI');
  await expectFieldsContained(dialog);
  await discardEditedDialog(page, dialog);

  await page.getByRole('tab', { name: 'Program' }).click();
  await page.getByRole('region', { name: 'Program bantuan' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect(dialog.getByLabel('Nama program')).toBeEditable();
  await dialog.getByLabel('Nama program').fill('PROGRAM UI E2E');
  await expect(dialog.getByLabel('Nama program')).toHaveValue('PROGRAM UI E2E');
  await expectFieldsContained(dialog);
  await discardEditedDialog(page, dialog);

  await page.getByRole('tab', { name: 'Jadwal' }).click();
  await page.getByRole('region', { name: 'Jadwal kabupaten' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect(dialog).toHaveAttribute('data-layout', 'wide');
  await expect(dialog.getByLabel('Nama jadwal')).toBeEditable();
  await expect(dialog.getByLabel('Konsultan pengawas')).toBeEditable();
  await dialog.getByLabel('Nama jadwal').fill('JADWAL UI E2E');
  await dialog.getByLabel('Konsultan pengawas').fill('Pengawas UI');
  await expect(dialog.getByLabel('Konsultan pengawas')).toHaveValue('Pengawas UI');
  await expectFieldsContained(dialog);
  await page.screenshot({ path: testInfo.outputPath('jadwal-edit-768.png') });
  await discardEditedDialog(page, dialog);

  await page.getByRole('tab', { name: 'Template' }).click();
  await page.getByRole('region', { name: 'Template paket' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect(dialog).toHaveAttribute('data-layout', 'workspace');
  await expect(dialog.getByLabel('Nama template')).toBeEditable();
  await dialog.getByLabel('Nama template').fill('PAKET UI E2E');
  await expect(dialog.getByLabel('Nama template')).toHaveValue('PAKET UI E2E');
  await expect(dialog.getByText('Kode template menjadi identitas versi dan tidak dapat diubah.')).toBeVisible();
  await expectFieldsContained(dialog);
  await discardEditedDialog(page, dialog);

  await page.getByRole('region', { name: 'Template dokumentasi' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect(dialog.getByLabel('Nama template')).toBeEditable();
  await dialog.getByLabel('Nama template').fill('DOKUMENTASI UI E2E');
  await expect(dialog.getByLabel('Nama template')).toHaveValue('DOKUMENTASI UI E2E');
  await expect(dialog.getByText('Kode template menjadi identitas versi dan tidak dapat diubah.')).toBeVisible();
  await expectFieldsContained(dialog);
  await page.screenshot({ path: testInfo.outputPath('dokumentasi-edit-768.png') });
});

test('edit dialogs use a safe width on desktop', async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page);
  await page.goto('/dashboard/persiapan-program');

  await page.getByRole('region', { name: 'Kabupaten operasional' }).getByRole('button', { name: /^Edit / }).first().click();
  let dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect.poll(async () => (await dialog.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(840);
  await dialog.getByRole('button', { name: 'Tutup' }).click();

  await page.getByRole('tab', { name: 'Jadwal' }).click();
  await page.getByRole('region', { name: 'Jadwal kabupaten' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  await expect.poll(async () => (await dialog.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(1080);
});

test('workspace navigation stays contained on desktop and mobile', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page);
  await page.goto('/dashboard/persiapan-program');

  const tabList = page.getByRole('tablist', { name: 'Persiapan program' });
  await expect(tabList).toBeVisible();
  await expect(tabList.getByRole('tab')).toHaveCount(4);
  await expect.poll(async () => tabList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('workspace-desktop-1366.png'), fullPage: true });

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(tabList).toBeVisible();
  await expect.poll(async () => tabList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  for (const tab of await tabList.getByRole('tab').all()) await expect(tab).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath('workspace-mobile-390.png'), fullPage: true });
});

test('package editor keeps the active add action reachable while rows grow', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 560, height: 900 });
  await login(page);
  await page.goto('/dashboard/persiapan-program?tab=templates');

  await page.getByRole('region', { name: 'Template paket' }).getByRole('button', { name: 'Tambah paket' }).click();
  const dialog = page.getByRole('dialog', { name: 'Tambah template paket' });
  const addMachine = dialog.getByRole('button', { name: 'Tambah opsi mesin' });

  for (let index = 1; index <= 4; index += 1) {
    await addMachine.click();
    const newField = dialog.getByLabel(`Kode mesin ${index}`);
    await expect(newField).toBeFocused();
    await expect(newField).toBeInViewport();
    await expect(addMachine, 'Kontrol tambah section aktif harus tetap terlihat setelah item ditambahkan').toBeInViewport();
    await expect(dialog.getByRole('heading', { name: 'Tambah template paket' }), 'Judul dialog harus tetap terlihat').toBeInViewport();
    await expect(dialog.getByRole('button', { name: 'Simpan' }), 'Aksi simpan harus tetap terlihat').toBeInViewport();
  }

  await expect(dialog.getByRole('group', { name: 'Opsi mesin 4' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('package-editor-growing-560.png') });
});

test('repeatable template actions stack safely and add editable rows on narrow web popups', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 560, height: 900 });
  await login(page);
  await page.goto('/dashboard/persiapan-program?tab=templates');

  await page.getByRole('region', { name: 'Template paket' }).getByRole('button', { name: /^Edit / }).first().click();
  let dialog = page.getByRole('dialog', { name: /^Edit / });
  for (const [name, fieldPattern] of [
    ['Tambah opsi mesin', /Kode mesin \d+/],
    ['Tambah opsi selang', /Kode selang \d+/],
    ['Tambah komponen', /Kode komponen \d+/],
  ] as const) {
    const button = dialog.getByRole('button', { name });
    const header = button.locator('xpath=ancestor::div[contains(@class,"slotEditorHead")]');
    const widthDifference = await header.evaluate((element) => {
      const button = element.querySelector('button');
      return button ? Math.abs(element.getBoundingClientRect().width - button.getBoundingClientRect().width) : Number.POSITIVE_INFINITY;
    });
    expect(widthDifference, `${name} harus selebar area editor`).toBeLessThanOrEqual(2);
    await button.click();
    const newField = dialog.getByLabel(fieldPattern).last();
    await expect(newField, `${name} harus menampilkan kolom baru`).toBeInViewport();
    await expect(newField, `${name} harus langsung mengaktifkan kolom baru`).toBeFocused();
  }
  await expect(dialog.getByLabel(/Merk mesin \d+/).last()).toBeEditable();
  await expect(dialog.getByLabel(/Merk selang \d+/).last()).toBeEditable();
  await expect(dialog.getByLabel(/Nama komponen \d+/).last()).toBeEditable();
  await expectFieldsContained(dialog);
  await page.screenshot({ path: testInfo.outputPath('repeatable-package-560.png') });
  await discardEditedDialog(page, dialog);

  await page.getByRole('region', { name: 'Template dokumentasi' }).getByRole('button', { name: /^Edit / }).first().click();
  dialog = page.getByRole('dialog', { name: /^Edit / });
  const addSlot = dialog.getByRole('button', { name: 'Tambah slot' });
  const slotHeader = addSlot.locator('xpath=ancestor::div[contains(@class,"slotEditorHead")]');
  const widthDifference = await slotHeader.evaluate((element) => {
    const button = element.querySelector('button');
    return button ? Math.abs(element.getBoundingClientRect().width - button.getBoundingClientRect().width) : Number.POSITIVE_INFINITY;
  });
  expect(widthDifference, 'Tambah slot harus selebar area editor').toBeLessThanOrEqual(2);
  await addSlot.click();
  const newSlot = dialog.getByLabel('Kode slot').last();
  await expect(newSlot).toBeEditable();
  await expect(newSlot).toBeInViewport();
  await expect(newSlot).toBeFocused();
  await expectFieldsContained(dialog);
  await page.screenshot({ path: testInfo.outputPath('repeatable-documentation-560.png') });
});
