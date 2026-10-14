import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest, ApiError } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SupportingDocumentsPage } from './SupportingDocumentsPage';
import type { ActivityMediaPage, SupportingDocumentSlot } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

afterEach(() => { vi.restoreAllMocks(); });

const slots: SupportingDocumentSlot[] = [
  { id: 'slot-1', regency_id: 'regency-1', name: 'SK Penetapan', is_active: true },
  { id: 'slot-2', regency_id: 'regency-1', name: 'Surat Tugas', is_active: true },
  { id: 'slot-3', regency_id: 'regency-1', name: 'Dokumen Lama', is_active: false },
];

function docMedia(id: string, displayName: string): ActivityMediaPage['items'][number] {
  return {
    id, regency_id: 'regency-1', regency_name: 'Wajo', regency_document_code: 'WJO', activity_type: 'dokumen_pendukung',
    display_name: displayName, original_filename: `${displayName}.pdf`, media_type: 'document', mime_type: 'application/pdf',
    byte_size: 100, checksum: 'abc', source: 'gallery', status: 'active',
    uploaded_at: '2026-09-16T15:45:00Z', created_at: '2026-09-16T15:45:00Z', updated_at: '2026-09-16T15:45:00Z',
    content_url: `/api/v1/activities/media/${id}/content`,
  };
}

function renderPage(permissions: string[], { gallery = { items: [], page: 1, page_size: 24, total: 0 } as ActivityMediaPage, slotList = slots, onRequest }: { gallery?: ActivityMediaPage; slotList?: SupportingDocumentSlot[]; onRequest?: (path: string, init?: RequestInit) => unknown } = {}) {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (onRequest) {
      const result = onRequest(path, init);
      if (result !== undefined) return Promise.resolve(result);
    }
    if (path.startsWith('/api/v1/activities/supporting-documents/slots')) return Promise.resolve({ data: slotList });
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: gallery });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter><SupportingDocumentsPage programId="program-1" regencyId="regency-1" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

test('renders the list of uploaded supporting documents', async () => {
  renderPage(['activities.view'], { gallery: { items: [docMedia('media-1', 'WJO-SK-20260916')], page: 1, page_size: 24, total: 1 } });
  expect(await screen.findByText('WJO-SK-20260916')).toBeVisible();
});

test('shows an empty state when there are no documents yet', async () => {
  renderPage(['activities.view']);
  expect(await screen.findByText('Belum ada dokumen pendukung yang diunggah.')).toBeVisible();
});

test('hides upload controls without activities.manage', async () => {
  renderPage(['activities.view']);
  await screen.findByText('Belum ada dokumen pendukung yang diunggah.');
  expect(screen.queryByRole('button', { name: 'Upload PDF' })).not.toBeInTheDocument();
});

test('clicking Upload PDF opens a slot picker dialog listing active slots only', async () => {
  renderPage(['activities.view', 'activities.manage']);

  const uploadButton = await screen.findByRole('button', { name: 'Upload PDF' });
  await waitFor(() => expect(uploadButton).toBeEnabled());
  fireEvent.click(uploadButton);

  expect(await screen.findByRole('dialog', { name: 'Pilih nama dokumen' })).toBeVisible();
  fireEvent.click(screen.getByRole('combobox'));
  expect(await screen.findByRole('option', { name: 'SK Penetapan' })).toBeVisible();
  expect(screen.getByRole('option', { name: 'Surat Tugas' })).toBeVisible();
  expect(screen.queryByRole('option', { name: 'Dokumen Lama' })).not.toBeInTheDocument();
});

test('file input stays disabled until a slot is chosen, then upload posts program_id, regency_id and slot_id', async () => {
  let capturedBody: FormData | null = null;
  renderPage(['activities.view', 'activities.manage'], {
    onRequest: (path, init) => {
      if (path === '/api/v1/activities/media' && init?.method === 'POST') {
        capturedBody = init.body as FormData;
        return { data: { id: 'media-2' } };
      }
      return undefined;
    },
  });

  const uploadButton = await screen.findByRole('button', { name: 'Upload PDF' });
  await waitFor(() => expect(uploadButton).toBeEnabled());
  fireEvent.click(uploadButton);
  await screen.findByRole('dialog', { name: 'Pilih nama dokumen' });

  const fileInput = screen.getByLabelText('Pilih berkas PDF');
  expect(fileInput).toBeDisabled();

  await userEvent.click(screen.getByRole('combobox'));
  await userEvent.click(await screen.findByRole('option', { name: 'SK Penetapan' }));

  await waitFor(() => expect(fileInput).toBeEnabled());

  const file = new File(['%PDF-1.4'], 'sk.pdf', { type: 'application/pdf' });
  fireEvent.change(fileInput, { target: { files: [file] } });

  await waitFor(() => expect(capturedBody).not.toBeNull());
  expect(capturedBody!.get('program_id')).toBe('program-1');
  expect(capturedBody!.get('regency_id')).toBe('regency-1');
  expect(capturedBody!.get('slot_id')).toBe('slot-1');
  expect(capturedBody!.get('activity_type')).toBe('dokumen_pendukung');
  expect(capturedBody!.get('source')).toBe('gallery');
  expect(capturedBody!.has('custom_name')).toBe(false);
});

