import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
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
  headers: ['Urutan', 'Penerima', 'NIK'],
  rows: [
    { source_row_number: 2, values: { Urutan: '1', Penerima: 'Siti Aminah', NIK: '7306014101900001' } },
    { source_row_number: 3, values: { Urutan: '2', Penerima: 'Hasan', NIK: '' } },
    { source_row_number: 4, values: { Urutan: '3', Penerima: '', NIK: '7306010101800002' } },
  ],
  status: 'draft',
};

function renderPage(permissions = ['dcp3.view', 'dcp3.import']) {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{
      id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z',
      program: { id: 'program-1', name: 'Program Petani 2026', program_type: 'farmer' },
      regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' },
    }] });
    if (path === '/api/v1/dcp3/previews') return Promise.resolve({ data: preview });
    if (path === '/api/v1/dcp3/imports' && init?.method === 'POST') return Promise.resolve({ data: {
      batch_id: 'batch-1', total_rows: 3, valid_rows: 1, warning_rows: 1, invalid_rows: 1,
    } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><DCP3ImportPage /></PermissionsProvider></QueryClientProvider>);
}

test('imports a DCP3 workbook through the four review steps', async () => {
  renderPage();
  expect(screen.getByText('1 Pilih jadwal')).toBeVisible();
  expect(screen.getByText('2 Upload DCP3')).toBeVisible();
  expect(screen.getByText('3 Cocokkan kolom')).toBeVisible();
  expect(screen.getByText('4 Periksa dan import')).toBeVisible();

  await screen.findByRole('option', { name: 'Wajo - Wajo Tahap 1' });
  await userEvent.selectOptions(screen.getByLabelText('Jadwal distribusi'), 'schedule-1');
  await userEvent.click(screen.getByRole('button', { name: 'Lanjut ke upload' }));
  const file = new File(['PK\x03\x04workbook'], 'dcp3-wajo.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
  await userEvent.upload(screen.getByLabelText('Pilih file DCP3'), file);
  await userEvent.click(screen.getByRole('button', { name: 'Unggah dan baca file' }));

  expect(await screen.findByText('Cocokkan kolom Excel')).toBeVisible();
  const continueButton = screen.getByRole('button', { name: 'Periksa data' });
  expect(continueButton).toBeDisabled();
  await userEvent.selectOptions(screen.getByLabelText(/Nomor urut DCP3/), 'Urutan');
  await userEvent.selectOptions(screen.getByLabelText(/Nama lengkap/), 'Penerima');
  await userEvent.selectOptions(screen.getByLabelText('NIK'), 'NIK');
  await userEvent.click(continueButton);

  const previewTable = await screen.findByRole('table');
  expect(within(previewTable).getByText('Valid')).toBeVisible();
  expect(within(previewTable).getByText('Peringatan')).toBeVisible();
  expect(within(previewTable).getByText('Konflik')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Import 3 data' }));

  expect(await screen.findByText('3 data selesai diproses')).toBeVisible();
  expect(screen.getByText('1 valid')).toBeVisible();
  expect(screen.getByText('1 peringatan')).toBeVisible();
  expect(screen.getByText('1 konflik')).toBeVisible();
});

test('lets the user pick the real header row when the workbook has leading title rows', async () => {
  let previewAttempts = 0;
  vi.mocked(apiRequest).mockImplementation((path, init) => {
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

  await screen.findByRole('option', { name: 'Wajo - Wajo Tahap 1' });
  await userEvent.selectOptions(screen.getByLabelText('Jadwal distribusi'), 'schedule-1');
  await userEvent.click(screen.getByRole('button', { name: 'Lanjut ke upload' }));
  const file = new File(['PK\x03\x04workbook'], 'dcp3-sukabumi.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
  await userEvent.upload(screen.getByLabelText('Pilih file DCP3'), file);
  await userEvent.click(screen.getByRole('button', { name: 'Unggah dan baca file' }));

  expect(await screen.findByText('DCP3 workbook headers are invalid')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Lihat & pilih baris header' }));

  await screen.findByText('No | Nama | NIK');
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
