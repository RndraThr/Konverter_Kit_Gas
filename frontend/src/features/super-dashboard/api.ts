import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api';
import type { Dashboard } from './types';

/** Progress of one program ("" = the newest active one). */
export function useDashboardProgress(programId: string) {
  return useQuery({
    queryKey: ['dashboard', 'progress', programId],
    queryFn: () => apiRequest<{ data: Dashboard }>(`/api/v1/dashboard/progress${programId ? `?${new URLSearchParams({ program_id: programId })}` : ''}`).then((r) => r.data),
    refetchInterval: 5 * 60_000,
  });
}
