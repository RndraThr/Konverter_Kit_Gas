import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import type { SlotExampleEntry } from '../distribution/slotExamples';
import { SlotExamplesPanel } from './SlotExamplesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const entries: SlotExampleEntry[] = [
  { slot_code: 'recipient_id_card', label: 'Foto KTP dan Nomor Urut', stage: 'dokumen', example: { caption: 'Dari depan', image_url: '/api/v1/documentation-examples/recipient_id_card/image?v=1', updated_at: '2026-11-20T00:00:00Z' } },
  { slot_code: 'machine_photo', label: 'Foto Mesin', stage: 'mesin', example: null },
];

function renderPanel(permissions = ['settings.view', 'documentation_examples.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}><SlotExamplesPanel /></PermissionsProvider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiRequest).mockImplementation(async (_path, init) => (init?.method ? {} : { data: entries }));
});

test('is hidden from accounts that cannot manage examples', () => {
  renderPanel(['settings.view', 'settings.manage']);
  expect(screen.queryByRole('heading', { name: 'Contoh foto dokumentasi' })).not.toBeInTheDocument();
  expect(apiRequest).not.toHaveBeenCalled();
});

test('uploads a first example photo with its caption', async () => {
  const user = userEvent.setup();
  renderPanel();
  const row = await screen.findByRole('form', { name: 'Contoh foto Foto Mesin' });
  const save = within(row).getByRole('button', { name: 'Simpan' });
  await user.type(within(row).getByLabelText('Keterangan'), 'Mesin terlihat utuh dari samping');
  expect(save).toBeDisabled();

  const photo = new File(['jpeg'], 'mesin.jpg', { type: 'image/jpeg' });
  await user.upload(within(row).getByLabelText('Foto contoh'), photo);
  await user.click(save);

  const [path, init] = vi.mocked(apiRequest).mock.calls.find(([, request]) => request?.method === 'PUT')!;
  expect(path).toBe('/api/v1/documentation-examples/machine_photo');
  const body = init!.body as FormData;
  expect(body.get('caption')).toBe('Mesin terlihat utuh dari samping');
  expect((body.get('file') as File).name).toBe('mesin.jpg');
});

test('updates only the caption of an existing example', async () => {
  const user = userEvent.setup();
  renderPanel();
  const row = await screen.findByRole('form', { name: 'Contoh foto Foto KTP dan Nomor Urut' });
  const caption = within(row).getByLabelText('Keterangan');
  await user.clear(caption);
  await user.type(caption, 'Ambil dari depan');
  await user.click(within(row).getByRole('button', { name: 'Simpan' }));

  const body = vi.mocked(apiRequest).mock.calls.find(([, request]) => request?.method === 'PUT')![1]!.body as FormData;
  expect(body.get('caption')).toBe('Ambil dari depan');
  expect(body.has('file')).toBe(false);
});

test('rejects a file that is not a photo', async () => {
  const user = userEvent.setup({ applyAccept: false });
  renderPanel();
  const row = await screen.findByRole('form', { name: 'Contoh foto Foto Mesin' });
  await user.upload(within(row).getByLabelText('Foto contoh'), new File(['pdf'], 'panduan.pdf', { type: 'application/pdf' }));
  expect(within(row).getByRole('alert')).toHaveTextContent('Gunakan foto JPEG, PNG, atau WebP.');
  expect(within(row).getByRole('button', { name: 'Simpan' })).toBeDisabled();
});
