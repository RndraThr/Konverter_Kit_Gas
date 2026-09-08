import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { HealthPage } from './HealthPage';
vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
test('shows safe dependency health without credentials', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { status: 'healthy', database: { status: 'healthy', message: 'PostgreSQL siap', latency_ms: 2 }, migration_version: 2, environment: 'local', version: 'dev', uptime_seconds: 60, checked_at: '2026-09-02T12:00:00Z' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);
  expect(await screen.findByText('PostgreSQL siap')).toBeInTheDocument();
  expect(screen.queryByText(/postgres:\/\//)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Periksa ulang' })).toBeInTheDocument();
});

test('renders degraded reports returned with readiness status 503', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { status: 'degraded', database: { status: 'unhealthy', message: 'Database tidak terhubung', latency_ms: 0 }, migration_version: 3, environment: 'local', version: 'dev', uptime_seconds: 60, checked_at: '2026-09-03T00:00:00Z' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><HealthPage /></QueryClientProvider>);
  expect(await screen.findByText('Perlu perhatian')).toBeInTheDocument();
  expect(screen.getByText('Database tidak terhubung')).toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent('Perlu perhatian');
  expect(screen.getByRole('button', { name: 'Periksa ulang' })).toBeInTheDocument();
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/system/health', { acceptedStatuses: [503] });
});
