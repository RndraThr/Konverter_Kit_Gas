import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { ApiError } from '@/lib/api';
import { uploadRequest } from '@/lib/upload';
import { uploadQueue } from '@/lib/uploadQueue';
import { UploadQueueIndicator } from './UploadQueueIndicator';

vi.mock('@/lib/upload', () => ({ uploadRequest: vi.fn() }));

function renderIndicator() {
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, 'invalidateQueries');
  render(<QueryClientProvider client={client}><UploadQueueIndicator /></QueryClientProvider>);
  return { invalidate };
}

test('stays hidden while nothing is queued', () => {
  renderIndicator();
  expect(screen.queryByRole('button', { name: /Antrian unggah/ })).not.toBeInTheDocument();
});

test('lists running and failed uploads and retries a failed one', async () => {
  vi.mocked(uploadRequest)
    .mockRejectedValueOnce(new ApiError(409, 'slot_full', 'Slot sudah penuh'))
    .mockImplementation(() => new Promise(() => undefined));
  renderIndicator();
  uploadQueue.enqueue({ target: { kind: 'distribution-slot', slotId: 'slot-1' }, label: 'Pendistribusian · KTP', fields: [], file: new File(['a'], 'ktp.jpg', { type: 'image/jpeg' }) });

  fireEvent.click(await screen.findByRole('button', { name: 'Antrian unggah: 1 unggahan gagal' }));
  expect(await screen.findByText('Pendistribusian · KTP')).toBeVisible();
  expect(screen.getByText(/Slot sudah penuh/)).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Coba unggah lagi ktp.jpg' }));
  expect(await screen.findByText('Mengunggah 0%')).toBeVisible();
  expect(uploadRequest).toHaveBeenCalledTimes(2);
});

test('refreshes the matching gallery when an upload finishes, even after leaving its page', async () => {
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-1' } });
  const { invalidate } = renderIndicator();
  uploadQueue.enqueue({ target: { kind: 'activity', programId: 'program-1', regencyId: 'regency-1', activityType: 'rakor' }, label: 'Rakor', fields: [], file: new File(['a'], 'rakor.jpg', { type: 'image/jpeg' }) });

  await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ['activities', 'rakor', 'program-1', 'regency-1'] }));
});
