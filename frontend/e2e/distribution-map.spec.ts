import { expect, test, type Page } from '@playwright/test';

const username = 'e2e.admin';
const password = 'Konkit-E2E-Password-2026';
const MAP_URL = '/dashboard/map-distribusi';

// Slots seeded by cmd/e2eseed (Kabupaten Indramayu): 1 completed alone, 2 + 3 share the titik serah
// "Balai Desa E2E", 4 photographed in Wajo (outside the boundary), 5 open and 6 linked without photos.
const LOCATED_SLOTS = 4;
const UNLOCATED_SLOTS = 2;

// A 1x1 transparent PNG stands in for every Esri raster tile.
const BLANK_TILE = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==', 'base64');

/**
 * Keeps the tests independent of external tile servers. The OpenFreeMap style is replaced with a
 * minimal local style (a background layer; its glyph URL is fulfilled with an empty 200, so symbol
 * labels draw nothing but raise no error) and Esri tiles with a blank PNG. Nothing here is aborted,
 * so any console error in a test is a real one.
 */
async function stubBasemaps(page: Page) {
  await page.route('**/tiles.openfreemap.org/**', (route) => {
    const url = route.request().url();
    if (url.includes('/styles/')) {
      return route.fulfill({
        contentType: 'application/json',
        headers: { 'access-control-allow-origin': '*' },
        body: JSON.stringify({
          version: 8, name: 'e2e', sources: {},
          glyphs: 'https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf',
          layers: [{ id: 'background', type: 'background', paint: { 'background-color': '#e8efe9' } }],
        }),
      });
    }
    return route.fulfill({ status: 200, contentType: 'application/x-protobuf', headers: { 'access-control-allow-origin': '*' }, body: '' });
  });
  await page.route('**/server.arcgisonline.com/**', (route) => route.fulfill({ status: 200, contentType: 'image/png', headers: { 'access-control-allow-origin': '*' }, body: BLANK_TILE }));
}

async function login(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Email atau username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page).toHaveURL(/\/dashboard/);
}

function collectConsoleErrors(page: Page) {
  const errors: string[] = [];
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
  page.on('pageerror', (error) => errors.push(error.message));
  return errors;
}

/**
 * The test database may hold other programs, so most tests look at the seeded Indramayu schedule only,
 * through the same schedule_id filter the page offers. The id comes from the API the page itself uses.
 */
async function scheduleScope(page: Page) {
  const response = await page.request.get('/api/v1/program-setup/schedules');
  expect(response.ok()).toBe(true);
  const { data } = await response.json() as { data: Array<{ id: string; name: string }> };
  const schedule = data.find((item) => item.name === 'E2E Peta Indramayu');
  expect(schedule, 'seeded schedule E2E Peta Indramayu').toBeTruthy();
  return `?schedule_id=${schedule?.id}`;
}

async function openMap(page: Page, query = '') {
  await page.goto(MAP_URL + query);
  await expect(page.getByRole('combobox', { name: /Cari kabupaten/ })).toBeVisible();
  await expect(page.locator('canvas.maplibregl-canvas')).toBeVisible();
}

async function pickSearchResult(page: Page, text: string, option: string | RegExp) {
  const search = page.getByRole('combobox', { name: /Cari kabupaten/ });
  await search.fill(text);
  await page.getByRole('option', { name: option }).first().click();
}

test.beforeEach(async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'Viewport is set explicitly per test');
  await stubBasemaps(page);
  await login(page);
});

