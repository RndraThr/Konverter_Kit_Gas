import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { useWatermarkLogo } from './useWatermarkLogo'

const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiRequest }))

const failing = new Set<string>()
const drawImage = vi.fn()
const logoUrl = (id: string) => `/api/v1/bast/branding/logos/${id}/content?program_id=p1`

class FakeImage {
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  naturalWidth = 100
  naturalHeight = 20
  private value = ''
  get src() { return this.value }
  set src(v: string) {
    this.value = v
    queueMicrotask(() => (failing.has(v) ? this.onerror?.() : this.onload?.()))
  }
}

beforeEach(() => {
  failing.clear()
  apiRequest.mockReset()
  vi.stubGlobal('Image', FakeImage)
  drawImage.mockClear()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage } as unknown as CanvasRenderingContext2D)
  // The built-in banner is 1940 px wide; any other width is a composed strip of chosen logos.
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockImplementation(function (this: HTMLCanvasElement) {
    return this.width === 1940 ? 'data:image/png;base64,BANNER' : 'data:image/png;base64,STRIP'
  })
  apiRequest.mockResolvedValue({
    data: [
      { id: 'b', sort_order: 1, is_visible: true },
      { id: 'a', sort_order: 0, is_visible: false },
      { id: 'c', sort_order: 2, is_visible: true },
    ],
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

test('prefers the custom logo', async () => {
  const { result } = renderHook(() => useWatermarkLogo('p1', 'data:image/png;base64,CUSTOM'), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe('data:image/png;base64,CUSTOM'))
  expect(apiRequest).not.toHaveBeenCalled()
})

test('uses the first visible tender logo of the program', async () => {
  const { result } = renderHook(() => useWatermarkLogo('p1', null), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe('/api/v1/bast/branding/logos/b/content?program_id=p1'))
  expect(apiRequest).toHaveBeenCalledWith('/api/v1/bast/branding?program_id=p1')
})

test('falls back to the built-in banner when the tender logo fails to load', async () => {
  failing.add('/api/v1/bast/branding/logos/b/content?program_id=p1')
  const { result } = renderHook(() => useWatermarkLogo('p1', null), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe('data:image/png;base64,BANNER'))
})

test('uses the built-in banner without a program', async () => {
  const { result } = renderHook(() => useWatermarkLogo(undefined, null), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe('data:image/png;base64,BANNER'))
  expect(apiRequest).not.toHaveBeenCalled()
})

test('composes several chosen logos side by side in the chosen order', async () => {
  const { result } = renderHook(() => useWatermarkLogo('p1', null, ['c', 'bawaan', 'b']), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe('data:image/png;base64,STRIP'))
  const drawn = drawImage.mock.calls.map(([img]) => (img as FakeImage).src).filter((src) => !src.startsWith('/src/') && !src.includes('logo-'))
  expect(drawn.slice(-3)).toEqual([logoUrl('c'), 'data:image/png;base64,BANNER', logoUrl('b')])
})

test('one chosen logo is used as-is, including a hidden one', async () => {
  const { result } = renderHook(() => useWatermarkLogo('p1', null, ['a']), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe(logoUrl('a')))
})

test('uses the stored choice for the program when none is passed', async () => {
  localStorage.setItem('konkit.watermark.v1', JSON.stringify({ tenderLogos: { p1: ['c'] } }))
  const { result } = renderHook(() => useWatermarkLogo('p1', null), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe(logoUrl('c')))
  localStorage.clear()
})

test('falls back to the built-in banner when nothing chosen is available', async () => {
  const empty = renderHook(() => useWatermarkLogo('p1', null, []), { wrapper })
  await waitFor(() => expect(empty.result.current?.src).toBe('data:image/png;base64,BANNER'))
  const gone = renderHook(() => useWatermarkLogo('p1', null, ['deleted']), { wrapper })
  await waitFor(() => expect(gone.result.current?.src).toBe('data:image/png;base64,BANNER'))
})

test('skips a chosen logo that fails to load and keeps the rest', async () => {
  failing.add(logoUrl('b'))
  const { result } = renderHook(() => useWatermarkLogo('p1', null, ['b', 'c']), { wrapper })
  await waitFor(() => expect(result.current?.src).toBe(logoUrl('c')))
})
