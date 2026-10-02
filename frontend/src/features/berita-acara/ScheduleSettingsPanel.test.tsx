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
