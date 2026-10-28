import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../../lib/api';

export type UnlocatedItem = { slot_id: string; code: string; regency_id: string; stage: number };
export type UnlocatedList = { items: UnlocatedItem[]; total: number };

/** Slots without a usable GPS photo (GET /map/unlocated, capped at 500 by the server). */
export function useUnlocated(filter: string, enabled: boolean) {
  return useQuery({
    queryKey: ['map', 'unlocated', filter],
    queryFn: async () => (await apiRequest<{ data: UnlocatedList }>(`/api/v1/map/unlocated${filter ? `?${filter}` : ''}`)).data,
    enabled,
  });
}
