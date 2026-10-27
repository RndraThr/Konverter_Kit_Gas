import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { GasElpijiPanel } from './GasElpijiPanel'
import type { GasElpijiSummary } from './types'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const receipt = {
  document: { id: 'doc-1', schedule_id: 'schedule-1', program_id: 'p', regency_id: 'r', document_type: 'gas_elpiji', document_date: '2026-12-02', filename: 'BA GAS ELPIJI - V1.pdf', recipient_count: 120, page_count: 1, version: 1, status: 'active' as const, checksum: 'a', finalized_at: '2026-12-02T08:00:00Z' },
  input: { schedule_id: 'schedule-1', document_date: '2026-12-02', warehouse_location: 'SPBE Bangka', warehouse_address: 'Jl. Raya Sungailiat', pic: 'Budi', cylinder_count: 120, remarks: 'Baik', receiver_name: 'Dwi', pertamina_name: 'Rina' },
  wet_scanned: false,
}

function mockApi(summary: GasElpijiSummary, wet: { enabled: boolean; scan?: { uploaded_at: string; valid: boolean } } = { enabled: false }) {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.includes('/wet-signature')) return { data: { supported: true, code: 'ABCD1234', ...wet } }
    if (path.includes('/settings')) return { data: { schedule_id: 'schedule-1', installer_name: 'ANDI PELAKSANA', pertamina_rep_name: 'RINA PERTAMINA' } }
    if (path.includes('/gas-elpiji/draft')) return { data: { saved: true } }
    if (path.includes('/gas-elpiji/summary')) return { data: summary }
    if (path.includes('/gas-elpiji/documents')) return { data: [] }
    if (path.includes('/gas-elpiji/finalize')) return { data: receipt.document }
    if (path.includes('/gas-elpiji/merged/finalize')) return { data: { ...receipt.document, id: 'merged-1', document_type: 'gas_elpiji_gabungan' } }
    throw new Error(`unexpected ${path}`)
  })
}

function renderPanel(defaultDate = '2026-12-02') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <GasElpijiPanel scheduleID="schedule-1" regencyName="Bangka" programType="farmer" defaultDate={defaultDate} />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('loads a finalized receipt into the form and offers merging', async () => {
  mockApi({ receipts: [receipt], drafts: [], total_cylinders: 120, merged: null, merged_current: false })
  renderPanel()
  expect(await screen.findByLabelText(/^Lokasi gudang/)).toHaveValue('SPBE Bangka')
  expect(screen.getByLabelText(/^Jumlah tabung/)).toHaveValue(120)
  expect(screen.getByText('1 penerimaan · 120 tabung')).toBeVisible()
  expect(screen.getByText('Belum dibuat')).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/gas-elpiji/preview', expect.objectContaining({ method: 'POST' })))

  fireEvent.click(screen.getByRole('button', { name: 'Gabungkan & finalisasi' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/gas-elpiji/merged/finalize', expect.objectContaining({ method: 'POST', body: JSON.stringify({ schedule_id: 'schedule-1' }) })))
})

test('a new delivery date keeps the warehouse and names but needs a fresh count', async () => {
  mockApi({ receipts: [receipt], drafts: [], total_cylinders: 120, merged: { ...receipt.document, id: 'merged-1' }, merged_current: true })
  renderPanel('2026-12-12')
  expect(await screen.findByLabelText(/^Lokasi gudang/)).toHaveValue('SPBE Bangka')
  expect(screen.getByLabelText(/^Jumlah tabung/)).toHaveValue(null)
  expect(screen.getByText('Terbaru')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Gabungkan & finalisasi' })).toBeDisabled()
  const finalize = screen.getByRole('button', { name: 'Finalisasi penerimaan' })
  expect(finalize).toBeDisabled()

  fireEvent.change(screen.getByLabelText(/^Jumlah tabung/), { target: { value: '80' } })
  expect(finalize).toBeEnabled()
  fireEvent.click(finalize)
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/gas-elpiji/finalize', expect.objectContaining({ method: 'POST' })))
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([path]) => path === '/api/v1/bast/gas-elpiji/finalize')![1]!.body as string)
  expect(body).toMatchObject({ document_date: '2026-12-12', cylinder_count: 80, warehouse_location: 'SPBE Bangka', remarks: '' })
})

test('wet signature mode saves the draft, blocks finalizing without a scan, and holds the merge', async () => {
  mockApi({ receipts: [receipt], drafts: [], total_cylinders: 120, merged: null, merged_current: false }, { enabled: true })
  renderPanel('2026-12-12')
  expect(await screen.findByText('Tanda tangan basah')).toBeVisible()
  expect(screen.getByLabelText(/^Nama penerima/)).toHaveAttribute('placeholder', 'ANDI PELAKSANA')
  expect(screen.getByText(/1 penerimaan belum memakai halaman bertanda tangan basah/)).toBeVisible()
  expect(screen.getByRole('button', { name: 'Gabungkan & finalisasi' })).toBeDisabled()

  fireEvent.change(screen.getByLabelText(/^Jumlah tabung/), { target: { value: '80' } })
  expect(screen.getByRole('button', { name: 'Finalisasi penerimaan' })).toBeDisabled()
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/gas-elpiji/draft', expect.objectContaining({ method: 'PUT' })), { timeout: 3000 })
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([path]) => path === '/api/v1/bast/gas-elpiji/draft')![1]!.body as string)
  expect(body).toMatchObject({ document_date: '2026-12-12', cylinder_count: 80 })
})

test('a saved draft for the date is loaded into the form', async () => {
  const draft = { ...receipt.input, document_date: '2026-12-20', cylinder_count: 55, warehouse_location: 'Gudang Draf' }
  mockApi({ receipts: [receipt], drafts: [draft], total_cylinders: 120, merged: null, merged_current: false })
  renderPanel('2026-12-20')
  expect(await screen.findByLabelText(/^Lokasi gudang/)).toHaveValue('Gudang Draf')
  expect(screen.getByLabelText(/^Jumlah tabung/)).toHaveValue(55)
})
