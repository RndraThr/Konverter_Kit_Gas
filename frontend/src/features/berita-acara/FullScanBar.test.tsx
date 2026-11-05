import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { FullScanCard } from './FullScanBar'
import type { FullScanStatus } from './fullScan'
import { useWetSignatureBlocked } from './WetSignatureBar'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn(), ApiError: class extends Error {} }))

let full: FullScanStatus

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>{children}</PermissionsProvider></QueryClientProvider>
}

beforeEach(() => {
  vi.clearAllMocks()
  full = { supported: true, code: 'ABCD1234', page_count: 3 }
  vi.mocked(apiRequest).mockImplementation(async (path: string) => ({ data: path.includes('/full-scan') ? full : { supported: true, enabled: false } }))
})

test('asks to print and scan the whole BA, with the page count', async () => {
  render(<FullScanCard scheduleID="schedule-1" document="dp3" date="2026-10-12" label="DP3" />, { wrapper })
  expect(await screen.findByText('Pindai lengkap')).toBeVisible()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  expect(screen.getByText(/semua 3 halamannya/)).toBeVisible()
  expect(screen.getByText('Belum ada pindaian')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Cetak BA' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pindai' })).toBeVisible()
})

test('a scan is listed with its pages and whether it is ready', async () => {
  full = { ...full, scan: { uploaded_at: '2026-10-12T01:00:00Z', page_count: 3, valid: false } }
  render(<FullScanCard scheduleID="schedule-1" document="dp3" date="2026-10-12" label="DP3" />, { wrapper })
  expect(await screen.findByText('Pindaian DP3')).toBeVisible()
  expect(screen.getByText('Perlu dipindai ulang')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Hapus pindaian DP3' })).toBeVisible()
})

test('uploads a PDF as multipart "file"', async () => {
  const { container } = render(<FullScanCard scheduleID="schedule-1" document="dp3" date="2026-10-12" label="DP3" />, { wrapper })
  await screen.findByText('Pindai lengkap')
  const input = container.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, new File(['%PDF'], 'ba.pdf', { type: 'application/pdf' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith(expect.stringContaining('/api/v1/bast/full-scan/scan?'), expect.objectContaining({ method: 'PUT' })))
  const call = vi.mocked(apiRequest).mock.calls.find(([path]) => String(path).includes('/full-scan/scan'))
  expect((call?.[1]?.body as FormData).get('file')).toBeInstanceOf(File)
})

test('finalize waits only when the scan is for older content', async () => {
  // No scan: the system makes the BA, nothing to wait for.
  const none = renderHook(() => useWetSignatureBlocked('schedule-1', 'dp3', '2026-10-12'), { wrapper })
  await waitFor(() => expect(apiRequest).toHaveBeenCalled())
  expect(none.result.current).toBe(false)
  full = { ...full, scan: { uploaded_at: '2026-10-12T01:00:00Z', page_count: 3, valid: false } }
  const stale = renderHook(() => useWetSignatureBlocked('schedule-1', 'dp3', '2026-10-12'), { wrapper })
  await waitFor(() => expect(stale.result.current).toBe(true))
  full = { ...full, scan: { uploaded_at: '2026-10-12T01:00:00Z', page_count: 3, valid: true } }
  const ready = renderHook(() => useWetSignatureBlocked('schedule-1', 'dp3', '2026-10-12'), { wrapper })
  await waitFor(() => expect(apiRequest).toHaveBeenCalledTimes(6))
  expect(ready.result.current).toBe(false)
})

