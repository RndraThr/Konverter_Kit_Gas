import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { PermissionsProvider } from '@/lib/permissions';
import { apiRequest } from '../../lib/api';
import { ReportsPage } from './ReportsPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn(), ApiError: class ApiError extends Error {} }));

async function chooseOption(label: string | RegExp, optionName: string | RegExp) {
  await userEvent.click(screen.getByRole('combobox', { name: label }));
  await userEvent.click(await screen.findByRole('option', { name: optionName }));
}

const row = {
  row_number: 1, full_name: 'Siti Aminah', nik: '7306014101900001', card_number: '6032988517402660', phone_number: '081234567890',
  address: 'Jl. Poros', village: 'Tempe', district: 'Sabbangparu', distribution_date: '2026-11-05', handover_location: 'Balai Desa Tempe',
  distribution_number: 7, machine_brand: 'SHARK', machine_type: 'SPWP', machine_power: '6.5 HP', machine_fuel_type: 'Bensin',
  machine_serial: 'M-001', converter_brand: 'ERGAS', converter_serial: 'C-001', hose_serial: 'H-001',
};
const synced = { available: true, filename: 'LAPORAN DP3 - WAJO.xlsx', recipient_count: 1, synced_at: '2026-11-06T03:20:00Z', last_error: '', last_attempt_at: '2026-11-06T03:20:00Z' };

function renderPage(permissions = ['distribution.view', 'distribution.manage'], rows = [row]) {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/program-setup/schedules') {
      return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' } }] });
    }
    if (path === '/api/v1/reports/schedule/schedule-1/rows') {
      return Promise.resolve({ data: { schedule_id: 'schedule-1', program_name: 'Konkit 2026', program_type: 'farmer', fiscal_year: 2026, regency_name: 'Wajo', province_name: 'Sulawesi Selatan', rows } });
    }
    if (path === '/api/v1/reports/schedule/schedule-1/drive') {
      return Promise.resolve({ data: { available: false, filename: '', recipient_count: 0, synced_at: null, last_error: '', last_attempt_at: null } });
    }
    if (path === '/api/v1/reports/schedule/schedule-1/drive/sync') return Promise.resolve({ data: synced });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><ReportsPage /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); });

test('lists completed recipients with their data and no statuses', async () => {
  renderPage();
  expect(screen.getByRole('heading', { name: 'Laporan DP3' })).toBeVisible();
  await chooseOption('Jadwal', /Wajo Tahap 1/);

  const table = await screen.findByRole('table');
  for (const text of ['Siti Aminah', '7306014101900001', '6032988517402660', '081234567890', 'Balai Desa Tempe', 'SHARK SPWP', 'M-001']) {
    expect(within(table).getByText(text)).toBeVisible();
  }
  expect(within(table).getByRole('columnheader', { name: 'No. Kartu Tani' })).toBeVisible();
  expect(within(table).queryByText(/status/i)).not.toBeInTheDocument();
  expect(screen.queryByRole('combobox', { name: /Status/ })).not.toBeInTheDocument();
  expect(within(screen.getByLabelText('Ringkasan laporan')).getByText('Penerima selesai')).toBeVisible();
});

test('links both exports to the selected schedule', async () => {
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
  renderPage();
  await chooseOption('Jadwal', /Wajo Tahap 1/);
  await screen.findByText('Siti Aminah');

  expect(screen.getByRole('link', { name: 'Export Excel' })).toHaveAttribute('href', '/api/v1/reports/schedule/schedule-1/export.xlsx');
  expect(screen.getByRole('link', { name: 'Export PDF' })).toHaveAttribute('href', '/api/v1/reports/schedule/schedule-1/export.pdf');
  expect(consoleError.mock.calls.flat().join(' ')).not.toContain('expected a native <button>');
});

test('lets a manager refresh the Drive file, but not a viewer', async () => {
  const { unmount } = renderPage(['distribution.view']);
  await chooseOption('Jadwal', /Wajo Tahap 1/);
  expect(await screen.findByText('Dibuat otomatis setelah ada distribusi selesai.')).toBeVisible();
  expect(screen.queryByRole('button', { name: /Perbarui sekarang/ })).not.toBeInTheDocument();
  unmount();

  renderPage();
  await chooseOption('Jadwal', /Wajo Tahap 1/);
  await userEvent.click(await screen.findByRole('button', { name: /Perbarui sekarang/ }));
  expect(await screen.findByText('1 penerima · diperbarui 6 November 2026 pukul 10.20 WIB')).toBeVisible();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/reports/schedule/schedule-1/drive/sync', expect.objectContaining({ method: 'POST' }));
});

test('explains an empty report until a distribution is completed', async () => {
  renderPage(undefined, []);
  await chooseOption('Jadwal', /Wajo Tahap 1/);
  expect(await screen.findByText('Belum ada penerima yang selesai')).toBeVisible();
});
