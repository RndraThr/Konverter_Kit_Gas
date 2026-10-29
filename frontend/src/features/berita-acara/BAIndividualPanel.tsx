import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, CloudUpload, Download, FileCheck2, FolderOpen, Signature, TriangleAlert, Users } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ApiError, apiBlobRequest, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { SignatureSettingsPanel } from './SignatureSettingsPanel';
import type { BundleRequest, DailyBundle, DateSummary, RecipientDocument } from './types';

// scheduleID: jadwal kabupaten ini, dipakai panel tanda tangan Pelaksana/Konsultan.
type Props = { programID: string; regencyID: string; regencyName: string; programType: ProgramType; scheduleID?: string };
type DataResponse<T> = { data: T };

// A date needs a sync when it is ready and its Drive PDF is missing or older than the data.
const needsSync = (item: DateSummary) => item.validation_status === 'ready' && item.sync_status !== 'synced';

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

export function BAIndividualPanel({ programID, regencyID, regencyName, programType, scheduleID }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [chosenDate, setChosenDate] = useState('');
  const [showRecipients, setShowRecipients] = useState(false);
  const [showSignatures, setShowSignatures] = useState(false);
  const [syncError, setSyncError] = useState('');

  const datesKey = ['bast', 'individual', 'dates', programID, regencyID];
  const dates = useQuery({
    queryKey: datesKey,
    queryFn: () => apiRequest<DataResponse<DateSummary[]>>(`/api/v1/bast/individual/dates?program_id=${encodeURIComponent(programID)}&regency_id=${encodeURIComponent(regencyID)}`),
    enabled: programType === 'farmer',
  });
  const dateItems = dates.data?.data ?? [];
  const selectedDate = dateItems.some((item) => item.local_date === chosenDate) ? chosenDate : dateItems[0]?.local_date ?? '';
  const selected = dateItems.find((item) => item.local_date === selectedDate);
  const ready = selected?.validation_status === 'ready';
  const emptied = ready && selected?.recipient_count === 0;
  const recipients = useQuery({
    queryKey: ['bast', 'individual', 'recipients', programID, regencyID, selectedDate],
    queryFn: () => apiRequest<DataResponse<RecipientDocument[]>>(`/api/v1/bast/individual/recipients?program_id=${encodeURIComponent(programID)}&regency_id=${encodeURIComponent(regencyID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '' && showRecipients && ready && !emptied,
  });
  const request = (localDate: string): BundleRequest => ({ program_id: programID, regency_id: regencyID, local_date: localDate });

  // Preview selalu memakai data terkini (nomor BA memakai kuota kabupaten, data
  // yang belum lengkap dicetak kosong) dan dimuat ulang setiap daftar tanggal
  // diperbarui, jadi menunjukkan isi yang akan ditulis saat disinkronkan.
  const canPreview = selected !== undefined && selected.validation_status !== 'configuration_required' && !emptied;
  const preview = useQuery({
    queryKey: ['bast', 'individual', 'preview', programID, regencyID, selectedDate, dates.dataUpdatedAt],
    queryFn: () => apiBlobRequest('/api/v1/bast/individual/bundles/preview', { method: 'POST', body: JSON.stringify(request(selectedDate)) }),
    enabled: programType === 'farmer' && selectedDate !== '' && canPreview,
  });

  // Sinkron ditulis per tanggal; "Sinkronkan semua" menjalankannya berurutan
  // agar Drive tidak menerima banyak unggahan sekaligus.
  const sync = useMutation({
    mutationFn: async (localDates: string[]) => {
      const failed: string[] = [];
      for (const localDate of localDates) {
        try {
          await apiRequest<DataResponse<DailyBundle>>('/api/v1/bast/individual/bundles/finalize', { method: 'POST', body: JSON.stringify(request(localDate)) });
        } catch (error) {
          failed.push(`${formatLocalDate(localDate)}: ${error instanceof ApiError || error instanceof Error ? error.message : 'gagal'}`);
        }
      }
      return { total: localDates.length, failed };
    },
    onSuccess: ({ total, failed }) => {
      void queryClient.invalidateQueries({ queryKey: datesKey });
      if (failed.length) {
        setSyncError(`Sebagian tanggal belum tersinkron. ${failed.join('; ')}`);
        return;
      }
      setSyncError('');
      toast.success(total > 1 ? `${total} tanggal berhasil disinkronkan ke Drive.` : 'PDF berhasil disinkronkan ke Drive.');
    },
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="BA Perorangan Nelayan belum tersedia" description="Data dan template Nelayan tetap dipisahkan dari Petani. Konfigurasi khusus Nelayan akan ditambahkan pada tahap berikutnya." />;
  if (dates.isPending) return <DataState kind="loading" title="Memuat BA Perorangan" description="Menyiapkan kumpulan pembagian per tanggal." />;
  if (dates.isError) return <DataState kind="error" title="BA Perorangan belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => dates.refetch() }} />;
  if (dateItems.length === 0) return <DataState kind="empty" title="Belum ada pembagian selesai" description={`Belum ditemukan distribusi selesai untuk ${regencyName}. Tanggal akan muncul otomatis setelah penyerahan diselesaikan.`} />;

  const sortedRecipients = [...(recipients.data?.data ?? [])].sort((a, b) => a.slot_number - b.slot_number);
  const pending = dateItems.filter(needsSync);
  const bundle = selected?.bundle?.status === 'active' ? selected.bundle : undefined;
  return <div className="grid gap-5">
    {scheduleID && showSignatures && <SignatureSettingsPanel scheduleID={scheduleID} date={selectedDate} document="ba-perorangan" documentLabel="BA Perorangan" />}
  <div className="grid gap-5 xl:grid-cols-[19rem_minmax(0,1fr)]">
    <Card className="self-start">
      <CardHeader className="gap-3">
        <CardTitle className="flex items-center gap-2 text-base"><CalendarDays className="size-4 text-primary" />Tanggal pembagian</CardTitle>
        {pending.length > 0 && <div className="grid gap-2 rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">
          <p><strong>{pending.length} tanggal belum sinkron</strong> dengan Drive.</p>
          {canManage && <Button size="sm" disabled={sync.isPending} onClick={() => sync.mutate(pending.map((item) => item.local_date))}><CloudUpload />{sync.isPending ? 'Menyinkronkan...' : 'Sinkronkan semua'}</Button>}
        </div>}
      </CardHeader>
      <CardContent className="grid gap-2">
        {dateItems.map((item) => <button
          aria-pressed={item.local_date === selectedDate}
          className="flex min-h-16 w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/5"
          key={item.local_date}
          onClick={() => { setChosenDate(item.local_date); setShowRecipients(false); }}
          type="button"
        >
          <span><strong className="block text-sm">{formatLocalDate(item.local_date)}</strong><span className="text-xs text-muted-foreground">{item.recipient_count} penerima</span></span>
          <StatusBadge item={item} />
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
          <div className="flex items-start gap-3 rounded-lg border bg-muted/25 p-3 text-sm"><FolderOpen className="mt-0.5 size-4 shrink-0 text-primary" /><p>Tidak perlu finalisasi. Preview selalu memakai data terbaru; klik <strong>Sinkronkan ke Drive</strong> untuk memperbarui PDF di folder 2. BA PERORANGAN.</p></div>
          {selected?.validation_status === 'configuration_required' && <DataState kind="error" title="Konfigurasi belum lengkap" description="Pastikan zona dan profil dokumen program sudah dipublikasikan." />}
          {ready && selected?.sync_status === 'out_of_sync' && <div role="status" className="flex items-start gap-3 rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100"><TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" /><p>{emptied
            ? 'Semua penerima tanggal ini sudah pindah tanggal atau direvisi. Sinkronkan untuk menghapus PDF lama dari Drive.'
            : 'Ada perubahan data sejak sinkron terakhir (data penerima, nomor, penandatangan, tanda tangan, stempel, atau logo). PDF di Drive belum memuat perubahan ini.'}</p></div>}
          {ready && selected?.sync_status === 'not_created' && <div role="status" className="flex items-start gap-3 rounded-lg border bg-muted/25 p-3 text-sm"><CloudUpload className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><p>PDF tanggal ini belum pernah disinkronkan ke Drive.</p></div>}
          {ready && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            {bundle
              ? <div><div className="flex flex-wrap items-center gap-2"><FileCheck2 className={selected?.sync_status === 'synced' ? 'size-4 text-emerald-600' : 'size-4 text-amber-600'} /><strong className="text-sm">{bundle.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {bundle.version} · {bundle.page_count} halaman · {bundle.recipient_count} penerima{bundle.synced_at ? ` · disinkronkan ${new Date(bundle.synced_at).toLocaleString('id-ID')}` : ''}</p></div>
              : <p className="text-sm text-muted-foreground">Belum ada PDF di Drive.</p>}
            <div className="flex flex-wrap gap-2">
              {bundle && <Button nativeButton={false} render={<a href={`/api/v1/bast/individual/bundles/${bundle.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>}
              {canManage && <Button disabled={sync.isPending || selected?.sync_status === 'synced'} onClick={() => sync.mutate([selectedDate])}><CloudUpload />{sync.isPending ? 'Menyinkronkan...' : selected?.sync_status === 'synced' ? 'Sudah sinkron' : 'Sinkronkan ke Drive'}</Button>}
            </div>
          </div>}
          {syncError && <p className="text-sm text-destructive" role="alert">{syncError}</p>}
          {ready && scheduleID && !emptied && <div className="flex flex-wrap gap-2">
            <Button variant="ghost" onClick={() => setShowSignatures((value) => !value)}><Signature />{showSignatures ? 'Tutup penandatangan' : 'Penandatangan'}</Button>
          </div>}
          {canPreview && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="BA Perorangan" />}
          {ready && !emptied && <Button className="justify-start" onClick={() => setShowRecipients((value) => !value)} variant="ghost"><Users />{showRecipients ? 'Sembunyikan penerima' : `Lihat ${selected?.recipient_count ?? 0} penerima`}</Button>}
        </CardContent>
      </Card>

      {showRecipients && (recipients.isPending ? <DataState kind="loading" title="Memuat penerima" description="Mengurutkan berdasarkan nomor pembagian." /> : recipients.isError ? <DataState kind="error" title="Daftar penerima gagal dimuat" description="Coba muat ulang daftar penerima." action={{ label: 'Coba lagi', onClick: () => recipients.refetch() }} /> : <DataTable label={`Penerima ${selectedDate}`} minimumWidth={680}>
        <thead><tr><th>No. pembagian</th><th>Nomor dokumen</th><th>Total kabupaten</th></tr></thead>
        <tbody>{sortedRecipients.map((recipient) => <tr key={recipient.distribution_slot_id}><td className="font-semibold tabular-nums">{recipient.slot_number}</td><td className="font-mono text-xs">{recipient.document_number}</td><td>{recipient.final_total}</td></tr>)}</tbody>
      </DataTable>)}
    </div>
  </div>
  </div>;
}

function StatusBadge({ item }: { item: DateSummary }) {
  if (item.validation_status === 'configuration_required') return <Badge className="shrink-0" variant="destructive">Perlu konfigurasi</Badge>;
  if (item.sync_status === 'synced') return <Badge className="shrink-0 bg-emerald-600 text-white">Tersinkron</Badge>;
  if (item.sync_status === 'out_of_sync') return <Badge className="shrink-0 bg-amber-500 text-white">Belum sinkron</Badge>;
  return <Badge className="shrink-0" variant="secondary">Belum dibuat</Badge>;
}
