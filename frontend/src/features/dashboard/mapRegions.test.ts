import { expect, test } from 'vitest';
import { buildRegencyPaths, buildRegionIndex, colorForValue, EMPTY_REGION_COLOR, mapMetrics, metricByKey, metricMax, normalizeRegencyName, progressDenominator, REGION_SCALE, type RegencyFeature } from './mapRegions';
import type { MapRegion } from './types';

function region(overrides: Partial<MapRegion> = {}): MapRegion {
  return {
    regency_id: 'regency-1', regency_name: 'KAB. BANGKA BARAT', province_name: 'Bangka Belitung', document_code: 'BBR',
    recipients: 0, candidate: 0, ready: 0, distributed: 0, needs_review: 0, replaced: 0,
    evidence_complete: 0, evidence_partial: 0, evidence_empty: 0, evidence_not_configured: 0,
    slot_quota: 0, slots_open: 0, slots_linked: 0, slots_completed: 0, slots_cancelled: 0,
    ...overrides,
  };
}

const features: RegencyFeature[] = [
  { n: 'Bangka Barat', c: [105.5, -2], r: [[[105, -1], [106, -1], [106, -3], [105, -3], [105, -1]]] },
  { n: 'Bangka', c: [106.5, -2], r: [[[106, -1], [107, -1], [107, -3], [106, -3], [106, -1]]] },
];

test('normalizes master and boundary regency names to the same key', () => {
  expect(normalizeRegencyName('KAB. BANGKA BARAT')).toBe('BANGKABARAT');
  expect(normalizeRegencyName('Bangka Barat')).toBe('BANGKABARAT');
  expect(normalizeRegencyName('KOTA PADANG PANJANG')).toBe('KOTAPADANGPANJANG');
  expect(normalizeRegencyName('KABUPATEN  KEPULAUAN SERIBU')).toBe('KEPULAUANSERIBU');
  expect(normalizeRegencyName('Kota Padang Panjang')).toBe(normalizeRegencyName('KOTA PADANG PANJANG'));
});

test('keeps a city distinct from a regency with the same base name', () => {
  expect(normalizeRegencyName('KAB. BANDUNG')).toBe('BANDUNG');
  expect(normalizeRegencyName('Bandung')).toBe('BANDUNG');
  expect(normalizeRegencyName('KOTA BANDUNG')).toBe('KOTABANDUNG');
  expect(normalizeRegencyName('Kota Bandung')).toBe('KOTABANDUNG');
});

test('indexes regions under the same key the boundary asset uses', () => {
  const index = buildRegionIndex([region()]);
  expect(index.get('BANGKABARAT')?.document_code).toBe('BBR');
});

test('projects every feature inside the SVG viewbox', () => {
  const paths = buildRegencyPaths(features, 1000, 420);
  expect(paths.map((item) => item.key)).toEqual(['BANGKABARAT', 'BANGKA']);
  for (const item of paths) {
    expect(item.path.startsWith('M')).toBe(true);
    expect(item.path.endsWith('Z')).toBe(true);
    const numbers = item.path.match(/-?\d+(\.\d+)?/g)?.map(Number) ?? [];
    expect(numbers.length).toBe(10);
    for (let index = 0; index < numbers.length; index += 2) {
      expect(numbers[index]).toBeGreaterThanOrEqual(0);
      expect(numbers[index]).toBeLessThanOrEqual(1000);
      expect(numbers[index + 1]).toBeGreaterThanOrEqual(0);
      expect(numbers[index + 1]).toBeLessThanOrEqual(420);
    }
    expect(item.center[0]).toBeGreaterThan(0);
  }
});

test('returns no paths for an empty asset instead of NaN coordinates', () => {
  expect(buildRegencyPaths([], 1000, 420)).toEqual([]);
});

test('buckets metric values across the sequential scale and marks missing data', () => {
  expect(colorForValue(0, 10, false)).toBe(EMPTY_REGION_COLOR);
  expect(colorForValue(0, 10, true)).toBe(REGION_SCALE[0]);
  expect(colorForValue(10, 10, true)).toBe(REGION_SCALE[REGION_SCALE.length - 1]);
  expect(colorForValue(5, 10, true)).toBe(REGION_SCALE[2]);
  // Monotonic: a larger value never gets a lighter colour.
  const steps = [0, 1, 3, 6, 10].map((value) => REGION_SCALE.indexOf(colorForValue(value, 10, true)));
  expect([...steps].sort((a, b) => a - b)).toEqual(steps);
});

test('derives the three map metrics from the aggregate', () => {
  const complete = region({ recipients: 4, evidence_complete: 3, slot_quota: 10, slots_completed: 5, slots_open: 2 });
  expect(metricByKey('recipients').value(complete)).toBe(4);
  expect(metricByKey('progress').value(complete)).toBeCloseTo(0.5);
  expect(metricByKey('evidence').value(complete)).toBeCloseTo(0.75);
  expect(metricByKey('progress').format(complete)).toBe('50% selesai · 5/10 nomor');
  expect(metricByKey('evidence').format(complete)).toBe('75% lengkap · 3/4 penerima');
  expect(metricByKey('recipients').format(complete)).toBe('4 penerima');
});

test('falls back to created slots when a schedule has no quota', () => {
  const noQuota = region({ slot_quota: 0, slots_open: 3, slots_linked: 1, slots_completed: 2 });
  expect(progressDenominator(noQuota)).toBe(6);
  expect(metricByKey('progress').value(noQuota)).toBeCloseTo(2 / 6);
  // Ratio metrics share one scale, counts scale to the largest region.
  expect(metricMax(metricByKey('progress'), [noQuota])).toBe(1);
  expect(metricMax(metricByKey('evidence'), [noQuota])).toBe(1);
  expect(metricMax(metricByKey('recipients'), [region({ recipients: 9 }), region({ recipients: 2 })])).toBe(9);
  expect(metricMax(metricByKey('recipients'), [])).toBe(1);
});

test('treats a region without recipients as zero rather than a division error', () => {
  const empty = region();
  expect(metricByKey('evidence').value(empty)).toBe(0);
  expect(metricByKey('evidence').format(empty)).toBe('0% lengkap · 0/0 penerima');
  expect(metricByKey('progress').value(empty)).toBe(0);
  expect(mapMetrics).toHaveLength(3);
});

test('distinguishes a real zero from a metric with no denominator', () => {
  const empty = region();
  const scheduled = region({ slot_quota: 10 });
  expect(metricByKey('recipients').hasData(empty)).toBe(false);
  expect(metricByKey('progress').hasData(empty)).toBe(false);
  expect(metricByKey('progress').hasData(scheduled)).toBe(true);
  expect(metricByKey('evidence').hasData(empty)).toBe(false);
});
