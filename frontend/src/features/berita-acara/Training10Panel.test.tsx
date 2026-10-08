import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { Training10Panel } from './Training10Panel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const settings = { schedule_id: 'schedule-1', training_10_location: 'Aula Kecamatan Tempe', training_10_row_count: 51, training_100_location: 'Balai Desa', training_100_row_count: 51 }

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: settings }
    if (path.includes('/uploads')) return { data: [] }
    if (path.includes('/training-10/dates')) return { data: [{ local_date: '2026-10-05', recipient_count: 23, participant_count: 3 }] }
    throw new Error(`unexpected ${path}`)
  })
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('uses the Training 10% endpoints and shows the first ten percent of the day', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <Training10Panel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
  expect(await screen.findByTitle('Preview BA Training 10%')).toBeVisible()
  expect(screen.getByText('3 dari 23 penerima')).toBeVisible()
  expect(screen.getByLabelText('Lokasi kegiatan')).toHaveValue('Aula Kecamatan Tempe')
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/training-10/preview', expect.objectContaining({ method: 'POST' })))
})
