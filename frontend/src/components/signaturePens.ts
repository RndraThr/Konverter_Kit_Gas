// Pen types of the signature pad, drawn the same way as the phone app
// (Konverter_Kit_Gas_Mobile signature_pad.dart): the Samsung Notes set minus
// the highlighter.

export type SignaturePen = 'pulpen' | 'tinta' | 'kaligrafi' | 'kuas' | 'pensil' | 'spidol';

type Point = { x: number; y: number };

export type SignatureStroke = {
  pen: SignaturePen;
  color: string;
  size: number;
  points: Point[];
  /** Milliseconds of each point (ink pens follow the speed). */
  times: number[];
};

export const signaturePens: Array<{ pen: SignaturePen; label: string; weight: number; swell: number }> = [
  { pen: 'pulpen', label: 'Pulpen', weight: 1.0, swell: 1 },
  { pen: 'tinta', label: 'Pena tinta', weight: 1.1, swell: 1.2 },
  { pen: 'kaligrafi', label: 'Kaligrafi', weight: 1.2, swell: 1.75 },
  { pen: 'kuas', label: 'Kuas', weight: 1.3, swell: 2.2 },
  { pen: 'pensil', label: 'Pensil', weight: 0.8, swell: 1 },
  { pen: 'spidol', label: 'Spidol', weight: 2.0, swell: 1 },
];

const penInfo = (pen: SignaturePen) => signaturePens.find((item) => item.pen === pen) ?? signaturePens[0];

export const isSignaturePen = (value: unknown): value is SignaturePen => signaturePens.some((item) => item.pen === value);

/** Widest the stroke can draw, for the export bounds. */
export function strokeMaxWidth(stroke: SignatureStroke) {
  const info = penInfo(stroke.pen);
  return stroke.size * info.weight * info.swell;
}

/** Bounds of the strokes including their width. */
export function strokesBounds(strokes: SignatureStroke[]) {
  let bounds: { minX: number; minY: number; maxX: number; maxY: number } | null = null;
  for (const stroke of strokes) {
    const r = strokeMaxWidth(stroke) / 2;
    for (const p of stroke.points) {
      bounds = bounds
        ? { minX: Math.min(bounds.minX, p.x - r), minY: Math.min(bounds.minY, p.y - r), maxX: Math.max(bounds.maxX, p.x + r), maxY: Math.max(bounds.maxY, p.y + r) }
        : { minX: p.x - r, minY: p.y - r, maxX: p.x + r, maxY: p.y + r };
    }
  }
  return bounds;
}

const lerp = (a: Point, b: Point, t: number): Point => ({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t });
const distance = (a: Point, b: Point) => Math.hypot(b.x - a.x, b.y - a.y);

/** Smoothed path through the points (quadratic curves via midpoints). */
function smoothPath(points: Point[]) {
  const path = new Path2D();
  path.moveTo(points[0].x, points[0].y);
  for (let i = 1; i < points.length - 1; i++) {
    const mid = lerp(points[i], points[i + 1], 0.5);
    path.quadraticCurveTo(points[i].x, points[i].y, mid.x, mid.y);
  }
  path.lineTo(points[points.length - 1].x, points[points.length - 1].y);
  return path;
}

/** Width at each point of a pressure-like pen, eased so it flows. */
export function pointWidths(stroke: SignatureStroke, base: number) {
  const { points, times, pen } = stroke;
  const n = points.length - 1;
  const widths = Array<number>(points.length).fill(base);
  let width = pen === 'kaligrafi' ? base : base * 1.2;
  for (let i = 1; i <= n; i++) {
    let target: number;
    if (pen === 'kaligrafi') {
      // Flat nib held at 45°: broad across, hairline along it. The direction
      // spans a few points so a jittery finger does not flicker the width.
      const from = points[Math.max(0, i - 3)];
      const direction = Math.atan2(points[i].y - from.y, points[i].x - from.x);
      target = base * (0.25 + 1.5 * Math.abs(Math.sin(direction - Math.PI / 4)));
    } else {
      // Speed over the last few points (single events are noisy).
      const from = Math.max(0, i - 3);
      let travelled = 0;
      for (let k = from + 1; k <= i; k++) travelled += distance(points[k - 1], points[k]);
      const speed = travelled / Math.max(1, times[i] - times[from]); // px per ms
      // Ink pen: full width when slow, a fine line when fast.
      target = pen === 'kuas'
        ? base * Math.min(2.2, Math.max(0.3, 2.2 - speed * 1.1))
        : base * Math.min(1.15, Math.max(0.28, 1.15 - speed * 0.7));
    }
    width = pen === 'tinta' ? width * 0.85 + target * 0.15 : width * 0.75 + target * 0.25;
    widths[i] = width;
  }
  widths[0] = widths.length > 1 ? widths[1] : base;
  // Pointed tips over a length of the stroke: long for the brush, shorter for the ink pen.
  const taperLength = pen === 'kuas' ? base * 6 : pen === 'tinta' ? base * 4.5 : 0;
  if (taperLength > 0 && n > 0) {
    const along = Array<number>(points.length).fill(0);
    for (let i = 1; i <= n; i++) along[i] = along[i - 1] + distance(points[i - 1], points[i]);
    const total = along[n];
    const length = Math.min(taperLength, total / 2);
    if (length > 0) {
      for (let i = 0; i <= n; i++) {
        const t = Math.min(1, Math.max(0, Math.min(along[i], total - along[i]) / length));
        widths[i] *= 0.15 + 0.85 * Math.sqrt(t);
      }
    }
  }
  return widths;
}

