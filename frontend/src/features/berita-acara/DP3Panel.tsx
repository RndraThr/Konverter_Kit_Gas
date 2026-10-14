import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Download, FileCheck2, RefreshCw, Signature, Users } from 'lucide-react';
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
import { SignatureSettingsPanel } from './SignatureSettingsPanel';
import { useWetSignatureBlocked, WetSignatureBar } from './WetSignatureBar';
import { incompletePreviewNote, previewBlockingStatus } from './previewStatus';
import { DocumentInfoFields } from './DocumentInfoFields';
import type { AggregateDocument, DailyRecapDate, DP3Recipient } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType };
type DataResponse<T> = { data: T };

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

const validationMessage: Record<string, string> = {
  zone_not_configured: 'Kabupaten belum dikonfigurasi ke zona.',
  ba_logo_required: 'Minimal satu logo Berita Acara aktif wajib dikonfigurasi.',
  handover_location_required: 'Lokasi atau titik serah belum dikonfigurasi.',
  signatory_required: 'Data penandatangan dokumen belum lengkap.',
  no_recipients: 'Belum ada penerima pada tanggal ini.',
  recipient_identity_incomplete: 'Identitas sebagian penerima belum lengkap.',
  allocation_snapshot_incomplete: 'Ada penerima tanpa snapshot varian mesin.',
  verification_snapshot_incomplete: 'Snapshot verifikasi distribusi sebagian penerima belum lengkap.',
  machine_power_required: 'Daya mesin belum lengkap pada sebagian penerima.',
  machine_fuel_required: 'Jenis BBM mesin belum lengkap pada sebagian penerima.',
};

