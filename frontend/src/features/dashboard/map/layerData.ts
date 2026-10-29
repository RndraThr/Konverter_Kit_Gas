import { EMPTY_REGION_COLOR, colorForValue, metricByKey, metricMax, normalizeRegencyName, type MetricKey, type RegencyAsset } from '../mapRegions';
import type { MapRegion } from '../types';
import { MILESTONES } from './milestones';
import type { ColorMode, PointFeature, ZoneCollection, ZoneGeometry } from './types';

// Categorical zone palette: hues are far apart and lightness alternates, so neighbouring zones
// stay distinguishable in greyscale and for colour-blind users.
export const ZONE_PALETTE = ['#3b5ba5', '#e8a33d', '#7a2e7e', '#86cbd9', '#b5452f', '#f3d36b', '#22706f', '#e898bb'];
export const PLACEHOLDER_ZONE_COLOR = '#9aa39c';
export const PLACEHOLDER_ZONE_LABEL = 'Belum dizonasi';

export const milestoneColor = (stage: number) => MILESTONES[Math.min(5, Math.max(1, stage)) - 1].color;

type Feature<G, P> = { type: 'Feature'; geometry: G; properties: P };
export type Collection<G, P> = { type: 'FeatureCollection'; features: Feature<G, P>[] };
type PointGeometry = { type: 'Point'; coordinates: [number, number] };
type MultiPolygonGeometry = { type: 'MultiPolygon'; coordinates: number[][][][] };

export type DotProperties = { slot_id: string; code: string; site_id: string; zone_id: string; stage: number; color: string; review: boolean };
export type SiteProperties = { site_id: string; site_name: string; count: number; stage: number; color: string; review: boolean; label: string };
export type RegionFillProperties = { regency_id: string; name: string; color: string };
export type ZoneFillProperties = { zone_id: string; name: string; color: string; is_placeholder: boolean };
export type ZoneLabelProperties = { zone_id: string; name: string; count: number; summary: string };

export type RegionFillCollection = Collection<MultiPolygonGeometry, RegionFillProperties>;

export type LayerInput = {
  /** Points already passed through the timeline and milestone filters. */
  points: PointFeature[];
  zones: ZoneCollection | null;
  regions: RegionFillCollection;
  colorMode: ColorMode;
  /** Outline to highlight (selected regency or zone). */
  focus?: ZoneGeometry | null;
};

export type LayerData = {
  points: Collection<PointGeometry, DotProperties>;
  sites: Collection<PointGeometry, SiteProperties>;
  /** Zone mode with zones loaded: the zone polygons; otherwise the regency fills. */
  regions: Collection<ZoneGeometry, { name: string; color: string }>;
  zones: Collection<ZoneGeometry, ZoneFillProperties>;
  zoneLabels: Collection<PointGeometry, ZoneLabelProperties>;
  focus: Collection<ZoneGeometry, Record<string, never>>;
};

export const needsReview = (point: PointFeature) => point.properties.site_outlier || point.properties.location_status === 'outside';
export const isNamedSite = (siteId: string) => siteId.startsWith('hl:');

function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

/** Groups points by `site_id`. */
export function groupSites(points: PointFeature[]): Map<string, PointFeature[]> {
  const groups = new Map<string, PointFeature[]>();
  for (const point of points) {
    const members = groups.get(point.properties.site_id);
    if (members) members.push(point); else groups.set(point.properties.site_id, [point]);
  }
  return groups;
}

/**
 * One marker per handover site (`hl:`) or per stacked `geo:` group of two or more slots. The marker
 * takes the LOWEST milestone of its members so unfinished work stays visible (spec §4.3).
 */
export function buildSites(points: PointFeature[]): { sites: LayerData['sites']; memberSlotIds: Set<string> } {
  const features: LayerData['sites']['features'] = [];
  const memberSlotIds = new Set<string>();
  for (const [siteId, members] of groupSites(points)) {
    const named = isNamedSite(siteId);
    if (!named && members.length < 2) continue;
    const stage = Math.min(...members.map((member) => member.properties.stage));
    const siteName = members[0].properties.site_name;
    const count = members.length;
    const label = named && siteName ? (count > 1 ? `${siteName} (${count})` : siteName) : String(count);
    for (const member of members) memberSlotIds.add(member.properties.slot_id);
    features.push({
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [median(members.map((m) => m.geometry.coordinates[0])), median(members.map((m) => m.geometry.coordinates[1]))] },
      properties: { site_id: siteId, site_name: siteName, count, stage, color: milestoneColor(stage), review: members.some(needsReview), label },
    });
  }
  return { sites: { type: 'FeatureCollection', features }, memberSlotIds };
}

