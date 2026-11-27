import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, CheckCheck, ChevronLeft, ChevronRight, CircleCheck, Flag, RotateCcw } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useCan } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import { albumKey, approveActivityPhotos, monitoringKey, requestActivityRevision, useActivityAlbum } from './api';
import { activityLabel, type ActivityAlbum, type ActivityPhoto } from './types';
import { ZoomableImage, type ZoomHandle } from './ZoomableImage';
import { BADocument } from './BADocument';
import { RotateButtons, useRotateFile } from './useRotateFile';

const formatTime = (value?: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-';
/** Photos copied from the distribution were checked there already. */
const checkable = (p: ActivityPhoto) => p.source !== 'distribution';

/**
 * Check of one activity album (kabupaten × activity type): carousel on the
 * left (arrow keys, zoom, thumbnails), the photo's check on the right. A
 * approves the photo shown, Shift + A every new photo; marked photos go back
 * to the uploader and the kabupaten's leaders.
 */
export function ActivityReviewDialog({ scheduleId, activity, onClose }: { scheduleId: string; activity: string; onClose: () => void }) {
  const open = scheduleId !== '' && activity !== '';
  const album = useActivityAlbum(scheduleId, activity);
  const canReview = useCan('monitoring.review');
  const queryClient = useQueryClient();
  const zoom = useRef<ZoomHandle>(null);
  const [index, setIndex] = useState(0);
  const [flags, setFlags] = useState<Record<string, string>>({});
  const [note, setNote] = useState('');
  const [confirming, setConfirming] = useState(false);
  const rotation = useRotateFile(scheduleId);
  useEffect(() => { setIndex(0); setFlags({}); setNote(''); setConfirming(false); }, [scheduleId, activity]);

  const photos = album.data?.photos ?? [];
  const current = photos[Math.min(index, Math.max(photos.length - 1, 0))];
  const flagged = Object.keys(flags);
  const fresh = photos.filter((p) => checkable(p) && !p.review && flags[p.id] === undefined);
  const approved = photos.filter((p) => !checkable(p) || p.review?.status === 'approved').length;
  const rejected = photos.filter((p) => p.review?.status === 'rejected').length;

  const settle = (result: ActivityAlbum, message: string) => {
    queryClient.setQueryData(albumKey(scheduleId, activity), result);
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'activities'] });
    const left = result.photos.findIndex((p) => checkable(p) && !p.review && flags[p.id] === undefined);
    if (left >= 0) setIndex(left);
    toast.success(message);
  };
  const approve = useMutation({
    mutationFn: (ids: string[]) => approveActivityPhotos(scheduleId, activity, ids),
    onSuccess: (result, ids) => settle(result, ids.length === 1 ? 'Foto disetujui' : `${ids.length} foto disetujui`),
    onError: (e) => toast.error(e.message),
  });
  const revise = useMutation({
    mutationFn: () => requestActivityRevision(scheduleId, activity, { flags: flagged.map((media_id) => ({ media_id, reason: flags[media_id].trim() })), note: note.trim() }),
    onSuccess: (result) => {
      const count = flagged.length;
      setFlags({}); setNote(''); setConfirming(false);
      settle(result, `${count} foto dikirim untuk revisi`);
    },
    onError: (e) => toast.error(e.message),
  });
  const busy = approve.isPending || revise.isPending;
  const canApproveCurrent = canReview && !!current && checkable(current) && current.review?.status !== 'approved' && flags[current.id] === undefined;
  // Photos and PDFs turn and are saved so; videos not.
  const canRotate = canReview && !!current && !current.mime_type.startsWith('video/');

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
      if (event.key === 'ArrowRight') setIndex((i) => Math.min(i + 1, photos.length - 1));
      else if (event.key === 'ArrowLeft') setIndex((i) => Math.max(i - 1, 0));
      else if (event.key === '+' || event.key === '=') zoom.current?.zoomIn();
      else if (event.key === '-') zoom.current?.zoomOut();
      else if (event.key === '0') zoom.current?.reset();
      else if ((event.key === 'r' || event.key === 'R') && canRotate) rotation.rotate('activity', current.id, event.shiftKey ? -90 : 90);
      else if ((event.key === 'a' || event.key === 'A') && event.shiftKey && canReview && fresh.length > 0 && !busy) approve.mutate(fresh.map((p) => p.id));
      else if ((event.key === 'a' || event.key === 'A') && canApproveCurrent && !busy) approve.mutate([current.id]);
      else return;
      event.preventDefault();
      event.stopPropagation();
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [open, photos.length, canReview, canApproveCurrent, canRotate, rotation, current, fresh, busy, approve]);

  const toggleFlag = (id: string, on: boolean) => setFlags((value) => {
    const next = { ...value };
    if (on) next[id] = next[id] ?? photos.find((p) => p.id === id)?.review?.reason ?? ''; else delete next[id];
    return next;
  });
  const video = current?.mime_type.startsWith('video/');
  const pdf = current?.mime_type === 'application/pdf';
  const missingSlots = (album.data?.slots ?? []).filter((slot) => slot.files === 0);

  return <Dialog open={open} onOpenChange={(value) => { if (!value) onClose(); }}>
    <DialogContent className="flex h-dvh w-screen max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 ring-0 sm:max-w-none">
      <div className="border-b px-4 py-2.5 pr-12">
        <DialogTitle className="truncate">{activityLabel(activity)} · {album.data?.regency_name ?? 'Memuat…'}</DialogTitle>
        <DialogDescription className="truncate text-xs">{album.data ? `${photos.length} foto · ${approved} disetujui · ${fresh.length} baru${rejected ? ` · ${rejected} ditolak` : ''}` : ' '}</DialogDescription>
      </div>
      {album.isPending ? <p className="m-auto text-sm text-muted-foreground">Memuat foto…</p>
        : album.isError || !album.data ? <p className="m-auto text-sm text-destructive">{album.error?.message ?? 'Data tidak ditemukan'}</p>
        : photos.length === 0 ? <div className="m-auto max-w-md text-center text-sm text-muted-foreground">
          <p>Belum ada file untuk {activityLabel(activity)}.</p>
          {missingSlots.length ? <ul className="mt-2 text-xs text-destructive">{missingSlots.map((slot) => <li key={slot.name}>{slot.name}</li>)}</ul> : null}
        </div>
        : <div className="grid min-h-0 flex-1 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="flex min-h-0 flex-col bg-neutral-950">
            <div className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden">
              {current ? (pdf
                ? <div className="relative size-full bg-neutral-100"><BADocument key={current.id} item={{ key: current.id, type: activity, label: current.slot || current.display_name, date: '', source: 'archive', content_url: rotation.src('activity', current.id, current.content_url), mime_type: current.mime_type, uploaded_by: current.uploaded_by, uploaded_at: current.uploaded_at, review: null }} />
                  {canRotate ? <RotateButtons angle={rotation.angle('activity', current.id)} saving={rotation.saving('activity', current.id)} onRotate={(delta) => rotation.rotate('activity', current.id, delta, false)} /> : null}</div>
                : video
                ? <video key={current.id} src={current.content_url} controls className="max-h-full max-w-full" />
                : <ZoomableImage ref={zoom} src={rotation.src('activity', current.id, current.content_url)} alt={current.display_name} rotation={rotation.angle('activity', current.id)}
                  onRotate={canRotate ? (delta) => rotation.rotate('activity', current.id, delta) : undefined} rotating={rotation.saving('activity', current.id)} />) : null}
              {current ? <div className="pointer-events-none absolute top-3 left-3 flex items-center gap-2 rounded-md bg-black/60 px-2.5 py-1 text-xs text-white">
                {index + 1}/{photos.length}
                {!checkable(current) ? <span className="rounded bg-sky-500 px-1.5 font-semibold">Dari distribusi</span>
                  : current.review?.status === 'approved' ? <span className="rounded bg-emerald-500 px-1.5 font-semibold">Disetujui</span>
                  : current.review?.status === 'rejected' ? <span className="rounded bg-destructive px-1.5 font-semibold">Ditolak</span>
                  : <span className="rounded bg-amber-400 px-1.5 font-semibold text-neutral-900">Baru</span>}
              </div> : null}
              {current && flags[current.id] !== undefined ? <div className="pointer-events-none absolute inset-0 ring-4 ring-destructive ring-inset" /> : null}
              <Button variant="secondary" size="icon" className="absolute top-1/2 left-3 -translate-y-1/2 opacity-80" aria-label="Foto sebelumnya" disabled={index === 0} onClick={() => setIndex((i) => i - 1)}><ChevronLeft /></Button>
              <Button variant="secondary" size="icon" className="absolute top-1/2 right-3 -translate-y-1/2 opacity-80" aria-label="Foto berikutnya" disabled={index >= photos.length - 1} onClick={() => setIndex((i) => i + 1)}><ChevronRight /></Button>
            </div>
            <div className="flex gap-2 overflow-x-auto border-t border-white/10 p-2">
              {photos.map((photo, i) => {
                const marked = flags[photo.id] !== undefined;
                return <button key={photo.id} type="button" onClick={() => setIndex(i)} aria-label={`Foto ${i + 1}`}
                  className={cn('relative size-16 shrink-0 overflow-hidden rounded-md border-2 bg-neutral-800', i === index ? 'border-white' : 'border-transparent opacity-70 hover:opacity-100', marked && 'border-destructive opacity-100')}>
                  {photo.mime_type === 'application/pdf' ? <span className="flex size-full items-center justify-center bg-white text-[11px] font-bold text-red-700">PDF</span>
                    : photo.mime_type.startsWith('video/') ? <video src={photo.content_url} preload="metadata" className="size-full object-cover" /> : <img src={photo.content_url} alt="" loading="lazy" className="size-full object-cover" />}
                  {marked || photo.review?.status === 'rejected' ? <Flag className="absolute top-1 right-1 size-3.5 fill-destructive text-destructive" />
                    : photo.review?.status === 'approved' || !checkable(photo) ? <CircleCheck className="absolute top-1 right-1 size-4 fill-emerald-500 text-white" />
                    : <span className="absolute top-1 right-1 size-2.5 rounded-full bg-amber-400 ring-2 ring-neutral-900" title="Belum dicek" />}
                </button>;
              })}
            </div>
          </div>
          <aside className="flex min-h-0 flex-col border-l">
            <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
              {current ? <section className="rounded-lg border p-3">
                <p className="truncate text-sm font-medium" title={current.display_name}>{current.slot || current.display_name}</p>
                {current.slot ? <p className="truncate text-xs text-muted-foreground">{current.display_name}</p> : null}
                <p className="text-xs text-muted-foreground">Diunggah {formatTime(current.uploaded_at)}{current.uploaded_by ? ` · ${current.uploaded_by}` : ''}</p>
                {!checkable(current) ? <p className="mt-2 rounded-md bg-sky-50 px-2 py-1 text-xs text-sky-900 dark:bg-sky-950/40 dark:text-sky-200">Disalin dari foto distribusi, dicek di antrean distribusi.</p> : null}
                {current.review ? <p className={cn('mt-2 rounded-md px-2 py-1 text-xs', current.review.status === 'approved' ? 'bg-emerald-50 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' : 'bg-destructive/10 text-destructive')}>
                  {current.review.status === 'approved' ? 'Disetujui' : `Ditolak${current.review.reason ? `: ${current.review.reason}` : ''}`} · {current.review.reviewed_by} · {formatTime(current.review.reviewed_at)}
                </p> : null}
                {canReview && checkable(current) ? <div className="mt-3 space-y-2">
                  <Button size="sm" className="w-full" disabled={busy || !canApproveCurrent} onClick={() => approve.mutate([current.id])} title="Pintasan: A">
                    <Check />{current.review?.status === 'approved' ? 'Sudah disetujui' : 'Setujui foto ini'}
                  </Button>
                  <Label className="flex items-center gap-2 font-normal">
                    <Checkbox checked={flags[current.id] !== undefined} onCheckedChange={(checked) => toggleFlag(current.id, checked === true)} />Tandai salah, minta diganti
                  </Label>
                  {flags[current.id] !== undefined ? <Input autoFocus value={flags[current.id]} maxLength={300} placeholder="Alasan, mis. bukan foto rakor" aria-label="Alasan revisi foto"
                    onChange={(e) => setFlags((value) => ({ ...value, [current.id]: e.target.value }))} /> : null}
                </div> : null}
              </section> : null}
              {album.data?.slots ? <section>
                <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">Daftar dokumen ({album.data.slots.length - missingSlots.length}/{album.data.slots.length})</h3>
                <ul className="space-y-0.5 text-xs">{album.data.slots.map((slot) => <li key={slot.name} className={cn('flex justify-between gap-2', slot.files === 0 && 'text-destructive')}>
                  <span className="truncate">{slot.name}</span><span className="shrink-0">{slot.files ? `${slot.files} file` : 'belum ada'}</span>
                </li>)}</ul>
              </section> : null}
              {flagged.length > 0 ? <section>
                <h3 className="mb-1 text-xs font-semibold text-destructive uppercase">Foto ditandai ({flagged.length})</h3>
                <ul className="space-y-1">{flagged.map((id) => <li key={id}>
                  <button type="button" className="w-full rounded-md bg-destructive/5 px-2 py-1 text-left text-xs hover:bg-destructive/10" onClick={() => setIndex(photos.findIndex((p) => p.id === id))}>
                    Foto {photos.findIndex((p) => p.id === id) + 1}{flags[id] ? `: ${flags[id]}` : ''}
                  </button>
                </li>)}</ul>
              </section> : null}
            </div>
            {canReview ? <div className="space-y-2 border-t p-3">
              {flagged.length > 0 ? <Input value={note} onChange={(e) => setNote(e.target.value)} maxLength={300} placeholder="Catatan revisi (opsional)" aria-label="Catatan revisi" /> : null}
              {confirming ? <div className="space-y-2 rounded-lg bg-destructive/5 p-2.5 text-xs">
                <p>Pengunggah dan leader kabupaten mendapat notifikasi untuk mengganti {flagged.length} foto.</p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" className="flex-1" onClick={() => setConfirming(false)} disabled={busy}>Batal</Button>
                  <Button variant="destructive" size="sm" className="flex-1" onClick={() => revise.mutate()} disabled={busy}><RotateCcw />Kirim revisi</Button>
                </div>
              </div> : <div className="flex gap-2">
                <Button variant="outline" className="flex-1" disabled={busy || flagged.length === 0} onClick={() => setConfirming(true)}><Flag />Kirim revisi{flagged.length ? ` (${flagged.length})` : ''}</Button>
                <Button className="flex-1" disabled={busy || fresh.length === 0} onClick={() => approve.mutate(fresh.map((p) => p.id))} title="Pintasan: Shift + A">
                  <CheckCheck />{fresh.length ? `Setujui ${fresh.length} foto baru` : 'Semua sudah dicek'}
                </Button>
              </div>}
              <p className="text-center text-[11px] text-muted-foreground">← → ganti foto · A setujui foto · Shift+A setujui semua baru · scroll zoom</p>
            </div> : null}
          </aside>
        </div>}
    </DialogContent>
  </Dialog>;
}
