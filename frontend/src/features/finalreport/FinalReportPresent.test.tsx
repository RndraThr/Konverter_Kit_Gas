import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import FinalReportPresent from './FinalReportPresent';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

afterEach(() => { vi.restoreAllMocks(); });

const previewData = {
  regency_label: 'KABUPATEN WAJO', fiscal_year: '2026', agriculture_office_name: 'Belum diisi',
  consultant_company_name: 'PT KSM', machine_manufacturer_name: 'Belum diisi',
  delivery_hose_date: 'Belum diisi', delivery_machine_group_date: 'Belum diisi', delivery_gas_cylinder_date: 'Belum diisi',
  delivery_range_start: 'Belum diisi', delivery_range_end: 'Belum diisi',
  rakorda_date: 'Belum diisi', sosialisasi_date: 'Belum diisi',
  leader_count: '0', admin_count: '0', technician_count: '0', helper_count: '0',
  handover_location: 'Belum diisi', distribution_start_date: 'Belum diisi', distribution_end_date: 'Belum diisi', distribution_total: 'Belum diisi',
  photo_picks: {},
};

function renderPresent(onClose: () => void = () => {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><FinalReportPresent programId="p1" regencyId="r1" onClose={onClose} /></QueryClientProvider>);
}

test('navigates between slides and shows blank-field placeholders', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: previewData });
  renderPresent();

  expect(await screen.findByText('KABUPATEN WAJO')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Slide berikutnya' }));
  expect(await screen.findAllByText('Belum diisi')).not.toHaveLength(0);
});

test('Escape calls onClose', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: previewData });
  const onClose = vi.fn();
  renderPresent(onClose);
  await screen.findByText('KABUPATEN WAJO');
  fireEvent.keyDown(window, { key: 'Escape' });
  expect(onClose).toHaveBeenCalledTimes(1);
});

test('ArrowRight advances the counter', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: previewData });
  renderPresent();
  await screen.findByText('KABUPATEN WAJO');
  expect(screen.getByText('1 / 9')).toBeInTheDocument();
  fireEvent.keyDown(window, { key: 'ArrowRight' });
  expect(screen.getByText('2 / 9')).toBeInTheDocument();
  fireEvent.keyDown(window, { key: 'ArrowLeft' });
  expect(screen.getByText('1 / 9')).toBeInTheDocument();
});

test('shows an error message with retry when the preview fails', async () => {
  vi.mocked(apiRequest).mockRejectedValue(new Error('boom'));
  renderPresent();
  expect(await screen.findByText('Gagal memuat tampilan laporan.')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeVisible();
});

test('shows at most the slide\'s slot count of photos (Rapat Koordinasi: 2)', async () => {
  const pick = (id: string) => ({ id, activity_media_id: id, slide_key: 'rapat_koordinasi', sort_order: 0, display_name: `Foto ${id}`, content_url: `/x/${id}` });
  vi.mocked(apiRequest).mockResolvedValue({ data: { ...previewData, photo_picks: { rapat_koordinasi: ['a', 'b', 'c'].map(pick) } } });
  renderPresent();
  await screen.findByText('KABUPATEN WAJO');
  for (let i = 0; i < 5; i += 1) fireEvent.keyDown(window, { key: 'ArrowRight' });
  expect(await screen.findByText('Dokumentasi Rapat Koordinasi')).toBeVisible();
  expect(screen.getByAltText('Foto a')).toBeVisible();
  expect(screen.getByAltText('Foto b')).toBeVisible();
  expect(screen.queryByAltText('Foto c')).not.toBeInTheDocument();
});
