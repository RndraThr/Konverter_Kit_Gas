import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '@/lib/api';
import { useRealtimeSignals } from '@/lib/realtime';
import type { ActivityAlbum, ActivityCell, BACell, BAItem, Overview, PhaseDay, QueueItem, SerialDuplicate, SlotDetail } from './types';

type DataResponse<T> = { data: T };

export const monitoringKey = ['monitoring'] as const;
const overviewKey = (programId: string) => [...monitoringKey, 'overview', programId] as const;
export const queueKey = (scheduleId: string) => [...monitoringKey, 'queue', scheduleId] as const;
export const slotKey = (scheduleId: string, slot: number) => [...monitoringKey, 'slot', scheduleId, slot] as const;

/** Today in WIB (the field's clock), as YYYY-MM-DD. */
export function todayWIB(now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(now);
}

export function useOverview(programId: string) {
  return useQuery({
    queryKey: overviewKey(programId),
    queryFn: () => apiRequest<DataResponse<Overview>>(`/api/v1/monitoring/overview?${new URLSearchParams({ program_id: programId })}`).then((r) => r.data),
    // Day change: today's phase moves without any data change.
    refetchInterval: 10 * 60_000,
  });
}

export function useQueue(scheduleId: string) {
  return useQuery({
    queryKey: queueKey(scheduleId),
    queryFn: () => apiRequest<DataResponse<QueueItem[]>>(`/api/v1/monitoring/queue?schedule_id=${scheduleId}`).then((r) => r.data),
    enabled: scheduleId !== '',
  });
}

export function useSlotDetail(scheduleId: string, slot: number | null) {
  return useQuery({
    queryKey: slotKey(scheduleId, slot ?? 0),
    queryFn: () => apiRequest<DataResponse<SlotDetail>>(`/api/v1/monitoring/slots/${slot}?schedule_id=${scheduleId}`).then((r) => r.data),
    enabled: scheduleId !== '' && slot !== null,
  });
}

/** Approves the given photos, or every photo not checked yet when none is given. */
export const approvePhotos = (scheduleId: string, slot: number, mediaIds: string[] = []) =>
  apiRequest<DataResponse<SlotDetail>>(`/api/v1/monitoring/slots/${slot}/approve?schedule_id=${scheduleId}`, { method: 'POST', body: JSON.stringify({ media_ids: mediaIds }) }).then((r) => r.data);

export const requestRevision = (scheduleId: string, slot: number, body: { flags: { media_id: string; reason: string }[]; note: string }) =>
  apiRequest<DataResponse<SlotDetail>>(`/api/v1/monitoring/slots/${slot}/revision?schedule_id=${scheduleId}`, { method: 'POST', body: JSON.stringify(body) }).then((r) => r.data);

export const saveCalendar = (body: { program_id: string; rows: { schedule_id: string; days: PhaseDay[] }[]; rakornas?: string[] }) =>
  apiRequest(`/api/v1/monitoring/calendar`, { method: 'PUT', body: JSON.stringify(body) });

export const saveAssignments = (body: { schedule_id: string; reviewers: string[]; leaders: string[] }) =>
  apiRequest(`/api/v1/monitoring/assignments`, { method: 'PUT', body: JSON.stringify(body) });

/**
 * Live page: a change in the field (slots), a check (reviews) or a calendar
 * edit (phases) refreshes what is shown, without polling.
 */
export function useMonitoringSignals() {
  const queryClient = useQueryClient();
  useRealtimeSignals((signal) => {
    if (!signal.t || !['slots', 'reviews', 'phases'].includes(signal.t)) return;
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'overview'] });
    if (signal.t === 'slots') void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'serial-duplicates'] });
    if (signal.t === 'reviews') void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'ba'] });
    if (signal.schedule_id && signal.t !== 'phases') {
      void queryClient.invalidateQueries({ queryKey: queueKey(signal.schedule_id) });
      void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'slot', signal.schedule_id] });
    }
  });
}

