import { expect, test } from 'vitest';
import { officeIcon } from './officeIcon';

test('the office icon is a filled circle with a white house in the middle and a clear outside', () => {
  const icon = officeIcon('#0f4c81');
  const pixel = (x: number, y: number) => Array.from(icon.data.slice((y * icon.width + x) * 4, (y * icon.width + x) * 4 + 4));
  const mid = icon.width / 2;
  expect(icon.data).toHaveLength(icon.width * icon.height * 4);
  expect(pixel(0, 0)[3]).toBe(0); // corner: transparent
  expect(pixel(Math.round(mid), Math.round(icon.height * 0.38))).toEqual([255, 255, 255, 255]); // roof
  expect(pixel(Math.round(mid), Math.round(icon.height * 0.7))).toEqual([15, 76, 129, 255]); // door
  expect(pixel(Math.round(icon.width * 0.15), Math.round(mid))).toEqual([15, 76, 129, 255]); // background
});
