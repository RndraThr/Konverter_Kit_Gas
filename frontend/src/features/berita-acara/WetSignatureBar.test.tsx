import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, renderHook, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { useWetSignatureBlocked, WetSignatureBar } from './WetSignatureBar'
import type { WetSignatureStatus } from './wetScan'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn(), ApiError: class extends Error {} }))

let status: WetSignatureStatus

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>{children}</PermissionsProvider></QueryClientProvider>
}

function renderBar() {
  return render(<WetSignatureBar scheduleID="schedule-1" document="dp3" date="2026-10-12" label="DP3" />, { wrapper })
}

beforeEach(() => {
  vi.clearAllMocks()
  status = { supported: true, enabled: true, code: 'ABCD1234' }
  vi.mocked(apiRequest).mockImplementation(async () => ({ data: status }))
})

test('renders nothing while wet-signature mode is off', async () => {
  status = { supported: true, enabled: false }
  const { container } = renderBar()
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/wet-signature?schedule_id=schedule-1&document=dp3&date=2026-10-12'))
  expect(container).toBeEmptyDOMElement()
})

test('asks for the signed page with the document code before finalizing', async () => {
  renderBar()
  expect(await screen.findByText('Kode ABCD1234')).toBeVisible()
  expect(screen.getByText('Belum diunggah')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Cetak halaman tanda tangan' })).toBeEnabled()
  expect(screen.getByText('Unggah hasil pindai')).toBeVisible()
})

test('flags a scan made for older content', async () => {
  status = { ...status, scan: { uploaded_at: '2026-10-12T01:00:00Z', valid: false } }
  renderBar()
  expect(await screen.findByText('Perlu ditandatangani ulang')).toBeVisible()
  expect(screen.getByText('Unggah ulang')).toBeVisible()
})

test('finalize stays blocked until a matching scan exists', async () => {
  const blocked = renderHook(() => useWetSignatureBlocked('schedule-1', 'dp3', '2026-10-12'), { wrapper })
  await waitFor(() => expect(blocked.result.current).toBe(true))

  status = { ...status, scan: { uploaded_at: '2026-10-12T01:00:00Z', valid: true } }
  const ready = renderHook(() => useWetSignatureBlocked('schedule-1', 'dp3', '2026-10-12'), { wrapper })
  await waitFor(() => expect(apiRequest).toHaveBeenCalledTimes(2))
  expect(ready.result.current).toBe(false)
})
