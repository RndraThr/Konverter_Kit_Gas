import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DistributionPage } from './DistributionPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const searchResult = {
  allocation_id: 'allocation-1', distribution_number: 7, full_name: 'Siti Aminah',
  masked_nik: '7306********0001', location: 'Tempe, Wajo', program_type: 'farmer',
  eligibility: 'previously_received', allocation_status: 'ready',
  documentation: [
    { id: 'slot-1', code: 'recipient_package', label: 'Penerima dan paket', status: 'complete' },
    { id: 'slot-2', code: 'signed_bast', label: 'BAST bertanda tangan', status: 'missing' },
  ],
};

const workspace = {
  allocation_id: 'allocation-1', distribution_id: 'distribution-1', schedule_id: 'schedule-1',
  distribution_number: 7, allocation_status: 'ready', distribution_status: 'draft',
  program_type: 'farmer', program_name: 'Program Petani 2026', regency_name: 'Wajo',
  full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier: 'KP01', sector_identifier_type: 'farmer_card',
  address: '', village: 'Tempe', district: 'Sabbangparu', phone_number: '',
  eligibility: 'previously_received', eligibility_reasons: ['Penerima tercatat sudah menerima paket pada program sebelumnya'],
  source_snapshot: { Nama: 'Siti Aminah', Desa: 'Tempe' },
  receipt_history: [{ completed_at: '2025-12-10T09:00:00Z', regency: 'Bone', program: 'Program Petani 2025', bast_number: '0102/BAST/BON/XII/2025' }],
  documentation: searchResult.documentation,
};

function renderPage(permissions = ['distribution.view', 'distribution.manage']) {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }, regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
    }] });
    if (path.startsWith('/api/v1/distribution/search?')) return Promise.resolve({ data: [searchResult] });
    if (path === '/api/v1/distribution/allocations/allocation-1' && !init?.method) return Promise.resolve({ data: workspace });
    if (path === '/api/v1/distribution/allocations/allocation-1/draft') return Promise.resolve({ data: { ...workspace, address: 'Jalan Sawah 10', phone_number: '0812345' } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DistributionPage /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => { vi.useRealTimers(); vi.clearAllMocks(); });

test('debounces recipient search and opens a masked eligibility result', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  vi.useFakeTimers();
  fireEvent.change(screen.getByLabelText('Jadwal distribusi'), { target: { value: 'schedule-1' } });
  fireEvent.change(screen.getByRole('combobox', { name: 'Cari penerima' }), { target: { value: 'Siti' } });
  await act(() => vi.advanceTimersByTimeAsync(299));
  expect(screen.queryByText('7306********0001')).not.toBeInTheDocument();
  await act(() => vi.advanceTimersByTimeAsync(1));
  await act(() => vi.runOnlyPendingTimersAsync());

  const result = screen.getByRole('option', { name: /Siti Aminah/ });
  expect(within(result).getByText('No. 7')).toBeVisible();
  expect(result).toHaveTextContent('7306********0001');
  expect(result).toHaveTextContent('Tempe, Wajo');
  expect(within(result).getByText('Pernah menerima')).toBeVisible();
  expect(within(result).getByLabelText('Penerima dan paket lengkap')).toBeVisible();
  expect(within(result).getByLabelText('BAST bertanda tangan belum lengkap')).toBeVisible();

  fireEvent.click(result);
  await act(() => vi.runOnlyPendingTimersAsync());
	expect(screen.queryByRole('listbox', { name: 'Hasil pencarian penerima' })).not.toBeInTheDocument();
  expect(screen.getByDisplayValue('7306014101900001')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Selesaikan distribusi' })).toBeDisabled();
  fireEvent.click(screen.getByText('Riwayat penerimaan (1)'));
  expect(screen.getByText('0102/BAST/BON/XII/2025')).toBeVisible();
});

test('saves missing recipient fields as a draft', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  vi.useFakeTimers();
  fireEvent.change(screen.getByLabelText('Jadwal distribusi'), { target: { value: 'schedule-1' } });
  fireEvent.change(screen.getByRole('combobox', { name: 'Cari penerima' }), { target: { value: 'Siti' } });
  await act(() => vi.advanceTimersByTimeAsync(300));
  await act(() => vi.runOnlyPendingTimersAsync());
  fireEvent.click(screen.getByRole('option', { name: /Siti Aminah/ }));
  await act(() => vi.runOnlyPendingTimersAsync());
  const address = screen.getByLabelText('Alamat');
  fireEvent.change(address, { target: { value: 'Jalan Sawah 10' } });
  fireEvent.change(screen.getByLabelText('Nomor telepon'), { target: { value: '0812345' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan draft' }));
  await act(() => vi.runOnlyPendingTimersAsync());
  expect(screen.getByText('Draft penerima tersimpan.')).toBeVisible();
});