test('shows points and the counts of located and unlocated slots', async ({ page }) => {
  const errors = collectConsoleErrors(page);
  const points = page.waitForResponse((response) => response.url().includes('/api/v1/map/points') && response.ok());
  await openMap(page, await scheduleScope(page));
  const body = await (await points).json() as { data: { features: Array<{ properties: { location_status: string } }>; meta: { located: number; unlocated: number; outside: number } } };

  expect(body.data.meta.located).toBe(LOCATED_SLOTS);
  expect(body.data.meta.unlocated).toBe(UNLOCATED_SLOTS);
  expect(body.data.meta.outside).toBe(1);
  expect(body.data.features.filter((f) => f.properties.location_status === 'inside')).toHaveLength(3);
  // The point response never carries a recipient name or NIK.
  expect(JSON.stringify(body)).not.toMatch(/Penerima Peta|full_name|nik/i);

  await expect(page.getByRole('button', { name: /belum ada lokasi/ })).toContainText(String(UNLOCATED_SLOTS));
  await expect(page.getByText('Titik terdata').first()).toBeVisible();
  await expect.poll(() => errors, { message: 'console errors while loading the map' }).toEqual([]);
});

test('selecting a program and zone changes the breadcrumb', async ({ page }) => {
  await openMap(page);
  const breadcrumb = page.getByRole('navigation', { name: 'Lokasi peta' });
  await expect(breadcrumb).toContainText('Indonesia');
  await expect(breadcrumb).not.toContainText('Zona E2E');

  await page.getByRole('combobox', { name: 'Program' }).click();
  await page.getByRole('option', { name: 'Program Petani E2E 2026' }).click();
  await page.getByRole('combobox', { name: 'Zona' }).click();
  await page.getByRole('option', { name: 'Zona E2E' }).click();

  await expect(breadcrumb).toContainText('Zona E2E');
  await expect(page).toHaveURL(/zone_id=/);
  // Back to Indonesia clears the zone again.
  await breadcrumb.getByRole('button', { name: 'Indonesia' }).click();
  await expect(breadcrumb).not.toContainText('Zona E2E');
});

test('a point opens its detail and its titik serah lists two or more slots', async ({ page }) => {
  await openMap(page, await scheduleScope(page));
  await pickSearchResult(page, '2', /^Nomor bagi 2\b/);

  const detail = page.getByRole('region', { name: 'Detail titik' });
  await expect(detail.getByRole('heading', { name: 'Nomor bagi 2' })).toBeVisible();
  await expect(page).toHaveURL(/slot=/);
  // The geo-site ids are never put in the URL; this slot has a named titik serah ("hl:" id).
  await detail.getByRole('button', { name: 'Lihat titik serah' }).click();

  await expect(detail.getByRole('heading', { name: 'Balai Desa E2E' })).toBeVisible();
  const members = detail.getByRole('list', { name: 'Daftar nomor bagi' }).getByRole('listitem');
  await expect(members).toHaveCount(2);
  await expect(members.filter({ hasText: 'Nomor bagi 2' })).toHaveCount(1);
  await expect(members.filter({ hasText: 'Nomor bagi 3' })).toHaveCount(1);

  // Choosing a member from the list goes back to the point view of that slot.
  await members.filter({ hasText: 'Nomor bagi 3' }).getByRole('button').click();
  await expect(detail.getByRole('heading', { name: 'Nomor bagi 3' })).toBeVisible();

  await detail.getByRole('button', { name: 'Tutup' }).click();
  await expect(detail).toBeHidden();
});

test('a point photographed outside its kabupaten is flagged for review', async ({ page }) => {
  await openMap(page, await scheduleScope(page));
  await pickSearchResult(page, '4', /^Nomor bagi 4\b/);
  const detail = page.getByRole('region', { name: 'Detail titik' });
  await expect(detail.getByRole('heading', { name: 'Nomor bagi 4' })).toBeVisible();
  await expect(detail.getByText(/Perlu ditinjau/).first()).toBeVisible();
});

test('lists the slots without a location', async ({ page }) => {
  await openMap(page, await scheduleScope(page));
  await page.getByRole('button', { name: /belum ada lokasi/ }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText('Nomor bagi 5')).toBeVisible();
  await expect(dialog.getByText('Nomor bagi 6')).toBeVisible();
  await expect(dialog.getByText('Nomor bagi 1', { exact: true })).toHaveCount(0);
});

