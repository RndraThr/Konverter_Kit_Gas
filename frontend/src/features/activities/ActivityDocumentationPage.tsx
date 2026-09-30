import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, ImageIcon, ImagePlus, PlayCircle, RefreshCw, Trash2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { DataState } from '@/components/DataState';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { PageHeader } from '@/components/PageHeader';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { buildPageItems } from './pagination';
import type { ActivityMedia, ActivityMediaPage, ActivityType, ProgramOption, ProgramZone } from './types';

type PendingFile = { file: File; source: 'camera' | 'gallery'; previewURL: string; programID: string; regencyID: string; activityType: ActivityType };
type ActivityTypeOption = { value: ActivityType; label: string };
const acceptedTypes = 'image/jpeg,image/png,image/webp,video/mp4,video/webm,video/quicktime';

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
  const programID = params.get('program_id') ?? '';
  const regencyID = params.get('regency_id') ?? '';
  const page = Number(params.get('page') ?? '1') || 1;
  const [pending, setPending] = useState<PendingFile | null>(null);
  const [preview, setPreview] = useState<ActivityMedia | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ActivityMedia | null>(null);

  useEffect(() => () => { if (pending?.previewURL) URL.revokeObjectURL(pending.previewURL); }, [pending]);

  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const zones = useQuery({
    queryKey: ['program-setup', 'programs', programID, 'zones'],
    queryFn: () => apiRequest<{ data: ProgramZone[] }>(`/api/v1/program-setup/programs/${programID}/zones`),
    enabled: programID !== '',
  });
  const regencies = zones.data?.data.flatMap((zone) => zone.regencies) ?? [];
  const gallery = useQuery({
    queryKey: ['activities', activityType, programID, regencyID, page],
    queryFn: () => apiRequest<{ data: ActivityMediaPage }>(`/api/v1/activities/media?activity_type=${activityType}&program_id=${programID}&regency_id=${regencyID}&page=${page}&page_size=24`),
    enabled: programID !== '' && regencyID !== '',
  });

  const setProgram = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('program_id', value); else next.delete('program_id'); next.delete('regency_id'); next.delete('page'); return next; });
  const setRegency = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); next.delete('page'); return next; });
  const setPage = (value: number) => setParams((prev) => { const next = new URLSearchParams(prev); next.set('page', String(value)); return next; });

  const upload = useMutation({
    mutationFn: ({ file, source, programID: uploadProgramID, regencyID: uploadRegencyID, activityType: uploadActivityType }: PendingFile) => {
      const body = new FormData();
      body.set('file', file); body.set('source', source); body.set('activity_type', uploadActivityType); body.set('program_id', uploadProgramID); body.set('regency_id', uploadRegencyID);
      return apiRequest<{ data: ActivityMedia }>('/api/v1/activities/media', { method: 'POST', body });
    },
    onSuccess: (_data, variables) => { setPending((current) => current === variables ? null : current); client.invalidateQueries({ queryKey: ['activities', variables.activityType, variables.programID, variables.regencyID] }); toast.success('Dokumentasi berhasil diunggah.'); },
    onError: () => toast.error('Gagal mengunggah dokumentasi.'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => { setPendingDelete(null); setPreview(null); client.invalidateQueries({ queryKey: ['activities', activityType, programID, regencyID] }); toast.success('Dokumentasi dihapus.'); },
    onError: () => toast.error('Dokumentasi belum dapat dihapus.'),
  });

  const choose = (file: File | undefined, source: 'camera' | 'gallery') => {
    if (!file || !programID || !regencyID) return;
    const selected = { file, source, previewURL: URL.createObjectURL(file), programID, regencyID, activityType };
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

    {/* === Filter & Upload Card === */}
    <Card>
      <CardContent className="space-y-4 pt-1">
        {options.length > 1 && <>
          <div className="space-y-1.5">
            <Label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Jenis Kegiatan</Label>
            <Tabs value={activityType} onValueChange={setActivityType}>
              <TabsList variant="line" aria-label={`Jenis ${label}`}>
                {options.map((opt) => <TabsTrigger key={opt.value} value={opt.value} disabled={Boolean(pending)}>{opt.label}</TabsTrigger>)}
              </TabsList>
            </Tabs>
          </div>
          <Separator />
        </>}
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:gap-4">
          <div className="grid min-w-0 flex-1 gap-2">
            <Label id="filter-program-label">Program / Tender</Label>
            <Select disabled={programs.isPending || Boolean(pending)} value={programID} onValueChange={setProgram}>
              <SelectTrigger aria-labelledby="filter-program-label"><SelectValue placeholder={programs.isPending ? 'Memuat program...' : 'Pilih program'} /></SelectTrigger>
              <SelectContent>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <div className="grid min-w-0 flex-1 gap-2">
            <Label id="filter-regency-label">Kabupaten / Kota</Label>
            <Select disabled={!programID || zones.isPending || Boolean(pending)} value={regencyID} onValueChange={setRegency}>
              <SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder={!programID ? 'Pilih program terlebih dahulu' : zones.isPending ? 'Memuat kabupaten...' : 'Pilih kabupaten untuk melihat galeri'} /></SelectTrigger>
              <SelectContent>{regencies.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          {canManage && regencyID && <div className="flex shrink-0 gap-2">
            <label className="inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50"><Camera className="size-4" aria-hidden="true" /><span className="hidden sm:inline">Ambil Foto</span><span className="sm:hidden">Kamera</span><input className="sr-only" type="file" aria-label="Ambil Foto" accept={acceptedTypes} capture="environment" disabled={Boolean(pending)} onChange={(e) => choose(e.target.files?.[0], 'camera')} /></label>
            <label className="inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50"><ImagePlus className="size-4" aria-hidden="true" /><span className="hidden sm:inline">Pilih dari Galeri</span><span className="sm:hidden">Galeri</span><input className="sr-only" type="file" aria-label="Pilih dari Galeri" accept={acceptedTypes} disabled={Boolean(pending)} onChange={(e) => choose(e.target.files?.[0], 'gallery')} /></label>
          </div>}
        </div>
      </CardContent>
    </Card>

    {/* === Content Area === */}
    {programs.isError
      ? <DataState kind="error" title="Daftar program belum dapat dimuat" description="Periksa koneksi, lalu coba muat kembali daftar program." action={{ label: 'Coba lagi', onClick: () => void programs.refetch() }} />
      : programs.isPending
        ? <DataState kind="loading" title="Memuat program" description="Menyiapkan pilihan tender dokumentasi." />
        : !programID
          ? <DataState kind="empty" title="Pilih program terlebih dahulu" description="Pilih program atau tender agar zona dan kabupaten yang sesuai dapat dimuat." />
          : zones.isError
            ? <DataState kind="error" title="Daftar kabupaten belum dapat dimuat" description="Periksa konfigurasi zona program, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => void zones.refetch() }} />
            : zones.isPending
              ? <DataState kind="loading" title="Memuat kabupaten" description="Menyiapkan wilayah dari zona program." />
              : !regencyID
          ? <DataState kind="empty" title="Pilih kabupaten terlebih dahulu" description="Pilih kabupaten atau kota di atas untuk melihat dan mengunggah dokumentasi." />
          : gallery.isError
            ? <DataState kind="error" title="Dokumentasi belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void gallery.refetch() }} />
            : gallery.isPending
              ? <DataState kind="loading" title="Memuat dokumentasi" description="Mengambil data dari kabupaten terpilih." />
              : <>

      {/* Stats Bar */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          <span className="font-semibold tabular-nums text-foreground">{total}</span> dokumentasi
          {totalPages > 1 && <> &middot; halaman <span className="font-medium text-foreground">{page}</span>/{totalPages}</>}
        </p>
      </div>

      {items.length === 0 && !pending
        ? <DataState kind="empty" title="Belum ada dokumentasi" description={canManage ? 'Unggah foto atau video pertama untuk kabupaten ini.' : 'Belum ada dokumentasi kegiatan yang diunggah.'} />
        : <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {pending && <figure className="group relative aspect-square overflow-hidden rounded-lg border-2 border-dashed border-primary/40 bg-primary/5">
            {pending.file.type.startsWith('video/')
              ? <video aria-label="Preview unggahan video" src={pending.previewURL} className="size-full object-cover opacity-50" muted />
              : <img src={pending.previewURL} alt="Preview unggahan" className="size-full object-cover opacity-50" />}
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/30">
              {upload.isPending && <div className="size-6 animate-spin rounded-full border-2 border-white border-t-transparent" />}
              {upload.isPending && <span className="text-xs font-medium text-white">Mengunggah&hellip;</span>}
            </div>
            <figcaption className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-1 bg-black/70 px-2 py-1.5">
              <span className="truncate text-xs text-white">{upload.isError ? 'Upload gagal' : pending.file.name}</span>
              {upload.isError && <span className="flex shrink-0 gap-1">
                <Button type="button" size="xs" variant="secondary" onClick={() => upload.mutate(pending)}><RefreshCw aria-hidden="true" />Coba lagi</Button>
                <Button type="button" size="xs" variant="outline" aria-label="Batalkan" onClick={cancelFailedUpload}><X aria-hidden="true" /></Button>
              </span>}
            </figcaption>
          </figure>}
          {items.map((item) => <button key={item.id} type="button" className="group relative aspect-square overflow-hidden rounded-lg border bg-muted transition-shadow hover:shadow-md hover:ring-2 hover:ring-ring/20 focus-visible:ring-2 focus-visible:ring-ring" onClick={() => setPreview(item)}>
            {item.media_type === 'video'
              ? <><video src={item.content_url} className="size-full object-cover" muted /><PlayCircle aria-hidden="true" className="absolute inset-0 m-auto size-10 text-white drop-shadow-lg transition-transform group-hover:scale-110" /></>
              : <img src={item.content_url} alt={item.display_name} className="size-full object-cover transition-transform duration-200 group-hover:scale-105" loading="lazy" />}
            <span className="absolute inset-x-0 bottom-0 flex items-center gap-1.5 bg-linear-to-t from-black/80 to-transparent px-2.5 py-2 pt-6 text-left text-xs text-white">
              {item.media_type === 'video' ? <PlayCircle className="size-3 shrink-0" /> : <ImageIcon className="size-3 shrink-0" />}
              <span className="truncate">{item.display_name}</span>
            </span>
          </button>)}
        </div>}

      {totalPages > 1 && <nav className="flex flex-wrap items-center justify-center gap-1 pt-2" aria-label={`Pagination ${label}`}>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman pertama" disabled={page <= 1} onClick={() => setPage(1)}><ChevronsLeft /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman sebelumnya" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
        {pageItems.map((item) => typeof item === 'number'
          ? <Button type="button" size="icon-sm" variant={item === page ? 'default' : 'outline'} aria-label={`Halaman ${item}`} aria-current={item === page ? 'page' : undefined} key={item} onClick={() => setPage(item)}>{item}</Button>
          : <span key={item} aria-hidden="true" className="flex size-9 items-center justify-center text-muted-foreground">&hellip;</span>)}
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman berikutnya" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman terakhir" disabled={page >= totalPages} onClick={() => setPage(totalPages)}><ChevronsRight /></Button>
      </nav>}
    </>}

    {/* === Preview Dialog === */}
    <Dialog open={Boolean(preview)} onOpenChange={(open) => { if (!open) setPreview(null); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader><DialogTitle>{preview?.display_name}</DialogTitle></DialogHeader>
        {preview && (preview.media_type === 'video'
          ? <video src={preview.content_url} controls className="max-h-[70vh] w-full rounded-lg" />
          : <img src={preview.content_url} alt={preview.display_name} className="max-h-[70vh] w-full rounded-lg object-contain" />)}
        {canManage && preview && <div className="flex justify-end"><Button type="button" variant="destructive" size="sm" onClick={() => setPendingDelete(preview)}><Trash2 className="size-4" />Hapus Dokumentasi</Button></div>}
      </DialogContent>
    </Dialog>

    {/* === Delete Confirmation === */}
    <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => { if (!open) setPendingDelete(null); }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Hapus {pendingDelete?.display_name}?</AlertDialogTitle><AlertDialogDescription>Dokumentasi ini akan dihapus dan tidak lagi tampil di galeri.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={remove.isPending}>Batal</AlertDialogCancel><AlertDialogAction disabled={remove.isPending} onClick={() => { if (pendingDelete) remove.mutate(pendingDelete.id); }}>{remove.isPending ? 'Menghapus\u2026' : 'Hapus'}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}

