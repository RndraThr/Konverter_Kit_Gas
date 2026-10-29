import { expect, test } from 'vitest';
import type maplibregl from 'maplibre-gl';
import { PLACEHOLDER_ZONE_COLOR, ZONE_PALETTE, buildLayerData, buildRegionFills, buildSites, zoneColorIndex } from './layerData';
import { LAYER_IDS, installLayers, updateLayers } from './layers';
import { MILESTONES, REVIEW_COLOR } from './milestones';
import type { PointFeature, PointProperties, ZoneCollection } from './types';
import type { MapRegion } from '../types';

function point(overrides: Partial<PointProperties> = {}, coordinates: [number, number] = [108, -6]): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates },
    properties: { slot_id: 'slot-1', code: '1', schedule_id: 's', regency_id: 'r', zone_id: 'z1', site_id: 'geo:s:1', site_name: '', site_outlier: false, stage: 1, stage_at: [], location_status: 'inside', ...overrides },
  };
}

const square = (x: number): number[][][] => [[[x, 0], [x + 1, 0], [x + 1, 1], [x, 1], [x, 0]]];
const zones: ZoneCollection = {
  type: 'FeatureCollection',
  features: [
    { type: 'Feature', geometry: { type: 'Polygon', coordinates: square(0) }, properties: { zone_id: 'z1', code: 'A', name: 'ZONA A', is_placeholder: false, regency_count: 1 } },
    { type: 'Feature', geometry: { type: 'Polygon', coordinates: square(2) }, properties: { zone_id: 'z0', code: '-', name: 'Tanpa zona', is_placeholder: true, regency_count: 1 } },
  ],
};

test('a site with several slots takes the colour of its lowest milestone', () => {
  const { sites } = buildSites([
    point({ slot_id: 'a', site_id: 'hl:1', site_name: 'Balai Desa', stage: 5 }),
    point({ slot_id: 'b', site_id: 'hl:1', site_name: 'Balai Desa', stage: 2 }),
    point({ slot_id: 'c', site_id: 'hl:1', site_name: 'Balai Desa', stage: 4 }),
  ]);
  expect(sites.features).toHaveLength(1);
  const site = sites.features[0].properties;
  expect(site.stage).toBe(2);
  expect(site.color).toBe(MILESTONES[1].color);
  expect(site.count).toBe(3);
  expect(site.label).toBe('Balai Desa (3)');
});

test('site members are drawn only as the site marker, single geo points stay dots', () => {
  const data = buildLayerData({
    points: [point({ slot_id: 'a', site_id: 'geo:s:1' }), point({ slot_id: 'b', site_id: 'geo:s:1' }), point({ slot_id: 'c', site_id: 'geo:s:2' })],
    zones: null, regions: { type: 'FeatureCollection', features: [] }, colorMode: 'progress',
  });
  expect(data.sites.features.map((f) => f.properties.count)).toEqual([2]);
  expect(data.sites.features[0].properties.label).toBe('2');
  expect(data.points.features.map((f) => f.properties.slot_id)).toEqual(['c']);
});

test('a single slot at a named handover location is still a labelled site', () => {
  const { sites } = buildSites([point({ site_id: 'hl:9', site_name: 'Balai Desa' })]);
  expect(sites.features[0].properties.label).toBe('Balai Desa');
});

test('outliers and points outside their regency carry the review ring', () => {
  const data = buildLayerData({
    points: [point({ slot_id: 'a', location_status: 'outside', site_id: 'geo:s:1' }), point({ slot_id: 'b', site_id: 'geo:s:2' })],
    zones: null, regions: { type: 'FeatureCollection', features: [] }, colorMode: 'progress',
  });
  expect(data.points.features.map((f) => f.properties.review)).toEqual([true, false]);
  const { sites } = buildSites([point({ slot_id: 'a', site_id: 'hl:1', site_outlier: true }), point({ slot_id: 'b', site_id: 'hl:1' })]);
  expect(sites.features[0].properties.review).toBe(true);
  expect(REVIEW_COLOR).toBe('#d9711c');
});

test('zone mode paints placeholder zones grey with the "Belum dizonasi" label', () => {
  const data = buildLayerData({ points: [], zones, regions: { type: 'FeatureCollection', features: [] }, colorMode: 'zone' });
  const placeholder = data.zones.features.find((f) => f.properties.zone_id === 'z0')!;
  expect(placeholder.properties.color).toBe(PLACEHOLDER_ZONE_COLOR);
  expect(PLACEHOLDER_ZONE_COLOR).toBe('#9aa39c');
  const label = data.zoneLabels.features.find((f) => f.properties.zone_id === 'z0')!;
  expect(label.properties.name).toBe('Belum dizonasi');
  expect(data.zones.features.find((f) => f.properties.zone_id === 'z1')!.properties.color).toBe(ZONE_PALETTE[0]);
});

