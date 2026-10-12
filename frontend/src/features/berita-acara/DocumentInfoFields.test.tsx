import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { DocumentInfoFields } from './DocumentInfoFields'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, apiRequest: vi.fn() }
})

const current = {
  schedule_id: 'schedule-1', handover_location: '', consultant_company_name: '', agriculture_office_name: 'DINAS',
  agriculture_office_nip: '123', installer_name: 'BUDI', supervisor_name: 'ANDI', pertamina_rep_name: '',
  consultant_company_default: 'PT Kian Santang Muliatama Tbk', updated_at: '2026-10-03T04:00:00Z',
}

function renderFields(permissions = ['bast.manage']) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><PermissionsProvider permissions={permissions}>
    <DocumentInfoFields scheduleID="schedule-1" fields={[{ key: 'handover_location', label: 'Lokasi / Titik Serah' }, { key: 'consultant_company_name', label: 'Konsultan Distribusi' }]} />
  </PermissionsProvider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (_path, init) => (init?.method === 'PUT' ? { data: current } : { data: current }))
})

test('shows the organization name as the automatic consultant when the schedule leaves it empty', async () => {
  renderFields()
  await screen.findByText('Kosong = Nama Organisasi.')
  const consultant = screen.getByLabelText('Konsultan Distribusi')
  expect(consultant).toHaveValue('')
  expect(consultant).toHaveAttribute('placeholder', 'PT Kian Santang Muliatama Tbk')
})

test('saves an edited field automatically when leaving it, uppercased, without read-only fields', async () => {
  renderFields()
  await screen.findByText('Kosong = Nama Organisasi.')
  await userEvent.type(screen.getByLabelText('Lokasi / Titik Serah'), 'Balai desa tempe')
  expect(vi.mocked(apiRequest).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
  await userEvent.tab()
  await waitFor(() => expect(vi.mocked(apiRequest).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true))
  const [, init] = vi.mocked(apiRequest).mock.calls.find(([, call]) => call?.method === 'PUT')!
  const body = JSON.parse(String(init?.body))
  expect(body).toMatchObject({ schedule_id: 'schedule-1', handover_location: 'BALAI DESA TEMPE', consultant_company_name: '', installer_name: 'BUDI' })
  expect(body).not.toHaveProperty('updated_at')
})

test('is read-only without bast.manage', async () => {
  renderFields(['bast.view'])
  expect(await screen.findByLabelText('Lokasi / Titik Serah')).toBeDisabled()
})
