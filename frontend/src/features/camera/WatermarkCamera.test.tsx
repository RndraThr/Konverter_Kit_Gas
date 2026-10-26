import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import WatermarkCamera from './WatermarkCamera'
import type { CapturedPhoto } from './types'

const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiRequest }))
vi.mock('../../lib/api', () => ({ apiRequest }))

type Call = { name: string; args: unknown[] }
type MockContext = Record<string, unknown> & { calls: Call[] }

const contexts: MockContext[] = []

function mockContext(): MockContext {
  const calls: Call[] = []
  const ctx: MockContext = { calls, font: '10px sans-serif' }
  const record = (name: string) => (...args: unknown[]) => { calls.push({ name, args }) }
  for (const name of ['drawImage', 'fillText', 'fill', 'fillRect', 'beginPath', 'roundRect', 'save', 'restore', 'clearRect', 'setTransform', 'scale', 'translate']) {
    ctx[name] = vi.fn(record(name))
  }
  ctx.measureText = (s: string) => ({ width: s.length * 6 })
  return ctx
}

function makeStream(facingMode: string) {
  const tracks = [{ stop: vi.fn(), kind: 'video', getSettings: () => ({ facingMode, deviceId: 'cam-1' }) }]
  return { tracks, stream: { getTracks: () => tracks, getVideoTracks: () => tracks } as unknown as MediaStream }
}

const getUserMedia = vi.fn()
const enumerateDevices = vi.fn()
const getCurrentPosition = vi.fn()
let current = makeStream('environment')

function setGeolocation(ok: boolean) {
  getCurrentPosition.mockImplementation((success: PositionCallback, failure: PositionErrorCallback) => {
    if (ok) success({ coords: { latitude: -6.2, longitude: 106.8, accuracy: 5 }, timestamp: Date.now() } as GeolocationPosition)
    else failure({ code: 1, message: 'denied' } as GeolocationPositionError)
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  contexts.length = 0
  localStorage.clear()
  current = makeStream('environment')
  getUserMedia.mockImplementation(() => Promise.resolve(current.stream))
  enumerateDevices.mockResolvedValue([{ kind: 'videoinput', deviceId: 'cam-1', label: 'Belakang' }])
  setGeolocation(true)
  apiRequest.mockResolvedValue({ data: { address: 'Jl. Mawar' } })
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia, enumerateDevices } })
  Object.defineProperty(navigator, 'geolocation', { configurable: true, value: { getCurrentPosition } })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function () {
    const ctx = mockContext()
    contexts.push(ctx)
    return ctx as unknown as CanvasRenderingContext2D
  } as unknown as HTMLCanvasElement['getContext'])
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (cb: BlobCallback, type?: string) {
    cb(new Blob(['x'], { type: type ?? 'image/png' }))
  })
  vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ width: 1080, height: 1920, close: vi.fn() }))
  URL.createObjectURL = vi.fn(() => 'blob:photo')
  URL.revokeObjectURL = vi.fn()
  Object.defineProperty(HTMLMediaElement.prototype, 'readyState', { configurable: true, get: () => 4 })
  Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, get: () => 1920 })
  Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, get: () => 1080 })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  delete (HTMLMediaElement.prototype as unknown as Record<string, unknown>).readyState
  delete (HTMLVideoElement.prototype as unknown as Record<string, unknown>).videoWidth
  delete (HTMLVideoElement.prototype as unknown as Record<string, unknown>).videoHeight
})

function renderCamera(props: { onCapture?: (p: CapturedPhoto) => void; onClose?: () => void } = {}) {
  const onCapture = props.onCapture ?? vi.fn()
  const onClose = props.onClose ?? vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <WatermarkCamera title="Foto Dokumentasi" onCapture={onCapture} onClose={onClose} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { ...view, onCapture, onClose }
}

async function ready() {
  await waitFor(() => expect(screen.getByRole('button', { name: 'Ambil foto' })).toBeEnabled())
}

test('opens a full-screen dialog and requests the rear camera at 1080p', async () => {
  renderCamera()
  expect(screen.getByRole('dialog', { name: 'Foto Dokumentasi' })).toBeInTheDocument()
  await ready()
  expect(getUserMedia).toHaveBeenCalledWith({
    video: { facingMode: { ideal: 'environment' }, width: { ideal: 1920 }, height: { ideal: 1080 } },
  })
  expect(screen.getByRole('link', { name: 'Pengaturan watermark' })).toHaveAttribute('href', '/pengaturan-watermark')
})

