import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DocumentationSlot } from './DocumentationSlot';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderSlot() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['documentation.manage']}><DocumentationSlot slot={{
    id: 'slot-1', code: 'signed_bast', label: 'BAST bertanda tangan', status: 'missing', required: true, min_files: 1, max_files: 2, files: [],
  }} onChanged={vi.fn()} /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test('offers camera and gallery then uploads an image preview', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockResolvedValue({ data: { id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/api/v1/distribution/media/media-1/content' } });
  renderSlot();

  const camera = screen.getByLabelText('Buka kamera');
  const gallery = screen.getByLabelText('Pilih galeri');
  expect(camera).toHaveAttribute('capture', 'environment');
  expect(gallery).not.toHaveAttribute('capture');
  const file = new File(['image'], 'bast.jpg', { type: 'image/jpeg' });
  fireEvent.change(camera, { target: { files: [file] } });

  expect(screen.getByAltText('Preview BAST bertanda tangan')).toHaveAttribute('src', 'blob:preview');
  expect(await screen.findByText('1 dari 1 foto wajib')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Hapus bast.jpg' })).toBeVisible();
});

test('shows a retry action when upload fails', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockRejectedValue(new Error('offline'));
  renderSlot();
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [new File(['image'], 'bast.jpg', { type: 'image/jpeg' })] } });
  expect(await screen.findByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();
});
