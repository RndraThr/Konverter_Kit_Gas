import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DashboardPage } from './DashboardPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function mockApi({ page = 1, pageSize = 50, total = 1 } = {}) {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients/stats')) return Promise.resolve({ data: { total: 3, by_allocation_status: { ready: 1, distributed: 1, needs_review: 1, cancelled: 0 }, by_evidence_status: { complete: 1, partial: 1, empty: 1, 'not-configured': 0 } } });
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients?')) return Promise.resolve({ data: { items: [
      { allocation_id: 'allocation-1', distribution_number: 7, allocation_status: 'ready', distribution_status: null, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier_type: 'farmer_card', sector_identifier: 'KP01', address: 'Jalan Tani 7', village: 'Tempe', district: 'Sabbangparu', phone_number: '08123456789', program_id: 'program-1', program_name: 'Program Petani 2026', program_type: 'farmer', zone_id: 'zone-1', zone_code: 'ZONA-1', zone_name: 'Zona 1', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', schedule_id: 'schedule-1', schedule_name: 'Wajo Tahap 1', evidence_slots: [
        { slot_code: 'portrait', label: 'Foto penerima', is_required: true, min_files: 1, accepted_files: 1, complete: true },
        { slot_code: 'handover', label: 'BAST bertanda tangan', is_required: true, min_files: 1, accepted_files: 0, complete: false },
        { slot_code: 'package_detail', label: 'Detail paket', is_required: false, min_files: 1, accepted_files: 1, complete: true },
      ] },
    ], page, page_size: pageSize, total } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [{ id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }] });
    if (path === '/api/v1/program-setup/programs/program-1/zones') return Promise.resolve({ data: [{ id: 'zone-1', program_id: 'program-1', code: 'ZONA-1', name: 'Zona 1', sort_order: 1, is_placeholder: false }] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', regency: { name: 'Wajo' }, program: { program_type: 'farmer' } }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

function renderPage(permissions = ['recipients.view', 'recipients.manage'], initialEntries = ['/dashboard']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={initialEntries}><PermissionsProvider permissions={permissions}><DashboardPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
}

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.clearAllMocks(); });

const replacementFixtureRecipient = {
  distribution_number: 7, distribution_status: null, nik: '7306014101900001', sector_identifier_type: 'farmer_card',
  sector_identifier: 'KP01', address: '', village: 'Tempe', district: 'Sabbangparu', phone_number: '',
  program_id: 'program-1', program_name: 'Program Petani 2026', program_type: 'farmer' as const,
  zone_id: 'zone-1', zone_code: 'ZONA-1', zone_name: 'Zona 1',
  regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO',
  schedule_id: 'schedule-1', schedule_name: 'Wajo Tahap 1', evidence_slots: [],
};

test('offers zone filtering after a program is selected', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');

  const zoneFilter = screen.getByRole('combobox', { name: 'Zona' });
  expect(zoneFilter).toBeDisabled();
  await userEvent.click(screen.getByRole('combobox', { name: 'Program' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Program Petani 2026' }));
  await waitFor(() => expect(zoneFilter).toBeEnabled());
  await userEvent.click(zoneFilter);
  await userEvent.click(await screen.findByRole('option', { name: 'ZONA-1 - Zona 1' }));

  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('/api/v1/recipients?') && path.includes('program_id=program-1') && path.includes('zone_id=zone-1'))).toBe(true));
});

test('provides a complete mobile card list without relying on the wide table', async () => {
  mockApi();
  renderPage();

  const mobileList = await screen.findByRole('list', { name: 'Daftar penerima mobile' });
  const card = within(mobileList).getByRole('listitem', { name: 'Siti Aminah' });
  expect(within(card).getByText('Zona 1')).toBeVisible();
  expect(within(card).getByText(/Tempe, Sabbangparu/)).toBeVisible();
  expect(within(card).getByText('1/2 wajib')).toBeVisible();

  await userEvent.click(within(card).getByRole('button', { name: 'Lihat detail Siti Aminah' }));
  expect(within(card).getByText('Jalan Tani 7')).toBeVisible();
  expect(within(card).getByText('08123456789')).toBeVisible();
});

