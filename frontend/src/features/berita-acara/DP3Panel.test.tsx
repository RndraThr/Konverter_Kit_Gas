import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { DP3Panel } from './DP3Panel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <DP3Panel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/summary')) return { data: { total_recipients: 2, numbered_recipients: 1, unmounted_recipients: 1, validation_status: 'ready' } }
    if (path.includes('/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  vi.stubGlobal('open', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('shows the DP3 preview inline as soon as the document is ready, without opening a new tab', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview DP3')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/dp3/preview', expect.objectContaining({ method: 'POST' })))
  expect(window.open).not.toHaveBeenCalled()
})

test('does not fetch a preview while the document is not ready', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/summary')) return { data: { total_recipients: 0, numbered_recipients: 0, unmounted_recipients: 0, validation_status: 'no_recipients' } }
    if (path.includes('/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  expect(await screen.findByText('Belum ada penerima pada jadwal ini.')).toBeVisible()
  expect(apiBlobRequest).not.toHaveBeenCalled()
})
