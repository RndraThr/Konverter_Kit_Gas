import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { AuditPage } from './AuditPage';
vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
test('renders audit history as read-only data', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { items: [{ id: 'a1', actor_name: 'Admin Konkit', action: 'settings.updated', resource_type: 'settings', resource_id: '', metadata: {}, created_at: '2026-09-02T12:00:00Z' }], total: 1, page: 1, page_size: 20 } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><AuditPage /></MemoryRouter></QueryClientProvider>);
  expect(await screen.findByText('settings.updated')).toBeInTheDocument();
  expect(screen.getByText('Admin Konkit')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /hapus/i })).not.toBeInTheDocument();
});
