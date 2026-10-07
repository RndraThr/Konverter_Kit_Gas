import { Camera, CheckCircle2, CircleAlert, ImagePlus, RefreshCw, Trash2, UploadCloud, Video, X } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { uploadRequest } from '../../lib/upload';
import { useCan } from '../../lib/permissions';
import type { DataResponse, MediaFile, SlotSummary } from './types';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { MediaPreviewDialog } from '@/components/MediaPreviewDialog';
import { cn } from '@/lib/utils';

type QueuedFile = {
  id: string;
  file: File;
  source: 'camera' | 'gallery';
  previewURL: string;
  status: 'queued' | 'uploading' | 'error';
  progress: number;
  controller: AbortController | null;
  errorMessage?: string;
};
const acceptedImageTypes = new Set(['image/jpeg', 'image/png', 'image/webp']);
const acceptedVideoTypes = new Set(['video/mp4', 'video/webm', 'video/quicktime']);
const imageAccept = 'image/jpeg,image/png,image/webp';
const videoAccept = 'video/mp4,video/webm,video/quicktime';
const MAX_IMAGE_BYTES = 25 * 1024 * 1024;
const MAX_VIDEO_BYTES = 500 * 1024 * 1024;

export function DocumentationSlot({ slot, onChanged }: { slot: SlotSummary; onChanged: (slot: SlotSummary) => void }) {
  const canManage = useCan('documentation.manage');
  const [files, setFiles] = useState<MediaFile[]>(slot.files ?? []);
  const [queue, setQueue] = useState<QueuedFile[]>([]);
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const [fileError, setFileError] = useState('');
  const processingRef = useRef(false);
  const queueRef = useRef<QueuedFile[]>([]);
  useEffect(() => setFiles(slot.files ?? []), [slot.files]);
  useEffect(() => { queueRef.current = queue; }, [queue]);
  useEffect(() => () => queueRef.current.forEach((item) => URL.revokeObjectURL(item.previewURL)), []);
  useEffect(() => { if (previewIndex !== null && previewIndex >= files.length) setPreviewIndex(null); }, [files.length, previewIndex]);

  const publish = (nextFiles: MediaFile[]) => {
    setFiles(nextFiles);
    onChanged({ ...slot, files: nextFiles, status: nextFiles.length >= (slot.min_files ?? 0) ? 'complete' : 'missing' });
  };
  const upload = useMutation({
    mutationFn: (item: QueuedFile) => {
      if (!slot.id) throw new Error('Slot dokumentasi tidak valid');
      const controller = new AbortController();
      setQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, controller } : entry));
      const body = new FormData();
      body.set('source', item.source);
      body.set('file_size', String(item.file.size));
      body.set('captured_at', new Date(item.file.lastModified || Date.now()).toISOString());
      body.set('file', item.file);
      return uploadRequest<DataResponse<MediaFile>>(`/api/v1/distribution/slots/${slot.id}/media`, body, {
        signal: controller.signal,
        onProgress: (progress) => setQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, progress } : entry)),
      });
    },
    onSuccess: ({ data }, item) => {
      publish([...files, data]);
      URL.revokeObjectURL(item.previewURL);
      setQueue((current) => current.filter((entry) => entry.id !== item.id));
    },
    onError: (error, item) => {
      if (error instanceof DOMException && error.name === 'AbortError') return;
      const errorMessage = error instanceof Error ? error.message : 'Media belum dapat diunggah.';
      setQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, status: 'error', errorMessage } : entry));
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/distribution/media/${id}`, { method: 'DELETE' }),
    onSuccess: (_, id) => publish(files.filter((file) => file.id !== id)),
  });

  // Processes the queue one upload at a time so a photo selected mid-upload
  // never overwrites the card tracking the one still in flight. The ref
  // guards against the effect re-firing before upload.isPending flips true.
  useEffect(() => {
    if (processingRef.current) return;
    const next = queue.find((item) => item.status === 'queued');
    if (!next) return;
    processingRef.current = true;
    setQueue((current) => current.map((item) => item.id === next.id ? { ...item, status: 'uploading' } : item));
    upload.mutate(next, { onSettled: () => { processingRef.current = false; } });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queue]);

  const required = slot.min_files ?? 0;
  const maxFiles = slot.max_files ?? 1;
  const complete = files.length >= required;
  const inputSource = slot.input_source ?? 'both';
  const mediaKind = slot.media_kind ?? 'image';
  const allowsImages = mediaKind !== 'video';
  const allowsVideos = mediaKind !== 'image';
  const mediaLabel = mediaKind === 'image' ? 'foto' : mediaKind === 'video' ? 'video' : 'media';
  const acceptedTypes = [allowsImages ? imageAccept : '', allowsVideos ? videoAccept : ''].filter(Boolean).join(',');
  const remainingCapacity = Math.max(0, maxFiles - files.length - queue.length);
  const canAddMore = canManage && remainingCapacity > 0;

  const chooseMany = (fileList: FileList | null | undefined, source: 'camera' | 'gallery') => {
    const incoming = Array.from(fileList ?? []);
    if (!incoming.length) return;
    let remaining = remainingCapacity;
    const accepted: File[] = [];
    let rejectedType = false, rejectedCapacity = false, rejectedImageSize = false, rejectedVideoSize = false;
    for (const file of incoming) {
      const isImage = acceptedImageTypes.has(file.type);
      const isVideo = acceptedVideoTypes.has(file.type);
      if ((!isImage && !isVideo) || (isImage && !allowsImages) || (isVideo && !allowsVideos)) { rejectedType = true; continue; }
      if (isImage && file.size > MAX_IMAGE_BYTES) { rejectedImageSize = true; continue; }
      if (isVideo && file.size > MAX_VIDEO_BYTES) { rejectedVideoSize = true; continue; }
      if (remaining <= 0) { rejectedCapacity = true; continue; }
      accepted.push(file);
      remaining -= 1;
    }
    if (rejectedImageSize) setFileError('Ukuran foto maksimal 25 MiB.');
    else if (rejectedVideoSize) setFileError('Ukuran video maksimal 500 MiB.');
    else if (rejectedType) setFileError(mediaKind === 'video' ? 'Gunakan video MP4, WebM, atau MOV.' : mediaKind === 'image_video' ? 'Gunakan foto JPEG, PNG, WebP atau video MP4, WebM, MOV.' : 'Gunakan file JPEG, PNG, atau WebP.');
    else if (rejectedCapacity) setFileError(`Hanya ${remainingCapacity} ${mediaLabel} lagi yang dapat ditambahkan pada slot ini.`);
    else setFileError('');
    if (!accepted.length) return;
    setQueue((current) => [...current, ...accepted.map((file) => ({ id: crypto.randomUUID(), file, source, previewURL: URL.createObjectURL(file), status: 'queued' as const, progress: 0, controller: null }))]);
  };
  const retry = (id: string) => setQueue((current) => current.map((item) => item.id === id ? { ...item, status: 'queued' as const, progress: 0, controller: null, errorMessage: undefined } : item));
  const cancelQueued = (id: string) => setQueue((current) => {
    const item = current.find((entry) => entry.id === id);
    if (item) { item.controller?.abort(); URL.revokeObjectURL(item.previewURL); }
    return current.filter((entry) => entry.id !== id);
  });
  const uploadError = queue.find((item) => item.status === 'error');

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
        <p className="text-xs text-muted-foreground">{files.length} dari {required} {mediaLabel}{slot.required ? ' wajib' : ''}</p>
      </div>
      <span className={cn('inline-flex shrink-0 items-center gap-1 text-xs font-medium', complete ? 'text-primary' : 'text-accent-foreground')}>
        {complete ? <CheckCircle2 className="size-4" aria-hidden="true" /> : <CircleAlert className="size-4" aria-hidden="true" />}
        {complete ? 'Lengkap' : slot.required ? 'Belum lengkap' : 'Belum diisi'}
      </span>
    </header>

    {(files.length > 0 || queue.length > 0) && <div className="grid grid-cols-3 gap-2">
      {files.map((file, index) => <figure key={file.id} className="relative aspect-4/3 overflow-hidden rounded-md border bg-muted">
        <button type="button" aria-label={`Lihat ${file.original_filename}`} className="absolute inset-0 size-full cursor-zoom-in focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-inset focus-visible:ring-ring" onClick={() => setPreviewIndex(index)}>
          {file.mime_type.startsWith('video/') ? <video src={file.content_url} preload="metadata" className="size-full object-cover" /> : <img src={file.content_url} alt="" className="size-full object-cover" />}
        </button>
        <figcaption className="pointer-events-none absolute inset-x-0 bottom-0 z-10 flex items-center justify-between gap-1 bg-black/70 px-2 py-1 text-[10px] text-white">
          <span className="truncate">{file.original_filename}</span>
          {canManage && <button className={cn(styles.removeMedia, 'pointer-events-auto')} type="button" aria-label={`Hapus ${file.original_filename}`} title="Hapus foto" onClick={() => remove.mutate(file.id)}><Trash2 className="size-3.5" /></button>}
        </figcaption>
      </figure>)}
      {queue.map((item) => <figure key={item.id} className="relative aspect-4/3 overflow-hidden rounded-md border bg-muted">
        {acceptedVideoTypes.has(item.file.type) ? <video src={item.previewURL} aria-label={`Preview ${slot.label}`} preload="metadata" className="size-full object-cover opacity-50" /> : <img src={item.previewURL} alt={`Preview ${slot.label}`} className="size-full object-cover opacity-50" />}
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/40 text-center text-white">
          {item.status === 'uploading' && <UploadCloud className="size-5 animate-pulse" aria-hidden="true" />}
          <span className="text-[10px] font-medium">{item.status === 'error' ? 'Upload gagal' : item.status === 'uploading' ? `${item.progress}%` : 'Menunggu...'}</span>
          {item.status === 'uploading' && <button className={styles.removeMedia} type="button" aria-label="Batalkan unggahan" title="Batalkan unggahan" onClick={() => cancelQueued(item.id)}><X className="size-4" /></button>}
          {item.status === 'error' && <div className="flex items-center gap-1">
            <button className={styles.removeMedia} type="button" aria-label="Coba unggah lagi" title="Coba unggah lagi" onClick={() => retry(item.id)}><RefreshCw className="size-4" /></button>
            <button className={styles.removeMedia} type="button" aria-label="Batalkan unggahan" title="Batalkan unggahan" onClick={() => cancelQueued(item.id)}><X className="size-4" /></button>
          </div>}
        </div>
      </figure>)}
    </div>}

    {canAddMore && <div className="grid gap-2 sm:grid-cols-[auto_minmax(0,1fr)]">
      {inputSource !== 'gallery' && <div className="flex flex-wrap gap-2">
        {allowsImages && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><Camera className="size-4" aria-hidden="true" />Buka kamera<input className="sr-only" aria-label="Buka kamera" type="file" accept={imageAccept} capture="environment" onChange={(event) => { chooseMany(event.target.files, 'camera'); event.target.value = ''; }} /></label>}
        {allowsVideos && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><Video className="size-4" aria-hidden="true" />Rekam video<input className="sr-only" aria-label="Rekam video" type="file" accept={videoAccept} capture="environment" onChange={(event) => { chooseMany(event.target.files, 'camera'); event.target.value = ''; }} /></label>}
      </div>}
      {inputSource !== 'camera' && <div
        role="group"
        aria-label={`Unggah ${mediaLabel} ${slot.label} melalui galeri`}
        className={cn('flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-3 text-center transition-colors', dragging ? 'border-primary bg-primary/10' : 'border-border bg-muted/25')}
        onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
        onDragOver={(event) => { event.preventDefault(); setDragging(true); }}
        onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
        onDrop={(event) => { event.preventDefault(); setDragging(false); chooseMany(event.dataTransfer.files, 'gallery'); }}
      >
        <UploadCloud className="size-5 text-muted-foreground" aria-hidden="true" />
        <span className="text-xs text-muted-foreground">Seret {mediaLabel} ke sini atau</span>
        <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><ImagePlus className="size-4" aria-hidden="true" />Pilih galeri<input className="sr-only" aria-label="Pilih galeri" type="file" accept={acceptedTypes} multiple onChange={(event) => { chooseMany(event.target.files, 'gallery'); event.target.value = ''; }} /></label>
      </div>}
    </div>}
    {uploadError && <p role="alert" className="text-sm text-destructive">{uploadError.file.name}: {uploadError.errorMessage ?? 'Foto belum dapat diunggah.'}</p>}
    {fileError && <p role="alert" className="text-sm text-destructive">{fileError}</p>}
    <MediaPreviewDialog items={files.map((mediaFile) => ({ id: mediaFile.id, url: mediaFile.content_url, title: mediaFile.original_filename, mediaType: mediaFile.mime_type.startsWith('video/') ? 'video' : 'image' }))} index={previewIndex} onIndexChange={setPreviewIndex} />
  </article>;
}
