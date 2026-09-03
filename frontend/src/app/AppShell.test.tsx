import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppShell } from './AppShell';

const bootstrap = {
  data: {
    id: 'user-1',
    full_name: 'Admin Konkit',
    username: 'admin',
    email: 'admin@konkit.test',
    roles: ['super_admin'],
    permissions: ['dashboard.view', 'dcp3.view', 'distribution.view', 'users.view', 'settings.view'],
  },
  meta: { csrf_token: 'csrf-token' },
};

function renderShell() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/']}>
        <AppShell />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe('AppShell', () => {
  it('renders identity and permission-aware navigation', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }));

    renderShell();

    expect(await screen.findByText('Admin Konkit')).toBeInTheDocument();
    expect(screen.getByAltText('Ergas')).toBeInTheDocument();
    expect(screen.getByAltText('PT Kian Santang Mulitama Tbk')).toBeInTheDocument();
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Administrasi')).toBeInTheDocument();
    expect(screen.getByText('Sistem')).toBeInTheDocument();
    expect(screen.getByText('Akun')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'DCP3' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Pendistribusian' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Pengguna' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Role & akses' })).not.toBeInTheDocument();
  });

  it('opens the mobile navigation from an accessible button', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));
    renderShell();
    await screen.findByText('Admin Konkit');

    const trigger = screen.getByRole('button', { name: 'Buka navigasi' });
    await userEvent.click(trigger);
    expect(screen.getByRole('dialog', { name: 'Navigasi utama' })).toBeInTheDocument();
  });

  it('redirects to login when bootstrap returns 401', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized' } }), { status: 401 }));
    const location = window.location;
    Object.defineProperty(window, 'location', { configurable: true, value: { ...location, assign: vi.fn() } });

    renderShell();
    await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/login'));

    Object.defineProperty(window, 'location', { configurable: true, value: location });
  });
});
