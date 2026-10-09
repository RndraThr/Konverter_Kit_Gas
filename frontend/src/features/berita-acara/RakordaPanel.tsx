import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, FileCheck2, MapPin, Printer, Save, Settings2, UsersRound } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ApiError, apiBlobRequest, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { uppercaseBusinessText } from '@/lib/text';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { RakordaArchive } from './RakordaArchive';
import type { ScheduleSettings } from './types';

// RakordaConfig mendeskripsikan satu jenis daftar hadir pola RAKORDA yang dicetak kosong,
// diisi tangan di lokasi, lalu hasilnya diunggah (RAKORDA, Sosialisasi). Training 10%/100%
// terisi dari distribusi per tanggal dan memakai TrainingPanel. apiPath adalah segmen rute
// /api/v1/bast/{apiPath}.
export type RakordaConfig = {
  apiPath: 'rakorda' | 'sosialisasi';
  name: string;
  documentTitle: string;
  locationKey: 'rakorda_location' | 'sosialisasi_location';
  rowCountKey: 'rakorda_row_count' | 'sosialisasi_row_count';
  defaultRowCount: number;
  folder: string;
  filenamePrefix: string;
  note?: string;
  /** Lokasi boleh kosong; dicetak titik-titik untuk ditulis tangan (Sosialisasi). */
  locationOptional?: boolean;
};

export const rakordaConfig: RakordaConfig = {
  apiPath: 'rakorda',
  name: 'RAKORDA',
  documentTitle: 'Daftar Hadir RAKORDA',
  locationKey: 'rakorda_location',
  rowCountKey: 'rakorda_row_count',
  defaultRowCount: 45,
  folder: '6. RAKORDA',
  filenamePrefix: 'DAFTAR-HADIR-RAKORDA',
};

