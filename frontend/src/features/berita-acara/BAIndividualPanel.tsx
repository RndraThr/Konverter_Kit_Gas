import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, FileCheck2, FolderOpen, LockKeyhole, RefreshCw, Users } from 'lucide-react';
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
import { PdfPreview } from './PdfPreview';
import type { BundleRequest, DailyBundle, DateSummary, LockResult, RecipientDocument } from './types';

type Props = { programID: string; regencyID: string; regencyName: string; programType: ProgramType };
type DataResponse<T> = { data: T };

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

export function BAIndividualPanel({ programID, regencyID, regencyName, programType }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [chosenDate, setChosenDate] = useState('');
  const [showRecipients, setShowRecipients] = useState(false);
  const [locallyLocked, setLocallyLocked] = useState<string[]>([]);
  const [operationError, setOperationError] = useState('');

  const dates = useQuery({
    queryKey: ['bast', 'individual', 'dates', programID, regencyID],
    queryFn: () => apiRequest<DataResponse<DateSummary[]>>(`/api/v1/bast/individual/dates?program_id=${encodeURIComponent(programID)}&regency_id=${encodeURIComponent(regencyID)}`),
    enabled: programType === 'farmer',
  });
  const dateItems = dates.data?.data ?? [];
  const selectedDate = dateItems.some((item) => item.local_date === chosenDate) ? chosenDate : dateItems[0]?.local_date ?? '';
  const selected = dateItems.find((item) => item.local_date === selectedDate);
  const ready = selected?.validation_status === 'ready' || locallyLocked.includes(selectedDate);
  const recipients = useQuery({
    queryKey: ['bast', 'individual', 'recipients', programID, regencyID, selectedDate],
    queryFn: () => apiRequest<DataResponse<RecipientDocument[]>>(`/api/v1/bast/individual/recipients?program_id=${encodeURIComponent(programID)}&regency_id=${encodeURIComponent(regencyID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '' && showRecipients && ready,
  });
  const request: BundleRequest = { program_id: programID, regency_id: regencyID, local_date: selectedDate };

  const lock = useMutation({
    mutationFn: () => apiRequest<DataResponse<LockResult>>('/api/v1/bast/individual/lock-total', { method: 'POST', body: JSON.stringify({ program_id: programID, regency_id: regencyID }) }),
    onSuccess: () => {
      setLocallyLocked((current) => current.includes(selectedDate) ? current : [...current, selectedDate]);
      setOperationError('');
      queryClient.invalidateQueries({ queryKey: ['bast', 'individual', 'dates', programID, regencyID] });
      toast.success('Jumlah total pembagian kabupaten telah dikunci.');
    },
    onError: (error) => setOperationError(errorMessage(error)),
  });
  const preview = useQuery({
    queryKey: ['bast', 'individual', 'preview', programID, regencyID, selectedDate],
    queryFn: () => apiBlobRequest('/api/v1/bast/individual/bundles/preview', { method: 'POST', body: JSON.stringify(request) }),
    enabled: programType === 'farmer' && selectedDate !== '' && ready,
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<DailyBundle>>('/api/v1/bast/individual/bundles/finalize', { method: 'POST', body: JSON.stringify(request) }),
    onSuccess: () => {
      setOperationError('');
      queryClient.invalidateQueries({ queryKey: ['bast', 'individual', 'dates', programID, regencyID] });
      preview.refetch();
      toast.success('PDF harian berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => setOperationError(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="BA Perorangan Nelayan belum tersedia" description="Data dan template Nelayan tetap dipisahkan dari Petani. Konfigurasi khusus Nelayan akan ditambahkan pada tahap berikutnya." />;
  if (dates.isPending) return <DataState kind="loading" title="Memuat BA Perorangan" description="Menyiapkan kumpulan pembagian per tanggal." />;
  if (dates.isError) return <DataState kind="error" title="BA Perorangan belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => dates.refetch() }} />;
  if (dateItems.length === 0) return <DataState kind="empty" title="Belum ada pembagian selesai" description={`Belum ditemukan distribusi selesai untuk ${regencyName}. Tanggal akan muncul otomatis setelah penyerahan diselesaikan.`} />;

  const sortedRecipients = [...(recipients.data?.data ?? [])].sort((a, b) => a.slot_number - b.slot_number);
  // preview.isPending is excluded: react-query v5 reports isPending=true for
  // a disabled query (never fetched), which would permanently disable these
  // action buttons whenever the preview isn't ready yet.
  const busy = lock.isPending || finalize.isPending;
  return <div className="grid gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
    <Card className="self-start">
      <CardHeader><CardTitle className="flex items-center gap-2 text-base"><CalendarDays className="size-4 text-primary" />Tanggal pembagian</CardTitle></CardHeader>
      <CardContent className="grid gap-2">
        {dateItems.map((item) => <button
          aria-pressed={item.local_date === selectedDate}
          className="flex min-h-16 w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/5"
          key={item.local_date}
          onClick={() => { setChosenDate(item.local_date); setShowRecipients(false); setOperationError(''); }}
          type="button"
        >
          <span><strong className="block text-sm">{formatLocalDate(item.local_date)}</strong><span className="text-xs text-muted-foreground">{item.recipient_count} penerima</span></span>
          <StatusBadge status={item.validation_status} bundleStatus={item.bundle?.status} />
        </button>)}
      </CardContent>
    </Card>

    <div className="grid min-w-0 gap-4">
      <Card>
        <CardHeader className="gap-2 border-b">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div><CardTitle>{selected ? formatLocalDate(selected.local_date) : 'Tanggal pembagian'}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · {selected?.recipient_count ?? 0} penerima dalam satu PDF rangkapan.</p></div>
            <Badge variant="outline">Petani</Badge>
          </div>
        </CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="flex items-start gap-3 rounded-lg border bg-muted/25 p-3 text-sm"><FolderOpen className="mt-0.5 size-4 shrink-0 text-primary" /><p>PDF tersimpan langsung di folder 2. BA PERORANGAN tanpa subfolder tanggal.</p></div>
          {selected?.validation_status === 'configuration_required' && <DataState kind="error" title="Konfigurasi belum lengkap" description="Pastikan zona dan profil dokumen program sudah dipublikasikan." />}
          {selected?.validation_status === 'total_not_locked' && !ready && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-4 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100"><strong>Total pembagian belum dikunci.</strong><p className="mt-1">Nomor BA memakai total pembagian kabupaten sebagai penyebut. Kunci total sebelum preview atau finalisasi.</p></div>}
          {selected?.bundle && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            <div><div className="flex flex-wrap items-center gap-2"><FileCheck2 className={selected.bundle.status === 'stale' ? 'size-4 text-amber-600' : 'size-4 text-emerald-600'} /><strong className="text-sm">{selected.bundle.filename}</strong>{selected.bundle.status === 'stale' && <Badge variant="destructive">Perlu dibuat ulang</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">Versi {selected.bundle.version} · {selected.bundle.page_count} halaman · {selected.bundle.recipient_count} penerima</p></div>
            <Button nativeButton={false} render={<a href={`/api/v1/bast/individual/bundles/${selected.bundle.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
          </div>}
          {operationError && <p className="text-sm text-destructive" role="alert">{operationError}</p>}
          {selected?.validation_status !== 'configuration_required' && <div className="flex flex-wrap gap-2">
            {canManage && !ready && <Button disabled={busy} onClick={() => lock.mutate()}><LockKeyhole />{lock.isPending ? 'Mengunci...' : 'Kunci total kabupaten'}</Button>}
            {canManage && <Button disabled={!ready || busy} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
          </div>}
          {ready && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="BA Perorangan" />}
          {ready && <Button className="justify-start" onClick={() => setShowRecipients((value) => !value)} variant="ghost"><Users />{showRecipients ? 'Sembunyikan penerima' : `Lihat ${selected?.recipient_count ?? 0} penerima`}</Button>}
        </CardContent>
      </Card>

      {showRecipients && (recipients.isPending ? <DataState kind="loading" title="Memuat penerima" description="Mengurutkan berdasarkan nomor pembagian." /> : recipients.isError ? <DataState kind="error" title="Daftar penerima gagal dimuat" description="Coba muat ulang daftar penerima." action={{ label: 'Coba lagi', onClick: () => recipients.refetch() }} /> : <DataTable label={`Penerima ${selectedDate}`} minimumWidth={680}>
        <thead><tr><th>No. pembagian</th><th>Nomor dokumen</th><th>Total kabupaten</th></tr></thead>
        <tbody>{sortedRecipients.map((recipient) => <tr key={recipient.distribution_slot_id}><td className="font-semibold tabular-nums">{recipient.slot_number}</td><td className="font-mono text-xs">{recipient.document_number}</td><td>{recipient.final_total}</td></tr>)}</tbody>
      </DataTable>)}
    </div>
  </div>;
}

function StatusBadge({ status, bundleStatus }: { status: DateSummary['validation_status']; bundleStatus?: DailyBundle['status'] }) {
  if (bundleStatus === 'active') return <Badge className="shrink-0 bg-emerald-600 text-white">Tersinkron</Badge>;
  if (bundleStatus === 'stale') return <Badge className="shrink-0" variant="destructive">Perlu dibuat ulang</Badge>;
  if (status === 'ready') return <Badge className="shrink-0" variant="secondary">Siap</Badge>;
  if (status === 'total_not_locked') return <Badge className="shrink-0" variant="outline">Perlu lock</Badge>;
  return <Badge className="shrink-0" variant="destructive">Perlu konfigurasi</Badge>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
