import { describe, expect, it } from 'vitest'
import { coverCrop, layoutWatermark, watermarkLines } from './watermarkLayout'
import { DEFAULT_WATERMARK_SETTINGS, type WatermarkSettings } from './watermarkSettings'

const base = (over: Partial<WatermarkSettings> = {}): WatermarkSettings => ({ ...DEFAULT_WATERMARK_SETTINGS, ...over })
const input = (width: number, height: number, settings: WatermarkSettings) => ({
  width,
  height,
  settings,
  logoAspect: 4850 / 876,
  timestampLines: 3,
})

describe('coverCrop', () => {
  it('centre-crops a landscape frame into a portrait box', () => {
    const r = coverCrop(1920, 1080, 1080, 1920)
    expect(r.width).toBeCloseTo(607.5, 6)
    expect(r.x).toBeCloseTo(656.25, 6)
    expect(r.y).toBe(0)
    expect(r.height).toBe(1080)
  })

  it('crops height when the box is wider than the frame', () => {
    const r = coverCrop(1000, 1000, 2000, 1000)
    expect(r).toEqual({ x: 0, y: 250, width: 1000, height: 500 })
  })
})

describe('layoutWatermark', () => {
  it('is proportionally identical at preview and full size', () => {
    const a = layoutWatermark(input(400, 300, base()))
    const b = layoutWatermark(input(4000, 3000, base()))
    for (const key of ['logo', 'timestamp'] as const) {
      expect(a[key]).not.toBeNull()
      for (const f of ['x', 'y', 'width', 'height'] as const) {
        expect(b[key]![f] / a[key]![f]).toBeCloseTo(10, 6)
      }
    }
    expect(b.fontSize / a.fontSize).toBeCloseTo(10, 6)
  })

  it('returns null for hidden elements', () => {
    expect(layoutWatermark(input(400, 300, base({ showLogo: false }))).logo).toBeNull()
    expect(layoutWatermark(input(400, 300, base({ showTimestamp: false }))).timestamp).toBeNull()
  })

  it('puts the logo at the top and the timestamp at the bottom on the chosen side', () => {
    const right = layoutWatermark(input(600, 800, base({ timestampSide: 'right' })))
    expect(right.logo!.y).toBeLessThan(40)
    expect(right.logo!.x + right.logo!.width / 2).toBeCloseTo(300, 6)
    expect(600 - (right.timestamp!.x + right.timestamp!.width)).toBeLessThan(30)
    expect(800 - (right.timestamp!.y + right.timestamp!.height)).toBeLessThan(30)

    const left = layoutWatermark(input(600, 800, base({ timestampSide: 'left' })))
    expect(left.timestamp!.x).toBeLessThan(30)
  })

  it('scales the logo and text with their settings', () => {
    const full = layoutWatermark(input(600, 800, base()))
    const half = layoutWatermark(input(600, 800, base({ logoScale: 0.5, textScale: 1.2 })))
    expect(half.logo!.width).toBeCloseTo(full.logo!.width / 2, 6)
    expect(half.fontSize).toBeCloseTo(full.fontSize * 1.2, 6)
  })

  it('centres a placement and multiplies the size by its scale', () => {
    const def = layoutWatermark(input(1200, 900, base()))
    const placed = layoutWatermark(input(1200, 900, base({ timestampPlacement: { x: 0.5, y: 0.5, scale: 2 } })))
    const t = placed.timestamp!
    expect(t.width).toBeCloseTo(def.timestamp!.width * 2, 6)
    expect(t.height).toBeCloseTo(def.timestamp!.height * 2, 6)
    expect(t.x + t.width / 2).toBeCloseTo(600, 6)
    expect(t.y + t.height / 2).toBeCloseTo(450, 6)
  })

  it('clamps the placement scale to 0.4-3.0', () => {
    const def = layoutWatermark(input(8000, 3000, base()))
    const lo = layoutWatermark(input(8000, 3000, base({ timestampPlacement: { x: 0.5, y: 0.5, scale: 0.01 } })))
    const hi = layoutWatermark(input(8000, 3000, base({ timestampPlacement: { x: 0.5, y: 0.5, scale: 50 } })))
    expect(lo.timestamp!.width).toBeCloseTo(def.timestamp!.width * 0.4, 6)
    expect(hi.timestamp!.width).toBeCloseTo(def.timestamp!.width * 3, 6)
  })

  it('clamps a placement inside the image', () => {
    const l = layoutWatermark(
      input(600, 800, base({ logoPlacement: { x: 1, y: 1, scale: 1 }, timestampPlacement: { x: 0, y: 0, scale: 1 } })),
    )
    for (const r of [l.logo!, l.timestamp!]) {
      expect(r.x).toBeGreaterThanOrEqual(0)
      expect(r.y).toBeGreaterThanOrEqual(0)
      expect(r.x + r.width).toBeLessThanOrEqual(600 + 1e-9)
      expect(r.y + r.height).toBeLessThanOrEqual(800 + 1e-9)
    }
    expect(l.logo!.x + l.logo!.width).toBeCloseTo(600, 6)
    expect(l.timestamp!.x).toBe(0)
  })

  it('shrinks an element that is larger than the image instead of overflowing', () => {
    const l = layoutWatermark(input(400, 300, base({ logoPlacement: { x: 0.5, y: 0.5, scale: 3 } })))
    expect(l.logo!.x).toBeGreaterThanOrEqual(0)
    expect(l.logo!.x + l.logo!.width).toBeLessThanOrEqual(400 + 1e-9)
  })
})

describe('watermarkLines', () => {
  const now = new Date('2026-10-23T14:05:09')

  it('formats the local timestamp, address and coordinates', () => {
    expect(watermarkLines(now, base(), 'Jl. Mawar', '-6.2, 106.8')).toEqual([
      '2026-10-23 14:05:09',
      'Jl. Mawar',
      '-6.2, 106.8',
    ])
  })

  it('replaces only the date with the backdate', () => {
    expect(watermarkLines(now, base({ backdate: '2026-10-20' }), 'x', null)[0]).toBe('2026-10-20 14:05:09')
  })

  it('omits the coordinates line when null', () => {
    expect(watermarkLines(now, base(), 'Jl. Mawar', null)).toHaveLength(2)
  })
})
