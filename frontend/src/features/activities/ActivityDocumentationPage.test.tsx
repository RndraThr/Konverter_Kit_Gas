import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { apiRequest, ApiError } from '../../lib/api';
import { uploadRequest } from '../../lib/upload';
import { PermissionsProvider } from '../../lib/permissions';
import { ActivityDocumentationPage } from './ActivityDocumentationPage';
import type { ActivityMediaPage } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});
vi.mock('../../lib/upload', () => ({ uploadRequest: vi.fn() }));

beforeEach(() => { vi.mocked(uploadRequest).mockReset(); });

function renderPage(permissions: string[], initialEntries: string[] = ['/dokumentasi/rakor']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={withProgram(initialEntries)}><ActivityDocumentationPage activityType="rakor" label="Rakor" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

const unloadingOptions = [
  { value: 'unloading_konkit' as const, label: 'Konkit' },
  { value: 'unloading_oli' as const, label: 'Oli' },
];

function renderGroupedPage(permissions: string[], initialEntries: string[] = ['/dokumentasi/unloading?regency_id=regency-1']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={withProgram(initialEntries)}><ActivityDocumentationPage label="Unloading" activityTypes={unloadingOptions} /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

function withProgram(entries: string[]) {
  return entries.map((entry) => `${entry}${entry.includes('?') ? '&' : '?'}program_id=program-1`);
}

function programSetupResponse(path: string) {
  if (path === '/api/v1/program-setup/programs') return { data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Program Petani 2026', program_type: 'farmer', status: 'active' }] };
  if (path === '/api/v1/program-setup/programs/program-1/zones') return { data: [{ id: 'zone-1', name: 'Zona 1', is_placeholder: false, regencies: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] }] };
  return undefined;
}

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function mockApi(gallery: ActivityMediaPage = { items: [], page: 1, page_size: 24, total: 0 }) {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: gallery });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

test('prompts to pick a kabupaten before loading the gallery', async () => {
  mockApi();
  renderPage(['activities.view']);
  expect(await screen.findByRole('heading', { name: 'Pilih kabupaten terlebih dahulu' })).toBeVisible();
});

test('handles program zones whose empty regencies field is omitted by the API', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/programs') return Promise.resolve(programSetupResponse(path)!);
    if (path === '/api/v1/program-setup/programs/program-1/zones') {
      return Promise.resolve({ data: [{ id: 'zone-empty', name: 'Zona Kosong', is_placeholder: false }] });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });

  renderPage(['activities.view']);

  expect(await screen.findByRole('heading', { name: 'Pilih kabupaten terlebih dahulu' })).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Kabupaten / Kota' })).toBeVisible();
});

test('requires a program before a kabupaten can be selected', async () => {
  mockApi();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['activities.view', 'activities.manage']}><MemoryRouter initialEntries={['/dokumentasi/rakor']}><ActivityDocumentationPage activityType="rakor" label="Rakor" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
  expect(await screen.findByRole('heading', { name: 'Pilih program terlebih dahulu' })).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Kabupaten / Kota' })).toBeDisabled();
  expect(screen.queryByLabelText('Pilih dari Galeri')).not.toBeInTheDocument();
});

test('shows the gallery once a kabupaten is selected', async () => {
  mockApi({
    items: [{
      id: 'media-1', regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', activity_type: 'rakor',
      display_name: 'WJO-RAKOR-20260916-154500', original_filename: 'foto.jpg', media_type: 'image', mime_type: 'image/jpeg',
      byte_size: 100, checksum: 'abc', source: 'gallery', status: 'active',
      uploaded_at: '2026-09-16T15:45:00Z', created_at: '2026-09-16T15:45:00Z', updated_at: '2026-09-16T15:45:00Z',
      content_url: '/api/v1/activities/media/media-1/content',
    }], page: 1, page_size: 24, total: 1,
  });
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  expect(await screen.findByAltText('WJO-RAKOR-20260916-154500')).toBeVisible();
});

function mediaItem(id: string, displayName: string): ActivityMediaPage['items'][number] {
  return {
    id, regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', activity_type: 'rakor',
    display_name: displayName, original_filename: `${displayName}.jpg`, media_type: 'image', mime_type: 'image/jpeg',
    byte_size: 100, checksum: 'abc', source: 'gallery', status: 'active',
    uploaded_at: '2026-09-16T15:45:00Z', created_at: '2026-09-16T15:45:00Z', updated_at: '2026-09-16T15:45:00Z',
    content_url: `/api/v1/activities/media/${id}/content`,
  };
}

test('opens the lightbox preview and browses between gallery items', async () => {
  mockApi({ items: [mediaItem('media-1', 'Foto Satu'), mediaItem('media-2', 'Foto Dua')], page: 1, page_size: 24, total: 2 });
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.click(await screen.findByRole('button', { name: 'Lihat Foto Satu' }));
  expect(screen.getByRole('dialog', { name: 'Preview Foto Satu' })).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Foto berikutnya' }));
  expect(screen.getByRole('dialog', { name: 'Preview Foto Dua' })).toBeVisible();
});