test('zone summary labels count the visible points of each zone', () => {
  const data = buildLayerData({ points: [point({ slot_id: 'a' }), point({ slot_id: 'b', site_id: 'geo:s:2' })], zones, regions: { type: 'FeatureCollection', features: [] }, colorMode: 'zone' });
  expect(data.zoneLabels.features.find((f) => f.properties.zone_id === 'z1')!.properties.count).toBe(2);
  expect(data.zoneLabels.features.find((f) => f.properties.zone_id === 'z0')!.properties.count).toBe(0);
});

test('the zone palette has 8 distinct colours that also differ in lightness', () => {
  expect(new Set(ZONE_PALETTE).size).toBe(8);
  const lightness = ZONE_PALETTE.map((hex) => {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
    return Math.round((0.2126 * r + 0.7152 * g + 0.0722 * b) / 10);
  });
  expect(new Set(lightness).size).toBeGreaterThanOrEqual(5);
});

test('regency fills use zone colours in zone mode and the metric scale otherwise', () => {
  const asset = { source: 't', license: 'l', regencies: [{ n: 'Indramayu', c: [0.5, 0.5] as [number, number], r: square(0) }, { n: 'Cirebon', c: [2.5, 0.5] as [number, number], r: square(2) }] };
  const regions = [
    { regency_id: 'r1', regency_name: 'KAB. INDRAMAYU', zone_id: 'z1', recipients: 10, slot_quota: 10, slots_open: 0, slots_linked: 0, slots_completed: 10, slots_cancelled: 0, evidence_complete: 10 },
    { regency_id: 'r2', regency_name: 'KAB. CIREBON', zone_id: 'z0', recipients: 0, slot_quota: 0, slots_open: 0, slots_linked: 0, slots_completed: 0, slots_cancelled: 0, evidence_complete: 0 },
  ] as MapRegion[];
  const colors = zoneColorIndex(zones);
  const zoneFills = buildRegionFills(asset, regions, 'zone', colors);
  expect(zoneFills.features.map((f) => [f.properties.regency_id, f.properties.color])).toEqual([['r1', ZONE_PALETTE[0]], ['r2', PLACEHOLDER_ZONE_COLOR]]);
  const progress = buildRegionFills(asset, regions, 'progress', colors);
  expect(progress.features[0].properties.color).toBe('#2f7d4a');
  expect(progress.features[0].geometry.type).toBe('MultiPolygon');
});

type FakeLayer = { id: string; minzoom?: number };
function fakeMap() {
  const layers: FakeLayer[] = [];
  const data: Record<string, unknown> = {};
  const paint: Record<string, unknown> = {};
  const zoom: Record<string, number> = {};
  const map = {
    getSource: (id: string) => (id in data ? { setData: (d: unknown) => { data[id] = d; } } : undefined),
    addSource: (id: string, spec: { data: unknown }) => { data[id] = spec.data; },
    getLayer: (id: string) => layers.find((l) => l.id === id),
    addLayer: (layer: FakeLayer) => { layers.push({ id: layer.id }); },
    setPaintProperty: (id: string, prop: string, value: unknown) => { paint[`${id}.${prop}`] = value; },
    setLayoutProperty: (id: string, prop: string, value: unknown) => { paint[`${id}.${prop}`] = value; },
    setLayerZoomRange: (id: string, min: number) => { zoom[id] = min; },
  };
  return { map: map as unknown as maplibregl.Map, layers, data, paint, zoom };
}

test('installLayers adds the konkit layers once, in drawing order', () => {
  const fake = fakeMap();
  const input = { points: [point()], zones, regions: { type: 'FeatureCollection' as const, features: [] }, colorMode: 'zone' as const };
  installLayers(fake.map, input);
  installLayers(fake.map, input);
  expect(fake.layers.map((l) => l.id)).toEqual(LAYER_IDS);
  for (const id of ['konkit-regions', 'konkit-zone-line', 'konkit-zone-label', 'konkit-clusters', 'konkit-cluster-count', 'konkit-dots', 'konkit-sites', 'konkit-focus']) {
    expect(LAYER_IDS).toContain(id);
  }
});

test('updateLayers swaps data and hides point layers below zoom 7 only in zone mode with zones', () => {
  const fake = fakeMap();
  const input = { points: [point()], zones, regions: { type: 'FeatureCollection' as const, features: [] }, colorMode: 'zone' as const };
  installLayers(fake.map, input);
  expect(fake.zoom['konkit-dots']).toBe(7);
  updateLayers(fake.map, { ...input, points: [], colorMode: 'progress' });
  expect((fake.data['konkit-points'] as { features: unknown[] }).features).toHaveLength(0);
  expect(fake.zoom['konkit-dots']).toBe(0);
  expect(fake.zoom['konkit-clusters']).toBe(0);
  expect(fake.paint['konkit-zone-line.line-dasharray']).toEqual([2, 2]);
});
