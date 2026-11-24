import { expect, test } from 'vitest';
import { cumulative, percent, zoneLabel } from './stats';

test('percent of the quota is rounded and 0 without a quota', () => {
  expect(percent(1, 3)).toBe(33);
  expect(percent(2, 3)).toBe(67);
  expect(percent(1, 0)).toBe(0);
  expect(percent(5, -1)).toBe(0);
});

test('cumulative adds the handovers day by day', () => {
  expect(cumulative([{ date: '2026-11-20', completed: 3 }, { date: '2026-11-21', completed: 0 }, { date: '2026-11-22', completed: 4 }])).toEqual([
    { date: '2026-11-20', completed: 3, total: 3 },
    { date: '2026-11-21', completed: 0, total: 3 },
    { date: '2026-11-22', completed: 4, total: 7 },
  ]);
  expect(cumulative([])).toEqual([]);
});

test('a kabupaten without a zone is grouped as Tanpa zona', () => {
  expect(zoneLabel('')).toBe('Tanpa zona');
  expect(zoneLabel('Zona I')).toBe('Zona I');
});
