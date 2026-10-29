import { expect, test } from 'vitest';
import { MILESTONES, REVIEW_COLOR } from './milestones';

test('milestones keep the agreed order, labels and colours', () => {
  expect(MILESTONES.map((m) => [m.stage, m.label, m.color])).toEqual([
    [1, 'Terjadwal', '#c9d1c9'],
    [2, 'Penerima tertaut', '#9fcdb0'],
    [3, 'Diserahkan', '#5fae7a'],
    [4, 'Bukti lengkap', '#2f7d4a'],
    [5, 'BA ditandatangani', '#143f24'],
  ]);
  expect(REVIEW_COLOR).toBe('#d9711c');
});
