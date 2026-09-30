import { Camera, CheckCircle2, CircleAlert, ImagePlus, RefreshCw, Trash2, UploadCloud } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { DataResponse, MediaFile, SlotSummary } from './types';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

type PendingFile = { file: File; source: 'camera' | 'gallery'; previewURL: string };

export function DocumentationSlot({ slot, onChanged }: { slot: SlotSummary; onChanged: (slot: SlotSummary) => void }) {
  const canManage = useCan('documentation.manage');
  const [files, setFiles] = useState<MediaFile[]>(slot.files ?? []);
  const [pending, setPending] = useState<PendingFile | null>(null);
  useEffect(() => setFiles(slot.files ?? []), [slot.files]);
  useEffect(() => () => { if (pending?.previewURL) URL.revokeObjectURL(pending.previewURL); }, [pending]);
  const publish = (nextFiles: MediaFile[]) => {
    setFiles(nextFiles);
    onChanged({ ...slot, files: nextFiles, status: nextFiles.length >= (slot.min_files ?? 0) ? 'complete' : 'missing' });
  };
  const upload = useMutation({
    mutationFn: ({ file, source }: PendingFile) => {
      if (!slot.id) throw new Error('Slot dokumentasi tidak valid');
      const body = new FormData(); body.set('file', file); body.set('source', source);
      if (slot.require_captured_at) body.set('captured_at', new Date().toISOString());
      return apiRequest<DataResponse<MediaFile>>(`/api/v1/distribution/slots/${slot.id}/media`, { method: 'POST', body });
    },
    onSuccess: ({ data }) => { publish([...files, data]); setPending(null); },
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/distribution/media/${id}`, { method: 'DELETE' }),
    onSuccess: (_, id) => publish(files.filter((file) => file.id !== id)),
  });
  const choose = (file: File | undefined, source: 'camera' | 'gallery') => {
    if (!file) return;
    const selected = { file, source, previewURL: URL.createObjectURL(file) };
    setPending(selected); upload.mutate(selected);
  };
  const required = slot.min_files ?? 0;
  const complete = files.length >= required;
  const inputSource = slot.input_source ?? 'both';
  const canAddMore = canManage && files.length < (slot.max_files ?? 1);

  return <article
    aria-label={slot.label}
    className={cn('space-y-3 rounded-lg border bg-card p-4', complete ? 'border-l-2 border-l-primary' : 'border-l-2 border-l-accent-foreground/50')}
  >
    <header className="flex items-start justify-between gap-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h4 className="text-sm font-medium">{slot.label}</h4>
          <Badge variant={slot.required ? 'default' : 'outline'}>{slot.required ? 'Wajib' : 'Opsional'}</Badge>
        </div>
        <p className="text-xs text-muted-foreground">{files.length} dari {required} foto wajib</p>
      </div>
      <span className={cn('inline-flex shrink-0 items-center gap-1 text-xs font-medium', complete ? 'text-primary' : 'text-accent-foreground')}>
        {complete ? <CheckCircle2 className="size-4" aria-hidden="true" /> : <CircleAlert className="size-4" aria-hidden="true" />}
        {complete ? 'Lengkap' : 'Belum lengkap'}
      </span>
    </header>

    {(files.length > 0 || pending) && <div className="grid grid-cols-3 gap-2">
      {files.map((file) => <figure key={file.id} className="relative aspect-4/3 overflow-hidden rounded-md border bg-muted">
        <img src={file.content_url} alt={file.original_filename} className="size-full object-cover" />
        <figcaption className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-1 bg-black/70 px-2 py-1 text-[10px] text-white">
          <span className="truncate">{file.original_filename}</span>
          {canManage && <button className={styles.removeMedia} type="button" aria-label={`Hapus ${file.original_filename}`} title="Hapus foto" onClick={() => remove.mutate(file.id)}><Trash2 className="size-3.5" /></button>}
        </figcaption>
      </figure>)}
      {pending && <figure className="relative aspect-4/3 overflow-hidden rounded-md border bg-muted">
        <img src={pending.previewURL} alt={`Preview ${slot.label}`} className="size-full object-cover opacity-50" />
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/40 text-center text-white">
          {upload.isPending && <UploadCloud className="size-5 animate-pulse" aria-hidden="true" />}
          <span className="text-[10px] font-medium">{upload.isError ? 'Upload gagal' : 'Mengunggah...'}</span>
          {upload.isError && <button className={styles.removeMedia} type="button" aria-label="Coba unggah lagi" title="Coba unggah lagi" onClick={() => upload.mutate(pending)}><RefreshCw className="size-4" /></button>}
        </div>
      </figure>}
    </div>}

    {canAddMore && <div className="flex flex-wrap gap-2">
      {inputSource !== 'gallery' && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><Camera className="size-4" aria-hidden="true" />Buka kamera<input className="sr-only" aria-label="Buka kamera" type="file" accept="image/jpeg,image/png,image/webp" capture="environment" onChange={(event) => choose(event.target.files?.[0], 'camera')} /></label>}
      {inputSource !== 'camera' && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><ImagePlus className="size-4" aria-hidden="true" />Pilih galeri<input className="sr-only" aria-label="Pilih galeri" type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => choose(event.target.files?.[0], 'gallery')} /></label>}
    </div>}
  </article>;
}
