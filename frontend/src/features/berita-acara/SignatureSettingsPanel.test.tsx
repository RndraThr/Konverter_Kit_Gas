import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { SignatureSettingsPanel } from './SignatureSettingsPanel'
import type { SignatureLayout, SignatureSettings } from './signatures'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), ApiError: class extends Error {} }))
vi.mock('./pdfRegion', () => ({ renderPdfRegion: vi.fn(async () => undefined), base64ToBytes: () => new Uint8Array() }))

const globalRules = { agriculture_office: { signature: false, stamp: false }, installer: { signature: true, stamp: true }, supervisor: { signature: true, stamp: false } }
const layout: SignatureLayout = {
  document_key: 'dp3', page: 2, page_width: 297, page_height: 210, pdf: '',
  boxes: [
    { role: 'agriculture_office', page: 2, x: 10, y: 40, w: 92, h: 28 },
    { role: 'installer', page: 2, x: 102, y: 40, w: 92, h: 28 },
    { role: 'supervisor', page: 2, x: 194, y: 40, w: 92, h: 28 },
  ],
}
let settings: SignatureSettings

function renderPanel(permissions = ['bast.view', 'bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <SignatureSettingsPanel scheduleID="schedule-1" document="dp3" documentLabel="DP3" date="2026-10-12" />
  </PermissionsProvider></QueryClientProvider>)
}

function writes(path: string, method = 'PUT') {
  return vi.mocked(apiRequest).mock.calls.filter(([url, init]) => url === path && init?.method === method).map(([, init]) => (init?.body ? JSON.parse(String(init.body)) : null))
}

const sidePanel = (name: string) => screen.findByRole('region', { name })
const selectParty = async (name: string) => userEvent.click(await screen.findByRole('tab', { name }))

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
    if (path.startsWith('/api/v1/bast/signatures/layout?')) return { data: layout }
    if (path.startsWith('/api/v1/bast/signatures?')) return { data: settings }
    if (init?.method === 'PUT' || init?.method === 'DELETE') return { data: settings }
    throw new Error(`unexpected ${path}`)
  })
})

test('shows the signature table with one column per party and marks where they print', async () => {
  renderPanel()
  expect(await screen.findByRole('group', { name: 'Kolom Dinas Pertanian Daerah' })).toBeVisible()
  const installer = screen.getByRole('group', { name: 'Kolom Pelaksana Pemasangan & Pendistribusian' })
  expect(within(installer).getByRole('button', { name: 'Tanda tangan Pelaksana Pemasangan & Pendistribusian' })).toBeVisible()
  expect(within(installer).getByRole('button', { name: 'Stempel Pelaksana Pemasangan & Pendistribusian' })).toBeVisible()
  expect(vi.mocked(apiRequest)).toHaveBeenCalledWith('/api/v1/bast/signatures/layout?schedule_id=schedule-1&document=dp3&date=2026-10-12')

  const dinas = await sidePanel('Dinas Pertanian Daerah')
  expect(within(dinas).getByLabelText('Nama')).toHaveValue('IR. HARYANTO')
  expect(within(dinas).getByLabelText('NIP')).toHaveValue('1972')
})

test('clicking a column selects that party in the side panel', async () => {
  renderPanel()
  fireEvent.pointerDown(await screen.findByRole('group', { name: 'Kolom Konsultan Pengawas' }))
  const konsultan = await sidePanel('Konsultan Pengawas')
  expect(within(konsultan).queryByLabelText('NIP')).not.toBeInTheDocument()
})

