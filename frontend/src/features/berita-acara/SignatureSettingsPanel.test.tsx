import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { SignatureSettingsPanel } from './SignatureSettingsPanel'
import type { SignatureSettings } from './signatures'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), ApiError: class extends Error {} }))

const globalRules = { agriculture_office: { signature: false, stamp: false }, installer: { signature: true, stamp: true }, supervisor: { signature: true, stamp: false } }
let settings: SignatureSettings

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <SignatureSettingsPanel scheduleID="schedule-1" document="dp3" documentLabel="DP3" />
  </PermissionsProvider></QueryClientProvider>)
}

function writes(path: string, method = 'PUT') {
  return vi.mocked(apiRequest).mock.calls.filter(([url, init]) => url === path && init?.method === method).map(([, init]) => (init?.body ? JSON.parse(String(init.body)) : null))
}

beforeEach(() => {
  vi.clearAllMocks()
  settings = {
    document_key: 'dp3', override: false, wet_signature: false, wet_supported: true, rules: globalRules, global_rules: globalRules,
    signatories: [
      { role: 'agriculture_office', name: 'IR. HARYANTO', nip: '1972' },
      { role: 'installer', name: 'BUDI SANTOSO', nip: '', signature_updated_at: '2026-10-12T00:00:00Z', stamp_updated_at: '2026-10-12T00:00:00Z' },
      { role: 'supervisor', name: '', nip: '' },
    ],
  }
  vi.mocked(apiRequest).mockImplementation(async (path, init) => {
    if (path.startsWith('/api/v1/bast/signatures?')) return { data: settings }
    if (init?.method === 'PUT' || init?.method === 'DELETE') return { data: settings }
    throw new Error(`unexpected ${path}`)
  })
})

test('shows one card per signatory with the signature box, name, and NIP only for Dinas', async () => {
  renderPanel()
  const dinas = await screen.findByRole('region', { name: 'Dinas Pertanian Daerah' })
  expect(within(dinas).getByLabelText('Area tanda tangan Dinas Pertanian Daerah')).toBeVisible()
  expect(within(dinas).getByText('Coret di sini untuk tanda tangan')).toBeVisible()
  expect(within(dinas).getByLabelText('Nama :')).toHaveValue('IR. HARYANTO')
  expect(within(dinas).getByLabelText('NIP :')).toHaveValue('1972')
  const pelaksana = screen.getByRole('region', { name: 'Pelaksana Pemasangan & Pendistribusian' })
  expect(within(pelaksana).queryByLabelText('NIP :')).not.toBeInTheDocument()
  expect(within(pelaksana).getByAltText('Tanda tangan Pelaksana Pemasangan & Pendistribusian')).toHaveAttribute('src', expect.stringContaining('/api/v1/bast/signatures/signatories/installer/signature?schedule_id=schedule-1'))
  expect(within(pelaksana).getByAltText('Stempel Pelaksana Pemasangan & Pendistribusian')).toBeVisible()
  expect(screen.queryByRole('region', { name: 'PT Pertamina Patra Niaga' })).not.toBeInTheDocument()
})

test('keeps changes local until Simpan, then saves the name, NIP, and print option together', async () => {
  renderPanel()
  const dinas = await screen.findByRole('region', { name: 'Dinas Pertanian Daerah' })
  const save = within(dinas).getByRole('button', { name: 'Simpan' })
  expect(save).toBeDisabled()

  const nip = within(dinas).getByLabelText('NIP :')
  await userEvent.clear(nip)
  await userEvent.type(nip, '197203151998031004')
  await userEvent.click(within(dinas).getByRole('checkbox', { name: 'Cetak tanda tangan digital' }))
  expect(writes('/api/v1/bast/signatures')).toHaveLength(0)

  await userEvent.click(save)
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  expect(writes('/api/v1/bast/signatures/signatories/agriculture_office?schedule_id=schedule-1')).toEqual([{ name: 'IR. HARYANTO', nip: '197203151998031004' }])
  const [rules] = writes('/api/v1/bast/signatures')
  expect(rules).toMatchObject({ schedule_id: 'schedule-1', document: 'dp3', override: false })
  expect(rules.rules.agriculture_office).toEqual({ signature: true, stamp: false })
})

test('Batal discards unsaved changes in the card', async () => {
  renderPanel()
  const dinas = await screen.findByRole('region', { name: 'Dinas Pertanian Daerah' })
  await userEvent.type(within(dinas).getByLabelText('Nama :'), ' M.SI.')
  await userEvent.click(within(dinas).getByRole('button', { name: 'Batal' }))
  expect(within(dinas).getByLabelText('Nama :')).toHaveValue('IR. HARYANTO')
  expect(within(dinas).getByRole('button', { name: 'Simpan' })).toBeDisabled()
})

test('clearing a stored signature deletes it on Simpan', async () => {
  renderPanel()
  const pelaksana = await screen.findByRole('region', { name: 'Pelaksana Pemasangan & Pendistribusian' })
  await userEvent.click(within(pelaksana).getByRole('button', { name: 'Ulangi' }))
  expect(within(pelaksana).queryByAltText('Tanda tangan Pelaksana Pemasangan & Pendistribusian')).not.toBeInTheDocument()
  await userEvent.click(within(pelaksana).getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures/signatories/installer/signature?schedule_id=schedule-1', 'DELETE')).toHaveLength(1))
})

test('shows the stamp upload when Tambah stempel is checked', async () => {
  renderPanel()
  const konsultan = await screen.findByRole('region', { name: 'Konsultan Pengawas' })
  expect(within(konsultan).queryByText('Unggah stempel')).not.toBeInTheDocument()
  await userEvent.click(within(konsultan).getByRole('checkbox', { name: 'Tambah stempel' }))
  expect(within(konsultan).getByText('Unggah stempel')).toBeVisible()
})

test('the per-document switch saves immediately and starts from the global rules', async () => {
  renderPanel()
  await userEvent.click(await screen.findByRole('switch', { name: 'Atur khusus untuk DP3' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toEqual([expect.objectContaining({ override: true, rules: globalRules })]))
})

test('the wet-signature switch saves immediately and keeps the current rules', async () => {
  renderPanel()
  await userEvent.click(await screen.findByRole('switch', { name: 'Tanda tangan basah' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toEqual([expect.objectContaining({ override: false, rules: globalRules, wet_signature: true })]))
})

test('hides the wet-signature switch for documents without a separate signature page', async () => {
  settings = { ...settings, wet_supported: false }
  renderPanel()
  await screen.findByRole('switch', { name: 'Atur khusus untuk DP3' })
  expect(screen.queryByRole('switch', { name: 'Tanda tangan basah' })).not.toBeInTheDocument()
})

test('is read-only without bast.manage', async () => {
  renderPanel(['bast.view'])
  expect(await screen.findByRole('switch', { name: 'Atur khusus untuk DP3' })).toBeDisabled()
  const dinas = screen.getByRole('region', { name: 'Dinas Pertanian Daerah' })
  expect(within(dinas).getByLabelText('Nama :')).toBeDisabled()
  expect(within(dinas).queryByRole('button', { name: 'Simpan' })).not.toBeInTheDocument()
})
