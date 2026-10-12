import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { RecipientSignature } from './RecipientSignature';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

const slot: DistributionSlot = {
  id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, distribution_date: '2026-10-20', status: 'linked', full_name: 'Siti Aminah', nik: '7306014101900001',
  documentation: [], needs_recompletion: false, created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
};

function renderSignature(value: DistributionSlot, editable = true) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const onChanged = vi.fn();
  render(<QueryClientProvider client={client}><RecipientSignature slot={value} editable={editable} onChanged={onChanged} /></QueryClientProvider>);
  return { onChanged };
}

afterEach(() => vi.clearAllMocks());

test('an unsigned slot opens the pad; saving waits for a stroke', () => {
  renderSignature(slot);
  expect(screen.getByLabelText('Area tanda tangan penerima')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Simpan tanda tangan' })).toBeDisabled();
});

test('a signed slot shows the image, versioned by signing time', () => {
  renderSignature({ ...slot, recipient_signed_at: '2026-10-20T08:00:00Z' });
  const image = screen.getByRole('img', { name: 'Tanda tangan Siti Aminah' });
  expect(image.getAttribute('src')).toBe('/api/v1/distribution/slots/7/signature?schedule_id=schedule-1&v=2026-10-20T08%3A00%3A00Z');
  expect(screen.getByRole('button', { name: 'Tanda tangan ulang' })).toBeVisible();
});

test('deleting reopens the pad', async () => {
  vi.mocked(apiRequest).mockResolvedValueOnce({ data: slot });
  const { onChanged } = renderSignature({ ...slot, recipient_signed_at: '2026-10-20T08:00:00Z' });
  await userEvent.click(screen.getByRole('button', { name: 'Hapus' }));
  await waitFor(() => expect(onChanged).toHaveBeenCalledWith(slot));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/signature?schedule_id=schedule-1', { method: 'DELETE' });
  expect(screen.getByLabelText('Area tanda tangan penerima')).toBeVisible();
});

test('read-only viewers without a signature see a note', () => {
  renderSignature(slot, false);
  expect(screen.getByText('Belum ada tanda tangan.')).toBeVisible();
});
