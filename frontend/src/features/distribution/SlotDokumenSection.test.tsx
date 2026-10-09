import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotDokumenSection } from './SlotDokumenSection';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

// The camera scanner needs a real camera stream; the tests only care that opening it for a given
// serial field routes the decoded text back into that same field.
vi.mock('./BarcodeScanner', () => ({
  default: ({ onResult }: { onResult: (text: string) => void }) => <button type="button" onClick={() => onResult('sn-scanned-123')}>kirim hasil scan</button>,
}));

const openSlot: DistributionSlot = {
	id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, distribution_date: null, status: 'open',
  machine_option_code: 'MSN-001', machine_serial_number: 'SN-MSN-1', hose_option_code: 'HSE-001', hose_serial_number: 'SN-HSE-1', converter_serial_number: 'SN-CNV-1',
  documentation: [], needs_recompletion: false, created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

const candidate = {
  allocation_id: 'allocation-1', full_name: 'Siti Aminah', nik: '7306014101900001',
  sector_identifier: 'KP01', sector_identifier_type: 'farmer_card',
  address: 'Jalan Sawah 10', village: 'Tempe', district: 'Sabbangparu', phone_number: '0812345',
  program_type: 'farmer' as const,
};

function renderSection(slot: DistributionSlot, permissions = ['distribution.pos_dokumen']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const onChanged = vi.fn();
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotDokumenSection slot={slot} machineOptions={[{ code: 'MSN-001', brand: 'SHARK', type: 'SPWP' }]} converterOptions={[{ code: 'CNV-001', brand: 'ERGAS' }]} hoseOptions={[{ code: 'HSE-001', brand: 'TRILIUN' }]} onChanged={onChanged} /></PermissionsProvider></QueryClientProvider>);
  return { onChanged };
}

afterEach(() => vi.clearAllMocks());

test('shows the NIK lookup form when open and permitted', () => {
  renderSection(openSlot);
  expect(screen.getByLabelText('POS Dokumen')).toBeVisible();
  expect(screen.getByText('Hubungkan penerima')).toBeVisible();
  expect(screen.getByLabelText('NIK Penerima')).toBeVisible();
  expect(screen.getByText('Ketik minimal 4 digit NIK.')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Cari di DCP3' })).not.toBeInTheDocument();
});

test('shows a locked placeholder when open without permission', () => {
  renderSection(openSlot, []);
  expect(screen.getByText('Menunggu penerima')).toBeVisible();
  expect(screen.queryByLabelText('NIK Penerima')).not.toBeInTheDocument();
});

test('shows matching recipients while the NIK is being typed and selects one from the dropdown', async () => {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/distribution/candidate-suggestions?schedule_id=schedule-1&nik_prefix=7306') {
      return Promise.resolve({ data: [candidate] });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderSection(openSlot);

  fireEvent.change(screen.getByLabelText('NIK Penerima'), { target: { value: '7306' } });

  const option = await screen.findByRole('option', { name: '7306014101900001 Siti Aminah' });
  fireEvent.click(option);

  expect(screen.getByLabelText('NIK Penerima')).toHaveValue('7306014101900001');
  expect(screen.getByDisplayValue('Siti Aminah')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Cari di DCP3' })).not.toBeInTheDocument();
});

test('selects a recipient suggestion with the keyboard', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: [candidate] });
  renderSection(openSlot);
  const input = screen.getByLabelText('NIK Penerima');

  fireEvent.change(input, { target: { value: '7306' } });
  await screen.findByRole('option', { name: '7306014101900001 Siti Aminah' });
  fireEvent.keyDown(input, { key: 'ArrowDown' });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(input).toHaveValue('7306014101900001');
  expect(screen.getByDisplayValue('Siti Aminah')).toBeVisible();
});