test('shows who replaced a recipient and who they replaced', async () => {
  const history = { reason: 'Penerima awal sakit', replaced_at: '2026-10-08T02:00:00Z' };
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients/stats')) return Promise.resolve({ data: { total: 2, by_allocation_status: { replaced: 1, ready: 1 }, by_evidence_status: {} } });
    if (typeof path === 'string' && path.startsWith('/api/v1/recipients?')) return Promise.resolve({ data: { items: [
      { ...replacementFixtureRecipient, allocation_id: 'allocation-old', allocation_status: 'replaced', full_name: 'Penerima Awal', replaced_by: { full_name: 'Pengganti Keluarga', ...history } },
      { ...replacementFixtureRecipient, allocation_id: 'allocation-new', allocation_status: 'ready', full_name: 'Pengganti Keluarga', replaces: { full_name: 'Penerima Awal', ...history } },
    ], page: 1, page_size: 50, total: 2 } });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderPage();

  expect(await screen.findByText('Digantikan oleh Pengganti Keluarga')).toBeVisible();
  expect(screen.getByText('Menggantikan Penerima Awal')).toBeVisible();
  expect(within(screen.getByRole('table')).getByText('Digantikan')).toBeVisible();
  // The reason and exact time stay reachable without widening the column.
  expect(screen.getByTitle(/Digantikan oleh Pengganti Keluarga/)).toHaveAttribute('title', expect.stringContaining('Penerima awal sakit'));
});

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
  expect(headers.slice(0, 6).map((header) => header.textContent)).toEqual([
    'NO. BAGI', 'IDENTITAS PENERIMA', 'WILAYAH', 'PROGRAM & JADWAL', 'DOKUMENTASI', 'STATUS',
  ]);
  expect(table).toHaveStyle({ minWidth: '1280px' });
  expect(table.querySelectorAll('col')).toHaveLength(7);

  const actionCell = within(table).getByRole('button', { name: 'Edit' }).closest('td');
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
  renderPage(undefined, ['/dashboard?search=Siti&regency_id=regency-1&program_id=program-1&zone_id=zone-1&schedule_id=schedule-1&district=Sabbangparu&allocation_status=needs_review&distribution_status=draft&evidence_status=partial&page=4&page_size=50&sort=full_name&direction=asc']);
  await screen.findByText('Siti Aminah');

  expect(screen.getByRole('combobox', { name: 'Kabupaten' })).toHaveTextContent('WJO - Wajo');
  expect(screen.getByRole('combobox', { name: 'Program' })).toHaveTextContent('Program Petani 2026');
  expect(screen.getByRole('combobox', { name: 'Zona' })).toHaveTextContent('ZONA-1 - Zona 1');
  expect(screen.getByRole('combobox', { name: 'Jadwal' })).toHaveTextContent('Wajo Tahap 1');
  expect(screen.getByLabelText('Kecamatan')).toHaveValue('Sabbangparu');
  expect(screen.getByRole('combobox', { name: 'Dokumentasi' })).toHaveTextContent('Sebagian');

  const statsCall = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.startsWith('/api/v1/recipients/stats?'))?.[0] as string;
  expect(statsCall).toContain('search=Siti');
  expect(statsCall).toContain('schedule_id=schedule-1');
  expect(statsCall).toContain('zone_id=zone-1');
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

  const sortName = screen.getByRole('button', { name: 'Urutkan Identitas penerima ascending' });
  await userEvent.click(sortName);
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('sort=full_name') && path.includes('direction=asc') && path.includes('page=1'))).toBe(true));
  expect(screen.getByRole('columnheader', { name: /IDENTITAS PENERIMA/ })).toHaveAttribute('aria-sort', 'ascending');

  await userEvent.click(screen.getByRole('button', { name: 'Urutkan Identitas penerima descending' }));
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('sort=full_name') && path.includes('direction=desc'))).toBe(true));
  expect(screen.getByRole('columnheader', { name: /IDENTITAS PENERIMA/ })).toHaveAttribute('aria-sort', 'descending');
});

