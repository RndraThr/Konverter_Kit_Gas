import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { dashboardRoutes } from './routes';

const bootstrap = {
  data: {
    id: 'user-1',
    full_name: 'Admin Konkit',
    username: 'admin',
    email: 'admin@konkit.test',
    roles: ['super_admin'],
    permissions: ['dashboard.view', 'distribution.view'],
  },
  meta: { csrf_token: 'csrf-token' },
};

afterEach(() => vi.restoreAllMocks());

test('opens Map Distribusi as its own dashboard feature', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(dashboardRoutes, { initialEntries: ['/map-distribusi'] });

  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>);

  expect(await screen.findByRole('heading', { name: 'Map Distribusi' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Ruang kerja Map Distribusi' })).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Area peta' })).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Kontrol peta' })).toBeInTheDocument();
  expect(screen.queryByText('Ringkasan program')).not.toBeInTheDocument();
});

test('redirects the old password URL to the unified profile page', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(dashboardRoutes, { initialEntries: ['/profil/password'] });

  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>);

  expect(await screen.findByRole('heading', { name: 'Profil saya' })).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Keamanan akun' })).toBeInTheDocument();
  expect(router.state.location.pathname).toBe('/profil');
});
