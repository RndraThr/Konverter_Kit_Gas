import { useQuery } from '@tanstack/react-query';
import { apiRequest } from '../../lib/api';

export type SlotExample = { caption: string; image_url: string; updated_at: string };

/** A documentation slot code with its example photo, set by a super admin in Pengaturan. */
export type SlotExampleEntry = {
  slot_code: string;
  label: string;
  stage: string;
  example: SlotExample | null;
};

export const slotExamplesQueryKey = ['documentation-examples'];

/** Every documentation slot code with its example photo, shared by all slots on the page. */
export function useSlotExamples(enabled = true) {
  return useQuery({
    enabled,
    queryKey: slotExamplesQueryKey,
    queryFn: () => apiRequest<{ data: SlotExampleEntry[] }>('/api/v1/documentation-examples'),
    select: (response) => (Array.isArray(response?.data) ? response.data : []),
    staleTime: 5 * 60_000,
  });
}

/** The example photo of one slot code, when a super admin has set one. */
export function useSlotExample(slotCode: string) {
  const examples = useSlotExamples();
  return examples.data?.find((entry) => entry.slot_code === slotCode)?.example ?? null;
}