test('shutter shows a review; "Gunakan Foto" hands over a JPEG with the fix and capture time', async () => {
  const { onCapture } = renderCamera()
  await ready()

  fireEvent.click(screen.getByRole('button', { name: 'Ambil foto' }))
  expect(await screen.findByRole('button', { name: 'Gunakan Foto' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Ambil Ulang' })).toBeVisible()
  expect(onCapture).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Gunakan Foto' }))
  expect(onCapture).toHaveBeenCalledOnce()
  const photo = vi.mocked(onCapture).mock.calls[0][0]
  expect(photo.file).toBeInstanceOf(File)
  expect(photo.file.type).toBe('image/jpeg')
  expect(photo.latitude).toBe(-6.2)
  expect(photo.longitude).toBe(106.8)
  expect(Math.abs(photo.capturedAt.getTime() - Date.now())).toBeLessThan(1000)
})

test('still captures without coordinates when geolocation is denied', async () => {
  setGeolocation(false)
  const { onCapture } = renderCamera()
  await ready()

  fireEvent.click(screen.getByRole('button', { name: 'Ambil foto' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Gunakan Foto' }))
  expect(onCapture).toHaveBeenCalledOnce()
  expect(vi.mocked(onCapture).mock.calls[0][0]).toMatchObject({ latitude: null, longitude: null })
})

test('mirrors a front camera in the preview and in the captured frame, but not the watermark', async () => {
  current = makeStream('user')
  renderCamera()
  await ready()
  const video = screen.getByRole('dialog').querySelector('video')!
  expect(video).toHaveStyle({ transform: 'scaleX(-1)' })

  fireEvent.click(screen.getByRole('button', { name: 'Ambil foto' }))
  await screen.findByRole('button', { name: 'Gunakan Foto' })

  const capture = contexts.find((ctx) => ctx.calls.some((c) => c.name === 'drawImage' && c.args[0] === video))!
  expect(capture).toBeDefined()
  const names = capture.calls.map((c) => c.name)
  const mirror = capture.calls.findIndex((c) => c.name === 'scale' && c.args[0] === -1 && c.args[1] === 1)
  const frame = capture.calls.findIndex((c) => c.name === 'drawImage' && c.args[0] === video)
  const reset = capture.calls.findIndex((c, i) => i > frame && c.name === 'setTransform' && c.args.join() === '1,0,0,1,0,0')
  const firstText = names.indexOf('fillText')
  expect(mirror).toBeGreaterThanOrEqual(0)
  expect(mirror).toBeLessThan(frame)
  expect(reset).toBeGreaterThan(frame)
  expect(firstText).toBeGreaterThan(reset)
})

test('does not mirror a rear camera', async () => {
  renderCamera()
  await ready()
  expect(screen.getByRole('dialog').querySelector('video')).not.toHaveStyle({ transform: 'scaleX(-1)' })
})

test('"Ambil Ulang" returns to the live view without calling onCapture', async () => {
  const { onCapture } = renderCamera()
  await ready()

  fireEvent.click(screen.getByRole('button', { name: 'Ambil foto' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Ambil Ulang' }))

  expect(await screen.findByRole('button', { name: 'Ambil foto' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Gunakan Foto' })).not.toBeInTheDocument()
  expect(onCapture).not.toHaveBeenCalled()
})

test('shows the Indonesian permission message and retries', async () => {
  getUserMedia.mockRejectedValueOnce(new DOMException('denied', 'NotAllowedError'))
  renderCamera()

  expect(await screen.findByRole('alert')).toHaveTextContent('Izin kamera ditolak')
  fireEvent.click(screen.getByRole('button', { name: 'Coba lagi' }))
  await ready()
  expect(getUserMedia).toHaveBeenCalledTimes(2)
})

test('closing stops every track', async () => {
  const { onClose, unmount } = renderCamera()
  await ready()

  fireEvent.click(screen.getByRole('button', { name: 'Tutup kamera' }))
  expect(onClose).toHaveBeenCalledOnce()
  for (const track of current.tracks) expect(track.stop).toHaveBeenCalled()
  unmount()
})

test('stops the tracks when unmounted', async () => {
  const { unmount } = renderCamera()
  await ready()
  unmount()
  for (const track of current.tracks) expect(track.stop).toHaveBeenCalled()
})

test('offers a camera switch only when several cameras exist', async () => {
  enumerateDevices.mockResolvedValue([
    { kind: 'videoinput', deviceId: 'cam-1', label: 'Belakang' },
    { kind: 'videoinput', deviceId: 'cam-2', label: 'Depan' },
  ])
  renderCamera()
  fireEvent.click(await screen.findByRole('button', { name: 'Ganti kamera' }))
  await waitFor(() => expect(getUserMedia).toHaveBeenLastCalledWith(expect.objectContaining({
    video: expect.objectContaining({ deviceId: { exact: 'cam-2' } }),
  })))
})

test('Logo and Waktu toggles are persisted', async () => {
  renderCamera()
  await ready()
  const waktu = screen.getByRole('button', { name: 'Waktu' })
  expect(waktu).toHaveAttribute('aria-pressed', 'true')
  fireEvent.click(waktu)
  expect(waktu).toHaveAttribute('aria-pressed', 'false')
  expect(JSON.parse(localStorage.getItem('konkit.watermark.v1')!).showTimestamp).toBe(false)
})
