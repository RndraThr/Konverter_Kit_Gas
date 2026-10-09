import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { ServisBerkalaPanel } from './ServisBerkalaPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const settings = { schedule_id: 'schedule-1', servis_1_start: '2027-05-03', servis_1_end: '2027-05-08', servis_2_start: '2027-11-01', servis_2_end: '2027-11-06' }
const readySummary = { total_packages: 12, services: [{ start: '2027-05-03', end: '2027-05-08' }, { start: '2027-11-01', end: '2027-11-06' }], schedule_ready: true }

function mockApi(summary: typeof readySummary) {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: settings }
    if (path.includes('/servis-berkala/summary')) return { data: summary }
    if (path.includes('/servis-berkala/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
}

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <ServisBerkalaPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-06" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('previews the servis berkala document once both service periods are saved', async () => {
  mockApi(readySummary)
  renderPanel()
  expect(await screen.findByTitle('Preview BA Servis Berkala')).toBeVisible()
  expect(screen.getByText('12 paket')).toBeVisible()
  expect(screen.getByLabelText(/^Mulai/, { selector: '#servis_1_start' })).toHaveValue('2027-05-03')
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/servis-berkala/preview', expect.objectContaining({ method: 'POST' })))
})

test('previews without service dates but blocks finalizing until they are saved', async () => {
  mockApi({ ...readySummary, schedule_ready: false })
  renderPanel()
  expect(await screen.findByText(/Tanggal servis belum lengkap/)).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/servis-berkala/preview', expect.objectContaining({ method: 'POST' })))
  expect(screen.getByRole('button', { name: 'Finalisasi & sinkronkan' })).toBeDisabled()
})
