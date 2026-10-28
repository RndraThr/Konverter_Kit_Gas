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
