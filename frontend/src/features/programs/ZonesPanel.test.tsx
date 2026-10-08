import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
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
  render(<MemoryRouter><QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view', 'programs.manage']}><ZonesPanel /></PermissionsProvider></QueryClientProvider></MemoryRouter>);
  expect(await screen.findByText('Petani')).toBeVisible();
  expect(await screen.findByText('1 kabupaten belum memiliki zona')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Tambah zona' })).toBeVisible();
});

test('lays regencies out as board columns, allows bulk selection, and only deletes empty zones', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (path.endsWith('/programs')) return Promise.resolve({ data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Tender Petani', program_type: 'farmer', status: 'active' }] });
    if (path.endsWith('/regencies')) return Promise.resolve({ data: [{ id: 'r1', name: 'Wajo', document_code: 'WJO' }, { id: 'r2', name: 'Bone', document_code: 'BON' }] });
    if (path.endsWith('/programs/program-1/zones')) return Promise.resolve({ data: [
      { id: 'placeholder', name: 'ZONA BELUM DIATUR', code: 'UNASSIGNED', is_placeholder: true, regencies: [] },
      { id: 'zone-1', name: 'ZONA 1', code: 'ZONA-1', is_placeholder: false, regencies: [{ id: 'r1', name: 'Wajo', document_code: 'WJO' }] },
      { id: 'zone-2', name: 'ZONA 2', code: 'ZONA-2', is_placeholder: false, regencies: [] },
    ] });
    if (path.includes('/zones/zone-2') && init?.method === 'DELETE') return Promise.resolve(undefined);
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<MemoryRouter><QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view', 'programs.manage']}><ZonesPanel /></PermissionsProvider></QueryClientProvider></MemoryRouter>);

  const unassigned = await screen.findByRole('list', { name: 'Kabupaten di Belum ada zona' });
  expect(within(unassigned).getByText('Bone')).toBeVisible();
  expect(within(screen.getByRole('list', { name: 'Kabupaten di ZONA 1' })).getByText('Wajo')).toBeVisible();

  await userEvent.click(screen.getByRole('checkbox', { name: 'Pilih Bone' }));
  expect(screen.getByText('1 kabupaten dipilih')).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Pindahkan ke zona' })).toBeVisible();

  await userEvent.type(screen.getByLabelText('Cari kabupaten'), 'waj');
  expect(within(unassigned).queryByText('Bone')).not.toBeInTheDocument();

  // Kolom bawaan berurutan alami; zona berisi tetap dapat dihapus setelah konfirmasi.
  expect(screen.getAllByRole('listitem', { name: /kabupaten$/ }).map((column) => column.getAttribute('aria-label'))).toEqual(['Belum ada zona, 1 kabupaten', 'ZONA 1, 1 kabupaten', 'ZONA 2, 0 kabupaten'])
  await userEvent.click(screen.getByRole('button', { name: 'Hapus ZONA 1' }));
  expect(await screen.findByText('1 kabupaten di zona ini akan dipindah ke Belum ada zona, dan No. PO zona ini ikut terhapus.')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
  await userEvent.click(screen.getByRole('button', { name: 'Hapus ZONA 2' }));
  await userEvent.click(await screen.findByRole('button', { name: 'Hapus zona' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/program-setup/programs/program-1/zones/zone-2', expect.objectContaining({ method: 'DELETE' })));
});

test('renders a zone with no regencies when the API omits the empty collection', async () => {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path.endsWith('/programs')) return Promise.resolve({ data: [{ id: 'program-1', code: 'PETANI-2026', name: 'Tender Petani', program_type: 'farmer', status: 'active' }] });
    if (path.endsWith('/regencies')) return Promise.resolve({ data: [] });
    if (path.endsWith('/programs/program-1/zones')) return Promise.resolve({ data: [{ id: 'zone-1', name: 'ZONA 1', code: 'ZONA-1', is_placeholder: false }] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  render(<MemoryRouter><QueryClientProvider client={client}><PermissionsProvider permissions={['programs.view']}><ZonesPanel /></PermissionsProvider></QueryClientProvider></MemoryRouter>);

  expect(await screen.findByText('ZONA 1')).toBeVisible();
  expect(within(screen.getByRole('list', { name: 'Kabupaten di ZONA 1' })).getByText('Belum ada kabupaten.')).toBeVisible();
});
