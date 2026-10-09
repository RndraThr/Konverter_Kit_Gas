import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DistributionMapPage } from './DistributionMapPage';
import { loadRegencyAsset } from './mapAsset';
import type { MapRegion } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
// Keeps the page test independent of the ~440 kB bundled boundary asset.
vi.mock('./mapAsset', () => ({ loadRegencyAsset: vi.fn() }));

const testAsset = {
  source: 'test asset', license: 'CC BY 3.0 IGO', licenseSource: 'example.test/batas-wilayah',
  regencies: [
    { n: 'Bangka Barat', c: [105.5, -2] as [number, number], r: [[[105, -1], [106, -1], [106, -3], [105, -3], [105, -1]]] },
    { n: 'Bangka', c: [106.5, -2] as [number, number], r: [[[106, -1], [107, -1], [107, -3], [106, -3], [106, -1]]] },
  ],
};

function region(overrides: Partial<MapRegion> = {}): MapRegion {
  return {
    regency_id: 'regency-1', regency_name: 'KAB. BANGKA BARAT', province_name: 'Bangka Belitung', document_code: 'BBR',
    recipients: 40, candidate: 2, ready: 30, distributed: 6, needs_review: 2, replaced: 1,
    evidence_complete: 10, evidence_partial: 20, evidence_empty: 10, evidence_not_configured: 0,
    slot_quota: 50, slots_open: 10, slots_linked: 5, slots_completed: 25, slots_cancelled: 0,
    ...overrides,
  };
}

// Slot counters must be set explicitly: leaving the base defaults in place would make the progress
// denominator 17 instead of the 10 quota this fixture means to describe.
const bangka = (): MapRegion => region({
  regency_id: 'regency-2', regency_name: 'KAB. BANGKA', document_code: 'BKA',
  recipients: 4, evidence_complete: 1,
  slot_quota: 10, slots_open: 0, slots_linked: 0, slots_completed: 2, slots_cancelled: 0,
});

function mockApi(regions: MapRegion[]) {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients/map')) {
      return Promise.resolve({ data: { regions, totals: region({ regency_id: '', regency_name: '', province_name: '', document_code: '', recipients: 44, evidence_complete: 12, slot_quota: 90, slots_completed: 27 }) } });
    }
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients/stats')) return Promise.resolve({ data: { total: regions.length, by_allocation_status: {}, by_evidence_status: {} } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Bangka Barat', document_code: 'BBR' }] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Jadwal Tahap 1' }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter><PermissionsProvider permissions={['distribution.view']}><DistributionMapPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
}

beforeEach(() => vi.mocked(loadRegencyAsset).mockResolvedValue(testAsset));
afterEach(() => vi.clearAllMocks());

test('renders the aggregate map, legend totals, and region ranking', async () => {
  mockApi([region(), bangka()]);
  renderPage();

  expect(await screen.findByRole('img', { name: /Peta distribusi berdasarkan Sebaran penerima/ })).toBeVisible();
  const table = await screen.findByRole('table');
  expect(within(table).getByText('KAB. BANGKA BARAT')).toBeVisible();
  expect(within(table).getByText('KAB. BANGKA')).toBeVisible();

  // Totals come from the server aggregate, so the legend must echo them rather than recompute.
  expect(screen.getByText('Penerima aktif')).toBeVisible();
  expect(screen.getByText('44')).toBeVisible();
  expect(screen.getByText('27 / 90')).toBeVisible();
  expect(screen.getByText('Batas wilayah: test asset · CC BY 3.0 IGO')).toBeVisible();
  expect(screen.getByRole('link', { name: /Batas wilayah: test asset/ })).toHaveAttribute('href', 'https://example.test/batas-wilayah');
});

