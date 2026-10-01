import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { PermissionsProvider } from '@/lib/permissions';
import { apiRequest } from '@/lib/api';
import { LogoTenderPanel } from './LogoTenderPanel';

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn() }));

const logo = { id: 'logo-1', program_id: 'program-1', slot_code: 'pertamina', original_filename: 'pertamina.png', mime_type: 'image/png', byte_size: 1200, checksum: 'abc', sort_order: 1, max_width_mm: 35, max_height_mm: 18, is_visible: true };

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><LogoTenderPanel programID="program-1" /></PermissionsProvider></QueryClientProvider>);
}

test('menampilkan logo tender dan menyembunyikan kontrol kelola untuk pengguna lihat-saja', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: [logo] });
  renderPanel(['bast.view']);

  expect(await screen.findByRole('img', { name: 'pertamina.png' })).toHaveAttribute('src', '/api/v1/bast/branding/logos/logo-1/content?program_id=program-1');
  expect(screen.getByText('Logo Tender')).toBeVisible();
  expect(screen.queryByLabelText('Unggah logo tender')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Sembunyikan/ })).not.toBeInTheDocument();
});

test('menampilkan petunjuk konfigurasi saat tender belum memiliki logo', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: [] });
  renderPanel();

  expect(await screen.findByText('Belum ada logo tender')).toBeVisible();
  expect(screen.getByText(/unggah minimal satu logo aktif/i)).toBeVisible();
});

test('mengunggah multipart dan dapat mengubah visibilitas logo', async () => {
  let uploaded: FormData | undefined;
  let patched: Record<string, unknown> | undefined;
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path === '/api/v1/bast/branding?program_id=program-1') return { data: [logo] };
    if (init?.method === 'POST') { uploaded = init.body as FormData; return { data: logo }; }
    if (init?.method === 'PATCH') { patched = JSON.parse(String(init.body)); return { data: { ...logo, is_visible: false } }; }
    return { data: [] };
  });
  renderPanel();

  await screen.findByRole('img', { name: 'pertamina.png' });
  const file = new File(['png'], 'baru.png', { type: 'image/png' });
  await userEvent.upload(screen.getByLabelText('Unggah logo tender'), file);
  await waitFor(() => expect(uploaded?.get('program_id')).toBe('program-1'));
  expect(uploaded?.get('file')).toBe(file);
  expect(uploaded?.get('slot_code')).toBe('logo_2');

  await userEvent.click(screen.getByRole('button', { name: 'Sembunyikan pertamina.png' }));
  await waitFor(() => expect(patched).toMatchObject({ program_id: 'program-1', is_visible: false, max_width_mm: 35, max_height_mm: 18 }));
});
