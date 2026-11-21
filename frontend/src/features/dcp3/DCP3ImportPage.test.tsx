import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest, ApiError } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DCP3ImportPage } from './DCP3ImportPage';

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>();
  return { ...actual, apiRequest: vi.fn() };
});

const preview = {
  id: 'batch-1',
  schedule_id: 'schedule-1',
  program_type: 'farmer',
  original_filename: 'dcp3-wajo.xlsx',
  sheet_name: 'Penerima',
  headers: ['Penerima', 'NIK'],
  rows: [
    { source_row_number: 2, values: { Penerima: 'Siti Aminah', NIK: '7306014101900001' } },
    { source_row_number: 3, values: { Penerima: 'Hasan', NIK: '' } },
    { source_row_number: 4, values: { Penerima: '', NIK: '7306010101800002' } },
  ],
  status: 'draft',
  machine_options: [],
};

let imports: unknown[] = [];
const oldImport = { id: 'batch-old', filename: 'DCP3 Bone.xlsx', total_rows: 3, imported_at: '2026-11-20T02:00:00Z', imported_by: 'Rendra', recipients: 3, linked: 1, distributed: 1 };
const removal = {
  import: oldImport, removed: 2, import_deleted: false,
  unlinked: [{ full_name: 'ANI', nik: '7301', slot_number: 4 }],
  kept: [{ full_name: 'CICI', nik: '7303', slot_number: 6, reason: 'sudah selesai didistribusikan' }],
};

function renderPage(permissions = ['dcp3.view', 'dcp3.import']) {
  vi.mocked(apiRequest).mockImplementation((path, _init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' },
      regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
    }] });
    if (path === '/api/v1/dcp3/previews') return Promise.resolve({ data: preview });
    if (path === '/api/v1/dcp3/imports?schedule_id=schedule-1') return Promise.resolve({ data: imports });
    if (path === '/api/v1/dcp3/imports/batch-old/removal') return Promise.resolve({ data: removal });
    if (path === '/api/v1/dcp3/imports/batch-old' && _init?.method === 'DELETE') return Promise.resolve({ data: removal });
    if (path === '/api/v1/dcp3/imports' && _init?.method === 'POST') return Promise.resolve({ data: {
      batch_id: 'batch-1', total_rows: 4, valid_rows: 1, warning_rows: 1, review_rows: 1, invalid_rows: 1,
      created_rows: 2, updated_rows: 1, kept_rows: 0, skipped_rows: 1,
      problems: [
        { source_row_number: 3, full_name: 'BUDI', nik: '7301010101010002', status: 'warning', messages: ['Distribusi penerima ini sudah berjalan, jadi datanya tidak diubah'] },
        { source_row_number: 4, full_name: 'CICI', nik: '7301010101010003', status: 'needs_review', messages: ['NIK muncul lebih dari sekali dalam batch'] },
      ],
    } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DCP3ImportPage /></PermissionsProvider></QueryClientProvider>);
}

async function chooseOption(label: string | RegExp, optionName: string) {
  const trigger = screen.getByRole('combobox', { name: label });
  await waitFor(() => expect(trigger).toBeEnabled());
  await userEvent.click(trigger);
  await userEvent.click(await screen.findByRole('option', { name: optionName }));
}

