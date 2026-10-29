import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { PermissionsProvider } from '@/lib/permissions';
import { apiBlobRequest, apiRequest } from '@/lib/api';
import { BAIndividualPanel } from './BAIndividualPanel';
import type { DateSummary } from './types';

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return { ...actual, apiRequest: vi.fn(), apiBlobRequest: vi.fn() };
});

const bundle = { id: 'bundle-1', program_id: 'program-1', regency_id: 'regency-1', local_date: '2024-12-10', filename: 'SELASA, 10 DESEMBER 2024.pdf', recipient_count: 2, page_count: 2, version: 1, status: 'active' as const, checksum: 'abc' };
let dates: DateSummary[];

function renderPanel(programType: 'farmer' | 'fisherman' = 'farmer', permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <BAIndividualPanel programID="program-1" regencyID="regency-1" regencyName="Wajo" programType={programType} />
  </PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  dates = [
    { local_date: '2024-12-10', recipient_count: 2, validation_status: 'ready', sync_status: 'synced', bundle },
    { local_date: '2024-12-09', recipient_count: 1, validation_status: 'ready', sync_status: 'not_created' },
  ];
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/dates')) return { data: dates };
    if (path.includes('/recipients')) return { data: [
      { distribution_slot_id: 'slot-10', slot_number: 10, final_total: 50, document_number: '10/50/KSM-KKT-WJO/XII/2024', local_date: '2024-12-10' },
      { distribution_slot_id: 'slot-2', slot_number: 2, final_total: 50, document_number: '02/50/KSM-KKT-WJO/XII/2024', local_date: '2024-12-10' },
    ] };
    if (path.endsWith('/bundles/finalize') && init?.method === 'POST') {
      const { local_date } = JSON.parse(init.body as string) as { local_date: string };
      dates = dates.map((item) => item.local_date === local_date ? { ...item, sync_status: 'synced', bundle: { ...bundle, id: `bundle-${local_date}`, local_date } } : item);
      return { data: bundle };
    }
    throw new Error(`unexpected ${path}`);
  });
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }));
  vi.stubGlobal('open', vi.fn());
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
});

test('shows the synced PDF and sorts recipients numerically', async () => {
  renderPanel();
  expect(await screen.findByText('SELASA, 10 DESEMBER 2024.pdf')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Sudah sinkron' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Unduh PDF' })).toHaveAttribute('href', '/api/v1/bast/individual/bundles/bundle-1/content');
  await userEvent.click(screen.getByRole('button', { name: /Lihat 2 penerima/i }));
  const rows = await screen.findAllByRole('row');
  expect(rows[1]).toHaveTextContent('2');
  expect(rows[2]).toHaveTextContent('10');
});

test('labels a changed date as not synced and syncs it on demand', async () => {
  dates[0] = { ...dates[0], sync_status: 'out_of_sync' };
  renderPanel();
  expect(await screen.findByText(/Ada perubahan data sejak sinkron terakhir/)).toBeVisible();
  expect(screen.getAllByText('Belum sinkron').length).toBeGreaterThan(0);
  // The last synced version stays downloadable while the new content is not in Drive yet.
  expect(screen.getByRole('button', { name: 'Unduh PDF' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Kunci total kabupaten' })).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole('button', { name: 'Sinkronkan ke Drive' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/individual/bundles/finalize', expect.objectContaining({ method: 'POST', body: JSON.stringify({ program_id: 'program-1', regency_id: 'regency-1', local_date: '2024-12-10' }) })));
  expect(await screen.findByRole('button', { name: 'Sudah sinkron' })).toBeDisabled();
});

test('syncs every pending date with one click', async () => {
  dates[0] = { ...dates[0], sync_status: 'out_of_sync' };
  renderPanel();
  const summary = (await screen.findByText(/tanggal belum sinkron/)).closest('div')!;
  expect(summary).toHaveTextContent('2 tanggal belum sinkron');
  await userEvent.click(within(summary).getByRole('button', { name: 'Sinkronkan semua' }));
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.filter(([path]) => path.endsWith('/bundles/finalize'))).toHaveLength(2));
  await waitFor(() => expect(screen.queryByText(/tanggal belum sinkron/)).not.toBeInTheDocument());
});

test('previews a date that has never been synced inline', async () => {
  renderPanel();
  await screen.findByText('SELASA, 10 DESEMBER 2024.pdf');
  await userEvent.click(screen.getByRole('button', { name: /09 Desember 2024/i }));
  expect(await screen.findByText('PDF tanggal ini belum pernah disinkronkan ke Drive.')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Unduh PDF' })).not.toBeInTheDocument();
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalled());
  expect(window.open).not.toHaveBeenCalled();
  expect(await screen.findByTitle('Preview BA Perorangan')).toBeVisible();
});

test('offers to remove the old PDF of a date whose recipients all moved away', async () => {
  dates = [{ local_date: '2024-12-01', recipient_count: 0, validation_status: 'ready', sync_status: 'out_of_sync', bundle: { ...bundle, id: 'old', local_date: '2024-12-01' } }];
  renderPanel();
  expect(await screen.findByText(/Sinkronkan untuk menghapus PDF lama/)).toBeVisible();
  expect(screen.queryByTitle('Preview BA Perorangan')).not.toBeInTheDocument();
  expect(apiBlobRequest).not.toHaveBeenCalled();
});

test('lets a viewer download but not sync', async () => {
  dates[0] = { ...dates[0], sync_status: 'out_of_sync' };
  renderPanel('farmer', ['bast.view']);
  expect(await screen.findByRole('button', { name: 'Unduh PDF' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Sinkronkan ke Drive' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Sinkronkan semua' })).not.toBeInTheDocument();
});

test('keeps the fisherman variant explicitly unavailable', () => {
  renderPanel('fisherman');
  expect(screen.getByText('BA Perorangan Nelayan belum tersedia')).toBeVisible();
  expect(apiRequest).not.toHaveBeenCalled();
});
