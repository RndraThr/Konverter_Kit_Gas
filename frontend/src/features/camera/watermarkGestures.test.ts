import { describe, expect, it, vi } from 'vitest'
import { createWatermarkGestures, type GestureTarget } from './watermarkGestures'
import type { ElementPlacement } from './watermarkSettings'

function setup(initial: ElementPlacement = null) {
  let placement: ElementPlacement = initial
  // A 400x800 box; the timestamp sits at (200-300, 700-760), the logo at the top.
  const target: GestureTarget = {
    box: () => ({ width: 400, height: 800 }),
    rects: () => ({
      logo: { x: 10, y: 10, width: 380, height: 70 },
      timestamp: { x: 200, y: 700, width: 100, height: 60 },
    }),
    placement: () => placement,
    setPlacement: vi.fn((_key, next: ElementPlacement) => {
      placement = next
    }),
    commit: vi.fn(),
  }
  return { gestures: createWatermarkGestures(target), target, get: () => placement }
}

describe('watermark gestures', () => {
  it('drags the element under the pointer and commits once on release', () => {
    const { gestures, target, get } = setup()
    expect(gestures.down(1, 250, 730)).toBe(true)
    gestures.move(1, 230, 630)
    gestures.move(1, 210, 530)
    expect(target.commit).not.toHaveBeenCalled()
    gestures.up(1)
    expect(target.setPlacement).toHaveBeenLastCalledWith('timestamp', expect.anything())
    expect(get()!.x).toBeCloseTo(210 / 400, 6)
    expect(get()!.y).toBeCloseTo(530 / 800, 6)
    expect(get()!.scale).toBe(1)
    expect(target.commit).toHaveBeenCalledOnce()
  })

  it('ignores pointers that start outside every element', () => {
    const { gestures, target } = setup()
    expect(gestures.down(1, 50, 400)).toBe(false)
    gestures.move(1, 60, 410)
    gestures.up(1)
    expect(target.setPlacement).not.toHaveBeenCalled()
    expect(target.commit).not.toHaveBeenCalled()
  })

  it('keeps the dragged element inside the box', () => {
    const { gestures, get } = setup()
    gestures.down(1, 250, 730)
    gestures.move(1, 1000, 2000)
    gestures.up(1)
    expect(get()!.x).toBeCloseTo((400 - 50) / 400, 6)
    expect(get()!.y).toBeCloseTo((800 - 30) / 800, 6)
  })

  it('pinches with two pointers and clamps the scale to 0.4-3.0', () => {
    const { gestures, target, get } = setup({ x: 0.5, y: 0.5, scale: 1 })
    gestures.down(1, 240, 730)
    gestures.down(2, 260, 730)
    gestures.move(2, 280, 730) // distance 20 -> 40
    expect(get()!.scale).toBeCloseTo(2, 6)
    gestures.move(2, 1000, 730)
    expect(get()!.scale).toBe(3)
    gestures.up(2)
    gestures.up(1)
    expect(target.commit).toHaveBeenCalledOnce()
  })

  it('scales the element under the pointer with the wheel', () => {
    const { gestures, get } = setup()
    expect(gestures.wheel(100, 40, -500)).toBe(true)
    expect(get()!.scale).toBeGreaterThan(1)
    expect(gestures.wheel(100, 40, 1e6)).toBe(true)
    expect(get()!.scale).toBe(0.4)
    expect(gestures.wheel(50, 400, -100)).toBe(false)
  })
})
