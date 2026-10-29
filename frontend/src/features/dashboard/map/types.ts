export type BasemapMode = 'map' | 'sat' | 'hyb';
export type ColorMode = 'zone' | 'evidence' | 'progress' | 'recipients';

// Mirrors GET /api/v1/map/config (inner `data` object).
export type MapConfig = {
  style_url: string;
  satellite_tiles: string;
  hybrid_overlay_tiles: string[];
  attribution: string;
  api_key: string;
};

export type MapUrlState = {
  center?: [number, number];
  zoom?: number;
  base: BasemapMode;
  color: ColorMode;
  slot?: string;
  // Only `hl:`-prefixed (handover location) ids are kept in the URL.
  site?: string;
  stage?: number;
};

// GET /api/v1/map/points (inner `data`). `code` is the slot number ("nomor bagi").
export type LocationStatus = 'inside' | 'outside' | 'unmatched';
export type PointProperties = {
  slot_id: string;
  code: string;
  schedule_id: string;
  regency_id: string;
  zone_id: string;
  site_id: string;
  site_name: string;
  site_outlier: boolean;
  stage: number;
  stage_at: string[];
  location_status: LocationStatus;
};
export type PointFeature = { type: 'Feature'; geometry: { type: 'Point'; coordinates: [number, number] }; properties: PointProperties };
export type PointMeta = { total: number; located: number; unlocated: number; outside: number };
export type PointCollection = { type: 'FeatureCollection'; features: PointFeature[]; meta?: PointMeta };

// GET /api/v1/map/zones?program_id= (inner `data`).
export type ZoneProperties = { zone_id: string; code: string; name: string; is_placeholder: boolean; regency_count: number };
export type ZoneGeometry = { type: 'Polygon'; coordinates: number[][][] } | { type: 'MultiPolygon'; coordinates: number[][][][] };
export type ZoneFeature = { type: 'Feature'; geometry: ZoneGeometry; properties: ZoneProperties };
export type ZoneCollection = { type: 'FeatureCollection'; features: ZoneFeature[] };

// GET /api/v1/map/regencies/{id}/boundary (inner `data`).
export type RegencyBoundary = { type: 'Feature'; geometry: ZoneGeometry; properties: { regency_id: string; name: string } };

// [[west, south], [east, north]]
export type Bounds = [[number, number], [number, number]];

// What the page has selected on the map. Task 7 renders the detail panel from it.
// A `hl:` site id is mirrored to the URL; a `geo:` site lives only in component state.
export type MapSelection = { kind: 'point'; slotId: string } | { kind: 'site'; siteId: string };
