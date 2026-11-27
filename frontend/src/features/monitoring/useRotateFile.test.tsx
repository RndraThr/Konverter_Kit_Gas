import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { RotateButtons, useRotateFile } from './useRotateFile'
import { rotationFit } from './ZoomableImage'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn() }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.mocked(apiRequest).mockResolvedValue({ data: { content_url: '/x?v=1' } })
})
afterEach(() => vi.useRealTimers())

function Harness() {
  const rotation = useRotateFile('schedule-1')
  return <div>
    <span data-testid="angle">{rotation.angle('photo', 'm1')}</span>
    <RotateButtons angle={rotation.angle('photo', 'm1')} saving={rotation.saving('photo', 'm1')} onRotate={(delta) => rotation.rotate('photo', 'm1', delta, false)} />
  </div>
}

test('turns the preview at once and saves the total turn once after the clicks', async () => {
  const client = new QueryClient()
  render(<QueryClientProvider client={client}><Harness /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  expect(screen.getByTestId('angle')).toHaveTextContent('270')
  expect(apiRequest).not.toHaveBeenCalled()
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(apiRequest).toHaveBeenCalledTimes(1)
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/monitoring/rotate', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ schedule_id: 'schedule-1', kind: 'photo', id: 'm1', degrees: 270 }),
  }))
  expect(screen.getByTestId('angle')).toHaveTextContent('0')
})

test('a full turn back is not saved', async () => {
  const client = new QueryClient()
  render(<QueryClientProvider client={client}><Harness /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kiri' }))
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(apiRequest).not.toHaveBeenCalled()
})

test('a quarter turn is scaled to fit the box', () => {
  expect(rotationFit(0, { width: 400, height: 300 }, { w: 400, h: 300 })).toBeUndefined()
  expect(rotationFit(180, { width: 400, height: 300 }, { w: 400, h: 300 })).toBe('rotate(180deg)')
  // 400x300 shown full; turned it is 300 wide, 400 tall: scaled to 300 tall.
  expect(rotationFit(90, { width: 400, height: 300 }, { w: 400, h: 300 })).toBe('rotate(90deg) scale(0.75)')
})
