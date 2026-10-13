import { lazy, Suspense, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, FileText, Trash2, Upload } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Button } from '../../components/ui/button';
import { Combobox, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxList } from '../../components/ui/combobox';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../components/ui/dialog';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import type { DataResponse, Schedule } from '../programs/types';
import type { ActivityMedia, ActivityMediaPage, SupportingDocumentSlot } from './types';

const DocumentScanner = lazy(() => import('./DocumentScanner'));

// Dokumen Pendukung is scheduled-scoped (schedule_id is the single gate — see
// the spec), unlike the other 9 activity modules which pick program+regency
// separately. scheduleId/programId/regencyId are supplied by the route-level
// schedule selector below, mirroring the Berita Acara page's schedule combobox.
export function SupportingDocumentsPage({ scheduleId, programId, regencyId }: { scheduleId: string; programId: string; regencyId: string }) {
  const canManage = useCan('activities.manage');
  const client = useQueryClient();
  const [pickerOpen, setPickerOpen] = useState<'upload' | 'scan' | null>(null);
  const [slotId, setSlotId] = useState('');
  const [scannerOpen, setScannerOpen] = useState(false);

  const slots = useQuery({
    queryKey: ['supporting-document-slots', scheduleId],
    queryFn: () => apiRequest<DataResponse<SupportingDocumentSlot[]>>(`/api/v1/activities/supporting-documents/slots?schedule_id=${encodeURIComponent(scheduleId)}`),
    enabled: !!scheduleId,
  });
  const documents = useQuery({
    queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId],
    queryFn: () => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=dokumen_pendukung&regency_id=${encodeURIComponent(regencyId)}`),
    enabled: !!regencyId,
  });

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append('activity_type', 'dokumen_pendukung');
      form.append('schedule_id', scheduleId);
      form.append('slot_id', slotId);
      form.append('source', 'gallery');
      form.append('file_size', String(file.size));
      form.append('file', file, file.name);
      return apiRequest<DataResponse<ActivityMedia>>('/api/v1/activities/media', { method: 'POST', body: form });
    },
    onSuccess: () => { setPickerOpen(null); setSlotId(''); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); },
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/activities/media/${id}`, { method: 'DELETE' }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }),
  });

  // Slots that were deactivated by the Admin Kabupaten in Persiapan Program
  // should no longer accept new uploads from the gallery.
  const activeSlots = (slots.data?.data ?? []).filter((slot) => slot.is_active);
  const items = documents.data?.data.items ?? [];

  return <div className="space-y-4">
    {canManage && <div className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" disabled={!activeSlots.length} onClick={() => setPickerOpen('upload')}><Upload aria-hidden="true" />Upload PDF</Button>
      <Button type="button" variant="outline" disabled={!activeSlots.length} onClick={() => setPickerOpen('scan')}><Camera aria-hidden="true" />Scan dari Kamera</Button>
      {!activeSlots.length && <p className="text-sm text-muted-foreground">Minta Admin Kabupaten menyiapkan daftar nama dokumen di Persiapan Program dulu.</p>}
    </div>}

    {items.length === 0
      ? <DataState kind="empty" title="Belum ada dokumen pendukung" description={canManage ? 'Unggah atau scan dokumen PDF pertama untuk jadwal ini.' : 'Belum ada dokumen pendukung yang diunggah.'} />
      : <ul className="grid gap-2 sm:grid-cols-2">
        {items.map((item) => <li key={item.id} className="flex items-center justify-between gap-2 rounded-lg border p-3">
          <a href={item.content_url} target="_blank" rel="noreferrer" className="flex min-w-0 items-center gap-2 text-sm font-medium"><FileText aria-hidden="true" className="shrink-0" /><span className="truncate">{item.display_name}</span></a>
          {canManage && <Button type="button" size="icon" variant="ghost" aria-label={`Hapus ${item.display_name}`} onClick={() => remove.mutate(item.id)}><Trash2 aria-hidden="true" /></Button>}
        </li>)}
      </ul>}

    {pickerOpen && <Dialog open onOpenChange={(open) => { if (!open) setPickerOpen(null); }}>
      <DialogContent aria-label="Pilih nama dokumen">
        <DialogHeader><DialogTitle>Pilih nama dokumen</DialogTitle></DialogHeader>
        <Select value={slotId} onValueChange={(value) => setSlotId(value ?? '')}>
          <SelectTrigger><SelectValue placeholder="Pilih nama dokumen" /></SelectTrigger>
          <SelectContent>{activeSlots.map((slot) => <SelectItem key={slot.id} value={slot.id}>{slot.name}</SelectItem>)}</SelectContent>
        </Select>
        {pickerOpen === 'upload'
          ? <label className="inline-flex h-11 cursor-pointer items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-3 text-sm font-medium transition-colors hover:bg-muted aria-disabled:pointer-events-none aria-disabled:opacity-50" aria-disabled={!slotId || upload.isPending}>
            <Upload className="size-4" aria-hidden="true" />Pilih berkas PDF
            <input className="sr-only" type="file" aria-label="Pilih berkas PDF" accept="application/pdf" disabled={!slotId || upload.isPending} onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); event.target.value = ''; }} />
          </label>
          : <Button type="button" disabled={!slotId} onClick={() => { setPickerOpen(null); setScannerOpen(true); }}>Mulai Scan</Button>}
        {upload.isError && <p role="alert" className="text-sm text-destructive">{upload.error instanceof ApiError ? upload.error.message : 'Upload belum dapat disimpan.'}</p>}
      </DialogContent>
    </Dialog>}

    {scannerOpen && <Suspense fallback={null}>
      <DocumentScanner
        scheduleId={scheduleId}
        slotId={slotId}
        programId={programId}
        onDone={() => { setScannerOpen(false); setSlotId(''); void client.invalidateQueries({ queryKey: ['activities', 'media', 'dokumen_pendukung', regencyId] }); }}
        onClose={() => setScannerOpen(false)}
      />
    </Suspense>}
  </div>;
}

