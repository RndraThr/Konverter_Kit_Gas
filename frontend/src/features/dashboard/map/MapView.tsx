import { useCallback, useEffect, useRef, useState } from 'react';
import type maplibregl from 'maplibre-gl';
import { MapCanvas } from './MapCanvas';
import { applyBasemap } from './basemaps';
import { bindLayerEvents, installLayers, updateLayers, type LayerHandlers, type LayerInput } from './layers';
import type { FitPadding } from './fitTargets';
import type { BasemapMode, Bounds, MapConfig } from './types';

// A camera move the page asks for. A new object (new `id`) is a new request.
export type CameraRequest =
  | { id: number; kind: 'fit'; bounds: Bounds; padding: FitPadding }
  | { id: number; kind: 'fly'; center: [number, number]; zoom: number }
  | { id: number; kind: 'jump'; center: [number, number]; zoom: number };

// Highest zoom fitBounds may reach, so a tight cluster of points still shows its surroundings.
const FIT_MAX_ZOOM = 14;

type Props = LayerHandlers & {
  config: MapConfig;
  basemap: BasemapMode;
  layers: LayerInput;
  camera: CameraRequest | null;
  onMove: (center: [number, number], zoom: number) => void;
  onStyleError: () => void;
};

/** MapCanvas plus the konkit data layers, basemap switching and camera requests. */
export function MapView({ config, basemap, layers, camera, onMove, onStyleError, onPoint, onSite }: Props) {
  const [map, setMap] = useState<maplibregl.Map | null>(null);
  const latest = useRef({ layers, basemap, config, onMove, handlers: { onPoint, onSite } });
  useEffect(() => { latest.current = { layers, basemap, config, onMove, handlers: { onPoint, onSite } }; });

  const onReady = useCallback((ready: maplibregl.Map) => {
    const current = latest.current;
    installLayers(ready, current.layers);
    bindLayerEvents(ready, () => latest.current.handlers);
    // `ready` is the map MapCanvas just marked style-ready, so the basemap applies immediately.
    applyBasemap(ready, current.basemap, current.config);
    ready.on('moveend', () => {
      const center = ready.getCenter();
      latest.current.onMove([center.lng, center.lat], ready.getZoom());
    });
    setMap(ready);
  }, []);

  useEffect(() => { if (map) updateLayers(map, layers); }, [map, layers]);
  useEffect(() => { if (map) applyBasemap(map, basemap, config); }, [map, basemap, config]);
  useEffect(() => {
    if (!map || !camera) return;
    if (camera.kind === 'fit') map.fitBounds(camera.bounds, { padding: camera.padding, maxZoom: FIT_MAX_ZOOM });
    else if (camera.kind === 'fly') map.flyTo({ center: camera.center, zoom: camera.zoom });
    else map.jumpTo({ center: camera.center, zoom: camera.zoom });
  }, [map, camera]);

  // MapLibre's own stylesheet forces `position: relative` on the element it owns, so the sizing
  // wrapper is a separate div and the map div just fills it.
  return <div className="absolute inset-0">
    <MapCanvas key={`${config.style_url}|${config.attribution}`} config={config} onReady={onReady} onStyleError={onStyleError} className="h-full w-full" />
  </div>;
}
