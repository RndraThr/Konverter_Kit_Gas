import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { Training100Panel } from './Training100Panel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const settings = { schedule_id: 'schedule-1', rakorda_location: 'Aula Kantor Bupati', rakorda_row_count: 45, sosialisasi_location: 'Balai Desa Tempe', sosialisasi_row_count: 51, training_10_location: 'Aula Kecamatan Tempe', training_10_row_count: 51, training_100_location: 'Aula Kecamatan Tempe', training_100_row_count: 51 }
const dates = [{ local_date: '2026-10-05', recipient_count: 23, participant_count: 23 }, { local_date: '2026-10-02', recipient_count: 4, participant_count: 4 }]

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <Training100Panel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
}

function mockApi(overrides: Partial<typeof settings> = {}) {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: { ...settings, ...overrides } }
    if (path.includes('/uploads')) return { data: [] }
    if (path.includes('/dates')) return { data: dates }
    throw new Error(`unexpected ${path}`)
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('lists distribution dates like the daily recap and previews the selected day', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview BA Training 100%')).toBeVisible()
  expect(screen.getByLabelText('Lokasi kegiatan')).toHaveValue('Aula Kecamatan Tempe')
  expect(screen.queryByLabelText('Jumlah baris')).not.toBeInTheDocument()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/training-100/preview', expect.objectContaining({ body: JSON.stringify({ schedule_id: 'schedule-1', date: '2026-10-05' }) })))

  await userEvent.click(screen.getByRole('button', { name: /02 Oktober 2026/ }))
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/training-100/preview', expect.objectContaining({ body: JSON.stringify({ schedule_id: 'schedule-1', date: '2026-10-02' }) })))
  expect(apiRequest).toHaveBeenCalledWith(expect.stringContaining('/api/v1/bast/training-100/uploads?schedule_id=schedule-1&date=2026-10-02'))
})

test('previews with a blank location so it can be filled by hand', async () => {
  mockApi({ training_100_location: '' })
  renderPanel()
  expect(await screen.findByText('Lokasi belum diisi: dicetak titik-titik untuk ditulis tangan.')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/training-100/preview', expect.objectContaining({ method: 'POST' })))
})
