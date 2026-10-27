import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { HandoverLocationsEditor } from './HandoverLocationsEditor'
import type { HandoverView } from './types'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const view: HandoverView = {
  schedule_id: 'schedule-1',
  locations: [{ id: 'loc-a', name: 'Balai Desa Tempe', areas: [], sort_order: 0, recipients: 2, assigned: 2, dates: [], status: 'none' }],
  areas: [{ district: 'TEMPE', village: 'SIENGKANG', recipients: 1 }, { district: 'SABBANGPARU', village: 'MADDUKELLENG', recipients: 1 }],
  recipients: [
    { slot_id: 'slot-1', slot_number: 1, name: 'Ahmad', district: 'Tempe', village: 'Siengkang', location_id: 'loc-a', source: 'single', completed: true },
    { slot_id: 'slot-2', slot_number: 2, name: 'Budi', district: 'Sabbangparu', village: 'Maddukelleng', location_id: 'loc-a', source: 'single', completed: false },
  ],
  unassigned: 0,
  unassigned_total: 0,
}

function renderEditor() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><HandoverLocationsEditor scheduleID="schedule-1" view={view} onClose={vi.fn()} /></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockResolvedValue({ data: view })
})

test('adds a second handover location with district coverage and saves the whole list', async () => {
  renderEditor()
  fireEvent.click(screen.getByRole('button', { name: 'Tambah titik serah' }))
  fireEvent.change(screen.getByLabelText('Nama titik serah 2'), { target: { value: 'Lapangan Sabbangparu' } })
  const sabbangparu = screen.getAllByText('Kec. SABBANGPARU')
  fireEvent.click(sabbangparu[1])
  fireEvent.click(screen.getByRole('button', { name: 'Simpan titik serah' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/handover-locations', expect.objectContaining({ method: 'PUT' })))
  const body = JSON.parse(String(vi.mocked(apiRequest).mock.calls[0][1]?.body))
  expect(body.locations).toEqual([
    { id: 'loc-a', name: 'Balai Desa Tempe', areas: [] },
    { id: '', name: 'Lapangan Sabbangparu', areas: [{ district: 'SABBANGPARU' }] },
  ])
})

test('moves one recipient to a handover location manually', async () => {
  renderEditor()
  expect(screen.getAllByText('Otomatis: Balai Desa Tempe (satu lokasi)')).toHaveLength(2)
  fireEvent.change(screen.getByLabelText('Titik serah Budi'), { target: { value: 'loc-a' } })
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/handover-locations/override', { method: 'PUT', body: JSON.stringify({ schedule_id: 'schedule-1', slot_id: 'slot-2', location_id: 'loc-a' }) }))
})