test('keeps changes local until Simpan, then saves names and print options together', async () => {
  renderPanel()
  const dinas = await sidePanel('Dinas Pertanian Daerah')
  const save = screen.getByRole('button', { name: 'Simpan' })
  expect(save).toBeDisabled()
  const nip = within(dinas).getByLabelText('NIP')
  await userEvent.clear(nip)
  await userEvent.type(nip, '197203151998031004')
  await userEvent.click(within(dinas).getByRole('checkbox', { name: 'Cetak tanda tangan digital' }))
  expect(writes('/api/v1/bast/signatures')).toHaveLength(0)
  expect(screen.getByText('Perubahan belum disimpan: Dinas Pertanian Daerah.')).toBeVisible()

  await userEvent.click(save)
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  expect(writes('/api/v1/bast/signatures/signatories/agriculture_office?schedule_id=schedule-1')).toEqual([{ name: 'IR. HARYANTO', nip: '197203151998031004' }])
  const [rules] = writes('/api/v1/bast/signatures')
  expect(rules).toMatchObject({ schedule_id: 'schedule-1', document: 'dp3', override: false })
  expect(rules.rules.agriculture_office).toMatchObject({ signature: true, stamp: false })
  expect(rules.rules.installer).toMatchObject({ signature: true, stamp: true })
})

test('Batal discards unsaved changes', async () => {
  renderPanel()
  const dinas = await sidePanel('Dinas Pertanian Daerah')
  await userEvent.type(within(dinas).getByLabelText('Nama'), ' M.SI.')
  await userEvent.click(screen.getByRole('button', { name: 'Batal' }))
  expect(within(dinas).getByLabelText('Nama')).toHaveValue('IR. HARYANTO')
  expect(screen.getByRole('button', { name: 'Simpan' })).toBeDisabled()
})

test('signature and stamp positions are saved with the print rule', async () => {
  renderPanel()
  await selectParty('Pelaksana Pemasangan & Pendistribusian')
  const pelaksana = await sidePanel('Pelaksana Pemasangan & Pendistribusian')
  fireEvent.change(within(pelaksana).getByLabelText('Putar stempel Pelaksana Pemasangan & Pendistribusian'), { target: { value: '15' } })
  fireEvent.change(within(pelaksana).getByLabelText('Ukuran stempel Pelaksana Pemasangan & Pendistribusian'), { target: { value: '100' } })
  fireEvent.change(within(pelaksana).getByLabelText('Putar tanda tangan Pelaksana Pemasangan & Pendistribusian'), { target: { value: '-5' } })
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  const installer = writes('/api/v1/bast/signatures')[0].rules.installer
  expect(installer.stamp_placement).toEqual({ x: -0.4, y: 0, size: 1, rotation: 15 })
  expect(installer.signature_placement).toMatchObject({ x: 0, y: 0, rotation: -5 })
})

test('size and rotation can be typed exactly, limited to their range', async () => {
  renderPanel()
  await selectParty('Pelaksana Pemasangan & Pendistribusian')
  const pelaksana = await sidePanel('Pelaksana Pemasangan & Pendistribusian')
  const rotation = within(pelaksana).getByLabelText('Putar stempel Pelaksana Pemasangan & Pendistribusian (derajat)')
  // The default stamp is straight.
  expect(rotation).toHaveValue('0')
  await userEvent.clear(rotation)
  await userEvent.type(rotation, '-12{Enter}')
  const size = within(pelaksana).getByLabelText('Ukuran stempel Pelaksana Pemasangan & Pendistribusian (persen)')
  await userEvent.clear(size)
  await userEvent.type(size, '900')
  fireEvent.blur(size)
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  expect(writes('/api/v1/bast/signatures')[0].rules.installer.stamp_placement).toEqual({ x: -0.4, y: 0, size: 2.5, rotation: -12 })
})

test('arrow keys nudge the selected mark', async () => {
  renderPanel()
  const stamp = await screen.findByRole('button', { name: 'Stempel Pelaksana Pemasangan & Pendistribusian' })
  fireEvent.pointerDown(stamp)
  fireEvent.keyDown(stamp, { key: 'ArrowRight', shiftKey: true })
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  expect(writes('/api/v1/bast/signatures')[0].rules.installer.stamp_placement).toMatchObject({ x: -0.35, y: 0 })
})

