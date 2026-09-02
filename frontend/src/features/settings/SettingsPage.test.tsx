import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { SettingsPage } from './SettingsPage';
vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
test('loads typed settings and saves changes', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: [{ key: 'application_name', value: 'Konkit Gas', type: 'string', description: '' }, { key: 'timezone', value: 'Asia/Jakarta', type: 'timezone', description: '' }] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><SettingsPage /></QueryClientProvider>);
  const name = await screen.findByLabelText('Nama aplikasi');
  await userEvent.clear(name); await userEvent.type(name, 'Konkit Nasional');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan pengaturan' }));
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/system/settings', expect.objectContaining({ method: 'PATCH' }));
});
