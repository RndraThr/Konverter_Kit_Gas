import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
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
    permissions: ['recipients.view', 'dashboard.view', 'programs.view', 'dcp3.view', 'distribution.view', 'activities.view', 'users.view', 'settings.view'],
  },
  meta: { csrf_token: 'csrf-token' },
};

function renderShell(initialEntry = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return { ...render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <AppShell />
      </MemoryRouter>
    </QueryClientProvider>,
  ), queryClient };
}

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

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
    expect(screen.getByRole('button', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByText('Administrasi')).toBeInTheDocument();
    expect(screen.getByText('Sistem')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Data Penerima' })).toHaveAttribute('href', '/');
    expect(screen.getByRole('link', { name: 'Map Distribusi' })).toHaveAttribute('href', '/map-distribusi');
    expect(screen.queryByRole('link', { name: 'Ringkasan' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Operasional' }));
    expect(screen.getByRole('link', { name: 'DCP3' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Dokumentasi' }));
    expect(screen.getByRole('link', { name: 'Pendistribusian' })).toHaveAttribute('href', '/dokumentasi/pendistribusian');
    expect(screen.getByRole('link', { name: 'Ceremony & Sosialisasi' })).toHaveAttribute('href', '/dokumentasi/ceremony-sosialisasi');
    expect(screen.getByRole('link', { name: 'Unloading' })).toHaveAttribute('href', '/dokumentasi/unloading');
    await userEvent.click(screen.getByRole('button', { name: 'Administrasi' }));
    expect(screen.getByRole('link', { name: 'Pengguna' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Profil saya' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Ubah password' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Role & akses' })).not.toBeInTheDocument();
  });

  it('opens the mobile navigation from an accessible button', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));
    renderShell();
    await screen.findByText('Admin Konkit');

    const trigger = screen.getByRole('button', { name: 'Buka navigasi' });
    await userEvent.click(trigger);
    const mobileNavigation = screen.getByRole('dialog', { name: 'Navigasi utama' });
    expect(mobileNavigation).toBeInTheDocument();

    await userEvent.click(within(mobileNavigation).getByRole('button', { name: 'Tutup navigasi' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Navigasi utama' })).not.toBeInTheDocument());
  });

  it('shows the current page title and breadcrumb in the desktop header', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));

    renderShell('/persiapan-program');

    const pageContext = await screen.findByRole('group', { name: 'Konteks halaman' });
    expect(within(pageContext).getByText('Persiapan program')).toBeInTheDocument();
    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(breadcrumb).getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/');
    expect(within(breadcrumb).getByText('Operasional')).toBeInTheDocument();
  });

  it('collapses a navigation group and remembers the preference', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify(bootstrap), { status: 200 }));
    const firstRender = renderShell('/');
    await screen.findByText('Admin Konkit');

    const operational = screen.getByRole('button', { name: 'Operasional' });
    expect(operational).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(operational);
    expect(operational).toHaveAttribute('aria-expanded', 'true');
    await userEvent.click(operational);

    expect(operational).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'DCP3' })).not.toBeInTheDocument();
    expect(localStorage.getItem('konkit.sidebar.closed-groups')).toContain('Operasional');

    firstRender.unmount();
    renderShell('/persiapan-program');
    await screen.findByText('Admin Konkit');

    expect(screen.getByRole('button', { name: 'Operasional' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Persiapan program' })).toBeInTheDocument();
  });

  it('keeps drawer navigation available through tablet widths', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));
    renderShell();
    await screen.findByText('Admin Konkit');

    expect(screen.getByRole('button', { name: 'Buka navigasi' })).toHaveClass('lg:hidden');
    expect(screen.getByLabelText('Sidebar utama')).toHaveClass('lg:flex');
  });

  it('keeps inactive navigation groups closed by default and opens the active documentation group', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify(bootstrap), { status: 200 }));
    const firstRender = renderShell('/');
    await screen.findByText('Admin Konkit');

    expect(screen.getByRole('button', { name: 'Dokumentasi' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'Ceremony & Sosialisasi' })).not.toBeInTheDocument();

    firstRender.unmount();
    renderShell('/dokumentasi/unloading?type=unloading_konkit');
    await screen.findByText('Admin Konkit');

    expect(screen.getByRole('button', { name: 'Dokumentasi' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.queryByText('Kegiatan')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Unloading' })).toHaveClass('bg-sidebar-accent');
    expect(within(screen.getByRole('group', { name: 'Konteks halaman' })).getByText('Unloading', { selector: 'p' })).toBeInTheDocument();
  });

  it('falls back to closed navigation groups when the saved preference is corrupt', async () => {
    localStorage.setItem('konkit.sidebar.closed-groups', '{invalid');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));

    renderShell();

    expect(await screen.findByRole('button', { name: 'Operasional' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('reports a disrupted connection when the liveness check fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      if (String(input).endsWith('/api/v1/health')) throw new TypeError('network unavailable');
      return new Response(JSON.stringify(bootstrap), { status: 200 });
    });

    renderShell();

    expect(await screen.findByText('Koneksi terganggu')).toBeInTheDocument();
    expect(screen.queryByText('Sistem terhubung')).not.toBeInTheDocument();
  });

  it('shows a checking state while refreshing a previously successful connection', async () => {
    let healthChecks = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      if (String(input).endsWith('/api/v1/health')) {
        healthChecks += 1;
        if (healthChecks === 1) return new Response(JSON.stringify({ status: 'ok' }), { status: 200 });
        return new Promise(() => undefined);
      }
      return new Response(JSON.stringify(bootstrap), { status: 200 });
    });

    const { queryClient } = renderShell();
    expect(await screen.findByText('Sistem terhubung')).toBeInTheDocument();

    void queryClient.invalidateQueries({ queryKey: ['liveness'] });
    expect(await screen.findByText('Memeriksa koneksi')).toBeInTheDocument();
  });

  it('minimizes the desktop sidebar and remembers the preference', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify(bootstrap), { status: 200 }));
    const firstRender = renderShell();
    await screen.findByText('Admin Konkit');

    const sidebar = screen.getByLabelText('Sidebar utama');
    const fullBrand = screen.getByAltText('PT Kian Santang Mulitama Tbk');
    const minimize = screen.getByRole('button', { name: 'Minimalkan sidebar' });
    expect(sidebar).toHaveAttribute('id', 'primary-sidebar');
    expect(minimize).toHaveAttribute('aria-controls', 'primary-sidebar');
    expect(minimize).toHaveClass('top-5');

    await userEvent.click(minimize);
    expect(sidebar).toHaveAttribute('data-state', 'collapsed');
    const maximize = screen.getByRole('button', { name: 'Maksimalkan sidebar' });
    expect(maximize).toHaveAttribute('aria-controls', 'primary-sidebar');
    expect(maximize).toHaveClass('top-5');
    expect(fullBrand).toBeInTheDocument();
    expect(fullBrand).toHaveAttribute('aria-hidden', 'true');
    const recipientLink = screen.getByRole('link', { name: 'Data Penerima' });
    expect(recipientLink).toBeInTheDocument();
    expect(within(recipientLink).getByText('Data Penerima')).toHaveClass('max-w-0', 'opacity-0');
    expect(within(recipientLink).getByText('Data Penerima')).not.toHaveClass('sr-only');
    expect(screen.getByText('Sistem terhubung')).toBeInTheDocument();
    await userEvent.hover(recipientLink);
    expect(recipientLink).toHaveAttribute('data-popup-open');
    expect(localStorage.getItem('konkit.sidebar.collapsed')).toBe('true');

    firstRender.unmount();
    renderShell();
    await screen.findByText('Admin Konkit');
    expect(screen.getByLabelText('Sidebar utama')).toHaveAttribute('data-state', 'collapsed');
    expect(screen.getByRole('button', { name: 'Maksimalkan sidebar' })).toBeInTheDocument();
  });

  it('keeps unloading available as one item in collapsed navigation', async () => {
    localStorage.setItem('konkit.sidebar.collapsed', 'true');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));

    renderShell('/dokumentasi/unloading?type=unloading_mesin_pompa');

    expect(await screen.findByRole('link', { name: 'Unloading' })).toHaveAttribute('href', '/dokumentasi/unloading');
  });

  it('keeps the loading sidebar compact when the collapsed preference is restored', () => {
    localStorage.setItem('konkit.sidebar.collapsed', 'true');
    vi.spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(() => undefined));

    renderShell();

    const sidebar = screen.getByLabelText('Sidebar utama');
    expect(sidebar).toHaveAttribute('data-state', 'collapsed');
    expect(sidebar).toHaveClass('p-2');
    expect(sidebar.querySelector('[data-slot="skeleton"]')).toHaveClass('w-8');
  });

  it('exposes the workspace and account actions through landmarks', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));
    renderShell();

    expect(await screen.findByRole('navigation', { name: 'Navigasi utama' })).toBeInTheDocument();
    expect(screen.getByRole('main')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Menu akun' }));

    expect(screen.getByRole('menuitem', { name: 'Profil saya' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Keluar' })).toBeInTheDocument();
    expect(consoleError.mock.calls.flat().join(' ')).not.toContain('expected a non-<button>');
  });

  it('redirects to login when bootstrap returns 401', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized' } }), { status: 401 }));
    const location = window.location;
    Object.defineProperty(window, 'location', { configurable: true, value: { ...location, assign: vi.fn() } });

    renderShell();
    await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/login?notice=session_expired'));

    Object.defineProperty(window, 'location', { configurable: true, value: location });
  });
});