// DP3 dibuat per hari distribusi seperti Rekap Harian: penerima yang
// penyerahannya selesai pada tanggal itu, nomor urut berlanjut dari hari
// sebelumnya, dan kolom Paraf memakai tanda tangan penerima dari distribusi.
export function DP3Panel({ scheduleID, regencyName, programType }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [chosenDate, setChosenDate] = useState('');
  const [showSignatures, setShowSignatures] = useState(false);
  const [showRecipients, setShowRecipients] = useState(false);

  const dates = useQuery({
    queryKey: ['bast', 'dp3', 'dates', scheduleID],
    queryFn: () => apiRequest<DataResponse<DailyRecapDate[]>>(`/api/v1/bast/dp3/dates?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const dateItems = dates.data?.data ?? [];
  const selectedDate = dateItems.some((item) => item.local_date === chosenDate) ? chosenDate : dateItems[0]?.local_date ?? '';
  const recipients = useQuery({
    queryKey: ['bast', 'dp3', 'recipients', scheduleID, selectedDate],
    queryFn: () => apiRequest<DataResponse<DP3Recipient[]>>(`/api/v1/bast/dp3/recipients?schedule_id=${encodeURIComponent(scheduleID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '' && showRecipients,
  });
  const documents = useQuery({
    queryKey: ['bast', 'dp3', 'documents', scheduleID, selectedDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/dp3/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${selectedDate}`),
    enabled: programType === 'farmer' && selectedDate !== '',
  });

  const preview = useQuery({
    queryKey: ['bast', 'dp3', 'preview', scheduleID, selectedDate],
    queryFn: () => apiBlobRequest('/api/v1/bast/dp3/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: selectedDate }) }),
    enabled: programType === 'farmer' && selectedDate !== '' && !previewBlockingStatus.has(dateItems.find((item) => item.local_date === selectedDate)?.validation_status ?? 'no_recipients'),
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/dp3/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: selectedDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'dp3'] });
      preview.refetch();
      toast.success('DP3 berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const wetBlocked = useWetSignatureBlocked(scheduleID, 'dp3', selectedDate);

  if (programType === 'fisherman') return <DataState kind="empty" title="DP3 Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (dates.isPending) return <DataState kind="loading" title="Memuat DP3" description="Menyiapkan tanggal distribusi selesai." />;
  if (dates.isError) return <DataState kind="error" title="DP3 belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => dates.refetch() }} />;
  if (dateItems.length === 0) return <DataState kind="empty" title="Belum ada distribusi selesai" description={`Belum ditemukan distribusi selesai untuk ${regencyName}. Tanggal akan muncul otomatis setelah penyerahan diselesaikan.`} />;

  const selected = dateItems.find((item) => item.local_date === selectedDate);
  const selectedReady = selected?.validation_status === 'ready';
  const activeDoc = documents.data?.data.find((doc) => doc.status === 'active');
  const versions = documents.data?.data ?? [];
  const historicalDocs = versions.filter((doc) => doc.status !== 'active');
  const recips = recipients.data?.data ?? [];
  // preview.isPending is excluded: react-query v5 reports isPending=true for
  // a disabled query (never fetched), which would permanently disable these
  // action buttons whenever the preview isn't ready yet.
  const busy = finalize.isPending;

  return <div className="grid gap-5">
    {showSignatures && <SignatureSettingsPanel scheduleID={scheduleID} date={selectedDate} document="dp3" documentLabel="DP3" />}

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
          <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle>{selected ? formatLocalDate(selected.local_date) : 'DP3'}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · {selected?.recipient_count ?? 0} penerima.</p></div><Badge variant="outline">Petani</Badge></div></CardHeader>
          <CardContent className="grid gap-4 pt-5">
            <div className="grid gap-3 sm:items-start sm:grid-cols-2">
              <DocumentInfoFields scheduleID={scheduleID} fields={[{ key: 'handover_location', label: 'Lokasi / Titik Serah' }, { key: 'consultant_company_name', label: 'Konsultan Distribusi' }]} />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
              <Button variant="ghost" onClick={() => setShowSignatures((value) => !value)}><Signature />{showSignatures ? 'Tutup penandatangan' : 'Penandatangan'}</Button>
              {canManage && <Button disabled={busy || !selectedReady || wetBlocked} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
            </div>
            <WetSignatureBar scheduleID={scheduleID} document="dp3" date={selectedDate} label="DP3" />

            {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
              <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} penerima</p></div>
              <Button nativeButton={false} render={<a href={`/api/v1/bast/dp3/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
            </div>}

            {historicalDocs.length > 0 && <div className="grid gap-2 rounded-lg border p-3">
              <strong className="text-sm">Riwayat versi</strong>
              {historicalDocs.map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="flex flex-wrap items-center gap-2 text-muted-foreground">Versi {doc.version}{doc.status === 'stale' && <Badge variant="destructive">Perlu dibuat ulang</Badge>}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/dp3/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
            </div>}

            {!selectedReady && selected && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{validationMessage[selected.validation_status] ?? selected.validation_status}</div>}
            {selected && !selectedReady && !previewBlockingStatus.has(selected.validation_status) && <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">{incompletePreviewNote}</p>}
            {selected && !previewBlockingStatus.has(selected.validation_status) && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} errorDescription={preview.error ? errorMessage(preview.error) : undefined} label="DP3" />}

            <Button className="justify-start" variant="ghost" onClick={() => setShowRecipients((value) => !value)}><Users />{showRecipients ? 'Sembunyikan rincian' : `Lihat ${selected?.recipient_count ?? 0} penerima`}</Button>
          </CardContent>
        </Card>

        {showRecipients && (recipients.isPending ? <DataState kind="loading" title="Memuat penerima" description="Mengurutkan berdasarkan nomor bagi." /> : recipients.isError ? <DataState kind="error" title="Daftar penerima gagal dimuat" description="Coba muat ulang." action={{ label: 'Coba lagi', onClick: () => recipients.refetch() }} /> : <>
          <DataTable label={`Penerima DP3 ${selectedDate}`} minimumWidth={840}>
            <thead><tr><th>No</th><th>No. Bagi</th><th>Nama Petani</th><th>NIK</th><th>Merek / Tipe</th><th>Daya</th><th>BBM</th></tr></thead>
            <tbody>{recips.map((recipient) => <tr key={recipient.slot_id || `${recipient.nik}-${recipient.full_name}`}>
              <td className="font-semibold tabular-nums">{recipient.row_number}</td>
              <td className="tabular-nums text-muted-foreground">{recipient.distribution_number ?? '-'}</td>
              <td><strong>{recipient.full_name}</strong><small className="subline">{recipient.address || recipient.village || recipient.district || '-'}</small></td>
              <td className="font-mono text-xs">{recipient.nik || '-'}</td>
              <td>{recipient.machine_brand} {recipient.machine_type}</td>
              <td>{recipient.machine_power || '-'}</td>
              <td>{recipient.machine_fuel_type || '-'}</td>
            </tr>)}</tbody>
          </DataTable>
        </>)}
      </div>
    </div>
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
