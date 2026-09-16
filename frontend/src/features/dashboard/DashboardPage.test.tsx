import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DashboardPage } from './DashboardPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function mockApi({ page = 1, pageSize = 20, total = 1 } = {}) {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients/stats')) return Promise.resolve({ data: { total: 3, by_allocation_status: { ready: 1, distributed: 1, needs_review: 1, cancelled: 0 }, by_evidence_status: { complete: 1, partial: 1, empty: 1, 'not-configured': 0 } } });
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients?')) return Promise.resolve({ data: { items: [
      { allocation_id: 'allocation-1', distribution_number: 7, allocation_status: 'ready', distribution_status: null, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier_type: 'farmer_card', sector_identifier: 'KP01', address: '', village: 'Tempe', district: 'Sabbangparu', phone_number: '', program_id: 'program-1', program_name: 'Program Petani 2026', program_type: 'farmer', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', schedule_id: 'schedule-1', schedule_name: 'Wajo Tahap 1', evidence_slots: [
        { slot_code: 'portrait', label: 'Foto penerima', is_required: true, min_files: 1, accepted_files: 1, complete: true },
        { slot_code: 'handover', label: 'BAST bertanda tangan', is_required: true, min_files: 1, accepted_files: 0, complete: false },
        { slot_code: 'package_detail', label: 'Detail paket', is_required: false, min_files: 1, accepted_files: 1, complete: true },
      ] },
    ], page, page_size: pageSize, total } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency: { name: 'Wajo' }, program: { program_type: 'farmer' } }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

function renderPage(permissions = ['recipients.view', 'recipients.manage'], initialEntries = ['/dashboard']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={initialEntries}><PermissionsProvider permissions={permissions}><DashboardPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
}

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.clearAllMocks(); });

test('renders statistics and the recipient table', async () => {
  mockApi();
  renderPage();
  expect(await screen.findByText('3')).toBeVisible();
  const table = screen.getByRole('table');
  expect(within(table).getByText('Siti Aminah')).toBeVisible();
  expect(within(table).getByText('WJO')).toBeVisible();
  expect(within(table).getByText('1/2 wajib')).toBeVisible();
  expect(within(table).getByText('Detail paket')).toBeInTheDocument();

  const headers = within(table).getAllByRole('columnheader');
  expect(headers.slice(0, 4).map((header) => header.textContent)).toEqual([
    'NO. PEMBAGIAN', 'NAMA', 'NIK', 'KELENGKAPAN EVIDENCE',
  ]);
  expect(headers[0]).toHaveClass('sticky');
  expect(headers[0]).toHaveClass('md:w-44', 'md:min-w-44', 'md:max-w-44');
  expect(headers[1]).toHaveClass('sticky');
  expect(headers[2]).toHaveClass('sticky');
  expect(table.querySelectorAll('col')[3]).toHaveClass('w-60');

  const actionCell = screen.getByRole('button', { name: 'Edit' }).closest('td');
  expect(actionCell).not.toHaveClass('flex');
});

test('searches in realtime after 350 ms and combines with filters without a submit button', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');
  expect(screen.queryByRole('button', { name: 'Cari' })).not.toBeInTheDocument();

  vi.mocked(apiRequest).mockClear();
  vi.useFakeTimers();
  fireEvent.change(screen.getByLabelText('Cari penerima'), { target: { value: 'Siti' } });
  expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.startsWith('/api/v1/recipients?'))).toBe(false);
  await act(async () => { vi.advanceTimersByTime(349); });
  expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('search=Siti'))).toBe(false);
  await act(async () => { vi.advanceTimersByTime(1); });
  expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('search=Siti') && path.includes('page=1'))).toBe(true);
  vi.useRealTimers();

  await userEvent.click(screen.getByRole('combobox', { name: 'Status alokasi' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Perlu ditinjau' }));

  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.includes('search=Siti') && path.includes('allocation_status=needs_review'));
  expect(call).toBeTruthy();
});

