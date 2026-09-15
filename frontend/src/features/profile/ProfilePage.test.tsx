import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, vi, test, expect } from 'vitest';
import { apiRequest, getBootstrap } from '../../lib/api';
import { ProfilePage } from './ProfilePage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn(), getBootstrap: vi.fn() }));

const account = { data: { id: 'u1', full_name: 'Admin Konkit', username: 'admin', email: 'admin@test.id', roles: ['super_admin'], permissions: ['*'], is_active: true, last_login_at: '2026-09-11T03:00:00Z' }, meta: { csrf_token: 'x' } };

beforeEach(() => vi.clearAllMocks());

test('combines account identity, profile editing, and password security in one page', async () => {
  vi.mocked(getBootstrap).mockResolvedValue(account);
  vi.mocked(apiRequest).mockResolvedValue({ data: { full_name: 'Admin Program' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><ProfilePage /></QueryClientProvider>);
  const name = await screen.findByLabelText('Nama lengkap');
  expect(screen.getByRole('heading', { name: 'Profil saya' })).toBeInTheDocument();
  expect(await screen.findByText('Aktif')).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Informasi profil' })).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Keamanan akun' })).toBeInTheDocument();
  expect(screen.getByLabelText('Password saat ini')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Ubah password' })).toBeInTheDocument();
  await userEvent.clear(name);
  await userEvent.type(name, 'Admin Program');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan perubahan' }));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/me', expect.objectContaining({ method: 'PATCH' }));
});

test('changes the password independently from profile information', async () => {
  vi.mocked(getBootstrap).mockResolvedValue(account);
  vi.mocked(apiRequest).mockResolvedValue({ data: { message: 'ok' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><ProfilePage /></QueryClientProvider>);

  await screen.findByDisplayValue('Admin Konkit');
  await userEvent.type(screen.getByLabelText('Password saat ini'), 'Current-password-2026');
  await userEvent.type(screen.getByLabelText('Password baru'), 'New-password-2026');
  await userEvent.type(screen.getByLabelText('Konfirmasi password baru'), 'New-password-2026');
  await userEvent.click(screen.getByRole('button', { name: 'Ubah password' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledOnce());
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/me/password', {
    method: 'PUT',
    body: JSON.stringify({ current_password: 'Current-password-2026', new_password: 'New-password-2026' }),
  });
  expect(screen.getByDisplayValue('Admin Konkit')).toBeInTheDocument();
});
