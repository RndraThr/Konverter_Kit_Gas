import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, CheckCheck, CircleCheck, Flag, RotateCcw } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { useCan } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import { approveBA, monitoringKey, requestBARevision, useBAItems } from './api';
import { BADocument } from './BADocument';
import { useRotateFile } from './useRotateFile';
import type { ZoomHandle } from './ZoomableImage';
import { baLabel, baSourceLabels, type BAItem } from './types';

const formatDate = (day: string) => new Date(`${day}T00:00:00`).toLocaleDateString('id-ID', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const formatTime = (value: string) => new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });

function StateBadge({ item }: { item: BAItem }) {
  if (!item.review) return <span className="rounded bg-amber-400 px-1.5 text-[11px] font-semibold text-neutral-900">Baru</span>;
  return item.review.status === 'approved'
    ? <span className="rounded bg-emerald-500 px-1.5 text-[11px] font-semibold text-white">Disetujui</span>
    : <span className="rounded bg-destructive px-1.5 text-[11px] font-semibold text-white">Revisi</span>;
}

/**
 * Check of one BA type of a kabupaten: its uploads (per date) on the left,
 * the document in the middle, the check on the right. A approves the shown
 * BA; a revision tells the uploader and the kabupaten's leaders.
 */
export function BAReviewDialog({ scheduleId, type, regencyName, onClose }: { scheduleId: string; type: string; regencyName: string; onClose: () => void }) {
  const open = scheduleId !== '' && type !== '';
  const items = useBAItems(scheduleId, type);
  const canReview = useCan('monitoring.review');
  const queryClient = useQueryClient();
  const [index, setIndex] = useState(0);
  const [reason, setReason] = useState('');
  const [flagging, setFlagging] = useState(false);
  const rotation = useRotateFile(scheduleId);
  const zoom = useRef<ZoomHandle>(null);
  useEffect(() => { setIndex(0); setReason(''); setFlagging(false); }, [scheduleId, type]);

  const list = items.data ?? [];
  const current = list[Math.min(index, Math.max(list.length - 1, 0))];
  // Leaving a rotated file shows its rotated version when coming back.
  const focusRotation = rotation.focus;
  useEffect(() => focusRotation('ba', current?.key), [focusRotation, current?.key]);
  const fresh = list.filter((item) => !item.review);
  const refresh = () => void queryClient.invalidateQueries({ queryKey: [...monitoringKey] });
  const next = () => { const i = list.findIndex((item, j) => j !== index && !item.review); if (i >= 0) setIndex(i); };
  const approve = useMutation({
    mutationFn: (keys: string[]) => approveBA(scheduleId, keys),
    onSuccess: (_, keys) => { toast.success(keys.length === 1 ? 'BA disetujui' : `${keys.length} BA disetujui`); refresh(); next(); },
    onError: (e) => toast.error(e.message),
  });
  const revise = useMutation({
    mutationFn: () => requestBARevision(scheduleId, [{ key: current.key, reason: reason.trim() }]),
    onSuccess: () => { toast.success('BA dikirim untuk revisi'); setReason(''); setFlagging(false); refresh(); next(); },
    onError: (e) => toast.error(e.message),
  });
  const busy = approve.isPending || revise.isPending;
  const canApprove = canReview && !!current && current.review?.status !== 'approved';
  // Uploaded scans and sheets turn (saved to the file); BA Perorangan is drawn from the BA.
  const canRotate = canReview && !!current && !current.individual;

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
      if (event.key === 'ArrowDown' || event.key === 'ArrowRight') setIndex((i) => Math.min(i + 1, list.length - 1));
      else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') setIndex((i) => Math.max(i - 1, 0));
      else if ((event.key === 'a' || event.key === 'A') && canApprove && !busy) approve.mutate([current.key]);
      else if ((event.key === 'r' || event.key === 'R') && canRotate) rotation.rotate('ba', current.key, event.shiftKey ? -90 : 90, current.mime_type.startsWith('image/') ? 'image' : 'pdf');
      else if (event.key === '+' || event.key === '=') zoom.current?.zoomIn();
      else if (event.key === '-') zoom.current?.zoomOut();
      else if (event.key === '0') zoom.current?.reset();
      else return;
      event.preventDefault();
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [open, list.length, canApprove, canRotate, rotation, busy, current, approve]);

  return <Dialog open={open} onOpenChange={(value) => { if (!value) onClose(); }}>
    <DialogContent className="flex h-dvh w-screen max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 ring-0 sm:max-w-none">
      <div className="border-b px-4 py-2.5 pr-12">
        <DialogTitle className="truncate">{baLabel(type)} · {regencyName}</DialogTitle>
        <DialogDescription className="text-xs">{items.data ? `${list.length} dokumen · ${fresh.length} belum dicek` : ' '}</DialogDescription>
      </div>
      {items.isPending ? <p className="m-auto text-sm text-muted-foreground">Memuat BA…</p>
        : items.isError ? <p className="m-auto text-sm text-destructive">{items.error.message}</p>
        : list.length === 0 ? <p className="m-auto text-sm text-muted-foreground">Belum ada BA yang diunggah.</p>
        : <div className="grid min-h-0 flex-1 lg:grid-cols-[15rem_minmax(0,1fr)_20rem]">
          <ul className="min-h-0 overflow-y-auto border-r p-2" aria-label="Dokumen BA">
            {list.map((item, i) => <li key={item.key}>
              <button type="button" onClick={() => setIndex(i)} className={cn('mb-1 w-full rounded-md px-2.5 py-2 text-left text-sm hover:bg-muted', i === index && 'bg-muted ring-1 ring-primary/40')}>
                <span className="flex items-center justify-between gap-2"><span className="font-medium">{formatDate(item.date)}</span><StateBadge item={item} /></span>
                <span className="block truncate text-xs text-muted-foreground">{baSourceLabels[item.source]}</span>
              </button>
            </li>)}
          </ul>
          <div className="relative flex min-h-0 bg-neutral-900">{current ? <>
            <BADocument ref={zoom} key={current.key} item={{ ...current, content_url: rotation.src('ba', current.key, current.content_url) }}
              rotation={rotation.angle('ba', current.key)} rotating={rotation.saving('ba', current.key)}
              onRotate={canRotate ? (delta) => rotation.rotate('ba', current.key, delta, current.mime_type.startsWith('image/') ? 'image' : 'pdf') : undefined} />
          </> : null}</div>
          <aside className="flex min-h-0 flex-col border-l">
            {current ? <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4 text-sm">
              <div>
                <p className="font-medium">{current.label} · {formatDate(current.date)}</p>
                <p className="text-xs text-muted-foreground">{baSourceLabels[current.source]} · diunggah {current.uploaded_by || '-'}, {formatTime(current.uploaded_at)}</p>
              </div>
              {current.review ? <p className={cn('rounded-md px-2 py-1 text-xs', current.review.status === 'approved' ? 'bg-emerald-50 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' : 'bg-destructive/10 text-destructive')}>
                {current.review.status === 'approved' ? 'Disetujui' : `Revisi${current.review.reason ? `: ${current.review.reason}` : ''}`} · {current.review.reviewed_by} · {formatTime(current.review.reviewed_at)}
              </p> : null}
              <p className="text-xs text-muted-foreground">Periksa tanda tangan, stempel, tanggal, dan isi. Dokumen yang diganti lapangan perlu dicek ulang.</p>
            </div> : null}
            {canReview && current ? <div className="space-y-2 border-t p-3">
              {flagging ? <div className="space-y-2 rounded-lg bg-destructive/5 p-2.5">
                <Input autoFocus value={reason} maxLength={300} placeholder="Alasan, mis. stempel dinas belum ada" aria-label="Alasan revisi BA" onChange={(e) => setReason(e.target.value)} />
                <p className="text-xs">Pengunggah dan leader kabupaten mendapat notifikasi.</p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" className="flex-1" onClick={() => setFlagging(false)} disabled={busy}>Batal</Button>
                  <Button variant="destructive" size="sm" className="flex-1" onClick={() => revise.mutate()} disabled={busy || !reason.trim()}><RotateCcw />Kirim revisi</Button>
                </div>
              </div> : <>
                <Button className="w-full" disabled={busy || !canApprove} onClick={() => approve.mutate([current.key])} title="Pintasan: A"><Check />{current.review?.status === 'approved' ? 'Sudah disetujui' : 'Setujui BA ini'}</Button>
                <div className="flex gap-2">
                  <Button variant="outline" className="flex-1" disabled={busy} onClick={() => setFlagging(true)}><Flag />Minta revisi</Button>
                  <Button variant="outline" className="flex-1" disabled={busy || fresh.length === 0} onClick={() => approve.mutate(fresh.map((item) => item.key))}><CheckCheck />Setujui {fresh.length || ''} baru</Button>
                </div>
              </>}
              <p className="text-center text-[11px] text-muted-foreground">↑ ↓ ganti dokumen · A setujui · R putar · scroll zoom</p>
            </div> : null}
          </aside>
        </div>}
      {list.length > 0 && fresh.length === 0 ? <p className="flex items-center justify-center gap-1 border-t py-1.5 text-xs text-emerald-700"><CircleCheck className="size-3.5" />Semua BA {baLabel(type)} kabupaten ini sudah dicek</p> : null}
    </DialogContent>
  </Dialog>;
}
