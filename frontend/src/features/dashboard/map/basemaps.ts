import type maplibregl from 'maplibre-gl';
import type { BasemapMode, MapConfig } from './types';

export const OWN_PREFIX = 'konkit-';
export const IMAGERY_LAYER = 'konkit-img';
export const overlayLayerId = (index: number) => `konkit-hyb-${index}`;

function rasterSource(tiles: string, attribution?: string): maplibregl.RasterSourceSpecification {
  return { type: 'raster', tiles: [tiles], tileSize: 256, maxzoom: 19, ...(attribution ? { attribution } : {}) };
}

// Raster layers are inserted below the first of our own data layers so dots and
// zones stay on top, regardless of when the basemap is applied.
function ensureRaster(map: maplibregl.Map, id: string, tiles: string) {
  if (map.getLayer(id)) return;
  if (!map.getSource(id)) map.addSource(id, rasterSource(tiles));
  const firstData = (map.getStyle().layers ?? []).find((l) => l.id.startsWith(OWN_PREFIX) && l.type !== 'raster');
  map.addLayer({ id, type: 'raster', source: id, layout: { visibility: 'none' } }, firstData?.id);
}

// Switches the basemap purely through layer visibility. Layers whose id starts
// with `konkit-` (our data) are never touched, except our own raster layers.
export function applyBasemap(map: maplibregl.Map, mode: BasemapMode, config: MapConfig): void {
  ensureRaster(map, IMAGERY_LAYER, config.satellite_tiles);
  config.hybrid_overlay_tiles.forEach((tiles, i) => ensureRaster(map, overlayLayerId(i), tiles));

  const visible = (on: boolean) => (on ? 'visible' : 'none');
  for (const layer of map.getStyle().layers ?? []) {
    if (!layer.id.startsWith(OWN_PREFIX)) map.setLayoutProperty(layer.id, 'visibility', visible(mode === 'map'));
  }
  map.setLayoutProperty(IMAGERY_LAYER, 'visibility', visible(mode !== 'map'));
  config.hybrid_overlay_tiles.forEach((_, i) => map.setLayoutProperty(overlayLayerId(i), 'visibility', visible(mode === 'hyb')));
}
