import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { DocumentProfilePanel } from './DocumentProfilePanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('renders a draft profile with accessible logo controls and uploads multipart logo data', async () => {
  let uploaded: FormData | undefined;
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path.endsWith('/programs')) return Promise.resolve({ data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Tender Petani', program_type: 'farmer', status: 'active' }] });
    if (path.endsWith('/document-profiles') && init?.method === 'POST') { uploaded = init.body as FormData; return Promise.resolve({ data: {} }); }
    if (path.endsWith('/document-profiles')) return Promise.resolve({ data: [{ id: 'profile-1', program_id: 'program-1', version: 1, title: 'BAST', subtitle: 'Form Penerima', procurement_description: 'Pengadaan', document_series: 'KSM-KKT', status: 'draft', logos: [] }] });
    if (path.endsWith('/document-profiles/profile-1/logos') && init?.method === 'POST') { uploaded = init.body as FormData; return new Promise(() => undefined); }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view', 'programs.manage']}><DocumentProfilePanel /></PermissionsProvider></QueryClientProvider>);
  expect(await screen.findByDisplayValue('BAST')).toBeVisible();
  const file = new File(['logo'], 'logo.png', { type: 'image/png' });
  fireEvent.change(screen.getByLabelText('Unggah logo'), { target: { files: [file] } });
  await waitFor(() => expect(uploaded?.get('slot_code')).toBe('logo_1'));
  expect(uploaded?.get('file')).toBe(file);
  expect(screen.getByRole('button', { name: 'Publikasikan versi' })).toBeDisabled();
});
