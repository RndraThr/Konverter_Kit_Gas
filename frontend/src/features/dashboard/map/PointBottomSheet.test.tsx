import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, test, vi } from 'vitest';
import { apiRequest } from '../../../lib/api';
import { PointBottomSheet } from './PointBottomSheet';
import type { PointFeature } from './types';

vi.mock('../../../lib/api', () => ({ apiRequest: vi.fn() }));

const point: PointFeature = {
  type: 'Feature', geometry: { type: 'Point', coordinates: [105.5, -2.25] },
  properties: { slot_id: 'slot-1', code: '12', schedule_id: 'schedule-1', regency_id: 'regency-1', zone_id: '', site_id: 'geo:schedule-1:1', site_name: '', site_outlier: false, stage: 1, stage_at: ['2026-11-01T03:00:00Z'], location_status: 'inside' },
};

afterEach(() => vi.clearAllMocks());

test('the bottom sheet shows the same detail and closes', async () => {
  vi.mocked(apiRequest).mockRejectedValue(new Error('no detail'));
  const onClose = vi.fn();
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><PointBottomSheet selected={{ kind: 'point', slotId: 'slot-1' }} points={[point]} regencyNames={new Map()} onSelect={vi.fn()} onClose={onClose} /></MemoryRouter></QueryClientProvider>);
  const sheet = screen.getByRole('region', { name: 'Detail titik' });
  expect(within(sheet).getByRole('heading', { name: 'Nomor bagi 12' })).toBeVisible();
  await userEvent.click(within(sheet).getByRole('button', { name: 'Tutup' }));
  expect(onClose).toHaveBeenCalled();
});