export const activitiesKey = (programId: string) => [...monitoringKey, 'activities', programId] as const;
export const albumKey = (scheduleId: string, activityType: string) => [...monitoringKey, 'album', scheduleId, activityType] as const;

export function useActivityCells(programId: string) {
  return useQuery({
    queryKey: activitiesKey(programId),
    queryFn: () => apiRequest<DataResponse<ActivityCell[]>>(`/api/v1/monitoring/activities?${new URLSearchParams({ program_id: programId })}`).then((r) => r.data),
    enabled: programId !== '',
  });
}

export function useActivityAlbum(scheduleId: string, activityType: string) {
  return useQuery({
    queryKey: albumKey(scheduleId, activityType),
    queryFn: () => apiRequest<DataResponse<ActivityAlbum>>(`/api/v1/monitoring/activities/album?${new URLSearchParams({ schedule_id: scheduleId, activity_type: activityType })}`).then((r) => r.data),
    enabled: scheduleId !== '' && activityType !== '',
  });
}

export const approveActivityPhotos = (scheduleId: string, activityType: string, mediaIds: string[] = []) =>
  apiRequest<DataResponse<ActivityAlbum>>(`/api/v1/monitoring/activities/approve`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleId, activity_type: activityType, media_ids: mediaIds }) }).then((r) => r.data);

export const requestActivityRevision = (scheduleId: string, activityType: string, body: { flags: { media_id: string; reason: string }[]; note: string }) =>
  apiRequest<DataResponse<ActivityAlbum>>(`/api/v1/monitoring/activities/revision`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleId, activity_type: activityType, ...body }) }).then((r) => r.data);

/** Activity photos uploaded, deleted or checked: refresh the grid and the open album. */
export function useActivitySignals() {
  const queryClient = useQueryClient();
  useRealtimeSignals((signal) => {
    if (signal.t !== 'activities') return;
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'activities'] });
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'album'] });
  });
}

export const duplicatesKey = (programId: string) => [...monitoringKey, 'serial-duplicates', programId] as const;

/** Serials recorded on several slots; refreshed by the slot signals of useMonitoringSignals. */
export function useSerialDuplicates(programId: string) {
  return useQuery({
    queryKey: duplicatesKey(programId),
    queryFn: () => apiRequest<DataResponse<SerialDuplicate[]>>(`/api/v1/monitoring/serial-duplicates?${new URLSearchParams({ program_id: programId })}`).then((r) => r.data),
    enabled: programId !== '',
  });
}

export const baKey = (programId: string) => [...monitoringKey, 'ba', programId] as const;
export const baItemsKey = (scheduleId: string, type: string) => [...monitoringKey, 'ba', 'items', scheduleId, type] as const;

/** Uploaded BA per kabupaten and type; uploads raise no signal, so it also refreshes every minute. */
export function useBACells(programId: string) {
  return useQuery({
    queryKey: baKey(programId),
    queryFn: () => apiRequest<DataResponse<BACell[]>>(`/api/v1/monitoring/ba?${new URLSearchParams({ program_id: programId })}`).then((r) => r.data),
    enabled: programId !== '',
    refetchInterval: 60_000,
  });
}

export function useBAItems(scheduleId: string, type: string) {
  return useQuery({
    queryKey: baItemsKey(scheduleId, type),
    queryFn: () => apiRequest<DataResponse<BAItem[]>>(`/api/v1/monitoring/ba/items?${new URLSearchParams({ schedule_id: scheduleId, type })}`).then((r) => r.data),
    enabled: scheduleId !== '' && type !== '',
  });
}

export const approveBA = (scheduleId: string, keys: string[]) =>
  apiRequest(`/api/v1/monitoring/ba/approve`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleId, keys }) });

export const requestBARevision = (scheduleId: string, flags: { key: string; reason: string }[], note = '') =>
  apiRequest(`/api/v1/monitoring/ba/revision`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleId, flags, note }) });
