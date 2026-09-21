import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { ActivityDocumentationPage } from './ActivityDocumentationPage';
import type { ActivityMediaPage } from './types';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPage(permissions: string[], initialEntries: string[] = ['/dokumentasi/rakor']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={initialEntries}><ActivityDocumentationPage activityType="rakor" label="Rakor" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

const unloadingOptions = [
  { value: 'unloading_konkit' as const, label: 'Konkit' },
  { value: 'unloading_oli' as const, label: 'Oli' },
];

function renderGroupedPage(permissions: string[], initialEntries: string[] = ['/dokumentasi/unloading?regency_id=regency-1']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={initialEntries}><ActivityDocumentationPage label="Unloading" activityTypes={unloadingOptions} /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function mockApi(gallery: ActivityMediaPage = { items: [], page: 1, page_size: 24, total: 0 }) {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: gallery });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
}

test('prompts to pick a kabupaten before loading the gallery', async () => {
  mockApi();
  renderPage(['activities.view']);
  expect(await screen.findByRole('heading', { name: 'Pilih kabupaten' })).toBeVisible();
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

test('hides upload controls without activities.manage', async () => {
  mockApi();
  renderPage(['activities.view'], ['/dokumentasi/rakor?regency_id=regency-1']);
  await screen.findByText('Belum ada dokumentasi');
  expect(screen.queryByLabelText('Buka kamera')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Pilih galeri')).not.toBeInTheDocument();
});

test('uploads a photo via the gallery picker', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') {
      return Promise.resolve({ data: { id: 'media-2', display_name: 'WJO-RAKOR-20260916-160000', media_type: 'image', content_url: '/api/v1/activities/media/media-2/content' } });
    }
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);
  const file = new File(['photo'], 'foto.jpg', { type: 'image/jpeg' });
  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [file] } });
  expect(await screen.findByAltText('Preview unggahan')).toHaveAttribute('src', 'blob:preview');
});

test('renders a video element while a video upload is pending', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') return new Promise(() => undefined);
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:video-preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [new File(['video'], 'rakor.mp4', { type: 'video/mp4' })] } });

  expect(await screen.findByLabelText('Preview unggahan video')).toHaveAttribute('src', 'blob:video-preview');
  expect(screen.getByRole('combobox', { name: 'Kabupaten' })).toBeDisabled();
});

test('offers retry and cancel actions after an upload fails', async () => {
  let uploadAttempts = 0;
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') {
      uploadAttempts += 1;
      if (uploadAttempts === 1) return Promise.reject(new Error('upload failed'));
      expect((init.body as FormData).get('regency_id')).toBe('regency-1');
      return Promise.resolve({ data: { id: 'media-2' } });
    }
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:failed-preview'), revokeObjectURL: vi.fn() });
  renderPage(['activities.view', 'activities.manage'], ['/dokumentasi/rakor?regency_id=regency-1']);

  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [new File(['photo'], 'foto.jpg', { type: 'image/jpeg' })] } });

  expect(await screen.findByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Batalkan unggahan' })).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Kabupaten' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Coba unggah lagi' }));
  expect(await screen.findByText('Belum ada dokumentasi')).toBeVisible();
  expect(uploadAttempts).toBe(2);
});

test('shows a retryable error when regencies cannot be loaded', async () => {
  let regencyAttempts = 0;
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/regencies') {
      regencyAttempts += 1;
      if (regencyAttempts === 1) return Promise.reject(new Error('regencies unavailable'));
      return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });

  renderPage(['activities.view']);

  expect(await screen.findByRole('heading', { name: 'Daftar kabupaten belum dapat dimuat' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(await screen.findByRole('combobox', { name: 'Kabupaten' })).toBeVisible();
  expect(regencyAttempts).toBe(2);
});

test('grouped mode defaults to the first tab and loads its gallery', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
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
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
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
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') {
      sentActivityType = (init.body as FormData).get('activity_type') as string;
      return new Promise(() => undefined);
    }
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() }));
  renderGroupedPage(['activities.view', 'activities.manage']);
  await screen.findByText('Belum ada dokumentasi');

  fireEvent.click(screen.getByRole('tab', { name: 'Oli' }));
  await screen.findByText('Belum ada dokumentasi');

  const file = new File(['photo'], 'foto.jpg', { type: 'image/jpeg' });
  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [file] } });
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
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path === '/api/v1/activities/media' && init?.method === 'POST') return Promise.reject(new Error('upload failed'));
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: { items: [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:failed-preview'), revokeObjectURL: vi.fn() });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = (activityType: 'rakor' | 'training_10', label: string) => (
    <QueryClientProvider client={client}><PermissionsProvider permissions={['activities.view', 'activities.manage']}><MemoryRouter initialEntries={['/dokumentasi/rakor?regency_id=regency-1']}><ActivityDocumentationPage activityType={activityType} label={label} /></MemoryRouter></PermissionsProvider></QueryClientProvider>
  );
  const view = render(wrapper('rakor', 'Rakor'));

  fireEvent.change(await screen.findByLabelText('Pilih galeri'), { target: { files: [new File(['photo'], 'foto.jpg', { type: 'image/jpeg' })] } });
  expect(await screen.findByRole('button', { name: 'Coba unggah lagi' })).toBeVisible();

  view.rerender(wrapper('training_10', 'Training 10%'));

  expect(await screen.findByRole('heading', { name: 'Training 10%' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Coba unggah lagi' })).not.toBeInTheDocument();
  expect(screen.queryByAltText('Preview unggahan')).not.toBeInTheDocument();
});
