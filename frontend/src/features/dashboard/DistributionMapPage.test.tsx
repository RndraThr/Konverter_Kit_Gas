import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DistributionMapPage } from './DistributionMapPage';
import { loadRegencyAsset } from './mapAsset';
import type { MapRegion } from './types';
import type { PointFeature, PointProperties } from './map/types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
// Keeps the page test independent of the ~440 kB bundled boundary asset.
vi.mock('./mapAsset', () => ({ loadRegencyAsset: vi.fn() }));
// jsdom has no WebGL. The canvas mock hands a recording fake map to onReady so camera requests can
// be asserted; layer/fit/timeline logic itself is covered by the pure tests.
const fakeMap = vi.hoisted(() => {
  const calls: Array<[string, unknown]> = [];
  const sources = new Set<string>();
  const layers: Array<{ id: string; type: string }> = [];
  const handlers = new Map<string, Array<() => void>>();
  const map = {
    calls,
    reset: () => { calls.length = 0; sources.clear(); layers.length = 0; handlers.clear(); },
    // Fires a map-level event the way resize() does (no originalEvent).
    emit: (type: string) => { handlers.get(type)?.forEach((fn) => fn()); },
    getStyle: () => ({ layers }),
    isStyleLoaded: () => true,
    getSource: (id: string) => (sources.has(id) ? { setData: () => undefined } : undefined),
    addSource: (id: string) => { sources.add(id); },
    getLayer: (id: string) => layers.find((layer) => layer.id === id),
    addLayer: (layer: { id: string; type: string }) => { layers.push({ id: layer.id, type: layer.type }); },
    setPaintProperty: () => undefined,
    setLayoutProperty: () => undefined,
    setLayerZoomRange: () => undefined,
    // Layer-scoped listeners (on(type, layerId, fn)) are not recorded; only map-level ones.
    on: (type: string, fn: unknown) => {
      if (typeof fn === 'function') handlers.set(type, [...(handlers.get(type) ?? []), fn as () => void]);
    },
    getCenter: () => ({ lng: 118, lat: -2.5 }),
    getZoom: () => 4,
    fitBounds: (bounds: unknown) => { calls.push(['fitBounds', bounds]); },
    flyTo: (options: unknown) => { calls.push(['flyTo', options]); },
    jumpTo: (options: unknown) => { calls.push(['jumpTo', options]); },
  };
  return map;
});
vi.mock('./map/MapCanvas', async () => {
  const { useEffect } = await import('react');
  return {
    MapCanvas: ({ onReady }: { onReady: (map: unknown) => void }) => {
      useEffect(() => { onReady(fakeMap); }, [onReady]);
      return <div data-testid="map-canvas" />;
    },
  };
});

const testAsset = {
  source: 'test asset', license: 'CC BY 3.0 IGO', licenseSource: 'example.test/batas-wilayah',
  regencies: [
    { n: 'Bangka Barat', c: [105.5, -2] as [number, number], r: [[[105, -1], [106, -1], [106, -3], [105, -3], [105, -1]]] },
    { n: 'Bangka', c: [106.5, -2] as [number, number], r: [[[106, -1], [107, -1], [107, -3], [106, -3], [106, -1]]] },
    { n: 'Indramayu', c: [108.2, -6.4] as [number, number], r: [[[107.9, -6.2], [108.5, -6.2], [108.5, -6.7], [107.9, -6.7], [107.9, -6.2]]] },
  ],
};

function region(overrides: Partial<MapRegion> = {}): MapRegion {
  return {
    regency_id: 'regency-1', regency_name: 'KAB. BANGKA BARAT', province_name: 'Bangka Belitung', document_code: 'BBR',
    recipients: 40, candidate: 2, ready: 30, distributed: 6, needs_review: 2, replaced: 1,
    evidence_complete: 10, evidence_partial: 20, evidence_empty: 10, evidence_not_configured: 0,
    slot_quota: 50, slots_open: 10, slots_linked: 5, slots_completed: 25, slots_cancelled: 0,
    zone_id: '', zone_code: '', zone_name: '',
    ...overrides,
  };
}

const bangka = (): MapRegion => region({ regency_id: 'regency-2', regency_name: 'KAB. BANGKA', document_code: 'BKA', recipients: 4 });

function point(overrides: Partial<PointProperties>): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates: [105.5, -2] },
    properties: { slot_id: 'slot-1', code: '1', schedule_id: 'schedule-1', regency_id: 'regency-1', zone_id: '', site_id: 'geo:schedule-1:1', site_name: '', site_outlier: false, stage: 1, stage_at: ['2026-11-01T03:00:00Z'], location_status: 'inside', ...overrides },
  };
}

