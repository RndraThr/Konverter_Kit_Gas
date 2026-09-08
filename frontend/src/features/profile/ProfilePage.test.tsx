import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi, test, expect } from 'vitest';
import { apiRequest, getBootstrap } from '../../lib/api';
import { ProfilePage } from './ProfilePage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn(), getBootstrap: vi.fn() }));

test('updates the signed-in profile', async () => {
  vi.mocked(getBootstrap).mockResolvedValue({ data: { id: 'u1', full_name: 'Admin Konkit', username: 'admin', email: 'admin@test.id', roles: ['super_admin'], permissions: ['*'] }, meta: { csrf_token: 'x' } });
  vi.mocked(apiRequest).mockResolvedValue({ data: { full_name: 'Admin Program' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><ProfilePage /></QueryClientProvider>);
  const name = await screen.findByLabelText('Nama lengkap');
  expect(screen.getByRole('heading', { name: 'Profil saya' })).toBeInTheDocument();
  expect(await screen.findByText('Aktif')).toBeInTheDocument();
  await userEvent.clear(name);
  await userEvent.type(name, 'Admin Program');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan perubahan' }));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/me', expect.objectContaining({ method: 'PATCH' }));
});
