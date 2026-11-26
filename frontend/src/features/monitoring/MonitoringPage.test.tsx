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
import { duplicatesMessage } from './DuplicatesPanel';
import { activityCellState, activityTypes, duplicatesBySchedule, type BACell, type BAItem, type SerialDuplicate, type ActivityAlbum, type ActivityCell, type MediaReview, type Overview, type QueueItem, type ScheduleRow, type SlotDetail } from './types';

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

// Malang has 2 new rakor photos; Tuban's materials arrived (its unloading phase is over) without photos.
const activityCells: ActivityCell[] = [
  { schedule_id: 's1', regency_id: 'r1', activity_type: 'rakor', photos: 2, approved: 0, rejected: 0, pending: 2, last_upload_at: null },
  { schedule_id: 's1', regency_id: 'r1', activity_type: 'training_100', photos: 3, approved: 3, rejected: 0, pending: 0, last_upload_at: null },
];
const album: ActivityAlbum = {
  schedule_id: 's1', regency_id: 'r1', regency_name: 'KAB. MALANG', activity_type: 'rakor',
  photos: ['rk1', 'rk2'].map((id) => ({ id, content_url: `/media/${id}`, mime_type: 'image/jpeg', display_name: `RAKOR ${id}`, source: 'camera', uploaded_at: '2026-11-26T03:00:00Z', uploaded_by: 'Petugas', review: null })),
};

// Malang #35 and Tuban #157 recorded the same reducer.
const serialDuplicates: SerialDuplicate[] = [{ field: 'converter', serial: '0626001192', entries: [
  { schedule_id: 's1', regency_name: 'KAB. MALANG', slot_number: 35, status: 'open', entered_by: 'Reza', entered_at: '2026-11-25T08:29:00Z' },
  { schedule_id: 's2', regency_name: 'KAB. TUBAN', slot_number: 157, status: 'open', entered_by: 'Saefi', entered_at: '2026-11-25T12:49:00Z' },
] }];

// Malang uploaded the wet-signed DP3 of two days.
const baCells: BACell[] = [{ schedule_id: 's1', type: 'dp3', items: 2, approved: 0, rejected: 0, pending: 2 }];
const baItems: BAItem[] = ['2026-11-28', '2026-11-29'].map((date) => ({
  key: `wet:dp3:${date}`, type: 'dp3', label: 'DP3', date, source: 'wet', content_url: `/scan/${date}`, mime_type: 'image/png', uploaded_by: 'Reza', uploaded_at: '2026-11-28T09:00:00Z', review: null,
}));

afterEach(() => vi.clearAllMocks());

