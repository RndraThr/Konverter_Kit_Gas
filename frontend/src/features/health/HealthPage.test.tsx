import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { HealthPage } from './HealthPage';
vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

const healthyReport = {
  status: 'healthy' as const,
  database: { status: 'healthy', message: 'PostgreSQL siap', latency_ms: 2 },
  operations: { status: 'healthy', message: 'Statistik operasional siap', latency_ms: 0 },
  migration_version: 56,
  environment: 'staging',
  version: 'dev',
  storage_backend: 'gdrive',
  media_worker_status: 'active',
  media_moves: { queued: 3, processing: 1, retry: 2, failed: 4 },
  uptime_seconds: 3660,
  checked_at: '2026-09-02T12:00:00Z',
};

test('shows safe dependency health without credentials', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: healthyReport });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);
  expect(await screen.findByText('PostgreSQL siap')).toBeInTheDocument();
  expect(screen.queryByText(/postgres:\/\//)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Periksa ulang' })).toBeInTheDocument();
});

test('shows operational metrics and refreshes on demand', async () => {
  const user = userEvent.setup();
  vi.mocked(apiRequest).mockResolvedValue({ data: healthyReport });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);

  expect(await screen.findByText('Sistem sehat')).toBeInTheDocument();
  expect(screen.getByText('Google Drive')).toBeInTheDocument();
  expect(screen.getByText('3 menunggu')).toBeInTheDocument();
  expect(screen.getByText('1 diproses')).toBeInTheDocument();
  expect(screen.getByText('2 dijadwalkan ulang')).toBeInTheDocument();
  expect(screen.getByText('4 gagal')).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Periksa ulang' }));
  expect(await screen.findByText(/Terakhir diperiksa/)).toBeInTheDocument();
  expect(apiRequest).toHaveBeenCalledTimes(2);
});

test('uses labelled regions for responsive health content', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: healthyReport });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);

  expect(await screen.findByRole('region', { name: 'Ringkasan kesehatan' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Antrean pemindahan media' })).toBeInTheDocument();
});

test('renders degraded reports returned with readiness status 503', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...healthyReport, status: 'degraded', database: { status: 'unhealthy', message: 'Database tidak terhubung', latency_ms: 0 }, operations: { status: 'degraded', code: 'operational_stats_unavailable', message: 'Statistik operasional tidak dapat dibaca', latency_ms: 0 } } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);
  expect(await screen.findByText('Perlu perhatian')).toBeInTheDocument();
  expect(screen.getByText('Database tidak terhubung')).toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent('Perlu perhatian');
  expect(screen.getByRole('button', { name: 'Periksa ulang' })).toBeInTheDocument();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/system/health', { acceptedStatuses: [503] });
});
