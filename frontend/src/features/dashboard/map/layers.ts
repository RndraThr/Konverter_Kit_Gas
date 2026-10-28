import type maplibregl from 'maplibre-gl';
import { buildLayerData, type LayerData, type LayerInput } from './layerData';
import { MILESTONES, REVIEW_COLOR } from './milestones';

export type { LayerInput } from './layerData';

// Drawing order, bottom to top. Basemap raster layers are inserted below the first of these.
export const LAYER_IDS = [
  'konkit-regions',
  'konkit-zone-line',
  'konkit-focus',
  'konkit-clusters',
  'konkit-cluster-count',
  'konkit-dots',
  'konkit-sites',
  'konkit-site-label',
  'konkit-zone-label',
];

const SOURCES: Record<string, keyof LayerData> = {
  'konkit-regions': 'regions',
  'konkit-zones': 'zones',
  'konkit-zone-labels': 'zoneLabels',
  'konkit-focus': 'focus',
  'konkit-points': 'points',
  'konkit-sites': 'sites',
};

// Layers that draw individual points; hidden below this zoom in zone mode, where the zone labels
// show a per-zone summary instead.
const POINT_LAYERS = ['konkit-clusters', 'konkit-cluster-count', 'konkit-dots', 'konkit-sites', 'konkit-site-label'];
export const ZONE_SUMMARY_MAX_ZOOM = 7;

// Glyphs come from the basemap style (OpenFreeMap serves Noto Sans).
const FONT = ['Noto Sans Bold'];

const stageColor: maplibregl.ExpressionSpecification = [
  'match', ['get', 'minStage'],
  1, MILESTONES[0].color, 2, MILESTONES[1].color, 3, MILESTONES[2].color, 4, MILESTONES[3].color,
  MILESTONES[4].color,
];

const reviewStroke = (normal: string): maplibregl.ExpressionSpecification => ['case', ['to-boolean', ['get', 'review']], REVIEW_COLOR, normal];
const reviewWidth = (normal: number): maplibregl.ExpressionSpecification => ['case', ['to-boolean', ['get', 'review']], 3, normal];

