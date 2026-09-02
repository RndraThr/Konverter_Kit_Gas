import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { ChangePasswordForm } from './ChangePasswordForm';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

test('does not submit a password when confirmation differs', async () => {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><ChangePasswordForm /></QueryClientProvider>);
  await userEvent.type(screen.getByLabelText('Password saat ini'), 'Current-password-2026');
  await userEvent.type(screen.getByLabelText('Password baru'), 'New-password-2026');
  await userEvent.type(screen.getByLabelText('Konfirmasi password baru'), 'Different-password-2026');
  await userEvent.click(screen.getByRole('button', { name: 'Ubah password' }));
  expect(await screen.findByText('Konfirmasi password baru tidak sama.')).toBeInTheDocument();
  expect(apiRequest).not.toHaveBeenCalled();
});
