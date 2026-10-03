import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { RakordaPanel } from './RakordaPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const settings = { schedule_id: 'schedule-1', rakorda_location: 'Aula Kantor Bupati', rakorda_row_count: 45 }

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <RakordaPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
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
  vi.stubGlobal('open', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('shows the blank attendance sheet preview inline once settings are saved, without a manual preview click', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview Daftar Hadir RAKORDA')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/rakorda/preview', expect.objectContaining({ method: 'POST' })))
  expect(screen.queryByRole('button', { name: /buat preview/i })).not.toBeInTheDocument()
  expect(window.open).not.toHaveBeenCalled()
})

test('skips the preview fetch when the RAKORDA location has not been saved yet', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/settings')) return { data: { ...settings, rakorda_location: '' } }
    if (path.includes('/uploads')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  await screen.findByText('Belum diatur')
  expect(apiBlobRequest).not.toHaveBeenCalled()
})