function layerSpecs(): maplibregl.LayerSpecification[] {
  return [
    { id: 'konkit-regions', type: 'fill', source: 'konkit-regions', paint: { 'fill-color': ['get', 'color'], 'fill-opacity': 0.4, 'fill-outline-color': '#ffffff' } },
    { id: 'konkit-zone-line', type: 'line', source: 'konkit-zones', paint: { 'line-color': '#1f2a24', 'line-width': 2.5 } },
    { id: 'konkit-focus', type: 'line', source: 'konkit-focus', paint: { 'line-color': '#111827', 'line-width': 3 } },
    {
      id: 'konkit-clusters', type: 'circle', source: 'konkit-points', filter: ['has', 'point_count'],
      paint: {
        'circle-color': stageColor,
        'circle-radius': ['step', ['get', 'point_count'], 14, 25, 18, 100, 24],
        'circle-stroke-color': ['case', ['>', ['get', 'review'], 0], REVIEW_COLOR, '#ffffff'],
        'circle-stroke-width': ['case', ['>', ['get', 'review'], 0], 3, 1.5],
      },
    },
    {
      id: 'konkit-cluster-count', type: 'symbol', source: 'konkit-points', filter: ['has', 'point_count'],
      layout: { 'text-field': ['get', 'point_count_abbreviated'], 'text-font': FONT, 'text-size': 12 },
      paint: { 'text-color': ['case', ['>=', ['get', 'minStage'], 3], '#ffffff', '#143f24'] },
    },
    {
      id: 'konkit-dots', type: 'circle', source: 'konkit-points', filter: ['!', ['has', 'point_count']],
      paint: { 'circle-color': ['get', 'color'], 'circle-radius': 6, 'circle-stroke-color': reviewStroke('#ffffff'), 'circle-stroke-width': reviewWidth(1) },
    },
    {
      id: 'konkit-sites', type: 'circle', source: 'konkit-sites',
      paint: { 'circle-color': ['get', 'color'], 'circle-radius': ['step', ['get', 'count'], 9, 5, 12, 20, 15], 'circle-stroke-color': reviewStroke('#ffffff'), 'circle-stroke-width': reviewWidth(2) },
    },
    {
      id: 'konkit-site-label', type: 'symbol', source: 'konkit-sites',
      layout: { 'text-field': ['get', 'label'], 'text-font': FONT, 'text-size': 11, 'text-offset': [0, 1.6], 'text-anchor': 'top', 'text-optional': true },
      paint: { 'text-color': '#143f24', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
    },
    {
      id: 'konkit-zone-label', type: 'symbol', source: 'konkit-zone-labels',
      layout: { 'text-field': ['get', 'name'], 'text-font': FONT, 'text-size': 12 },
      paint: { 'text-color': '#1f2a24', 'text-halo-color': '#ffffff', 'text-halo-width': 1.5 },
    },
  ];
}

// Paint/layout/zoom that depend on the colour mode.
function applyMode(map: maplibregl.Map, input: LayerInput) {
  const zoneMode = input.colorMode === 'zone';
  const summary = zoneMode && (input.zones?.features.length ?? 0) > 0;
  for (const id of POINT_LAYERS) map.setLayerZoomRange(id, summary ? ZONE_SUMMARY_MAX_ZOOM : 0, 24);
  // Zone mode: thick solid outline. Other modes: thin dashed outline plus a small label.
  map.setPaintProperty('konkit-zone-line', 'line-width', zoneMode ? 2.5 : 1);
  map.setPaintProperty('konkit-zone-line', 'line-dasharray', zoneMode ? [1, 0] : [2, 2]);
  map.setLayoutProperty('konkit-zone-label', 'text-field', summary ? ['step', ['zoom'], ['get', 'summary'], ZONE_SUMMARY_MAX_ZOOM, ['get', 'name']] : ['get', 'name']);
  map.setLayoutProperty('konkit-zone-label', 'text-size', zoneMode ? 13 : 10);
}

/** Adds every konkit source and layer once (no-op for the ones that exist), then applies the data. */
export function installLayers(map: maplibregl.Map, input: LayerInput): void {
  const data = buildLayerData(input);
  for (const [sourceId, key] of Object.entries(SOURCES)) {
    if (map.getSource(sourceId)) continue;
    const spec: maplibregl.GeoJSONSourceSpecification = { type: 'geojson', data: data[key] as unknown as GeoJSON.GeoJSON };
    if (sourceId === 'konkit-points') {
      Object.assign(spec, {
        cluster: true, clusterRadius: 40, clusterMaxZoom: 14,
        // Clusters, like sites, take the lowest milestone of their members.
        clusterProperties: { minStage: ['min', ['get', 'stage']], review: ['+', ['case', ['to-boolean', ['get', 'review']], 1, 0]] },
      });
    }
    map.addSource(sourceId, spec);
  }
  for (const layer of layerSpecs()) {
    if (!map.getLayer(layer.id)) map.addLayer(layer);
  }
  updateLayers(map, input, data);
}

/** Pushes new data into the existing sources and re-applies mode-dependent styling. */
export function updateLayers(map: maplibregl.Map, input: LayerInput, prebuilt?: LayerData): void {
  const data = prebuilt ?? buildLayerData(input);
  for (const [sourceId, key] of Object.entries(SOURCES)) {
    const source = map.getSource(sourceId) as maplibregl.GeoJSONSource | undefined;
    source?.setData(data[key] as unknown as GeoJSON.GeoJSON);
  }
  applyMode(map, input);
}

export type LayerHandlers = {
  onPoint: (slotId: string) => void;
  onSite: (siteId: string) => void;
};

/** Click and hover behaviour of the point layers. Handlers are read through the getter on each event. */
export function bindLayerEvents(map: maplibregl.Map, handlers: () => LayerHandlers): void {
  map.on('click', 'konkit-dots', (event) => {
    const id = event.features?.[0]?.properties?.slot_id;
    if (typeof id === 'string') handlers().onPoint(id);
  });
  map.on('click', 'konkit-sites', (event) => {
    const id = event.features?.[0]?.properties?.site_id;
    if (typeof id === 'string') handlers().onSite(id);
  });
  map.on('click', 'konkit-clusters', (event) => {
    const feature = event.features?.[0];
    const clusterId = feature?.properties?.cluster_id;
    const source = map.getSource('konkit-points') as maplibregl.GeoJSONSource | undefined;
    if (!feature || typeof clusterId !== 'number' || !source) return;
    void source.getClusterExpansionZoom(clusterId).then((zoom) => {
      map.easeTo({ center: (feature.geometry as GeoJSON.Point).coordinates as [number, number], zoom });
    }).catch(() => undefined);
  });
  for (const id of ['konkit-dots', 'konkit-sites', 'konkit-clusters']) {
    map.on('mouseenter', id, () => { map.getCanvas().style.cursor = 'pointer'; });
    map.on('mouseleave', id, () => { map.getCanvas().style.cursor = ''; });
  }
}
