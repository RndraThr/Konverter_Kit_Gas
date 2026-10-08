import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { SosialisasiPanel } from './SosialisasiPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const settings = { schedule_id: 'schedule-1', rakorda_location: 'Aula Kantor Bupati', rakorda_row_count: 45, sosialisasi_location: 'Balai Desa Tempe', sosialisasi_row_count: 51 }

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <SosialisasiPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: settings }
    if (path.includes('/uploads')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('uses the sosialisasi settings and endpoints, not the RAKORDA ones', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview BA Sosialisasi')).toBeVisible()
  expect(screen.getByText('Balai Desa Tempe')).toBeVisible()
  expect(screen.getByText('7. SOSIALISASI')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/sosialisasi/preview', expect.objectContaining({ method: 'POST' })))
  expect(apiRequest).toHaveBeenCalledWith(expect.stringContaining('/api/v1/bast/sosialisasi/uploads?schedule_id=schedule-1'))
})

test('skips the preview fetch when the sosialisasi location has not been saved yet', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: { ...settings, sosialisasi_location: '' } }
    if (path.includes('/uploads')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  await screen.findByText('Belum diatur')
  expect(apiBlobRequest).not.toHaveBeenCalled()
})