test('a phone viewport shows the detail as a bottom sheet', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openMap(page, await scheduleScope(page));
  await pickSearchResult(page, '1', /^Nomor bagi 1\b/);

  const sheet = page.locator('section[data-variant="sheet"]');
  await expect(sheet).toBeVisible();
  await expect(page.locator('section[data-variant="panel"]')).toHaveCount(0);
  const box = await sheet.boundingBox();
  expect(box).not.toBeNull();
  expect(Math.round((box?.y ?? 0) + (box?.height ?? 0))).toBe(844);
  expect(box?.width).toBe(390);
  // The page itself does not scroll sideways under the sheet.
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.keyboard.press('Escape');
  await expect(sheet).toBeHidden();
});

test('switching to Hybrid and back raises no console error', async ({ page }) => {
  const errors = collectConsoleErrors(page);
  await openMap(page, await scheduleScope(page));
  const layers = page.getByRole('group', { name: 'Jenis peta' });
  for (const name of ['Hybrid', 'Satelit', 'Biasa', 'Hybrid']) {
    await layers.getByRole('button', { name, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`base=${{ Hybrid: 'hyb', Satelit: 'sat', Biasa: 'map' }[name]}`));
    await page.waitForTimeout(400);
  }
  await expect(page.locator('canvas.maplibregl-canvas')).toBeVisible();
  expect(errors).toEqual([]);
});

test('attribution for OpenStreetMap, Esri and geoBoundaries is shown', async ({ page }) => {
  await openMap(page, await scheduleScope(page));
  // The compact control keeps its text in the DOM while collapsed, so read textContent.
  const attribution = page.locator('.maplibregl-ctrl-attrib-inner');
  for (const credit of ['OpenStreetMap', 'Esri', 'geoBoundaries']) {
    await expect.poll(() => attribution.evaluate((element) => element.textContent ?? '')).toContain(credit);
  }
});

test('a basemap that cannot load falls back to light mode with a notice', async ({ page }) => {
  test.setTimeout(60_000);
  // Registered after stubBasemaps, so it wins: the style request fails like a dead link would.
  await page.route('**/tiles.openfreemap.org/**', (route) => route.abort());
  await page.goto(MAP_URL);

  const notice = page.getByRole('status').filter({ hasText: 'Peta ringan dipakai karena peta dasar gagal dimuat' });
  await expect(notice).toBeVisible({ timeout: 20_000 });
  await expect(page.locator('canvas.maplibregl-canvas')).toHaveCount(0);
  // The progress table keeps working without the basemap, and the way back is offered.
  await expect(notice.getByRole('button', { name: 'Coba peta interaktif lagi' })).toBeVisible();
  await expect(page.getByRole('button', { name: /belum ada lokasi/ })).toBeVisible();
});

test('the map fills its frame with a real height', async ({ page }) => {
  await openMap(page, await scheduleScope(page));
  const map = page.locator('div.maplibregl-map');
  const box = await map.boundingBox();
  const parentBox = await map.locator('xpath=..').boundingBox();
  expect(box?.height ?? 0).toBeGreaterThan(200);
  expect(Math.abs((box?.height ?? 0) - (parentBox?.height ?? -1))).toBeLessThanOrEqual(2);
});

test('the toolbar fits a 390 px phone without sideways scroll', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openMap(page, await scheduleScope(page));
  await expect(page.getByRole('button', { name: /belum ada lokasi/ })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  const layers = page.getByRole('group', { name: 'Jenis peta' });
  const targets = [
    layers.getByRole('button', { name: 'Satelit', exact: true }),
    layers.getByRole('button', { name: 'Hybrid', exact: true }),
    page.getByRole('button', { name: /belum ada lokasi/ }),
  ];
  for (const target of targets) {
    const box = await target.boundingBox();
    expect(box).not.toBeNull();
    expect(box?.x ?? -1).toBeGreaterThanOrEqual(0);
    expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(390);
  }
});
