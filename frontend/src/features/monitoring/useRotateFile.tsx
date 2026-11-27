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
 * and database). While the file stays open it keeps showing the turned
 * preview (no swap, the zoom stays); the rotated file is shown once another
 * file was opened, e.g. when coming back to it.
 */
export function useRotateFile(scheduleId: string) {
  const queryClient = useQueryClient();
  // Turn shown on the file as it was opened (degrees), saved or not.
  const [pending, setPending] = useState<Record<string, number>>({});
  // The part of that turn already saved to the file.
  const [held, setHeld] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState<Record<string, boolean>>({});
  // New URL of a rotated file until the monitoring data has it too.
  const [saved, setSaved] = useState<Record<string, string>>({});
  const pendingRef = useRef(pending);
  const heldRef = useRef(held);
  // The file open in the viewer.
  const current = useRef<string | undefined>(undefined);
  // URL last shown per file.
  const shown = useRef(new Map<string, string>());
  const timers = useRef(new Map<string, number>());
  useEffect(() => {
    const all = timers.current;
    return () => all.forEach((timer) => window.clearTimeout(timer));
  }, []);

  const setPendingOf = useCallback((key: string, change: (value: number) => number) => setPending((value) => {
    const next = { ...value, [key]: change(value[key] ?? 0) };
    pendingRef.current = next;
    return next;
  }), []);

  /** Shows the rotated file instead of the turned preview (the file was left). */
  const release = useCallback((key: string) => {
    const done = heldRef.current[key] ?? 0;
    heldRef.current = { ...heldRef.current, [key]: 0 };
    setHeld(heldRef.current);
    setPendingOf(key, (value) => {
      const rest = value - done;
      // Whole turns left over (e.g. four clicks) show nothing: forget them.
      return quarter(rest) === 0 && !timers.current.has(key) ? 0 : rest;
    });
  }, [setPendingOf]);

  /** The file now open in the viewer; the one left shows its rotated file from now on. */
  const focus = useCallback((kind: RotateKind, id?: string) => {
    const key = id ? `${kind}:${id}` : undefined;
    const previous = current.current;
    current.current = key;
    if (previous && previous !== key) release(previous);
  }, [release]);

  const rotate = useCallback((kind: RotateKind, id: string, delta: number, file: RotateFile = 'image') => {
    const key = `${kind}:${id}`;
    setPendingOf(key, (value) => value + delta);
    window.clearTimeout(timers.current.get(key));
    timers.current.set(key, window.setTimeout(async () => {
      timers.current.delete(key);
      // Only what is not saved yet.
      const degrees = (pendingRef.current[key] ?? 0) - (heldRef.current[key] ?? 0);
      if (quarter(degrees) === 0) {
        if (current.current !== key) release(key);
        return;
      }
      setSaving((value) => ({ ...value, [key]: true }));
      try {
        const result = await rotateFile({ schedule_id: scheduleId, kind, id, degrees: quarter(degrees) });
        // Loaded ahead, so the rotated file shows at once when opened again.
        if (file === 'image') void preload(result.content_url);
        else if (file === 'pdf') void prefetchPdfPages(queryClient, result.content_url);
        heldRef.current = { ...heldRef.current, [key]: (heldRef.current[key] ?? 0) + degrees };
        setHeld(heldRef.current);
        setSaved((value) => ({ ...value, [key]: result.content_url }));
        toast.success('Putaran disimpan ke file');
        void queryClient.invalidateQueries({ queryKey: monitoringKey });
        if (current.current !== key) release(key);
      } catch (error) {
        setPendingOf(key, (value) => value - degrees);
        toast.error(error instanceof Error ? error.message : 'Putaran belum tersimpan');
      } finally {
        setSaving((value) => ({ ...value, [key]: false }));
      }
    }, 900));
  }, [queryClient, release, scheduleId, setPendingOf]);

  return {
    rotate,
    focus,
    angle: (kind: RotateKind, id: string) => pending[`${kind}:${id}`] ?? 0,
    saving: (kind: RotateKind, id: string) => Boolean(saving[`${kind}:${id}`]),
    /**
     * The file's URL: the rotated one until the loaded data is as new. While
     * a turn is shown on the file (saved or not), the URL it was turned on
     * stays, so the preview does not change under the reviewer.
     */
    src: (kind: RotateKind, id: string, url: string) => {
      const key = `${kind}:${id}`;
      const own = saved[key];
      const fresh = own && versionOf(own) > versionOf(url) ? own : url;
      const busy = (pending[key] ?? 0) !== 0 || (held[key] ?? 0) !== 0 || Boolean(saving[key]);
      const next = busy ? (shown.current.get(key) ?? fresh) : fresh;
      shown.current.set(key, next);
      return next;
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
