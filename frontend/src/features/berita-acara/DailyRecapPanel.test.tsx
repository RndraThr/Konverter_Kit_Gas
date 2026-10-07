import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { DailyRecapPanel } from './DailyRecapPanel'

vi.mock('@/lib/api', () => ({
  apiRequest: vi.fn(),
  apiBlobRequest: vi.fn(),
  ApiError: class ApiError extends Error {},
}))

let dates = [{ local_date: '2026-10-02', recipient_count: 3, validation_status: 'ready' }]

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <DailyRecapPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  dates = [{ local_date: '2026-10-02', recipient_count: 3, validation_status: 'ready' }]
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/dates')) return { data: dates }
    if (path.includes('/documents')) return { data: [] }
    throw new Error(`unexpected ${path}`)
  })
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  vi.stubGlobal('open', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('shows the daily recap preview inline for the selected date, without opening a new tab', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview Rekap Harian')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/daily-recap/preview', expect.objectContaining({ method: 'POST' })))
  expect(window.open).not.toHaveBeenCalled()
})

test('does not request a preview and explains why when recipient data is incomplete', async () => {
  dates = [{ local_date: '2026-10-02', recipient_count: 1, validation_status: 'machine_power_required' }]
  renderPanel()

  expect(await screen.findByText('Daya mesin belum lengkap pada sebagian penerima.')).toBeVisible()
  expect(apiBlobRequest).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Finalisasi & sinkronkan' })).toBeDisabled()
})

test('shows the API reason when a ready preview request still fails', async () => {
  vi.mocked(apiBlobRequest).mockRejectedValue(new Error('Snapshot verifikasi distribusi belum lengkap'))
  renderPanel()

  expect(await screen.findByText('Snapshot verifikasi distribusi belum lengkap')).toBeVisible()
})

test('keeps a stale daily recap downloadable while allowing finalization again', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/dates')) return { data: dates }
    if (path.includes('/documents')) return { data: [{ id: 'stale-daily', version: 1, status: 'stale', document_date: '2026-10-02' }] }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  expect(await screen.findByText('Perlu dibuat ulang')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Unduh' })).toHaveAttribute('href', '/api/v1/bast/daily-recap/documents/stale-daily/content')
  expect(screen.getByRole('button', { name: 'Finalisasi & sinkronkan' })).toBeEnabled()
})
