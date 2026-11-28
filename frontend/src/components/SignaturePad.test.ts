import { expect, test } from 'vitest';
import { tintPixels } from './SignaturePad';

test('tinting keeps the transparency of every pixel and only changes the ink', () => {
  // Opaque stroke, soft edge, empty background.
  const pixels = new Uint8ClampedArray([30, 45, 110, 255, 30, 45, 110, 90, 0, 0, 0, 0]);
  tintPixels(pixels, '#d32f2f');
  expect([...pixels]).toEqual([211, 47, 47, 255, 211, 47, 47, 90, 0, 0, 0, 0]);
});