test('imports a DCP3 workbook through the four review steps', async () => {
  renderPage();
  const stepper = screen.getByRole('list', { name: 'Tahapan import DCP3' });
  expect(within(stepper).getByText('Jadwal').closest('li')).toHaveAttribute('aria-current', 'step');
  expect(within(stepper).getByText('Workbook')).toBeVisible();
  expect(within(stepper).getByText('Pemetaan')).toBeVisible();
  expect(within(stepper).getByText('Tinjau')).toBeVisible();

  await chooseOption('Jadwal distribusi', 'Wajo - Wajo Tahap 1');
  await userEvent.click(screen.getByRole('button', { name: 'Lanjut ke upload' }));
  expect(within(stepper).getByText('Workbook').closest('li')).toHaveAttribute('aria-current', 'step');
  const file = new File(['PK\x03\x04workbook'], 'dcp3-wajo.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
  await userEvent.upload(screen.getByLabelText('Pilih file DCP3'), file);
  await userEvent.click(screen.getByRole('button', { name: 'Unggah dan baca file' }));

  expect(await screen.findByText('Cocokkan kolom Excel')).toBeVisible();
  expect(within(stepper).getByText('Pemetaan').closest('li')).toHaveAttribute('aria-current', 'step');
  const continueButton = screen.getByRole('button', { name: 'Periksa data' });
  expect(continueButton).toBeDisabled();
  expect(screen.queryByRole('combobox', { name: /Nomor urut DCP3/ })).not.toBeInTheDocument();
  await chooseOption(/Nama lengkap/, 'Penerima');
  expect(continueButton).toBeEnabled();
  await chooseOption('NIK', 'NIK');
  await userEvent.click(continueButton);

  expect(within(stepper).getByText('Tinjau').closest('li')).toHaveAttribute('aria-current', 'step');
  const previewRegion = await screen.findByRole('region', { name: 'Pratinjau data DCP3' });
  const previewTable = within(previewRegion).getByRole('table', { name: 'Pratinjau data DCP3' });
  expect(within(previewTable).getByText('Valid')).toBeVisible();
  expect(within(previewTable).getByText('Peringatan')).toBeVisible();
  expect(within(previewTable).getByText('Konflik')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Import 3 data' }));

  expect(await screen.findByText('4 data selesai diproses')).toBeVisible();
  expect(screen.getByText(/^2 penerima baru, 1 diperbarui, 1 dilewati\./)).toBeVisible();
  expect(screen.getByText('1 valid')).toBeVisible();
  expect(screen.getByText('1 peringatan')).toBeVisible();
  expect(screen.getByText('1 perlu ditinjau')).toBeVisible();
  expect(screen.getByText('1 tidak valid')).toBeVisible();
  const problems = screen.getByRole('table', { name: 'Baris yang perlu perhatian' });
  expect(within(problems).getByText(/sudah berjalan/)).toBeVisible();
  expect(within(problems).getByText('Perlu ditinjau')).toBeVisible();
}, 15000);

test('lets the user pick the real header row when the workbook has leading title rows', async () => {
  let previewAttempts = 0;
  vi.mocked(apiRequest).mockImplementation((path, _init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' },
      regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
    }] });
    if (path === '/api/v1/dcp3/previews') {
      previewAttempts += 1;
      if (previewAttempts === 1) return Promise.reject(new ApiError(400, 'dcp3_headers_invalid', 'DCP3 workbook headers are invalid', { file: 'DCP3 workbook headers are invalid' }));
      return Promise.resolve({ data: preview });
    }
    if (path === '/api/v1/dcp3/raw-preview') return Promise.resolve({ data: [
      ['USULAN CALON PENERIMA BANTUAN'],
      ['KABUPATEN SUKABUMI'],
      ['No', 'Nama', 'NIK'],
    ] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['dcp3.view', 'dcp3.import']}><DCP3ImportPage /></PermissionsProvider></QueryClientProvider>);

  await chooseOption('Jadwal distribusi', 'Wajo - Wajo Tahap 1');
  await userEvent.click(screen.getByRole('button', { name: 'Lanjut ke upload' }));
  const file = new File(['PK\x03\x04workbook'], 'dcp3-sukabumi.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
  await userEvent.upload(screen.getByLabelText('Pilih file DCP3'), file);
  await userEvent.click(screen.getByRole('button', { name: 'Unggah dan baca file' }));

  expect(await screen.findByText('DCP3 workbook headers are invalid')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Lihat & pilih baris header' }));

  await screen.findByText('No | Nama | NIK');
  expect(screen.getByRole('region', { name: 'Baris awal workbook DCP3' })).toBeVisible();
  await userEvent.click(screen.getByRole('radio', { name: 'Baris 3' }));
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi dengan baris ini' }));

  expect(await screen.findByText('Cocokkan kolom Excel')).toBeVisible();
});

test('keeps DCP3 readable without showing import controls', async () => {
  renderPage(['dcp3.view']);
  expect(await screen.findByText('Data calon penerima paket perdana')).toBeVisible();
  expect(screen.queryByLabelText('Pilih file DCP3')).not.toBeInTheDocument();
  expect(screen.getByText('Akses import diperlukan untuk menambahkan data DCP3.')).toBeVisible();
});

test('a wrongly uploaded import can be removed after seeing what happens to its recipients', async () => {
  imports = [oldImport];
  renderPage();
  await chooseOption('Jadwal distribusi', 'Wajo - Wajo Tahap 1');
  expect(await screen.findByText('DCP3 Bone.xlsx')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: /Hapus import/ }));
  const dialog = await screen.findByRole('dialog');
  expect(await within(dialog).findByText(/akan dihapus/)).toHaveTextContent('2 penerima akan dihapus');
  expect(within(dialog).getByText(/#4 ANI/)).toBeVisible();
  expect(within(dialog).getByText(/CICI — sudah selesai didistribusikan/)).toBeVisible();
  await userEvent.click(within(dialog).getByRole('button', { name: /Hapus import/ }));
  await waitFor(() => expect(vi.mocked(apiRequest)).toHaveBeenCalledWith('/api/v1/dcp3/imports/batch-old', { method: 'DELETE' }));
  imports = [];
});
