import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { ProgramSetupPage } from './ProgramSetupPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const responses: Record<string, unknown> = {
  '/api/v1/program-setup/regencies': { data: [{ id: 'reg-1', province_name: 'Sulawesi Selatan', name: 'Wajo', document_code: 'WJO', is_active: true }] },
  '/api/v1/program-setup/programs': { data: [
    { id: 'prog-1', code: 'PETANI-2026', name: 'Program Petani 2026', program_type: 'farmer', fiscal_year: 2026, status: 'active' },
    { id: 'prog-2', code: 'NELAYAN-2027', name: 'Program Nelayan 2027', program_type: 'fisherman', fiscal_year: 2027, status: 'draft' },
  ] },
  '/api/v1/program-setup/schedules': { data: [{ id: 'schedule-1', program_id: 'prog-1', regency_id: 'reg-1', package_template_version_id: 'package-1', documentation_template_version_id: 'document-1', name: 'Wajo Tahap 1', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z', status: 'active', distribution_number_padding: 4, program: { id: 'prog-1', name: 'Program Petani 2026' }, regency: { id: 'reg-1', name: 'Wajo', document_code: 'WJO' } }] },
  '/api/v1/program-setup/package-templates': { data: [{ id: 'package-1', template_code: 'PETANI-LPG', version: 1, name: 'Paket Petani LPG', program_type: 'farmer', values: {}, status: 'published' }] },
  '/api/v1/program-setup/documentation-templates': { data: [{ id: 'document-1', template_code: 'DOK-PETANI', version: 1, name: 'Foto Distribusi Petani', program_type: 'farmer', status: 'published', slots: [] }] },
};

function LocationProbe() {
  return <output aria-label="URL aktif">{useLocation().search}</output>;
}

function renderPage(permissions: string[], options: { initialEntry?: string; request?: (path: string) => Promise<unknown> } = {}) {
  vi.mocked(apiRequest).mockImplementation((path) => (options.request?.(path) ?? Promise.resolve(responses[path] ?? { data: [] })) as never);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<MemoryRouter initialEntries={[options.initialEntry ?? '/dashboard/persiapan-program']}><QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><ProgramSetupPage /><LocationProbe /></PermissionsProvider></QueryClientProvider></MemoryRouter>);
}

test('keeps the active workspace in the URL and shows live tab counts', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=schedules' });

  expect(await screen.findByRole('tab', { name: 'Jadwal' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('region', { name: 'Jadwal kabupaten' })).toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Kabupaten' })).toHaveTextContent('1'));
  expect(screen.getByRole('tab', { name: 'Program' })).toHaveTextContent('2');
  expect(screen.getByRole('tab', { name: 'Template' })).toHaveTextContent('2');

  await userEvent.click(screen.getByRole('tab', { name: 'Template' }));
  expect(screen.getByLabelText('URL aktif')).toHaveTextContent('?tab=templates');
});

test('filters program data immediately and combines search with status', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=programs' });
  expect(await screen.findByText('Program Petani 2026')).toBeVisible();
  expect(screen.getByText('Program Nelayan 2027')).toBeVisible();

  await userEvent.type(screen.getByRole('searchbox', { name: 'Cari program' }), 'nelayan');
  expect(screen.queryByText('Program Petani 2026')).not.toBeInTheDocument();
  expect(screen.getByText('Program Nelayan 2027')).toBeVisible();
  expect(screen.getByText('1 dari 2 program')).toBeVisible();

  await userEvent.click(screen.getByRole('combobox', { name: 'Filter status program' }));
  await userEvent.click(screen.getByRole('option', { name: 'Aktif' }));
  expect(await screen.findByText('Tidak ada program yang sesuai')).toBeVisible();
});

test('shows an honest loading state before data arrives', async () => {
  let resolveRegencies!: (value: unknown) => void;
  const pendingRegencies = new Promise((resolve) => { resolveRegencies = resolve; });
  renderPage(['programs.view'], { request: (path) => path.endsWith('/regencies') ? pendingRegencies : Promise.resolve(responses[path] ?? { data: [] }) });

  expect(await screen.findByText('Memuat data kabupaten')).toBeVisible();
  expect(screen.queryByText('Belum ada kabupaten')).not.toBeInTheDocument();
  resolveRegencies(responses['/api/v1/program-setup/regencies']);
  expect(await screen.findByText('Wajo')).toBeVisible();
});

test('can retry a failed workspace request without reloading the page', async () => {
  let attempts = 0;
  renderPage(['programs.view'], { request: (path) => {
    if (path.endsWith('/regencies') && attempts++ === 0) return Promise.reject(new Error('network'));
    return Promise.resolve(responses[path] ?? { data: [] });
  } });

  expect(await screen.findByText('Data kabupaten belum dapat dimuat')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(await screen.findByText('Wajo')).toBeVisible();
});

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

test('moves focus, selection, and named workspace regions with arrow keys', async () => {
  renderPage(['programs.view', 'programs.manage']);
  const regencyTab = await screen.findByRole('tab', { name: 'Kabupaten' });
  const programTab = screen.getByRole('tab', { name: 'Program' });
  const scheduleTab = screen.getByRole('tab', { name: 'Jadwal' });
  const templateTab = screen.getByRole('tab', { name: 'Template' });

  regencyTab.focus();
  expect(regencyTab).toHaveFocus();
  expect(regencyTab).toHaveAttribute('aria-selected', 'true');
  expect(await screen.findByRole('region', { name: 'Kabupaten operasional' })).toBeInTheDocument();

  await userEvent.keyboard('{ArrowRight}');
  expect(programTab).toHaveFocus();
  expect(programTab).toHaveAttribute('aria-selected', 'true');
  expect(await screen.findByRole('region', { name: 'Program bantuan' })).toBeInTheDocument();

  await userEvent.keyboard('{ArrowRight}');
  expect(scheduleTab).toHaveFocus();
  expect(scheduleTab).toHaveAttribute('aria-selected', 'true');
  expect(await screen.findByRole('region', { name: 'Jadwal kabupaten' })).toBeInTheDocument();

  await userEvent.keyboard('{ArrowRight}');
  expect(templateTab).toHaveFocus();
  expect(templateTab).toHaveAttribute('aria-selected', 'true');
  expect(await screen.findByRole('region', { name: 'Template paket' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Template dokumentasi' })).toBeInTheDocument();

  await userEvent.keyboard('{ArrowLeft}');
  expect(scheduleTab).toHaveFocus();
  expect(scheduleTab).toHaveAttribute('aria-selected', 'true');
  expect(await screen.findByRole('region', { name: 'Jadwal kabupaten' })).toBeInTheDocument();
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

test('uses Indonesian status language consistently', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=programs' });

  expect(await screen.findByText('Draf')).toBeVisible();
  await userEvent.click(screen.getByRole('tab', { name: 'Template' }));
  expect(await screen.findAllByText('Terbit')).not.toHaveLength(0);
  await userEvent.click(screen.getByRole('button', { name: 'Tambah paket' }));
  expect(screen.getByRole('combobox', { name: 'Status' })).toHaveTextContent('Draf');
});

test('shows a valid loading ellipsis in workspace counts', () => {
  renderPage(['programs.view'], { request: () => new Promise(() => undefined) });

  const regencyTab = screen.getByRole('tab', { name: 'Kabupaten' });
  expect(regencyTab).toHaveTextContent('…');
  expect(regencyTab).not.toHaveTextContent('â€¦');
});

test('groups the schedule editor into a wide, scannable workflow', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=schedules' });
  await userEvent.click(await screen.findByRole('button', { name: 'Tambah jadwal' }));

  const dialog = screen.getByRole('dialog', { name: 'Tambah jadwal' });
  expect(dialog).toHaveAttribute('data-layout', 'wide');
  expect(screen.getByRole('heading', { name: 'Identitas jadwal' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Program dan wilayah' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Template distribusi' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Periode dan pengaturan' })).toBeVisible();
});

test('opens template editors as structured workspaces', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=templates' });
  await userEvent.click(await screen.findByRole('button', { name: 'Tambah paket' }));

  const dialog = screen.getByRole('dialog', { name: 'Tambah template paket' });
  expect(dialog).toHaveAttribute('data-layout', 'workspace');
  expect(screen.getByRole('heading', { name: 'Identitas template' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Opsi mesin' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Opsi selang' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Komponen paket' })).toBeVisible();
});

test('asks before discarding unsaved dialog changes', async () => {
  renderPage(['programs.view', 'programs.manage'], { initialEntry: '/dashboard/persiapan-program?tab=programs' });
  await userEvent.click(await screen.findByRole('button', { name: 'Tambah program' }));

  await userEvent.type(screen.getByRole('textbox', { name: 'Kode program' }), 'uji');
  await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));

  expect(screen.getByRole('alertdialog', { name: 'Buang perubahan?' })).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Lanjut mengedit' }));
  expect(screen.getByRole('dialog', { name: 'Tambah program' })).toBeVisible();

  await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
  await userEvent.click(screen.getByRole('button', { name: 'Buang perubahan' }));
  expect(screen.queryByRole('dialog', { name: 'Tambah program' })).not.toBeInTheDocument();
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
  await userEvent.click(screen.getByRole('button', { name: 'Buang perubahan' }));
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
  expect(screen.getByRole('group', { name: 'Opsi mesin 1' })).toBeVisible();
  await userEvent.type(screen.getByLabelText('Merk mesin 1'), 'shark');
  await userEvent.type(screen.getByLabelText('Tipe mesin 1'), 'spwp 80-30/3"');
  expect(screen.getByLabelText('Merk mesin 1')).toHaveValue('SHARK');
  expect(screen.getByLabelText('Tipe mesin 1')).toHaveValue('SPWP 80-30/3"');

  await userEvent.click(screen.getByRole('button', { name: 'Tambah opsi selang' }));
  expect(screen.getByRole('group', { name: 'Opsi selang 1' })).toBeVisible();
  await userEvent.type(screen.getByLabelText('Merk selang 1'), 'triliunhose');
  expect(screen.getByLabelText('Merk selang 1')).toHaveValue('TRILIUNHOSE');

  await userEvent.click(screen.getByRole('button', { name: 'Tambah komponen' }));
  expect(screen.getByRole('group', { name: 'Komponen 1' })).toBeVisible();
  await userEvent.type(screen.getByLabelText('Nama komponen 1'), 'Tabung LPG 3 Kg');
  expect(screen.getByLabelText('Nama komponen 1')).toHaveValue('Tabung LPG 3 Kg');

  await userEvent.click(screen.getByRole('button', { name: 'Hapus opsi mesin 1' }));
  expect(screen.queryByLabelText('Merk mesin 1')).not.toBeInTheDocument();
});
