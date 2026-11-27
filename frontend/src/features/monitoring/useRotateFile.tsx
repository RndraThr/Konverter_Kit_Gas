import { useQueryClient } from '@tanstack/react-query';
import { Loader2, RotateCcw, RotateCw } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { apiRequest } from '@/lib/api';
import { monitoringKey } from './api';

/** photo: distribution photo, activity: activity file, ba: uploaded BA (id = item key). */
export type RotateKind = 'photo' | 'activity' | 'ba';

export const rotateFile = (body: { schedule_id: string; kind: RotateKind; id: string; degrees: number }) =>
  apiRequest<{ data: { content_url: string } }>('/api/v1/monitoring/rotate', { method: 'POST', body: JSON.stringify(body) }).then((r) => r.data);

const quarter = (degrees: number) => ((degrees % 360) + 360) % 360;

/**
 * Rotation of files in the monitoring: each click turns the preview at once;
 * shortly after the last click the turn is saved to the file itself (Drive
 * and database), then the monitoring data is read again with its new URL.
 */
export function useRotateFile(scheduleId: string) {
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState<Record<string, boolean>>({});
  const pendingRef = useRef(pending);
  const timers = useRef(new Map<string, number>());
  useEffect(() => { pendingRef.current = pending; }, [pending]);
  useEffect(() => {
    const all = timers.current;
    return () => all.forEach((timer) => window.clearTimeout(timer));
  }, []);

  const rotate = useCallback((kind: RotateKind, id: string, delta: number) => {
    const key = `${kind}:${id}`;
    setPending((value) => {
      const next = { ...value, [key]: (value[key] ?? 0) + delta };
      pendingRef.current = next;
      return next;
    });
    window.clearTimeout(timers.current.get(key));
    timers.current.set(key, window.setTimeout(async () => {
      timers.current.delete(key);
      const degrees = pendingRef.current[key] ?? 0;
      const settle = () => setPending((value) => ({ ...value, [key]: (value[key] ?? 0) - degrees }));
      if (quarter(degrees) === 0) { settle(); return; }
      setSaving((value) => ({ ...value, [key]: true }));
      try {
        await rotateFile({ schedule_id: scheduleId, kind, id, degrees: quarter(degrees) });
        await queryClient.invalidateQueries({ queryKey: monitoringKey });
        toast.success('Putaran disimpan ke file');
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Putaran belum tersimpan');
      } finally {
        settle();
        setSaving((value) => ({ ...value, [key]: false }));
      }
    }, 900));
  }, [queryClient, scheduleId]);

  return {
    rotate,
    angle: (kind: RotateKind, id: string) => pending[`${kind}:${id}`] ?? 0,
    saving: (kind: RotateKind, id: string) => Boolean(saving[`${kind}:${id}`]),
  };
}

/** Rotate buttons over a document (PDF, BA scan) that has no zoom toolbar. */
export function RotateButtons({ onRotate, saving, angle = 0 }: { onRotate: (delta: number) => void; saving?: boolean; angle?: number }) {
  const shown = quarter(angle);
  return <div className="absolute right-3 bottom-3 z-10 flex items-center gap-1 rounded-lg bg-black/60 p-1 text-white backdrop-blur">
    {saving || shown ? <span className="px-1.5 text-xs">{saving ? 'Menyimpan…' : `${shown}°`}</span> : null}
    <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Putar ke kiri" title="Putar ke kiri (Shift+R), tersimpan ke file" onClick={() => onRotate(-90)}><RotateCcw /></Button>
    <Button variant="ghost" size="icon-sm" className="text-white hover:bg-white/20 hover:text-white" aria-label="Putar ke kanan" title="Putar ke kanan (R), tersimpan ke file" onClick={() => onRotate(90)}>{saving ? <Loader2 className="animate-spin" /> : <RotateCw />}</Button>
  </div>;
}
