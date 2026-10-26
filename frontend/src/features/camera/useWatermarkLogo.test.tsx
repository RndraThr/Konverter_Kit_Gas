import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { useWatermarkLogo } from './useWatermarkLogo'

const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiRequest }))

const failing = new Set<string>()

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
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage: vi.fn() } as unknown as CanvasRenderingContext2D)
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,BANNER')
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
