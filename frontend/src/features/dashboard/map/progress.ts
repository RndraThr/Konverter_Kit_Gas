import type { MapRegion } from '../types';
import type { PointFeature } from './types';

export type StageCounts = [number, number, number, number, number];
export type ProgressRow = { regency_id: string; name: string; counts: StageCounts; total: number };
export type ProgressGroup = { key: string; name: string; rows: ProgressRow[]; counts: StageCounts; total: number };

export const NO_ZONE_LABEL = 'Tanpa zona';
export const ALL_REGENCIES_LABEL = 'Semua kabupaten';

const zeros = (): StageCounts => [0, 0, 0, 0, 0];

/** Number of points per milestone (index 0 = M1). */
export function stageCounts(points: PointFeature[]): StageCounts {
  const counts = zeros();
  for (const point of points) {
    const stage = point.properties.stage;
    if (stage >= 1 && stage <= 5) counts[stage - 1] += 1;
  }
  return counts;
}

/**
 * Milestone progress per regency, grouped per zone with subtotals (zones in code order, regencies
 * without a zone last). Without a program there are no zones, so everything is one group.
 */
export function progressByZone(points: PointFeature[], regions: MapRegion[], hasProgram: boolean): ProgressGroup[] {
  const pointsByRegency = new Map<string, PointFeature[]>();
  for (const point of points) {
    const list = pointsByRegency.get(point.properties.regency_id);
    if (list) list.push(point); else pointsByRegency.set(point.properties.regency_id, [point]);
  }

  const groups = new Map<string, ProgressGroup & { code: string }>();
  for (const region of regions) {
    const regencyPoints = pointsByRegency.get(region.regency_id) ?? [];
    if (regencyPoints.length === 0 && region.recipients === 0 && region.slot_quota === 0) continue;
    const zoned = hasProgram && Boolean(region.zone_id);
    const key = zoned ? region.zone_id! : '';
    const name = zoned ? region.zone_name || region.zone_code || NO_ZONE_LABEL : hasProgram ? NO_ZONE_LABEL : ALL_REGENCIES_LABEL;
    let group = groups.get(key);
    if (!group) {
      group = { key, name, code: zoned ? region.zone_code ?? '' : '￿', rows: [], counts: zeros(), total: 0 };
      groups.set(key, group);
    }
    const counts = stageCounts(regencyPoints);
    group.rows.push({ regency_id: region.regency_id, name: region.regency_name, counts, total: regencyPoints.length });
    counts.forEach((count, i) => { group.counts[i] += count; });
    group.total += regencyPoints.length;
  }

  return [...groups.values()]
    .sort((a, b) => a.code.localeCompare(b.code, 'id') || a.name.localeCompare(b.name, 'id'))
    .map(({ code: _code, ...group }) => ({ ...group, rows: group.rows.sort((a, b) => b.total - a.total || a.name.localeCompare(b.name, 'id')) }));
}
