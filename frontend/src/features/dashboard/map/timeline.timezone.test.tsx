import { render, screen } from '@testing-library/react';
import { afterAll, describe, expect, test, vi } from 'vitest';
import { TimelinePlayer } from './TimelinePlayer';
import { dayStart, filterPoints, timelineRange } from './timeline';
import type { PointFeature } from './types';

// A browser set to UTC. Set before the modules load (vi.hoisted runs ahead of the imports), so
// formatters created at import time see it too. Days must still start at 00:00 WIB.
const originalTZ = vi.hoisted(() => {
  const original = process.env.TZ;
  process.env.TZ = 'UTC';
  return original;
});
afterAll(() => {
  if (originalTZ === undefined) delete process.env.TZ;
  else process.env.TZ = originalTZ;
});

function point(stageAt: string[]): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates: [108, -6] },
    properties: { slot_id: stageAt[0], code: '1', schedule_id: 's', regency_id: 'r', zone_id: '', site_id: 'geo:s:1', site_name: '', site_outlier: false, stage: stageAt.length, stage_at: stageAt, location_status: 'inside' },
  };
}

describe('days follow Asia/Jakarta whatever the browser time zone', () => {
  // 23:30 WIB on 1 Nov and 00:30 WIB on 2 Nov; both are 1 Nov in UTC.
  const late = point(['2026-11-01T16:30:00Z']);
  const early = point(['2026-11-01T17:30:00Z']);

  test('the timeline splits at WIB midnight', () => {
    const range = timelineRange([late, early])!;
    expect(range.start.toISOString()).toBe('2026-10-31T17:00:00.000Z');
    expect(range.days).toBe(2);
    expect(filterPoints([late, early], { range, day: 0 })).toEqual([late]);
    expect(filterPoints([late, early], { range, day: 1 })).toHaveLength(2);
    expect(dayStart(range.start, 1).toISOString()).toBe('2026-11-01T17:00:00.000Z');
  });

  test('the player labels days in WIB', () => {
    // 00:00 WIB on 1 Nov, which is still 31 Oct in UTC.
    const start = new Date('2026-10-31T17:00:00Z');
    const { rerender } = render(<TimelinePlayer start={start} days={2} day={0} onChange={() => undefined} />);
    expect(screen.getByText(/Hari ke-1 · 1 Nov 2026/)).toBeInTheDocument();
    rerender(<TimelinePlayer start={start} days={2} day={1} onChange={() => undefined} />);
    expect(screen.getByText(/Hari ke-2 · 2 Nov 2026/)).toBeInTheDocument();
  });
});
