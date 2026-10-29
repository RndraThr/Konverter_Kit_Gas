import { normalizeRegencyName, type RegencyAsset } from '../mapRegions';
import type { MapRegion } from '../types';
import type { Bounds, PointCollection, PointFeature, ZoneGeometry } from './types';

export const INDONESIA_BOUNDS: Bounds = [[94, -11.5], [141.5, 6.5]];
export const FIT_PADDING = 60;
// Width the desktop detail panel (Task 7) covers on the left of the map.
export const DETAIL_PANEL_WIDTH = 360;
const DESKTOP_MIN_WIDTH = 768;
// Half-size (degrees, ~1 km) of the box around a lone point so fitBounds does not zoom to the max.
const SINGLE_POINT_PAD = 0.01;

type Extent = { w: number; s: number; e: number; n: number };
const emptyExtent = (): Extent => ({ w: Infinity, s: Infinity, e: -Infinity, n: -Infinity });

function extend(extent: Extent, lng: number, lat: number) {
  if (lng < extent.w) extent.w = lng;
  if (lng > extent.e) extent.e = lng;
  if (lat < extent.s) extent.s = lat;
  if (lat > extent.n) extent.n = lat;
}

function toBounds(extent: Extent): Bounds | null {
  if (!Number.isFinite(extent.w) || !Number.isFinite(extent.s)) return null;
  return [[extent.w, extent.s], [extent.e, extent.n]];
}

export function ringsBounds(rings: number[][][]): Bounds | null {
  const extent = emptyExtent();
  for (const ring of rings) for (const [lng, lat] of ring) extend(extent, lng, lat);
  return toBounds(extent);
}

export function geometryBounds(geometry: ZoneGeometry): Bounds | null {
  return ringsBounds(geometry.type === 'Polygon' ? geometry.coordinates : geometry.coordinates.flat());
}

export function unionBounds(list: Array<Bounds | null | undefined>): Bounds | null {
  const extent = emptyExtent();
  for (const bounds of list) {
    if (!bounds) continue;
    extend(extent, bounds[0][0], bounds[0][1]);
    extend(extent, bounds[1][0], bounds[1][1]);
  }
  return toBounds(extent);
}

export function pointsBounds(points: PointFeature[]): Bounds | null {
  const extent = emptyExtent();
  for (const point of points) extend(extent, point.geometry.coordinates[0], point.geometry.coordinates[1]);
  const bounds = toBounds(extent);
  if (!bounds) return null;
  const [[w, s], [e, n]] = bounds;
  if (w === e && s === n) return [[w - SINGLE_POINT_PAD, s - SINGLE_POINT_PAD], [e + SINGLE_POINT_PAD, n + SINGLE_POINT_PAD]];
  return bounds;
}

/** Bounding boxes of every boundary-asset regency, keyed by its display name. */
export function assetRegencyBounds(asset: RegencyAsset): Array<{ name: string; key: string; bounds: Bounds }> {
  const out: Array<{ name: string; key: string; bounds: Bounds }> = [];
  for (const feature of asset.regencies) {
    const bounds = ringsBounds(feature.r);
    if (bounds) out.push({ name: feature.n, key: normalizeRegencyName(feature.n), bounds });
  }
  return out;
}

/** Master regency id -> bounds, matched to the boundary asset by normalised name. */
export function regencyBoundsIndex(asset: RegencyAsset, regions: Pick<MapRegion, 'regency_id' | 'regency_name'>[]): Map<string, Bounds> {
  const byKey = new Map(assetRegencyBounds(asset).map((item) => [item.key, item.bounds]));
  const index = new Map<string, Bounds>();
  for (const region of regions) {
    const bounds = byKey.get(normalizeRegencyName(region.regency_name));
    if (bounds) index.set(region.regency_id, bounds);
  }
  return index;
}

/**
 * Where the map opens: the located points; otherwise the regencies in the user's scope (a field
 * officer whose regency has no GPS photo yet still lands on their own kabupaten); otherwise Indonesia.
 */
export function initialFitTarget(points: PointCollection, scopeRegencyIds: string[], boundaries: Map<string, Bounds>): Bounds {
  return pointsBounds(points.features)
    ?? unionBounds(scopeRegencyIds.map((id) => boundaries.get(id)))
    ?? INDONESIA_BOUNDS;
}

export type FitPadding = { top: number; right: number; bottom: number; left: number };

/** 60 px on every side; on desktop the left side also clears the detail panel when it is open. */
export function fitPadding(viewportWidth: number, panelWidth: number): FitPadding {
  const left = viewportWidth >= DESKTOP_MIN_WIDTH ? FIT_PADDING + panelWidth : FIT_PADDING;
  return { top: FIT_PADDING, right: FIT_PADDING, bottom: FIT_PADDING, left };
}
