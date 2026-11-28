import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { HandoverLocationField, placeKey, readLast, similarLocation } from './HandoverLocationField'
import type { DistributionSlot } from './types'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn() }))

const slot = {
  id: 'slot-2', schedule_id: 'schedule-1', slot_number: 2, distribution_date: '2026-10-20', status: 'linked',
  documentation: [], needs_recompletion: false, created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z',
} as DistributionSlot

function renderField(value: DistributionSlot = slot) {
  const onSaved = vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={client}><HandoverLocationField slot={value} editable onSaved={onSaved} /></QueryClientProvider>)
  return { onSaved }
}

afterEach(() => { vi.clearAllMocks(); localStorage.clear() })

test('the last titik serah of the schedule is one click away', async () => {
  localStorage.setItem('konkit.handover.last.schedule-1', JSON.stringify({ id: 'loc-1', name: 'Balai Desa Tempe' }))
  vi.mocked(apiRequest).mockImplementation((path: string) => path.startsWith('/api/v1/distribution/handover-locations')
    ? Promise.resolve({ data: [{ id: 'loc-1', name: 'Balai Desa Tempe' }] })
    : Promise.resolve({ data: { ...slot, handover_location_id: 'loc-1', handover_location_name: 'Balai Desa Tempe' } }))
  const { onSaved } = renderField()
  expect(screen.getByLabelText('Lokasi penyerahan')).toHaveValue('')
  await userEvent.click(await screen.findByRole('button', { name: 'Pakai: Balai Desa Tempe' }))
  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/distribution/slots/2/handover-location?schedule_id=schedule-1',
    { method: 'PATCH', body: JSON.stringify({ handover_location_id: 'loc-1', handover_location_name: 'Balai Desa Tempe' }) })
})

test('no shortcut when the last place has left the list', async () => {
  localStorage.setItem('konkit.handover.last.schedule-1', JSON.stringify({ id: 'loc-1', name: 'Balai Desa Tempe' }))
  vi.mocked(apiRequest).mockResolvedValue({ data: [{ id: 'loc-2', name: 'Lapangan Sabbangparu' }] })
  renderField()
  await waitFor(() => expect(apiRequest).toHaveBeenCalled())
  await waitFor(() => expect(screen.queryByRole('button', { name: /^Pakai:/ })).not.toBeInTheDocument())
})

test('older copies that kept only the name are still read', () => {
  localStorage.setItem('konkit.handover.last.schedule-9', 'Balai Desa Tempe')
  expect(readLast('schedule-9')).toEqual({ name: 'Balai Desa Tempe' })
})

test('place keys ignore case, spaces and punctuation like the server', () => {
  expect(placeKey(' Balai  Ds. Tempe ')).toBe('BALAI DS TEMPE')
})

test('suggests a close existing location before a new one is added', () => {
  const locations = [{ id: 'a', name: 'Balai Desa Tempe' }, { id: 'b', name: 'Lapangan Sabbangparu' }]
  expect(similarLocation('Balai Dsea Tempe', locations)?.id).toBe('a')
  expect(similarLocation('balai desa tempe', locations)).toBeUndefined()
  expect(similarLocation('Gudang Kecamatan', locations)).toBeUndefined()
})