const points = [
  point({ slot_id: 'slot-1', code: '12', stage: 3, stage_at: ['2026-11-01T03:00:00Z', '2026-11-02T03:00:00Z', '2026-11-03T03:00:00Z'] }),
  point({ slot_id: 'slot-2', code: '13', site_id: 'geo:schedule-1:2', stage: 5, location_status: 'outside', stage_at: ['2026-11-01T03:00:00Z', '2026-11-02T03:00:00Z', '2026-11-03T03:00:00Z', '2026-11-04T03:00:00Z', '2026-11-05T03:00:00Z'] }),
  point({ slot_id: 'slot-3', code: '4', regency_id: 'regency-2', site_id: 'geo:schedule-2:1', stage: 1 }),
];

function mockApi(regions: MapRegion[], features: PointFeature[] = points, gate: Promise<void> = Promise.resolve()) {
  vi.mocked(apiRequest).mockImplementation((path) => {
    const url = String(path);
    if (url.startsWith('/api/v1/recipients/map')) {
      return gate.then(() => ({ data: { regions, totals: region({ regency_id: '', recipients: 44 }) } }));
    }
    if (url.startsWith('/api/v1/map/points')) return gate.then(() => ({ data: { type: 'FeatureCollection', features, meta: { total: 5, located: features.length, unlocated: 5 - features.length, outside: 1 } } }));
    if (url.startsWith('/api/v1/map/zones')) return Promise.resolve({ data: { type: 'FeatureCollection', features: [] } });
    if (url === '/api/v1/map/config') return Promise.resolve({ data: { style_url: 'https://style.test', satellite_tiles: 'https://sat.test/{z}/{y}/{x}', hybrid_overlay_tiles: [], attribution: 'attr', api_key: '' } });
    if (url === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Bangka Barat', document_code: 'BBR' }] });
    if (url === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (url === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Jadwal Tahap 1' }] });
    return Promise.reject(new Error(`Unexpected request: ${url}`));
  });
}

function renderPage(entry = '/map') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[entry]}><PermissionsProvider permissions={['distribution.view']}><DistributionMapPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
}

const requested = () => vi.mocked(apiRequest).mock.calls.map(([path]) => String(path));

beforeEach(() => {
  fakeMap.reset();
  vi.mocked(loadRegencyAsset).mockResolvedValue(testAsset);
});
afterEach(() => vi.clearAllMocks());

test('an officer whose scope has no located point yet opens on their own regency (Review Focus 1)', async () => {
  mockApi([region({ regency_id: 'indramayu', regency_name: 'KAB. INDRAMAYU' })], []);
  renderPage();

  await waitFor(() => expect(fakeMap.calls.find(([name]) => name === 'fitBounds')?.[1]).toEqual([[107.9, -6.7], [108.5, -6.2]]));
});

test('a resize moveend before the data arrives does not cancel the opening fit', async () => {
  let open!: () => void;
  const gate = new Promise<void>((resolve) => { open = resolve; });
  mockApi([region({ regency_id: 'indramayu', regency_name: 'KAB. INDRAMAYU' })], [], gate);
  renderPage();

  await screen.findByTestId('map-canvas');
  await waitFor(() => expect(fakeMap.calls).toEqual([])); // map ready, nothing to fit yet
  // MapCanvas's ResizeObserver -> map.resize() fires moveend at the default view.
  act(() => { fakeMap.emit('moveend'); });
  open();

  await waitFor(() => expect(fakeMap.calls.find(([name]) => name === 'fitBounds')?.[1]).toEqual([[107.9, -6.7], [108.5, -6.2]]));
  expect(fakeMap.calls.some(([name]) => name === 'jumpTo')).toBe(false);
});

test('a shared link position from the URL at mount is used instead of fitting', async () => {
  mockApi([region(), bangka()]);
  renderPage('/map?c=108.1,-6.4&z=9');
  await screen.findByRole('button', { name: /M3 Diserahkan/ });

  await waitFor(() => expect(fakeMap.calls).toContainEqual(['jumpTo', { center: [108.1, -6.4], zoom: 9 }]));
  expect(fakeMap.calls.some(([name]) => name === 'fitBounds')).toBe(false);
});

test('selecting a slot number flies to it at zoom 15', async () => {
  mockApi([region(), bangka()]);
  renderPage();
  await screen.findByRole('button', { name: /M3 Diserahkan/ });

  await userEvent.type(screen.getByRole('combobox', { name: 'Cari kabupaten di Indonesia atau nomor bagi…' }), '13');
  await userEvent.click(await screen.findByRole('option', { name: /Nomor bagi 13/ }));
  await waitFor(() => expect(fakeMap.calls).toContainEqual(['flyTo', { center: [105.5, -2], zoom: 15 }]));
});

test('renders the KPI strip from the located points', async () => {
  mockApi([region(), bangka()]);
  renderPage();

  const kpis = await screen.findByRole('group', { name: 'Ringkasan titik' });
  await waitFor(() => expect(within(kpis).getByText('Titik terdata').parentElement).toHaveTextContent('5'));
  expect(within(kpis).getByText('Diserahkan').parentElement).toHaveTextContent('2');
  expect(within(kpis).getByText('Bukti lengkap').parentElement).toHaveTextContent('1');
  expect(within(kpis).getByText('BA ditandatangani').parentElement).toHaveTextContent('1');
  expect(within(kpis).getByText('Perlu ditinjau').parentElement).toHaveTextContent('1');
  expect(screen.getByTestId('map-canvas')).toBeInTheDocument();
  expect(screen.getByText('Pilih titik di peta')).toBeVisible();
});

test('the milestone filter shows counts per stage and writes the stage selection', async () => {
  mockApi([region(), bangka()]);
  renderPage();

  const m3 = await screen.findByRole('button', { name: /M3 Diserahkan: 1 titik/ });
  expect(screen.getByRole('button', { name: /M1 Terjadwal: 1 titik/ })).toBeVisible();
  await userEvent.click(m3);
  expect(m3).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('button', { name: /Semua titik/ })).toHaveAttribute('aria-pressed', 'false');
});

