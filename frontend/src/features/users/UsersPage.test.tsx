import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { UsersPage } from './UsersPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('lists users and opens the create dialog', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => path.includes('/role-options')
    ? { data: [] }
    : { data: { items: [{ id: 'u1', full_name: 'Petugas Wajo', username: 'wajo', email: 'wajo@test.id', is_active: true, roles: [] }], total: 1, page: 1, page_size: 20 } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><UsersPage /></MemoryRouter></QueryClientProvider>);
  expect(await screen.findByText('Petugas Wajo')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Tambah pengguna' }));
  expect(screen.getByRole('dialog', { name: 'Tambah pengguna' })).toBeInTheDocument();
});

test('hides mutation controls for read-only users', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => path.includes('/role-options')
    ? { data: [] }
    : { data: { items: [{ id: 'u1', full_name: 'Petugas Wajo', username: 'wajo', email: 'wajo@test.id', is_active: true, roles: [] }], total: 1, page: 1, page_size: 20 } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><PermissionsProvider permissions={['users.view']}><UsersPage /></PermissionsProvider></MemoryRouter></QueryClientProvider>);
  expect(await screen.findByText('Petugas Wajo')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Tambah pengguna' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit Petugas Wajo' })).not.toBeInTheDocument();
});

test('shows an API error when creating a user fails', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/role-options')) return { data: [] };
    if (init?.method === 'POST') throw new Error('Username atau email sudah digunakan');
    return { data: { items: [], total: 0, page: 1, page_size: 20 } };
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><UsersPage /></MemoryRouter></QueryClientProvider>);
  await userEvent.click(await screen.findByRole('button', { name: 'Tambah pengguna' }));
  await userEvent.type(screen.getByLabelText('Nama lengkap'), 'Petugas Baru');
  await userEvent.type(screen.getByLabelText('Username'), 'petugas.baru');
  await userEvent.type(screen.getByLabelText('Email'), 'petugas@konkit.test');
  await userEvent.type(document.querySelector<HTMLInputElement>('input[name="password"]')!, 'Password-awal-2026');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan pengguna' }));
  expect(await screen.findByText('Username atau email sudah digunakan')).toBeInTheDocument();
});
