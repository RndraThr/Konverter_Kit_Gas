import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { ClosingKabupatenPanel } from './ClosingKabupatenPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const rows = [
  { location: 'Lapangan Desa Tempe', machine_brand: 'SHARK', machine_type: 'SPWP 80-30/3"', count: 20 },
  { location: 'Lapangan Desa Tempe', machine_brand: 'YANMAR', machine_type: 'TF 85', count: 5 },
]

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <ClosingKabupatenPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/rows')) return { data: rows }
    if (path.includes('/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  vi.stubGlobal('open', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('shows the closing kabupaten preview inline once rows are available, without opening a new tab', async () => {
  renderPanel()
  expect(await screen.findByText('25')).toBeVisible()
  expect(await screen.findByTitle('Preview Closing Kabupaten')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/closing-kabupaten/preview', expect.objectContaining({ method: 'POST' })))
  expect(window.open).not.toHaveBeenCalled()
})

test('shows a configuration message and skips the preview fetch when there are no completed distributions', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/rows')) return { data: [] }
    if (path.includes('/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  expect(await screen.findByText('Belum ada distribusi selesai pada jadwal ini.')).toBeVisible()
  expect(apiBlobRequest).not.toHaveBeenCalled()
})