// Route-level wrapper: Dokumen Pendukung is scoped to a single schedule
// (program + kabupaten are derived from it), so it follows the Berita Acara
// page's schedule combobox pattern instead of the 9 other activity modules'
// separate program/regency filters.
export default function SupportingDocumentsRoute() {
  const [searchParams, setSearchParams] = useSearchParams();
  const schedules = useQuery({
    queryKey: ['program-setup', 'schedules'],
    queryFn: () => apiRequest<DataResponse<Schedule[]>>('/api/v1/program-setup/schedules'),
  });
  const scheduleID = searchParams.get('schedule_id') ?? '';
  const activeSchedules = schedules.data?.data.filter((schedule) => schedule.status === 'active') ?? [];
  const selectedSchedule = activeSchedules.find((schedule) => schedule.id === scheduleID);
  const scheduleOptions = activeSchedules.map((schedule) => ({ value: schedule.id, label: `${schedule.regency?.name ?? ''} / ${schedule.name}` }));
  const selectedOption = scheduleOptions.find((option) => option.value === scheduleID) ?? null;

  const updateSchedule = (value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set('schedule_id', value);
    else next.delete('schedule_id');
    setSearchParams(next, { replace: true });
  };

  return <div className="page">
    <PageHeader title="Dokumen Pendukung" description="Kelola dokumen pendukung PDF (SK, surat tugas, dan sejenisnya) per jadwal program." />

    <section className="grid gap-4 py-5 lg:grid-cols-[minmax(0,28rem)_1fr] lg:items-end" aria-label="Konteks dokumen pendukung">
      <div className="grid gap-1.5">
        <Label id="dokumen-pendukung-schedule-label">Jadwal program</Label>
        <Combobox
          items={scheduleOptions}
          value={selectedOption}
          onValueChange={(option) => updateSchedule(option?.value ?? '')}
          itemToStringLabel={(option) => option.label}
          isItemEqualToValue={(option, value) => option.value === value.value}
        >
          <ComboboxInput
            className="w-full"
            aria-labelledby="dokumen-pendukung-schedule-label"
            disabled={schedules.isPending || schedules.isError}
            placeholder={schedules.isPending ? 'Memuat jadwal...' : schedules.isError ? 'Jadwal tidak tersedia' : 'Cari kabupaten atau jadwal'}
          />
          <ComboboxContent>
            <ComboboxEmpty>Jadwal tidak ditemukan.</ComboboxEmpty>
            <ComboboxList>{(option: { value: string; label: string }) => <ComboboxItem key={option.value} value={option}>{option.label}</ComboboxItem>}</ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
    </section>

    {schedules.isError && <DataState kind="error" title="Jadwal belum dapat dimuat" description="Periksa koneksi atau hak akses, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => schedules.refetch() }} />}

    {!schedules.isError && !selectedSchedule && <DataState kind="empty" title="Pilih jadwal program terlebih dahulu" description="Pilih jadwal program untuk melihat dan mengunggah dokumen pendukung." />}

    {selectedSchedule && <SupportingDocumentsPage scheduleId={selectedSchedule.id} programId={selectedSchedule.program_id} regencyId={selectedSchedule.regency_id} />}
  </div>;
}
