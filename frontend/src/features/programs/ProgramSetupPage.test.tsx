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

test('exposes the active setup workspace as a named region', async () => {
  renderPage(['programs.view', 'programs.manage']);
  expect(await screen.findByRole('region', { name: 'Kabupaten operasional' })).toBeInTheDocument();
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

test('accepts an optional supervisor name in the schedule dialog', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await userEvent.click(await screen.findByRole('tab', { name: 'Jadwal' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));

  const supervisor = screen.getByRole('textbox', { name: 'Konsultan pengawas' });
  await userEvent.type(supervisor, 'Andi Amrullah');

  expect(supervisor).toHaveValue('Andi Amrullah');
});

test('normalizes regency and program identity fields to uppercase while typing', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await screen.findByText('Wajo');
  await userEvent.click(screen.getByRole('button', { name: 'Tambah kabupaten' }));

  const province = screen.getByRole('textbox', { name: 'Provinsi' });
  const regencyName = screen.getByRole('textbox', { name: 'Kabupaten' });
  await userEvent.type(province, 'Sulawesi selatan');
  await userEvent.type(regencyName, 'Wajo');
  expect(province).toHaveValue('SULAWESI SELATAN');
  expect(regencyName).toHaveValue('WAJO');

  await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
  await userEvent.click(screen.getByRole('tab', { name: 'Program' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah program' }));

  const programName = screen.getByRole('textbox', { name: 'Nama program' });
  await userEvent.type(programName, 'Program petani 2027');
  expect(programName).toHaveValue('PROGRAM PETANI 2027');
});

test('keeps notes and supervisor name in their original case', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await screen.findByText('Wajo');
  await userEvent.click(screen.getByRole('button', { name: 'Tambah kabupaten' }));

  const notes = screen.getByRole('textbox', { name: 'Catatan' });
  await userEvent.type(notes, 'Menunggu verifikasi camat');
  expect(notes).toHaveValue('Menunggu verifikasi camat');
});

test('manages package template equipment options and components as repeatable rows', async () => {
  renderPage(['programs.view', 'programs.manage']);
  await userEvent.click(await screen.findByRole('tab', { name: 'Template' }));
  await userEvent.click(screen.getByRole('button', { name: 'Tambah paket' }));

  const converterBrand = screen.getByRole('textbox', { name: 'Merk Konkit/Reducer' });
  await userEvent.type(converterBrand, 'ergas');
  expect(converterBrand).toHaveValue('ERGAS');

  await userEvent.click(screen.getByRole('button', { name: 'Tambah opsi mesin' }));
  await userEvent.type(screen.getByLabelText('Merk mesin 1'), 'shark');
  await userEvent.type(screen.getByLabelText('Tipe mesin 1'), 'spwp 80-30/3"');
  expect(screen.getByLabelText('Merk mesin 1')).toHaveValue('SHARK');
  expect(screen.getByLabelText('Tipe mesin 1')).toHaveValue('SPWP 80-30/3"');

  await userEvent.click(screen.getByRole('button', { name: 'Tambah opsi selang' }));
  await userEvent.type(screen.getByLabelText('Merk selang 1'), 'triliunhose');
  expect(screen.getByLabelText('Merk selang 1')).toHaveValue('TRILIUNHOSE');

  await userEvent.click(screen.getByRole('button', { name: 'Tambah komponen' }));
  await userEvent.type(screen.getByLabelText('Nama komponen 1'), 'Tabung LPG 3 Kg');
  expect(screen.getByLabelText('Nama komponen 1')).toHaveValue('Tabung LPG 3 Kg');

  await userEvent.click(screen.getByRole('button', { name: 'Hapus opsi mesin 1' }));
  expect(screen.queryByLabelText('Merk mesin 1')).not.toBeInTheDocument();
});
