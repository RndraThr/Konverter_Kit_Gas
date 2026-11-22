import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { SuperDashboardPage } from './SuperDashboardPage';
import type { Dashboard, DashboardStages } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});
// jsdom has no layout: give the charts a fixed size.
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts');
  return { ...actual, ResponsiveContainer: ({ children }: { children: ReactNode }) => <div style={{ width: 800, height: 400 }}>{children}</div> };
});

const stages = (quota: number, linked: number, completed: number, approved: number, ba_final: number): DashboardStages => ({ quota, linked, completed, approved, ba_final });

const dashboard = (overrides: Partial<Dashboard> = {}): Dashboard => ({
  programs: [{ id: 'p1', name: 'Konkit 2026', fiscal_year: 2026, status: 'active' }, { id: 'p2', name: 'Konkit 2025', fiscal_year: 2025, status: 'closed' }],
  program_id: 'p1',
  totals: stages(1500, 1200, 1000, 800, 600),
  zones: [{ zone_name: 'Zona I', zone_sort: 1, ...stages(1000, 900, 800, 700, 600) }, { zone_name: '', zone_sort: 0, ...stages(500, 300, 200, 100, 0) }],
  regencies: [
    { schedule_id: 's1', regency_id: 'r1', regency_name: 'KAB. MALANG', province_name: 'JAWA TIMUR', zone_name: 'Zona I', zone_sort: 1, ...stages(1000, 900, 800, 700, 600) },
    { schedule_id: 's2', regency_id: 'r2', regency_name: 'KAB. TUBAN', province_name: 'JAWA TIMUR', zone_name: '', zone_sort: 0, ...stages(500, 300, 200, 100, 0) },
  ],
  daily: [{ date: '2026-11-20', completed: 400 }, { date: '2026-11-21', completed: 600 }],
  activities: [{ activity_type: 'rakor', media: 12 }, { activity_type: 'jenis_baru', media: 3 }],
  ...overrides,
});

afterEach(() => vi.mocked(apiRequest).mockReset());

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><SuperDashboardPage /></MemoryRouter></QueryClientProvider>);
}

test('shows the totals against the quota and groups the kabupaten per zone', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: dashboard() });
  renderPage();
  const card = await screen.findByRole('group', { name: 'Diserahkan' });
  expect(within(card).getByText('1.000')).toBeInTheDocument();
  expect(within(card).getByText('67% dari kuota')).toBeInTheDocument();
  expect(within(screen.getByRole('group', { name: 'Kuota' })).getByText('1.500')).toBeInTheDocument();
  const regencies = screen.getByRole('region', { name: 'Per kabupaten' });
  expect(within(regencies).getByRole('heading', { name: 'Zona I' })).toBeInTheDocument();
  expect(within(regencies).getByRole('heading', { name: 'Tanpa zona' })).toBeInTheDocument();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/dashboard/progress');
});

test('a program without kabupaten shows the empty state', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: dashboard({ regencies: [], zones: [], daily: [], activities: [], totals: stages(0, 0, 0, 0, 0) }) });
  renderPage();
  expect(await screen.findByText('Program ini belum punya jadwal kabupaten')).toBeInTheDocument();
});

test('a failed load can be tried again', async () => {
  vi.mocked(apiRequest).mockRejectedValueOnce(new Error('Server sibuk')).mockResolvedValue({ data: dashboard() });
  renderPage();
  expect(await screen.findByText('Server sibuk')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(await screen.findByRole('group', { name: 'Diserahkan' })).toBeInTheDocument();
});

test('choosing another program loads its numbers', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: dashboard() });
  renderPage();
  await userEvent.click(await screen.findByRole('combobox', { name: 'Program' }));
  await userEvent.click(await screen.findByRole('option', { name: 'Konkit 2025' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/dashboard/progress?program_id=p2'));
});
