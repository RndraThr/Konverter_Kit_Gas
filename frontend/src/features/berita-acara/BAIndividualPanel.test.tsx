import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { PermissionsProvider } from '@/lib/permissions';
import { apiBlobRequest, apiRequest } from '@/lib/api';
import { BAIndividualPanel } from './BAIndividualPanel';

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }));

const dates = [
  { local_date: '2024-12-10', recipient_count: 2, validation_status: 'ready', bundle: { id: 'bundle-1', filename: 'SELASA, 10 DESEMBER 2024.pdf', recipient_count: 2, page_count: 2, version: 1, status: 'active', checksum: 'abc' } },
  { local_date: '2024-12-09', recipient_count: 1, validation_status: 'total_not_locked' },
];

function renderPanel(programType: 'farmer' | 'fisherman' = 'farmer', permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <BAIndividualPanel programID="program-1" regencyID="regency-1" regencyName="Wajo" programType={programType} />
  </PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/dates')) return { data: dates };
    if (path.includes('/recipients')) return { data: [
      { distribution_slot_id: 'slot-10', slot_number: 10, final_total: 50, padding: 4, document_number: '0010/50/KSM-KKT-WJO/XII/2024', local_date: '2024-12-10' },
      { distribution_slot_id: 'slot-2', slot_number: 2, final_total: 50, padding: 4, document_number: '0002/50/KSM-KKT-WJO/XII/2024', local_date: '2024-12-10' },
    ] };
    if (path.endsWith('/lock-total') && init?.method === 'POST') return { data: { final_total: 50 } };
    if (path.endsWith('/finalize') && init?.method === 'POST') return { data: { ...dates[0].bundle, id: 'bundle-new' } };
    throw new Error(`unexpected ${path}`);
  });
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }));
  vi.stubGlobal('open', vi.fn());
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
});

test('shows daily bundles and sorts recipients numerically', async () => {
  renderPanel();
  expect(await screen.findByText('SELASA, 10 DESEMBER 2024.pdf')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: /Lihat 2 penerima/i }));
  const rows = await screen.findAllByRole('row');
  expect(rows[1]).toHaveTextContent('2');
  expect(rows[2]).toHaveTextContent('10');
  expect(screen.getByText('PDF tersimpan langsung di folder 2. BA PERORANGAN tanpa subfolder tanggal.')).toBeVisible();
});

test('locks total, previews, and finalizes the selected day', async () => {
  renderPanel();
  await screen.findByText('SELASA, 10 DESEMBER 2024.pdf');
  await userEvent.click(screen.getByRole('button', { name: /09 Desember 2024/i }));
  await userEvent.click(screen.getByRole('button', { name: 'Kunci total kabupaten' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/individual/lock-total', expect.objectContaining({ method: 'POST' })));

  await userEvent.click(screen.getByRole('button', { name: 'Preview PDF' }));
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalled());
  expect(window.open).toHaveBeenCalledWith('blob:preview', '_blank', 'noopener,noreferrer');

  await userEvent.click(screen.getByRole('button', { name: 'Finalisasi & sinkronkan' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/individual/bundles/finalize', expect.objectContaining({ method: 'POST' })));
});

test('keeps the fisherman variant explicitly unavailable', () => {
  renderPanel('fisherman');
  expect(screen.getByText('BA Perorangan Nelayan belum tersedia')).toBeVisible();
  expect(apiRequest).not.toHaveBeenCalled();
});