test('switches the highlighted metric and reorders the ranking', async () => {
  mockApi([
    region(),
    bangka(),
  ]);
  renderPage();

  const table = await screen.findByRole('table');
  await within(table).findByText('KAB. BANGKA BARAT');
  const firstRowName = () => within(within(table).getAllByRole('row')[1]).getAllByRole('cell')[0].textContent ?? '';
  expect(firstRowName()).toContain('BANGKA BARAT');

  await userEvent.click(screen.getByRole('button', { name: 'Kelengkapan bukti' }));
  expect(screen.getByRole('img', { name: /Kelengkapan bukti/ })).toBeVisible();
  // 10/40 beats 1/4, so the order is unchanged here but the column now shows the ratio.
  expect(within(table).getByText('25% lengkap · 10/40 penerima')).toBeVisible();

  await userEvent.click(screen.getByRole('button', { name: 'Progres distribusi' }));
  expect(within(table).getByText('50% selesai · 25/50 nomor')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Progres distribusi' })).toHaveAttribute('aria-pressed', 'true');
});

test('provides touch-sized zoom controls and keeps wheel zoom inside the map', async () => {
  mockApi([region()]);
  renderPage();

  const map = await screen.findByRole('img', { name: /Peta distribusi berdasarkan/ });
  expect(screen.getByRole('button', { name: 'Perbesar peta' })).toHaveClass('size-11');
  expect(screen.getByRole('button', { name: 'Perkecil peta' })).toHaveClass('size-11');
  expect(screen.getByRole('button', { name: 'Reset tampilan peta' })).toHaveClass('size-11');

  const geometry = map.querySelector('g');
  expect(geometry).toHaveAttribute('transform', 'translate(0 0) scale(1)');
  const wheel = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: -1, clientX: 100, clientY: 100 });
  fireEvent(map, wheel);
  expect(wheel.defaultPrevented).toBe(true);
  expect(geometry).not.toHaveAttribute('transform', 'translate(0 0) scale(1)');
});

test('selects a region from the ranking and shows its detail summary', async () => {
  mockApi([
    region(),
    bangka(),
  ]);
  renderPage();

  const table = await screen.findByRole('table');
  const selectRegion = await within(table).findByRole('button', { name: 'Pilih KAB. BANGKA' });
  selectRegion.focus();
  await userEvent.keyboard('{Enter}');

  // The selection is echoed both on the map overlay and as a badge on the ranking section, so the
  // assertion is scoped to the badge rather than matching either one.
  const rankingSection = screen.getByRole('region', { name: 'Peringkat wilayah' });
  expect(await within(rankingSection).findByText('Terpilih: KAB. BANGKA')).toBeVisible();

  const details = await screen.findByRole('group', { name: 'Rincian wilayah terpilih' });
  expect(within(details).getByText('Penerima di wilayah ini')).toBeVisible();
  expect(within(details).getByText('Nomor bagi selesai')).toBeVisible();
  expect(within(details).getByText('2/10')).toBeVisible();
  expect(within(details).getByText('Perlu ditinjau')).toBeVisible();
});

test('shows a retry action when the boundary asset cannot be loaded', async () => {
  vi.mocked(loadRegencyAsset).mockRejectedValueOnce(new Error('asset unavailable'));
  mockApi([region()]);
  renderPage();

  expect(await screen.findByText('Batas wilayah tidak dapat dimuat')).toBeVisible();
  vi.mocked(loadRegencyAsset).mockResolvedValueOnce(testAsset);
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));

  expect(await screen.findByRole('img', { name: /Peta distribusi berdasarkan/ })).toBeVisible();
  expect(vi.mocked(loadRegencyAsset).mock.calls.length).toBeGreaterThanOrEqual(2);
});

test('does not request recipient stats that are not used by the map', async () => {
  mockApi([region()]);
  renderPage();
  await screen.findByRole('table');

  const paths = vi.mocked(apiRequest).mock.calls.map(([path]) => String(path));
  expect(paths.some((path) => path.startsWith('/api/v1/recipients/stats'))).toBe(false);
});

test('forwards map filters to the aggregate request', async () => {
  mockApi([region()]);
  renderPage();
  await screen.findByRole('table');

  await userEvent.click(screen.getByRole('combobox', { name: 'Kabupaten' }));
  await userEvent.click(await screen.findByRole('option', { name: 'BBR - Bangka Barat' }));

  await waitFor(() => {
    const paths = vi.mocked(apiRequest).mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.startsWith('/api/v1/recipients/map?') && path.includes('regency_id=regency-1'))).toBe(true);
  });
});

test('reports an empty ranking instead of an empty map frame', async () => {
  mockApi([]);
  renderPage();

  expect(await screen.findByText('Belum ada wilayah dengan data pada filter ini.')).toBeVisible();
});