test('deletes a documentation item from the grid thumbnail', async () => {
  let deleted = false;
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path === '/api/v1/activities/media/media-1' && init?.method === 'DELETE') { deleted = true; return Promise.resolve(undefined); }
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: deleted ? [] : [mediaItem('media-1', 'Foto Satu')], page: 1, page_size: 24, total: deleted ? 0 : 1 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.click(await screen.findByRole('button', { name: 'Hapus Foto Satu' }));
  expect(await screen.findByRole('heading', { name: 'Hapus Foto Satu?' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Hapus' }));
  expect(await screen.findByText('Belum ada dokumentasi')).toBeVisible();
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

test('hides upload controls without activities.manage', async () => {
  mockApi();
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  await screen.findByText('Belum ada dokumentasi');
  expect(screen.queryByText('Ambil Foto')).not.toBeInTheDocument();
  expect(screen.queryByText('Pilih dari Galeri')).not.toBeInTheDocument();
});

test('uploads a photo dropped onto the upload zone', async () => {
  vi.mocked(uploadRequest).mockImplementation(async (_path, body) => {
    expect(body.get('source')).toBe('gallery');
    return { data: { id: 'media-2', display_name: 'WJO-RAKOR-20260916-160000', media_type: 'image', content_url: '/api/v1/activities/media/media-2/content' } as never };
  });
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:dropped-preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  const file = new File(['photo'], 'dropped.jpg', { type: 'image/jpeg' });
  fireEvent.drop(await screen.findByLabelText('Unggah dokumentasi Rakor'), { dataTransfer: { files: [file] } });

  expect(await screen.findByAltText('Preview unggahan')).toHaveAttribute('src', 'blob:dropped-preview');
});

test('uploads a photo via the gallery picker', async () => {
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-2', display_name: 'WJO-RAKOR-20260916-160000', media_type: 'image', content_url: '/api/v1/activities/media/media-2/content' } as never });
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);
  const file = new File(['photo'], 'foto.jpg', { type: 'image/jpeg' });
  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [file] } });
  expect(await screen.findByAltText('Preview unggahan')).toHaveAttribute('src', 'blob:preview');
});

test('renders a video element while a video upload is pending', async () => {
  vi.mocked(uploadRequest).mockImplementation(() => new Promise(() => undefined));
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:video-preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [new File(['video'], 'rakor.mp4', { type: 'video/mp4' })] } });

  expect(await screen.findByLabelText('Preview unggahan video')).toHaveAttribute('src', 'blob:video-preview');
  expect(screen.getByRole('combobox', { name: 'Kabupaten / Kota' })).toBeDisabled();
});

test('validates media sizes and sends activity metadata before the file', async () => {
  mockApi();
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-2' } as never });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);
  const gallery = await screen.findByLabelText('Pilih dari Galeri');

  const largeImage = new File(['image'], 'large.jpg', { type: 'image/jpeg' });
  Object.defineProperty(largeImage, 'size', { value: (25 * 1024 * 1024) + 1 });
  fireEvent.change(gallery, { target: { files: [largeImage] } });
  expect(await screen.findByRole('alert')).toHaveTextContent('25 MiB');
  expect(uploadRequest).not.toHaveBeenCalled();

  const largeVideo = new File(['video'], 'large.mp4', { type: 'video/mp4' });
  Object.defineProperty(largeVideo, 'size', { value: (500 * 1024 * 1024) + 1 });
  fireEvent.change(gallery, { target: { files: [largeVideo] } });
  expect(await screen.findByRole('alert')).toHaveTextContent('500 MiB');
  expect(uploadRequest).not.toHaveBeenCalled();

  fireEvent.change(gallery, { target: { files: [new File(['photo'], 'proof.jpg', { type: 'image/jpeg' })] } });
  await waitFor(() => expect(uploadRequest).toHaveBeenCalledOnce());
  expect(Array.from(vi.mocked(uploadRequest).mock.calls[0][1].keys())).toEqual(['program_id', 'regency_id', 'activity_type', 'source', 'file_size', 'file']);
});

test('shows native upload progress and cancels an active video upload', async () => {
  mockApi();
  vi.mocked(uploadRequest).mockImplementation((_path, _body, options) => {
    options?.onProgress?.(42);
    return new Promise((_resolve, reject) => options?.signal?.addEventListener('abort', () => reject(new DOMException('cancelled', 'AbortError'))));
  });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Rekam Video'), { target: { files: [new File(['video'], 'proof.mp4', { type: 'video/mp4' })] } });
  expect(await screen.findByText('42%')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Batalkan unggahan' }));
  await waitFor(() => expect(screen.queryByLabelText('Preview unggahan video')).not.toBeInTheDocument());
});

test('preserves a backend upload reason for the operator', async () => {
  mockApi();
  vi.mocked(uploadRequest).mockRejectedValue(new ApiError(429, 'video_upload_busy', 'Unggahan video sedang penuh, coba lagi sebentar.'));
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [new File(['video'], 'proof.mp4', { type: 'video/mp4' })] } });
  expect(await screen.findByRole('alert')).toHaveTextContent('Unggahan video sedang penuh, coba lagi sebentar.');
  expect(screen.getByText('Coba lagi')).toBeVisible();
});

