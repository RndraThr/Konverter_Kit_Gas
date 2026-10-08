import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarRange, Download, FileCheck2, RefreshCw, Save, Settings2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import type { AggregateDocument, ScheduleSettings, ServisBerkalaSummary } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };
type ServisDates = Pick<ScheduleSettings, 'servis_1_start' | 'servis_1_end' | 'servis_2_start' | 'servis_2_end'>;

const emptyDates: ServisDates = { servis_1_start: '', servis_1_end: '', servis_2_start: '', servis_2_end: '' };

// Tanggal ISO (YYYY-MM-DD) dapat dibandingkan langsung sebagai string.
const periodValid = (start: string, end: string) => start !== '' && end !== '' && start <= end;

export function ServisBerkalaPanel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [dates, setDates] = useState<ServisDates>(emptyDates);
  const [showSettings, setShowSettings] = useState(false);

  const settings = useQuery({
    queryKey: ['bast', 'schedule-settings', scheduleID],
    queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`),
    enabled: programType === 'farmer',
  });
  useEffect(() => {
    const current = settings.data?.data;
    if (current) setDates({ servis_1_start: current.servis_1_start ?? '', servis_1_end: current.servis_1_end ?? '', servis_2_start: current.servis_2_start ?? '', servis_2_end: current.servis_2_end ?? '' });
  }, [settings.data]);
  const summary = useQuery({
    queryKey: ['bast', 'servis-berkala', 'summary', scheduleID],
    queryFn: () => apiRequest<DataResponse<ServisBerkalaSummary>>(`/api/v1/bast/servis-berkala/summary?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const documents = useQuery({
    queryKey: ['bast', 'servis-berkala', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/servis-berkala/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  const totalPackages = summary.data?.data.total_packages ?? 0;
  const scheduleReady = summary.data?.data.schedule_ready ?? false;
  const status = totalPackages === 0 ? 'no_recipients' : !scheduleReady ? 'servis_schedule_required' : 'ready';
  const datesValid = periodValid(dates.servis_1_start, dates.servis_1_end) && periodValid(dates.servis_2_start, dates.servis_2_end);

  const preview = useQuery({
    queryKey: ['bast', 'servis-berkala', 'preview', scheduleID, documentDate, summary.data?.data.services],
    queryFn: () => apiBlobRequest('/api/v1/bast/servis-berkala/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && status === 'ready' && documentDate !== '',
  });
  const saveDates = useMutation({
    mutationFn: async () => {
      const current = settings.data?.data;
      if (!current) throw new Error('Pengaturan belum dimuat');
      return apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify({ ...current, ...dates, updated_at: undefined }) });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] });
      queryClient.invalidateQueries({ queryKey: ['bast', 'servis-berkala'] });
      toast.success('Jadwal servis berkala tersimpan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/servis-berkala/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'servis-berkala'] });
      toast.success('BA Servis Berkala berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="BA Servis Berkala Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (settings.isPending || summary.isPending) return <DataState kind="loading" title="Memuat BA Servis Berkala" description="Mengambil jadwal servis dan jumlah paket." />;
  if (settings.isError || summary.isError) return <DataState kind="error" title="BA Servis Berkala belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => { settings.refetch(); summary.refetch(); } }} />;

  const versions = documents.data?.data ?? [];
  const activeDoc = versions.find((doc) => doc.status === 'active');
  const setDate = (key: keyof ServisDates) => (value: string) => setDates((current) => ({ ...current, [key]: value }));

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} />}

    <div className="grid gap-5 xl:grid-cols-[22rem_minmax(0,1fr)]">
      <Card className="self-start">
        <CardHeader className="border-b"><CardTitle className="flex items-center gap-2 text-base"><CalendarRange className="size-4 text-primary" aria-hidden="true" />Jadwal servis berkala</CardTitle></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          {([1, 2] as const).map((n) => {
            const start = `servis_${n}_start` as const
            const end = `servis_${n}_end` as const
            return <fieldset key={n} className="rounded-lg border bg-muted/30">
              <legend className="sr-only">Servis ke-{n}</legend>
              <div aria-hidden="true" className="flex items-center gap-2 border-b px-3 py-2 text-sm font-semibold">
                <span className="flex size-5 items-center justify-center rounded-full bg-primary text-xs text-primary-foreground">{n}</span>Servis ke-{n}
              </div>
              <div className="grid gap-3 p-3">
                <FormField label="Mulai" name={start} type="date" required value={dates[start]} disabled={!canManage} onChange={(e) => setDate(start)(e.target.value)} />
                <FormField label="Selesai" name={end} type="date" required value={dates[end]} disabled={!canManage} onChange={(e) => setDate(end)(e.target.value)} />
              </div>
            </fieldset>
          })}
          {!datesValid && <p className="text-xs text-muted-foreground">Isi keempat tanggal; tanggal selesai tidak boleh sebelum tanggal mulai.</p>}
          {canManage && <Button disabled={!datesValid || saveDates.isPending} onClick={() => saveDates.mutate()}><Save aria-hidden="true" />{saveDates.isPending ? 'Menyimpan...' : 'Simpan jadwal'}</Button>}
          <p className="text-xs leading-5 text-muted-foreground">Nama Pelaksana, perusahaan pelaksana, serta nama dan NIP Dinas Pertanian diambil dari Konfigurasi Berita Acara jadwal.</p>
        </CardContent>
      </Card>

      <Card className="min-w-0">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="text-base">BA Agenda Servis Berkala</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · disimpan ke 11. SERVIS BERKALA</p></div><Badge variant="outline">{totalPackages} paket</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-3 sm:grid-cols-[16rem_1fr] sm:items-end">
            <FormField label="Tanggal dokumen" name="servis_document_date" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              {canManage && <Button disabled={finalize.isPending || status !== 'ready' || documentDate === ''} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
              <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan'}</Button>
            </div>
          </div>

          {status !== 'ready' && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{status === 'no_recipients' ? 'Belum ada distribusi selesai pada jadwal ini.' : 'Simpan jadwal servis ke-1 dan ke-2 terlebih dahulu.'}</div>}

          {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} paket</p></div>
            <Button nativeButton={false} render={<a href={`/api/v1/bast/servis-berkala/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
          </div>}

          {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
            <strong className="text-sm">Riwayat versi</strong>
            {versions.filter((doc) => doc.status === 'superseded').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version} · {doc.document_date}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/servis-berkala/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
          </div>}

          {status === 'ready' && documentDate !== '' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="BA Servis Berkala" />}
        </CardContent>
      </Card>
    </div>
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
