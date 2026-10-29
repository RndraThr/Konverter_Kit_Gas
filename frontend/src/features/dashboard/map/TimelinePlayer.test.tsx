import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { TimelinePlayer, TIMELINE_STEP_MS } from './TimelinePlayer';
import { filterPoints, stageAtDay, timelineRange } from './timeline';
import type { PointFeature } from './types';

function point(stageAt: string[]): PointFeature {
  return {
    type: 'Feature', geometry: { type: 'Point', coordinates: [108, -6] },
    properties: { slot_id: 's', code: '1', schedule_id: 's', regency_id: 'r', zone_id: '', site_id: 'geo:s:1', site_name: '', site_outlier: false, stage: stageAt.length, stage_at: stageAt, location_status: 'inside' },
  };
}

const p = point(['2026-11-01T10:00:00', '2026-11-03T10:00:00', '2026-11-06T10:00:00']);

afterEach(() => vi.useRealTimers());

test('stageAtDay is 0 before the first milestone', () => {
  expect(stageAtDay(p, new Date('2026-10-31T00:00:00'))).toBe(0);
});

test('stageAtDay between milestone 2 and 3 is 2', () => {
  expect(stageAtDay(p, new Date('2026-11-04T12:00:00'))).toBe(2);
  expect(stageAtDay(p, new Date('2026-11-06T10:00:00'))).toBe(3);
});

test('timelineRange spans from the first stage date to the last, one entry per day', () => {
  const range = timelineRange([p, point(['2026-11-02T10:00:00'])])!;
  expect(range.days).toBe(6);
  expect(timelineRange([])).toBeNull();
});

test('filterPoints applies the timeline day and the milestone filter', () => {
  const early = point(['2026-11-05T10:00:00']);
  const range = timelineRange([p, early])!;
  // Day index 2 is 2026-11-03 (local end of day): p is at stage 2, early has not appeared yet.
  const atDay2 = filterPoints([p, early], { range, day: 2 });
  expect(atDay2.map((f) => f.properties.stage)).toEqual([2]);
  expect(filterPoints([p, early], { range, day: null, stage: 1 })).toEqual([early]);
});

test('the player advances one day every 700 ms and stops on the last day', () => {
  vi.useFakeTimers();
  const onChange = vi.fn();
  const { rerender } = render(<TimelinePlayer start={new Date(2026, 10, 1)} days={3} day={0} onChange={onChange} />);
  expect(TIMELINE_STEP_MS).toBe(700);
  expect(screen.getByText(/Hari ke-1 · /)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Putar timeline' }));
  act(() => { vi.advanceTimersByTime(700); });
  expect(onChange).toHaveBeenLastCalledWith(1);

  rerender(<TimelinePlayer start={new Date(2026, 10, 1)} days={3} day={2} onChange={onChange} />);
  onChange.mockClear();
  act(() => { vi.advanceTimersByTime(2100); });
  expect(onChange).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Putar timeline' })).toBeInTheDocument();
});

test('the slider picks a day', () => {
  const onChange = vi.fn();
  render(<TimelinePlayer start={new Date(2026, 10, 1)} days={5} day={null} onChange={onChange} />);
  const slider = screen.getByRole('slider', { name: 'Tanggal timeline' });
  expect(slider).toHaveValue('4');
  fireEvent.change(slider, { target: { value: '1' } });
  expect(onChange).toHaveBeenCalledWith(1);
});
