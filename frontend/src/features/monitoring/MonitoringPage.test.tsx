import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { webLink } from '../finance/NotificationBell';
import { addDays, blocksOf, clearRange, dayNumbers, moveBlock, moveRange, resizeBlock, shiftAll } from './calendarBlocks';
import { format, parseISO } from 'date-fns';
import { id } from 'date-fns/locale';
import { todayWIB } from './api';
import { MonitoringPage } from './MonitoringPage';
import { zoomAt } from './ZoomableImage';
import type { MediaReview, Overview, QueueItem, ScheduleRow, SlotDetail } from './types';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn(), getBootstrap: vi.fn().mockResolvedValue({ data: { id: 'me' }, meta: { csrf_token: '' } }) };
});
vi.mock('@/lib/realtime', () => ({ useRealtimeSignals: vi.fn(), subscribeRealtime: vi.fn(() => () => undefined) }));

const row = (overrides: Partial<ScheduleRow>): ScheduleRow => ({
  schedule_id: 's1', name: 'KAB. MALANG', regency_id: 'r1', regency_name: 'KAB. MALANG', province_name: 'JAWA TIMUR', zone_name: 'Zona II', zone_sort: 2,
  start_date: '2026-11-21', end_date: '2026-12-01', quota: 320, sheet_no: 20, days: [{ day: '2026-11-20', phase: 'distribusi' }], reviewers: [{ id: 'me', name: 'Rendra' }], leaders: [],
  today_phase: 'distribusi', distribution_day: 1, distribution_of: 3, completed: 40, completed_today: 12, approved: 30, pending: 10, revision: 2, pending_photos: 14, last_activity_at: null,
  ...overrides,
});

const overview: Overview = {
  programs: [{ id: 'p1', name: 'Konkit 2026', fiscal_year: 2026, status: 'active' }], program_id: 'p1', date: '2026-11-20', rakornas: ['2026-11-23'],
  rows: [row({}), row({ schedule_id: 's2', regency_name: 'KAB. TUBAN', today_phase: 'material', days: [{ day: '2026-11-20', phase: 'material' }], reviewers: [], completed: 0, approved: 0, pending: 0, revision: 0, pending_photos: 0, quota: 220 })],
  reviewers: [{ id: 'me', name: 'Rendra' }], users: [{ id: 'me', name: 'Rendra' }, { id: 'u2', name: 'Leader Malang' }],
};

const item = (slot_number: number, full_name: string, state: QueueItem['state'], approved: number, upload: string): QueueItem => ({
  slot_number, full_name, nik: `350${slot_number}`, village: 'Sukamaju', machine_serial: `SN-${slot_number}`, distribution_date: '2026-11-20',
  status: 'completed', completed_at: '2026-11-20T02:00:00Z', completed_by: 'Petugas',
  photos: 2, approved_photos: approved, rejected_photos: 0, pending_photos: 2 - approved, state, last_upload_at: upload, reviewed_by: '', reviewed_at: null,
});
const queue: QueueItem[] = [item(7, 'Budi', 'pending', 0, '2026-11-20T01:00:00Z'), item(9, 'Siti', 'pending', 0, '2026-11-20T02:00:00Z'), item(3, 'Ani', 'approved', 2, '2026-11-19T01:00:00Z')];

const detail = (slot: number, reviews: Record<string, MediaReview> = {}): SlotDetail => ({
  slot: {
    id: `slot-${slot}`, schedule_id: 's1', slot_number: slot, status: 'completed', distribution_date: '2026-11-20', full_name: slot === 7 ? 'Budi' : 'Siti', nik: '3507',
    machine_serial_number: `SN-${slot}`, needs_recompletion: false, created_at: '', updated_at: '',
    documentation: [
      { code: 'ktp', label: 'Foto KTP', stage: 'dokumen', status: 'complete', files: [{ id: `ktp-${slot}`, slot_id: 'ktp', original_filename: 'k.jpg', mime_type: 'image/jpeg', byte_size: 1, source: 'camera', status: 'accepted', content_url: `/media/ktp-${slot}`, storage_state: 'final' }] },
      { code: 'mesin', label: 'Foto Mesin', stage: 'mesin', status: 'complete', files: [{ id: `mesin-${slot}`, slot_id: 'mesin', original_filename: 'm.jpg', mime_type: 'image/jpeg', byte_size: 1, source: 'camera', status: 'accepted', content_url: `/media/mesin-${slot}`, storage_state: 'final' }] },
    ],
  },
  schedule_name: 'KAB. MALANG', regency_name: 'KAB. MALANG', completed_at: '2026-11-20T02:00:00Z', completed_by: 'Petugas', state: 'pending', reviews,
});

