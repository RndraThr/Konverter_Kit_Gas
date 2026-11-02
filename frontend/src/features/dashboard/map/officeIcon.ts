// The Dinas Pertanian marker: a white house on a filled circle with a white ring. Drawn into raw
// RGBA pixels (no canvas), so it works the same in every browser and in tests.

export const OFFICE_ICON_ID = 'konkit-office-icon';
// Drawn at twice the display size for sharp edges on high-density screens.
export const OFFICE_ICON_PIXEL_RATIO = 2;
const SIZE = 52;
const SAMPLES = 4; // per axis, for anti-aliased edges

type Rgb = [number, number, number];

function hexToRgb(hex: string): Rgb {
  const value = Number.parseInt(hex.replace('#', ''), 16);
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

// Shape colour at (x, y) in a 0..1 square, or null outside the marker.
function shade(x: number, y: number, fill: Rgb): Rgb | null {
  const white: Rgb = [255, 255, 255];
  const r = Math.hypot(x - 0.5, y - 0.5);
  if (r > 0.5) return null;
  if (r > 0.42) return white; // ring
  // Roof: triangle from (0.22, 0.5) to (0.78, 0.5) with its apex at (0.5, 0.24).
  if (y >= 0.24 && y <= 0.5 && Math.abs(x - 0.5) <= ((y - 0.24) / 0.26) * 0.28) return white;
  // Walls, with a door cut out.
  if (x >= 0.3 && x <= 0.7 && y >= 0.48 && y <= 0.74) {
    if (x >= 0.45 && x <= 0.55 && y >= 0.6) return fill;
    return white;
  }
  return fill;
}

/** The marker as MapLibre image data (`map.addImage(id, data, { pixelRatio })`). */
export function officeIcon(color: string): { width: number; height: number; data: Uint8Array } {
  const fill = hexToRgb(color);
  const data = new Uint8Array(SIZE * SIZE * 4);
  for (let py = 0; py < SIZE; py++) {
    for (let px = 0; px < SIZE; px++) {
      let red = 0, green = 0, blue = 0, hits = 0;
      for (let sy = 0; sy < SAMPLES; sy++) {
        for (let sx = 0; sx < SAMPLES; sx++) {
          const rgb = shade((px + (sx + 0.5) / SAMPLES) / SIZE, (py + (sy + 0.5) / SAMPLES) / SIZE, fill);
          if (!rgb) continue;
          red += rgb[0]; green += rgb[1]; blue += rgb[2]; hits++;
        }
      }
      if (!hits) continue;
      const i = (py * SIZE + px) * 4;
      data[i] = Math.round(red / hits);
      data[i + 1] = Math.round(green / hits);
      data[i + 2] = Math.round(blue / hits);
      data[i + 3] = Math.round((hits / (SAMPLES * SAMPLES)) * 255);
    }
  }
  return { width: SIZE, height: SIZE, data };
}
