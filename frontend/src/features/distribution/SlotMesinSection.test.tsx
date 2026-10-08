import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotMesinSection } from './SlotMesinSection';
import type { DistributionSlot, EquipmentOption } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});
vi.mock('../../lib/upload', () => ({ uploadRequest: vi.fn() }));

const machineDocumentation = {
  id: 'doc-1', code: 'machine', label: 'Foto Mesin', stage: 'mesin' as const,
  status: 'missing', required: true, min_files: 1, max_files: 2,
  media_kind: 'image' as const, input_source: 'both' as const, files: [],
};
const slot: DistributionSlot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'linked',
  distribution_date: null, documentation: [machineDocumentation], needs_recompletion: false,
  created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};
const machineOptions: EquipmentOption[] = [{ code: 'MSN-001', brand: 'SHARK', type: 'SPWP 80-30' }];

function renderSection(overrides: Partial<DistributionSlot> = {}, permissions = ['distribution.pos_mesin']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const onChanged = vi.fn();
  const view = render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotMesinSection slot={{ ...slot, ...overrides }} machineOptions={machineOptions} converterOptions={[]} hoseOptions={[]} onChanged={onChanged} /></PermissionsProvider></QueryClientProvider>);
  return { ...view, onChanged };
}

test('shows only machine documentation and keeps it manageable for POS Mesin officers', () => {
  renderSection();

  expect(screen.getByText('Foto Mesin')).toBeVisible();
  expect(screen.getByLabelText('Buka kamera')).toBeVisible();
  expect(screen.queryByLabelText('Tanggal distribusi')).not.toBeInTheDocument();
  expect(screen.queryByText('Merk/Tipe Mesin')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Serial Number Mesin')).not.toBeInTheDocument();
  expect(screen.queryByLabelText(/NIK/i)).not.toBeInTheDocument();
});

test('keeps machine documentation read-only without POS Mesin permission', () => {
  renderSection({}, ['distribution.view']);

  expect(screen.getByRole('article', { name: 'Foto Mesin' })).toBeVisible();
  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Pilih galeri')).not.toBeInTheDocument();
});

test('keeps completed slots read-only until a machine-stage revision succeeds', async () => {
  const completed = { ...slot, status: 'completed' as const, needs_recompletion: false };
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...completed, status: 'linked', needs_recompletion: true, reopened_stage: 'mesin' } });
  const { onChanged } = renderSection(completed);

  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Buka revisi POS Mesin' }));
  await userEvent.type(screen.getByLabelText('Alasan revisi'), 'Foto mesin perlu diperbaiki');
  await userEvent.click(screen.getByRole('button', { name: 'Buka revisi' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/slot-1/reopen', {
    method: 'POST', body: JSON.stringify({ stage: 'mesin', reason: 'Foto mesin perlu diperbaiki' }),
  }));
  expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ status: 'linked', reopened_stage: 'mesin' }));
  expect(await screen.findByLabelText('Buka kamera')).toBeVisible();
});
