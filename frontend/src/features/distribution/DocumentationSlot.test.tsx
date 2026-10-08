import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { ApiError } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DocumentationSlot } from './DocumentationSlot';
import styles from './Distribution.module.css';
import type { MediaFile } from './types';
import { uploadRequest } from '../../lib/upload';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});
vi.mock('../../lib/upload', () => ({ uploadRequest: vi.fn() }));

type TestMediaFile = Omit<MediaFile, 'storage_state'> & { storage_state?: MediaFile['storage_state'] };

function renderSlot(files: TestMediaFile[] = [], required = true, mediaKind: 'image' | 'video' | 'image_video' = 'image', canManage = true, canRetryMove = false) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['documentation.manage']}><DocumentationSlot slot={{
    id: 'slot-1', code: 'signed_bast', label: 'BAST bertanda tangan', stage: 'penyerahan', status: 'missing', required, min_files: 1, max_files: 2, media_kind: mediaKind, files: files.map((file) => ({ ...file, storage_state: file.storage_state ?? 'final' })),
  }} canManage={canManage} canRetryMove={canRetryMove} onChanged={vi.fn()} /></PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'camera', status: 'accepted', content_url: '/api/v1/distribution/media/media-1/content', storage_state: 'staging' } });
});

afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test('offers camera and gallery then uploads an image preview', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
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
  const request = vi.mocked(uploadRequest).mock.calls[0];
  expect((request[1] as FormData).get('captured_at')).toBeTruthy();
});

test('shows a retry action when upload fails', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(uploadRequest).mockRejectedValue(new Error('offline'));
  renderSlot();
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [new File(['image'], 'bast.jpg', { type: 'image/jpeg' })] } });
  expect(await screen.findByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();
});

test('shows the backend reason when an upload fails', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(uploadRequest).mockRejectedValue(new ApiError(409, 'zone_not_configured', 'Kabupaten belum dikonfigurasi ke zona'));
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
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-2', slot_id: 'slot-1', original_filename: 'drop.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/drop.jpg', storage_state: 'staging' } });
  renderSlot();
  const file = new File(['image'], 'drop.jpg', { type: 'image/jpeg' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [file] } });

  expect(await screen.findByRole('button', { name: 'Lihat drop.jpg' })).toBeVisible();
  const request = vi.mocked(uploadRequest).mock.calls[0];
  expect((request[1] as FormData).get('source')).toBe('gallery');
});

test('rejects a non-image dropped into the gallery zone', async () => {
  renderSlot();
  const file = new File(['document'], 'catatan.pdf', { type: 'application/pdf' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [file] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Gunakan file JPEG, PNG, atau WebP.');
  await waitFor(() => expect(uploadRequest).not.toHaveBeenCalled());
});

test('uploads several dropped photos one at a time without losing track of either', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(uploadRequest).mockImplementation(async (_path, body) => {
    const name = body.get('file') instanceof File ? (body.get('file') as File).name : '';
    return { data: { id: `media-${name}`, slot_id: 'slot-1', original_filename: name, mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: `/media/${name}`, storage_state: 'staging' as const } };
  });
  renderSlot([], true);
  const first = new File(['a'], 'first.jpg', { type: 'image/jpeg' });
  const second = new File(['b'], 'second.jpg', { type: 'image/jpeg' });

  fireEvent.drop(screen.getByLabelText('Unggah foto BAST bertanda tangan melalui galeri'), { dataTransfer: { files: [first, second] } });

  expect(await screen.findByRole('button', { name: 'Lihat first.jpg' })).toBeVisible();
  expect(await screen.findByRole('button', { name: 'Lihat second.jpg' })).toBeVisible();
  expect(uploadRequest).toHaveBeenCalledTimes(2);
});

test('caps a multi-file selection at the slot\'s remaining capacity with a clear message', async () => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-only', slot_id: 'slot-1', original_filename: 'only.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/only.jpg', storage_state: 'staging' } });
  renderSlot([{ id: 'media-existing', slot_id: 'slot-1', original_filename: 'existing.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/existing.jpg' }], true);
  const first = new File(['a'], 'only.jpg', { type: 'image/jpeg' });
  const second = new File(['b'], 'extra.jpg', { type: 'image/jpeg' });

  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [first, second] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Hanya 1 foto lagi yang dapat ditambahkan pada slot ini.');
  await waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole('button', { name: 'Lihat extra.jpg' })).not.toBeInTheDocument();
});

test('renders policy-specific camera controls and native capture hints', () => {
  const imageView = renderSlot([], true, 'image');
  expect(screen.getByLabelText('Buka kamera')).toHaveAttribute('accept', 'image/jpeg,image/png,image/webp');
  expect(screen.queryByLabelText('Rekam video')).not.toBeInTheDocument();
  imageView.unmount();
  const videoView = renderSlot([], true, 'video');
  expect(screen.getByLabelText('Rekam video')).toHaveAttribute('accept', 'video/mp4,video/webm,video/quicktime');
  expect(screen.getByLabelText('Rekam video')).toHaveAttribute('capture', 'environment');
  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  videoView.unmount();
  renderSlot([], true, 'image_video');
  expect(screen.getByLabelText('Buka kamera')).toBeVisible();
  expect(screen.getByLabelText('Rekam video')).toBeVisible();
});

