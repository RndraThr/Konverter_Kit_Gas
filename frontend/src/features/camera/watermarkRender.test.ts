import { describe, expect, it, vi } from 'vitest'
import { layoutWatermark } from './watermarkLayout'
import { drawWatermark, wrapWatermarkLines } from './watermarkRender'
import { DEFAULT_WATERMARK_SETTINGS } from './watermarkSettings'

type MockCtx = CanvasRenderingContext2D

function fontPx(font: string): number {
  return Number.parseFloat(font) || 10
}

/**
 * `measureText` that is deliberately not linear in the font size (like real
 * hinting / glyph rounding): a 130 px font measures relatively wider than a 13 px one.
 */
function mockCtx(opts: { roundRect?: boolean } = {}): MockCtx {
  const ctx: Record<string, unknown> & { font: string } = {
    font: '10px sans-serif',
    drawImage: vi.fn(),
    fillText: vi.fn(),
    fill: vi.fn(),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    measureText: vi.fn((s: string) => {
      const px = fontPx(ctx.font)
      return { width: s.length * px * 0.46 * (1 + px / 400) }
    }),
  }
  if (opts.roundRect !== false) ctx.roundRect = vi.fn()
  return ctx as unknown as MockCtx
}

const settings = DEFAULT_WATERMARK_SETTINGS

function render(width: number, height: number, lines: string[]) {
  const ctx = mockCtx()
  const wrapped = wrapWatermarkLines(ctx, lines, settings)
  const layout = layoutWatermark({ width, height, settings, logoAspect: 5, timestampLines: wrapped.flat().length })
  drawWatermark(ctx, layout, wrapped, {} as CanvasImageSource, settings)
  return { ctx, layout, texts: vi.mocked(ctx.fillText).mock.calls.map((c) => c[0]) }
}

describe('drawWatermark', () => {
  it('draws the logo, the accent bar and every text line', () => {
    const { ctx, texts } = render(600, 800, ['2026-10-23 14:05:09', 'Jl. Mawar', '-6.2, 106.8'])
    expect(ctx.drawImage).toHaveBeenCalledOnce()
    expect(ctx.roundRect).toHaveBeenCalledOnce()
    expect(texts).toEqual(['2026-10-23 14:05:09', 'Jl. Mawar', '-6.2, 106.8'])
    expect(ctx.fillStyle).toBe(settings.textColor)
  })

  it('skips hidden elements', () => {
    const ctx = mockCtx()
    drawWatermark(ctx, { logo: null, timestamp: null, fontSize: 13 }, [['x']], {} as CanvasImageSource, settings)
    expect(ctx.drawImage).not.toHaveBeenCalled()
    expect(ctx.fillText).not.toHaveBeenCalled()
  })

  it('falls back to fillRect for the accent bar when roundRect is unavailable', () => {
    const ctx = mockCtx({ roundRect: false })
    const wrapped = wrapWatermarkLines(ctx, ['a', 'b'], settings)
    const layout = layoutWatermark({ width: 600, height: 800, settings, logoAspect: 5, timestampLines: 2 })
    expect(() => drawWatermark(ctx, layout, wrapped, null, settings)).not.toThrow()
    expect(ctx.fillRect).toHaveBeenCalledOnce()
    expect(ctx.fillText).toHaveBeenCalledTimes(2)
  })

  it('breaks lines identically at preview and full size even with non-linear text metrics', () => {
    // Long enough that at least one line wraps right around the column edge.
    const lines = [
      '2026-10-23 14:05:09',
      'Jl. Raya Mawar Melati Nomor 123, Kelurahan Sukamaju, Kecamatan Sukajadi, Kota Bandung',
      'Lat: -6.20000, Lng: 106.80000',
    ]
    const preview = render(390, 520, lines)
    const full = render(3900, 5200, lines)
    expect(preview.texts.length).toBeGreaterThan(lines.length)
    expect(full.texts).toEqual(preview.texts)
    expect(full.layout.timestamp!.height / preview.layout.timestamp!.height).toBeCloseTo(10, 6)
  })
})

describe('wrapWatermarkLines', () => {
  it('wraps long text to the reference column and breaks over-long words', () => {
    const ctx = mockCtx()
    // Reference column is 220 units at a 13 px first line / 12 px other lines.
    const out = wrapWatermarkLines(ctx, ['short', 'aaaa '.repeat(30).trim(), 'x'.repeat(80)], settings)
    expect(out[0]).toEqual(['short'])
    expect(out[1].length).toBeGreaterThan(1)
    expect(out[1].join(' ')).toBe('aaaa '.repeat(30).trim())
    expect(out[2].length).toBeGreaterThan(1)
    expect(out[2].join('')).toBe('x'.repeat(80))
  })

  it('does not depend on the font already set on the context', () => {
    const a = mockCtx()
    const b = mockCtx()
    b.font = '130px sans-serif'
    const lines = ['Jl. Raya Mawar Melati Nomor 123, Kelurahan Sukamaju, Kota Bandung']
    expect(wrapWatermarkLines(b, lines, settings)).toEqual(wrapWatermarkLines(a, lines, settings))
  })
})
