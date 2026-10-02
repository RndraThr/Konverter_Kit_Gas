import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, Eye, FileCheck2, RefreshCw, Settings2, Users } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import type { AggregateDocument, DailyRecapDate, DailyRecapRecipient } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType };
type DataResponse<T> = { data: T };

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

export function DailyRecapPanel({ scheduleID, regencyName, programType }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [chosenDate, setChosenDate] = useState('');
  const [showSettings, setShowSettings] = useState(false);
  const [showRecipients, setShowRecipients] = useState(false);

  const dates = useQuery({
    queryKey: ['bast', 'daily-recap', 'dates', scheduleID],
    queryFn: () => apiRequest<DataResponse<DailyRecapDate[]>>(`/api/v1/bast/daily-recap/dates?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const dateItems = dates.data?.data ?? [];
  const selectedDate = dateItems.some((item) => item.local_date === chosenDate) ? chosenDate : dateItems[0]?.local_date ?? '';
  const recipients = useQuery({
    queryKey: ['bast', 'daily-recap', 'recipients', scheduleID, selectedDate],
    queryFn: () => apiRequest<DataResponse<DailyRecapRecipient[]>>(`/api/v1/bast/daily-recap/recipients?schedule_id=${encodeURIComponent(scheduleID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '' && showRecipients,
  });
  const documents = useQuery({
    queryKey: ['bast', 'daily-recap', 'documents', scheduleID, selectedDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/daily-recap/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '',
  });

  const preview = useMutation({
    mutationFn: () => apiBlobRequest('/api/v1/bast/daily-recap/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, local_date: selectedDate }) }),
    onSuccess: (blob) => { const url = URL.createObjectURL(blob); window.open(url, '_blank', 'noopener,noreferrer'); window.setTimeout(() => URL.revokeObjectURL(url), 60_000); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/daily-recap/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, local_date: selectedDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'daily-recap'] });
      toast.success('Rekap Harian berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="Rekap Harian Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (dates.isPending) return <DataState kind="loading" title="Memuat Rekap Harian" description="Menyiapkan tanggal distribusi selesai." />;
  if (dates.isError) return <DataState kind="error" title="Rekap Harian belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => dates.refetch() }} />;
  if (dateItems.length === 0) return <DataState kind="empty" title="Belum ada distribusi selesai" description={`Belum ditemukan distribusi selesai untuk ${regencyName}. Tanggal akan muncul otomatis setelah penyerahan diselesaikan.`} />;

  const selected = dateItems.find((item) => item.local_date === selectedDate);
  const activeDoc = documents.data?.data.find((doc) => doc.status === 'active');
  const versions = documents.data?.data ?? [];
  const recips = recipients.data?.data ?? [];
  const variants = groupVariants(recips);
  const busy = preview.isPending || finalize.isPending;

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} requirePertaminaRep />}

    <div className="grid gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
      <Card className="self-start">
        <CardHeader><CardTitle className="flex items-center gap-2 text-base"><CalendarDays className="size-4 text-primary" />Tanggal distribusi</CardTitle></CardHeader>
        <CardContent className="grid gap-2">
          {dateItems.map((item) => <button
            aria-pressed={item.local_date === selectedDate}
            className="flex min-h-16 w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/5"
            key={item.local_date}
            onClick={() => { setChosenDate(item.local_date); setShowRecipients(false); }}
            type="button"
          >
            <span><strong className="block text-sm">{formatLocalDate(item.local_date)}</strong><span className="text-xs text-muted-foreground">{item.recipient_count} penerima</span></span>
            {item.document ? <Badge className="shrink-0 bg-emerald-600 text-white">Tersinkron</Badge> : <Badge className="shrink-0" variant="secondary">{item.validation_status === 'ready' ? 'Siap' : 'Perlu konfigurasi'}</Badge>}
          </button>)}
        </CardContent>
      </Card>

      <div className="grid min-w-0 gap-4">
        <Card>
          <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle>{selected ? formatLocalDate(selected.local_date) : 'Rekap Harian'}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · {selected?.recipient_count ?? 0} penerima.</p></div><Badge variant="outline">Petani</Badge></div></CardHeader>
          <CardContent className="grid gap-4 pt-5">
            <div className="flex flex-wrap gap-2">
              <Button disabled={busy} onClick={() => preview.mutate()} variant="outline"><Eye />{preview.isPending ? 'Menyiapkan...' : 'Preview PDF'}</Button>
              {canManage && <Button disabled={busy} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
              <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan'}</Button>
            </div>

            {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
              <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} penerima</p></div>
              <Button nativeButton={false} render={<a href={`/api/v1/bast/daily-recap/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
            </div>}

            {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
              <strong className="text-sm">Riwayat versi</strong>
              {versions.filter((doc) => doc.status === 'superseded').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/daily-recap/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
            </div>}

            <Button className="justify-start" variant="ghost" onClick={() => setShowRecipients((value) => !value)}><Users />{showRecipients ? 'Sembunyikan rincian' : `Lihat ${selected?.recipient_count ?? 0} penerima`}</Button>
          </CardContent>
        </Card>

        {showRecipients && (recipients.isPending ? <DataState kind="loading" title="Memuat penerima" description="Mengurutkan berdasarkan nomor bagi." /> : recipients.isError ? <DataState kind="error" title="Daftar penerima gagal dimuat" description="Coba muat ulang." action={{ label: 'Coba lagi', onClick: () => recipients.refetch() }} /> : <>
          <DataTable label={`Penerima ${selectedDate}`} minimumWidth={760}>
            <thead><tr><th>No</th><th>Nama Petani</th><th>No. Kartu Petani</th><th>Merek Mesin</th><th>Tipe Mesin</th><th>No. Seri Mesin</th></tr></thead>
            <tbody>{recips.map((recipient) => <tr key={recipient.slot_number}><td className="font-semibold tabular-nums">{recipient.slot_number}</td><td><strong>{recipient.full_name}</strong></td><td>{recipient.farmer_card_number || '-'}</td><td>{recipient.machine_brand}</td><td>{recipient.machine_type}</td><td className="font-mono text-xs">{recipient.machine_serial || '-'}</td></tr>)}</tbody>
          </DataTable>
          <Card><CardHeader><CardTitle className="text-base">Grand Total per varian</CardTitle></CardHeader><CardContent>
            <DataTable label="Grand total" minimumWidth={480}>
              <thead><tr><th>Varian</th><th>Deskripsi</th><th>Jumlah</th></tr></thead>
              <tbody>{variants.map((variant, index) => <tr key={variant.key}><td>Varian {index + 1}</td><td>{variant.brand} / {variant.type}</td><td>{variant.count} Set</td></tr>)}</tbody>
            </DataTable>
          </CardContent></Card>
        </>)}
      </div>
    </div>
  </div>;
}

function groupVariants(recipients: DailyRecapRecipient[]) {
  const order: string[] = [];
  const map = new Map<string, { brand: string; type: string; count: number }>();
  for (const recipient of recipients) {
    const key = `${recipient.machine_brand}|${recipient.machine_type}|${recipient.machine_power}|${recipient.machine_fuel_type}`;
    if (!map.has(key)) { order.push(key); map.set(key, { brand: recipient.machine_brand, type: recipient.machine_type, count: 0 }); }
    map.get(key)!.count += 1;
  }
  return order.map((key) => ({ key, ...map.get(key)! }));
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
