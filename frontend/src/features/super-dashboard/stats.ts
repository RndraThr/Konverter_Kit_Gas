import type { DashboardDay, DashboardStages } from './types';

/** Part of the quota in whole percent; 0 when there is no quota. */
export function percent(part: number, quota: number): number {
  return quota > 0 ? Math.round((part / quota) * 100) : 0;
}

/** Handovers per day with the running total. */
export function cumulative(days: DashboardDay[]): (DashboardDay & { total: number })[] {
  let total = 0;
  return days.map((day) => ({ ...day, total: (total += day.completed) }));
}

export function zoneLabel(name: string): string {
  return name || 'Tanpa zona';
}

export type StageKey = Exclude<keyof DashboardStages, 'quota'>;

/**
 * The stages after the quota, in order. One green ordinal ramp, light to
 * dark (validated: monotone lightness, light end 2.25:1 on the surface).
 */
export const stages: { key: StageKey; label: string; color: string }[] = [
  { key: 'linked', label: 'Tertaut', color: '#7bb88d' },
  { key: 'completed', label: 'Diserahkan', color: '#4a9663' },
  { key: 'approved', label: 'Foto disetujui', color: '#2f7d4a' },
  { key: 'ba_final', label: 'BA final', color: '#1b4a2b' },
];

export const formatNumber = (value: number) => value.toLocaleString('id-ID');
