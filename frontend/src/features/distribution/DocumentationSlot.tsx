import { Camera, ImagePlus, RefreshCw, Trash2, UploadCloud } from 'lucide-react';
import { useMutation } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import type { DataResponse, MediaFile, SlotSummary } from './types';
import styles from './Distribution.module.css';

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

  return <article className={`${styles.documentationSlot} ${complete ? styles.documentationComplete : styles.documentationMissing}`}>
    <header><div><h4>{slot.label}</h4><span>{files.length} dari {required} foto wajib</span></div><strong>{complete ? 'Lengkap' : 'Belum lengkap'}</strong></header>
    <div className={styles.mediaGrid}>
      {files.map((file) => <figure key={file.id}><img src={file.content_url} alt={file.original_filename} /><figcaption><span>{file.original_filename}</span>{canManage && <button type="button" aria-label={`Hapus ${file.original_filename}`} title="Hapus foto" onClick={() => remove.mutate(file.id)}><Trash2 /></button>}</figcaption></figure>)}
      {pending && <figure className={styles.pendingMedia}><img src={pending.previewURL} alt={`Preview ${slot.label}`} /><figcaption><span>{upload.isError ? 'Upload gagal' : 'Mengunggah...'}</span>{upload.isError && <button type="button" aria-label="Coba unggah lagi" title="Coba unggah lagi" onClick={() => upload.mutate(pending)}><RefreshCw /></button>}</figcaption>{upload.isPending && <UploadCloud className={styles.uploadingIcon} aria-hidden="true" />}</figure>}
    </div>
    {canManage && files.length < (slot.max_files ?? 1) && <div className={styles.captureActions}>
      {(slot.input_source ?? 'both') !== 'gallery' && <label className="secondaryButton"><Camera />Buka kamera<input aria-label="Buka kamera" type="file" accept="image/jpeg,image/png,image/webp" capture="environment" onChange={(event) => choose(event.target.files?.[0], 'camera')} /></label>}
      {(slot.input_source ?? 'both') !== 'camera' && <label className="secondaryButton"><ImagePlus />Pilih galeri<input aria-label="Pilih galeri" type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => choose(event.target.files?.[0], 'gallery')} /></label>}
    </div>}
  </article>;
}