test('offers retry and cancel actions after an upload fails', async () => {
  let uploadAttempts = 0;
  vi.mocked(uploadRequest).mockImplementation(async (_path, body) => {
    uploadAttempts += 1;
    if (uploadAttempts === 1) throw new Error('upload failed');
    expect(body.get('program_id')).toBe('program-1');
    expect(body.get('regency_id')).toBe('regency-1');
    return { data: { id: 'media-2' } as never };
  });
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:failed-preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [new File(['photo'], 'foto.jpg', { type: 'image/jpeg' })] } });

  expect(await screen.findByText('Coba lagi')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Batalkan' })).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Kabupaten / Kota' })).toBeDisabled();
  fireEvent.click(screen.getByText('Coba lagi'));
  expect(await screen.findByText('Belum ada dokumentasi')).toBeVisible();
  expect(uploadAttempts).toBe(2);
});

test('shows a retryable error when program zones cannot be loaded', async () => {
  let regencyAttempts = 0;
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/programs') return Promise.resolve(programSetupResponse(path)!);
    if (path === '/api/v1/program-setup/programs/program-1/zones') {
      regencyAttempts += 1;
      if (regencyAttempts === 1) return Promise.reject(new Error('regencies unavailable'));
      return Promise.resolve(programSetupResponse(path)!);
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });

  renderPage(['activities.view']);

  expect(await screen.findByRole('heading', { name: 'Daftar kabupaten belum dapat dimuat' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(await screen.findByRole('combobox', { name: 'Kabupaten / Kota' })).toBeVisible();
  expect(regencyAttempts).toBe(2);
});

test('grouped mode defaults to the first tab and loads its gallery', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) {
      const params = new URLSearchParams(path.split('?')[1]);
      expect(params.get('activity_type')).toBe('unloading_konkit');
      return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderGroupedPage(['activities.view']);

  const konkitTab = await screen.findByRole('tab', { name: 'Konkit' });
  expect(konkitTab).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('tab', { name: 'Oli' })).toHaveAttribute('aria-selected', 'false');
  await screen.findByText('Belum ada dokumentasi');
});

test('switching tabs refetches the gallery for the newly selected activity type', async () => {
  const seenTypes: string[] = [];
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) {
      const params = new URLSearchParams(path.split('?')[1]);
      seenTypes.push(params.get('activity_type') ?? '');
      return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  renderGroupedPage(['activities.view']);
  await screen.findByText('Belum ada dokumentasi');

  fireEvent.click(screen.getByRole('tab', { name: 'Oli' }));

  await screen.findByText('Belum ada dokumentasi');
  expect(seenTypes).toContain('unloading_konkit');
  expect(seenTypes).toContain('unloading_oli');
});

test('uploads to the currently active tab, not the first option', async () => {
  let sentActivityType: string | null = null;
  vi.mocked(uploadRequest).mockImplementation((_path, body) => {
    sentActivityType = body.get('activity_type') as string;
    return new Promise(() => undefined);
  });
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() }));
  renderGroupedPage(['activities.view', 'activities.manage']);
  await screen.findByText('Belum ada dokumentasi');

  fireEvent.click(screen.getByRole('tab', { name: 'Oli' }));
  await screen.findByText('Belum ada dokumentasi');

  const file = new File(['photo'], 'foto.jpg', { type: 'image/jpeg' });
  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [file] } });
  await screen.findByAltText('Preview unggahan');
  expect(sentActivityType).toBe('unloading_oli');
});

test('does not render tabs in single-activity-type mode', async () => {
  mockApi();
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  await screen.findByText('Belum ada dokumentasi');
  expect(screen.queryByRole('tab')).not.toBeInTheDocument();
});

test('clears a failed upload when navigating to another activity type', async () => {
  vi.mocked(uploadRequest).mockRejectedValue(new Error('upload failed'));
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    const setup = programSetupResponse(path); if (setup) return Promise.resolve(setup);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:failed-preview'), revokeObjectURL: vi.fn() });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = (activityType: 'rakor' | 'training_10', label: string) => (
    <QueryClientProvider client={client}><PermissionsProvider permissions={['activities.view', 'activities.manage']}><MemoryRouter initialEntries={['/dokumentasi/rakor?program_id=program-1&regency_id=regency-1']}><ActivityDocumentationPage activityType={activityType} label={label} /></MemoryRouter></PermissionsProvider></QueryClientProvider>
  );
  const view = render(wrapper('rakor', 'Rakor'));

  fireEvent.change(await screen.findByLabelText('Pilih dari Galeri'), { target: { files: [new File(['photo'], 'foto.jpg', { type: 'image/jpeg' })] } });
  expect(await screen.findByText('Coba lagi')).toBeVisible();

  view.rerender(wrapper('training_10', 'Training 10%'));

  expect(await screen.findByRole('heading', { name: 'Training 10%' })).toBeVisible();
  expect(screen.queryByText('Coba lagi')).not.toBeInTheDocument();
  expect(screen.queryByAltText('Preview unggahan')).not.toBeInTheDocument();
});
