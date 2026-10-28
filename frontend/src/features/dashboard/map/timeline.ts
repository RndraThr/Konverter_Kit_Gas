import type { PointFeature } from './types';

const DAY_MS = 24 * 60 * 60 * 1000;

/** Milestone a point had reached at `day` (0 = it did not exist yet). `stage_at[i]` is milestone i+1. */
export function stageAtDay(point: PointFeature, day: Date): number {
  const limit = day.getTime();
  let stage = 0;
  for (const at of point.properties.stage_at) {
    if (new Date(at).getTime() <= limit) stage += 1;
    else break;
  }
  return stage;
}

const startOfDay = (time: number) => {
  const date = new Date(time);
  date.setHours(0, 0, 0, 0);
  return date;
};

export type TimelineRange = { start: Date; days: number };

/** Local calendar days from the earliest to the latest milestone time. */
export function timelineRange(points: PointFeature[]): TimelineRange | null {
  let min = Infinity;
  let max = -Infinity;
  for (const point of points) {
    for (const at of point.properties.stage_at) {
      const time = new Date(at).getTime();
      if (!Number.isFinite(time)) continue;
      if (time < min) min = time;
      if (time > max) max = time;
    }
  }
  if (!Number.isFinite(min)) return null;
  const start = startOfDay(min);
  const last = startOfDay(max);
  // Rounding absorbs the 1 h DST shift in zones that have one.
  return { start, days: Math.round((last.getTime() - start.getTime()) / DAY_MS) + 1 };
}

/** Date of timeline day `index` (0-based). */
export function dayDate(range: TimelineRange, index: number): Date {
  const date = new Date(range.start);
  date.setDate(date.getDate() + index);
  return date;
}

/** End of timeline day `index`, so every milestone reached that day counts. */
export function endOfDay(range: TimelineRange, index: number): Date {
  const date = dayDate(range, index);
  date.setHours(23, 59, 59, 999);
  return date;
}

export type PointFilter = { range?: TimelineRange | null; day?: number | null; stage?: number };

/**
 * Points visible for the timeline day (with their stage on that day) and the milestone filter.
 * `day = null` means "now": the live stage is kept.
 */
export function filterPoints(points: PointFeature[], { range, day, stage }: PointFilter): PointFeature[] {
  const at = range && day !== null && day !== undefined ? endOfDay(range, day) : null;
  const out: PointFeature[] = [];
  for (const point of points) {
    const current = at ? stageAtDay(point, at) : point.properties.stage;
    if (current === 0) continue;
    if (stage !== undefined && current !== stage) continue;
    out.push(current === point.properties.stage ? point : { ...point, properties: { ...point.properties, stage: current } });
  }
  return out;
}
