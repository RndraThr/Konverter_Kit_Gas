import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '@/lib/api';
import { BeritaAcaraPage } from './BeritaAcaraPage';

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }));

const schedules = { data: [
  {
    id: 'schedule-farmer', program_id: 'program-farmer', regency_id: 'regency-wajo', name: 'Wajo Petani 2026',
    status: 'active', start_date: '2026-09-01T00:00:00Z', end_date: '2026-09-30T00:00:00Z', distribution_number_padding: 4,
    program: { id: 'program-farmer', code: 'PETANI-2026', name: 'Program Petani 2026', program_type: 'farmer', fiscal_year: 2026, status: 'active' },
    regency: { id: 'regency-wajo', province_name: 'Sulawesi Selatan', name: 'Wajo', document_code: 'WJO', is_active: true },
  },
  {
    id: 'schedule-fisherman', program_id: 'program-fisherman', regency_id: 'regency-bone', name: 'Bone Nelayan 2026',
    status: 'active', start_date: '2026-10-01T00:00:00Z', end_date: '2026-10-31T00:00:00Z', distribution_number_padding: 4,
    program: { id: 'program-fisherman', code: 'NELAYAN-2026', name: 'Program Nelayan 2026', program_type: 'fisherman', fiscal_year: 2026, status: 'active' },
    regency: { id: 'regency-bone', province_name: 'Sulawesi Selatan', name: 'Bone', document_code: 'BON', is_active: true },
  },
] };

function LocationProbe() {
  return <output aria-label="URL aktif">{useLocation().search}</output>;
}

function renderPage(initialEntry = '/berita-acara', request: () => Promise<unknown> = () => Promise.resolve(schedules)) {
  vi.mocked(apiRequest).mockImplementation(request as typeof apiRequest);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<MemoryRouter initialEntries={[initialEntry]}>
    <QueryClientProvider client={client}><BeritaAcaraPage /><LocationProbe /></QueryClientProvider>
  </MemoryRouter>);
}

test('shows the twelve document types as one Berita Acara tab workspace', async () => {
  renderPage();

  expect(await screen.findByRole('combobox', { name: 'Jadwal program' })).toBeVisible();
  for (const label of [
    'DP3', 'BA Perorangan', 'Rekap Harian', 'Closing Titik Serah', 'Closing Kabupaten', 'Rakorda',
    'Sosialisasi', 'Training 10%', 'Training 100%', 'BA Pemeriksaan', 'Servis Berkala', 'TKDN',
  ]) {
    expect(screen.getByRole('tab', { name: label })).toBeVisible();
  }
});

test('derives the Petani or Nelayan document variant from the selected schedule', async () => {
  renderPage('/berita-acara?schedule_id=schedule-fisherman&tab=ba-perorangan');

  expect(await screen.findByText('Nelayan')).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Jadwal program' })).toHaveTextContent('Bone Nelayan 2026');
  expect(screen.getByRole('tab', { name: 'BA Perorangan' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('region', { name: 'BA Perorangan Nelayan' })).toBeVisible();
  expect(screen.getByText('BA Perorangan Nelayan belum tersedia')).toBeVisible();
});

test('keeps the selected schedule and document tab in the URL', async () => {
  renderPage('/berita-acara?schedule_id=schedule-farmer&tab=dp3');

  expect(await screen.findByText('Petani')).toBeVisible();
  await userEvent.click(screen.getByRole('tab', { name: 'TKDN' }));

  expect(screen.getByLabelText('URL aktif')).toHaveTextContent('schedule_id=schedule-farmer');
  expect(screen.getByLabelText('URL aktif')).toHaveTextContent('tab=tkdn');
  expect(screen.getByRole('region', { name: 'TKDN Petani' })).toBeVisible();
});

test('shows a retryable error instead of presenting a failed schedule request as an empty list', async () => {
  let attempts = 0;
  renderPage('/berita-acara', () => attempts++ === 0 ? Promise.reject(new Error('forbidden')) : Promise.resolve(schedules));

  expect(await screen.findByText('Jadwal berita acara belum dapat dimuat')).toBeVisible();
  expect(screen.getByRole('combobox', { name: 'Jadwal program' })).toBeDisabled();
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));

  expect(await screen.findByRole('combobox', { name: 'Jadwal program' })).toBeEnabled();
  expect(screen.queryByText('Jadwal berita acara belum dapat dimuat')).not.toBeInTheDocument();
});