test('Posisi bawaan clears a stored stamp position', async () => {
  settings = { ...settings, rules: { ...globalRules, installer: { signature: true, stamp: true, stamp_placement: { x: 0.2, y: 0.1, size: 1.1, rotation: 20 } } } }
  renderPanel()
  await selectParty('Pelaksana Pemasangan & Pendistribusian')
  const pelaksana = await sidePanel('Pelaksana Pemasangan & Pendistribusian')
  await userEvent.click(within(pelaksana).getByRole('button', { name: 'Posisi bawaan stempel' }))
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures')).toHaveLength(1))
  expect(writes('/api/v1/bast/signatures')[0].rules.installer.stamp_placement).toBeNull()
})

test('removing a stored signature deletes it on Simpan', async () => {
  renderPanel()
  await selectParty('Pelaksana Pemasangan & Pendistribusian')
  const pelaksana = await sidePanel('Pelaksana Pemasangan & Pendistribusian')
  await userEvent.click(within(pelaksana).getByRole('button', { name: 'Hapus tanda tangan' }))
  expect(screen.queryByRole('button', { name: 'Tanda tangan Pelaksana Pemasangan & Pendistribusian' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Simpan' }))
  await waitFor(() => expect(writes('/api/v1/bast/signatures/signatories/installer/signature?schedule_id=schedule-1', 'DELETE')).toHaveLength(1))
})

test('shows the stamp upload when Tambah stempel is checked', async () => {
  renderPanel()
  await selectParty('Konsultan Pengawas')
  const konsultan = await sidePanel('Konsultan Pengawas')
  expect(within(konsultan).queryByText('Unggah stempel')).not.toBeInTheDocument()
  await userEvent.click(within(konsultan).getByRole('checkbox', { name: 'Tambah stempel' }))
  expect(within(konsultan).getByText('Unggah stempel')).toBeVisible()
})

test('Gambar tanda tangan turns the selected column into a canvas, without a dialog', async () => {
  renderPanel()
  await selectParty('Pelaksana Pemasangan & Pendistribusian')
  const pelaksana = await sidePanel('Pelaksana Pemasangan & Pendistribusian')
  await userEvent.click(within(pelaksana).getByRole('button', { name: 'Gambar ulang' }))
  const column = screen.getByRole('group', { name: 'Kolom Pelaksana Pemasangan & Pendistribusian' })
  expect(within(column).getByLabelText('Kanvas tanda tangan Pelaksana Pemasangan & Pendistribusian')).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  // Tanda tangan lama disembunyikan selama dicoret ulang; Selesai menunggu coretan.
  expect(within(column).queryByRole('button', { name: 'Tanda tangan Pelaksana Pemasangan & Pendistribusian' })).not.toBeInTheDocument()
  expect(within(column).getByRole('button', { name: 'Selesai' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Perbesar' })).toBeDisabled()

  await userEvent.click(within(column).getByRole('button', { name: 'Batal' }))
  expect(within(column).queryByLabelText('Kanvas tanda tangan Pelaksana Pemasangan & Pendistribusian')).not.toBeInTheDocument()
  expect(within(column).getByRole('button', { name: 'Tanda tangan Pelaksana Pemasangan & Pendistribusian' })).toBeVisible()
})

test('falls back to the side panel when the table cannot be rendered for the date', async () => {
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.startsWith('/api/v1/bast/signatures/layout?')) throw new Error('Belum ada penerima pada jadwal ini')
    return { data: settings }
  })
  renderPanel()
  expect(await screen.findByText(/Tabel tanda tangan belum dapat ditampilkan untuk tanggal ini: Belum ada penerima pada jadwal ini/)).toBeVisible()
  expect(await sidePanel('Dinas Pertanian Daerah')).toBeVisible()
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
  const dinas = await sidePanel('Dinas Pertanian Daerah')
  expect(within(dinas).getByLabelText('Nama')).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'Simpan' })).not.toBeInTheDocument()
  expect(within(dinas).queryByRole('button', { name: 'Gambar tanda tangan' })).not.toBeInTheDocument()
})
