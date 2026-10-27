import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { uploadRequest } from '../../lib/upload';
import { AppReleasesPanel, type AppRelease } from './AppReleasesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));
vi.mock('../../lib/upload', () => ({ uploadRequest: vi.fn() }));

const releases: AppRelease[] = [
  { id: 'r3', version_name: '1.2.0', version_code: 3, notes: 'Scan lebih cepat', mandatory: true, size_bytes: 52428800, uploaded_by_name: 'Admin', created_at: '2026-10-27T09:00:00Z' },
  { id: 'r2', version_name: '1.1.0', version_code: 2, notes: '', mandatory: false, size_bytes: 50331648, created_at: '2026-10-20T09:00:00Z' },
];

function renderPanel(permissions = ['settings.view', 'settings.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><AppReleasesPanel /></PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiRequest).mockResolvedValue({ data: releases });
  vi.mocked(uploadRequest).mockResolvedValue({ data: { ...releases[0], id: 'r4', version_name: '1.3.0', version_code: 4 } });
});

test('lists releases newest first with mandatory and latest marks', async () => {
  renderPanel();
  expect(await screen.findByText('Scan lebih cepat')).toBeVisible();
  expect(screen.getByText('Terbaru')).toBeVisible();
  expect(screen.getByText('Wajib')).toBeVisible();
});

test('uploads a new release only with a version code above the latest', async () => {
  const user = userEvent.setup();
  renderPanel();
  await screen.findByText('Scan lebih cepat');
  const submit = screen.getByRole('button', { name: 'Rilis versi' });

  await user.type(screen.getByLabelText('Nama versi'), '1.3.0');
  await user.type(screen.getByLabelText('Kode versi'), '3');
  await user.upload(screen.getByLabelText('Berkas APK'), new File(['PK'], 'konkit.apk', { type: 'application/vnd.android.package-archive' }));
  expect(submit).toBeDisabled();
  expect(screen.getByText('Harus lebih dari 3.')).toBeVisible();

  await user.clear(screen.getByLabelText('Kode versi'));
  await user.type(screen.getByLabelText('Kode versi'), '4');
  await user.click(screen.getByText('Wajib dipasang'));
  expect(submit).toBeEnabled();
  await user.click(submit);

  expect(uploadRequest).toHaveBeenCalledTimes(1);
  const form = vi.mocked(uploadRequest).mock.calls[0][1];
  expect(form.get('version_name')).toBe('1.3.0');
  expect(form.get('version_code')).toBe('4');
  expect(form.get('mandatory')).toBe('true');
});

test('read-only accounts see the list without the upload form', async () => {
  renderPanel(['settings.view']);
  await screen.findByText('Scan lebih cepat');
  expect(screen.queryByRole('button', { name: 'Rilis versi' })).not.toBeInTheDocument();
});
