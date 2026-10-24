import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AddressTracker, distanceMeters, formatCoordinates, getFix, type Fix } from './location'

const FALLBACK = 'Alamat tidak tersedia'

function fix(latitude: number, longitude: number): Fix {
  return { latitude, longitude, accuracy: 5, at: 0 }
}

// ~1 m of latitude in degrees
const M = 1 / 111195

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('getFix', () => {
  it('resolves null when geolocation is unsupported', async () => {
    vi.stubGlobal('navigator', {})
    expect(await getFix()).toBeNull()
  })

  it('resolves null on PERMISSION_DENIED', async () => {
    const getCurrentPosition = vi.fn((_ok: PositionCallback, err: PositionErrorCallback) =>
      err({ code: 1, message: 'denied' } as GeolocationPositionError),
    )
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } })
    expect(await getFix()).toBeNull()
  })

  it('requests a high accuracy fix and maps the position', async () => {
    const getCurrentPosition = vi.fn((ok: PositionCallback, _err?: PositionErrorCallback, _opts?: PositionOptions) =>
      ok({ coords: { latitude: -4.1, longitude: 120.2, accuracy: 12 }, timestamp: 99 } as GeolocationPosition),
    )
    vi.stubGlobal('navigator', { geolocation: { getCurrentPosition } })
    expect(await getFix()).toEqual({ latitude: -4.1, longitude: 120.2, accuracy: 12, at: 99 })
    expect(getCurrentPosition.mock.calls[0][2]).toEqual({ enableHighAccuracy: true, timeout: 8000, maximumAge: 300000 })
  })

  it('resolves null when getCurrentPosition throws', async () => {
    vi.stubGlobal('navigator', {
      geolocation: {
        getCurrentPosition: () => {
          throw new Error('boom')
        },
      },
    })
    expect(await getFix()).toBeNull()
  })
})

describe('distanceMeters', () => {
  it('is about 111 km per degree of latitude', () => {
    const d = distanceMeters({ latitude: 0, longitude: 0 }, { latitude: 1, longitude: 0 })
    expect(Math.abs(d - 111195) / 111195).toBeLessThan(0.005)
  })

  it('is zero for identical points', () => {
    expect(distanceMeters({ latitude: -4, longitude: 120 }, { latitude: -4, longitude: 120 })).toBe(0)
  })
})

describe('formatCoordinates', () => {
  it('prints five decimals', () => {
    expect(formatCoordinates(fix(-4.123449, 120.12345))).toBe('Lat: -4.12345, Lng: 120.12345')
  })
})

describe('AddressTracker', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('returns the fallback before any success', () => {
    expect(new AddressTracker(vi.fn()).current()).toBe(FALLBACK)
  })

  it('fetches once for two fixes 5 m apart and again for one 20 m away', async () => {
    const fetchAddress = vi.fn().mockResolvedValue('Jl. A')
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    tracker.update(fix(-4 + 5 * M, 120))
    expect(fetchAddress).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(0)
    expect(tracker.current()).toBe('Jl. A')
    tracker.update(fix(-4 + 20 * M, 120))
    expect(fetchAddress).toHaveBeenCalledTimes(2)
  })

  it('measures distance from the last requested position', () => {
    const fetchAddress = vi.fn().mockResolvedValue('x')
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    tracker.update(fix(-4 + 10 * M, 120))
    tracker.update(fix(-4 + 20 * M, 120))
    expect(fetchAddress).toHaveBeenCalledTimes(2)
  })

  it('keeps the previous address when the fetcher resolves empty', async () => {
    const fetchAddress = vi.fn().mockResolvedValueOnce('Jl. A').mockResolvedValueOnce('')
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    await vi.advanceTimersByTimeAsync(0)
    tracker.update(fix(-4 + 50 * M, 120))
    await vi.advanceTimersByTimeAsync(0)
    expect(tracker.current()).toBe('Jl. A')
  })

  it('keeps the previous address when the fetcher rejects', async () => {
    const fetchAddress = vi.fn().mockResolvedValueOnce('Jl. A').mockRejectedValueOnce(new Error('x'))
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    await vi.advanceTimersByTimeAsync(0)
    tracker.update(fix(-4 + 50 * M, 120))
    await vi.advanceTimersByTimeAsync(0)
    expect(tracker.current()).toBe('Jl. A')
  })

  it('aborts after 4000 ms and keeps the previous address', async () => {
    let signal: AbortSignal | undefined
    let resolveLate: (v: string) => void = () => {}
    const fetchAddress = vi
      .fn()
      .mockResolvedValueOnce('Jl. A')
      .mockImplementationOnce((_a: number, _b: number, s?: AbortSignal) => {
        signal = s
        return new Promise<string>((r) => {
          resolveLate = r
        })
      })
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    await vi.advanceTimersByTimeAsync(0)
    tracker.update(fix(-4 + 50 * M, 120))
    await vi.advanceTimersByTimeAsync(3999)
    expect(signal?.aborted).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect(signal?.aborted).toBe(true)
    resolveLate('Jl. Late')
    await vi.advanceTimersByTimeAsync(0)
    expect(tracker.current()).toBe('Jl. A')
  })

  it('ignores a stale response from a superseded request', async () => {
    const resolvers: Array<(v: string) => void> = []
    const fetchAddress = vi.fn(() => new Promise<string>((r) => resolvers.push(r)))
    const tracker = new AddressTracker(fetchAddress)
    tracker.update(fix(-4, 120))
    tracker.update(fix(-4 + 50 * M, 120))
    resolvers[1]('Baru')
    await vi.advanceTimersByTimeAsync(0)
    resolvers[0]('Lama')
    await vi.advanceTimersByTimeAsync(0)
    expect(tracker.current()).toBe('Baru')
  })

  it('notifies only on actual change and supports unsubscribe', async () => {
    const fetchAddress = vi.fn().mockResolvedValueOnce('Jl. A').mockResolvedValueOnce('Jl. A').mockResolvedValueOnce('Jl. B')
    const tracker = new AddressTracker(fetchAddress)
    const cb = vi.fn()
    const off = tracker.onChange(cb)
    tracker.update(fix(-4, 120))
    await vi.advanceTimersByTimeAsync(0)
    tracker.update(fix(-4 + 50 * M, 120))
    await vi.advanceTimersByTimeAsync(0)
    expect(cb.mock.calls).toEqual([['Jl. A']])
    off()
    tracker.update(fix(-4 + 100 * M, 120))
    await vi.advanceTimersByTimeAsync(0)
    expect(cb).toHaveBeenCalledTimes(1)
    expect(tracker.current()).toBe('Jl. B')
  })
})
