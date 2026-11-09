import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { DP3ExcelCard } from './DP3ExcelCard'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), ApiError: class ApiError extends Error {} }))

const synced = {
  available: true, filename: 'DP3 - WAJO - Data Penerima.xlsx', recipient_count: 12,
  synced_at: '2026-11-06T03:20:00Z', last_error: '', last_attempt_at: '2026-11-06T03:20:00Z',
}

function renderCard(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <DP3ExcelCard scheduleID="schedule-1" regencyName="Wajo" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
})

test('offers the synced workbook for download with its recipient count and time', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ data: synced })
  renderCard(['bast.view'])
  expect(await screen.findByText('12 penerima · diperbarui 6 November 2026 pukul 10.20 WIB')).toBeVisible()
  expect(screen.getByRole('button', { name: /Unduh Excel/ })).toHaveAttribute('href', '/api/v1/bast/dp3/excel/content?schedule_id=schedule-1')
  expect(screen.queryByRole('button', { name: /Perbarui sekarang/ })).not.toBeInTheDocument()
})

test('hides the download until a workbook exists and lets a manager rebuild it now', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path === '/api/v1/bast/dp3/excel/sync') return { data: synced }
    return { data: { available: false, filename: '', recipient_count: 0, synced_at: null, last_error: 'drive down', last_attempt_at: '2026-11-06T03:00:00Z' } }
  })
  renderCard()
  expect(await screen.findByText('Pembaruan terakhir gagal. Sistem mencoba lagi otomatis.')).toBeVisible()
  expect(screen.queryByRole('button', { name: /Unduh Excel/ })).not.toBeInTheDocument()

  await userEvent.click(screen.getByRole('button', { name: /Perbarui sekarang/ }))
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/dp3/excel/sync', expect.objectContaining({ method: 'POST', body: JSON.stringify({ schedule_id: 'schedule-1' }) }))
  expect(await screen.findByRole('button', { name: /Unduh Excel/ })).toBeVisible()
})
