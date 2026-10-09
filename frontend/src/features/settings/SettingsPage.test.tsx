import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SettingsPage } from './SettingsPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const settings = [
  { key: 'application_name', value: 'KONKIT GAS', type: 'string', description: 'Nama aplikasi', updated_by: 'user-1', updated_by_name: 'Rendra Baskoro', updated_at: '2026-10-09T09:15:00Z' },
  { key: 'organization_name', value: 'PT KIAN SANTANG MULIATAMA TBK', type: 'string', description: 'Nama organisasi', updated_by: 'user-1', updated_by_name: 'Rendra Baskoro', updated_at: '2026-10-09T09:15:00Z' },
  { key: 'timezone', value: 'Asia/Jakarta', type: 'timezone', description: 'Zona waktu', updated_by: '', updated_at: '2026-10-08T09:15:00Z' },
  { key: 'date_format', value: '02/01/2006', type: 'date_format', description: 'Format tanggal', updated_by: '', updated_at: '2026-10-08T09:15:00Z' },
  { key: 'locale', value: 'id-ID', type: 'locale', description: 'Bahasa', updated_by: '', updated_at: '2026-10-08T09:15:00Z' },
];

function renderPage(permissions = ['settings.view', 'settings.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SettingsPage /></PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiRequest).mockImplementation(async (_path, init) => {
    if (init?.method === 'PATCH') return { data: settings };
    return { data: settings };
  });
});

test('tracks dirty state, cancels edits, and saves changed keys only', async () => {
  const user = userEvent.setup();
  renderPage();

  const name = await screen.findByLabelText('Nama aplikasi');
  const save = screen.getByRole('button', { name: 'Simpan perubahan' });
  const cancel = screen.getByRole('button', { name: 'Batalkan perubahan' });
  expect(save).toBeDisabled();
  expect(cancel).toBeDisabled();

  await user.clear(name);
  await user.type(name, 'Konkit Baru');
  expect(name).toHaveValue('KONKIT BARU');
  expect(save).toBeEnabled();
  expect(cancel).toBeEnabled();

  await user.click(cancel);
  expect(name).toHaveValue('KONKIT GAS');
  expect(save).toBeDisabled();

  await user.clear(name);
  await user.type(name, 'Konkit Baru');
  await user.click(save);

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/system/settings', expect.objectContaining({
    method: 'PATCH',
    body: JSON.stringify({ values: { application_name: 'KONKIT BARU' } }),
  })));
});

test('shows live preview and latest updater identity', async () => {
  renderPage();
  expect(await screen.findByText('Pratinjau tampilan')).toBeInTheDocument();
  expect(screen.getByText('KONKIT GAS')).toBeInTheDocument();
  expect(screen.getByText(/PT KIAN SANTANG/)).toBeInTheDocument();
  expect(screen.getByText('Rendra Baskoro').parentElement).toHaveTextContent('Terakhir diperbarui oleh Rendra Baskoro');
  expect(screen.getByText(/09\/10\/2026/)).toBeInTheDocument();
});

test('keeps draft values when saving fails', async () => {
  const user = userEvent.setup();
  vi.mocked(apiRequest).mockImplementation(async (_path, init) => {
    if (init?.method === 'PATCH') throw new Error('network down');
    return { data: settings };
  });
  renderPage();

  const name = await screen.findByLabelText('Nama aplikasi');
  await user.clear(name);
  await user.type(name, 'Draft Aman');
  await user.click(screen.getByRole('button', { name: 'Simpan perubahan' }));

  expect(await screen.findByText('Pengaturan belum dapat disimpan')).toBeInTheDocument();
  expect(name).toHaveValue('DRAFT AMAN');
});

test('explains read-only access and disables editing', async () => {
  renderPage(['settings.view']);
  expect(await screen.findByLabelText('Nama aplikasi')).toBeDisabled();
  expect(screen.getByText(/memerlukan izin pengelolaan/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Simpan perubahan' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Batalkan perubahan' })).toBeDisabled();
});