test('uses separate responsive containers for mobile cards and the desktop table', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');
  expect(screen.getByRole('list', { name: 'Daftar penerima mobile' })).toHaveClass('lg:hidden');
  expect(screen.getByRole('table').parentElement?.parentElement).toHaveClass('hidden', 'lg:block');
});

test('keeps uppercase sortable headers readable and exposes compact action icons', async () => {
  mockApi();
  renderPage();
  await screen.findByText('Siti Aminah');

  const table = screen.getByRole('table');
  expect(within(table).getByRole('columnheader', { name: /IDENTITAS PENERIMA/ })).toHaveClass('uppercase', 'tracking-wide');
  const nameSort = within(table).getByRole('button', { name: 'Urutkan Identitas penerima ascending' });
  expect(nameSort).toHaveClass('w-full', 'justify-between', 'gap-2', 'overflow-hidden');
  expect(within(nameSort).getByText('IDENTITAS PENERIMA')).toBeVisible();
  expect(nameSort.querySelector('span')).toHaveClass('truncate');
  expect(nameSort.querySelector('svg')).toHaveClass('shrink-0');

  const edit = within(table).getByRole('button', { name: 'Edit' });
  const cancel = within(table).getByRole('button', { name: 'Batalkan' });
  expect(edit).toHaveAttribute('title', 'Edit penerima');
  expect(cancel).toHaveAttribute('title', 'Batalkan penerima');
  expect(edit).toHaveTextContent('');
  expect(cancel).toHaveTextContent('');
});

test('changes page size and resets to the first page', async () => {
  mockApi({ page: 3, pageSize: 50, total: 240 });
  renderPage(undefined, ['/dashboard?page=3']);
  await screen.findByText('Siti Aminah');

  const table = screen.getByRole('table');
  const pageSize = screen.getByRole('combobox', { name: 'Jumlah data per halaman' });
  expect(pageSize.compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

  await userEvent.click(pageSize);
  expect(await screen.findByRole('option', { name: '50 data' })).toBeVisible();
  expect(screen.getByRole('option', { name: '100 data' })).toBeVisible();
  expect(screen.getByRole('option', { name: 'Semua data' })).toBeVisible();
  expect(screen.queryByRole('option', { name: '20 per halaman' })).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole('option', { name: '100 data' }));

  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('page_size=100') && path.includes('page=1'))).toBe(true));
});

test('shows all matching recipients without page navigation', async () => {
  mockApi({ page: 4, pageSize: 50, total: 240 });
  renderPage(undefined, ['/dashboard?page=4']);
  await screen.findByText('Siti Aminah');

  await userEvent.click(screen.getByRole('combobox', { name: 'Jumlah data per halaman' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Semua data' }));

  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([path]) => typeof path === 'string' && path.includes('page_size=all') && path.includes('page=1'))).toBe(true));
  expect(screen.queryByRole('navigation', { name: 'Pagination penerima' })).not.toBeInTheDocument();
  expect(screen.getByText('240 penerima ditemukan')).toBeVisible();
});

test('renders adaptive numbered pagination for a large result', async () => {
  mockApi({ page: 6, pageSize: 20, total: 240 });
  renderPage(undefined, ['/dashboard?page=6']);
  await screen.findByText('Siti Aminah');

  const pagination = screen.getByRole('navigation', { name: 'Pagination penerima' });
  expect(within(pagination).getByRole('button', { name: 'Halaman 5' })).toBeVisible();
  expect(within(pagination).getByRole('button', { name: 'Halaman 6' })).toHaveAttribute('aria-current', 'page');
  expect(within(pagination).getByRole('button', { name: 'Halaman 7' })).toBeVisible();
  expect(screen.getByText('101–120 dari 240 penerima')).toBeVisible();
});

test('hides management actions without recipients.manage', async () => {
  mockApi();
  renderPage(['recipients.view']);
  await screen.findByText('Siti Aminah');
  expect(screen.queryByRole('button', { name: 'Tambah penerima' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
});
