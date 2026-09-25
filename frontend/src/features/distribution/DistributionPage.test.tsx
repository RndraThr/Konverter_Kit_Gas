import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DistributionPage } from './DistributionPage';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

async function chooseSchedule(name: string | RegExp) {
  await userEvent.click(screen.getByRole('combobox', { name: 'Jadwal distribusi' }));
  await userEvent.click(await screen.findByRole('option', { name }));
}

const slot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'open',
  machine_option_code: '', machine_serial_number: '', hose_option_code: '', hose_serial_number: '', converter_serial_number: '',
  documentation: [], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

function renderPage(permissions = ['distribution.view', 'distribution.pos_mesin']) {
  vi.mocked(apiRequest).mockImplementation((path, _init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' }, regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
      package_template_version_id: 'pkg-1',
    }] });
    if (path === '/api/v1/program-setup/package-templates') return Promise.resolve({ data: [{
      id: 'pkg-1', template_code: 'PETANI-LPG', version: 1, name: 'Paket Petani', program_type: 'farmer', status: 'published',
      values: {
        machine_options: [{ code: 'machine-1', brand: 'SHARK', type: 'SPWP 80-30' }],
        hose_options: [{ code: 'hose-1', brand: 'TRILIUNHOSE', spec: '6m / 10m' }],
      },
    }] });
    if (path.startsWith('/api/v1/distribution/slots/search')) return Promise.resolve({ data: slot });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DistributionPage /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => vi.clearAllMocks());

test('searches for a slot by number and shows the 3 POS sections', async () => {
  renderPage();
  await chooseSchedule(/Wajo Tahap 1/);
  fireEvent.change(screen.getByLabelText('Nomor bagi atau NIK'), { target: { value: '7' } });
  fireEvent.click(screen.getByRole('button', { name: 'Cari' }));
  await waitFor(() => expect(screen.getByLabelText('POS Mesin')).toBeVisible());
  expect(screen.getByLabelText('POS Dokumen')).toBeVisible();
  expect(screen.getByLabelText('POS Penyerahan')).toBeVisible();
});

test('hides the create-slot button without distribution.pos_mesin', async () => {
  renderPage(['distribution.view']);
  await chooseSchedule(/Wajo Tahap 1/);
  expect(screen.queryByRole('button', { name: 'Buat Slot Mesin Baru' })).not.toBeInTheDocument();
});

test('loads equipment options from the schedule package template reference', async () => {
  renderPage();
  await chooseSchedule(/Wajo Tahap 1/);
  await userEvent.click(screen.getByRole('button', { name: 'Buat Slot Mesin Baru' }));
  await userEvent.click(screen.getByRole('combobox', { name: 'Merk/Tipe Mesin' }));
  expect(await screen.findByRole('option', { name: /SHARK SPWP 80-30/ })).toBeVisible();
});
