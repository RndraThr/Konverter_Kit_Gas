import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { RolesPage } from './RolesPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('marks system roles as protected', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => path.includes('/permissions') ? { data: [] } : { data: [{ id: 'r1', code: 'super_admin', name: 'Super Admin', is_system: true, permissions: [], user_count: 1 }] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><RolesPage /></QueryClientProvider>);
  expect(await screen.findByText('Super Admin')).toBeInTheDocument();
  expect(screen.getByText('Role sistem')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Edit Super Admin' })).toBeDisabled();
});