test('zone colour mode asks for a program when none is selected', async () => {
  mockApi([region()]);
  renderPage();

  expect(await screen.findByText('Pilih program untuk mewarnai zona.')).toBeVisible();
  expect(requested().some((path) => path.startsWith('/api/v1/map/zones'))).toBe(false);

  await userEvent.click(screen.getByRole('button', { name: 'Diserahkan' }));
  expect(screen.queryByText('Pilih program untuk mewarnai zona.')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Diserahkan' })).toHaveAttribute('aria-pressed', 'true');
});

test('progress per regency is grouped by zone with subtotals', async () => {
  mockApi([
    region({ zone_id: 'zone-a', zone_code: 'A', zone_name: 'ZONA A' }),
    bangka(),
  ]);
  renderPage('/map?program_id=program-1');

  const section = await screen.findByRole('region', { name: 'Progres milestone per kabupaten' });
  const zoneA = await within(section).findByRole('group', { name: 'ZONA A' });
  expect(within(zoneA).getByText('KAB. BANGKA BARAT')).toBeVisible();
  expect(within(zoneA).getByText('Subtotal: 2 titik')).toBeVisible();
  const none = within(section).getByRole('group', { name: 'Tanpa zona' });
  expect(within(none).getByText('KAB. BANGKA')).toBeVisible();
  expect(requested().some((path) => path === '/api/v1/map/zones?program_id=program-1')).toBe(true);
});

test('search finds Indonesian regencies from the boundary asset and slot numbers from the points', async () => {
  mockApi([region(), bangka()]);
  renderPage();
  await screen.findByRole('button', { name: /M3 Diserahkan/ });

  const search = screen.getByRole('combobox', { name: 'Cari kabupaten di Indonesia atau nomor bagi…' });
  await userEvent.type(search, 'indra');
  expect(await screen.findByRole('option', { name: /Indramayu/ })).toBeVisible();

  await userEvent.clear(search);
  await userEvent.type(search, '12');
  const option = await screen.findByRole('option', { name: /Nomor bagi 12/ });
  await userEvent.click(option);
  expect(await screen.findByText('Titik terpilih: Nomor bagi 12')).toBeVisible();
});

test('forwards only filter params to the aggregate and point requests', async () => {
  mockApi([region()]);
  renderPage('/map?regency_id=regency-1&color=progress&base=sat');
  await screen.findByRole('button', { name: /M3 Diserahkan/ });

  const paths = requested();
  expect(paths).toContain('/api/v1/recipients/map?regency_id=regency-1');
  expect(paths).toContain('/api/v1/map/points?regency_id=regency-1');
});

test('the filter controls still update the aggregate request', async () => {
  mockApi([region()]);
  renderPage();
  await screen.findByRole('button', { name: /M3 Diserahkan/ });

  await userEvent.click(screen.getByRole('combobox', { name: 'Kabupaten' }));
  await userEvent.click(await screen.findByRole('option', { name: 'BBR - Bangka Barat' }));

  await waitFor(() => expect(requested()).toContain('/api/v1/recipients/map?regency_id=regency-1'));
});

test('reports an empty progress table when no regency has data', async () => {
  mockApi([], []);
  renderPage();

  expect(await screen.findByText('Belum ada wilayah dengan data pada filter ini.')).toBeVisible();
});
