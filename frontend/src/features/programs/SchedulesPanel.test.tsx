import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SchedulesPanel } from './SchedulesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const responses: Record<string, unknown> = {
  '/api/v1/program-setup/schedules': { data: [] },
  '/api/v1/program-setup/programs': { data: [{ id: 'prog-1', code: 'PETANI-2026', name: 'Program Petani 2026', program_type: 'farmer', fiscal_year: 2026, status: 'active' }] },
  '/api/v1/program-setup/regencies': { data: [{ id: 'reg-1', province_name: 'Sulawesi Selatan', name: 'Wajo', document_code: 'WJO', is_active: true }] },
  '/api/v1/program-setup/package-templates': { data: [{ id: 'package-1', template_code: 'PETANI-LPG', version: 1, name: 'Paket Petani LPG', program_type: 'farmer', values: {}, status: 'published' }] },
  '/api/v1/program-setup/documentation-templates': { data: [{ id: 'document-1', template_code: 'DOK-PETANI', version: 1, name: 'Foto Distribusi Petani', program_type: 'farmer', status: 'published', slots: [] }] },
};

function renderPanel(onSave?: (body: Record<string, unknown>) => void) {
  vi.mocked(apiRequest).mockImplementation(((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/schedules' && init?.method === 'POST') {
      onSave?.(JSON.parse(init.body as string));
      return Promise.resolve({ data: {} });
    }
    return Promise.resolve(responses[path] ?? { data: [] });
  }) as typeof apiRequest);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.manage']}><SchedulesPanel /></PermissionsProvider></QueryClientProvider>);
}

async function fillRequiredFields() {
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));
  await userEvent.type(screen.getByRole('textbox', { name: 'Nama jadwal' }), 'Wajo Tahap 1');

  await userEvent.click(screen.getByRole('combobox', { name: 'Program' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Program Petani 2026' }));

  await userEvent.click(screen.getByRole('combobox', { name: 'Kabupaten' }));
  await userEvent.click(await screen.findByRole('option', { name: 'WJO - Wajo' }));

  await userEvent.click(screen.getByRole('combobox', { name: 'Template paket' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Paket Petani LPG v1' }));

  await userEvent.click(screen.getByRole('combobox', { name: 'Template dokumentasi' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Foto Distribusi Petani v1' }));
}

test('sends null slot_quota when the field is left empty', async () => {
  let savedBody: Record<string, unknown> | undefined;
  renderPanel((body) => { savedBody = body; });
  await fillRequiredFields();

  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));

  await waitFor(() => expect(savedBody).toBeDefined());
  expect(savedBody!.slot_quota).toBeNull();
}, 15000);

test('sends the entered slot_quota as a number', async () => {
  let savedBody: Record<string, unknown> | undefined;
  renderPanel((body) => { savedBody = body; });
  await fillRequiredFields();
  await userEvent.type(screen.getByLabelText(/Kuota slot/), '46');

  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));

  await waitFor(() => expect(savedBody).toBeDefined());
  expect(savedBody!.slot_quota).toBe(46);
}, 15000);
