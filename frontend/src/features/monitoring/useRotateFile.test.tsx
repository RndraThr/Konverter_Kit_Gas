import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { apiRequest } from '@/lib/api'
import { RotateButtons, useRotateFile } from './useRotateFile'
import { rotationFit, ZoomableImage } from './ZoomableImage'

vi.mock('@/lib/api', () => ({ apiRequest: vi.fn() }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.mocked(apiRequest).mockResolvedValue({ data: { content_url: '/x?v=1' } })
})
afterEach(() => vi.useRealTimers())

function Harness({ url = '/x', open }: { url?: string; open?: string }) {
  const rotation = useRotateFile('schedule-1')
  const focus = rotation.focus
  useEffect(() => { if (open) focus('photo', open) }, [focus, open])
  return <div>
    <span data-testid="angle">{rotation.angle('photo', 'm1')}</span>
    <span data-testid="src">{rotation.src('photo', 'm1', url)}</span>
    <RotateButtons angle={rotation.angle('photo', 'm1')} saving={rotation.saving('photo', 'm1')} onRotate={(delta) => rotation.rotate('photo', 'm1', delta, 'none')} />
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

test('the turned file keeps its URL until the turn is saved', async () => {
  const client = new QueryClient()
  const { rerender } = render(<QueryClientProvider client={client}><Harness /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  // A live update brings the newer file while the turn is still shown.
  rerender(<QueryClientProvider client={client}><Harness url="/x?v=1" /></QueryClientProvider>)
  expect(screen.getByTestId('src')).toHaveTextContent(/^\/x$/)
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(screen.getByTestId('angle')).toHaveTextContent('0')
  expect(screen.getByTestId('src')).toHaveTextContent('/x?v=1')
})

test('a new version of the photo replaces the old one only once loaded', () => {
  const { container, rerender } = render(<ZoomableImage src="/m?v=1" alt="Foto" rotation={90} />)
  rerender(<ZoomableImage src="/m?v=2" alt="Foto" rotation={0} />)
  const shown = screen.getByAltText('Foto')
  expect(shown).toHaveAttribute('src', '/m?v=1')
  expect(shown.style.transform).toContain('rotate(90deg)')
  const loading = container.querySelector('img[aria-hidden="true"]')!
  expect(loading).toHaveAttribute('src', '/m?v=2')
  fireEvent.load(loading)
  expect(screen.getByAltText('Foto')).toHaveAttribute('src', '/m?v=2')
  expect(screen.getByAltText('Foto').style.transform).toBe('')
})

test('another photo shows at once', () => {
  const { rerender } = render(<ZoomableImage src="/a" alt="Foto" />)
  rerender(<ZoomableImage src="/b" alt="Foto" />)
  expect(screen.getByAltText('Foto')).toHaveAttribute('src', '/b')
})

test('the open file keeps its turned preview after saving; leaving it shows the rotated file', async () => {
  const client = new QueryClient()
  const tree = (url: string, open: string) => <QueryClientProvider client={client}><Harness url={url} open={open} /></QueryClientProvider>
  const { rerender } = render(tree('/x', 'm1'))
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(apiRequest).toHaveBeenCalledTimes(1)
  // Saved, the data has the new file, but the open photo does not change.
  rerender(tree('/x?v=1', 'm1'))
  expect(screen.getByTestId('angle')).toHaveTextContent('90')
  expect(screen.getByTestId('src')).toHaveTextContent(/^\/x$/)
  // Another turn on the open photo saves only the new quarter.
  fireEvent.click(screen.getByRole('button', { name: 'Putar ke kanan' }))
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(apiRequest).toHaveBeenLastCalledWith('/api/v1/monitoring/rotate', expect.objectContaining({
    body: JSON.stringify({ schedule_id: 'schedule-1', kind: 'photo', id: 'm1', degrees: 90 }),
  }))
  // Another photo opened: this one shows its rotated file, unturned.
  await act(async () => { rerender(tree('/x?v=1', 'm2')) })
  expect(screen.getByTestId('angle')).toHaveTextContent('0')
  expect(screen.getByTestId('src')).toHaveTextContent('/x?v=1')
})
