import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../../lib/api';
import type { RegencyBoundary, ZoneCollection } from './types';

/** Zone outlines of one program. Zones only mean something within a program, so no program = no request. */
export function useZoneBoundaries(programId: string | undefined) {
  return useQuery({
    queryKey: ['map', 'zones', programId ?? ''],
    queryFn: async () => (await apiRequest<{ data: ZoneCollection }>(`/api/v1/map/zones?program_id=${encodeURIComponent(programId ?? '')}`)).data,
    enabled: Boolean(programId),
    staleTime: 5 * 60_000,
  });
}

/** Precise boundary of a master regency, used to highlight the selected kabupaten. */
export function useRegencyBoundary(regencyId: string | undefined) {
  return useQuery({
    queryKey: ['map', 'regency-boundary', regencyId ?? ''],
    queryFn: async () => (await apiRequest<{ data: RegencyBoundary }>(`/api/v1/map/regencies/${encodeURIComponent(regencyId ?? '')}/boundary`)).data,
    enabled: Boolean(regencyId),
    staleTime: Infinity,
    retry: false,
  });
}