function renderPage(path: string, permissions = ['monitoring.view', 'monitoring.review', 'monitoring.manage']) {
  const calls: { path: string; init?: RequestInit }[] = [];
  // The photo checks made during the test, by slot, as the server would keep them.
  const reviews: Record<number, Record<string, MediaReview>> = {};
  const activityReviews: Record<string, MediaReview> = {};
  const baReviews: Record<string, MediaReview> = {};
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
    if (url === '/api/v1/monitoring/activities?program_id=p1') return Promise.resolve({ data: activityCells });
    if (url === '/api/v1/monitoring/serial-duplicates?program_id=p1') return Promise.resolve({ data: serialDuplicates });
    if (url === '/api/v1/monitoring/ba?program_id=p1') return Promise.resolve({ data: baCells });
    if (url.startsWith('/api/v1/monitoring/ba/items')) return Promise.resolve({ data: baItems.map((item) => ({ ...item, review: baReviews[item.key] ?? null })) });
    if (url === '/api/v1/monitoring/ba/approve' || url === '/api/v1/monitoring/ba/revision') {
      const body = JSON.parse(init!.body as string);
      const keys: string[] = body.keys ?? body.flags.map((f: { key: string }) => f.key);
      keys.forEach((key) => { baReviews[key] = { status: body.keys ? 'approved' : 'rejected', reason: '', reviewed_by: 'Rendra', reviewed_at: '2026-11-28T05:00:00Z' }; });
      return Promise.resolve({ data: { saved: true } });
    }
    if (url.startsWith('/api/v1/monitoring/activities/album')) return Promise.resolve({ data: { ...album, photos: album.photos.map((p) => ({ ...p, review: activityReviews[p.id] ?? null })) } });
    if (url === '/api/v1/monitoring/activities/approve' || url === '/api/v1/monitoring/activities/revision') {
      const body = JSON.parse(init!.body as string);
      const ids: string[] = body.media_ids ?? body.flags.map((f: { media_id: string }) => f.media_id);
      ids.forEach((id) => { activityReviews[id] = { status: body.flags ? 'rejected' : 'approved', reason: '', reviewed_by: 'Rendra', reviewed_at: '2026-11-20T05:00:00Z' }; });
      return Promise.resolve({ data: { ...album, photos: album.photos.map((p) => ({ ...p, review: activityReviews[p.id] ?? null })) } });
    }
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

test('the kabupaten list can be searched and shows the photos to check', async () => {
  renderPage('/monitoring-distribusi?tab=antrean&schedule=s1');
  const list = await screen.findByRole('listbox', { name: 'Kabupaten' });
  const malang = within(list).getByRole('option', { name: /KAB. MALANG/ });
  expect(malang).toHaveAttribute('aria-selected', 'true');
  expect(within(malang).getByTitle('14 foto belum dicek')).toHaveTextContent('14');
  await userEvent.type(screen.getByRole('textbox', { name: 'Cari kabupaten' }), 'tuban');
  expect(within(list).queryByRole('option', { name: /KAB. MALANG/ })).toBeNull();
  expect(within(list).getByText('Tidak ada kabupaten.')).toBeVisible();
});

test('an activity type is missing once its phase is over, and checked or fixed first', () => {
  const rakor = activityTypes.find((t) => t.code === 'rakor')!;
  const days = [{ day: '2026-11-26', phase: 'rakor' as const }];
  expect(activityCellState(undefined, rakor, days, '2026-11-26')).toBe('empty');
  expect(activityCellState(undefined, rakor, days, '2026-11-27')).toBe('missing');
  expect(activityCellState(undefined, activityTypes.find((t) => t.code === 'kegiatan')!, days, '2026-12-30')).toBe('empty');
  const cell = { schedule_id: 's1', regency_id: 'r1', activity_type: 'rakor', photos: 3, approved: 1, rejected: 1, pending: 1, last_upload_at: null };
  expect(activityCellState(cell, rakor, days, '2026-11-27')).toBe('revision');
  expect(activityCellState({ ...cell, rejected: 0 }, rakor, days, '2026-11-27')).toBe('pending');
  expect(activityCellState({ ...cell, rejected: 0, pending: 0 }, rakor, days, '2026-11-27')).toBe('approved');
  const docs = activityTypes.find((t) => t.code === 'dokumen_pendukung')!;
  expect(activityCellState({ ...cell, activity_type: 'dokumen_pendukung', rejected: 0, pending: 0, missing: 2 }, docs, days, '2026-11-27')).toBe('missing');
  expect(activityCellState({ ...cell, activity_type: 'dokumen_pendukung', rejected: 0, pending: 0, missing: 0 }, docs, days, '2026-11-27')).toBe('approved');
});

test('an activity revision notification opens the album, or the activity page for the field', () => {
  expect(webLink('/kegiatan/rakor?schedule=s1&revisi=1', true)).toBe('/monitoring-distribusi?tab=kegiatan&schedule=s1&activity=rakor');
  expect(webLink('/kegiatan/unloading_oli?schedule=s1&revisi=1', false)).toBe('/dokumentasi/unloading?type=unloading_oli');
  expect(webLink('/kegiatan/kegiatan_distribusi?schedule=s1&revisi=1', false)).toBe('/dokumentasi/kegiatan-pendistribusian');
  expect(webLink('/kegiatan/training_10?schedule=s1&revisi=1', false)).toBe('/dokumentasi/training-10');
  expect(webLink('/ba/dp3/2026-11-28?schedule=s1&revisi=1', true)).toBe('/monitoring-distribusi?tab=ba&schedule=s1&ba=dp3');
  expect(webLink('/ba/dp3/2026-11-28?schedule=s1&revisi=1', false)).toBe('/berita-acara');
  expect(webLink('/dokumen-pendukung?schedule=s1&revisi=1', true)).toBe('/monitoring-distribusi?tab=kegiatan&schedule=s1&activity=dokumen_pendukung');
  expect(webLink('/dokumen-pendukung?schedule=s1&revisi=1', false)).toBe('/dokumen-pendukung');
});

test('checks the activity photos of a kabupaten: approves one, sends one back', async () => {
  const calls = renderPage('/monitoring-distribusi?tab=kegiatan');
  await waitFor(() => expect(screen.getByRole('tab', { name: /Kegiatan/ })).toHaveTextContent(/2$/));
  await userEvent.click(await screen.findByRole('checkbox', { name: /Hanya kabupaten saya/ }));
  await userEvent.click(await screen.findByRole('button', { name: 'Rakor KAB. MALANG' }));
  const dialog = await screen.findByRole('dialog');
  expect(await within(dialog).findByAltText('RAKOR rk1')).toBeVisible();
  fireEvent.keyDown(window, { key: 'a' });
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/activities/approve')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/activities/approve')!.init!.body as string)).toEqual({ schedule_id: 's1', activity_type: 'rakor', media_ids: ['rk1'] });
  expect(await within(dialog).findByAltText('RAKOR rk2')).toBeVisible();
  await userEvent.click(within(dialog).getByRole('checkbox', { name: /Tandai salah/ }));
  await userEvent.type(within(dialog).getByRole('textbox', { name: 'Alasan revisi foto' }), 'Bukan rakor');
  await userEvent.click(within(dialog).getByRole('button', { name: /Kirim revisi \(1\)/ }));
  await userEvent.click(within(dialog).getByRole('button', { name: /^Kirim revisi$/ }));
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/activities/revision')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/activities/revision')!.init!.body as string))
    .toEqual({ schedule_id: 's1', activity_type: 'rakor', flags: [{ media_id: 'rk2', reason: 'Bukan rakor' }], note: '' });
});