afterEach(() => vi.clearAllMocks());

function renderPage(path: string, permissions = ['monitoring.view', 'monitoring.review', 'monitoring.manage']) {
  const calls: { path: string; init?: RequestInit }[] = [];
  // The photo checks made during the test, by slot, as the server would keep them.
  const reviews: Record<number, Record<string, MediaReview>> = {};
  const check = (slot: number, ids: string[], status: MediaReview['status']) => {
    reviews[slot] = { ...reviews[slot], ...Object.fromEntries(ids.map((id) => [id, { status, reason: '', reviewed_by: 'Rendra', reviewed_at: '2026-11-20T05:00:00Z' }])) };
  };
  vi.mocked(apiRequest).mockImplementation((url, init) => {
    calls.push({ path: url, init });
    if (url.startsWith('/api/v1/monitoring/overview')) return Promise.resolve({ data: overview });
    if (url === '/api/v1/monitoring/queue?schedule_id=s1') return Promise.resolve({ data: queue });
    const slot = /slots\/(\d+)(\/\w+)?\?/.exec(url);
    if (slot) {
      const n = Number(slot[1]);
      const body = init?.body ? JSON.parse(init.body as string) : {};
      if (slot[2] === '/approve') check(n, body.media_ids, 'approved');
      if (slot[2] === '/revision') check(n, body.flags.map((f: { media_id: string }) => f.media_id), 'rejected');
      return Promise.resolve({ data: detail(n, reviews[n]) });
    }
    if (url === '/api/v1/monitoring/calendar') return Promise.resolve({ data: { saved: true } });
    return Promise.reject(new Error(`Unexpected request: ${url}`));
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><MemoryRouter initialEntries={[path]}><MonitoringPage /></MemoryRouter></PermissionsProvider></QueryClientProvider>);
  return calls;
}

test('day numbers count each phase separately and dates move across months', () => {
  expect(dayNumbers({ '2026-11-19': 'material', '2026-11-18': 'material', '2026-11-30': 'distribusi', '2026-12-01': 'distribusi' }))
    .toEqual({ '2026-11-18': 1, '2026-11-19': 2, '2026-11-30': 1, '2026-12-01': 2 });
  expect(addDays('2026-11-30', 2)).toBe('2026-12-02');
});

test('opens on a rolling month: 3 days back and 27 days ahead', async () => {
  renderPage('/monitoring-distribusi?tab=jadwal');
  await screen.findAllByRole('table');
  const today = todayWIB();
  const show = (day: string) => format(parseISO(day), 'd MMM', { locale: id });
  expect(screen.getByRole('button', { name: 'Pilih tanggal mulai' })).toHaveTextContent(`${show(addDays(today, -3))} – ${show(addDays(today, 27))}`);
  expect(screen.getByRole('radio', { name: 'Bulan' })).toHaveAttribute('aria-checked', 'true');
  // Each month header plus 31 day columns.
  const dayHeaders = screen.getAllByRole('table')[0].querySelectorAll('thead tr:nth-child(3) th');
  expect(dayHeaders).toHaveLength(31);
});

test('blocks move, stretch and shift like bars on the sheet', () => {
  const days = { '2026-11-20': 'material', '2026-11-21': 'material', '2026-11-23': 'distribusi', '2026-11-24': 'distribusi' } as const;
  const [material, distribusi] = blocksOf(days);
  expect(material).toEqual({ phase: 'material', start: '2026-11-20', end: '2026-11-21' });
  expect(moveBlock(days, distribusi, 2)).toEqual({ '2026-11-20': 'material', '2026-11-21': 'material', '2026-11-25': 'distribusi', '2026-11-26': 'distribusi' });
  expect(resizeBlock(days, material, 'end', '2026-11-22')).toMatchObject({ '2026-11-22': 'material' });
  expect(Object.keys(resizeBlock(days, material, 'start', '2026-11-25'))).not.toContain('2026-11-20');
  expect(moveRange(days, '2026-11-21', '2026-11-23', 1)).toEqual({ '2026-11-20': 'material', '2026-11-22': 'material', '2026-11-24': 'distribusi' });
  expect(clearRange(days, '2026-11-21', '2026-11-23')).toEqual({ '2026-11-20': 'material', '2026-11-24': 'distribusi' });
  expect(shiftAll(days, -1)).toEqual({ '2026-11-19': 'material', '2026-11-20': 'material', '2026-11-22': 'distribusi', '2026-11-23': 'distribusi' });
});

test('zoom keeps the point under the cursor in place', () => {
  const view = zoomAt({ scale: 1, x: 0, y: 0 }, 2, 100, 50);
  expect(view).toEqual({ scale: 2, x: -100, y: -50 });
  // The cursor point maps to the same image point before and after.
  expect((100 - view.x) / view.scale).toBe(100);
  expect(zoomAt(view, 0.1, 10, 10)).toEqual({ scale: 1, x: 0, y: 0 });
  expect(zoomAt({ scale: 8, x: 0, y: 0 }, 2, 0, 0).scale).toBe(8);
});

test('a revision notification opens the slot in the monitoring, or the distribution page for the field', () => {
  expect(webLink('/nomor-bagi/12?schedule=s1&revisi=1', true)).toBe('/monitoring-distribusi?tab=antrean&schedule=s1&slot=12');
  expect(webLink('/nomor-bagi/12?schedule=s1&revisi=1', false)).toBe('/dokumentasi/pendistribusian');
  expect(webLink('/keuangan/5', true)).toBe('/keuangan/5');
});

test('live shows only the kabupaten distributing today, all on request', async () => {
  renderPage('/monitoring-distribusi');
  expect(await screen.findByText('KAB. MALANG')).toBeVisible();
  expect(screen.getByText(/1 kabupaten distribusi hari ini/)).toBeVisible();
  expect(screen.queryByRole('button', { name: /KAB. TUBAN/ })).toBeNull();
  expect(screen.getByText(/KAB. TUBAN · Material/)).toBeVisible();
  await userEvent.click(screen.getByRole('checkbox', { name: 'Tampilkan semua kabupaten' }));
  expect(await screen.findByRole('button', { name: /KAB. TUBAN/ })).toBeVisible();
});

test('checks photos one by one, sends a marked photo back, then approves every new photo of the next recipient', async () => {
  const calls = renderPage('/monitoring-distribusi?tab=antrean&schedule=s1');
  const table = await screen.findByRole('table');
  expect(within(table).getByText('Budi')).toBeVisible();
  expect(within(table).queryByText('Ani')).toBeNull();
  await userEvent.click(within(table).getByRole('button', { name: 'Budi' }));

  const dialog = await screen.findByRole('dialog');
  // Photos follow the POS order: POS Mesin first. A approves the photo shown and moves to the next new one.
  expect(await within(dialog).findByAltText('Foto Mesin')).toBeVisible();
  fireEvent.keyDown(window, { key: 'a' });
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/slots/7/approve?schedule_id=s1')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path.includes('/7/approve'))!.init!.body as string)).toEqual({ media_ids: ['mesin-7'] });
  expect(await within(dialog).findByAltText('Foto KTP')).toBeVisible();

  await userEvent.click(within(dialog).getByRole('checkbox', { name: /Tandai salah/ }));
  await userEvent.type(within(dialog).getByRole('textbox', { name: 'Alasan revisi foto' }), 'KTP buram');
  expect(within(dialog).getByRole('button', { name: 'Semua sudah dicek' })).toBeDisabled();
  await userEvent.click(within(dialog).getByRole('button', { name: /Kirim revisi \(1\)/ }));
  expect(within(dialog).getByText(/dibuka lagi di/)).toHaveTextContent('POS Dokumen');
  await userEvent.click(within(dialog).getByRole('button', { name: /^Kirim revisi$/ }));
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/slots/7/revision?schedule_id=s1')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path.includes('/revision'))!.init!.body as string)).toEqual({ flags: [{ media_id: 'ktp-7', reason: 'KTP buram' }], note: '' });

  // Every photo of Budi is checked: on to the next recipient with new photos, all approved with Shift + A.
  expect(await within(await screen.findByRole('dialog')).findByText('#9 · Siti')).toBeVisible();
  fireEvent.keyDown(window, { key: 'A', shiftKey: true });
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/slots/9/approve?schedule_id=s1')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path.includes('/9/approve'))!.init!.body as string).media_ids.sort()).toEqual(['ktp-9', 'mesin-9']);
});

test('drags a block to new dates, paints a day, and saves the calendar', async () => {
  const calls = renderPage('/monitoring-distribusi?tab=jadwal');
  await screen.findAllByRole('table');
  // The month view follows today's date; the sheet view is fixed by the data.
  await userEvent.click(screen.getByRole('radio', { name: 'Jadwal' }));
  const malang = screen.getByText('KAB. MALANG').closest('tr')!;
  // The sheet range starts a day before the first phase day (20 Nov).
  const day = (date: number) => within(malang).getAllByRole('cell')[5 + (date - 19)];
  // Geser: the distribution block of 20 Nov moves to 22 Nov (grabbed in the middle, not at an edge).
  day(20).getBoundingClientRect = () => ({ left: 0, width: 28, top: 0, height: 24, right: 28, bottom: 24, x: 0, y: 0, toJSON: () => ({}) });
  fireEvent.pointerDown(day(20), { button: 0, clientX: 14 });
  fireEvent.pointerEnter(day(22));
  fireEvent.pointerUp(window);
  await userEvent.click(screen.getByRole('radio', { name: /Penutupan/ }));
  fireEvent.pointerDown(day(23), { button: 0 });
  fireEvent.pointerUp(window);
  await userEvent.click(screen.getByRole('button', { name: /Simpan jadwal/ }));
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/calendar')).toBe(true));
  const body = JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/calendar')!.init!.body as string);
  expect(body).toEqual({ program_id: 'p1', rows: [{ schedule_id: 's1', days: [
    { day: '2026-11-22', phase: 'distribusi' }, { day: '2026-11-23', phase: 'penutupan' },
  ] }] });
});

test('selects dates of several kabupaten and moves them together with the arrow keys', async () => {
  const calls = renderPage('/monitoring-distribusi?tab=jadwal');
  await screen.findAllByRole('table');
  // The month view follows today's date; the sheet view is fixed by the data.
  await userEvent.click(screen.getByRole('radio', { name: 'Jadwal' }));
  const cellOf = (name: string, date: number) => within(screen.getByText(name).closest('tr')!).getAllByRole('cell')[5 + (date - 19)];
  // From an empty cell of Malang to an empty cell of Tuban: both rows, 19-21 Nov.
  fireEvent.pointerDown(cellOf('KAB. MALANG', 21), { button: 0 });
  fireEvent.pointerEnter(cellOf('KAB. TUBAN', 19));
  fireEvent.pointerUp(window);
  // A pop-up next to the pointer offers the moves; the arrow keys work too.
  expect(screen.getByRole('dialog', { name: 'Pilihan tanggal' })).toHaveTextContent('2 kab · 3 hari');
  await userEvent.click(screen.getByRole('button', { name: 'Geser pilihan 1 hari lebih lambat' }));
  fireEvent.keyDown(window, { key: 'ArrowRight' });
  await userEvent.click(screen.getByRole('button', { name: /Simpan jadwal/ }));
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/calendar')).toBe(true));
  const body = JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/calendar')!.init!.body as string);
  expect(body.rows).toEqual(expect.arrayContaining([
    { schedule_id: 's1', days: [{ day: '2026-11-22', phase: 'distribusi' }] },
    { schedule_id: 's2', days: [{ day: '2026-11-22', phase: 'material' }] },
  ]));
});
