import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotMesinSection } from './SlotMesinSection';
import type { DistributionSlot, EquipmentOption } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const slot: DistributionSlot = {
	id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'linked',
	distribution_date: '2026-10-20',
  machine_option_code: 'MSN-001', machine_serial_number: 'SN-MSN-1',
  hose_option_code: 'HSE-001', hose_serial_number: 'SN-HSE-1', converter_option_code: 'CNV-001', converter_serial_number: 'SN-CNV-1',
  documentation: [], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

const machineOptions: EquipmentOption[] = [{ code: 'MSN-001', brand: 'SHARK', type: 'SPWP 80-30' }];
const converterOptions: EquipmentOption[] = [{ code: 'CNV-001', brand: 'ERGAS' }];
const hoseOptions: EquipmentOption[] = [{ code: 'HSE-001', brand: 'TRILIUNHOSE', spec: '2 INCI' }];

function renderSection(overrides: Partial<DistributionSlot> = {}, permissions = ['*']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotMesinSection slot={{ ...slot, ...overrides }} machineOptions={machineOptions} converterOptions={converterOptions} hoseOptions={hoseOptions} onChanged={vi.fn()} /></PermissionsProvider></QueryClientProvider>);
}

const mesinPhotoDocumentation = { id: 'doc-1', code: 'machine', label: 'Foto Mesin', stage: 'mesin' as const, status: 'complete', files: [{ id: 'media-1', slot_id: 'doc-1', original_filename: 'foto.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/1' }] };

test('pre-fills the equipment fields with the slot\'s current values', () => {
  renderSection();
  expect(screen.getByText('SHARK SPWP 80-30')).toBeVisible();
  expect(screen.getByLabelText('Serial Number Mesin')).toHaveValue('SN-MSN-1');
  expect(screen.getByText('ERGAS')).toBeVisible();
  expect(screen.getByLabelText('Serial Number Konkit/Reducer')).toHaveValue('SN-CNV-1');
  expect(screen.getByText('TRILIUNHOSE 2 INCI')).toBeVisible();
});

test('keeps an unknown historical option code selectable instead of blank', () => {
  renderSection({ machine_option_code: 'MSN-LEGACY' });
  expect(screen.getByText('MSN-LEGACY')).toBeVisible();
});

test('saves equipment changes for the correct slot number', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...slot, machine_serial_number: 'SN-NEW' } });
  renderSection();

  fireEvent.change(screen.getByLabelText('Serial Number Mesin'), { target: { value: 'sn-new' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan data mesin' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith(
    '/api/v1/distribution/slots/7/equipment?schedule_id=schedule-1',
    expect.objectContaining({ method: 'PATCH' }),
  ));
  const [, init] = vi.mocked(apiRequest).mock.calls[0];
  const body = JSON.parse(init!.body as string);
  expect(body.machine_serial_number).toBe('SN-NEW');
});

test('locks equipment fields once a POS Mesin photo has been uploaded', () => {
  renderSection({ documentation: [mesinPhotoDocumentation] });
  expect(screen.getByLabelText('Serial Number Mesin')).toBeDisabled();
  expect(screen.getByLabelText('Serial Number Konkit/Reducer')).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Simpan data mesin' })).not.toBeInTheDocument();
  expect(screen.getByText('Data mesin dikunci setelah foto POS Mesin diunggah.')).toBeVisible();
});

test('leaves equipment editable when photos exist for other stages only', () => {
  renderSection({ documentation: [{ id: 'doc-2', code: 'handover', label: 'Foto Penyerahan', stage: 'penyerahan', status: 'complete', files: [{ id: 'media-2', slot_id: 'doc-2', original_filename: 'foto.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/media/2' }] }] });
  expect(screen.getByLabelText('Serial Number Mesin')).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Simpan data mesin' })).toBeVisible();
});

test('hides edit controls without distribution.pos_mesin permission', () => {
  renderSection({}, ['distribution.view']);
  expect(screen.getByLabelText('Serial Number Mesin')).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Simpan data mesin' })).not.toBeInTheDocument();
});

test('allows the distribution date to change before any photo is uploaded', () => {
	renderSection();
	expect(screen.getByLabelText('Tanggal distribusi')).toHaveValue('2026-10-20');
	expect(screen.getByRole('button', { name: 'Simpan tanggal distribusi' })).toBeEnabled();
});

test('locks the distribution date after the first photo is uploaded', () => {
	renderSection({ documentation: [mesinPhotoDocumentation] });
	expect(screen.getByLabelText('Tanggal distribusi')).toBeDisabled();
	expect(screen.getByText('Tanggal dikunci setelah foto pertama diunggah.')).toBeVisible();
});

test('offers barcode scanning for serial numbers while equipment is editable', () => {
  renderSection();
  expect(screen.getByRole('button', { name: 'Scan Serial Number Mesin' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Scan Serial Number Konkit/Reducer' })).toBeVisible();
});
