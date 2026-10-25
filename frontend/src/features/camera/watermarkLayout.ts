import type { ElementPlacement, WatermarkSettings } from './watermarkSettings'

export type Rect = { x: number; y: number; width: number; height: number }

export type WatermarkLayout = { logo: Rect | null; timestamp: Rect | null; fontSize: number }

export const MIN_PLACEMENT_SCALE = 0.4
export const MAX_PLACEMENT_SCALE = 3.0

/** Line height multiplier of the timestamp text (mobile: height 1.25). */
export const LINE_HEIGHT = 1.25

// Mobile metrics are logical px on a ~390 px wide portrait phone; every size
// here is that value divided by 390, times the photo's shorter side, so the
// preview and the full-size photo get identical proportions.
const REF = 390
/** Short side of the reference photo the timestamp text is wrapped at. */
export const WATERMARK_REF_SIDE = REF
const LOGO_WIDTH = 382 / REF // screen width minus 4px each side
const LOGO_TOP = 14 / REF
const TS_MARGIN_SIDE = 12 / REF
const TS_MARGIN_BOTTOM = 12 / REF
const TS_TEXT_WIDTH = 220 / REF
const TS_BAR_WIDTH = 3 / REF
const TS_BAR_GAP = 8 / REF
const TS_LINE_GAP = 4 / REF
const FONT_FIRST = 13 / REF
const FONT_REST = 12 / REF

/** The region of the source frame visible in an `object-fit: cover` box (centre crop). */
export function coverCrop(sourceW: number, sourceH: number, boxW: number, boxH: number): Rect {
  const boxAspect = boxW / boxH
  if (sourceW / sourceH > boxAspect) {
    const width = sourceH * boxAspect
    return { x: (sourceW - width) / 2, y: 0, width, height: sourceH }
  }
  const height = sourceW / boxAspect
  return { x: 0, y: (sourceH - height) / 2, width: sourceW, height }
}

/** Pixel metrics of the timestamp text for a photo of this size. */
export function timestampMetrics(width: number, height: number, settings: WatermarkSettings) {
  const short = Math.min(width, height)
  return {
    /** Size of the first (date-time) line; the other lines are 12/13 of it. */
    fontSize: short * FONT_FIRST * settings.textScale,
    /** Widest a text line may be before it wraps. */
    textWidth: short * TS_TEXT_WIDTH,
    barWidth: short * TS_BAR_WIDTH,
    barGap: short * TS_BAR_GAP,
    lineGap: short * TS_LINE_GAP,
    restRatio: FONT_REST / FONT_FIRST,
  }
}

function clampScale(scale: number): number {
  return Math.min(MAX_PLACEMENT_SCALE, Math.max(MIN_PLACEMENT_SCALE, scale))
}

/**
 * Applies a placement (centre + size multiplier) to a default-sized box, then
 * keeps the result inside the image. A box that is larger than the image is
 * shrunk (aspect kept) rather than overflowing.
 */
function place(
  width: number,
  height: number,
  imgW: number,
  imgH: number,
  placement: ElementPlacement,
  fallback: (w: number, h: number) => { x: number; y: number },
): Rect {
  let w = width
  let h = height
  if (placement) {
    const s = clampScale(placement.scale)
    w *= s
    h *= s
  }
  const fit = Math.min(1, imgW / w, imgH / h)
  w *= fit
  h *= fit
  const pos = placement ? { x: placement.x * imgW - w / 2, y: placement.y * imgH - h / 2 } : fallback(w, h)
  return {
    x: Math.min(Math.max(0, pos.x), imgW - w),
    y: Math.min(Math.max(0, pos.y), imgH - h),
    width: w,
    height: h,
  }
}

export function layoutWatermark(input: {
  width: number
  height: number
  settings: WatermarkSettings
  logoAspect: number
  timestampLines: number
}): WatermarkLayout {
  const { width, height, settings, logoAspect, timestampLines } = input
  const short = Math.min(width, height)
  const m = timestampMetrics(width, height, settings)

  let logo: Rect | null = null
  if (settings.showLogo && logoAspect > 0) {
    const w = short * LOGO_WIDTH * settings.logoScale
    logo = place(w, w / logoAspect, width, height, settings.logoPlacement, (rw) => ({
      x: (width - rw) / 2,
      y: short * LOGO_TOP,
    }))
  }

  let timestamp: Rect | null = null
  if (settings.showTimestamp && timestampLines > 0) {
    const w = m.textWidth + m.barGap + m.barWidth
    const lineH = m.fontSize * LINE_HEIGHT
    const h = lineH + (timestampLines - 1) * (lineH * m.restRatio) + (timestampLines - 1) * m.lineGap
    timestamp = place(w, h, width, height, settings.timestampPlacement, (rw, rh) => ({
      x: settings.timestampSide === 'right' ? width - short * TS_MARGIN_SIDE - rw : short * TS_MARGIN_SIDE,
      y: height - short * TS_MARGIN_BOTTOM - rh,
    }))
  }

  // The placement scale (and any shrink-to-fit) applies to the text as well.
  const tsK = timestamp ? timestamp.width / (m.textWidth + m.barGap + m.barWidth) : 1
  return { logo, timestamp, fontSize: m.fontSize * tsK }
}

/** Splits a timestamp rect into text column, accent bar and gap (all in px). */
export function timestampParts(rect: Rect, settings: WatermarkSettings) {
  const m = timestampMetrics(1, 1, settings)
  const total = m.textWidth + m.barGap + m.barWidth
  return {
    textWidth: (rect.width * m.textWidth) / total,
    barWidth: (rect.width * m.barWidth) / total,
    barGap: (rect.width * m.barGap) / total,
    lineGap: (rect.width * m.lineGap) / total,
    restRatio: m.restRatio,
  }
}

const pad = (n: number, len = 2) => String(n).padStart(len, '0')

/** Timestamp (backdate replaces only the date), address, and coordinates when present. */
export function watermarkLines(now: Date, settings: WatermarkSettings, address: string, coords: string | null): string[] {
  const date = settings.backdate ?? `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
  const time = `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
  const lines = [`${date} ${time}`, address]
  if (coords !== null) lines.push(coords)
  return lines
}