test('links the candidate selected from NIK suggestions', async () => {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/distribution/candidate-suggestions?schedule_id=schedule-1&nik_prefix=7306014101900001') {
      return Promise.resolve({ data: [candidate] });
    }
    if (path === '/api/v1/distribution/slots/7/link?schedule_id=schedule-1' && init?.method === 'POST') {
      return Promise.resolve({ data: { ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik } });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const { onChanged } = renderSection(openSlot);

  fireEvent.change(screen.getByLabelText('NIK Penerima'), { target: { value: '7306014101900001' } });
  fireEvent.click(await screen.findByRole('option', { name: '7306014101900001 Siti Aminah' }));

  await waitFor(() => expect(screen.getByDisplayValue('Siti Aminah')).toBeVisible());
  expect(screen.getByLabelText('Nomor kartu petani')).toHaveValue('KP01');
  expect(screen.getByLabelText('Nomor telepon')).toHaveValue('0812345');
  expect(screen.getByLabelText('Alamat')).toHaveValue('Jalan Sawah 10');

  fireEvent.change(screen.getByLabelText('Alamat'), { target: { value: 'Jl. Nelayan' } });
  fireEvent.change(screen.getByLabelText('Desa/kelurahan'), { target: { value: 'Desa Baru' } });
  fireEvent.change(screen.getByLabelText('Kecamatan'), { target: { value: 'Wajo' } });
  expect(screen.getByLabelText('Alamat')).toHaveValue('JL. NELAYAN');
  expect(screen.getByLabelText('Desa/kelurahan')).toHaveValue('DESA BARU');
  expect(screen.getByLabelText('Kecamatan')).toHaveValue('WAJO');

  fireEvent.click(screen.getByRole('button', { name: 'Hubungkan ke Nomor Bagi Ini' }));

  await waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ status: 'linked', full_name: 'Siti Aminah' })));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/link?schedule_id=schedule-1', expect.objectContaining({ method: 'POST' }));
});

test('renders a read-only recipient summary once linked', () => {
  const linkedSlot: DistributionSlot = { ...openSlot, status: 'linked', full_name: 'Siti Aminah', nik: '7306014101900001' };
  renderSection(linkedSlot);
  expect(screen.getByText('Siti Aminah')).toBeVisible();
  expect(screen.getByText('Terhubung')).toBeVisible();
  expect(screen.getByText('7306014101900001')).toBeVisible();
  expect(screen.queryByLabelText('NIK Penerima')).not.toBeInTheDocument();
});

test('saves the date before mounting a recipient and keeps both changes in the workflow', async () => {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path.includes('/date') && init?.method === 'PATCH') return Promise.resolve({ data: { ...openSlot, distribution_date: '2026-10-21' } });
    if (path.includes('candidate-suggestions')) return Promise.resolve({ data: [candidate] });
    if (path.includes('/link') && init?.method === 'POST') return Promise.resolve({ data: { ...openSlot, distribution_date: '2026-10-21', status: 'linked', full_name: candidate.full_name, nik: candidate.nik } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderSection(openSlot);

  fireEvent.change(screen.getByLabelText('Tanggal distribusi'), { target: { value: '2026-10-21' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan tanggal distribusi' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/date?schedule_id=schedule-1', { method: 'PATCH', body: JSON.stringify({ distribution_date: '2026-10-21' }) }));
  fireEvent.change(screen.getByLabelText('NIK Penerima'), { target: { value: candidate.nik } });
  fireEvent.click(await screen.findByRole('option', { name: `${candidate.nik} ${candidate.full_name}` }));
  fireEvent.click(screen.getByRole('button', { name: 'Hubungkan ke Nomor Bagi Ini' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/link?schedule_id=schedule-1', expect.objectContaining({ method: 'POST' })));
});

test('can mount a recipient first and save the date afterward', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik };
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...linked, distribution_date: '2026-10-22' } });
  renderSection(linked);

  fireEvent.change(screen.getByLabelText('Tanggal distribusi'), { target: { value: '2026-10-22' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan tanggal distribusi' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/date?schedule_id=schedule-1', { method: 'PATCH', body: JSON.stringify({ distribution_date: '2026-10-22' }) }));
});

test('edits recipient details without allowing NIK mutation and supports replacement search', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik, address: 'Alamat Lama', village: 'Tempe', district: 'Wajo', phone_number: '0812', sector_identifier: 'KP01' };
  vi.mocked(apiRequest).mockImplementation((path) => path.includes('candidate-suggestions') ? Promise.resolve({ data: [candidate] }) : Promise.resolve({ data: linked }));
  renderSection(linked);

  expect(screen.getByLabelText('NIK terpasang')).toBeDisabled();
  fireEvent.change(screen.getByLabelText('Alamat'), { target: { value: 'Alamat Baru' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan data penerima' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/recipient?schedule_id=schedule-1', expect.objectContaining({ method: 'PATCH' })));

  await userEvent.click(screen.getByRole('button', { name: 'Ganti penerima' }));
  expect(screen.getByLabelText('NIK pengganti')).toBeVisible();
});

