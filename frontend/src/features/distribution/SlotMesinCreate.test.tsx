import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiRequest } from '../../lib/api';
import { SlotMesinCreate } from './SlotMesinCreate';

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api');
  return { ...actual, apiRequest: vi.fn() };
});

function renderCreate() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}>
    <SlotMesinCreate
      scheduleID="schedule-1"
      slotNumber={3}
      machineOptions={[{ code: 'm1', brand: 'SHARK', type: 'SPWP 80-30' }]}
      hoseOptions={[{ code: 'h1', brand: 'TRILIUNHOSE', spec: '6m' }]}
      converterOptions={[{ code: 'ergas', brand: 'ERGAS' }]}
      onCreated={vi.fn()}
    />
  </QueryClientProvider>);
}

test('creates an empty machine-documentation slot with only schedule and slot number', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: { id: 'slot-3' } });
  renderCreate();

  expect(screen.getByText('Nomor bagi #3')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'Mulai dokumentasi' }));

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots', {
    method: 'POST',
    body: JSON.stringify({ schedule_id: 'schedule-1', slot_number: 3 }),
  }));
});

test('does not render date, equipment, serial, or recipient fields', () => {
  renderCreate();

  expect(screen.queryByLabelText('Tanggal distribusi')).not.toBeInTheDocument();
  expect(screen.queryByText('Merk/Tipe Mesin')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Serial Number Mesin')).not.toBeInTheDocument();
  expect(screen.queryByLabelText(/NIK/i)).not.toBeInTheDocument();
});
