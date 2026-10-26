import { apiRequest } from '../../lib/api'

export type Fix = { latitude: number; longitude: number; accuracy: number; at: number }

type Point = { latitude: number; longitude: number }

const EARTH_RADIUS_M = 6371008.8
const MOVE_THRESHOLD_M = 15
const ADDRESS_TIMEOUT_MS = 4000
const ADDRESS_FALLBACK = 'Alamat tidak tersedia'

type AddressFetcher = (lat: number, lng: number, signal?: AbortSignal) => Promise<string>

/** One GPS fix; resolves null (never rejects) when unsupported, denied or timed out. */
export function getFix(): Promise<Fix | null> {
  return new Promise((resolve) => {
    if (typeof navigator === 'undefined' || !navigator.geolocation) {
      resolve(null)
      return
    }
    try {
      navigator.geolocation.getCurrentPosition(
        (pos) =>
          resolve({
            latitude: pos.coords.latitude,
            longitude: pos.coords.longitude,
            accuracy: pos.coords.accuracy,
            at: pos.timestamp,
          }),
        () => resolve(null),
        { enableHighAccuracy: true, timeout: 8000, maximumAge: 300000 },
      )
    } catch {
      resolve(null)
    }
  })
}

const toRad = (deg: number) => (deg * Math.PI) / 180

/** Great-circle (haversine) distance in meters. */
export function distanceMeters(a: Point, b: Point): number {
  const dLat = toRad(b.latitude - a.latitude)
  const dLng = toRad(b.longitude - a.longitude)
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(toRad(a.latitude)) * Math.cos(toRad(b.latitude)) * Math.sin(dLng / 2) ** 2
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(h)))
}

export function formatCoordinates(fix: Fix): string {
  return `Lat: ${fix.latitude.toFixed(5)}, Lng: ${fix.longitude.toFixed(5)}`
}

const defaultFetcher: AddressFetcher = async (lat, lng, signal) => {
  const res = await apiRequest<{ data: { address: string } }>(`/api/v1/geo/reverse?lat=${lat}&lng=${lng}`, { signal })
  return res.data.address
}

/** Tracks the reverse-geocoded address, refetching only after moving more than 15 m. */
export class AddressTracker {
  private address = ''
  private lastRequested: Point | null = null
  private seq = 0
  private listeners = new Set<(address: string) => void>()

  constructor(private readonly fetchAddress: AddressFetcher = defaultFetcher) {}

  update(fix: Fix): void {
    if (this.lastRequested && distanceMeters(this.lastRequested, fix) <= MOVE_THRESHOLD_M) return
    this.lastRequested = { latitude: fix.latitude, longitude: fix.longitude }
    void this.request(fix.latitude, fix.longitude, ++this.seq)
  }

  current(): string {
    return this.address || ADDRESS_FALLBACK
  }

  onChange(cb: (address: string) => void): () => void {
    this.listeners.add(cb)
    return () => {
      this.listeners.delete(cb)
    }
  }

  private async request(lat: number, lng: number, id: number): Promise<void> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), ADDRESS_TIMEOUT_MS)
    let next = ''
    try {
      next = await this.fetchAddress(lat, lng, controller.signal)
    } catch {
      return
    } finally {
      clearTimeout(timer)
    }
    if (id !== this.seq || controller.signal.aborted || !next || next === this.address) return
    this.address = next
    this.listeners.forEach((cb) => cb(next))
  }
}