test('keeps the replacement submit disabled until a reason is given', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik };
  vi.mocked(apiRequest).mockImplementation((path) => path.includes('candidate-suggestions') ? Promise.resolve({ data: [candidate] }) : Promise.resolve({ data: linked }));
  renderSection(linked);

  await userEvent.click(screen.getByRole('button', { name: 'Ganti penerima' }));
  fireEvent.change(screen.getByLabelText('NIK pengganti'), { target: { value: candidate.nik } });
  await userEvent.click(await screen.findByRole('option', { name: `${candidate.nik} ${candidate.full_name}` }));

  const submit = screen.getByRole('button', { name: 'Pasang penerima pengganti' });
  expect(submit).toBeDisabled();

  await userEvent.type(screen.getByLabelText('Alasan penggantian'), 'Penerima awal tidak dapat hadir');
  expect(submit).toBeEnabled();
});

test('offers recipient creation for a substitute who is not registered in this schedule', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik };
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path.includes('candidate-suggestions')) return Promise.resolve({ data: [] });
    if (path.includes('candidate-lookup')) return Promise.resolve({ data: { state: 'not_registered' } });
    return Promise.resolve({ data: { ...linked, nik: '7306014101900099', full_name: 'PENGGANTI KELUARGA' } });
  });
  renderSection(linked);

  await userEvent.click(screen.getByRole('button', { name: 'Ganti penerima' }));
  fireEvent.change(screen.getByLabelText('NIK pengganti'), { target: { value: '7306014101900099' } });

  expect(await screen.findByText('Pengganti belum terdaftar')).toBeVisible();
  await userEvent.type(screen.getByLabelText('Nama lengkap pengganti'), 'Pengganti Keluarga');
  await userEvent.type(screen.getByLabelText('Alasan penggantian'), 'Penerima awal sakit');
  await userEvent.click(screen.getByRole('button', { name: 'Pasang penerima pengganti' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith(
    '/api/v1/distribution/slots/7/replace-recipient?schedule_id=schedule-1',
    expect.objectContaining({ method: 'POST' }),
  ));
  const [, init] = vi.mocked(apiRequest).mock.calls.find(([path]) => path.includes('replace-recipient'))!;
  const body = JSON.parse(init!.body as string);
  expect(body).toMatchObject({ nik: '7306014101900099', full_name: 'PENGGANTI KELUARGA', reason: 'Penerima awal sakit' });
});

test('blocks a substitute who is registered but flagged for review instead of offering creation', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik };
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path.includes('candidate-suggestions')) return Promise.resolve({ data: [] });
    if (path.includes('candidate-lookup')) return Promise.resolve({ data: { state: 'needs_review', allocation_status: 'needs_review' } });
    return Promise.resolve({ data: linked });
  });
  renderSection(linked);

  await userEvent.click(screen.getByRole('button', { name: 'Ganti penerima' }));
  fireEvent.change(screen.getByLabelText('NIK pengganti'), { target: { value: '7306014101900088' } });

  expect(await screen.findByText(/perlu ditinjau/)).toBeVisible();
  expect(screen.queryByLabelText('Nama lengkap pengganti')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Pasang penerima pengganti' })).not.toBeInTheDocument();
});

test('shows the recorded replacement history for the slot', async () => {
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik };
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path.includes('/replacements')) {
      return Promise.resolve({ data: [{
        id: 'replacement-1', slot_number: 7, old_person_id: 'person-old', old_full_name: 'PENERIMA AWAL',
        new_person_id: 'person-new', new_full_name: 'PENGGANTI KELUARGA', origin: 'new_allocation',
        reason: 'Penerima awal sakit', replaced_by_name: 'Petugas Lapangan', replaced_at: '2026-10-08T02:00:00Z',
      }] });
    }
    return Promise.resolve({ data: linked });
  });
  renderSection(linked);

  expect(await screen.findByText('Riwayat penggantian penerima')).toBeVisible();
  expect(screen.getByText(/PENERIMA AWAL/)).toBeVisible();
  expect(screen.getByText('Alasan: Penerima awal sakit')).toBeVisible();
});

