import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../../lib/api';
import type { MapConfig } from './types';

export function useMapConfig() {
  return useQuery({
    queryKey: ['map', 'config'],
    queryFn: async () => (await apiRequest<{ data: MapConfig }>('/api/v1/map/config')).data,
    staleTime: Infinity,
  });
}
