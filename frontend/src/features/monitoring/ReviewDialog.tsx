import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, CheckCheck, ChevronLeft, ChevronRight, CircleCheck, Flag, MapPin, RotateCcw, SkipForward } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useCan } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import type { MediaFile } from '../distribution/types';
import { apiRequest } from '@/lib/api';
import { approveBA, approvePhotos, monitoringKey, requestBARevision, requestRevision, slotKey, useSlotDetail } from './api';
import { BADocument } from './BADocument';
import { stageLabels, stateLabels, type SlotDetail } from './types';
import { ZoomableImage, type ZoomHandle } from './ZoomableImage';
import { useRotateFile } from './useRotateFile';

type Photo = { id: string; url: string; video: boolean; label: string; stage: string; file?: MediaFile & { latitude?: number; longitude?: number } };
const stageOrder = ['mesin', 'dokumen', 'penyerahan'];

/** Every photo of a recipient in POS order, then the recipient's signature. */
/** The recipient again, after a check made outside the slot endpoints (BA). */
const loadSlot = (scheduleId: string, slot: number) =>
  apiRequest<{ data: SlotDetail }>(`/api/v1/monitoring/slots/${slot}?schedule_id=${scheduleId}`).then((r) => r.data);

export function photosOf(detail: SlotDetail): Photo[] {
  const docs = [...detail.slot.documentation].sort((a, b) => stageOrder.indexOf(a.stage) - stageOrder.indexOf(b.stage));
  const photos: Photo[] = docs.flatMap((doc) => (doc.files ?? []).map((file) => ({
    id: file.id, url: file.content_url, video: file.mime_type.startsWith('video/'), label: doc.label, stage: doc.stage, file,
  })));
  if (detail.slot.recipient_signed_at) {
    photos.push({ id: 'signature', url: `/api/v1/distribution/slots/${detail.slot.slot_number}/signature?schedule_id=${encodeURIComponent(detail.slot.schedule_id)}`, video: false, label: 'Tanda tangan penerima', stage: 'dokumen' });
  }
  // Last: the recipient's BA Perorangan, once final.
  if (detail.ba) photos.push({ id: 'ba', url: '', video: false, label: 'BA Perorangan', stage: 'ba' });
  return photos;
}

const formatTime = (value?: string | null) => value ? new Date(value).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-';