test('saves equipment and allows document media management', async () => {
  const documentation = [{ id: 'doc-1', code: 'document', label: 'Foto Dokumen', stage: 'dokumen' as const, status: 'missing', required: true, min_files: 1, max_files: 2, media_kind: 'image' as const, input_source: 'both' as const, files: [] }];
  const linked = { ...openSlot, status: 'linked' as const, full_name: candidate.full_name, nik: candidate.nik, documentation };
  vi.mocked(apiRequest).mockResolvedValue({ data: linked });
  renderSection(linked);

  await userEvent.click(screen.getByRole('combobox', { name: 'Merk/Tipe Mesin' }));
  await userEvent.click(screen.getByRole('option', { name: 'SHARK SPWP' }));
  fireEvent.change(screen.getByLabelText('Serial Number Mesin'), { target: { value: 'sn-001' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan data peralatan' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/equipment?schedule_id=schedule-1', expect.objectContaining({ method: 'PATCH' })));
  expect(screen.getByLabelText('Buka kamera')).toBeVisible();
});

test('offers barcode scanning for the supported serial numbers while equipment is editable', () => {
  renderSection({ ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik });

  expect(screen.getByRole('button', { name: 'Scan Serial Number Mesin' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Scan Serial Number Konkit/Reducer' })).toBeVisible();
  expect(screen.queryByLabelText('Serial Number Selang')).not.toBeInTheDocument();
});

test('hides barcode scanning when the document POS is read-only', () => {
  renderSection({ ...openSlot, status: 'completed', full_name: candidate.full_name, nik: candidate.nik }, ['distribution.view']);

  expect(screen.queryByRole('button', { name: 'Scan Serial Number Mesin' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Scan Serial Number Selang' })).not.toBeInTheDocument();
});

test('fills the scanned barcode into the supported serial number field that opened the scanner', async () => {
  renderSection({ ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik });

  await userEvent.click(screen.getByRole('button', { name: 'Scan Serial Number Konkit/Reducer' }));
  await userEvent.click(await screen.findByRole('button', { name: 'kirim hasil scan' }));

  expect(screen.getByLabelText('Serial Number Konkit/Reducer')).toHaveValue('SN-SCANNED-123');
  expect(screen.getByLabelText('Serial Number Mesin')).toHaveValue('SN-MSN-1');
});

test('shows relocation state, failed retry, and keeps previews visible', () => {
  const documentation = [{ id: 'doc-1', code: 'document', label: 'Foto Dokumen', stage: 'dokumen' as const, status: 'complete', required: true, min_files: 1, max_files: 2, media_kind: 'image' as const, files: [
    { id: 'media-1', slot_id: 'doc-1', original_filename: 'pending.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/1', storage_state: 'moving' as const },
    { id: 'media-2', slot_id: 'doc-1', original_filename: 'failed.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/2', storage_state: 'move_failed' as const },
  ] }];
  renderSection({ ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik, documentation });

  expect(screen.getByRole('status')).toHaveTextContent('1 media sedang dipindahkan');
  expect(screen.getByRole('alert')).toHaveTextContent('1 pemindahan media gagal');
  expect(screen.getByRole('button', { name: 'Lihat pending.jpg' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Coba pindahkan lagi failed.jpg' })).toBeVisible();
});

test('retries failed machine media from POS Dokumen', async () => {
  const failed = { id: 'media-machine-failed', slot_id: 'machine-doc', original_filename: 'mesin-gagal.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/machine-failed', storage_state: 'move_failed' as const };
  const documentation = [
    { id: 'machine-doc', code: 'machine', label: 'Foto Mesin', stage: 'mesin' as const, status: 'complete', required: true, min_files: 1, max_files: 2, media_kind: 'image' as const, files: [failed] },
    { id: 'document-doc', code: 'document', label: 'Foto Dokumen', stage: 'dokumen' as const, status: 'missing', required: true, min_files: 1, max_files: 2, media_kind: 'image' as const, files: [] },
  ];
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...failed, storage_state: 'moving', storage_last_error: '' } });
  renderSection({ ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik, documentation });

  await userEvent.click(screen.getByRole('button', { name: 'Coba pindahkan lagi mesin-gagal.jpg' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/media/media-machine-failed/retry-move', { method: 'POST' }));
  expect(screen.getByRole('status')).toHaveTextContent('1 media sedang dipindahkan');
});

test('locks completed document POS until revision succeeds and refreshes automatically', async () => {
  const documentation = [{ id: 'doc-1', code: 'document', label: 'Foto Dokumen', stage: 'dokumen' as const, status: 'missing', required: true, min_files: 1, max_files: 2, media_kind: 'image' as const, files: [] }];
  const completed = { ...openSlot, status: 'completed' as const, distribution_date: '2026-10-20', full_name: candidate.full_name, nik: candidate.nik, documentation };
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...completed, status: 'linked', needs_recompletion: true, reopened_stage: 'dokumen' } });
  renderSection(completed);

  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Buka revisi POS Dokumen' }));
  await userEvent.type(screen.getByLabelText('Alasan revisi'), 'Perbaiki dokumen');
  await userEvent.click(screen.getByRole('button', { name: 'Buka revisi' }));
  expect(await screen.findByLabelText('Buka kamera')).toBeVisible();
});
