import { normalizeRegencyName } from '../mapRegions';
import type { Bounds, PointFeature } from './types';

export type SearchRegency = { name: string; key: string; bounds: Bounds };
export type SearchResult =
  | { kind: 'regency'; id: string; label: string; detail: string; bounds: Bounds }
  | { kind: 'point'; id: string; label: string; detail: string; point: PointFeature };

const LIMIT = 6;

/**
 * Regencies anywhere in Indonesia (from the boundary asset) and slot numbers ("nomor bagi") of the
 * loaded points. Prefix matches rank before substring matches.
 */
export function searchMap(query: string, regencies: SearchRegency[], points: PointFeature[], regencyNames: Map<string, string>): SearchResult[] {
  const text = query.trim();
  if (!text) return [];
  const results: SearchResult[] = [];

  const key = normalizeRegencyName(text);
  if (key) {
    const prefix: SearchRegency[] = [];
    const inner: SearchRegency[] = [];
    for (const regency of regencies) {
      if (regency.key.startsWith(key)) prefix.push(regency);
      else if (regency.key.includes(key)) inner.push(regency);
    }
    for (const regency of [...prefix, ...inner].slice(0, LIMIT)) {
      results.push({ kind: 'regency', id: regency.key, label: regency.name, detail: 'Kabupaten/kota', bounds: regency.bounds });
    }
  }

  const code = text.replace(/^(nomor\s*bagi|no\.?)\s*/i, '').replace(/^#/, '');
  if (/^\d+$/.test(code)) {
    const exact = points.filter((point) => point.properties.code === code);
    const prefix = points.filter((point) => point.properties.code !== code && point.properties.code.startsWith(code));
    for (const point of [...exact, ...prefix].slice(0, LIMIT)) {
      results.push({
        kind: 'point', id: point.properties.slot_id, label: `Nomor bagi ${point.properties.code}`,
        detail: regencyNames.get(point.properties.regency_id) ?? 'Titik distribusi', point,
      });
    }
  }
  return results;
}
