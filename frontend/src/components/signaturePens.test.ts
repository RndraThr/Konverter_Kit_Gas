import { expect, test } from 'vitest';
import { pointWidths, strokeMaxWidth, strokesBounds, type SignatureStroke } from './signaturePens';

const line = (pen: SignatureStroke['pen'], step: number, msPerPoint: number, count = 30): SignatureStroke => ({
  pen, color: '#111111', size: 5,
  points: Array.from({ length: count }, (_, i) => ({ x: i * step, y: 0 })),
  times: Array.from({ length: count }, (_, i) => i * msPerPoint),
});

test('pena tinta draws thinner when the hand moves fast', () => {
  const slow = pointWidths(line('tinta', 1, 10), 5.5);
  const fast = pointWidths(line('tinta', 12, 4), 5.5);
  expect(fast[15]).toBeLessThan(slow[15]);
});

test('kuas tapers to a point at both ends', () => {
  const widths = pointWidths(line('kuas', 3, 16), 6.5);
  expect(widths[0]).toBeLessThan(widths[15] / 3);
  expect(widths[widths.length - 1]).toBeLessThan(widths[15] / 3);
});

test('kaligrafi is broad across the 45° nib and fine along it', () => {
  const along = { ...line('kaligrafi', 2, 16), points: Array.from({ length: 30 }, (_, i) => ({ x: i * 2, y: i * 2 })) };
  const across = { ...line('kaligrafi', 2, 16), points: Array.from({ length: 30 }, (_, i) => ({ x: i * 2, y: -i * 2 })) };
  expect(pointWidths(across, 6)[20]).toBeGreaterThan(pointWidths(along, 6)[20] * 3);
});

test('export bounds include the widest the pen can draw', () => {
  const stroke = line('spidol', 1, 16, 2);
  expect(strokeMaxWidth(stroke)).toBe(10);
  expect(strokesBounds([stroke])).toEqual({ minX: -5, minY: -5, maxX: 6, maxY: 5 });
  expect(strokesBounds([])).toBeNull();
});
