import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { MapSearch } from './MapSearch';
import type { PointFeature } from './types';

const point: PointFeature = {
  type: 'Feature', geometry: { type: 'Point', coordinates: [108, -6] },
  properties: { slot_id: 'slot-12', code: '12', schedule_id: 's', regency_id: 'r', zone_id: '', site_id: 'geo:s:1', site_name: '', site_outlier: false, stage: 1, stage_at: [], location_status: 'inside' },
};

test('ArrowDown with no results, then results arriving, still lets Enter pick the first result', async () => {
  const onPick = vi.fn();
  const names = new Map<string, string>();
  const { rerender } = render(<MapSearch regencies={[]} points={[]} regencyNames={names} onPick={onPick} />);
  const input = screen.getByRole('combobox', { name: 'Cari kabupaten di Indonesia atau nomor bagi…' });

  await userEvent.type(input, '12');
  await userEvent.keyboard('{ArrowDown}');
  // Points load later (refetch) without another keystroke.
  rerender(<MapSearch regencies={[]} points={[point]} regencyNames={names} onPick={onPick} />);
  expect(await screen.findByRole('option', { name: /Nomor bagi 12/ })).toBeVisible();

  await userEvent.keyboard('{Enter}');
  expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ kind: 'point', id: 'slot-12' }));
});
