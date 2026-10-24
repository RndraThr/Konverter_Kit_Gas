import { LINE_HEIGHT, timestampParts, type WatermarkLayout } from './watermarkLayout'
import type { WatermarkSettings } from './watermarkSettings'

const FONT_FAMILY = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'

function fontFor(size: number): string {
  return `${size}px ${FONT_FAMILY}`
}

function wrapOne(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string[] {
  const out: string[] = []
  let line = ''
  const flush = () => {
    if (line) out.push(line)
    line = ''
  }
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const candidate = line ? `${line} ${word}` : word
    if (ctx.measureText(candidate).width <= maxWidth) {
      line = candidate
      continue
    }
    flush()
    // A single word wider than the column is broken by character.
    let chunk = ''
    for (const ch of word) {
      if (chunk && ctx.measureText(chunk + ch).width > maxWidth) {
        out.push(chunk)
        chunk = ''
      }
      chunk += ch
    }
    line = chunk
  }
  flush()
  return out.length ? out : ['']
}

/**
 * Word-wraps each source line to the timestamp column. `fontSize` and
 * `maxWidth` must be consistent (same photo size), e.g. both from
 * `timestampMetrics`; the result is scale-invariant, so its total length is the
 * `timestampLines` to hand to `layoutWatermark`.
 */
export function wrapWatermarkLines(
  ctx: CanvasRenderingContext2D,
  lines: string[],
  fontSize: number,
  maxWidth: number,
  restRatio = 12 / 13,
): string[][] {
  return lines.map((text, i) => {
    ctx.font = fontFor(i === 0 ? fontSize : fontSize * restRatio)
    return wrapOne(ctx, text, maxWidth)
  })
}

/** Draws the logo banner and the timestamp block (accent bar + shadowed text). */
export function drawWatermark(
  ctx: CanvasRenderingContext2D,
  layout: WatermarkLayout,
  lines: string[],
  logo: CanvasImageSource | null,
  settings: WatermarkSettings,
): void {
  if (layout.logo && logo) {
    ctx.drawImage(logo, layout.logo.x, layout.logo.y, layout.logo.width, layout.logo.height)
  }

  const rect = layout.timestamp
  if (!rect || lines.length === 0) return
  const p = timestampParts(rect, settings)
  const right = settings.timestampSide === 'right'
  const barX = right ? rect.x + rect.width - p.barWidth : rect.x
  const textEdge = right ? rect.x + rect.width - p.barWidth - p.barGap : rect.x + p.barWidth + p.barGap
  const unit = layout.fontSize / 13

  ctx.save()
  ctx.fillStyle = settings.accentColor
  ctx.beginPath()
  ctx.roundRect(barX, rect.y, p.barWidth, rect.height, Math.min(p.barWidth, 4 * unit))
  ctx.fill()

  ctx.fillStyle = settings.textColor
  ctx.textAlign = right ? 'right' : 'left'
  ctx.textBaseline = 'top'
  ctx.shadowColor = '#000000'
  ctx.shadowOffsetX = unit
  ctx.shadowOffsetY = unit
  ctx.shadowBlur = 3 * unit

  let y = rect.y
  wrapWatermarkLines(ctx, lines, layout.fontSize, p.textWidth, p.restRatio).forEach((parts, i) => {
    const size = i === 0 ? layout.fontSize : layout.fontSize * p.restRatio
    ctx.font = fontFor(size)
    for (const part of parts) {
      ctx.fillText(part, textEdge, y + (size * (LINE_HEIGHT - 1)) / 2)
      y += size * LINE_HEIGHT
    }
    y += p.lineGap
  })
  ctx.restore()
}
