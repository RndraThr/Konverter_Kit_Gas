import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { HandoverLocationsEditor } from './HandoverLocationsEditor'
import type { HandoverView } from './types'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const view: HandoverView = {
  schedule_id: 'schedule-1',
  locations: [
    { id: 'loc-a', name: 'Balai Desa Tempe', recipients: 2, assigned: 2, dates: [], status: 'none' },
    { id: 'loc-b', name: 'Balai Ds Tmpe', recipients: 1, assigned: 1, dates: [], status: 'none' },
    { id: 'loc-c', name: 'Aula Kosong', recipients: 0, assigned: 0, dates: [], status: 'empty' },
  ],
  unassigned: [{ slot_id: 'slot-9', slot_number: 9, name: 'Budi', district: 'Tempe', village: 'Siengkang' }],
}

function renderEditor() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><HandoverLocationsEditor scheduleID="schedule-1" view={view} onClose={vi.fn()} /></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockResolvedValue({ data: view })
})

test('merges a misspelled variant into the right handover location', async () => {
  renderEditor()
  fireEvent.click(screen.getAllByRole('button', { name: 'Gabungkan' })[1])
  fireEvent.change(screen.getByLabelText('Gabungkan Balai Ds Tmpe ke'), { target: { value: 'loc-a' } })
  fireEvent.click(screen.getByRole('button', { name: 'Gabungkan sekarang' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/handover-locations/loc-b/merge?schedule_id=schedule-1', { method: 'POST', body: JSON.stringify({ target_id: 'loc-a' }) }))
})

test('renames a location and offers delete only when unused', async () => {
  renderEditor()
  expect(screen.getAllByRole('button', { name: 'Hapus' })).toHaveLength(1)
  fireEvent.click(screen.getAllByRole('button', { name: 'Ubah nama' })[0])
  fireEvent.change(screen.getByLabelText('Nama baru Balai Desa Tempe'), { target: { value: 'Balai Desa Tempe Baru' } })
  fireEvent.click(screen.getByRole('button', { name: 'Simpan nama' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/handover-locations/loc-a?schedule_id=schedule-1', { method: 'PATCH', body: JSON.stringify({ name: 'Balai Desa Tempe Baru' }) }))
})

test('fills the handover location of a recipient without one', async () => {
  renderEditor()
  fireEvent.change(screen.getByLabelText('Titik serah Budi'), { target: { value: 'loc-a' } })
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/handover-locations/assign?schedule_id=schedule-1', { method: 'PUT', body: JSON.stringify({ slot_id: 'slot-9', location_id: 'loc-a' }) }))
})
