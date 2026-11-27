import { useQueryClient } from '@tanstack/react-query';
import { Loader2, RotateCcw, RotateCw } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { apiRequest } from '@/lib/api';
import { monitoringKey } from './api';
import { prefetchPdfPages } from './pdfPages';

/** photo: distribution photo, activity: activity file, ba: uploaded BA (id = item key). */
export type RotateKind = 'photo' | 'activity' | 'ba';

/** What to load before the rotated file is shown: an image, PDF pages, or nothing. */
export type RotateFile = 'image' | 'pdf' | 'none';

export const rotateFile = (body: { schedule_id: string; kind: RotateKind; id: string; degrees: number }) =>
  apiRequest<{ data: { content_url: string } }>('/api/v1/monitoring/rotate', { method: 'POST', body: JSON.stringify(body) }).then((r) => r.data);

const quarter = (degrees: number) => ((degrees % 360) + 360) % 360;

/** ?v= of a content URL (0 before any rotation). */
const versionOf = (url: string) => Number(new URL(url, window.location.origin).searchParams.get('v') ?? 0);

/** Loads and decodes an image so showing it later is instant. */
function preload(url: string): Promise<void> {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => { void img.decode().catch(() => undefined).then(() => resolve()); };
    img.onerror = () => resolve();
    img.src = url;
  });
}

/**
 * Rotation of files in the monitoring: each click turns the preview at once;
 * shortly after the last click the turn is saved to the file itself (Drive
 * and database). The rotated image is loaded first, then shown in place of
 * the turned preview in the same render, so the photo never jumps back.
 */
export function useRotateFile(scheduleId: string) {
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState<Record<string, boolean>>({});
  // New URL of a rotated file until the monitoring data has it too.
  const [saved, setSaved] = useState<Record<string, string>>({});
  const pendingRef = useRef(pending);
  const timers = useRef(new Map<string, number>());
  useEffect(() => { pendingRef.current = pending; }, [pending]);
  useEffect(() => {
    const all = timers.current;
    return () => all.forEach((timer) => window.clearTimeout(timer));
  }, []);

  const rotate = useCallback((kind: RotateKind, id: string, delta: number, file: RotateFile = 'image') => {
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
      let ok = false;
      if (quarter(degrees) === 0) { settle(); return; }
      setSaving((value) => ({ ...value, [key]: true }));
      try {
        const result = await rotateFile({ schedule_id: scheduleId, kind, id, degrees: quarter(degrees) });
        if (file === 'image') await preload(result.content_url);
        else if (file === 'pdf') await prefetchPdfPages(queryClient, result.content_url);
        // One render: the rotated file replaces the turned preview.
        setSaved((value) => ({ ...value, [key]: result.content_url }));
        settle();
        ok = true;
        toast.success('Putaran disimpan ke file');
        void queryClient.invalidateQueries({ queryKey: monitoringKey });
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Putaran belum tersimpan');
      } finally {
        if (!ok) settle();
        setSaving((value) => ({ ...value, [key]: false }));
      }
    }, 900));
  }, [queryClient, scheduleId]);

  return {
    rotate,
    angle: (kind: RotateKind, id: string) => pending[`${kind}:${id}`] ?? 0,
    saving: (kind: RotateKind, id: string) => Boolean(saving[`${kind}:${id}`]),
    /** The file's URL: the rotated one until the loaded data is as new. */
    src: (kind: RotateKind, id: string, url: string) => {
      const own = saved[`${kind}:${id}`];
      return own && versionOf(own) > versionOf(url) ? own : url;
    },
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
