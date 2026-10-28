import type maplibregl from 'maplibre-gl';
import type { BasemapMode, MapConfig } from './types';

export const OWN_PREFIX = 'konkit-';
export const IMAGERY_LAYER = 'konkit-img';
export const overlayLayerId = (index: number) => `konkit-hyb-${index}`;

// Appends the provider key as a query parameter; unchanged when the key is empty.
export function withApiKey(url: string, key: string): string {
  if (!key) return url;
  return `${url}${url.includes('?') ? '&' : '?'}key=${encodeURIComponent(key)}`;
}

function rasterSource(tiles: string, attribution?: string): maplibregl.RasterSourceSpecification {
  return { type: 'raster', tiles: [tiles], tileSize: 256, maxzoom: 19, ...(attribution ? { attribution } : {}) };
}

// Raster layers are inserted below the first of our own data layers so dots and
// zones stay on top, regardless of when the basemap is applied.
function ensureRaster(map: maplibregl.Map, id: string, tiles: string, key: string) {
  if (map.getLayer(id)) return;
  if (!map.getSource(id)) map.addSource(id, rasterSource(withApiKey(tiles, key)));
  const firstData = (map.getStyle().layers ?? []).find((l) => l.id.startsWith(OWN_PREFIX) && l.type !== 'raster');
  map.addLayer({ id, type: 'raster', source: id, layout: { visibility: 'none' } }, firstData?.id);
}

// Switches the basemap purely through layer visibility. Layers whose id starts
// with `konkit-` (our data) are never touched, except our own raster layers.
// Maps whose first `load` has fired. After that, applyBasemap applies immediately:
// isStyleLoaded() flips back to false whenever tiles load, so it cannot gate calls.
const ready = new WeakSet<maplibregl.Map>();
// Before the first load the call is deferred; only the latest request is applied.
const pending = new WeakMap<maplibregl.Map, { mode: BasemapMode; config: MapConfig }>();

export function markStyleReady(map: maplibregl.Map): void {
  ready.add(map);
}

export function applyBasemap(map: maplibregl.Map, mode: BasemapMode, config: MapConfig): void {
  if (!ready.has(map)) {
    if (map.getStyle() && map.isStyleLoaded()) {
      ready.add(map);
    } else {
      const waiting = pending.has(map);
      pending.set(map, { mode, config });
      if (!waiting) {
        map.once('load', () => {
          ready.add(map);
          const latest = pending.get(map);
          pending.delete(map);
          if (latest) applyBasemap(map, latest.mode, latest.config);
        });
      }
      return;
    }
  }
  const key = config.api_key;
  ensureRaster(map, IMAGERY_LAYER, config.satellite_tiles, key);
  config.hybrid_overlay_tiles.forEach((tiles, i) => ensureRaster(map, overlayLayerId(i), tiles, key));

  const visible = (on: boolean) => (on ? 'visible' : 'none');
  for (const layer of map.getStyle().layers ?? []) {
    if (!layer.id.startsWith(OWN_PREFIX)) map.setLayoutProperty(layer.id, 'visibility', visible(mode === 'map'));
  }
  map.setLayoutProperty(IMAGERY_LAYER, 'visibility', visible(mode !== 'map'));
  config.hybrid_overlay_tiles.forEach((_, i) => map.setLayoutProperty(overlayLayerId(i), 'visibility', visible(mode === 'hyb')));
}
