import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { TemplatesPanel } from './TemplatesPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

const legacyTemplate = {
  id: 'document-1', template_code: 'DOK-PETANI', version: 1, name: 'Dokumentasi Petani', program_type: 'farmer', status: 'published',
  slots: [{ slot_code: 'bukti', label: 'Bukti', stage: 'mesin', is_required: true, min_files: 1, max_files: 1, input_source: 'both', require_location: false, require_captured_at: false, sort_order: 10 }],
};

function renderPanel() {
  vi.mocked(apiRequest).mockImplementation(((path: string) => Promise.resolve(path.endsWith('/package-templates') ? { data: [] } : { data: [legacyTemplate] })) as typeof apiRequest);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.manage']}><TemplatesPanel /></PermissionsProvider></QueryClientProvider>);
}

test('normalizes legacy slots and new slots to image media', async () => {
  renderPanel();
  await userEvent.click(await screen.findByRole('button', { name: 'Edit Dokumentasi Petani' }));
  const dialog = screen.getByRole('dialog', { name: 'Edit Dokumentasi Petani' });
  expect(within(dialog).getByRole('combobox', { name: 'Jenis media' })).toHaveTextContent('Foto saja');

  await userEvent.click(within(dialog).getByRole('button', { name: 'Tambah slot' }));
  const mediaSelectors = within(dialog).getAllByRole('combobox', { name: 'Jenis media' });
  expect(mediaSelectors).toHaveLength(2);
  expect(mediaSelectors[1]).toHaveTextContent('Foto saja');
});

test('shows all media policies and saves video policy', async () => {
  renderPanel();
  await userEvent.click(await screen.findByRole('button', { name: 'Edit Dokumentasi Petani' }));
  const dialog = screen.getByRole('dialog', { name: 'Edit Dokumentasi Petani' });
  const mediaSelect = within(dialog).getByRole('combobox', { name: 'Jenis media' });
  await userEvent.click(mediaSelect);
  expect(screen.getByRole('option', { name: 'Foto saja' })).toBeVisible();
  expect(screen.getByRole('option', { name: 'Video saja' })).toBeVisible();
  expect(screen.getByRole('option', { name: 'Foto & video' })).toBeVisible();
  await userEvent.click(screen.getByRole('option', { name: 'Video saja' }));

  let saved: { slots: Array<{ media_kind: string }> } | undefined;
  vi.mocked(apiRequest).mockImplementation(((path: string, init?: RequestInit) => {
    if (path === '/api/v1/program-setup/documentation-templates/document-1' && init?.method === 'PATCH') {
      saved = JSON.parse(init.body as string);
      return Promise.resolve({ data: { ...legacyTemplate, ...saved } });
    }
    return Promise.resolve(path.endsWith('/package-templates') ? { data: [] } : { data: [legacyTemplate] });
  }) as typeof apiRequest);
  await userEvent.click(within(dialog).getByRole('button', { name: 'Simpan' }));
  await waitFor(() => expect(saved).toBeDefined());
  expect(saved!.slots[0].media_kind).toBe('video');
});
