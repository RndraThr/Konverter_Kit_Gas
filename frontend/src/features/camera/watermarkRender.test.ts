import { describe, expect, it, vi } from 'vitest'
import { layoutWatermark } from './watermarkLayout'
import { drawWatermark, wrapWatermarkLines } from './watermarkRender'
import { DEFAULT_WATERMARK_SETTINGS } from './watermarkSettings'

function mockCtx() {
  return {
    drawImage: vi.fn(),
    fillText: vi.fn(),
    fill: vi.fn(),
    beginPath: vi.fn(),
    roundRect: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    measureText: (s: string) => ({ width: s.length * 6 }),
  } as unknown as CanvasRenderingContext2D & Record<string, ReturnType<typeof vi.fn>>
}

describe('drawWatermark', () => {
  it('draws the logo, the accent bar and every text line', () => {
    const ctx = mockCtx()
    const lines = ['2026-10-23 14:05:09', 'Jl. Mawar', '-6.2, 106.8']
    const layout = layoutWatermark({
      width: 600,
      height: 800,
      settings: DEFAULT_WATERMARK_SETTINGS,
      logoAspect: 5,
      timestampLines: lines.length,
    })
    drawWatermark(ctx, layout, lines, {} as CanvasImageSource, DEFAULT_WATERMARK_SETTINGS)
    expect(ctx.drawImage).toHaveBeenCalledOnce()
    expect(ctx.roundRect).toHaveBeenCalledOnce()
    expect(ctx.fillText).toHaveBeenCalledTimes(3)
    expect(ctx.fillStyle).toBe(DEFAULT_WATERMARK_SETTINGS.textColor)
  })

  it('skips hidden elements', () => {
    const ctx = mockCtx()
    drawWatermark(ctx, { logo: null, timestamp: null, fontSize: 13 }, ['x'], {} as CanvasImageSource, DEFAULT_WATERMARK_SETTINGS)
    expect(ctx.drawImage).not.toHaveBeenCalled()
    expect(ctx.fillText).not.toHaveBeenCalled()
  })
})

describe('wrapWatermarkLines', () => {
  it('wraps long text to the column and breaks over-long words', () => {
    const ctx = mockCtx()
    const out = wrapWatermarkLines(ctx, ['short', 'aaa bbb ccc ddd', 'x'.repeat(30)], 13, 60)
    expect(out[0]).toEqual(['short'])
    expect(out[1]).toEqual(['aaa bbb', 'ccc ddd'])
    expect(out[2].every((p) => p.length <= 10)).toBe(true)
    expect(out[2].join('')).toBe('x'.repeat(30))
  })
})
