import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../../lib/api';
import { PointDetailPanel } from './PointDetailPanel';
import type { PointFeature, PointProperties } from './types';

vi.mock('../../../lib/api', () => ({ apiRequest: vi.fn() }));

function point(overrides: Partial<PointProperties>, coordinates: [number, number] = [105.5, -2.25]): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates },
    properties: { slot_id: 'slot-1', code: '12', schedule_id: 'schedule-1', regency_id: 'regency-1', zone_id: '', site_id: 'geo:schedule-1:1', site_name: '', site_outlier: false, stage: 3, stage_at: ['2026-11-01T03:00:00Z', '2026-11-02T20:00:00Z', '2026-11-03T03:00:00Z'], location_status: 'inside', ...overrides },
  };
}

const slotDetail = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 12, status: 'completed', distribution_date: '2026-11-03',
  full_name: 'Siti Aminah', village: 'Desa Sukamaju', machine_option_code: 'KONKIT-A', machine_serial_number: 'SN-0042',
  documentation: [{ code: 'foto_serah', label: 'Foto serah terima', stage: 'penyerahan', status: 'complete', files: [{ id: 'm1', slot_id: 's', original_filename: 'serah.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/serah.jpg', storage_state: 'final' }] }],
  needs_recompletion: false, created_at: '', updated_at: '',
};

const names = new Map([['regency-1', 'KAB. BANGKA BARAT']]);

function renderWith(ui: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>);
}

function mockSlot() {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (String(path).startsWith('/api/v1/distribution/slots/search')) return Promise.resolve({ data: slotDetail });
    return Promise.reject(new Error(`Unexpected request: ${String(path)}`));
  });
}

afterEach(() => vi.clearAllMocks());

test('a selected point shows its milestones with Jakarta dates, the recipient, photos and links (R6)', async () => {
  mockSlot();
  renderWith(<PointDetailPanel selected={{ kind: 'point', slotId: 'slot-1' }} points={[point({})]} regencyNames={names} onSelect={vi.fn()} onClose={vi.fn()} />);

  expect(screen.getByRole('heading', { name: 'Nomor bagi 12' })).toBeVisible();
  expect(screen.getByText('KAB. BANGKA BARAT')).toBeVisible();
  const milestones = screen.getByRole('list', { name: 'Milestone' });
  expect(within(milestones).getByText('Diserahkan').closest('li')).toHaveTextContent('3 November 2026');
  // 20:00Z on the 2nd is already the 3rd in WIB (UTC+7).
  expect(within(milestones).getByText('Penerima tertaut').closest('li')).toHaveTextContent('3 November 2026');
  expect(within(milestones).getByText('Terjadwal').closest('li')).toHaveTextContent('1 November 2026');
  expect(within(milestones).getByText('Bukti lengkap').closest('li')).toHaveTextContent('Belum');

  expect(await screen.findByText('Siti Aminah')).toBeVisible();
  expect(screen.getByText(/SN-0042/)).toBeVisible();
  expect(screen.getByRole('img', { name: /Foto serah terima/ })).toHaveAttribute('src', '/media/serah.jpg');
  expect(vi.mocked(apiRequest)).toHaveBeenCalledWith('/api/v1/distribution/slots/search?schedule_id=schedule-1&q=12');

  expect(screen.getByRole('link', { name: 'Buka nomor bagi' })).toHaveAttribute('href', '/dokumentasi/pendistribusian');
  expect(screen.getByRole('link', { name: 'Arahkan ke lokasi' })).toHaveAttribute('href', 'https://www.google.com/maps/dir/?api=1&destination=-2.25,105.5');
  expect(screen.queryByText('Perlu ditinjau')).not.toBeInTheDocument();
});

test('a point outside its regency shows the Perlu ditinjau warning', () => {
  mockSlot();
  renderWith(<PointDetailPanel selected={{ kind: 'point', slotId: 'slot-1' }} points={[point({ location_status: 'outside' })]} regencyNames={names} onSelect={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByRole('status')).toHaveTextContent('Perlu ditinjau');
  expect(screen.getByRole('status')).toHaveTextContent('di luar batas kabupaten');
});

test('a slot detail failure keeps the milestones usable', async () => {
  vi.mocked(apiRequest).mockRejectedValue(new Error('forbidden'));
  renderWith(<PointDetailPanel selected={{ kind: 'point', slotId: 'slot-1' }} points={[point({})]} regencyNames={names} onSelect={vi.fn()} onClose={vi.fn()} />);
  expect(await screen.findByText('Data penerima tidak dapat dimuat.')).toBeVisible();
  expect(screen.getByRole('list', { name: 'Milestone' })).toBeVisible();
});

test('a selected point that the filter removed explains itself and can be closed', async () => {
  mockSlot();
  const onClose = vi.fn();
  renderWith(<PointDetailPanel selected={{ kind: 'point', slotId: 'gone' }} points={[point({})]} regencyNames={names} onSelect={vi.fn()} onClose={onClose} />);
  expect(screen.getByText(/tidak ada di hasil filter/)).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
  expect(onClose).toHaveBeenCalled();
});

test('a handover site with 4 slots lists 4 rows and picking one selects that point', async () => {
  mockSlot();
  const onSelect = vi.fn();
  const members = ['1', '2', '3', '4'].map((code) => point({ slot_id: `slot-${code}`, code, site_id: 'hl:balai', site_name: 'Balai Desa Sukamaju', stage: Number(code) }));
  members[3].properties.site_outlier = true;
  renderWith(<PointDetailPanel selected={{ kind: 'site', siteId: 'hl:balai' }} points={members} regencyNames={names} onSelect={onSelect} onClose={vi.fn()} />);

  expect(screen.getByRole('heading', { name: 'Balai Desa Sukamaju' })).toBeVisible();
  const list = screen.getByRole('list', { name: 'Daftar nomor bagi' });
  expect(within(list).getAllByRole('listitem')).toHaveLength(4);
  expect(within(list).getAllByText('Perlu ditinjau')).toHaveLength(1);
  await userEvent.click(within(list).getByRole('button', { name: /Nomor bagi 3/ }));
  expect(onSelect).toHaveBeenCalledWith({ kind: 'point', slotId: 'slot-3' });
});
