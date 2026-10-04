import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest, ApiError } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import type { MediaFile } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

function renderSlot(files: MediaFile[] = [], required = true) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['documentation.manage']}><DocumentationSlot slot={{
    id: 'slot-1', code: 'signed_bast', label: 'BAST bertanda tangan', stage: 'penyerahan', status: 'missing', required, min_files: 1, max_files: 2, files,
  }} onChanged={vi.fn()} /></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => {
  vi.clearAllMocks();
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
  const request = vi.mocked(apiRequest).mock.calls[0];
  expect((request[1]?.body as FormData).get('captured_at')).toBeTruthy();
});

test('shows a retry action when upload fails', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockRejectedValue(new Error('offline'));
  renderSlot();
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [new File(['image'], 'bast.jpg', { type: 'image/jpeg' })] } });
  expect(await screen.findByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();
});

test('shows the backend reason when an upload fails', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockRejectedValue(new ApiError(409, 'zone_not_configured', 'Kabupaten belum dikonfigurasi ke zona'));
  renderSlot();

  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [new File(['image'], 'bast.jpg', { type: 'image/jpeg' })] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Kabupaten belum dikonfigurasi ke zona');
  expect(screen.getByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();
});

test('uses the 44-pixel remove-media target contract', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);
  expect(screen.getByRole('button', { name: 'Hapus bast.jpg' })).toHaveClass(styles.removeMedia);
});

test('opens an uploaded photo in a dialog and supports zoom controls', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);

  fireEvent.click(screen.getByRole('button', { name: 'Lihat bast.jpg' }));

  expect(screen.getByRole('dialog', { name: 'Preview bast.jpg' })).toBeVisible();
  expect(screen.getByRole('img', { name: 'bast.jpg' })).toHaveAttribute('src', '/media/bast.jpg');
  expect(screen.getByText('100%')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Perbesar foto' }));
  expect(screen.getByText('125%')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Reset zoom' }));
  expect(screen.getByText('100%')).toBeVisible();
});

test('browses between multiple photos inside the open preview dialog', () => {
  renderSlot([
    { id: 'media-1', slot_id: 'slot-1', original_filename: 'first.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/first.jpg' },
    { id: 'media-2', slot_id: 'slot-1', original_filename: 'second.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/second.jpg' },
  ]);

  fireEvent.click(screen.getByRole('button', { name: 'Lihat first.jpg' }));
  expect(screen.getByRole('dialog', { name: 'Preview first.jpg' })).toBeVisible();
  expect(screen.getByText('1/2')).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Foto berikutnya' }));
  expect(screen.getByRole('dialog', { name: 'Preview second.jpg' })).toBeVisible();
  expect(screen.getByText('2/2')).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Foto berikutnya' }));
  expect(screen.getByRole('dialog', { name: 'Preview first.jpg' })).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Foto sebelumnya' }));
  expect(screen.getByRole('dialog', { name: 'Preview second.jpg' })).toBeVisible();
});

test('zooms an opened photo with the mouse wheel', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);
  fireEvent.click(screen.getByRole('button', { name: 'Lihat bast.jpg' }));

  fireEvent.wheel(screen.getByLabelText('Area preview foto'), { deltaY: -100 });

  expect(screen.getByText('125%')).toBeVisible();
});

test('toggles zoom on double-click', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);
  fireEvent.click(screen.getByRole('button', { name: 'Lihat bast.jpg' }));
  const area = screen.getByLabelText('Area preview foto');

  fireEvent.doubleClick(area);
  expect(screen.getByText('200%')).toBeVisible();

  fireEvent.doubleClick(area);
  expect(screen.getByText('100%')).toBeVisible();
});

test('pans a zoomed photo by dragging with one pointer', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);
  fireEvent.click(screen.getByRole('button', { name: 'Lihat bast.jpg' }));
  const area = screen.getByLabelText('Area preview foto');
  const image = screen.getByRole('img', { name: 'bast.jpg' });

  fireEvent.pointerDown(area, { pointerId: 1, clientX: 100, clientY: 100 });
  fireEvent.pointerMove(area, { pointerId: 1, clientX: 60, clientY: 80 });

  expect(image).toHaveStyle({ transform: 'translate(-40px, -20px) scale(1)' });
});

