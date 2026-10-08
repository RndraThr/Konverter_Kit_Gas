import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiBlobRequest, apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { PemeriksaanPanel } from './PemeriksaanPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const checklist = { packaging: 'baik', quantity: 'lengkap', specification: 'sesuai', condition: 'baru_baik', documents: [], other_document: '', function_test: 'ya', conclusion: 'diterima' }
const profile = {
  program_id: 'program-1', is_default: true,
  forms: [
    { code: 'mesin', title: 'Mesin Pompa', rows: [], checklist, note: '' },
    { code: 'oli', title: 'Oli', rows: [], checklist, note: '' },
  ],
}
const forms = [
  { code: 'mesin', title: 'Mesin Pompa', po_number: 'PO-MESIN', rows: [{ ref: 'machine', description: 'MESIN SHARK', unit: 'PAKET', quantity_per_package: 1, notes: '' }] },
  { code: 'oli', title: 'Oli', po_number: '', rows: [{ ref: 'component:oil', description: 'OLI PERTAMINA', unit: 'LITER', quantity_per_package: 2, notes: '' }] },
]
const scheduleItems = { schedule_id: 'schedule-1', program_id: 'program-1', zone_name: 'ZONA 1', machine_brand: 'SHARK', items: [] }
const activeOli = { id: 'doc-oli', schedule_id: 'schedule-1', program_id: 'program-1', regency_id: 'r', document_type: 'pemeriksaan:oli', document_date: '2026-11-03', filename: 'BA PEMERIKSAAN OLI.pdf', recipient_count: 12, page_count: 2, version: 3, status: 'active', checksum: 'x', finalized_at: '2026-11-03T00:00:00Z' }

function mockApi(summaryProfile = profile) {
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/pemeriksaan/summary')) return { data: { total_packages: 12, profile: summaryProfile, forms, entries: [] } }
    if (path.includes('/items/schedule')) return { data: scheduleItems }
    if (path.includes('/pemeriksaan/documents')) return { data: [activeOli] }
    if (path.includes('/pemeriksaan/finalize') && init?.method === 'POST') return { data: [activeOli, { ...activeOli, id: 'doc-mesin', document_type: 'pemeriksaan:mesin' }] }
    throw new Error(`unexpected ${path}`)
  })
}

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.view', 'bast.manage']}>
    <PemeriksaanPanel scheduleID="schedule-1" programID="program-1" regencyName="Wajo" programType="farmer" defaultDate="2026-11-03" />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi()
  vi.mocked(apiBlobRequest).mockResolvedValue(new Blob(['%PDF-preview'], { type: 'application/pdf' }))
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:preview') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

test('lists every form with quantities, PO from the item master, and final status beside one combined preview', async () => {
  renderPanel()
  expect(await screen.findByTitle('Preview BA Pemeriksaan')).toBeVisible()
  const list = screen.getByRole('list', { name: 'Form BA Pemeriksaan' })
  expect(within(list).getByText('12 PAKET')).toBeVisible()
  expect(within(list).getByText('24 LITER')).toBeVisible()
  expect(within(list).getByText('PO PO-MESIN')).toBeVisible()
  expect(within(list).getByText('PO belum diisi')).toBeVisible()
  expect(within(list).getByText('V3')).toBeVisible()
  expect(within(list).getByText('Draft')).toBeVisible()
  expect(screen.getByText('1/2 final')).toBeVisible()
  expect(screen.queryByLabelText(/No. Purchase Order/)).not.toBeInTheDocument()
  await waitFor(() => expect(apiBlobRequest).toHaveBeenCalledWith('/api/v1/bast/pemeriksaan/preview', expect.objectContaining({ method: 'POST' })))
})

test('opens a form without default supporting documents in the editor without crashing', async () => {
  // Regresi: server mengirim "documents": null untuk form tanpa dokumen default.
  mockApi({ ...profile, forms: profile.forms.map((form) => ({ ...form, checklist: { ...form.checklist, documents: null } })) } as unknown as typeof profile)
  renderPanel()
  await userEvent.click(await screen.findByRole('button', { name: 'Atur daftar form' }))
  await userEvent.click(within(screen.getByRole('navigation', { name: 'Form BA Pemeriksaan' })).getByRole('button', { name: 'Oli' }))
  expect(screen.getByLabelText('Judul form')).toHaveValue('Oli')
  expect(screen.getByRole('group', { name: 'Dokumen Pendukung' })).toBeVisible()
})

test('picks form rows by ticking package template items', async () => {
  const entries = [
    { ref: 'machine', kind: 'machine', label: 'Mesin', unit: 'PAKET', quantity_per_package: 1, variants: [{ code: 'shark', brand: 'SHARK', type: 'SPWP', spec: '', tkdn_percent: 61.42 }] },
    { ref: 'component:oil', kind: 'component', label: 'OLI', unit: 'Ltr', quantity_per_package: 2, variants: [{ code: 'oil', brand: 'Pertamina Enduro', type: '', spec: '', tkdn_percent: 53.42 }] },
  ]
  const withRows = { ...profile, forms: profile.forms.map((form) => ({ ...form, rows: [{ ref: form.code === 'mesin' ? 'machine' : 'component:oil', description: '', unit: '', quantity_per_package: 0, notes: '' }] })) }
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.includes('/pemeriksaan/summary')) return { data: { total_packages: 12, profile: withRows, forms, entries } }
    if (path.includes('/pemeriksaan/documents')) return { data: [] }
    if (path.includes('/pemeriksaan/profile') && init?.method === 'PUT') return { data: withRows }
    throw new Error(`unexpected ${path}`)
  })
  renderPanel()
  await userEvent.click(await screen.findByRole('button', { name: 'Atur daftar form' }))
  const oil = screen.getByRole('checkbox', { name: 'Komponen · OLI (Pertamina Enduro)' })
  expect(screen.getByRole('checkbox', { name: 'Opsi mesin · SHARK' })).toBeChecked()
  expect(oil).not.toBeChecked()
  await userEvent.click(oil)
  await userEvent.click(screen.getByRole('button', { name: 'Simpan daftar form' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/pemeriksaan/profile', expect.objectContaining({ method: 'PUT' })))
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([path]) => path === '/api/v1/bast/pemeriksaan/profile')![1]!.body as string)
  expect(body.forms[0].rows.map((row: { ref: string }) => row.ref)).toEqual(['machine', 'component:oil'])
})

test('jumps the combined preview to the selected form and finalizes all forms at once', async () => {
  renderPanel()
  await screen.findByTitle('Preview BA Pemeriksaan')
  await userEvent.click(screen.getByRole('button', { name: 'Lihat Oli' }))
  expect(screen.getByTitle('Preview BA Pemeriksaan')).toHaveAttribute('src', 'blob:preview#page=3')

  await userEvent.click(screen.getByRole('button', { name: 'Finalisasi semua form' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/pemeriksaan/finalize', expect.objectContaining({ method: 'POST' })))
})