/**
 * Varying-width stroke along the smoothed curve, sampled densely so neither
 * the line nor its width shows corners.
 */
function paintVariable(ctx: CanvasRenderingContext2D, stroke: SignatureStroke, base: number) {
  const { points } = stroke;
  const widths = pointWidths(stroke, base);
  const n = points.length - 1;
  let last: Point | null = null;
  let lastWidth = widths[0];
  const sample = (at: Point, width: number) => {
    if (last) {
      ctx.lineWidth = (lastWidth + width) / 2;
      ctx.beginPath();
      ctx.moveTo(last.x, last.y);
      ctx.lineTo(at.x, at.y);
      ctx.stroke();
    }
    last = at;
    lastWidth = width;
  };
  // Segment k runs from the midpoint before point k to the one after it,
  // bending towards point k (the stroke ends at its real end points).
  for (let k = 1; k < n; k++) {
    const start = k === 1 ? points[0] : lerp(points[k - 1], points[k], 0.5);
    const end = k === n - 1 ? points[n] : lerp(points[k], points[k + 1], 0.5);
    const startWidth = k === 1 ? widths[0] : (widths[k - 1] + widths[k]) / 2;
    const endWidth = k === n - 1 ? widths[n] : (widths[k] + widths[k + 1]) / 2;
    const steps = Math.floor(Math.min(24, Math.max(2, distance(start, points[k]) + distance(points[k], end))) / 1.5);
    for (let j = k === 1 ? 0 : 1; j <= steps; j++) {
      const t = j / steps;
      const u = 1 - t;
      const at = {
        x: start.x * u * u + points[k].x * 2 * u * t + end.x * t * t,
        y: start.y * u * u + points[k].y * 2 * u * t + end.y * t * t,
      };
      sample(at, startWidth * u * u + widths[k] * 2 * u * t + endWidth * t * t);
    }
  }
  if (n === 1) {
    sample(points[0], widths[0]);
    sample(points[1], widths[1]);
  }
}

/**
 * Draws one stroke:
 * - pulpen and spidol: even width;
 * - pena tinta: thinner when the hand moves fast, like a nib;
 * - kaligrafi: flat nib at 45°, width follows the direction;
 * - kuas: strong speed swell with tapered ends;
 * - pensil: thin, slightly grainy graphite.
 */
export function paintStroke(ctx: CanvasRenderingContext2D, stroke: SignatureStroke) {
  const { points } = stroke;
  if (points.length === 0) return;
  const base = stroke.size * penInfo(stroke.pen).weight;
  ctx.save();
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  ctx.strokeStyle = stroke.color;
  ctx.fillStyle = stroke.color;
  if (points.every((p) => p.x === points[0].x && p.y === points[0].y)) {
    ctx.globalAlpha = stroke.pen === 'pensil' ? 0.8 : 1;
    ctx.beginPath();
    ctx.arc(points[0].x, points[0].y, base / 2, 0, Math.PI * 2);
    ctx.fill();
  } else if (stroke.pen === 'pulpen' || stroke.pen === 'spidol') {
    ctx.lineWidth = base;
    ctx.stroke(smoothPath(points));
  } else if (stroke.pen === 'pensil') {
    const path = smoothPath(points);
    ctx.globalAlpha = 0.78;
    ctx.lineWidth = base;
    ctx.stroke(path);
    // Grain: faint offset passes so the line is not perfectly solid.
    for (const [dx, dy, alpha, scale] of [[0.45, 0.35, 0.22, 0.7], [-0.35, -0.3, 0.18, 0.6]]) {
      ctx.save();
      ctx.translate(dx, dy);
      ctx.globalAlpha = alpha;
      ctx.lineWidth = base * scale;
      ctx.stroke(path);
      ctx.restore();
    }
  } else {
    paintVariable(ctx, stroke, base);
  }
  ctx.restore();
}
