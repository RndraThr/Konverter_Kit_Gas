import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, MapPin, Printer, Save, Settings2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ApiError, apiBlobRequest, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { uppercaseBusinessText } from '@/lib/text';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { RakordaArchive } from './RakordaArchive';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import type { ScheduleSettings, TrainingDate } from './types';

export type TrainingConfig = {
  apiPath: 'training-10' | 'training-100';
  name: string;
  documentTitle: string;
  locationKey: 'training_10_location' | 'training_100_location';
  folder: string;
  filenamePrefix: string;
  /** Penjelasan cara peserta dipilih untuk tanggal tersebut. */
  participantRule: string;
};

type Props = { config: TrainingConfig; scheduleID: string; regencyName: string; programType: ProgramType };
type DataResponse<T> = { data: T };

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { weekday: 'long', day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

/** BA Training per tanggal distribusi (pola Rekap Harian): daftar tanggal di kiri, dokumen tanggal terpilih di kanan. */
export function TrainingPanel({ config, scheduleID, regencyName, programType }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [chosenDate, setChosenDate] = useState('');
  const [location, setLocation] = useState('');
  const [showSettings, setShowSettings] = useState(false);
  const apiBase = `/api/v1/bast/${config.apiPath}`;
  const enabled = programType === 'farmer';

  const settings = useQuery({ queryKey: ['bast', 'schedule-settings', scheduleID], queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`), enabled });
  const savedLocation = settings.data?.data[config.locationKey] ?? '';
  useEffect(() => setLocation(savedLocation), [savedLocation]);
  const dates = useQuery({ queryKey: ['bast', config.apiPath, 'dates', scheduleID], queryFn: () => apiRequest<DataResponse<TrainingDate[]>>(`${apiBase}/dates?schedule_id=${encodeURIComponent(scheduleID)}`), enabled });
  const dateItems = dates.data?.data ?? [];
  const selectedDate = dateItems.some((item) => item.local_date === chosenDate) ? chosenDate : dateItems[0]?.local_date ?? '';
  const selected = dateItems.find((item) => item.local_date === selectedDate);
  const ready = savedLocation.trim() !== '' && selectedDate !== '';
  const preview = useQuery({
    queryKey: ['bast', config.apiPath, 'preview', scheduleID, selectedDate, savedLocation],
    queryFn: () => apiBlobRequest(`${apiBase}/preview`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, date: selectedDate }) }),
    enabled: enabled && ready,
  });
  const saveLocation = useMutation({
    mutationFn: async () => {
      const current = settings.data?.data;
      if (!current) throw new Error('Pengaturan belum dimuat');
      return apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify({ ...current, [config.locationKey]: uppercaseBusinessText(location), updated_at: undefined }) });
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] }); toast.success(`Lokasi ${config.name} tersimpan.`); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.'),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title={`${config.name} Nelayan belum tersedia`} description="Format daftar hadir saat ini mengikuti program Petani." />;
  if (dates.isPending || settings.isPending) return <DataState kind="loading" title={`Memuat ${config.name}`} description="Menyiapkan tanggal distribusi selesai." />;
  if (dates.isError || settings.isError) return <DataState kind="error" title={`${config.name} belum dapat dimuat`} description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => { dates.refetch(); settings.refetch(); } }} />;
  if (dateItems.length === 0) return <DataState kind="empty" title="Belum ada distribusi selesai" description={`Belum ditemukan distribusi selesai untuk ${regencyName}. Tanggal akan muncul otomatis setelah penyerahan diselesaikan.`} />;

  const fileName = `${config.filenamePrefix}-${regencyName}-${selectedDate}.pdf`;
  const downloadPreview = () => {
    if (!preview.data) return;
    const url = URL.createObjectURL(preview.data); const anchor = document.createElement('a'); anchor.href = url; anchor.download = fileName; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const printPreview = () => {
    if (!preview.data) return;
    const url = URL.createObjectURL(preview.data); const frame = document.createElement('iframe'); frame.style.display = 'none'; frame.src = url; document.body.appendChild(frame); frame.onload = () => { frame.contentWindow?.print(); setTimeout(() => { frame.remove(); URL.revokeObjectURL(url); }, 1000); };
  };
  const locationDirty = uppercaseBusinessText(location).trim() !== savedLocation.trim();

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} />}

    <div className="grid gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
      <Card className="self-start">
        <CardHeader><CardTitle className="flex items-center gap-2 text-base"><CalendarDays className="size-4 text-primary" />Tanggal distribusi</CardTitle></CardHeader>
        <CardContent className="grid gap-2">
          {dateItems.map((item) => <button key={item.local_date} type="button" aria-pressed={item.local_date === selectedDate} onClick={() => setChosenDate(item.local_date)}
            className="flex min-h-16 w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/5">
            <span><strong className="block text-sm">{formatLocalDate(item.local_date)}</strong><span className="text-xs text-muted-foreground">{item.participant_count === item.recipient_count ? `${item.recipient_count} penerima` : `${item.participant_count} dari ${item.recipient_count} penerima`}</span></span>
            <Badge variant="secondary" className="shrink-0">{item.participant_count} peserta</Badge>
          </button>)}
          <p className="pt-1 text-xs leading-5 text-muted-foreground">{config.participantRule}</p>
        </CardContent>
      </Card>

      <div className="grid min-w-0 gap-4">
        <Card>
          <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle>{selected ? formatLocalDate(selected.local_date) : config.documentTitle}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{config.documentTitle} · {regencyName} · disimpan ke {config.folder}</p></div><Badge variant="outline">{selected?.participant_count ?? 0} peserta</Badge></div></CardHeader>
          <CardContent className="grid gap-4 pt-5">
            <div className="flex flex-wrap items-end gap-2">
              <div className="grid min-w-64 flex-1 gap-1.5 sm:max-w-md">
                <label htmlFor={`${config.apiPath}-location`} className="flex items-center gap-1.5 text-sm font-medium"><MapPin className="size-3.5 text-primary" aria-hidden="true" />Lokasi kegiatan</label>
                <Input id={`${config.apiPath}-location`} value={location} disabled={!canManage} placeholder="Contoh: Balai Desa Tempe" onChange={(e) => setLocation(uppercaseBusinessText(e.target.value))} />
              </div>
              {canManage && <Button variant="outline" disabled={!location.trim() || !locationDirty || saveLocation.isPending} onClick={() => saveLocation.mutate()}><Save aria-hidden="true" />{saveLocation.isPending ? 'Menyimpan...' : 'Simpan lokasi'}</Button>}
              <div className="flex flex-wrap gap-2 sm:ml-auto">
                <Button variant="outline" disabled={!preview.data} onClick={printPreview}><Printer aria-hidden="true" />Cetak</Button>
                <Button variant="outline" disabled={!preview.data} onClick={downloadPreview}><Download aria-hidden="true" />Unduh PDF</Button>
                <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Penandatangan'}</Button>
              </div>
            </div>
            {!savedLocation.trim() && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">Isi dan simpan lokasi kegiatan untuk menampilkan dokumen.</div>}
            {ready && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label={config.documentTitle} />}
          </CardContent>
        </Card>

        <RakordaArchive apiPath={config.apiPath} name={config.name} scheduleID={scheduleID} date={selectedDate} />
      </div>
    </div>
  </div>;
}