test('lists serials recorded twice, copies them for the group, and opens a slot', async () => {
  expect(duplicatesBySchedule(serialDuplicates)).toEqual({ s1: 1, s2: 1 });
  expect(duplicatesMessage(serialDuplicates)).toContain('1. Konkit/Reducer 0626001192: MALANG #35 (Reza) dan TUBAN #157 (Saefi)');
  renderPage('/monitoring-distribusi?tab=dobel');
  await waitFor(() => expect(screen.getByRole('tab', { name: /Serial dobel/ })).toHaveTextContent(/1$/));
  expect(await screen.findByText('0626001192')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Nomor bagi 157 KAB. TUBAN' }));
  expect(await screen.findByRole('dialog')).toBeVisible();
});

test('live marks a kabupaten with a serial recorded twice', async () => {
  renderPage('/monitoring-distribusi');
  expect(await screen.findByText('1 serial dobel')).toBeVisible();
});

test('checks the signed BA of a kabupaten: approves one day, sends the other back', async () => {
  const calls = renderPage('/monitoring-distribusi?tab=ba');
  await waitFor(() => expect(screen.getByRole('tab', { name: /^BA/ })).toHaveTextContent(/2$/));
  await userEvent.click(await screen.findByRole('button', { name: 'DP3 KAB. MALANG' }));
  const dialog = await screen.findByRole('dialog');
  expect(await within(dialog).findByAltText('DP3 2026-11-28')).toBeVisible();
  fireEvent.keyDown(window, { key: 'a' });
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/ba/approve')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/ba/approve')!.init!.body as string)).toEqual({ schedule_id: 's1', keys: ['wet:dp3:2026-11-28'] });
  expect(await within(dialog).findByAltText('DP3 2026-11-29')).toBeVisible();
  await userEvent.click(within(dialog).getByRole('button', { name: /Minta revisi/ }));
  await userEvent.type(within(dialog).getByRole('textbox', { name: 'Alasan revisi BA' }), 'Stempel belum ada');
  await userEvent.click(within(dialog).getByRole('button', { name: /^Kirim revisi$/ }));
  await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/monitoring/ba/revision')).toBe(true));
  expect(JSON.parse(calls.find((c) => c.path === '/api/v1/monitoring/ba/revision')!.init!.body as string))
    .toEqual({ schedule_id: 's1', flags: [{ key: 'wet:dp3:2026-11-29', reason: 'Stempel belum ada' }], note: '' });
});
