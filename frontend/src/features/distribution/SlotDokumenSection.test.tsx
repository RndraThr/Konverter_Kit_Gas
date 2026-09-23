import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotDokumenSection } from './SlotDokumenSection';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

const openSlot: DistributionSlot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'open',
  machine_option_code: 'MSN-001', machine_serial_number: 'SN-MSN-1', hose_option_code: 'HSE-001', hose_serial_number: 'SN-HSE-1', converter_serial_number: 'SN-CNV-1',
  documentation: [], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
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
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotDokumenSection slot={slot} onChanged={onChanged} /></PermissionsProvider></QueryClientProvider>);
  return { onChanged };
}

afterEach(() => vi.clearAllMocks());

test('shows the NIK lookup form when open and permitted', () => {
  renderSection(openSlot);
  expect(screen.getByLabelText('POS Dokumen')).toBeVisible();
  expect(screen.getByText('Hubungkan penerima')).toBeVisible();
  expect(screen.getByLabelText('NIK Penerima')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Cari di DCP3' })).toBeDisabled();
});

test('shows a locked placeholder when open without permission', () => {
  renderSection(openSlot, []);
  expect(screen.getByText('Menunggu penerima')).toBeVisible();
  expect(screen.queryByLabelText('NIK Penerima')).not.toBeInTheDocument();
});

test('looks up a candidate by NIK then links the slot', async () => {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/distribution/candidates?schedule_id=schedule-1&nik=7306014101900001') {
      return Promise.resolve({ data: candidate });
    }
    if (path === '/api/v1/distribution/slots/7/link?schedule_id=schedule-1' && init?.method === 'POST') {
      return Promise.resolve({ data: { ...openSlot, status: 'linked', full_name: candidate.full_name, nik: candidate.nik } });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const { onChanged } = renderSection(openSlot);

  fireEvent.change(screen.getByLabelText('NIK Penerima'), { target: { value: '7306014101900001' } });
  fireEvent.click(screen.getByRole('button', { name: 'Cari di DCP3' }));

  await waitFor(() => expect(screen.getByDisplayValue('Siti Aminah')).toBeVisible());
  expect(screen.getByLabelText('Nomor kartu petani')).toHaveValue('KP01');
  expect(screen.getByLabelText('Nomor telepon')).toHaveValue('0812345');
  expect(screen.getByLabelText('Alamat')).toHaveValue('Jalan Sawah 10');

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