test('rejects files above client limits and appends metadata before the file', async () => {
  renderSlot([], true, 'image_video');
  const oversized = new File(['x'], 'large.mp4', { type: 'video/mp4' });
  Object.defineProperty(oversized, 'size', { value: (500 * 1024 * 1024) + 1 });
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [oversized] } });
  expect(await screen.findByRole('alert')).toHaveTextContent('500 MiB');
  expect(uploadRequest).not.toHaveBeenCalled();

  const image = new File(['image'], 'proof.jpg', { type: 'image/jpeg', lastModified: 1 });
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [image] } });
  await waitFor(() => expect(uploadRequest).toHaveBeenCalledOnce());
  const body = vi.mocked(uploadRequest).mock.calls[0][1];
  expect(Array.from(body.keys())).toEqual(['source', 'file_size', 'captured_at', 'file']);
});

test('shows progress and can cancel an active upload', async () => {
  vi.mocked(uploadRequest).mockImplementation((_path, _body, options) => {
    options?.onProgress?.(42);
    return new Promise((_resolve, reject) => options?.signal?.addEventListener('abort', () => reject(new DOMException('cancelled', 'AbortError'))));
  });
  renderSlot([], true, 'video');
  fireEvent.change(screen.getByLabelText('Pilih galeri'), { target: { files: [new File(['video'], 'proof.mp4', { type: 'video/mp4' })] } });
  expect(await screen.findByText('42%')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Batalkan unggahan' }));
  await waitFor(() => expect(screen.queryByText('42%')).not.toBeInTheDocument());
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

test('hides upload, drag-drop, and delete controls when the owning POS cannot manage media', () => {
  renderSlot([{ id: 'media-1', slot_id: 'slot-1', original_filename: 'bast.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/bast.jpg' }], true, 'image', false);

  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Pilih galeri')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Unggah foto BAST bertanda tangan melalui galeri')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Hapus bast.jpg' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lihat bast.jpg' })).toBeVisible();
});

test.each([
  ['staging', 'Tersimpan sementara', 'Menunggu tanggal dan penerima'],
  ['moving', 'Sedang dipindahkan', null],
  ['final', 'Tersimpan di folder final', null],
  ['move_failed', 'Pemindahan gagal — akan dicoba kembali', null],
] as const)('shows storage state %s while keeping preview available', (storageState, label, detail) => {
  renderSlot([{ id: `media-${storageState}`, slot_id: 'slot-1', original_filename: `${storageState}.jpg`, mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: `/media/${storageState}.jpg`, storage_state: storageState }]);

  expect(screen.getByText(label)).toBeVisible();
  if (detail) expect(screen.getByText(detail)).toBeVisible();
  expect(screen.getByRole('button', { name: `Lihat ${storageState}.jpg` })).toBeVisible();
});

test('offers retry only for failed moves and publishes the returned media state', async () => {
  const onChanged = vi.fn();
  const onRetryMove = vi.fn();
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const failed: MediaFile = { id: 'media-failed', slot_id: 'slot-1', original_filename: 'failed.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/failed.jpg', storage_state: 'move_failed', storage_last_error: 'Drive timeout' };
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...failed, storage_state: 'moving', storage_last_error: '' } });
  render(<QueryClientProvider client={client}><DocumentationSlot slot={{ id: 'slot-1', code: 'proof', label: 'Bukti', stage: 'mesin', status: 'complete', required: true, min_files: 1, max_files: 2, media_kind: 'image', files: [failed] }} canManage canRetryMove onChanged={onChanged} onRetryMove={onRetryMove} /></QueryClientProvider>);

  fireEvent.click(screen.getByRole('button', { name: 'Coba pindahkan lagi failed.jpg' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/media/media-failed/retry-move', { method: 'POST' }));
  expect(await screen.findByText('Sedang dipindahkan')).toBeVisible();
  expect(onRetryMove).toHaveBeenCalledWith(expect.objectContaining({ id: 'media-failed', storage_state: 'moving' }));
  expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ files: [expect.objectContaining({ storage_state: 'moving' })] }));
});

test('does not offer move retry for staging, moving, or final media', () => {
  renderSlot((['staging', 'moving', 'final'] as const).map((storageState) => ({ id: `media-${storageState}`, slot_id: 'slot-1', original_filename: `${storageState}.jpg`, mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: `/media/${storageState}.jpg`, storage_state: storageState })), true, 'image', true, true);

  expect(screen.queryByRole('button', { name: /Coba pindahkan lagi/ })).not.toBeInTheDocument();
});

test('shows the backend reason when retrying a failed move is rejected', async () => {
  vi.mocked(apiRequest).mockRejectedValue(new ApiError(409, 'media_move_not_retryable', 'Tanggal dan penerima belum lengkap'));
  renderSlot([{ id: 'media-failed', slot_id: 'slot-1', original_filename: 'failed.jpg', mime_type: 'image/jpeg', byte_size: 10, source: 'gallery', status: 'accepted', content_url: '/media/failed.jpg', storage_state: 'move_failed' }], true, 'image', true, true);

  fireEvent.click(screen.getByRole('button', { name: 'Coba pindahkan lagi failed.jpg' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('Tanggal dan penerima belum lengkap');
});