test('switching to a custom name uploads custom_name instead of slot_id', async () => {
  let capturedBody: FormData | null = null;
  renderPage(['activities.view', 'activities.manage'], {
    onRequest: (path, init) => {
      if (path === '/api/v1/activities/media' && init?.method === 'POST') {
        capturedBody = init.body as FormData;
        return { data: { id: 'media-3' } };
      }
      return undefined;
    },
  });

  fireEvent.click(await screen.findByRole('button', { name: 'Upload PDF' }));
  await screen.findByRole('dialog', { name: 'Pilih nama dokumen' });

  await userEvent.click(screen.getByRole('tab', { name: 'Nama lain' }));
  await userEvent.type(screen.getByLabelText('Nama dokumen'), 'Surat Keterangan Domisili');

  const fileInput = await screen.findByLabelText('Pilih berkas PDF');
  await waitFor(() => expect(fileInput).toBeEnabled());
  fireEvent.change(fileInput, { target: { files: [new File(['%PDF-1.4'], 'sk.pdf', { type: 'application/pdf' })] } });

  await waitFor(() => expect(capturedBody).not.toBeNull());
  expect(capturedBody!.get('custom_name')).toBe('Surat Keterangan Domisili');
  expect(capturedBody!.has('slot_id')).toBe(false);
});

test('shows an upload error message from the API', async () => {
  renderPage(['activities.view', 'activities.manage'], {
    onRequest: (path, init) => {
      if (path === '/api/v1/activities/media' && init?.method === 'POST') {
        return Promise.reject(new ApiError(422, 'invalid_file', 'Berkas tidak valid.'));
      }
      return undefined;
    },
  });

  const uploadButton = await screen.findByRole('button', { name: 'Upload PDF' });
  await waitFor(() => expect(uploadButton).toBeEnabled());
  fireEvent.click(uploadButton);
  await screen.findByRole('dialog', { name: 'Pilih nama dokumen' });
  await userEvent.click(screen.getByRole('combobox'));
  await userEvent.click(await screen.findByRole('option', { name: 'SK Penetapan' }));

  const fileInput = await screen.findByLabelText('Pilih berkas PDF');
  await waitFor(() => expect(fileInput).toBeEnabled());
  fireEvent.change(fileInput, { target: { files: [new File(['%PDF-1.4'], 'sk.pdf', { type: 'application/pdf' })] } });

  expect(await screen.findByRole('alert')).toHaveTextContent('Berkas tidak valid.');
});

test('deletes a document from the list', async () => {
  let deleted = false;
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path === '/api/v1/activities/media/media-1' && init?.method === 'DELETE') { deleted = true; return Promise.resolve(undefined); }
    if (path.startsWith('/api/v1/activities/supporting-documents/slots')) return Promise.resolve({ data: slots });
    if (path.startsWith('/api/v1/activities/media?')) return Promise.resolve({ data: deleted ? { items: [], page: 1, page_size: 24, total: 0 } : { items: [docMedia('media-1', 'WJO-SK-20260916')], page: 1, page_size: 24, total: 1 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['activities.view', 'activities.manage']}><MemoryRouter><SupportingDocumentsPage programId="program-1" regencyId="regency-1" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);

  fireEvent.click(await screen.findByRole('button', { name: 'Hapus WJO-SK-20260916' }));
  await waitFor(() => expect(deleted).toBe(true));
  expect(await screen.findByText('Belum ada dokumen pendukung')).toBeVisible();
});

test('upload and scan buttons stay enabled when no slots are configured — custom name still works', async () => {
  renderPage(['activities.view', 'activities.manage'], { slotList: [] });

  const uploadButton = await screen.findByRole('button', { name: 'Upload PDF' });
  expect(uploadButton).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Scan dari Kamera' })).toBeEnabled();

  fireEvent.click(uploadButton);
  await screen.findByRole('dialog', { name: 'Pilih nama dokumen' });
  expect(screen.getByRole('combobox')).toBeDisabled();

  await userEvent.click(screen.getByRole('tab', { name: 'Nama lain' }));
  expect(screen.getByLabelText('Nama dokumen')).toBeVisible();
});