type Props = { config?: RakordaConfig; scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

export function RakordaPanel({ config = rakordaConfig, scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [date, setDate] = useState(defaultDate);
  const [location, setLocation] = useState('');
  const [rowCount, setRowCount] = useState(config.defaultRowCount);
  const apiBase = `/api/v1/bast/${config.apiPath}`;

  const settings = useQuery({ queryKey: ['bast', 'schedule-settings', scheduleID], queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`), enabled: programType === 'farmer' });
  useEffect(() => { if (settings.data?.data) { setLocation(settings.data.data[config.locationKey] ?? ''); setRowCount(settings.data.data[config.rowCountKey] || config.defaultRowCount); } }, [settings.data, config]);

  const locationOK = (value: string) => config.locationOptional || value.trim() !== '';
  const validSettings = locationOK(location) && rowCount >= 5 && rowCount <= 200;
  const savedLocation = settings.data?.data[config.locationKey] ?? '';
  const savedRowCount = settings.data?.data[config.rowCountKey] ?? 0;
  const settingsReady = locationOK(savedLocation) && savedRowCount >= 5 && savedRowCount <= 200;
  const preview = useQuery({
    queryKey: ['bast', config.apiPath, 'preview', scheduleID, date, savedLocation, savedRowCount],
    queryFn: () => apiBlobRequest(`${apiBase}/preview`, { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, date }) }),
    enabled: programType === 'farmer' && settingsReady && Boolean(date),
  });

  const saveSettings = useMutation({
    mutationFn: async () => {
      const current = settings.data?.data;
      if (!current) throw new Error('Pengaturan belum dimuat');
      return apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify({ ...current, [config.locationKey]: uppercaseBusinessText(location), [config.rowCountKey]: rowCount, updated_at: undefined }) });
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] }); toast.success(`Pengaturan ${config.name} tersimpan.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title={`${config.name} Nelayan belum tersedia`} description="Format daftar hadir saat ini mengikuti program Petani." />;
  if (settings.isPending) return <DataState kind="loading" title={`Memuat ${config.name}`} description="Mengambil lokasi dan jumlah baris daftar hadir." />;
  if (settings.isError) return <DataState kind="error" title={`${config.name} belum dapat dimuat`} description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => settings.refetch() }} />;
  const downloadPreview = () => {
    if (!preview.data) return;
    const url = URL.createObjectURL(preview.data); const anchor = document.createElement('a'); anchor.href = url; anchor.download = `${config.filenamePrefix}-${regencyName}-${date}.pdf`; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const printPreview = () => {
    if (!preview.data) return;
    const url = URL.createObjectURL(preview.data); const frame = document.createElement('iframe'); frame.style.display = 'none'; frame.src = url; document.body.appendChild(frame); frame.onload = () => { frame.contentWindow?.print(); setTimeout(() => { frame.remove(); URL.revokeObjectURL(url); }, 1000); };
  };

  return <div className="grid gap-5">
    <div className="grid gap-5 xl:grid-cols-[22rem_minmax(0,1fr)]">
      <Card className="self-start">
        <CardHeader className="border-b"><CardTitle className="flex items-center gap-2 text-base"><Settings2 className="size-4 text-primary" aria-hidden="true" />Pengaturan daftar hadir</CardTitle></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <FormField label={`Lokasi ${config.name}`} name={config.locationKey} required={!config.locationOptional} value={location} disabled={!canManage} onChange={(event) => setLocation(uppercaseBusinessText(event.target.value))} hint={config.locationOptional ? 'Dicetak pada header; boleh dikosongkan untuk ditulis tangan.' : 'Dicetak pada header daftar hadir.'} />
          <FormField label="Jumlah baris" name={config.rowCountKey} type="number" min={5} max={200} required value={rowCount} disabled={!canManage} onChange={(event) => setRowCount(Number(event.target.value))} hint="Minimal 5 dan maksimal 200 baris; halaman bertambah otomatis." />
          {config.note && <p className="text-xs leading-5 text-muted-foreground">{config.note}</p>}
          {canManage && <Button disabled={!validSettings || saveSettings.isPending} onClick={() => saveSettings.mutate()}><Save aria-hidden="true" />{saveSettings.isPending ? 'Menyimpan...' : 'Simpan pengaturan'}</Button>}
        </CardContent>
      </Card>

      <Card className="min-w-0">
        <CardHeader className="border-b"><div className="flex flex-wrap items-start justify-between gap-3"><div><CardTitle className="flex items-center gap-2"><UsersRound className="size-5 text-primary" aria-hidden="true" />{config.documentTitle}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · cetak kosong, isi di lokasi, lalu unggah hasilnya.</p></div><Badge variant="outline">{rowCount} baris</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-2 sm:max-w-xs"><Label htmlFor={`${config.apiPath}-date`}>Tanggal kegiatan</Label><div className="relative"><CalendarDays className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input id={`${config.apiPath}-date`} type="date" className="pl-9" value={date} onChange={(event) => setDate(event.target.value)} /></div></div>
          <div className="grid gap-3 rounded-xl border bg-muted/20 p-4 sm:grid-cols-3">
            <div className="flex gap-3"><MapPin className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Lokasi</span><strong className="text-sm">{location || 'Belum diatur'}</strong></div></div>
            <div className="flex gap-3"><UsersRound className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Kapasitas</span><strong className="text-sm">{rowCount} peserta</strong></div></div>
            <div className="flex gap-3"><FileCheck2 className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Penyimpanan</span><strong className="text-sm">{config.folder}</strong></div></div>
          </div>
          {settingsReady && date && <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={!preview.data} onClick={printPreview}><Printer aria-hidden="true" />Cetak</Button>
            <Button variant="outline" disabled={!preview.data} onClick={downloadPreview}><Download aria-hidden="true" />Unduh PDF kosong</Button>
          </div>}
          {settingsReady && date && config.locationOptional && !savedLocation.trim() && <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">Lokasi belum diisi: dicetak titik-titik untuk ditulis tangan.</p>}
          {settingsReady && date && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label={config.documentTitle} />}
        </CardContent>
      </Card>
    </div>

    <RakordaArchive apiPath={config.apiPath} name={config.name} scheduleID={scheduleID} date={date} />
  </div>;
}

const errorMessage = (error: unknown) => error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
