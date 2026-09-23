import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SlotPenyerahanSection } from './SlotPenyerahanSection';
import type { DistributionSlot, SlotSummary } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

const penyerahanDoc = (status: string): SlotSummary => ({
  id: 'doc-1', code: 'handover_photo', label: 'Foto serah terima', stage: 'penyerahan', status, required: true, min_files: 1, max_files: 2,
});

const linkedSlot: DistributionSlot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'linked',
  full_name: 'Siti Aminah', nik: '7306014101900001',
  machine_option_code: 'MSN-001', machine_serial_number: 'SN-MSN-1', hose_option_code: 'HSE-001', hose_serial_number: 'SN-HSE-1', converter_serial_number: 'SN-CNV-1',
  documentation: [penyerahanDoc('complete')], created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

function renderSection(slot: DistributionSlot, permissions = ['distribution.pos_penyerahan']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const onChanged = vi.fn();
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotPenyerahanSection slot={slot} onChanged={onChanged} /></PermissionsProvider></QueryClientProvider>);
  return { onChanged };
}

afterEach(() => vi.clearAllMocks());

test('enables Selesaikan Distribusi when all required documentation is complete', () => {
  renderSection(linkedSlot);
  expect(screen.getByLabelText('POS Penyerahan')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Selesaikan Distribusi' })).toBeEnabled();
});

test('disables Selesaikan Distribusi when a required item is not complete', () => {
  renderSection({ ...linkedSlot, documentation: [penyerahanDoc('missing')] });
  expect(screen.getByRole('button', { name: 'Selesaikan Distribusi' })).toBeDisabled();
});

test('renders the locked placeholder when the slot is still open', () => {
  renderSection({ ...linkedSlot, status: 'open' });
  expect(screen.getByText('Menunggu dokumen selesai')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Selesaikan Distribusi' })).not.toBeInTheDocument();
});

test('renders the read-only completed view once completed', () => {
  renderSection({ ...linkedSlot, status: 'completed', distributed_at: '2026-09-21T08:00:00Z' });
  expect(screen.getByText('Distribusi selesai')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Selesaikan Distribusi' })).not.toBeInTheDocument();
});

test('confirms and completes the distribution through the dialog', async () => {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/distribution/slots/7/complete?schedule_id=schedule-1' && init?.method === 'POST') {
      return Promise.resolve({ data: { ...linkedSlot, status: 'completed', distributed_at: '2026-09-22T10:00:00Z' } });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const { onChanged } = renderSection(linkedSlot);

  await userEvent.click(screen.getByRole('button', { name: 'Selesaikan Distribusi' }));
  const dialog = await screen.findByLabelText('Konfirmasi distribusi');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Konfirmasi Penyerahan' }));

  await waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ status: 'completed' })));
});
