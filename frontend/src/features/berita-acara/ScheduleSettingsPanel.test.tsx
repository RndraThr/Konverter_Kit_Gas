import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { PermissionsProvider } from '@/lib/permissions'
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, apiRequest: vi.fn() }
})

test('uppercases BA business settings while preserving NIP digits', async () => {
  vi.mocked(apiRequest).mockResolvedValue({
    data: {
      schedule_id: 'schedule-1', handover_location: '', consultant_company_name: '', agriculture_office_name: '',
      agriculture_office_nip: '', installer_name: '', supervisor_name: '', pertamina_rep_name: '',
    },
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.manage']}><ScheduleSettingsPanel scheduleID="schedule-1" /></PermissionsProvider></QueryClientProvider>)

  const location = await screen.findByLabelText('Lokasi / Titik Serah')
  await userEvent.type(location, 'Gudang Wajo')
  await userEvent.type(screen.getByLabelText('Konsultan Pengawas'), 'Andi Saputra')
  await userEvent.type(screen.getByLabelText('NIP Dinas Pertanian'), '19800101')

  expect(location).toHaveValue('GUDANG WAJO')
  expect(screen.getByLabelText('Konsultan Pengawas')).toHaveValue('ANDI SAPUTRA')
  expect(screen.getByLabelText('NIP Dinas Pertanian')).toHaveValue('19800101')
})

// Regression: the GET response carries read-only fields (e.g. updated_at)
// that the backend's PUT decoder rejects via DisallowUnknownFields. Saving
// must only send the editable input fields, never the raw draft object.
test('save strips read-only fields such as updated_at from the PUT body', async () => {
  vi.mocked(apiRequest).mockResolvedValue({
    data: {
      schedule_id: 'schedule-1', handover_location: 'Gudang', consultant_company_name: 'PT KSM',
      agriculture_office_name: 'Dinas', agriculture_office_nip: '123', installer_name: 'Budi',
      supervisor_name: 'Andi', pertamina_rep_name: 'Rian',
      updated_at: '2026-10-03T04:00:00Z',
    },
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><PermissionsProvider permissions={['bast.manage']}><ScheduleSettingsPanel scheduleID="schedule-1" /></PermissionsProvider></QueryClientProvider>)

  await screen.findByLabelText('Lokasi / Titik Serah')
  await userEvent.click(screen.getByRole('button', { name: /simpan konfigurasi/i }))

  const putCall = vi.mocked(apiRequest).mock.calls.find(([, init]) => init?.method === 'PUT')
  expect(putCall).toBeDefined()
  const body = JSON.parse(putCall![1]!.body as string)
  expect(body).not.toHaveProperty('updated_at')
  expect(Object.keys(body).sort()).toEqual([
    'agriculture_office_name', 'agriculture_office_nip', 'consultant_company_name',
    'handover_location', 'installer_name', 'pertamina_rep_name', 'schedule_id', 'supervisor_name',
  ])
})
