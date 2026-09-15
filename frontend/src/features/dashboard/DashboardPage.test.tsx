import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DashboardPage } from './DashboardPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function mockApi() {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/recipients/stats') return Promise.resolve({ data: { total: 3, by_allocation_status: { ready: 1, distributed: 1, needs_review: 1, cancelled: 0 } } });
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients?')) return Promise.resolve({ data: { items: [
      { allocation_id: 'allocation-1', distribution_number: 7, allocation_status: 'ready', distribution_status: null, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier_type: 'farmer_card', sector_identifier: 'KP01', address: '', village: 'Tempe', district: 'Sabbangparu', phone_number: '', program_id: 'program-1', program_name: 'Program Petani 2026', program_type: 'farmer', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', schedule_id: 'schedule-1', schedule_name: 'Wajo Tahap 1' },
    ], page: 1, page_size: 20, total: 1 } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency: { name: 'Wajo' }, program: { program_type: 'farmer' } }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

function renderPage(permissions = ['recipients.view', 'recipients.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter><PermissionsProvider permissions={permissions}><DashboardPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
}

afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); });

test('renders statistics and the recipient table', async () => {
  mockApi();
  renderPage();
  expect(await screen.findByText('3')).toBeVisible();
  const table = screen.getByRole('table');
  expect(within(table).getByText('Siti Aminah')).toBeVisible();
  expect(within(table).getByText('WJO')).toBeVisible();
});

test('combining search and status filter updates the query', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');
  await userEvent.type(screen.getByLabelText('Cari penerima'), 'Siti');
  await userEvent.click(screen.getByRole('button', { name: 'Cari' }));
  await userEvent.click(screen.getByRole('combobox', { name: 'Status alokasi' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Perlu ditinjau' }));

  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.includes('search=Siti') && path.includes('allocation_status=needs_review'));
  expect(call).toBeTruthy();
});

test('hides management actions without recipients.manage', async () => {
  mockApi();
  renderPage(['recipients.view']);
  await screen.findByText('Siti Aminah');
  expect(screen.queryByRole('button', { name: 'Tambah penerima' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
});
