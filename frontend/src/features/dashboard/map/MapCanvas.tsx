import { useEffect, useRef } from 'react';
import type maplibregl from 'maplibre-gl';
import { withApiKey } from './basemaps';
import type { MapConfig } from './types';

export const STYLE_LOAD_TIMEOUT_MS = 8000;

type Props = {
  config: MapConfig;
  onReady: (map: maplibregl.Map) => void;
  onStyleError: () => void;
  className?: string;
};

type MapErrorEvent = { error?: unknown; sourceId?: string; tile?: unknown };

// Owns the MapLibre instance. maplibre-gl is imported dynamically so it ships as
// its own chunk. Data layers are added by the caller from `onReady`.
export function MapCanvas({ config, onReady, onStyleError, className }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const onReadyRef = useRef(onReady);
  const onStyleErrorRef = useRef(onStyleError);
  useEffect(() => {
    onReadyRef.current = onReady;
    onStyleErrorRef.current = onStyleError;
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    let disposed = false;
    let settled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let observer: ResizeObserver | undefined;
    let map: maplibregl.Map | undefined;

    const fail = () => {
      if (settled || disposed) return;
      settled = true;
      clearTimeout(timer);
      onStyleErrorRef.current();
    };

    void (async () => {
      let lib: typeof maplibregl;
      try {
        const [mod] = await Promise.all([import('maplibre-gl'), import('maplibre-gl/dist/maplibre-gl.css')]);
        lib = (mod as unknown as { default?: typeof maplibregl }).default ?? (mod as unknown as typeof maplibregl);
      } catch {
        fail();
        return;
      }
      if (disposed) return;

      map = new lib.Map({
        container,
        style: withApiKey(config.style_url, config.api_key),
        center: [118, -2.5],
        zoom: 4,
        attributionControl: { compact: true, customAttribution: config.attribution },
      });
      map.addControl(new lib.NavigationControl({ visualizePitch: true }), 'top-right');
      map.addControl(new lib.ScaleControl({ unit: 'metric' }), 'bottom-right');

      const created = map;
      observer = new ResizeObserver(() => created.resize());
      observer.observe(container);

      timer = setTimeout(fail, STYLE_LOAD_TIMEOUT_MS);
      created.on('load', () => {
        if (settled || disposed) return;
        settled = true;
        clearTimeout(timer);
        onReadyRef.current(created);
      });
      // Tile/source errors are routine on weak links; only a style-level error
      // before the first load means the basemap is unusable.
      created.on('error', (e: MapErrorEvent) => {
        if (settled || e.sourceId || e.tile) return;
        fail();
      });
    })();

    return () => {
      disposed = true;
      clearTimeout(timer);
      observer?.disconnect();
      map?.remove();
    };
    // The map is created once per style; config changes remount via key at the caller.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.style_url]);

  return <div ref={containerRef} className={className ?? 'h-full w-full'} />;
}
