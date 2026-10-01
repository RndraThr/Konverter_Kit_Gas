import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { ZonesPanel } from './ZonesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('shows the selected program type and warns about placeholder assignments', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path.endsWith('/programs')) return Promise.resolve({ data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Tender Petani', program_type: 'farmer', status: 'active' }] });
    if (path.endsWith('/regencies')) return Promise.resolve({ data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] });
    if (path.endsWith('/programs/program-1/zones')) return Promise.resolve({ data: [{ id: 'placeholder', name: 'ZONA BELUM DIATUR', code: 'UNASSIGNED', is_placeholder: true, regencies: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }] }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view', 'programs.manage']}><ZonesPanel /></PermissionsProvider></QueryClientProvider>);
  expect(await screen.findByText('Petani')).toBeVisible();
  expect(await screen.findByText('1 kabupaten belum memiliki zona')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Tambah zona' })).toBeVisible();
});

test('renders a zone with no regencies when the API omits the empty collection', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path.endsWith('/programs')) return Promise.resolve({ data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Tender Petani', program_type: 'farmer', status: 'active' }] });
    if (path.endsWith('/regencies')) return Promise.resolve({ data: [] });
    if (path.endsWith('/programs/program-1/zones')) return Promise.resolve({ data: [{ id: 'zone-1', name: 'ZONA 1', code: 'ZONA-1', is_placeholder: false }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view']}><ZonesPanel /></PermissionsProvider></QueryClientProvider>);

  expect(await screen.findByText('ZONA 1')).toBeVisible();
  expect(screen.getByText('Belum ada kabupaten.')).toBeVisible();
});
