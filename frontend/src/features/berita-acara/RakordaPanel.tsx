import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, FileCheck2, FileUp, MapPin, Printer, Save, ScanLine, Settings2, Trash2, UsersRound } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
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
import { RakordaScanEditor } from './RakordaScanEditor';
import type { RakordaUpload, ScheduleSettings } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

export function RakordaPanel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const uploadInput = useRef<HTMLInputElement>(null);
  const [date, setDate] = useState(defaultDate);
  const [location, setLocation] = useState('');
  const [rowCount, setRowCount] = useState(45);
  const [showScanner, setShowScanner] = useState(false);

  const settings = useQuery({ queryKey: ['bast', 'schedule-settings', scheduleID], queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`), enabled: programType === 'farmer' });
  useEffect(() => { if (settings.data?.data) { setLocation(settings.data.data.rakorda_location ?? ''); setRowCount(settings.data.data.rakorda_row_count || 45); } }, [settings.data]);
  const uploads = useQuery({ queryKey: ['bast', 'rakorda', 'uploads', scheduleID, date], queryFn: () => apiRequest<DataResponse<RakordaUpload[]>>(`/api/v1/bast/rakorda/uploads?schedule_id=${encodeURIComponent(scheduleID)}&date=${encodeURIComponent(date)}`), enabled: programType === 'farmer' && Boolean(date) });

  const validSettings = location.trim() !== '' && rowCount >= 5 && rowCount <= 200;
  const savedLocation = settings.data?.data.rakorda_location ?? '';
  const savedRowCount = settings.data?.data.rakorda_row_count ?? 0;
  const settingsReady = savedLocation.trim() !== '' && savedRowCount >= 5 && savedRowCount <= 200;
  const preview = useQuery({
    queryKey: ['bast', 'rakorda', 'preview', scheduleID, date, savedLocation, savedRowCount],
    queryFn: () => apiBlobRequest('/api/v1/bast/rakorda/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, date }) }),
    enabled: programType === 'farmer' && settingsReady && Boolean(date),
  });

  const saveSettings = useMutation({
    mutationFn: async () => {
      const current = settings.data?.data;
      if (!current) throw new Error('Pengaturan belum dimuat');
      return apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify({ ...current, rakorda_location: uppercaseBusinessText(location), rakorda_row_count: rowCount, updated_at: undefined }) });
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] }); toast.success('Pengaturan RAKORDA tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) {
        const form = new FormData(); form.set('schedule_id', scheduleID); form.set('date', date); form.set('file', file);
        await apiRequest<DataResponse<RakordaUpload>>('/api/v1/bast/rakorda/uploads', { method: 'POST', body: form });
      }
    },
    onSuccess: (_, files) => { queryClient.invalidateQueries({ queryKey: ['bast', 'rakorda', 'uploads', scheduleID] }); toast.success(`${files.length} berkas RAKORDA berhasil diunggah.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/bast/rakorda/uploads/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'rakorda', 'uploads', scheduleID] }); toast.success('Berkas dihapus.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="RAKORDA Nelayan belum tersedia" description="Format daftar hadir saat ini mengikuti program Petani." />;
  if (settings.isPending) return <DataState kind="loading" title="Memuat RAKORDA" description="Mengambil lokasi dan jumlah baris daftar hadir." />;
  if (settings.isError) return <DataState kind="error" title="RAKORDA belum dapat dimuat" description="Periksa koneksi lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => settings.refetch() }} />;
  const downloadPreview = () => {
    if (!preview.data) return;
    const url = URL.createObjectURL(preview.data); const anchor = document.createElement('a'); anchor.href = url; anchor.download = `DAFTAR-HADIR-RAKORDA-${regencyName}-${date}.pdf`; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
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
          <FormField label="Lokasi RAKORDA" name="rakorda_location" required value={location} disabled={!canManage} onChange={(event) => setLocation(uppercaseBusinessText(event.target.value))} hint="Dicetak pada header daftar hadir." />
          <FormField label="Jumlah baris" name="rakorda_row_count" type="number" min={5} max={200} required value={rowCount} disabled={!canManage} onChange={(event) => setRowCount(Number(event.target.value))} hint="Minimal 5 dan maksimal 200 baris; halaman bertambah otomatis." />
          {canManage && <Button disabled={!validSettings || saveSettings.isPending} onClick={() => saveSettings.mutate()}><Save aria-hidden="true" />{saveSettings.isPending ? 'Menyimpan...' : 'Simpan pengaturan'}</Button>}
        </CardContent>
      </Card>

      <Card className="min-w-0">
        <CardHeader className="border-b"><div className="flex flex-wrap items-start justify-between gap-3"><div><CardTitle className="flex items-center gap-2"><UsersRound className="size-5 text-primary" aria-hidden="true" />Daftar Hadir RAKORDA</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · cetak kosong, isi di lokasi, lalu unggah hasilnya.</p></div><Badge variant="outline">{rowCount} baris</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-2 sm:max-w-xs"><Label htmlFor="rakorda-date">Tanggal kegiatan</Label><div className="relative"><CalendarDays className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" /><Input id="rakorda-date" type="date" className="pl-9" value={date} onChange={(event) => setDate(event.target.value)} /></div></div>
          <div className="grid gap-3 rounded-xl border bg-muted/20 p-4 sm:grid-cols-3">
            <div className="flex gap-3"><MapPin className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Lokasi</span><strong className="text-sm">{location || 'Belum diatur'}</strong></div></div>
            <div className="flex gap-3"><UsersRound className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Kapasitas</span><strong className="text-sm">{rowCount} peserta</strong></div></div>
            <div className="flex gap-3"><FileCheck2 className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><span className="block text-xs text-muted-foreground">Penyimpanan</span><strong className="text-sm">6. RAKORDA</strong></div></div>
          </div>
          {settingsReady && date && <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={!preview.data} onClick={printPreview}><Printer aria-hidden="true" />Cetak</Button>
            <Button variant="outline" disabled={!preview.data} onClick={downloadPreview}><Download aria-hidden="true" />Unduh PDF kosong</Button>
          </div>}
          {settingsReady && date && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="Daftar Hadir RAKORDA" />}
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="flex items-center gap-2 text-base"><FileUp className="size-4 text-primary" aria-hidden="true" />Arsip daftar hadir terisi</CardTitle><p className="mt-1 text-sm text-muted-foreground">Unggah PDF, JPG, atau PNG. Beberapa berkas dapat dipilih sekaligus.</p></div>{canManage && <div className="flex gap-2"><input ref={uploadInput} className="sr-only" type="file" multiple accept="application/pdf,image/jpeg,image/png" onChange={(event) => { const files = Array.from(event.target.files ?? []); if (files.length) upload.mutate(files); event.target.value = ''; }} /><Button variant="outline" disabled={!date || upload.isPending} onClick={() => uploadInput.current?.click()}><FileUp aria-hidden="true" />{upload.isPending ? 'Mengunggah...' : 'Unggah berkas'}</Button><Button aria-expanded={showScanner} onClick={() => setShowScanner((value) => !value)}><ScanLine aria-hidden="true" />{showScanner ? 'Tutup pemindai' : 'Pindai dari foto'}</Button></div>}</div></CardHeader>
      <CardContent className="pt-5">
        {uploads.isPending ? <DataState kind="loading" title="Memuat arsip" description="Mengambil berkas RAKORDA." /> : uploads.isError ? <DataState kind="error" title="Arsip gagal dimuat" description="Coba muat ulang." action={{ label: 'Coba lagi', onClick: () => uploads.refetch() }} /> : (uploads.data?.data.length ?? 0) === 0 ? <DataState kind="empty" title="Belum ada dokumen terisi" description="Setelah daftar hadir ditandatangani, unggah hasil pindai atau fotonya di sini." /> : <div className="grid gap-2">{uploads.data!.data.map((item) => <div key={item.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"><div className="min-w-0"><strong className="block truncate text-sm">{item.original_name}</strong><span className="text-xs text-muted-foreground">{formatBytes(item.byte_size)} · {new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(item.created_at))}</span></div><div className="flex gap-1"><Button nativeButton={false} render={<a href={`/api/v1/bast/rakorda/uploads/${item.id}/content`} />} variant="ghost" size="sm"><Download aria-hidden="true" />Unduh</Button>{canManage && <Button variant="ghost" size="icon-sm" aria-label={`Hapus ${item.original_name}`} disabled={remove.isPending} onClick={() => remove.mutate(item.id)}><Trash2 /></Button>}</div></div>)}</div>}
      </CardContent>
    </Card>

    {showScanner && canManage && <RakordaScanEditor disabled={!date || upload.isPending} onPdfReady={async (file) => upload.mutateAsync([file])} />}
  </div>;
}

const formatBytes = (size: number) => size >= 1_048_576 ? `${(size / 1_048_576).toFixed(1)} MB` : `${Math.max(1, Math.round(size / 1024))} KB`;
const errorMessage = (error: unknown) => error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
