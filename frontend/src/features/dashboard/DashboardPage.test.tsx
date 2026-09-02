import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { DashboardPage } from './DashboardPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('shows operational counts from the API', async () => {
  vi.mocked(apiRequest)
    .mockResolvedValueOnce({ data: { items: [], total: 12, page: 1, page_size: 1 } })
    .mockResolvedValueOnce({ data: [{ id: 'r1' }, { id: 'r2' }] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><DashboardPage /></QueryClientProvider>);
  expect(await screen.findByText('12')).toBeInTheDocument();
  expect(screen.getByText('2')).toBeInTheDocument();
});
