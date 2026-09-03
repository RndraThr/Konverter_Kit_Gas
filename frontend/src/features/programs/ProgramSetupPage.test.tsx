import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { ProgramSetupPage } from './ProgramSetupPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const responses: Record<string, unknown> = {
  '/api/v1/program-setup/regencies': { data: [{ id: 'reg-1', province_name: 'Sulawesi Selatan', name: 'Wajo', document_code: 'WJO', is_active: true }] },
  '/api/v1/program-setup/programs': { data: [{ id: 'prog-1', code: 'PETANI-2026', name: 'Program Petani 2026', program_type: 'farmer', fiscal_year: 2026, status: 'active' }] },
  '/api/v1/program-setup/schedules': { data: [{ id: 'schedule-1', program_id: 'prog-1', regency_id: 'reg-1', package_template_version_id: 'package-1', documentation_template_version_id: 'document-1', name: 'Wajo Tahap 1', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z', status: 'active', distribution_number_padding: 4, program: { id: 'prog-1', name: 'Program Petani 2026' }, regency: { id: 'reg-1', name: 'Wajo', document_code: 'WJO' } }] },
  '/api/v1/program-setup/package-templates': { data: [{ id: 'package-1', template_code: 'PETANI-LPG', version: 1, name: 'Paket Petani LPG', program_type: 'farmer', values: {}, status: 'published' }] },
  '/api/v1/program-setup/documentation-templates': { data: [{ id: 'document-1', template_code: 'DOK-PETANI', version: 1, name: 'Foto Distribusi Petani', program_type: 'farmer', status: 'published', slots: [] }] },
};

function renderPage(permissions: string[]) {
  vi.mocked(apiRequest).mockImplementation((path) => Promise.resolve(responses[path] ?? { data: [] }) as never);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><ProgramSetupPage /></PermissionsProvider></QueryClientProvider>);
}

test('shows the four program preparation workspaces', async () => {
  renderPage(['programs.view', 'programs.manage']);
  expect(await screen.findByRole('tab', { name: 'Kabupaten' })).toBeVisible();
  expect(screen.getByRole('tab', { name: 'Program' })).toBeVisible();
  expect(screen.getByRole('tab', { name: 'Jadwal' })).toBeVisible();
  expect(screen.getByRole('tab', { name: 'Template' })).toBeVisible();
  expect(await screen.findByText('Wajo')).toBeVisible();

  await userEvent.click(screen.getByRole('tab', { name: 'Program' }));
  expect(await screen.findByText('Program Petani 2026')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Tambah program' })).toBeVisible();

  await userEvent.click(screen.getByRole('tab', { name: 'Jadwal' }));
  expect(await screen.findByText('Paket Petani LPG')).toBeVisible();
  expect(screen.getByText('Foto Distribusi Petani')).toBeVisible();
});

test('keeps data readable without mutation controls', async () => {
  renderPage(['programs.view']);
  expect(await screen.findByText('Wajo')).toBeVisible();
  expect(screen.queryByRole('button', { name: /Tambah kabupaten/i })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Edit Wajo/i })).not.toBeInTheDocument();

  await userEvent.click(screen.getByRole('tab', { name: 'Program' }));
  expect(await screen.findByText('Program Petani 2026')).toBeVisible();
  expect(screen.queryByRole('button', { name: /Tambah program/i })).not.toBeInTheDocument();
});

test('normalizes the schedule name to uppercase while typing', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await userEvent.click(await screen.findByRole('tab', { name: 'Jadwal' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));

  const name = screen.getByRole('textbox', { name: 'Nama jadwal' });
  await userEvent.type(name, 'Wajo tahap 1');

  expect(name).toHaveValue('WAJO TAHAP 1');
});
