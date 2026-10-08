import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { ZonePOPanel } from './ZonePOPanel'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn(), apiBlobRequest: vi.fn() }))

const zones = [{ zone_id: 'z1', zone_name: 'ZONA 1', rows: [
  { kind: 'machine', code: 'shark', name: 'Mesin Pompa', brand: 'SHARK', po_number: 'PO-1' },
  { kind: 'component', code: 'oil', name: 'Oli', brand: 'Pertamina Enduro', po_number: '' },
] }]

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiRequest).mockImplementation(async (path) => {
    if (path.startsWith('/api/v1/bast/items/zone-po')) return { data: zones }
    throw new Error(`unexpected ${path}`)
  })
})

test('fills a missing PO for one zone and saves every brand row of that zone', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={client}><ZonePOPanel programID="p1" canManage /></QueryClientProvider>)
  expect(await screen.findByText('1/2')).toBeVisible()
  await userEvent.type(screen.getByLabelText('No. PO Oli Pertamina Enduro ZONA 1'), 'PO-OLI')
  await userEvent.click(screen.getByRole('button', { name: 'Simpan (1 zona)' }))
  await waitFor(() => expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/items/zone-po', expect.objectContaining({ method: 'PUT' })))
  const body = JSON.parse(vi.mocked(apiRequest).mock.calls.find(([, init]) => init?.method === 'PUT')![1]!.body as string)
  expect(body).toEqual({ program_id: 'p1', zone_id: 'z1', po_numbers: [
    { kind: 'machine', code: 'shark', po_number: 'PO-1' },
    { kind: 'component', code: 'oil', po_number: 'PO-OLI' },
  ] })
})