function Info({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-2 py-1 text-sm">
    <dt className="text-muted-foreground">{label}</dt>
    <dd className={cn('break-words', mono && 'font-mono text-xs leading-5')}>{children || '-'}</dd>
  </div>;
}

/** The handover's data that can be sent back, with the POS that holds it (flag key: "field:<name>"). */
const dataFields: Record<string, { label: string; stage: string }> = {
  full_name: { label: 'Nama', stage: 'dokumen' },
  nik: { label: 'NIK', stage: 'dokumen' },
  sector_identifier: { label: 'No. kartu tani / KUSUKA', stage: 'dokumen' },
  phone_number: { label: 'No. HP', stage: 'dokumen' },
  address: { label: 'Alamat', stage: 'dokumen' },
  village: { label: 'Desa', stage: 'dokumen' },
  district: { label: 'Kecamatan', stage: 'dokumen' },
  machine_serial_number: { label: 'No. mesin', stage: 'mesin' },
  converter_serial_number: { label: 'No. konkit', stage: 'mesin' },
};
const fieldKey = (field: string) => `field:${field}`;
const fieldOf = (key: string) => (key.startsWith('field:') ? key.slice(6) : null);

/**
 * A piece of the recipient's data with a flag: marked wrong, it goes back to
 * the field with the photos; one still flagged (not corrected yet) shows so.
 */
function FlagInfo({ label, value, mono, flagKey, flags, setFlags, active, canFlag }: {
  label: string; value?: string | null; mono?: boolean; flagKey: string; flags: Record<string, string>;
  setFlags: (update: (value: Record<string, string>) => Record<string, string>) => void; active?: string; canFlag: boolean;
}) {
  const marked = flags[flagKey] !== undefined;
  return <div className={cn('-mx-2 rounded-md px-2', (marked || active !== undefined) && 'bg-destructive/5')}>
    <div className="grid grid-cols-[7.5rem_minmax(0,1fr)_auto] items-start gap-2 py-1 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn('break-words', mono && 'font-mono text-xs leading-5')}>
        {value || '-'}
        {active !== undefined && !marked ? <span className="mt-0.5 block text-xs text-destructive">Masih direvisi{active ? `: ${active}` : ''}</span> : null}
      </dd>
      {canFlag ? <button type="button" title={marked ? 'Batal tandai' : 'Tandai salah, minta diperbaiki'} aria-label={`${marked ? 'Batal tandai' : 'Tandai salah'} ${label}`} aria-pressed={marked}
        className={cn('rounded p-0.5 text-muted-foreground hover:text-destructive', marked && 'text-destructive')}
        onClick={() => setFlags((current) => { const next = { ...current }; if (marked) delete next[flagKey]; else next[flagKey] = active ?? ''; return next; })}>
        <Flag className={cn('size-3.5', marked && 'fill-destructive')} />
      </button> : null}
    </div>
    {marked ? <Input autoFocus className="mb-1.5 h-8 text-xs" value={flags[flagKey]} maxLength={300} placeholder={`Alasan, mis. ${label} salah ketik`} aria-label={`Alasan revisi ${label}`}
      onChange={(e) => { const reason = e.target.value; setFlags((current) => ({ ...current, [flagKey]: reason })); }} /> : null}
  </div>;
}

/**
 * Review of one recipient, photo by photo: a carousel (arrow keys, zoom,
 * thumbnails) on the left, the recipient's data and the photo checks on the
 * right. Photos are approved as they come (POS Mesin may arrive a day early);
 * a new or replaced photo needs its own check. A approves the photo shown,
 * Shift + A every new photo; marked photos go back to the field.
 */
export function ReviewDialog({ scheduleId, slot, onClose, onNext, onStep }: {
  scheduleId: string; slot: number | null; onClose: () => void;
  onNext: (current: number) => void; onStep: (current: number, step: number) => void;
}) {
  const detail = useSlotDetail(scheduleId, slot);
  const canReview = useCan('monitoring.review');
  const queryClient = useQueryClient();
  const [index, setIndex] = useState(0);
  const zoom = useRef<ZoomHandle>(null);
  const [flags, setFlags] = useState<Record<string, string>>({});
  const [note, setNote] = useState('');
  const [confirming, setConfirming] = useState(false);
  const rotation = useRotateFile(scheduleId);

  useEffect(() => { setIndex(0); setFlags({}); setNote(''); setConfirming(false); }, [slot, scheduleId]);

  const data = detail.data;
  const photos = useMemo(() => (data ? photosOf(data) : []), [data]);
  const current = photos[Math.min(index, Math.max(photos.length - 1, 0))];
  // Leaving a rotated file shows its rotated version when coming back.
  const focusRotation = rotation.focus;
  useEffect(() => focusRotation('photo', current?.id), [focusRotation, current?.id]);
  const reviewOf = (id: string) => (id === 'ba' ? data?.ba?.review ?? undefined : data?.reviews[id]);
  const checkable = photos.filter((p) => p.id !== 'signature');
  const flagged = Object.keys(flags);
  const fresh = checkable.filter((p) => !reviewOf(p.id) && flags[p.id] === undefined);
  const approvedCount = checkable.filter((p) => reviewOf(p.id)?.status === 'approved').length;
  const rejectedCount = checkable.filter((p) => reviewOf(p.id)?.status === 'rejected').length;

  /** After a check: show the result, go to the next photo still to check, or the next recipient. */
  const settle = (result: SlotDetail, message: string) => {
    queryClient.setQueryData(slotKey(scheduleId, slot!), result);
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'overview'] });
    void queryClient.invalidateQueries({ queryKey: [...monitoringKey, 'queue', scheduleId] });
    const left = photosOf(result).findIndex((p) => p.id !== 'signature' && !(p.id === 'ba' ? result.ba?.review : result.reviews[p.id]) && flags[p.id] === undefined);
    if (left >= 0) { setIndex(left); toast.success(message); return; }
    if (Object.keys(flags).length === 0) { toast.success(`${message} · nomor bagi #${slot} selesai dicek`); onNext(slot!); return; }
    toast.success(message);
  };
  const approve = useMutation({
    // The BA Perorangan is approved on its own endpoint; then the recipient is read again.
    mutationFn: async (ids: string[]) => {
      if (ids.includes('ba') && data?.ba) await approveBA(scheduleId, [data.ba.key]);
      const photoIds = ids.filter((id) => id !== 'ba');
      return photoIds.length ? approvePhotos(scheduleId, slot!, photoIds) : loadSlot(scheduleId, slot!);
    },
    onSuccess: (result, ids) => settle(result, ids.length === 1 ? 'Foto disetujui' : `${ids.length} foto disetujui`),
    onError: (e) => toast.error(e.message),
  });
  const revise = useMutation({
    mutationFn: async () => {
      if (flags.ba !== undefined && data?.ba) await requestBARevision(scheduleId, [{ key: data.ba.key, reason: flags.ba.trim() }], note.trim());
      const marks = flagged.filter((id) => id !== 'ba').map((id) => {
        const field = fieldOf(id);
        return field ? { field, reason: flags[id].trim() } : { media_id: id, reason: flags[id].trim() };
      });
      return marks.length
        ? requestRevision(scheduleId, slot!, { flags: marks, note: note.trim() })
        : loadSlot(scheduleId, slot!);
    },
    onSuccess: (result) => {
      const count = flagged.length;
      setFlags({}); setNote(''); setConfirming(false);
      queryClient.setQueryData(slotKey(scheduleId, slot!), result);
      void queryClient.invalidateQueries({ queryKey: monitoringKey });
      toast.success(`${count} item dikirim untuk revisi`);
      if (!photosOf(result).some((p) => p.id !== 'signature' && !(p.id === 'ba' ? result.ba?.review : result.reviews[p.id]))) onNext(slot!);
    },
    onError: (e) => toast.error(e.message),
  });
  const busy = approve.isPending || revise.isPending;
  const canApproveCurrent = canReview && !!current && current.id !== 'signature' && reviewOf(current.id)?.status !== 'approved' && flags[current.id] === undefined;
  // The photo itself turns (saved to its file); not the signature, the BA or a video.
  const canRotate = canReview && !!current && !current.video && current.id !== 'signature' && current.id !== 'ba';

  useEffect(() => {
    if (slot === null) return;
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
      if (event.key === 'ArrowRight') setIndex((i) => Math.min(i + 1, photos.length - 1));
      else if (event.key === 'ArrowLeft') setIndex((i) => Math.max(i - 1, 0));
      else if (event.key === '+' || event.key === '=') zoom.current?.zoomIn();
      else if (event.key === '-') zoom.current?.zoomOut();
      else if (event.key === '0') zoom.current?.reset();
      else if ((event.key === 'r' || event.key === 'R') && canRotate) rotation.rotate('photo', current.id, event.shiftKey ? -90 : 90);
      else if ((event.key === 'a' || event.key === 'A') && event.shiftKey && canReview && fresh.length > 0 && !busy) approve.mutate(fresh.map((p) => p.id));
      else if ((event.key === 'a' || event.key === 'A') && canApproveCurrent && !busy) approve.mutate([current.id]);
      else return;
      event.preventDefault();
      event.stopPropagation();
    };
    // Capture phase: the dialog's own focus handling must not take the arrow keys.
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [slot, photos.length, canReview, canApproveCurrent, canRotate, rotation, current, fresh, busy, approve]);

  const toggleFlag = (id: string, on: boolean) => setFlags((value) => {
    const next = { ...value };
    if (on) next[id] = next[id] ?? reviewOf(id)?.reason ?? ''; else delete next[id];
    return next;
  });
  const earliest = stageOrder.find((stage) => photos.some((p) => flags[p.id] !== undefined && p.stage === stage)
    || flagged.some((key) => dataFields[fieldOf(key) ?? '']?.stage === stage)) ?? 'dokumen';
  const s = data?.slot;
  // Data still flagged from an earlier revision (not corrected yet), by field.
  const activeData = Object.fromEntries((s?.revision_flags ?? []).filter((f) => f.field).map((f) => [f.field!, f.reason]));
  const dataRow = (field: string, value?: string | null, mono?: boolean, label = dataFields[field].label) =>
    <FlagInfo key={field} label={label} value={value} mono={mono} flagKey={fieldKey(field)} flags={flags} setFlags={setFlags} active={activeData[field]} canFlag={canReview} />;
  const currentReview = current ? reviewOf(current.id) : undefined;

  return <Dialog open={slot !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
    <DialogContent className="flex h-dvh w-screen max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 ring-0 sm:max-w-none">
      <div className="flex items-center gap-2 border-b px-4 py-2.5 pr-12">
        <Button variant="ghost" size="icon-sm" aria-label="Pelanggan sebelumnya" onClick={() => slot !== null && onStep(slot, -1)}><ChevronLeft /></Button>
        <div className="min-w-0 flex-1">
          <DialogTitle className="truncate">#{slot} · {s?.full_name ?? 'Memuat…'}</DialogTitle>
          <DialogDescription className="truncate text-xs">{data ? `${data.regency_name} · ${checkable.length} foto · ${approvedCount} disetujui · ${fresh.length} baru${rejectedCount ? ` · ${rejectedCount} ditolak` : ''}` : ' '}</DialogDescription>
        </div>
        {data?.state ? <span className={cn('rounded-md px-2 py-0.5 text-xs font-medium', stateLabels[data.state].tone)}>{stateLabels[data.state].label}</span> : null}
        <Button variant="ghost" size="icon-sm" aria-label="Pelanggan berikutnya" onClick={() => slot !== null && onStep(slot, 1)}><ChevronRight /></Button>
      </div>
      {detail.isPending ? <p className="m-auto text-sm text-muted-foreground">Memuat dokumentasi…</p>
        : detail.isError || !data || !s ? <p className="m-auto text-sm text-destructive">{detail.error?.message ?? 'Data tidak ditemukan'}</p>
        : <div className="grid min-h-0 flex-1 lg:grid-cols-[minmax(0,1fr)_400px]">
          <div className="flex min-h-0 flex-col bg-neutral-950">
            <div className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden">
              {current?.id === 'ba' && data.ba ? <BADocument ref={zoom} key={data.ba.key + data.ba.uploaded_at} item={data.ba} />
                : current ? (current.video
                ? <video key={current.id} src={current.url} controls className="max-h-full max-w-full" />
                : <ZoomableImage ref={zoom} src={rotation.src('photo', current.id, current.url)} alt={current.label} rotation={rotation.angle('photo', current.id)}
                  onRotate={canRotate ? (delta) => rotation.rotate('photo', current.id, delta) : undefined} rotating={rotation.saving('photo', current.id)} />)
                : <p className="text-sm text-neutral-400">Belum ada foto</p>}
              {current ? <div className="pointer-events-none absolute top-3 left-3 flex items-center gap-2 rounded-md bg-black/60 px-2.5 py-1 text-xs text-white">
                {stageLabels[current.stage] ?? current.stage} · {current.label} <span className="opacity-70">({index + 1}/{photos.length})</span>
                {currentReview?.status === 'approved' ? <span className="rounded bg-emerald-500 px-1.5 font-semibold">Disetujui</span>
                  : currentReview?.status === 'rejected' ? <span className="rounded bg-destructive px-1.5 font-semibold">Ditolak</span>
                  : current.id !== 'signature' ? <span className="rounded bg-amber-400 px-1.5 font-semibold text-neutral-900">Baru</span> : null}
              </div> : null}
              {current && flags[current.id] !== undefined ? <div className="pointer-events-none absolute inset-0 ring-4 ring-destructive ring-inset" /> : null}
              <Button variant="secondary" size="icon" className="absolute top-1/2 left-3 -translate-y-1/2 opacity-80" aria-label="Foto sebelumnya" disabled={index === 0} onClick={() => setIndex((i) => i - 1)}><ChevronLeft /></Button>
              <Button variant="secondary" size="icon" className="absolute top-1/2 right-3 -translate-y-1/2 opacity-80" aria-label="Foto berikutnya" disabled={index >= photos.length - 1} onClick={() => setIndex((i) => i + 1)}><ChevronRight /></Button>
            </div>
            <div className="flex gap-2 overflow-x-auto border-t border-white/10 p-2">
              {photos.map((photo, i) => {
                const review = reviewOf(photo.id);
                const marked = flags[photo.id] !== undefined;
                return <button key={photo.id} type="button" onClick={() => setIndex(i)} title={photo.label}
                  className={cn('relative size-16 shrink-0 overflow-hidden rounded-md border-2 bg-neutral-800', i === index ? 'border-white' : 'border-transparent opacity-70 hover:opacity-100', marked && 'border-destructive opacity-100')}>
                  {photo.id === 'ba' ? <span className="flex size-full items-center justify-center bg-white text-[11px] font-bold text-neutral-800">BA</span>
                    : photo.video ? <video src={photo.url} preload="metadata" className="size-full object-cover" /> : <img src={photo.url} alt="" loading="lazy" className="size-full object-cover" />}
                  {marked || review?.status === 'rejected' ? <Flag className="absolute top-1 right-1 size-3.5 fill-destructive text-destructive" />
                    : review?.status === 'approved' ? <CircleCheck className="absolute top-1 right-1 size-4 fill-emerald-500 text-white" />
                    : photo.id !== 'signature' ? <span className="absolute top-1 right-1 size-2.5 rounded-full bg-amber-400 ring-2 ring-neutral-900" title="Belum dicek" /> : null}
                  <span className="absolute inset-x-0 bottom-0 truncate bg-black/60 px-1 text-[10px] text-white">{photo.label}</span>
                </button>;
              })}
            </div>
          </div>
          <aside className="flex min-h-0 flex-col border-l">
            <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
              {current ? <section className="rounded-lg border p-3">
                <p className="text-sm font-medium">{current.label}</p>
                <p className="text-xs text-muted-foreground">{stageLabels[current.stage]}{current.file?.captured_at ? ` · diambil ${formatTime(current.file.captured_at)}` : ''}</p>
                {current.file?.latitude != null && current.file.longitude != null ? <a className="mt-1 inline-flex items-center gap-1 text-xs text-primary hover:underline" target="_blank" rel="noreferrer"
                  href={`https://www.google.com/maps?q=${current.file.latitude},${current.file.longitude}`}><MapPin className="size-3" />{current.file.latitude.toFixed(5)}, {current.file.longitude.toFixed(5)}</a> : null}
                {currentReview ? <p className={cn('mt-2 rounded-md px-2 py-1 text-xs', currentReview.status === 'approved' ? 'bg-emerald-50 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' : 'bg-destructive/10 text-destructive')}>
                  {currentReview.status === 'approved' ? 'Disetujui' : `Ditolak${currentReview.reason ? `: ${currentReview.reason}` : ''}`} · {currentReview.reviewed_by} · {formatTime(currentReview.reviewed_at)}
                </p> : null}
                {canReview && current.id !== 'signature' ? <div className="mt-3 space-y-2">
                  <Button size="sm" className="w-full" disabled={busy || !canApproveCurrent} onClick={() => approve.mutate([current.id])} title="Pintasan: A">
                    <Check />{currentReview?.status === 'approved' ? 'Sudah disetujui' : 'Setujui foto ini'}
                  </Button>
                  <Label className="flex items-center gap-2 font-normal">
                    <Checkbox checked={flags[current.id] !== undefined} onCheckedChange={(checked) => toggleFlag(current.id, checked === true)} />Tandai salah, minta diganti
                  </Label>
                  {flags[current.id] !== undefined ? <Input autoFocus value={flags[current.id]} maxLength={300} placeholder="Alasan, mis. KTP buram" aria-label="Alasan revisi foto"
                    onChange={(e) => setFlags((value) => ({ ...value, [current.id]: e.target.value }))} /> : null}
                </div> : null}
              </section> : null}
              {flagged.length > 0 ? <section>
                <h3 className="mb-1 text-xs font-semibold text-destructive uppercase">Ditandai ({flagged.length})</h3>
                <ul className="space-y-1">{flagged.map((id) => {
                  const field = fieldOf(id);
                  const photo = field ? undefined : photos.find((p) => p.id === id);
                  return <li key={id}><button type="button" className="w-full rounded-md bg-destructive/5 px-2 py-1 text-left text-xs hover:bg-destructive/10" onClick={() => { if (!field) setIndex(photos.findIndex((p) => p.id === id)); }}>
                    <span className="font-medium">{field ? `Data · ${dataFields[field]?.label ?? field}` : photo?.label}</span>{flags[id] ? `: ${flags[id]}` : ''}
                  </button></li>;
                })}</ul>
              </section> : null}
              <section>
                <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">Pelanggan</h3>
                <dl>
                  {dataRow('full_name', s.full_name)}
                  {dataRow('nik', s.nik, true)}
                  {dataRow('sector_identifier', s.sector_identifier, true, data.program_type === 'fisherman' ? 'No. KUSUKA' : 'No. kartu tani')}
                  <Info label="Nomor bagi">#{s.slot_number}</Info>
                  {dataRow('village', s.village)}
                  {dataRow('district', s.district)}
                  {dataRow('address', s.address)}
                  {dataRow('phone_number', s.phone_number)}
                </dl>
              </section>
              <section>
                <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">Peralatan</h3>
                <dl>
                  {dataRow('machine_serial_number', s.machine_serial_number, true)}
                  {dataRow('converter_serial_number', s.converter_serial_number, true, 'Konverter')}
                  <Info label="Selang" mono>{s.hose_serial_number}</Info>
                </dl>
              </section>
              <section>
                <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">Penyerahan</h3>
                <dl>
                  <Info label="Tanggal">{s.distribution_date ?? '-'}</Info>
                  <Info label="Selesai">{s.status === 'completed' ? formatTime(data.completed_at) : 'Belum selesai'}</Info>
                  <Info label="Petugas">{data.completed_by}</Info>
                  <Info label="Titik serah">{s.handover_location_name}</Info>
                </dl>
              </section>
            </div>
            {canReview ? <div className="space-y-2 border-t p-3">
              {flagged.length > 0 ? <Input value={note} onChange={(e) => setNote(e.target.value)} maxLength={300} placeholder="Catatan revisi (opsional)" aria-label="Catatan revisi" /> : null}
              {confirming ? <div className="space-y-2 rounded-lg bg-destructive/5 p-2.5 text-xs">
                <p>{s.status === 'completed' && flagged.some((id) => id !== 'ba')
                  ? <>Nomor bagi dibuka lagi di <b>{stageLabels[earliest]}</b> dan BA yang sudah dibuat perlu dibuat ulang. </>
                  : null}Petugas dan leader kabupaten mendapat notifikasi untuk memperbaiki {flagged.length} item.</p>
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
              {fresh.length === 0 && flagged.length === 0 ? <Button variant="ghost" size="sm" className="w-full" onClick={() => slot !== null && onNext(slot)}><SkipForward />Pelanggan berikutnya yang ada foto baru</Button> : null}
              <p className="text-center text-[11px] text-muted-foreground">← → ganti foto · A setujui foto · Shift+A setujui semua baru · R putar · scroll zoom</p>
            </div> : <div className="border-t p-3">
              <Button variant="outline" className="w-full" onClick={() => slot !== null && onNext(slot)}><SkipForward />Berikutnya ada foto baru</Button>
            </div>}
          </aside>
        </div>}
    </DialogContent>
  </Dialog>;
}
