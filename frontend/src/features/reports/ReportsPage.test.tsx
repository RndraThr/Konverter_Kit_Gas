import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { ReportsPage } from './ReportsPage';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPage() {
  vi.mocked(apiRequest).mockImplementation((path) => {
    if (path === '/api/v1/program-setup/schedules') {
      return Promise.resolve({ data: [{ id: 'schedule-1', name: 'Wajo Tahap 1', status: 'active', regency: { id: 'regency-1', name: 'Wajo', document_code: 'WJO' } }] });
    }
    if (path.startsWith('/api/v1/reports/schedule/schedule-1/summary')) {
      return Promise.resolve({ data: {
        total_allocations: 3,
        allocation_status_counts: [{ status: 'ready', count: 1 }, { status: 'distributed', count: 2 }],
        distribution_status_counts: [{ status: 'draft', count: 1 }, { status: 'completed', count: 2 }],
        documentation_incomplete: 1,
      } });
    }
    if (path.startsWith('/api/v1/reports/schedule/schedule-1/rows')) {
      return Promise.resolve({ data: [
        { distribution_number: 7, full_name: 'Siti Aminah', nik: '7306014101900001', sector_identifier: 'KP01', village: 'Tempe', district: 'Sabbangparu', allocation_status: 'distributed', distribution_status: 'completed', documentation_complete: true, completed_at: '2026-09-10T08:30:00Z' },
      ] });
    }
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><ReportsPage /></QueryClientProvider>);
}

afterEach(() => { vi.clearAllMocks(); });

test('loads summary and rows for the selected schedule with full NIK', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });

  expect(await screen.findByText('3')).toBeVisible();
  const table = screen.getByRole('table');
  expect(within(table).getByText('Siti Aminah')).toBeVisible();
  expect(within(table).getByText('7306014101900001')).toBeVisible();
  expect(within(table).getByText('Lengkap')).toBeVisible();
});

test('applies status filters to the rows request', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });
  await screen.findByText('Siti Aminah');
  fireEvent.change(screen.getByLabelText('Status alokasi'), { target: { value: 'distributed' } });

  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => typeof path === 'string' && path.startsWith('/api/v1/reports/schedule/schedule-1/rows') && path.includes('allocation_status=distributed'));
  expect(call).toBeTruthy();
});

test('renders export links scoped to the selected schedule and active filters', async () => {
  renderPage();
  await screen.findByRole('option', { name: /Wajo Tahap 1/ });
  fireEvent.change(screen.getByLabelText('Jadwal'), { target: { value: 'schedule-1' } });
  await screen.findByText('Siti Aminah');
  fireEvent.change(screen.getByLabelText('Status alokasi'), { target: { value: 'distributed' } });

  const excelLink = screen.getByRole('link', { name: 'Export Excel' });
  const pdfLink = screen.getByRole('link', { name: 'Export PDF' });
  expect(excelLink.getAttribute('href')).toBe('/api/v1/reports/schedule/schedule-1/export.xlsx?allocation_status=distributed&distribution_status=&documentation_status=');
  expect(pdfLink.getAttribute('href')).toBe('/api/v1/reports/schedule/schedule-1/export.pdf?allocation_status=distributed&distribution_status=&documentation_status=');
});
