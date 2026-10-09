import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { AuditPage } from './AuditPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const response = {
  data: {
    items: [{
      id: 'a1', actor_user_id: 'user-1', actor_name: 'Admin Konkit', action: 'settings.updated',
      resource_type: 'system_settings', resource_id: 'application_name', metadata: { previous: 'Lama', current: 'Baru' },
      ip_address: '192.0.2.1', user_agent: 'Mobile Safari private-agent', created_at: '2026-10-09T12:00:00Z',
    }],
    total: 41, page: 1, page_size: 20, summary: { today: 7, system: 3 },
  },
};

function renderPage(initialEntry = '/dashboard/audit') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[initialEntry]}><AuditPage /></MemoryRouter></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiRequest).mockResolvedValue(response);
});

test('renders summaries, desktop table, mobile cards, and pagination', async () => {
  renderPage();
  expect(await screen.findByText('41 aktivitas')).toBeInTheDocument();
  expect(screen.getByText('7 hari ini')).toBeInTheDocument();
  expect(screen.getByText('3 oleh sistem')).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Tabel aktivitas' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Kartu aktivitas mobile' })).toBeInTheDocument();
  expect(screen.getByText('Halaman 1 dari 3')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /hapus aktivitas/i })).not.toBeInTheDocument();
});

test('applies URL-backed filters and removes active chips', async () => {
  const user = userEvent.setup();
  renderPage();
  await screen.findByText('41 aktivitas');

  await user.type(screen.getByLabelText('Cari aktivitas'), 'login');
  await user.click(screen.getByText('Filter lanjutan'));
  await user.type(screen.getByLabelText('Filter aksi'), 'auth.login');
  await user.type(screen.getByLabelText('Filter pelaku'), 'rendra');
  await user.type(screen.getByLabelText('Tanggal mulai'), '2026-10-01');
  await user.type(screen.getByLabelText('Tanggal akhir'), '2026-10-09');
  await user.selectOptions(screen.getByLabelText('Jumlah per halaman'), '50');
  await user.click(screen.getByRole('button', { name: 'Terapkan' }));

  await waitFor(() => {
    const lastPath = vi.mocked(apiRequest).mock.calls.at(-1)?.[0] ?? '';
    expect(lastPath).toContain('query=login');
    expect(lastPath).toContain('action=auth.login');
    expect(lastPath).toContain('actor=rendra');
    expect(lastPath).toContain('date_from=2026-10-01');
    expect(lastPath).toContain('date_to=2026-10-09');
    expect(lastPath).toContain('page_size=50');
    expect(lastPath).toContain('page=1');
  });
  expect(screen.getByText('Pencarian: login')).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Hapus filter Pencarian: login' }));
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.at(-1)?.[0]).not.toContain('query=login'));
});

test('keeps network details private until the detail sheet opens', async () => {
  const user = userEvent.setup();
  renderPage();
  const tableRegion = await screen.findByRole('region', { name: 'Tabel aktivitas' });
  expect(screen.queryByText('192.0.2.1')).not.toBeInTheDocument();
  expect(screen.queryByText('Mobile Safari private-agent')).not.toBeInTheDocument();

  await user.click(within(tableRegion).getByRole('button', { name: 'Lihat detail' }));
  expect(await screen.findByRole('dialog', { name: 'Detail aktivitas' })).toBeInTheDocument();
  expect(screen.getByText('192.0.2.1')).toBeInTheDocument();
  expect(screen.getByText('Mobile Safari private-agent')).toBeInTheDocument();
  expect(screen.getByText('Metadata mentah')).toBeInTheDocument();
});

test('shows a filter-aware empty state', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, summary: { today: 0, system: 0 } } });
  renderPage('/dashboard/audit?query=missing');
  expect(await screen.findByText('Belum ada aktivitas yang sesuai')).toBeInTheDocument();
  expect(screen.getByText(/Ubah atau reset filter/)).toBeInTheDocument();
});
