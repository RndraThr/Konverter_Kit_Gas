import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { RecipientWorkspace } from './RecipientWorkspace';
import type { RecipientWorkspaceData } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const ready: RecipientWorkspaceData = {
  allocation_id: 'allocation-1', distribution_id: 'distribution-1', schedule_id: 'schedule-1',
  distribution_number: 7, allocation_status: 'ready', distribution_status: 'draft',
  program_type: 'farmer', program_name: 'Program Petani 2026', regency_name: 'Wajo',
  full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier: 'KP01',
  sector_identifier_type: 'farmer_card', address: 'Jalan Sawah', village: 'Tempe',
  district: 'Sabbangparu', phone_number: '', eligibility: 'eligible', eligibility_reasons: [],
  source_snapshot: {}, package_snapshot: {
    converter_brand: 'ERGAS',
    machine_options: [{ code: 'shark-spwp8030', brand: 'SHARK', type: 'SPWP 80-30/3"' }],
    hose_options: [{ code: 'triliunhose', brand: 'TRILIUNHOSE', spec: '6m/10m' }],
  }, receipt_history: [], documentation: [{ id: 'slot-1', code: 'recipient_package', label: 'Penerima dan paket', status: 'complete', required: true, min_files: 1, max_files: 1, files: [] }],
};

function renderWorkspace(permissions = ['distribution.manage']) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const onSaved = vi.fn();
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><RecipientWorkspace data={ready} onSaved={onSaved} /></PermissionsProvider></QueryClientProvider>);
  return onSaved;
}

test('confirms a complete distribution and publishes the completed workspace', async () => {
  const completed = { ...ready, allocation_status: 'distributed', distribution_status: 'completed' };
  vi.mocked(apiRequest).mockResolvedValue({ data: completed });
  const onSaved = renderWorkspace();

  fireEvent.click(screen.getByRole('button', { name: 'Selesaikan distribusi' }));
  const dialog = screen.getByRole('dialog', { name: 'Konfirmasi distribusi' });
  expect(dialog).toHaveTextContent('Siti Aminah');
  expect(dialog).toHaveTextContent('Program Petani 2026');
	expect(dialog).toHaveTextContent('ERGAS');
  expect(dialog).toHaveTextContent('Penerima dan paket');
  fireEvent.click(screen.getByRole('button', { name: 'Konfirmasi penyerahan' }));

  expect(await screen.findByText('Distribusi berhasil diselesaikan.')).toBeVisible();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/allocations/allocation-1/complete', { method: 'POST' });
  expect(onSaved).toHaveBeenCalledWith(completed);
});

test('hides final distribution controls without manage permission', () => {
  renderWorkspace([]);
  expect(screen.queryByText('Konfirmasi distribusi')).not.toBeInTheDocument();
});

test('saves equipment fields as part of the recipient draft', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...ready, machine_option_code: 'shark-spwp8030', machine_serial_number: 'SP 06IABD 421291' } });
  renderWorkspace();

  fireEvent.change(screen.getByLabelText('Merk/Tipe Mesin'), { target: { value: 'shark-spwp8030' } });
  fireEvent.change(screen.getByLabelText('Serial Number Mesin'), { target: { value: 'SP 06IABD 421291' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan draft' }));

  expect(await screen.findByText('Draft penerima tersimpan.')).toBeVisible();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/allocations/allocation-1/draft', expect.objectContaining({
    method: 'PATCH',
    body: expect.stringContaining('"machine_serial_number":"SP 06IABD 421291"'),
  }));
});