/** zone_id -> fill colour. Placeholder zones are grey; the rest cycle through the palette in order. */
export function zoneColorIndex(zones: ZoneCollection | null): Map<string, string> {
  const index = new Map<string, string>();
  let next = 0;
  for (const zone of zones?.features ?? []) {
    if (zone.properties.is_placeholder) index.set(zone.properties.zone_id, PLACEHOLDER_ZONE_COLOR);
    else index.set(zone.properties.zone_id, ZONE_PALETTE[next++ % ZONE_PALETTE.length]);
  }
  return index;
}

const metricForMode: Record<Exclude<ColorMode, 'zone'>, MetricKey> = { evidence: 'evidence', progress: 'progress', recipients: 'recipients' };

/** Regency polygons from the boundary asset, coloured by zone or by the chosen metric. */
export function buildRegionFills(asset: RegencyAsset | null, regions: MapRegion[], colorMode: ColorMode, zoneColors: Map<string, string>): RegionFillCollection {
  if (!asset) return { type: 'FeatureCollection', features: [] };
  const byKey = new Map(regions.map((region) => [normalizeRegencyName(region.regency_name), region]));
  const metric = colorMode === 'zone' ? null : metricByKey(metricForMode[colorMode]);
  const max = metric ? metricMax(metric, regions) : 1;
  const features: RegionFillCollection['features'] = [];
  for (const regency of asset.regencies) {
    const region = byKey.get(normalizeRegencyName(regency.n));
    if (!region) continue;
    const color = metric
      ? colorForValue(metric.value(region), max, metric.hasData(region))
      : (region.zone_id && zoneColors.get(region.zone_id)) || EMPTY_REGION_COLOR;
    features.push({
      type: 'Feature',
      geometry: { type: 'MultiPolygon', coordinates: regency.r.map((ring) => [ring]) },
      properties: { regency_id: region.regency_id, name: region.regency_name, color },
    });
  }
  return { type: 'FeatureCollection', features };
}

// Label anchor: centre of the bounding box of the zone's largest part.
function labelAnchor(geometry: ZoneGeometry): [number, number] | null {
  const polygons = geometry.type === 'Polygon' ? [geometry.coordinates] : geometry.coordinates;
  let best: [number, number] | null = null;
  let bestArea = -1;
  for (const polygon of polygons) {
    const ring = polygon[0];
    if (!ring?.length) continue;
    const lngs = ring.map((c) => c[0]);
    const lats = ring.map((c) => c[1]);
    const [w, e, s, n] = [Math.min(...lngs), Math.max(...lngs), Math.min(...lats), Math.max(...lats)];
    const area = (e - w) * (n - s);
    if (area > bestArea) { bestArea = area; best = [(w + e) / 2, (s + n) / 2]; }
  }
  return best;
}

export function buildLayerData(input: LayerInput): LayerData {
  const { sites, memberSlotIds } = buildSites(input.points);
  const dots: LayerData['points']['features'] = [];
  const perZone = new Map<string, number>();
  for (const point of input.points) {
    const p = point.properties;
    perZone.set(p.zone_id, (perZone.get(p.zone_id) ?? 0) + 1);
    if (memberSlotIds.has(p.slot_id)) continue;
    dots.push({
      type: 'Feature', geometry: point.geometry,
      properties: { slot_id: p.slot_id, code: p.code, site_id: p.site_id, zone_id: p.zone_id, stage: p.stage, color: milestoneColor(p.stage), review: needsReview(point) },
    });
  }

  const colors = zoneColorIndex(input.zones);
  const zones: LayerData['zones']['features'] = [];
  const zoneLabels: LayerData['zoneLabels']['features'] = [];
  for (const zone of input.zones?.features ?? []) {
    const { zone_id, is_placeholder } = zone.properties;
    const name = is_placeholder ? PLACEHOLDER_ZONE_LABEL : zone.properties.name;
    zones.push({ type: 'Feature', geometry: zone.geometry, properties: { zone_id, name, color: colors.get(zone_id) ?? PLACEHOLDER_ZONE_COLOR, is_placeholder } });
    const anchor = labelAnchor(zone.geometry);
    const count = perZone.get(zone_id) ?? 0;
    if (anchor) zoneLabels.push({ type: 'Feature', geometry: { type: 'Point', coordinates: anchor }, properties: { zone_id, name, count, summary: `${name}\n${count} titik` } });
  }

  return {
    points: { type: 'FeatureCollection', features: dots },
    sites,
    regions: input.colorMode === 'zone' && zones.length > 0 ? { type: 'FeatureCollection', features: zones } : input.regions,
    zones: { type: 'FeatureCollection', features: zones },
    zoneLabels: { type: 'FeatureCollection', features: zoneLabels },
    focus: { type: 'FeatureCollection', features: input.focus ? [{ type: 'Feature', geometry: input.focus, properties: {} }] : [] },
  };
}
