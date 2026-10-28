import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { ClosingTitikSerahPanel } from './ClosingTitikSerahPanel'
import type { HandoverView } from './types'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const rows = [
  { local_date: '2026-10-01', machine_brand: 'SHARK', machine_type: 'SPWP 80-30/3"', count: 12 },
  { local_date: '2026-10-02', machine_brand: 'SHARK', machine_type: 'SPWP 80-30/3"', count: 8 },
]

const handover: HandoverView = {
  schedule_id: 'schedule-1',
  locations: [
    { id: 'loc-a', name: 'Balai Desa Tempe', recipients: 20, assigned: 20, dates: ['2026-10-01', '2026-10-02'], status: 'none' },
    { id: 'loc-b', name: 'Lapangan Sabbangparu', recipients: 4, assigned: 6, dates: ['2026-10-02'], status: 'final', document_date: '2026-10-03' },
    { id: 'loc-c', name: 'Gudang Kabupaten Lain', recipients: 0, assigned: 0, dates: [], status: 'empty' },
  ],
  unassigned: [
    { slot_id: 's7', slot_number: 7, name: 'Ahmad', district: 'Tempe', village: 'Siengkang' },
    { slot_id: 's8', slot_number: 8, name: 'Budi', district: 'Tempe', village: 'Siengkang' },
  ],
}

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <ClosingTitikSerahPanel scheduleID="schedule-1" regencyName="Wajo" programType="farmer" defaultDate="2026-10-03" />
  </PermissionsProvider></QueryClientProvider>)
}

function mockApi(rowItems = rows, documents: unknown[] = []) {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/handover-locations')) return { data: handover }
    if (path.includes('/rows')) return { data: rowItems }
    if (path.includes('/documents')) return { data: documents }
    throw new Error(`unexpected ${path}`)
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  vi.stubGlobal('open', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('shows the closing titik serah preview of the first location inline, without opening a new tab', async () => {
  renderPanel()
  expect(await screen.findByText('20')).toBeVisible()
  expect(await screen.findByTitle('Preview Closing Titik Serah')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/closing-titik-serah/preview', expect.objectContaining({ method: 'POST', body: expect.stringContaining('"location_id":"loc-a"') })))
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/closing-titik-serah/rows?schedule_id=schedule-1&location_id=loc-a')
  expect(window.open).not.toHaveBeenCalled()
})

test('summarises every handover location and warns about recipients without one', async () => {
  renderPanel()
  expect(await screen.findByText('2 penerima selesai distribusi belum ada titik serah. Isi lewat "Rapikan titik serah".')).toBeVisible()
  // Locations of the regency unused by this schedule stay out of the recap.
  expect(screen.queryByText('Gudang Kabupaten Lain')).toBeNull()
  expect(screen.getByText('Belum dibuat')).toBeVisible()
  expect(screen.getByText('Final')).toBeVisible()
  expect(screen.getByText('4 penerima selesai dari 6')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: /Lapangan Sabbangparu/ }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/closing-titik-serah/rows?schedule_id=schedule-1&location_id=loc-b'))
  expect(await screen.findByText('BA Serah Terima · Lapangan Sabbangparu')).toBeVisible()
})

test('shows a configuration message and skips the preview fetch when the location has no completed distributions', async () => {
  mockApi([])
  renderPanel()
  expect(await screen.findByText('Belum ada distribusi selesai pada titik serah ini.')).toBeVisible()
  expect(apiBlobRequest).not.toHaveBeenCalled()
})

test('keeps a stale closing titik serah downloadable while allowing finalization again', async () => {
  mockApi(rows, [{ id: 'stale-titik', version: 1, status: 'stale', document_date: '2026-10-03' }])
  renderPanel()
  expect(await screen.findByText('Perlu dibuat ulang')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Unduh' })).toHaveAttribute('href', '/api/v1/bast/closing-titik-serah/documents/stale-titik/content')
  expect(screen.getByRole('button', { name: 'Finalisasi & sinkronkan' })).toBeEnabled()
})
