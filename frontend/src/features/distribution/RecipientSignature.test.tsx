import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiBlobRequest, apiRequest } from '../../lib/api';
import { recolorSignature } from '@/components/SignaturePad';
import { RecipientSignature } from './RecipientSignature';
import type { DistributionSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn(), apiBlobRequest: vi.fn() };
});
vi.mock('@/components/SignaturePad', async () => {
  const actual = await vi.importActual<typeof import('@/components/SignaturePad')>('@/components/SignaturePad');
  return { ...actual, recolorSignature: vi.fn() };
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

afterEach(() => { vi.clearAllMocks(); localStorage.clear(); });

test('the ink colour can be picked and is kept for the next signature', async () => {
  const first = render(<QueryClientProvider client={new QueryClient()}><RecipientSignature slot={slot} editable onChanged={vi.fn()} /></QueryClientProvider>);
  await userEvent.click(screen.getByRole('button', { name: 'Pena & warna' }));
  expect(screen.getByRole('button', { name: 'Tinta Biru tua' })).toHaveAttribute('aria-pressed', 'true');
  await userEvent.click(screen.getByRole('button', { name: 'Tinta Merah' }));
  expect(screen.getByRole('button', { name: 'Tinta Merah' })).toHaveAttribute('aria-pressed', 'true');
  expect(localStorage.getItem('konkit.signature.ink')).toBe('#d32f2f');
  first.unmount();

  renderSignature(slot);
  await userEvent.click(screen.getByRole('button', { name: 'Pena & warna' }));
  expect(screen.getByRole('button', { name: 'Tinta Merah' })).toHaveAttribute('aria-pressed', 'true');
});

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

test('a saved signature can change colour without signing again', async () => {
  const signed = { ...slot, recipient_signed_at: '2026-10-20T08:00:00Z' };
  const stored = new Blob(['png'], { type: 'image/png' });
  vi.mocked(apiBlobRequest).mockResolvedValueOnce(stored);
  vi.mocked(recolorSignature).mockResolvedValueOnce('VElOVEVE');
  vi.mocked(apiRequest).mockResolvedValueOnce({ data: signed });
  const { onChanged } = renderSignature(signed);

  await userEvent.click(screen.getByRole('button', { name: 'Ganti warna' }));
  expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/signature?schedule_id=schedule-1');
  const saveColor = await screen.findByRole('button', { name: 'Simpan warna' });
  expect(saveColor).toBeDisabled();
  await userEvent.click(screen.getByRole('button', { name: 'Tinta Biru' }));
  expect(recolorSignature).toHaveBeenCalledWith(stored, '#1e5bd8');
  await waitFor(() => expect(screen.getByRole('img', { name: 'Tanda tangan Siti Aminah' })).toHaveAttribute('src', 'data:image/png;base64,VElOVEVE'));

  await userEvent.click(saveColor);
  await waitFor(() => expect(onChanged).toHaveBeenCalledWith(signed));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/7/signature?schedule_id=schedule-1', { method: 'PUT', body: JSON.stringify({ image_png: 'VElOVEVE' }) });
  expect(screen.queryByRole('button', { name: 'Simpan warna' })).not.toBeInTheDocument();
});

test('read-only viewers cannot change the colour', () => {
  renderSignature({ ...slot, recipient_signed_at: '2026-10-20T08:00:00Z' }, false);
  expect(screen.queryByRole('button', { name: 'Ganti warna' })).not.toBeInTheDocument();
});

test('the pen type and size can be picked like on the phone and are kept', async () => {
  const first = render(<QueryClientProvider client={new QueryClient()}><RecipientSignature slot={slot} editable onChanged={vi.fn()} /></QueryClientProvider>);
  await userEvent.click(screen.getByRole('button', { name: 'Pena & warna' }));
  expect(screen.getByRole('button', { name: 'Pena tinta' })).toHaveAttribute('aria-pressed', 'true');
  await userEvent.click(screen.getByRole('button', { name: 'Kaligrafi' }));
  expect(screen.getByRole('button', { name: 'Kaligrafi' })).toHaveAttribute('aria-pressed', 'true');
  expect(localStorage.getItem('konkit.signature.pen')).toBe('kaligrafi');
  first.unmount();

  renderSignature(slot);
  await userEvent.click(screen.getByRole('button', { name: 'Pena & warna' }));
  expect(screen.getByRole('button', { name: 'Kaligrafi' })).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('slider', { name: 'Tebal pena' })).toHaveValue('5');
  expect(screen.getByRole('button', { name: 'Batalkan coretan' })).toBeDisabled();
});

test('the pen and colour sit behind one compact button so the pad stays roomy', async () => {
  renderSignature(slot);
  const tools = screen.getByRole('button', { name: 'Pena & warna' });
  expect(tools).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByRole('group', { name: 'Warna tinta' })).not.toBeInTheDocument();
  await userEvent.click(tools);
  expect(screen.getByRole('group', { name: 'Warna tinta' })).toBeVisible();
  // Starting to sign closes the panel.
  await userEvent.pointer({ keys: '[MouseLeft>]', target: screen.getByLabelText('Area tanda tangan penerima') });
  expect(screen.queryByRole('group', { name: 'Warna tinta' })).not.toBeInTheDocument();
});

test('the pad opens full screen like the phone and Esc brings it back', async () => {
  renderSignature(slot);
  await userEvent.click(screen.getByRole('button', { name: 'Layar penuh' }));
  const dialog = screen.getByRole('dialog', { name: 'Tanda tangan layar penuh' });
  expect(within(dialog).getByLabelText('Area tanda tangan penerima')).toBeVisible();
  expect(within(dialog).getByRole('button', { name: 'Simpan tanda tangan' })).toBeVisible();
  expect(document.body.style.overflow).toBe('hidden');

  await userEvent.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', { name: 'Tanda tangan layar penuh' })).not.toBeInTheDocument();
  expect(screen.getByLabelText('Area tanda tangan penerima')).toBeVisible();
  expect(document.body.style.overflow).toBe('');
});
