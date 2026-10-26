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

function media(id: string, mime_type = 'image/jpeg', media_type = 'image') {
  return { id, display_name: `Foto ${id}`, content_url: `/api/v1/activities/media/${id}/content`, media_type, mime_type };
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

test('limits Rapat Koordinasi to its 2 photo slots with a toast and never calls the API', async () => {
  const ids = ['a', 'b', 'c'];
  const puts: unknown[] = [];
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: ids.map((id) => media(id)),
    rakorPicks: ids.slice(0, 2),
    onRequest: (path, init) => { if (init?.method === 'PUT' && path.includes('photo-picks')) { puts.push(init.body); return { data: [] }; } return undefined; },
  });
  const third = await screen.findByRole('checkbox', { name: 'Pilih Foto c' });
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto a' })).toBeChecked());
  await waitFor(() => expect(third).not.toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(third);
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Maksimal 2 foto untuk slide ini.'));
  expect(puts).toHaveLength(0);
});

test('shows each slide\'s own photo limit (3 / 2 / 4)', async () => {
  renderPage(['final_report.view', 'final_report.manage']);
  expect(await screen.findByText('Maksimal 3 foto')).toBeVisible();
  expect(screen.getByText('Maksimal 2 foto')).toBeVisible();
  expect(screen.getByText('Maksimal 4 foto')).toBeVisible();
});

test('requests program-scoped photos with page_size=100 and hides non-JPEG/PNG media', async () => {
  const paths: string[] = [];
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: [media('jpg'), media('png', 'image/png'), media('webp', 'image/webp'), media('vid', 'video/mp4', 'video')],
    onRequest: (path) => { paths.push(path); return undefined; },
  });
  expect(await screen.findByAltText('Foto jpg')).toBeVisible();
  expect(screen.getByAltText('Foto png')).toBeVisible();
  expect(screen.queryByAltText('Foto webp')).not.toBeInTheDocument();
  expect(screen.queryByAltText('Foto vid')).not.toBeInTheDocument();
  const rakor = paths.find((path) => path.includes('activity_type=rakor'));
  expect(rakor).toContain('/api/v1/activities/media?');
  expect(rakor).toContain('regency_id=r1');
  expect(rakor).toContain('program_id=p1');
  expect(rakor).toContain('page_size=100');
});

test('shows a saved pick that is missing from the photo list and lets it be unchecked', async () => {
  const puts: Record<string, unknown>[] = [];
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: [],
    rakorPicks: ['old'],
    onRequest: (path, init) => { if (init?.method === 'PUT' && path.includes('photo-picks')) { puts.push(JSON.parse(init.body as string)); return { data: [] }; } return undefined; },
  });
  const saved = await screen.findByRole('checkbox', { name: 'Pilih old' });
  await waitFor(() => expect(saved).toBeChecked());
  await waitFor(() => expect(saved).not.toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(saved);
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0].activity_media_ids).toEqual([]);
});

test('disables Generate while the settings draft has unsaved changes', async () => {
  renderPage(['final_report.view', 'final_report.manage']);
  const generate = await screen.findByRole('button', { name: 'Generate PPTX' });
  await waitFor(() => expect(generate).toBeEnabled());
  fireEvent.change(screen.getByLabelText('Leader'), { target: { value: '2' } });
  expect(screen.getByRole('button', { name: 'Generate PPTX' })).toBeDisabled();
  expect(screen.getByText('Simpan pengaturan terlebih dahulu')).toBeVisible();
  fireEvent.change(screen.getByLabelText('Leader'), { target: { value: '0' } });
  expect(screen.getByRole('button', { name: 'Generate PPTX' })).toBeEnabled();
  expect(screen.queryByText('Simpan pengaturan terlebih dahulu')).not.toBeInTheDocument();
});

