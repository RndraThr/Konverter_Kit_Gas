import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { PermissionsProvider } from '../../lib/permissions';
import { SupportingDocumentSlotsPanel } from './SupportingDocumentSlotsPanel';

vi.mock('../../lib/api', () => ({ apiRequest: vi.fn() }));

function renderPanel() {
  vi.mocked(apiRequest).mockImplementation(((url: string) => {
    if (url.includes('/slots?schedule_id=schedule-1')) {
      return Promise.resolve({ data: [{ id: 'slot-1', schedule_id: 'schedule-1', name: 'Surat Tugas', is_active: true }] });
    }
    return Promise.resolve({ data: { id: 'slot-2', schedule_id: 'schedule-1', name: 'SK Penetapan', is_active: true } });
  }) as typeof apiRequest);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['programs.manage']}><SupportingDocumentSlotsPanel scheduleId="schedule-1" /></PermissionsProvider></QueryClientProvider>);
}

test('lists existing slots and submits a new one', async () => {
  renderPanel();
  await waitFor(() => expect(screen.getByText('Surat Tugas')).toBeVisible());
  await userEvent.type(screen.getByLabelText('Nama dokumen baru'), 'SK Penetapan');
  await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/activities/supporting-documents/slots', expect.objectContaining({ method: 'POST', body: JSON.stringify({ schedule_id: 'schedule-1', name: 'SK Penetapan' }) })));
});
