import { Camera, CheckCircle2, CircleAlert, Clock3, ImagePlus, RefreshCw, Trash2, UploadCloud, Video, X } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { type DragEvent, lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { sameTarget, uploadQueue, useUploadCompletions, useUploadJobs, type UploadJob, type UploadTarget } from '../../lib/uploadQueue';
import { useObjectURL } from '../../lib/useObjectURL';
import type { DataResponse, MediaFile, MediaStorageState, SlotSummary } from './types';
import styles from './Distribution.module.css';
import { Badge } from '@/components/ui/badge';
import { MediaPreviewDialog } from '@/components/MediaPreviewDialog';
import { cn } from '@/lib/utils';
import type { CapturedPhoto } from '../camera/types';
import { shrinkGalleryImage } from '../camera/shrinkGalleryImage';
import { useWatermarkProgram } from '../camera/WatermarkProgramContext';

const WatermarkCamera = lazy(() => import('../camera/WatermarkCamera'));

type Incoming = { file: File; source: 'camera' | 'gallery'; capturedAt: Date; latitude: number | null; longitude: number | null };
const acceptedImageTypes = new Set(['image/jpeg', 'image/png', 'image/webp']);
const acceptedVideoTypes = new Set(['video/mp4', 'video/webm', 'video/quicktime']);
const imageAccept = 'image/jpeg,image/png,image/webp';
const videoAccept = 'video/mp4,video/webm,video/quicktime';
const MAX_IMAGE_BYTES = 25 * 1024 * 1024;
const MAX_VIDEO_BYTES = 500 * 1024 * 1024;
const LOCATION_REQUIRED_CAMERA = 'Slot ini mewajibkan lokasi GPS. Aktifkan izin lokasi lalu ambil ulang foto.';
const LOCATION_REQUIRED_GALLERY = 'Slot ini mewajibkan foto dari kamera dengan lokasi GPS.';

type Props = {
  slot: SlotSummary;
  canManage: boolean;
  canRetryMove?: boolean;
  onChanged: (slot: SlotSummary) => void;
  onRetryMove?: (file: MediaFile) => void;
};

const storageStateCopy: Record<MediaStorageState, { label: string; detail?: string }> = {
  staging: { label: 'Tersimpan sementara', detail: 'Menunggu tanggal dan penerima' },
  moving: { label: 'Sedang diunggah ke Drive' },
  final: { label: 'Sudah diunggah ke Drive' },
  move_failed: { label: 'Pemindahan gagal — akan dicoba kembali' },
};

export function DocumentationSlot({ slot, canManage, canRetryMove = false, onChanged, onRetryMove }: Props) {
  const [files, setFiles] = useState<MediaFile[]>(slot.files ?? []);
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const [fileError, setFileError] = useState('');
  const [moveError, setMoveError] = useState('');
  const [cameraOpen, setCameraOpen] = useState(false);
  const programId = useWatermarkProgram();
  const filesRef = useRef(files);
  useEffect(() => setFiles(slot.files ?? []), [slot.files]);
  useEffect(() => { filesRef.current = files; }, [files]);
  useEffect(() => { if (previewIndex !== null && previewIndex >= files.length) setPreviewIndex(null); }, [files.length, previewIndex]);

  // Unggahan berjalan di antrian aplikasi (lib/uploadQueue), jadi tetap
  // diproses saat halaman ini ditinggalkan atau dimuat ulang.
  const target = useMemo<UploadTarget | null>(() => (slot.id ? { kind: 'distribution-slot', slotId: slot.id } : null), [slot.id]);
  const allJobs = useUploadJobs();
  const queue = useMemo(() => (target ? allJobs.filter((job) => sameTarget(job.target, target)) : []), [allJobs, target]);

  const publish = (nextFiles: MediaFile[]) => {
    filesRef.current = nextFiles;
    setFiles(nextFiles);
    onChanged({ ...slot, files: nextFiles, status: nextFiles.length >= (slot.min_files ?? 0) ? 'complete' : 'missing' });
  };
  useUploadCompletions(({ job, response }) => {
    if (!target || !sameTarget(job.target, target)) return;
    const data = (response as DataResponse<MediaFile> | undefined)?.data;
    if (data && !filesRef.current.some((file) => file.id === data.id)) publish([...filesRef.current, data]);
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/distribution/media/${id}`, { method: 'DELETE' }),
    onSuccess: (_, id) => publish(files.filter((file) => file.id !== id)),
  });
  const retryMove = useMutation({
    mutationFn: (id: string) => apiRequest<DataResponse<MediaFile>>(`/api/v1/distribution/media/${id}/retry-move`, { method: 'POST' }),
    onSuccess: ({ data }) => {
      setMoveError('');
      publish(files.map((file) => file.id === data.id ? data : file));
      onRetryMove?.(data);
    },
    onError: (cause) => setMoveError(cause instanceof Error ? cause.message : 'Pemindahan media belum dapat dicoba kembali.'),
  });

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

  const requireLocation = slot.require_location === true;

  // Validates and queues files. Capacity is read from the queue at call time, because
  // gallery images arrive after an async resize and the render-time value may be stale by then.
  const enqueue = (incoming: Incoming[]) => {
    if (!incoming.length || !target) return;
    const queued = uploadQueue.getSnapshot().filter((job) => sameTarget(job.target, target)).length;
    let remaining = Math.max(0, maxFiles - filesRef.current.length - queued);
    const capacity = remaining;
    const accepted: Incoming[] = [];
    let rejectedType = false, rejectedCapacity = false, rejectedImageSize = false, rejectedVideoSize = false, rejectedLocation = false;
    for (const item of incoming) {
      const { file } = item;
      const isImage = acceptedImageTypes.has(file.type);
      const isVideo = acceptedVideoTypes.has(file.type);
      if ((!isImage && !isVideo) || (isImage && !allowsImages) || (isVideo && !allowsVideos)) { rejectedType = true; continue; }
      if (isImage && requireLocation && item.source === 'gallery') { rejectedLocation = true; continue; }
      if (isImage && file.size > MAX_IMAGE_BYTES) { rejectedImageSize = true; continue; }
      if (isVideo && file.size > MAX_VIDEO_BYTES) { rejectedVideoSize = true; continue; }
      if (remaining <= 0) { rejectedCapacity = true; continue; }
      accepted.push(item);
      remaining -= 1;
    }
    if (rejectedLocation) setFileError(LOCATION_REQUIRED_GALLERY);
    else if (rejectedImageSize) setFileError('Ukuran foto maksimal 25 MiB.');
    else if (rejectedVideoSize) setFileError('Ukuran video maksimal 500 MiB.');
    else if (rejectedType) setFileError(mediaKind === 'video' ? 'Gunakan video MP4, WebM, atau MOV.' : mediaKind === 'image_video' ? 'Gunakan foto JPEG, PNG, WebP atau video MP4, WebM, MOV.' : 'Gunakan file JPEG, PNG, atau WebP.');
    else if (rejectedCapacity) setFileError(`Hanya ${capacity} ${mediaLabel} lagi yang dapat ditambahkan pada slot ini.`);
    else setFileError('');
    if (!accepted.length) return;
    for (const item of accepted) {
      const fields: [string, string][] = [['source', item.source], ['file_size', String(item.file.size)], ['captured_at', item.capturedAt.toISOString()]];
      if (item.latitude !== null && item.longitude !== null) fields.push(['latitude', String(item.latitude)], ['longitude', String(item.longitude)]);
      uploadQueue.enqueue({ target, label: `Pendistribusian · ${slot.label}`, fields, file: item.file });
    }
  };

  const fromFiles = (list: File[], source: 'camera' | 'gallery'): Incoming[] =>
    list.map((file) => ({ file, source, capturedAt: new Date(file.lastModified || Date.now()), latitude: null, longitude: null }));

  const chooseMany = (fileList: FileList | null | undefined, source: 'camera' | 'gallery') => {
    const incoming = Array.from(fileList ?? []);
    if (!incoming.length) return;
    // Gallery photos are shrunk before upload (videos untouched); rejected ones are not worth resizing.
    const shrink = source === 'gallery' && !requireLocation;
    if (!shrink || !incoming.some((file) => acceptedImageTypes.has(file.type) && allowsImages)) {
      enqueue(fromFiles(incoming, source));
      return;
    }
    void Promise.all(incoming.map((file) => (acceptedImageTypes.has(file.type) && allowsImages ? shrinkGalleryImage(file) : file)))
      .then((prepared) => enqueue(fromFiles(prepared, source)));
  };

  const capturePhoto = (photo: CapturedPhoto) => {
    setCameraOpen(false);
    if (requireLocation && (photo.latitude === null || photo.longitude === null)) {
      setFileError(LOCATION_REQUIRED_CAMERA);
      return;
    }
    enqueue([{ file: photo.file, source: 'camera', capturedAt: photo.capturedAt, latitude: photo.latitude, longitude: photo.longitude }]);
  };
  const uploadError = queue.find((item) => item.status === 'error');
  const dropHandlers = {
    onDragEnter: (event: DragEvent) => { event.preventDefault(); setDragging(true); },
    onDragOver: (event: DragEvent) => { event.preventDefault(); setDragging(true); },
    onDragLeave: (event: DragEvent) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); },
    onDrop: (event: DragEvent) => { event.preventDefault(); setDragging(false); chooseMany(event.dataTransfer.files, 'gallery'); },
  };

  return <article
    aria-label={slot.label}
    className={cn('space-y-3 rounded-lg border border-l-4 bg-card p-4', complete ? 'border-l-primary' : slot.required ? 'border-l-destructive bg-destructive/[0.03]' : 'border-l-muted-foreground/30')}
  >
    <header className="flex items-start justify-between gap-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h4 className="text-sm font-medium">{slot.label}</h4>
          {slot.required
            ? <Badge className="bg-destructive/10 font-semibold uppercase tracking-wide text-destructive">Wajib</Badge>
            : <Badge variant="outline" className="uppercase tracking-wide text-muted-foreground">Opsional</Badge>}
        </div>
        <p className="text-xs text-muted-foreground">{maxFiles > 1
          ? `${files.length} ${mediaLabel}${required > 0 ? ` · min. ${required}` : ''} · maks. ${maxFiles}`
          : `${files.length} dari ${required} ${mediaLabel}${slot.required ? ' wajib' : ''}`}</p>
      </div>
      <span className={cn('inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold', complete ? 'bg-primary/10 text-primary' : slot.required ? 'bg-destructive/10 text-destructive' : 'bg-muted text-muted-foreground')}>
        {complete ? <CheckCircle2 className="size-4" aria-hidden="true" /> : <CircleAlert className="size-4" aria-hidden="true" />}
        {complete ? 'Lengkap' : slot.required ? 'Belum lengkap' : 'Belum diisi'}
      </span>
    </header>

    {(files.length > 0 || queue.length > 0) && <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
      {files.map((file, index) => <figure key={file.id} className="group relative aspect-square overflow-hidden rounded-lg border bg-muted">
        <button type="button" aria-label={`Lihat ${file.original_filename}`} className="absolute inset-0 size-full cursor-zoom-in focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-inset focus-visible:ring-ring" onClick={() => setPreviewIndex(index)}>
          {file.mime_type.startsWith('video/') ? <video src={file.content_url} preload="metadata" className="size-full object-cover" /> : <img src={file.content_url} alt="" className="size-full object-cover transition-transform duration-200 group-hover:scale-105" />}
        </button>
        <StorageBadge state={file.storage_state} />
        {canManage && <button className={cn(styles.removeMedia, 'absolute top-1 right-1 z-10 size-7! rounded-full! bg-black/60! backdrop-blur-sm hover:bg-destructive!')} type="button" aria-label={`Hapus ${file.original_filename}`} title="Hapus foto" onClick={() => remove.mutate(file.id)}><Trash2 className="size-3.5" aria-hidden="true" /></button>}
      </figure>)}
      {queue.map((item) => <QueuedMedia key={item.id} job={item} slotLabel={slot.label} />)}
      {canAddMore && files.length + queue.length > 0 && <div
        role="group"
        aria-label={`Tambah ${mediaLabel} ${slot.label}`}
        className={cn('flex aspect-square flex-col items-center justify-center gap-1.5 rounded-lg border-2 border-dashed p-1.5 text-muted-foreground transition-colors', dragging ? 'border-primary bg-primary/10' : 'border-border hover:border-primary/50')}
        {...dropHandlers}
      >
        <span className="text-[11px] font-medium">Tambah</span>
        <div className="flex gap-1">
          {inputSource !== 'gallery' && allowsImages && <button type="button" aria-label="Buka kamera" title="Ambil dengan kamera" className={cn(styles.captureControl, 'inline-flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90')} onClick={() => { setFileError(''); setCameraOpen(true); }}><Camera className="size-4" aria-hidden="true" /></button>}
          {inputSource !== 'gallery' && allowsVideos && <label title="Rekam video" className={cn(styles.captureControl, 'inline-flex size-8 shrink-0 items-center justify-center rounded-md cursor-pointer border bg-background hover:bg-muted')}><Video className="size-4" aria-hidden="true" /><input className="sr-only" aria-label="Rekam video" type="file" accept={videoAccept} capture="environment" onChange={(event) => { chooseMany(event.target.files, 'camera'); event.target.value = ''; }} /></label>}
          {inputSource !== 'camera' && <label title="Pilih dari galeri" className={cn(styles.captureControl, 'inline-flex size-8 shrink-0 items-center justify-center rounded-md cursor-pointer border bg-background hover:bg-muted')}><ImagePlus className="size-4" aria-hidden="true" /><input className="sr-only" aria-label="Pilih galeri" type="file" accept={acceptedTypes} multiple onChange={(event) => { chooseMany(event.target.files, 'gallery'); event.target.value = ''; }} /></label>}
        </div>
        <span className="text-[10px]">{remainingCapacity} lagi</span>
      </div>}
    </div>}
    {files.length > 0 && <StorageSummary files={files} />}
    {files.filter((file) => file.storage_state === 'move_failed').map((file) => <div key={file.id} className="flex flex-wrap items-center gap-2 rounded-md bg-destructive/5 px-3 py-2 text-xs text-destructive">
      <CircleAlert className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="min-w-0 flex-1">{file.original_filename}: {file.storage_last_error || storageStateCopy.move_failed.label}</span>
      {canRetryMove && <button type="button" className="inline-flex min-h-11 items-center gap-1 font-medium text-primary underline-offset-4 hover:underline disabled:opacity-50" aria-label={`Coba pindahkan lagi ${file.original_filename}`} disabled={retryMove.isPending} onClick={() => { setMoveError(''); retryMove.mutate(file.id); }}><RefreshCw className="size-3.5" aria-hidden="true" />Coba lagi</button>}
    </div>)}

    {canAddMore && files.length + queue.length === 0 && <div
      role="group"
      aria-label={`Unggah ${mediaLabel} ${slot.label} melalui galeri`}
      className={cn('flex flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed px-4 py-5 text-center transition-colors', dragging ? 'border-primary bg-primary/10' : 'border-border bg-muted/25')}
      {...dropHandlers}
    >
      <UploadCloud className="size-6 text-muted-foreground" aria-hidden="true" />
      <span className="text-xs text-muted-foreground">{inputSource === 'camera' ? `Ambil ${mediaLabel} dengan kamera` : `Ambil dengan kamera, pilih dari galeri, atau seret ${mediaLabel} ke sini`}</span>
      <div className="flex flex-wrap justify-center gap-2">
        {inputSource !== 'gallery' && allowsImages && <button type="button" className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90')} onClick={() => { setFileError(''); setCameraOpen(true); }}><Camera className="size-4" aria-hidden="true" />Buka kamera</button>}
        {inputSource !== 'gallery' && allowsVideos && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><Video className="size-4" aria-hidden="true" />Rekam video<input className="sr-only" aria-label="Rekam video" type="file" accept={videoAccept} capture="environment" onChange={(event) => { chooseMany(event.target.files, 'camera'); event.target.value = ''; }} /></label>}
        {inputSource !== 'camera' && <label className={cn(styles.captureControl, 'inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted')}><ImagePlus className="size-4" aria-hidden="true" />Pilih galeri<input className="sr-only" aria-label="Pilih galeri" type="file" accept={acceptedTypes} multiple onChange={(event) => { chooseMany(event.target.files, 'gallery'); event.target.value = ''; }} /></label>}
      </div>
    </div>}
    {uploadError && <p role="alert" className="text-sm text-destructive">{uploadError.file.name}: {uploadError.errorMessage ?? 'Foto belum dapat diunggah.'}</p>}
    {fileError && <p role="alert" className="text-sm text-destructive">{fileError}</p>}
    {moveError && <p role="alert" className="text-sm text-destructive">{moveError}</p>}
    {cameraOpen && <Suspense fallback={null}><WatermarkCamera title={slot.label} programId={programId} onCapture={capturePhoto} onClose={() => setCameraOpen(false)} /></Suspense>}
    <MediaPreviewDialog items={files.map((mediaFile) => ({ id: mediaFile.id, url: mediaFile.content_url, title: mediaFile.original_filename, mediaType: mediaFile.mime_type.startsWith('video/') ? 'video' : 'image' }))} index={previewIndex} onIndexChange={setPreviewIndex} />
  </article>;
}

/** Where a photo is: a small corner mark, the words are in the title and the summary below. */
function StorageBadge({ state }: { state: MediaStorageState }) {
  const copy = storageStateCopy[state];
  const [Icon, tone] = state === 'final' ? [CheckCircle2, 'bg-primary text-primary-foreground'] : state === 'move_failed' ? [CircleAlert, 'bg-destructive text-white'] : [Clock3, 'bg-amber-500 text-white'];
  return <span title={copy.detail ? `${copy.label} · ${copy.detail}` : copy.label} className={cn('pointer-events-none absolute bottom-1 left-1 z-10 grid size-5 place-items-center rounded-full shadow-sm', tone)}>
    <Icon className="size-3" aria-hidden="true" /><span className="sr-only">{copy.label}</span>
  </span>;
}

/** One line for the slot instead of a caption under every photo. */
function StorageSummary({ files }: { files: MediaFile[] }) {
  const counts = files.reduce<Record<string, number>>((acc, file) => ({ ...acc, [file.storage_state]: (acc[file.storage_state] ?? 0) + 1 }), {});
  if (counts.final === files.length) return <p className="flex items-center gap-1 text-xs text-muted-foreground"><CheckCircle2 className="size-3.5 text-primary" aria-hidden="true" />{storageStateCopy.final.label}</p>;
  const waiting = (counts.staging ?? 0) + (counts.moving ?? 0);
  if (!waiting) return null;
  return <p className="flex items-center gap-1 text-xs text-muted-foreground"><Clock3 className="size-3.5 text-amber-500" aria-hidden="true" />{counts.moving ? storageStateCopy.moving.label : `${storageStateCopy.staging.label} · ${storageStateCopy.staging.detail}`}</p>;
}

function QueuedMedia({ job, slotLabel }: { job: UploadJob; slotLabel: string }) {
  const previewURL = useObjectURL(job.file);
  return <figure className="relative aspect-square overflow-hidden rounded-lg border bg-muted">
    {previewURL && (acceptedVideoTypes.has(job.file.type) ? <video src={previewURL} aria-label={`Preview ${slotLabel}`} preload="metadata" className="size-full object-cover opacity-50" /> : <img src={previewURL} alt={`Preview ${slotLabel}`} className="size-full object-cover opacity-50" />)}
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/40 text-center text-white">
      {job.status === 'uploading' && <UploadCloud className="size-5 animate-pulse" aria-hidden="true" />}
      <span className="text-[10px] font-medium">{job.status === 'error' ? (job.autoRetry ? 'Gagal, dicoba lagi otomatis' : 'Upload gagal') : job.status === 'uploading' ? `${job.progress}%` : 'Menunggu antrian...'}</span>
      {job.status !== 'error' && <button className={styles.removeMedia} type="button" aria-label="Batalkan unggahan" title="Batalkan unggahan" onClick={() => uploadQueue.cancel(job.id)}><X className="size-4" /></button>}
      {job.status === 'error' && <div className="flex items-center gap-1">
        <button className={styles.removeMedia} type="button" aria-label="Coba unggah lagi" title="Coba unggah lagi" onClick={() => uploadQueue.retry(job.id)}><RefreshCw className="size-4" /></button>
        <button className={styles.removeMedia} type="button" aria-label="Batalkan unggahan" title="Batalkan unggahan" onClick={() => uploadQueue.cancel(job.id)}><X className="size-4" /></button>
      </div>}
    </div>
  </figure>;
}
