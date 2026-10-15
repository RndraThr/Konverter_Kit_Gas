import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { toast } from 'sonner';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { FinalReportPage } from './FinalReportPage';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

afterEach(() => { vi.restoreAllMocks(); vi.mocked(toast.error).mockClear(); vi.mocked(toast.success).mockClear(); });

const settings = { program_id: 'p1', regency_id: 'r1', leader_count: 0, admin_count: 0, technician_count: 0, helper_count: 0, machine_manufacturer_name: '', delivery_hose_date: '', delivery_machine_group_date: '', delivery_gas_cylinder_date: '' };

function media(id: string) {
  return { id, display_name: `Foto ${id}`, content_url: `/api/v1/activities/media/${id}/content` };
}

function renderPage(permissions: string[], { rakorItems = [] as ReturnType<typeof media>[], rakorPicks = [] as string[], onRequest }: { rakorItems?: ReturnType<typeof media>[]; rakorPicks?: string[]; onRequest?: (path: string, init?: RequestInit) => unknown } = {}) {
  vi.mocked(apiRequest).mockImplementation((path: string, init?: RequestInit) => {
    if (onRequest) {
      const result = onRequest(path, init);
      if (result !== undefined) return Promise.resolve(result);
    }
    if (path.includes('/final-report/settings')) return Promise.resolve({ data: settings });
    if (path.includes('/final-report/documents')) return Promise.resolve({ data: [] });
    if (path.includes('/final-report/photo-picks')) return Promise.resolve({ data: path.includes('slide_key=rapat_koordinasi') ? rakorPicks.map((id) => ({ id: `pick-${id}`, activity_media_id: id, slide_key: 'rapat_koordinasi', sort_order: 0, display_name: id, content_url: '' })) : [] });
    if (path.includes('/activities/media?')) return Promise.resolve({ data: { items: path.includes('activity_type=rakor') ? rakorItems : [], page: 1, page_size: 24, total: 0 } });
    return Promise.reject(new Error(`Unexpected request: ${path}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter><FinalReportPage programId="p1" regencyId="r1" /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
}

test('renders settings form and photo sections for a manager', async () => {
  renderPage(['final_report.view', 'final_report.manage']);
  expect(await screen.findByText('Susunan Tim & Rincian Pengiriman')).toBeVisible();
  expect(await screen.findByText('Pengiriman & Penerimaan Barang')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Generate PPTX' })).toBeVisible();
});

test('hides the settings form, Generate and checkboxes for a viewer without manage', async () => {
  renderPage(['final_report.view'], { rakorItems: [media('a')] });
  expect(await screen.findByRole('button', { name: 'Tampilkan' })).toBeVisible();
  await screen.findByAltText('Foto a');
  expect(screen.queryByText('Susunan Tim & Rincian Pengiriman')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Generate PPTX' })).not.toBeInTheDocument();
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
});

test('blocks selecting a 7th photo with a toast and never calls the API', async () => {
  const ids = ['a', 'b', 'c', 'd', 'e', 'f', 'g'];
  const puts: unknown[] = [];
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: ids.map(media),
    rakorPicks: ids.slice(0, 6),
    onRequest: (path, init) => { if (init?.method === 'PUT' && path.includes('photo-picks')) { puts.push(init.body); return { data: [] }; } return undefined; },
  });
  const seventh = await screen.findByRole('checkbox', { name: 'Pilih Foto g' });
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto a' })).toBeChecked());
  fireEvent.click(seventh);
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Maksimal 6 foto per slide.'));
  expect(puts).toHaveLength(0);
});

test('clamps cleared/negative team counts to 0 when saving settings', async () => {
  let body: Record<string, unknown> | null = null;
  renderPage(['final_report.view', 'final_report.manage'], {
    onRequest: (path, init) => { if (init?.method === 'PUT' && path.includes('/final-report/settings')) { body = JSON.parse(init.body as string); return { data: settings }; } return undefined; },
  });
  const leader = await screen.findByLabelText('Leader');
  fireEvent.change(leader, { target: { value: '' } });
  fireEvent.change(screen.getByLabelText('Teknisi'), { target: { value: '-3' } });
  fireEvent.click(screen.getByRole('button', { name: 'Simpan Pengaturan' }));
  await waitFor(() => expect(body).not.toBeNull());
  expect(body!.leader_count).toBe(0);
  expect(body!.technician_count).toBe(0);
});

test('checkboxes are disabled and send no PUT while photo-picks is still loading', async () => {
  const puts: unknown[] = [];
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: [media('a')],
    onRequest: (path, init) => {
      if (init?.method === 'PUT' && path.includes('photo-picks')) { puts.push(init.body); return { data: [] }; }
      if (path.includes('/final-report/photo-picks') && path.includes('slide_key=rapat_koordinasi')) return new Promise(() => {}) as unknown;
      return undefined;
    },
  });
  const box = await screen.findByRole('checkbox', { name: 'Pilih Foto a' });
  expect(box).toHaveAttribute('aria-disabled', 'true');
  fireEvent.click(box);
  expect(puts).toHaveLength(0);
});

test('toggle PUT carries slide_key plus existing picks, and blocks a second click while in flight', async () => {
  const puts: Record<string, unknown>[] = [];
  let release: () => void = () => {};
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: ['x', 'y', 'a', 'b'].map(media),
    rakorPicks: ['x', 'y'],
    onRequest: (path, init) => {
      if (init?.method === 'PUT' && path.includes('photo-picks')) {
        puts.push(JSON.parse(init.body as string));
        return new Promise((resolve) => { release = () => resolve({ data: [] }); }) as unknown;
      }
      return undefined;
    },
  });
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto x' })).toBeChecked());
  const a = screen.getByRole('checkbox', { name: 'Pilih Foto a' });
  await waitFor(() => expect(a).not.toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(a);
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0].slide_key).toBe('rapat_koordinasi');
  expect([...(puts[0].activity_media_ids as string[])].sort()).toEqual(['a', 'x', 'y']);
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto b' })).toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Pilih Foto b' }));
  expect(puts).toHaveLength(1);
  release();
});
