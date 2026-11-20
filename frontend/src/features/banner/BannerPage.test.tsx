import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiBlobRequest, apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { BannerPage } from './BannerPage';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn(), apiBlobRequest: vi.fn() };
});
vi.mock('@/features/berita-acara/PdfCanvasPreview', () => ({ default: ({ label }: { label: string }) => <div>{label}</div> }));

const overview = {
  schedule_id: 'schedule-1', schedule_name: 'Bone 2026', regency_name: 'KAB. BONE', display_name: 'Kabupaten Bone', short_name: 'Bone',
  supported: true, brand: 'shark', has_logo: true, logo_updated_at: '2026-11-19T00:00:00Z',
  master_service_center: { name: 'Bengkel Shark', address: 'Jl. Sawah', phone: '0811' },
  service_center: { name: 'Bengkel Shark', address: 'Jl. Sawah', phone: '0811' },
  settings: { service_center_name: '', service_center_address: '', service_center_phone: '', seremoni_name: '', plakat_name: '', plakat_place: '', plakat_date: '' },
  style_defaults: { seremoni_name: '#232C7A', plakat_date: '#222222', plakat_name: '#232C7A', backdrop_service: '#1D1819' },
  kinds: [
    { kind: 'backdrop', label: 'Backdrop', size: '300 x 200 cm', filename: 'BANNER BACKDROP PETANI KABUPATEN BONE.pdf' },
    { kind: 'seremoni', label: 'Seremoni', size: '400 x 250 cm', filename: 'BANNER SEREMONI PETANI KABUPATEN BONE.pdf' },
    { kind: 'plakat', label: 'Plakat', size: '60 x 40 cm', filename: 'PLAKAT PETANI KABUPATEN BONE.pdf' },
  ],
};

afterEach(() => vi.clearAllMocks());

function renderPage(permissions = ['banner.view', 'banner.manage']) {
  vi.mocked(apiRequest).mockImplementation((path, init) => {
    if (path === '/api/v1/program-setup/schedules') return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Bone 2026', status: 'active', regency: { id: 'r1', name: 'KAB. BONE' } }] });
    if (path === '/api/v1/banners?schedule_id=schedule-1') return Promise.resolve({ data: overview });
    if (path === '/api/v1/banners/settings?schedule_id=schedule-1' && init?.method === 'PUT') return Promise.resolve({ data: { ...overview, settings: { ...overview.settings, seremoni_name: 'Ir. Lamhot' } } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF'], { type: 'application/pdf' }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><BannerPage /></PermissionsProvider></QueryClientProvider>);
}

test('fills the backdrop from the master data, saves the names, and previews each banner', async () => {
  renderPage();
  const schedule = await screen.findByRole('combobox', { name: 'Jadwal' });
  await userEvent.click(schedule);
  await userEvent.click(await screen.findByRole('option', { name: 'KAB. BONE / Bone 2026' }));

  expect(await screen.findByDisplayValue('Bengkel Shark')).toBeVisible();
  expect(screen.getByText('Data master Shark')).toBeVisible();
  expect(await screen.findByText('Backdrop Kabupaten Bone')).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Unduh PDF' })).toHaveAttribute('href', '/api/v1/banners/backdrop.pdf?schedule_id=schedule-1');

  // Each banner has its own tab with its fields; Pelatihan Teknis has none.
  expect(screen.queryByRole('textbox', { name: /^Nama$/ })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('tab', { name: 'Seremoni' }));
  fireEvent.change(screen.getByRole('textbox', { name: /^Nama$/ }), { target: { value: 'Ir. Lamhot' } });
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/banners/settings?schedule_id=schedule-1', expect.objectContaining({ method: 'PUT', body: expect.stringContaining('"seremoni_name":"Ir. Lamhot"') })));

  await userEvent.click(screen.getByRole('tab', { name: 'Plakat' }));
  expect(await screen.findByText('Plakat Kabupaten Bone')).toBeInTheDocument();
  expect(screen.getByText(/Nama atau tanggal plakat belum diisi/)).toBeVisible();
});

test('viewers see the banners without editing', async () => {
  renderPage(['banner.view']);
  await userEvent.click(await screen.findByRole('combobox', { name: 'Jadwal' }));
  await userEvent.click(await screen.findByRole('option', { name: 'KAB. BONE / Bone 2026' }));
  expect(await screen.findByDisplayValue('Bengkel Shark')).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Simpan' })).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Pilih logo kabupaten')).not.toBeInTheDocument();
});

test('changes the size and color of the seremoni name', async () => {
  renderPage();
  await userEvent.click(await screen.findByRole('combobox', { name: 'Jadwal' }));
  await userEvent.click(await screen.findByRole('option', { name: 'KAB. BONE / Bone 2026' }));
  await userEvent.click(await screen.findByRole('tab', { name: 'Seremoni' }));
  fireEvent.change(screen.getByLabelText('Ukuran Nama'), { target: { value: '120' } });
  fireEvent.input(screen.getByLabelText('Warna Nama'), { target: { value: '#8b0000' } });
  expect(screen.getByText('120%')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/banners/settings?schedule_id=schedule-1', expect.objectContaining({
    method: 'PUT', body: expect.stringContaining('"seremoni_name":{"size_percent":120,"color":"#8B0000"}'),
  })));
});