test('combines every dashboard filter and requests statistics with the same scope', async () => {
  mockApi();
  renderPage(undefined, ['/dashboard?search=Siti&regency_id=regency-1&program_id=program-1&schedule_id=schedule-1&district=Sabbangparu&allocation_status=needs_review&distribution_status=draft&evidence_status=partial&page=4&page_size=50&sort=full_name&direction=asc']);
  await screen.findByText('Siti Aminah');

  expect(screen.getByRole('combobox', { name: 'Kabupaten' })).toHaveTextContent('WJO - Wajo');
  expect(screen.getByRole('combobox', { name: 'Program' })).toHaveTextContent('Program Petani 2026');
  expect(screen.getByRole('combobox', { name: 'Jadwal' })).toHaveTextContent('Wajo Tahap 1');
  expect(screen.getByLabelText('Kecamatan')).toHaveValue('Sabbangparu');
  expect(screen.getByRole('combobox', { name: 'Kelengkapan evidence' })).toHaveTextContent('Sebagian');

  const statsCall = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.startsWith('/api/v1/recipients/stats?'))?.[0] as string;
  expect(statsCall).toContain('search=Siti');
  expect(statsCall).toContain('schedule_id=schedule-1');
  expect(statsCall).toContain('district=Sabbangparu');
  expect(statsCall).toContain('evidence_status=partial');
  expect(statsCall).not.toContain('page=');
  expect(statsCall).not.toContain('page_size=');
  expect(statsCall).not.toContain('sort=');
  expect(statsCall).not.toContain('direction=');

  expect(screen.getByText('Total hasil').previousElementSibling).toHaveTextContent('3');
  expect(screen.getByText('Evidence lengkap').previousElementSibling).toHaveTextContent('1');
  expect(screen.getByText('Evidence perlu dilengkapi').previousElementSibling).toHaveTextContent('2');
  expect(screen.getByRole('button', { name: 'Reset filter' })).toBeVisible();
});

test('sorts a column ascending then descending and resets pagination', async () => {
  mockApi();
  renderPage(undefined, ['/dashboard?page=4']);
  await screen.findByText('Siti Aminah');

  const sortName = screen.getByRole('button', { name: 'Urutkan Nama ascending' });
  await userEvent.click(sortName);
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('sort=full_name') && path.includes('direction=asc') && path.includes('page=1'))).toBe(true));
  expect(screen.getByRole('columnheader', { name: /NAMA/ })).toHaveAttribute('aria-sort', 'ascending');

  await userEvent.click(screen.getByRole('button', { name: 'Urutkan Nama descending' }));
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('sort=full_name') && path.includes('direction=desc'))).toBe(true));
  expect(screen.getByRole('columnheader', { name: /NAMA/ })).toHaveAttribute('aria-sort', 'descending');
});

test('keeps sticky body cells opaque while a row is hovered', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');
  const row = screen.getByText('Siti Aminah').closest('tr');
  expect(row).toHaveClass('hover:bg-muted');
  for (const cell of within(row!).getAllByRole('cell').slice(0, 3)) {
    expect(cell).toHaveClass('bg-card', 'group-hover:bg-muted');
    expect(cell).not.toHaveClass('group-hover:bg-muted/50');
  }
});

test('keeps uppercase sortable headers readable and exposes compact action icons', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');

  const table = screen.getByRole('table');
  expect(table.querySelector('thead')).toHaveClass('[&_th]:uppercase', '[&_th]:tracking-wide');
  const nameSort = within(table).getByRole('button', { name: 'Urutkan Nama ascending' });
  expect(nameSort).toHaveClass('w-full', 'justify-between', 'gap-2', 'overflow-hidden');
  expect(within(nameSort).getByText('NAMA')).toBeVisible();
  expect(nameSort.querySelector('span')).toHaveClass('truncate');
  expect(nameSort.querySelector('svg')).toHaveClass('shrink-0');

  const edit = screen.getByRole('button', { name: 'Edit' });
  const cancel = screen.getByRole('button', { name: 'Batalkan' });
  expect(edit).toHaveAttribute('title', 'Edit penerima');
  expect(cancel).toHaveAttribute('title', 'Batalkan penerima');
  expect(edit).toHaveTextContent('');
  expect(cancel).toHaveTextContent('');
});

test('changes page size and resets to the first page', async () => {
  mockApi({ page: 3, pageSize: 20, total: 240 });
  renderPage(undefined, ['/dashboard?page=3']);
  await screen.findByText('Siti Aminah');

  await userEvent.click(screen.getByRole('combobox', { name: 'Jumlah data per halaman' }));
  await userEvent.click(await screen.findByRole('option', { name: '50 per halaman' }));

  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('page_size=50') && path.includes('page=1'))).toBe(true));
});

test('renders adaptive numbered pagination for a large result', async () => {
  mockApi({ page: 6, pageSize: 20, total: 240 });
  renderPage(undefined, ['/dashboard?page=6']);
  await screen.findByText('Siti Aminah');

  const pagination = screen.getByRole('navigation', { name: 'Pagination penerima' });
  expect(within(pagination).getByRole('button', { name: 'Halaman 5' })).toBeVisible();
  expect(within(pagination).getByRole('button', { name: 'Halaman 6' })).toHaveAttribute('aria-current', 'page');
  expect(within(pagination).getByRole('button', { name: 'Halaman 7' })).toBeVisible();
  expect(within(pagination).getByText('101–120 dari 240 penerima')).toBeVisible();
});

test('hides management actions without recipients.manage', async () => {
  mockApi();
  renderPage(['recipients.view']);
  await screen.findByText('Siti Aminah');
  expect(screen.queryByRole('button', { name: 'Tambah penerima' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
});