test('disables Generate while a settings save is pending', async () => {
  renderPage(['final_report.view', 'final_report.manage'], {
    onRequest: (path, init) => (init?.method === 'PUT' && path.includes('/final-report/settings') ? new Promise(() => {}) : undefined),
  });
  const generate = await screen.findByRole('button', { name: 'Generate PPTX' });
  await waitFor(() => expect(generate).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name: 'Simpan Pengaturan' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Generate PPTX' })).toBeDisabled());
});

test('lists versions with finalized time, status and a download link', async () => {
  const docs = [
    { id: 'd2', version: 2, filename: 'LAPORAN AKHIR - PETANI KAB. X.pptx', status: 'active', finalized_at: '2026-10-15T03:00:00Z' },
    { id: 'd1', version: 1, filename: 'LAPORAN AKHIR - PETANI KAB. X.pptx', status: 'superseded', finalized_at: '2026-10-14T03:00:00Z' },
  ];
  renderPage(['final_report.view'], { onRequest: (path) => (path.includes('/final-report/documents') ? { data: docs } : undefined) });
  const latest = await screen.findByRole('link', { name: 'Unduh v2' });
  expect(latest).toHaveAttribute('href', '/api/v1/final-report/documents/d2/content');
  expect(screen.getByRole('link', { name: 'Unduh v1' })).toHaveAttribute('href', '/api/v1/final-report/documents/d1/content');
  expect(screen.getByText('Aktif')).toBeVisible();
  expect(screen.getByText('Lama')).toBeVisible();
  expect(screen.getByText(new Date('2026-10-15T03:00:00Z').toLocaleString('id-ID'))).toBeVisible();
});

test('shows error states with retry for settings, photos, picks and documents', async () => {
  renderPage(['final_report.view', 'final_report.manage'], {
    onRequest: (path, init) => {
      if (init?.method) return undefined;
      if (path.includes('/final-report/settings') || path.includes('/final-report/documents')) return Promise.reject(new Error('boom'));
      if (path.includes('/activities/media?') && path.includes('activity_type=rakor')) return Promise.reject(new Error('boom'));
      if (path.includes('/final-report/photo-picks') && path.includes('slide_key=sosialisasi')) return Promise.reject(new Error('boom'));
      return undefined;
    },
  });
  expect(await screen.findByText('Pengaturan belum dapat dimuat')).toBeVisible();
  expect(await screen.findByText('Riwayat versi belum dapat dimuat')).toBeVisible();
  expect(await screen.findByText('Foto kegiatan belum dapat dimuat')).toBeVisible();
  expect(await screen.findByText('Pilihan foto belum dapat dimuat')).toBeVisible();
  expect(screen.getAllByRole('button', { name: 'Coba lagi' }).length).toBeGreaterThanOrEqual(4);
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
    rakorItems: ['x', 'y', 'a', 'b'].map((id) => media(id)),
    rakorPicks: ['x'],
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
  expect([...(puts[0].activity_media_ids as string[])].sort()).toEqual(['a', 'x']);
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto b' })).toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Pilih Foto b' }));
  expect(puts).toHaveLength(1);
  release();
});

test('stays locked after the PUT resolves until the picks refetch completes', async () => {
  const puts: unknown[] = [];
  let pickGets = 0;
  let finishRefetch: () => void = () => {};
  renderPage(['final_report.view', 'final_report.manage'], {
    rakorItems: ['x', 'a', 'b'].map((id) => media(id)),
    rakorPicks: ['x'],
    onRequest: (path, init) => {
      if (init?.method === 'PUT' && path.includes('photo-picks')) { puts.push(init.body); return { data: [] }; }
      if (!init?.method && path.includes('/final-report/photo-picks') && path.includes('slide_key=rapat_koordinasi')) {
        pickGets += 1;
        if (pickGets === 2) return new Promise((resolve) => { finishRefetch = () => resolve({ data: [{ id: 'p-x', activity_media_id: 'x', slide_key: 'rapat_koordinasi', sort_order: 0, display_name: 'x', content_url: '' }, { id: 'p-a', activity_media_id: 'a', slide_key: 'rapat_koordinasi', sort_order: 1, display_name: 'a', content_url: '' }] }); }) as unknown;
      }
      return undefined;
    },
  });
  const a = await screen.findByRole('checkbox', { name: 'Pilih Foto a' });
  await waitFor(() => expect(a).not.toHaveAttribute('aria-disabled', 'true'));
  fireEvent.click(a);
  await waitFor(() => expect(puts).toHaveLength(1));
  await waitFor(() => expect(pickGets).toBe(2));
  const b = screen.getByRole('checkbox', { name: 'Pilih Foto b' });
  expect(b).toHaveAttribute('aria-disabled', 'true');
  fireEvent.click(b);
  expect(puts).toHaveLength(1);
  finishRefetch();
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Pilih Foto b' })).not.toHaveAttribute('aria-disabled', 'true'));
  expect(screen.getByRole('checkbox', { name: 'Pilih Foto a' })).toBeChecked();
});
