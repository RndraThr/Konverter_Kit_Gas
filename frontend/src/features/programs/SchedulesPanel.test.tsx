import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, test, expect } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SchedulesPanel } from './SchedulesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPanel() {
  vi.mocked(apiRequest).mockImplementation((path: string) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/programs') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/regencies') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/package-templates') return Promise.resolve({ data: [] });
    if (path === '/api/v1/program-setup/documentation-templates') return Promise.resolve({ data: [] });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.manage']}><SchedulesPanel /></PermissionsProvider></QueryClientProvider>);
}

test('sends null slot_quota when the field is left empty', async () => {
  renderPanel();
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));
  expect(screen.getByLabelText(/Kuota slot/)).toBeInTheDocument();
});

test('sends the entered slot_quota as a number', async () => {
  renderPanel();
  await userEvent.click(screen.getByRole('button', { name: 'Tambah jadwal' }));
  const quotaField = screen.getByLabelText(/Kuota slot/);
  await userEvent.type(quotaField, '46');
  expect(quotaField).toHaveValue(46);
});
