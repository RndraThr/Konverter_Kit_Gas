import type { MapRegion } from './types';

// Boundary asset shape produced by the simplification step that bundles geoBoundaries IDN ADM2.
// Short keys keep the bundled file small; `c` is the bounding-box centre in [lon, lat].
export type RegencyFeature = { n: string; c: [number, number]; r: number[][][] };
export type RegencyAsset = { source: string; license: string; licenseSource?: string; regencies: RegencyFeature[] };

/**
 * The boundary asset names regencies "Bangka Barat" while the regency master stores
 * "KAB. BANGKA BARAT"/"KOTA PADANG PANJANG", so both sides reduce to one comparable key. Kabupaten
 * prefixes are absent from the boundary asset and can be removed. The KOTA prefix must remain,
 * because Indonesia has multiple city/regency pairs with the same base name (for example Bandung).
 */
export function normalizeRegencyName(name: string): string {
  return name
    .toUpperCase()
    .replace(/^(KABUPATEN|KAB\.?)\s+/, '')
    .replace(/[^A-Z0-9]/g, '');
}

export function buildRegionIndex(regions: MapRegion[]): Map<string, MapRegion> {
  const index = new Map<string, MapRegion>();
  for (const region of regions) index.set(normalizeRegencyName(region.regency_name), region);
  return index;
}

export type MetricKey = 'recipients' | 'progress' | 'evidence';

export type MetricDefinition = {
  key: MetricKey;
  label: string;
  description: string;
  /** True when the metric is a 0..1 ratio rather than a raw count. */
  ratio: boolean;
  value: (region: MapRegion) => number;
  format: (region: MapRegion) => string;
  hasData: (region: MapRegion) => boolean;
};

const share = (part: number, whole: number) => (whole > 0 ? part / whole : 0);
const numberFormat = new Intl.NumberFormat('id-ID');

/** Quota is the intended scale, but a schedule may not set one; fall back to the slots created. */
export function progressDenominator(region: MapRegion): number {
  const created = region.slots_open + region.slots_linked + region.slots_completed + region.slots_cancelled;
  return Math.max(region.slot_quota, created);
}

export const mapMetrics: MetricDefinition[] = [
  {
    key: 'recipients',
    label: 'Sebaran penerima',
    description: 'Jumlah penerima aktif pada setiap kabupaten/kota.',
    ratio: false,
    value: (region) => region.recipients,
    format: (region) => `${numberFormat.format(region.recipients)} penerima`,
    hasData: (region) => region.recipients > 0,
  },
  {
    key: 'progress',
    label: 'Progres distribusi',
    description: 'Bagian nomor bagi yang sudah selesai terhadap kuota jadwal.',
    ratio: true,
    value: (region) => share(region.slots_completed, progressDenominator(region)),
    format: (region) => `${Math.round(share(region.slots_completed, progressDenominator(region)) * 100)}% selesai · ${region.slots_completed}/${progressDenominator(region)} nomor`,
    hasData: (region) => progressDenominator(region) > 0,
  },
  {
    key: 'evidence',
    label: 'Kelengkapan bukti',
    description: 'Bagian penerima yang seluruh dokumentasi wajibnya sudah lengkap.',
    ratio: true,
    value: (region) => share(region.evidence_complete, region.recipients),
    format: (region) => `${Math.round(share(region.evidence_complete, region.recipients) * 100)}% lengkap · ${region.evidence_complete}/${region.recipients} penerima`,
    hasData: (region) => region.recipients > 0,
  },
];

export function metricByKey(key: MetricKey): MetricDefinition {
  return mapMetrics.find((metric) => metric.key === key) ?? mapMetrics[0];
}

/** Sequential scale anchored on the application's primary green (#2f7d4a). */
export const EMPTY_REGION_COLOR = '#e5e7eb';
export const REGION_SCALE = ['#e8f3ec', '#bcdcc8', '#86c19f', '#4f9c72', '#2f7d4a'];

export function metricMax(metric: MetricDefinition, regions: MapRegion[]): number {
  if (metric.ratio) return 1;
  return Math.max(1, ...regions.map((region) => metric.value(region)));
}

export function colorForValue(value: number, max: number, hasData: boolean): string {
  if (!hasData) return EMPTY_REGION_COLOR;
  if (max <= 0) return REGION_SCALE[0];
  const step = Math.ceil((value / max) * REGION_SCALE.length) - 1;
  return REGION_SCALE[Math.min(REGION_SCALE.length - 1, Math.max(0, step))];
}

export type ProjectedRegency = { key: string; name: string; path: string; center: [number, number] };

/**
 * Projects regency rings into SVG coordinates. Equirectangular with a uniform scale is enough for a
 * dashboard outline map: shapes stay recognisable and no tile server or network access is involved.
 * Coordinates are rounded to one decimal because the extra precision is invisible at this size and
 * would multiply the DOM size across ~25k points.
 */
export function buildRegencyPaths(features: RegencyFeature[], width: number, height: number, padding = 6): ProjectedRegency[] {
  let minLon = Infinity;
  let maxLon = -Infinity;
  let minLat = Infinity;
  let maxLat = -Infinity;
  for (const feature of features) {
    for (const ring of feature.r) {
      for (const [lon, lat] of ring) {
        if (lon < minLon) minLon = lon;
        if (lon > maxLon) maxLon = lon;
        if (lat < minLat) minLat = lat;
        if (lat > maxLat) maxLat = lat;
      }
    }
  }
  if (!Number.isFinite(minLon) || !Number.isFinite(minLat)) return [];

  const lonSpan = Math.max(maxLon - minLon, 1e-6);
  const latSpan = Math.max(maxLat - minLat, 1e-6);
  const scale = Math.min((width - padding * 2) / lonSpan, (height - padding * 2) / latSpan);
  const offsetX = (width - lonSpan * scale) / 2;
  const offsetY = (height - latSpan * scale) / 2;
  const project = (lon: number, lat: number): [number, number] => [
    offsetX + (lon - minLon) * scale,
    offsetY + (maxLat - lat) * scale,
  ];

  return features.map((feature) => {
    const path = feature.r
      .map((ring) => `M${ring.map(([lon, lat]) => project(lon, lat).map((value) => value.toFixed(1)).join(',')).join('L')}Z`)
      .join('');
    const [cx, cy] = project(feature.c[0], feature.c[1]);
    return {
      key: normalizeRegencyName(feature.n),
      name: feature.n,
      path,
      center: [Number(cx.toFixed(1)), Number(cy.toFixed(1))] as [number, number],
    };
  });
}

export function formatRegencyLabel(region: MapRegion): string {
  return region.regency_name || region.regency_id;
}
