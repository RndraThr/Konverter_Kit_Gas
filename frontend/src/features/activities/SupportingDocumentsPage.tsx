import { lazy, Suspense, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, FileText, LoaderCircle, Trash2, Upload } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Button } from '../../components/ui/button';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { Tabs, TabsList, TabsTrigger } from '../../components/ui/tabs';
import type { DataResponse } from '../programs/types';
import type { ActivityMedia, ActivityMediaPage, ProgramOption, ProgramZone, SupportingDocumentSlot } from './types';

const DocumentScanner = lazy(() => import('./DocumentScanner'));

// Dokumen Pendukung picks Program then Kabupaten, same as the 9 other
// activity modules (ActivityDocumentationPage) — the slot list itself is
// managed per kabupaten in Persiapan Program > Template, independent of
// any one Jadwal.
export function SupportingDocumentsPage({ programId, regencyId }: { programId: string; regencyId: string }) {
  const canManage = useCan('activities.manage');
  const client = useQueryClient();
  const [pickerOpen, setPickerOpen] = useState<'upload' | 'scan' | null>(null);
  const [slotId, setSlotId] = useState('');
  const [useCustomName, setUseCustomName] = useState(false);
  const [customName, setCustomName] = useState('');
  const [scannerOpen, setScannerOpen] = useState(false);
  const [selectedFileName, setSelectedFileName] = useState('');
  const [deleting, setDeleting] = useState<ActivityMedia | null>(null);

  const slots = useQuery({
    queryKey: ['supporting-document-slots', regencyId],
    queryFn: () => apiRequest<DataResponse<SupportingDocumentSlot[]>>(`/api/v1/activities/supporting-documents/slots?regency_id=${encodeURIComponent(regencyId)}`),
    enabled: !!regencyId,
  });
  // Documents of the chosen program only (a kabupaten can run several
  // programs), 100 per page with "Muat lebih banyak".
  const documents = useInfiniteQuery({
    queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId, programId],
    queryFn: ({ pageParam }) => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=${encodeURIComponent(regencyId)}&program_id=${encodeURIComponent(programId)}&page=${pageParam}&page_size=100`),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.data.page * last.data.page_size < last.data.total ? last.data.page + 1 : undefined),
    enabled: !!regencyId,
  });

  const resetPicker = () => { setPickerOpen(null); setSlotId(''); setUseCustomName(false); setCustomName(''); setSelectedFileName(''); };
  const nameReady = useCustomName ? customName.trim() !== '' : slotId !== '';

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append('activity_type', 'dokumen_pendukung');
      form.append('program_id', programId);
      form.append('regency_id', regencyId);
      if (useCustomName) form.append('custom_name', customName.trim());
      else form.append('slot_id', slotId);
      form.append('source', 'gallery');
      form.append('file_size', String(file.size));
      form.append('file', file, file.name);
      return apiRequest<DataResponse<ActivityMedia>>('/api/v1/activities/media', { method: 'POST', body: form });
    },
    onSuccess: () => { resetPicker(); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); toast.success('Dokumen berhasil diunggah.'); },
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => { setDeleting(null); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); toast.success('Dokumen dihapus.'); },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : 'Dokumen belum dapat dihapus.'),
  });

  // Slots that were deactivated by the Admin Kabupaten in Persiapan Program
  // should no longer be offered for new uploads from the gallery.
  const activeSlots = (slots.data?.data ?? []).filter((slot) => slot.is_active);
  const items = documents.data?.pages.flatMap((page) => page.data.items) ?? [];
  const total = documents.data?.pages[0]?.data.total ?? 0;
  const uploadedAt = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));

  return <div className="space-y-4">
    {canManage && <div className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" onClick={() => setPickerOpen('upload')}><Upload aria-hidden="true" />Upload PDF</Button>
      <Button type="button" variant="outline" onClick={() => setPickerOpen('scan')}><Camera aria-hidden="true" />Scan dari Kamera</Button>
    </div>}

    {documents.isError
      ? <DataState kind="error" title="Dokumen pendukung belum dapat dimuat" description="Periksa koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void documents.refetch() }} />
      : documents.isPending
        ? <DataState kind="loading" title="Memuat dokumen pendukung" description="Mengambil data dari kabupaten terpilih." />
        : items.length === 0
          ? <DataState kind="empty" title="Belum ada dokumen pendukung" description={canManage ? 'Unggah atau scan dokumen PDF pertama untuk kabupaten ini.' : 'Belum ada dokumen pendukung yang diunggah.'} />
          : <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{total} dokumen</p>
        <ul className="grid gap-2 sm:grid-cols-2">
          {items.map((item) => <li key={item.id} className="flex items-center justify-between gap-2 rounded-lg border p-3">
            <a href={item.content_url} target="_blank" rel="noreferrer" className="flex min-w-0 items-center gap-2 text-sm"><FileText aria-hidden="true" className="shrink-0 text-primary" /><span className="min-w-0"><span className="block truncate font-medium" title={item.display_name}>{item.display_name}</span><span className="block text-xs text-muted-foreground">{uploadedAt(item.uploaded_at)}</span></span></a>
            {canManage && <Button type="button" size="icon" variant="ghost" aria-label={`Hapus ${item.display_name}`} onClick={() => setDeleting(item)}><Trash2 aria-hidden="true" /></Button>}
          </li>)}
        </ul>
        {documents.hasNextPage && <Button type="button" variant="outline" disabled={documents.isFetchingNextPage} onClick={() => void documents.fetchNextPage()}>{documents.isFetchingNextPage ? 'Memuat...' : `Muat lebih banyak (${items.length} dari ${total})`}</Button>}
      </div>}

    <AlertDialog open={deleting !== null} onOpenChange={(open) => { if (!open && !remove.isPending) setDeleting(null); }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Hapus dokumen?</AlertDialogTitle><AlertDialogDescription>{deleting?.display_name} akan dihapus dari daftar dan arsip. Tindakan ini tidak dapat dibatalkan.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={remove.isPending}>Batal</AlertDialogCancel><AlertDialogAction disabled={remove.isPending} onClick={() => { if (deleting) remove.mutate(deleting.id); }}>{remove.isPending ? 'Menghapus...' : 'Hapus dokumen'}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    {pickerOpen && <Dialog open onOpenChange={(open) => { if (!open) resetPicker(); }}>
      <DialogContent aria-label="Pilih nama dokumen">
        <DialogHeader><DialogTitle>Pilih nama dokumen</DialogTitle></DialogHeader>
        <Tabs value={useCustomName ? 'custom' : 'slot'} onValueChange={(value) => { if (value === 'custom') { setSlotId(''); setUseCustomName(true); } else { setCustomName(''); setUseCustomName(false); } }}>
          <TabsList className="w-full">
            <TabsTrigger value="slot">Dari daftar nama</TabsTrigger>
            <TabsTrigger value="custom">Nama lain</TabsTrigger>
          </TabsList>
        </Tabs>
        {!useCustomName && <Select value={slotId} onValueChange={(value) => setSlotId(value ?? '')} disabled={!activeSlots.length}>
          <SelectTrigger><SelectValue placeholder={activeSlots.length ? 'Pilih nama dokumen' : 'Belum ada nama dokumen untuk kabupaten ini'} /></SelectTrigger>
          <SelectContent>{activeSlots.map((slot) => <SelectItem key={slot.id} value={slot.id}>{slot.name}</SelectItem>)}</SelectContent>
        </Select>}
        {useCustomName && <FormField label="Nama dokumen" name="custom_name" value={customName} onChange={(event) => setCustomName(event.target.value)} hint="Untuk dokumen di luar daftar nama yang sudah disiapkan." />}
        {pickerOpen === 'upload'
          ? <div className="space-y-2">
            <label className="inline-flex h-11 cursor-pointer items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted aria-disabled:pointer-events-none aria-disabled:opacity-50" aria-disabled={!nameReady || upload.isPending}>
              {upload.isPending ? <LoaderCircle className="size-4 animate-spin" aria-hidden="true" /> : <Upload className="size-4" aria-hidden="true" />}
              {upload.isPending ? 'Mengunggah...' : 'Pilih berkas PDF'}
              <input className="sr-only" type="file" aria-label="Pilih berkas PDF" accept="application/pdf" disabled={!nameReady || upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) { setSelectedFileName(file.name); upload.mutate(file); } event.target.value = ''; }} />
            </label>
            {upload.isPending && <p className="truncate text-sm text-muted-foreground">Mengunggah {selectedFileName}...</p>}
          </div>
          : <Button type="button" disabled={!nameReady} onClick={() => { setPickerOpen(null); setScannerOpen(true); }}>Mulai Scan</Button>}
        {upload.isError && <p role="alert" className="text-sm text-destructive">{upload.error instanceof ApiError ? upload.error.message : 'Upload belum dapat disimpan.'}</p>}
      </DialogContent>
    </Dialog>}

    {scannerOpen && <Suspense fallback={null}>
      <DocumentScanner
        programId={programId}
        regencyId={regencyId}
        slotId={useCustomName ? '' : slotId}
        customName={useCustomName ? customName.trim() : ''}
        onDone={() => { setScannerOpen(false); resetPicker(); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); toast.success('Dokumen hasil scan berhasil disimpan.'); }}
        onClose={() => setScannerOpen(false)}
      />
    </Suspense>}
  </div>;
}

// Route-level wrapper: Program then Kabupaten, the same two-step filter the
// 9 other activity modules use (see ActivityDocumentationPage) — the slot
// list itself is managed per kabupaten independent of any Jadwal.
export default function SupportingDocumentsRoute() {
  const [params, setParams] = useSearchParams();
  const programID = params.get('program_id') ?? '';
  const regencyID = params.get('regency_id') ?? '';

  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<ProgramOption[]>>('/api/v1/program-setup/programs') });
  const zones = useQuery({
    queryKey: ['program-setup', 'programs', programID, 'zones'],
    queryFn: () => apiRequest<DataResponse<ProgramZone[]>>(`/api/v1/program-setup/programs/${programID}/zones`),
    enabled: programID !== '',
  });
  const regencies = zones.data?.data.flatMap((zone) => zone.regencies ?? []) ?? [];

  const setProgram = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('program_id', value); else next.delete('program_id'); next.delete('regency_id'); return next; });
  const setRegency = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); return next; });

  return <div className="page">
    <PageHeader title="Dokumen Pendukung" description="Kelola dokumen pendukung PDF (SK, surat tugas, dan sejenisnya) per kabupaten." />

    <section className="grid gap-4 py-5 sm:grid-cols-2" aria-label="Konteks dokumen pendukung">
      <div className="grid gap-1.5">
        <Label id="dokumen-pendukung-program-label">Program</Label>
        <Select disabled={programs.isPending} value={programID} onValueChange={setProgram}>
          <SelectTrigger className="w-full" aria-labelledby="dokumen-pendukung-program-label"><SelectValue placeholder={programs.isPending ? 'Memuat program...' : 'Pilih program'} /></SelectTrigger>
          <SelectContent>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <div className="grid gap-1.5">
        <Label id="dokumen-pendukung-regency-label">Kabupaten</Label>
        <Select disabled={!programID || zones.isPending} value={regencyID} onValueChange={setRegency}>
          <SelectTrigger className="w-full" aria-labelledby="dokumen-pendukung-regency-label"><SelectValue placeholder={!programID ? 'Pilih program terlebih dahulu' : zones.isPending ? 'Memuat kabupaten...' : 'Pilih kabupaten'} /></SelectTrigger>
          <SelectContent>{regencies.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
    </section>

    {!programID || !regencyID
      ? <DataState kind="empty" title="Pilih program dan kabupaten terlebih dahulu" description="Pilih program lalu kabupaten untuk melihat dan mengunggah dokumen pendukung." />
      : <SupportingDocumentsPage programId={programID} regencyId={regencyID} />}
  </div>;
}
