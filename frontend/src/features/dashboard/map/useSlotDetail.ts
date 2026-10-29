import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../../lib/api';
import type { DataResponse, DistributionSlot } from '../../distribution/types';

/** The existing POS slot lookup: recipient, machine and documentation photos of one nomor bagi. */
export function useSlotDetail(scheduleId: string | undefined, code: string | undefined) {
  return useQuery({
    queryKey: ['map', 'slot-detail', scheduleId, code],
    queryFn: async () => (await apiRequest<DataResponse<DistributionSlot>>(`/api/v1/distribution/slots/search?schedule_id=${encodeURIComponent(scheduleId ?? '')}&q=${encodeURIComponent(code ?? '')}`)).data,
    enabled: Boolean(scheduleId && code),
    staleTime: 30_000,
    retry: false,
  });
}
