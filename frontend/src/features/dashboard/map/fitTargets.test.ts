import { expect, test } from 'vitest';
import { INDONESIA_BOUNDS, fitPadding, geometryBounds, initialFitTarget, pointsBounds, regencyBoundsIndex } from './fitTargets';
import type { PointCollection, PointFeature } from './types';
import type { MapRegion } from '../types';

function point(lng: number, lat: number): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates: [lng, lat] },
    properties: { slot_id: `${lng},${lat}`, code: '1', schedule_id: 's', regency_id: 'r', zone_id: '', site_id: 'geo:s:1', site_name: '', site_outlier: false, stage: 1, stage_at: [], location_status: 'inside' },
  };
}
const collection = (features: PointFeature[]): PointCollection => ({ type: 'FeatureCollection', features });

const indramayuBounds: [[number, number], [number, number]] = [[107.85, -6.67], [108.55, -6.22]];

test('a scoped officer without located points is fitted to the regencies of the scope (Review Focus 1)', () => {
  const boundaries = new Map([['indramayu', indramayuBounds], ['bandung', [[107, -7.3], [107.9, -6.8]] as [[number, number], [number, number]]]]);
  expect(initialFitTarget(collection([]), ['indramayu'], boundaries)).toEqual(indramayuBounds);
});

test('scope bounds are the union of every scoped regency that has a boundary', () => {
  const boundaries = new Map([['a', [[100, -2], [101, -1]] as [[number, number], [number, number]]], ['b', [[102, -4], [103, -3]] as [[number, number], [number, number]]]]);
  expect(initialFitTarget(collection([]), ['a', 'b', 'missing'], boundaries)).toEqual([[100, -4], [103, -1]]);
});

test('located points win over the scope', () => {
  const target = initialFitTarget(collection([point(108.1, -6.4), point(108.3, -6.3)]), ['indramayu'], new Map([['indramayu', indramayuBounds]]));
  expect(target).toEqual([[108.1, -6.4], [108.3, -6.3]]);
});

test('falls back to the whole of Indonesia when there are no points and no scope boundary', () => {
  expect(initialFitTarget(collection([]), ['unknown'], new Map())).toEqual([[94, -11.5], [141.5, 6.5]]);
  expect(INDONESIA_BOUNDS).toEqual([[94, -11.5], [141.5, 6.5]]);
});

test('a single point gets a small box instead of a zero-area bounds', () => {
  const [[w, s], [e, n]] = pointsBounds([point(108, -6)])!;
  expect(w).toBeLessThan(108);
  expect(e).toBeGreaterThan(108);
  expect(s).toBeLessThan(-6);
  expect(n).toBeGreaterThan(-6);
});

test('regency bounds are indexed by master regency id through the name used in the boundary asset', () => {
  const asset = { source: 't', license: 'l', regencies: [{ n: 'Indramayu', c: [108, -6.4] as [number, number], r: [[[107.85, -6.22], [108.55, -6.22], [108.55, -6.67], [107.85, -6.67]]] }] };
  const regions = [{ regency_id: 'indramayu-id', regency_name: 'KAB. INDRAMAYU' }] as MapRegion[];
  expect(regencyBoundsIndex(asset, regions).get('indramayu-id')).toEqual(indramayuBounds);
});

test('geometryBounds covers polygons and multipolygons', () => {
  expect(geometryBounds({ type: 'MultiPolygon', coordinates: [[[[1, 1], [2, 3]]], [[[0, -1], [1, 0]]]] })).toEqual([[0, -1], [2, 3]]);
});

test('fit padding is 60 px and adds the panel width on the left on desktop only', () => {
  expect(fitPadding(1280, 360)).toEqual({ top: 60, right: 60, bottom: 60, left: 420 });
  expect(fitPadding(1280, 0)).toEqual({ top: 60, right: 60, bottom: 60, left: 60 });
  expect(fitPadding(500, 360)).toEqual({ top: 60, right: 60, bottom: 60, left: 60 });
});
