import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
  await userEvent.click(screen.getByRole('button', { name: 'Aksi Super Admin' }));
  expect(screen.getByRole('menuitem', { name: 'Edit Super Admin' })).toHaveAttribute('aria-disabled', 'true');
});

test('shows assigned regencies and toggles the all-regencies checkbox', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    const url = path as string;
    if (url.includes('/permissions')) return { data: [] };
    if (url.includes('/program-setup/regencies')) return { data: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }, { id: 'regency-2', name: 'Bone', document_code: 'BON' }] };
    return { data: [{ id: 'role-1', code: 'petugas_wajo', name: 'Petugas Wajo', is_system: false, permissions: [], all_regencies_access: false, regencies: [{ id: 'regency-1', name: 'Wajo', document_code: 'WJO' }], user_count: 0 }] };
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><RolesPage /></QueryClientProvider>);

  await userEvent.click(await screen.findByRole('button', { name: 'Aksi Petugas Wajo' }));
  await userEvent.click(screen.getByRole('menuitem', { name: 'Edit Petugas Wajo' }));

  const dialog = screen.getByRole('dialog', { name: 'Edit Petugas Wajo' });
  expect(dialog.querySelector('[data-slot="dialog-footer"]')).toHaveClass('mx-0', 'mb-0');

  expect(screen.getByRole('checkbox', { name: 'Wajo' })).toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Bone' })).not.toBeChecked();

  await userEvent.click(screen.getByRole('checkbox', { name: 'Akses semua kabupaten' }));
  expect(screen.queryByRole('checkbox', { name: 'Wajo' })).not.toBeInTheDocument();
});

test('uses named role actions and confirms deletion', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => path.includes('/permissions') ? { data: [] } : { data: [{ id: 'r1', code: 'petugas_wajo', name: 'Petugas Wajo', is_system: false, permissions: [], all_regencies_access: true, regencies: [], user_count: 0 }] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><RolesPage /></QueryClientProvider>);
  await userEvent.click(await screen.findByRole('button', { name: 'Aksi Petugas Wajo' }));
  expect(screen.getByRole('menuitem', { name: 'Edit Petugas Wajo' })).toBeInTheDocument();
  await userEvent.click(screen.getByRole('menuitem', { name: 'Hapus Petugas Wajo' }));
  expect(screen.getByRole('alertdialog', { name: 'Hapus role Petugas Wajo?' })).toBeInTheDocument();
});
