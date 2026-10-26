import { MAX_PLACEMENT_SCALE, MIN_PLACEMENT_SCALE, type Rect } from './watermarkLayout'
import type { ElementPlacement } from './watermarkSettings'

export type ElementKey = 'logo' | 'timestamp'

/** What the gestures act on; all coordinates are in the displayed box (CSS px). */
export type GestureTarget = {
  box: () => { width: number; height: number }
  /** Rects currently drawn for each element (null when hidden). */
  rects: () => Record<ElementKey, Rect | null>
  placement: (key: ElementKey) => ElementPlacement
  /** Live update while the gesture runs (not persisted). */
  setPlacement: (key: ElementKey, placement: ElementPlacement) => void
  /** Persist; called once when a gesture ends. */
  commit: () => void
}

type Point = { x: number; y: number }
type Active = { key: ElementKey; rect: Rect; start: { x: number; y: number; scale: number }; base: Map<number, Point> }

const WHEEL_COMMIT_MS = 300

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))
const clampScale = (s: number) => clamp(s, MIN_PLACEMENT_SCALE, MAX_PLACEMENT_SCALE)
const contains = (r: Rect, p: Point) => p.x >= r.x && p.x <= r.x + r.width && p.y >= r.y && p.y <= r.y + r.height

/**
 * Drag (one pointer), pinch (two pointers) and wheel scaling of the watermark
 * elements. Placements are written live through `setPlacement` and persisted
 * through `commit` only when the gesture ends (last pointer up / wheel idle).
 */
export function createWatermarkGestures(target: GestureTarget) {
  const pointers = new Map<number, Point>()
  let active: Active | null = null
  let wheelTimer: ReturnType<typeof setTimeout> | null = null

  const hit = (p: Point): ElementKey | null => {
    const rects = target.rects()
    for (const key of ['timestamp', 'logo'] as const) {
      const r = rects[key]
      if (r && contains(r, p)) return key
    }
    return null
  }

  /** Current placement, starting from where the element is actually drawn. */
  const startOf = (key: ElementKey, rect: Rect) => {
    const { width, height } = target.box()
    return {
      x: (rect.x + rect.width / 2) / width,
      y: (rect.y + rect.height / 2) / height,
      scale: clampScale(target.placement(key)?.scale ?? 1),
    }
  }

  const apply = (a: Active, x: number, y: number, scale: number) => {
    const { width, height } = target.box()
    const k = scale / a.start.scale
    const halfW = Math.min(0.5, (a.rect.width * k) / 2 / width)
    const halfH = Math.min(0.5, (a.rect.height * k) / 2 / height)
    target.setPlacement(a.key, { x: clamp(x, halfW, 1 - halfW), y: clamp(y, halfH, 1 - halfH), scale })
  }

  /** Restart the gesture from the current state when the pointer count changes. */
  const rebase = () => {
    if (!active) return
    const rect = target.rects()[active.key]
    if (!rect) {
      active = null
      return
    }
    active = { key: active.key, rect, start: startOf(active.key, rect), base: new Map(pointers) }
  }

  const first2 = (m: Map<number, Point>) => [...m.values()].slice(0, 2)
  const mid = ([a, b]: Point[]) => ({ x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 })
  const dist = ([a, b]: Point[]) => Math.hypot(a.x - b.x, a.y - b.y)

  return {
    /** Returns true when the pointer grabbed (or joined a gesture on) an element. */
    down(id: number, x: number, y: number): boolean {
      const p = { x, y }
      if (!active) {
        const key = hit(p)
        if (!key) return false
        active = { key, rect: target.rects()[key]!, start: { x: 0, y: 0, scale: 1 }, base: new Map() }
      }
      pointers.set(id, p)
      rebase()
      return true
    },

    move(id: number, x: number, y: number): void {
      if (!active || !pointers.has(id)) return
      pointers.set(id, { x, y })
      const { width, height } = target.box()
      const a = active
      if (pointers.size === 1) {
        const from = a.base.get(id)
        if (!from) return
        apply(a, a.start.x + (x - from.x) / width, a.start.y + (y - from.y) / height, a.start.scale)
        return
      }
      const before = first2(a.base)
      const now = first2(pointers)
      if (before.length < 2 || dist(before) === 0) return
      const m0 = mid(before)
      const m1 = mid(now)
      const scale = clampScale((a.start.scale * dist(now)) / dist(before))
      apply(a, a.start.x + (m1.x - m0.x) / width, a.start.y + (m1.y - m0.y) / height, scale)
    },

    up(id: number): void {
      if (!pointers.delete(id) || !active) return
      if (pointers.size === 0) {
        active = null
        target.commit()
        return
      }
      rebase()
    },

    /** Wheel over an element scales it; returns false when nothing is under the pointer. */
    wheel(x: number, y: number, deltaY: number): boolean {
      const key = hit({ x, y })
      if (!key) return false
      const rect = target.rects()[key]!
      const start = startOf(key, rect)
      const scale = clampScale(start.scale * Math.exp(-deltaY * 0.0015))
      apply({ key, rect, start, base: new Map() }, start.x, start.y, scale)
      if (wheelTimer) clearTimeout(wheelTimer)
      wheelTimer = setTimeout(() => {
        wheelTimer = null
        target.commit()
      }, WHEEL_COMMIT_MS)
      return true
    },

    /** Persists a pending wheel change right away (e.g. on unmount). */
    flush(): void {
      if (!wheelTimer) return
      clearTimeout(wheelTimer)
      wheelTimer = null
      target.commit()
    },
  }
}

export type WatermarkGestures = ReturnType<typeof createWatermarkGestures>