test('pinch-zooms by tracking the distance between two pointers', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }]);
  fireEvent.click(screen.getByRole('button', { name: 'Lihat bast.jpg' }));
  const area = screen.getByLabelText('Area preview foto');

  fireEvent.pointerDown(area, { pointerId: 1, clientX: 100, clientY: 100 });
  fireEvent.pointerDown(area, { pointerId: 2, clientX: 200, clientY: 100 });
  fireEvent.pointerMove(area, { pointerId: 1, clientX: 50, clientY: 100 });

  expect(screen.getByText('150%')).toBeVisible();
});

test('uploads a dropped image as a gallery file', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:dropped'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockResolvedValue({ data: { id: 'media-2', slot_id: 'slot-1', original_filename: 'drop.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/drop.jpg' } });
  renderSlot();
  const file = new File(['image'], 'drop.jpg', { type: 'image/jpeg' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [file] } });

  expect(await screen.findByRole('button', { name: 'Lihat drop.jpg' })).toBeVisible();
  const request = vi.mocked(apiRequest).mock.calls[0];
  expect((request[1]?.body as FormData).get('source')).toBe('gallery');
});

test('rejects a non-image dropped into the gallery zone', async () => {
  renderSlot();
  const file = new File(['document'], 'catatan.pdf', { type: 'application/pdf' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [file] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Gunakan file JPEG, PNG, atau WebP.');
  await waitFor(() => expect(apiRequest).not.toHaveBeenCalled());
});

test('uploads several dropped photos one at a time without losing track of either', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockImplementation(async (_path, init) => {
    const name = (init?.body as FormData).get('file') instanceof File ? ((init?.body as FormData).get('file') as File).name : '';
    return { data: { id: `media-${name}`, slot_id: 'slot-1', original_filename: name, mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: `/media/${name}` } };
  });
  renderSlot([], true);
  const first = new File(['a'], 'first.jpg', { type: 'image/jpeg' });
  const second = new File(['b'], 'second.jpg', { type: 'image/jpeg' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [first, second] } });

  expect(await screen.findByRole('button', { name: 'Lihat first.jpg' })).toBeVisible();
  expect(await screen.findByRole('button', { name: 'Lihat second.jpg' })).toBeVisible();
  expect(apiRequest).toHaveBeenCalledTimes(2);
});

test('caps a multi-file selection at the slot\'s remaining capacity with a clear message', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(apiRequest).mockResolvedValue({ data: { id: 'media-only', slot_id: 'slot-1', original_filename: 'only.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/only.jpg' } });
  renderSlot([{ id: 'media-existing', slot_id: 'slot-1', original_filename: 'existing.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/existing.jpg' }], true);
  const first = new File(['a'], 'only.jpg', { type: 'image/jpeg' });
  const second = new File(['b'], 'extra.jpg', { type: 'image/jpeg' });

  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [first, second] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Hanya 1 foto lagi yang dapat ditambahkan pada slot ini.');
  await waitFor(() => expect(apiRequest).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole('button', { name: 'Lihat extra.jpg' })).not.toBeInTheDocument();
});

test('does not describe an optional slot as mandatory', () => {
  renderSlot([], false);
  expect(screen.getByText('Opsional')).toBeVisible();
  expect(screen.getByText('0 dari 1 foto')).toBeVisible();
  expect(screen.getByText('Belum diisi')).toBeVisible();
  expect(screen.queryByText('0 dari 1 foto wajib')).not.toBeInTheDocument();
});

test('associates camera and gallery inputs with visible focus controls', () => {
  renderSlot();
  const cameraControl = screen.getByLabelText('Buka kamera').closest('label');
  const galleryControl = screen.getByLabelText('Pilih galeri').closest('label');
  expect(cameraControl).toHaveClass(styles.captureControl);
  expect(galleryControl).toHaveClass(styles.captureControl);
});
