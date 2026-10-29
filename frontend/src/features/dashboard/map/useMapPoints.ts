import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../../lib/api';
import type { PointCollection } from './types';

// The Data Penerima filter contract (recipientFilterFromRequest). Map view params (base, color, c,
// z, slot, site, stage) are deliberately excluded so panning or switching basemap never refetches.
export const FILTER_KEYS = ['regency_id', 'program_id', 'schedule_id', 'zone_id', 'program_type', 'district', 'allocation_status', 'distribution_status', 'evidence_status', 'search'] as const;

export function filterQuery(params: URLSearchParams): string {
  const out = new URLSearchParams();
  for (const key of FILTER_KEYS) {
    const value = params.get(key);
    if (value) out.set(key, value);
  }
  return out.toString();
}

export const POINTS_REFETCH_MS = 60_000;

/** Located slots for the current filter. Refreshed every minute while visible and on focus. */
export function useMapPoints(filter: string) {
  return useQuery({
    queryKey: ['map', 'points', filter],
    queryFn: async () => (await apiRequest<{ data: PointCollection }>(`/api/v1/map/points${filter ? `?${filter}` : ''}`)).data,
    refetchInterval: POINTS_REFETCH_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });
}
