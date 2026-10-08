import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { TKDNPanel } from './TKDNPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const rows = [{ name: 'Pompa Air Irigasi', group: '', ref: 'machine' }, { name: 'Konverter Kit', group: 'Konverter Kit dan kelengkapannya', ref: 'converter' }]
const profile = { program_id: 'program-1', rows, total_tkdn: 68.53, is_default: true }
const entries = [{ ref: 'machine', kind: 'machine', label: 'Mesin', unit: 'PAKET', quantity_per_package: 1, variants: [{ code: 'shark', brand: 'SHARK', type: 'SPWP', spec: '', tkdn_percent: 61.42 }] }, { ref: 'converter', kind: 'converter', label: 'Konkit', unit: 'PAKET', quantity_per_package: 1, variants: [{ code: 'ergas', brand: 'ERGAS', type: '', spec: '', tkdn_percent: 93.27 }] }]
const items = [
  { group: '', name: 'Pompa Air Irigasi', brand: 'SHARK', quantity_per_package: 1, tkdn_percent: 61.42 },
  { group: 'Konverter Kit dan kelengkapannya', name: 'Konverter Kit', brand: 'ERGAS', quantity_per_package: 1, tkdn_percent: 93.27 },
]
const scheduleItems = {
  schedule_id: 'schedule-1', program_id: 'program-1', zone_name: 'ZONA 1', machine_brand: 'SHARK',
  items: [
    { ref: 'machine', name: 'Pompa Air Irigasi', variants: [{ code: 'shark', brand: 'SHARK' }, { code: 'honda', brand: 'HONDA' }], selected: 'shark', source: 'machine', po_number: 'PO-Z1-SHARK', inspected: true },
    { ref: 'converter', name: 'Konverter Kit', variants: [{ code: 'ergas', brand: 'ERGAS' }], selected: 'ergas', source: 'single', po_number: '', inspected: true },
  ],
}

function mockApi(totalPackages: number) {
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/tkdn/summary')) return { data: { total_packages: totalPackages, profile, items, entries } }
    if (path.includes('/tkdn/documents')) return { data: [] }
    if (path.includes('/tkdn/profile') && init?.method === 'PUT') return { data: { ...profile, is_default: false } }
    if (path.includes('/items/schedule')) return { data: scheduleItems }
    throw new Error(`unexpected ${path}`)
  })
}

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <TKDNPanel scheduleID="schedule-1" programID="program-1" regencyName="Wajo" programType="farmer" defaultDate="2026-11-02" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('previews the TKDN document for the regency', async () => {
  mockApi(12)
  renderPanel()
  expect(await screen.findByTitle('Preview Realisasi TKDN')).toBeVisible()
  expect(screen.getByText('12 paket')).toBeVisible()
  expect(screen.getByText(/2 baris TKDN/)).toBeVisible()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/tkdn/preview', expect.objectContaining({ method: 'POST' })))
})

test('saves the TKDN layout with the combined value; brands come from the package template', async () => {
  mockApi(0)
  renderPanel()
  const total = await screen.findByLabelText('TKDN gabungan (%)')
  await userEvent.clear(total)
  await userEvent.type(total, '70.1')
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))

  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/tkdn/profile', expect.objectContaining({ method: 'PUT' })))
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([path]) => path === '/api/v1/bast/tkdn/profile')![1]!.body as string)
  expect(body).toEqual({ program_id: 'program-1', rows, total_tkdn: 70.1 })
  expect(apiBlobRequest).not.toHaveBeenCalled()
})

test('edits a TKDN row name in the layout editor', async () => {
  mockApi(0)
  renderPanel()
  await userEvent.click(await screen.findByRole('button', { name: 'Susunan TKDN' }))
  const name = screen.getByLabelText('Nama baris 1')
  await userEvent.clear(name)
  await userEvent.type(name, 'Pompa Irigasi')
  expect(screen.getByLabelText('Barang template baris 1')).toHaveTextContent('Opsi mesin · SHARK')
  await userEvent.click(screen.getByRole('button', { name: 'Simpan susunan TKDN' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/tkdn/profile', expect.objectContaining({ method: 'PUT' })))
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([path]) => path === '/api/v1/bast/tkdn/profile')![1]!.body as string)
  expect(body.rows[0]).toEqual({ name: 'Pompa Irigasi', group: '', ref: 'machine' })
})
