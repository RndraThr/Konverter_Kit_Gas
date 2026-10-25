import { LINE_HEIGHT, timestampMetrics, timestampParts, WATERMARK_REF_SIDE, type WatermarkLayout } from './watermarkLayout'
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
 * Word-wraps each source line to the timestamp column. Text is always measured
 * at the reference photo size (390 short side), never at the actual pixel size,
 * so the preview and the full-size photo break every line at the same place
 * even though real font metrics are not perfectly linear. Pass the result to
 * `drawWatermark`; its total row count is the `timestampLines` for
 * `layoutWatermark`.
 */
export function wrapWatermarkLines(ctx: CanvasRenderingContext2D, lines: string[], settings: WatermarkSettings): string[][] {
  const m = timestampMetrics(WATERMARK_REF_SIDE, WATERMARK_REF_SIDE, settings)
  const previousFont = ctx.font
  const out = lines.map((text, i) => {
    ctx.font = fontFor(i === 0 ? m.fontSize : m.fontSize * m.restRatio)
    return wrapOne(ctx, text, m.textWidth)
  })
  ctx.font = previousFont
  return out
}

/** Draws the logo banner and the timestamp block (accent bar + shadowed text). */
export function drawWatermark(
  ctx: CanvasRenderingContext2D,
  layout: WatermarkLayout,
  wrapped: string[][],
  logo: CanvasImageSource | null,
  settings: WatermarkSettings,
): void {
  if (layout.logo && logo) {
    ctx.drawImage(logo, layout.logo.x, layout.logo.y, layout.logo.width, layout.logo.height)
  }

  const rect = layout.timestamp
  if (!rect || wrapped.length === 0) return
  const p = timestampParts(rect, settings)
  const right = settings.timestampSide === 'right'
  const barX = right ? rect.x + rect.width - p.barWidth : rect.x
  const textEdge = right ? rect.x + rect.width - p.barWidth - p.barGap : rect.x + p.barWidth + p.barGap
  const unit = layout.fontSize / 13

  ctx.save()
  ctx.fillStyle = settings.accentColor
  if (typeof ctx.roundRect === 'function') {
    ctx.beginPath()
    ctx.roundRect(barX, rect.y, p.barWidth, rect.height, Math.min(p.barWidth, 4 * unit))
    ctx.fill()
  } else {
    // Older browsers (Safari < 16, Firefox < 112) lack roundRect.
    ctx.fillRect(barX, rect.y, p.barWidth, rect.height)
  }

  ctx.fillStyle = settings.textColor
  ctx.textAlign = right ? 'right' : 'left'
  ctx.textBaseline = 'top'
  ctx.shadowColor = '#000000'
  ctx.shadowOffsetX = unit
  ctx.shadowOffsetY = unit
  ctx.shadowBlur = 3 * unit

  let y = rect.y
  wrapped.forEach((parts, i) => {
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
