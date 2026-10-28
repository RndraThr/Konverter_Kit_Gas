import type { PointFeature } from './types';

const DAY_MS = 24 * 60 * 60 * 1000;
// Timeline days are WIB calendar days, like the dates in the detail panel, whatever the browser's
// time zone. Asia/Jakarta is UTC+7 all year (no DST), so a fixed offset is exact.
const WIB_OFFSET_MS = 7 * 60 * 60 * 1000;

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

/** 00:00 WIB of the day `time` falls on. */
const startOfDay = (time: number) => new Date(Math.floor((time + WIB_OFFSET_MS) / DAY_MS) * DAY_MS - WIB_OFFSET_MS);

export type TimelineRange = { start: Date; days: number };

/** WIB calendar days from the earliest to the latest milestone time; `start` is 00:00 WIB of the first. */
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
  return { start, days: Math.round((last.getTime() - start.getTime()) / DAY_MS) + 1 };
}

/** Start (00:00 WIB) of timeline day `index` (0-based) counted from `start`. */
export function dayStart(start: Date, index: number): Date {
  return new Date(start.getTime() + index * DAY_MS);
}

/** End of timeline day `index`, so every milestone reached that day counts. */
export function endOfDay(range: TimelineRange, index: number): Date {
  return new Date(dayStart(range.start, index + 1).getTime() - 1);
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
