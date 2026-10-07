import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ApiError, apiRequest } from '../../lib/api';
import { RevisionDialog } from './RevisionDialog';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

const slot = { id: 'slot-1', schedule_id: 'schedule-1', slot_number: 7, status: 'completed', distribution_date: '2026-10-03', documentation: [], needs_recompletion: false, created_at: '', updated_at: '' } satisfies DistributionSlot;

test('requires a meaningful reason before reopening the requested stage', async () => {
  const onReopened = vi.fn();
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...slot, status: 'linked', reopened_stage: 'dokumen' } });
  render(<QueryClientProvider client={client}><RevisionDialog slot={slot} stage="dokumen" open onOpenChange={vi.fn()} onReopened={onReopened} /></QueryClientProvider>);

  const submit = screen.getByRole('button', { name: 'Buka revisi' });
  await userEvent.type(screen.getByLabelText('Alasan revisi'), '   ');
  expect(submit).toBeDisabled();

  await userEvent.clear(screen.getByLabelText('Alasan revisi'));
  await userEvent.type(screen.getByLabelText('Alasan revisi'), 'Nomor seri perlu dikoreksi');
  await userEvent.click(submit);

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/slot-1/reopen', {
    method: 'POST',
    body: JSON.stringify({ stage: 'dokumen', reason: 'Nomor seri perlu dikoreksi' }),
  }));
  expect(onReopened).toHaveBeenCalledWith(expect.objectContaining({ id: 'slot-1', reopened_stage: 'dokumen' }));
});

test('shows the backend reason when reopening is rejected', async () => {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  vi.mocked(apiRequest).mockRejectedValue(new ApiError(403, 'forbidden', 'Anda tidak memiliki akses POS Dokumen'));
  render(<QueryClientProvider client={client}><RevisionDialog slot={slot} stage="dokumen" open onOpenChange={vi.fn()} onReopened={vi.fn()} /></QueryClientProvider>);

  await userEvent.type(screen.getByLabelText('Alasan revisi'), 'Perbaiki data');
  await userEvent.click(screen.getByRole('button', { name: 'Buka revisi' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('Anda tidak memiliki akses POS Dokumen');
});
