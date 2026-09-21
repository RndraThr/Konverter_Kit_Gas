import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, ImagePlus, PlayCircle, RefreshCw, Trash2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { DataState } from '@/components/DataState';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { PageHeader } from '@/components/PageHeader';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { buildPageItems } from './pagination';
import type { ActivityMedia, ActivityMediaPage, ActivityType, RegencyOption } from './types';

type PendingFile = { file: File; source: 'camera' | 'gallery'; previewURL: string; regencyID: string; activityType: ActivityType };
type ActivityTypeOption = { value: ActivityType; label: string };
const acceptedTypes = 'image/jpeg,image/png,image/webp,video/mp4,video/webm,video/quicktime';
const uploadButtonClass = 'inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-md border bg-secondary px-4 py-2 text-sm font-medium text-secondary-foreground transition-colors hover:bg-secondary/80 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 has-[:disabled]:pointer-events-none has-[:disabled]:opacity-50';

type Props =
  | { label: string; activityType: ActivityType; activityTypes?: undefined }
  | { label: string; activityType?: undefined; activityTypes: ActivityTypeOption[] };

export function ActivityDocumentationPage({ label, ...props }: Props) {
  const canManage = useCan('activities.manage');
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const options: ActivityTypeOption[] = props.activityTypes ?? [{ value: props.activityType, label }];
  const activeOption = options.find((option) => option.value === params.get('type')) ?? options[0];
  const activityType = activeOption.value;
  const setActivityType = (value: string | number) => setParams((prev) => { const next = new URLSearchParams(prev); next.set('type', String(value)); next.delete('page'); return next; });
  const regencyID = params.get('regency_id') ?? '';
  const page = Number(params.get('page') ?? '1') || 1;
  const [pending, setPending] = useState<PendingFile | null>(null);
  const [preview, setPreview] = useState<ActivityMedia | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ActivityMedia | null>(null);

  useEffect(() => () => { if (pending?.previewURL) URL.revokeObjectURL(pending.previewURL); }, [pending]);

  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const gallery = useQuery({
    queryKey: ['activities', activityType, regencyID, page],
    queryFn: () => apiRequest<{ data: ActivityMediaPage }>(`/api/v1/activities/media?activity_type=${activityType}&regency_id=${regencyID}&page=${page}&page_size=24`),
    enabled: regencyID !== '',
  });

  const setRegency = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); next.delete('page'); return next; });
  const setPage = (value: number) => setParams((prev) => { const next = new URLSearchParams(prev); next.set('page', String(value)); return next; });

  const upload = useMutation({
    mutationFn: ({ file, source, regencyID: uploadRegencyID, activityType: uploadActivityType }: PendingFile) => {
      const body = new FormData();
      body.set('file', file); body.set('source', source); body.set('activity_type', uploadActivityType); body.set('regency_id', uploadRegencyID);
      return apiRequest<{ data: ActivityMedia }>('/api/v1/activities/media', { method: 'POST', body });
    },
    onSuccess: (_data, variables) => { setPending((current) => current === variables ? null : current); client.invalidateQueries({ queryKey: ['activities', variables.activityType, variables.regencyID] }); toast.success('Dokumentasi berhasil diunggah.'); },
    onError: () => toast.error('Gagal mengunggah dokumentasi.'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => { setPendingDelete(null); setPreview(null); client.invalidateQueries({ queryKey: ['activities', activityType, regencyID] }); toast.success('Dokumentasi dihapus.'); },
    onError: () => toast.error('Dokumentasi belum dapat dihapus.'),
  });

  const choose = (file: File | undefined, source: 'camera' | 'gallery') => {
    if (!file || !regencyID) return;
    const selected = { file, source, previewURL: URL.createObjectURL(file), regencyID, activityType };
    upload.reset();
    setPending(selected); upload.mutate(selected);
  };

  const cancelFailedUpload = () => {
    upload.reset();
    setPending(null);
  };

  useEffect(() => {
    upload.reset();
    setPending(null);
    setPreview(null);
    setPendingDelete(null);
  }, [activityType]);

  const items = gallery.data?.data.items ?? [];
  const total = gallery.data?.data.total ?? 0;
  const pageSize = gallery.data?.data.page_size ?? 24;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pageItems = buildPageItems(page, totalPages);

  return <div className="space-y-6">
    <PageHeader title={label} description="Dokumentasi foto/video kegiatan lapangan, tidak terikat jadwal." />

    {options.length > 1 && <Tabs value={activityType} onValueChange={setActivityType}>
      <TabsList variant="line" aria-label={`Jenis ${label}`}>
        {options.map((option) => <TabsTrigger key={option.value} value={option.value} disabled={Boolean(pending)}>{option.label}</TabsTrigger>)}
      </TabsList>
    </Tabs>}

    {regencies.isError
      ? <DataState kind="error" title="Daftar kabupaten belum dapat dimuat" description="Periksa koneksi, lalu coba muat kembali daftar kabupaten." action={{ label: 'Coba lagi', onClick: () => regencies.refetch() }} />
      : <>
        <div className="grid max-w-xs gap-2 rounded-xl border bg-card p-4"><Label id="filter-regency-label">Kabupaten</Label><Select disabled={regencies.isPending || Boolean(pending)} value={regencyID} onValueChange={setRegency}><SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder={regencies.isPending ? 'Memuat kabupaten...' : 'Pilih kabupaten'} /></SelectTrigger><SelectContent>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>

        {regencies.isPending ? <DataState kind="loading" title="Memuat kabupaten" description="Menyiapkan pilihan wilayah dokumentasi." /> : !regencyID ? <DataState kind="empty" title="Pilih kabupaten" description="Pilih kabupaten untuk melihat dan mengunggah dokumentasi." /> : <>
      {canManage && <div className="flex flex-wrap gap-2">
        <label className={uploadButtonClass}><Camera aria-hidden="true" />Buka kamera<input aria-label="Buka kamera" type="file" accept={acceptedTypes} capture="environment" className="sr-only" disabled={Boolean(pending)} onChange={(event) => choose(event.target.files?.[0], 'camera')} /></label>
        <label className={uploadButtonClass}><ImagePlus aria-hidden="true" />Pilih galeri<input aria-label="Pilih galeri" type="file" accept={acceptedTypes} className="sr-only" disabled={Boolean(pending)} onChange={(event) => choose(event.target.files?.[0], 'gallery')} /></label>
      </div>}

      {gallery.isError ? <DataState kind="error" title="Dokumentasi belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => gallery.refetch() }} />
        : gallery.isPending ? <DataState kind="loading" title="Memuat dokumentasi" description="Mengambil data dari kabupaten terpilih." />
        : items.length === 0 && !pending ? <DataState kind="empty" title="Belum ada dokumentasi" description="Unggah foto atau video pertama untuk kegiatan ini." />
        : <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
          {pending && <figure className="relative aspect-square overflow-hidden rounded-lg border bg-muted">
            {pending.file.type.startsWith('video/')
              ? <video aria-label="Preview unggahan video" src={pending.previewURL} className="size-full object-cover" muted />
              : <img src={pending.previewURL} alt="Preview unggahan" className="size-full object-cover" />}
            <figcaption className="absolute inset-x-0 bottom-0 bg-black/70 p-2 text-xs text-white">
              <span>{upload.isError ? 'Unggahan gagal' : 'Mengunggah...'}</span>
              {upload.isError && <span className="mt-2 grid grid-cols-2 gap-1.5">
                <Button type="button" size="xs" variant="secondary" aria-label="Coba unggah lagi" onClick={() => upload.mutate(pending)}><RefreshCw aria-hidden="true" />Coba lagi</Button>
                <Button type="button" size="xs" variant="outline" aria-label="Batalkan unggahan" onClick={cancelFailedUpload}><X aria-hidden="true" />Batal</Button>
              </span>}
            </figcaption>
          </figure>}
          {items.map((item) => <button key={item.id} type="button" className="group relative aspect-square overflow-hidden rounded-lg border bg-muted" onClick={() => setPreview(item)}>
            {item.media_type === 'video'
              ? <><video src={item.content_url} className="size-full object-cover" muted /><PlayCircle aria-hidden="true" className="absolute inset-0 m-auto size-8 text-white drop-shadow" /></>
              : <img src={item.content_url} alt={item.display_name} className="size-full object-cover" />}
            <span className="absolute inset-x-0 bottom-0 truncate bg-black/60 px-2 py-1 text-left text-xs text-white">{item.display_name}</span>
          </button>)}
        </div>}

      {totalPages > 1 && <nav className="flex flex-wrap items-center justify-center gap-1" aria-label={`Pagination ${label}`}>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman pertama" disabled={page <= 1} onClick={() => setPage(1)}><ChevronsLeft /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman sebelumnya" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
        {pageItems.map((item) => typeof item === 'number'
          ? <Button type="button" size="icon-sm" variant={item === page ? 'default' : 'outline'} aria-label={`Halaman ${item}`} aria-current={item === page ? 'page' : undefined} key={item} onClick={() => setPage(item)}>{item}</Button>
          : <span key={item} aria-hidden="true" className="flex size-11 items-center justify-center">…</span>)}
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman berikutnya" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman terakhir" disabled={page >= totalPages} onClick={() => setPage(totalPages)}><ChevronsRight /></Button>
      </nav>}
        </>}
      </>}

    <Dialog open={Boolean(preview)} onOpenChange={(open) => { if (!open) setPreview(null); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader><DialogTitle>{preview?.display_name}</DialogTitle></DialogHeader>
        {preview && (preview.media_type === 'video'
          ? <video src={preview.content_url} controls className="max-h-[70vh] w-full rounded-lg" />
          : <img src={preview.content_url} alt={preview.display_name} className="max-h-[70vh] w-full rounded-lg object-contain" />)}
        {canManage && preview && <Button type="button" variant="destructive" onClick={() => setPendingDelete(preview)}><Trash2 />Hapus</Button>}
      </DialogContent>
    </Dialog>

    <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Hapus {pendingDelete?.display_name}?</AlertDialogTitle><AlertDialogDescription>Dokumentasi ini akan dihapus dan tidak lagi tampil di galeri.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={remove.isPending}>Batal</AlertDialogCancel><AlertDialogAction disabled={remove.isPending} onClick={() => { if (pendingDelete) remove.mutate(pendingDelete.id); }}>{remove.isPending ? 'Menghapus...' : 'Hapus'}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
