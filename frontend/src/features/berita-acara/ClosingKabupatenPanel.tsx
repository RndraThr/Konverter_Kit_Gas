import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, FileCheck2, RefreshCw, Settings2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { FormField } from '@/components/FormField';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import type { AggregateDocument, ClosingKabupatenRow } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

const validationMessage: Record<string, string> = {
  zone_not_configured: 'Kabupaten belum dikonfigurasi ke zona.',
  ba_logo_required: 'Minimal satu logo BA aktif harus dikonfigurasi.',
  handover_location_required: 'Lokasi/titik serah belum diisi.',
  signatory_required: 'Penandatangan dokumen belum lengkap.',
  no_recipients: 'Belum ada distribusi selesai pada jadwal ini.',
  verification_snapshot_incomplete: 'Snapshot verifikasi distribusi belum lengkap.',
};

export function ClosingKabupatenPanel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [showSettings, setShowSettings] = useState(false);
  const [showRows, setShowRows] = useState(false);

  const rows = useQuery({
    queryKey: ['bast', 'closing-kabupaten', 'rows', scheduleID],
    queryFn: () => apiRequest<DataResponse<ClosingKabupatenRow[]>>(`/api/v1/bast/closing-kabupaten/rows?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const documents = useQuery({
    queryKey: ['bast', 'closing-kabupaten', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/closing-kabupaten/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  const rowItems = rows.data?.data ?? [];
  const grandTotal = rowItems.reduce((sum, row) => sum + row.count, 0);
  const status = rows.isError ? 'no_recipients' : rowItems.length === 0 ? 'no_recipients' : 'ready';

  const preview = useQuery({
    queryKey: ['bast', 'closing-kabupaten', 'preview', scheduleID, documentDate],
    queryFn: () => apiBlobRequest('/api/v1/bast/closing-kabupaten/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && status === 'ready' && documentDate !== '',
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/closing-kabupaten/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'closing-kabupaten'] });
      preview.refetch();
      toast.success('Closing Kabupaten berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="Closing Kabupaten Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (rows.isPending) return <DataState kind="loading" title="Memuat Closing Kabupaten" description="Mengumpulkan rekapitulasi lintas titik serah." />;

  const activeDoc = documents.data?.data.find((doc) => doc.status === 'active');
  const versions = documents.data?.data ?? [];
  const busy = finalize.isPending;

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} requirePertaminaRep />}

    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-1">
        <CardHeader><CardTitle className="text-base">Ringkasan</CardTitle></CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="flex items-center justify-between rounded-lg border px-3 py-2"><span className="text-muted-foreground">Baris titik serah</span><strong className="tabular-nums">{rowItems.length}</strong></div>
          <div className="flex items-center justify-between rounded-lg border px-3 py-2"><span className="text-muted-foreground">Total paket (set)</span><strong className="tabular-nums">{grandTotal}</strong></div>
          {status !== 'ready' && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{validationMessage[status] ?? status}</div>}
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-base">Dokumen Closing Kabupaten</CardTitle><Badge variant="outline">Petani</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-3 sm:grid-cols-[16rem_1fr] sm:items-end">
            <FormField label="Tanggal closing" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              {canManage && <Button disabled={busy || status !== 'ready'} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
              <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan'}</Button>
            </div>
          </div>

          {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} paket</p></div>
            <Button nativeButton={false} render={<a href={`/api/v1/bast/closing-kabupaten/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
          </div>}

          {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
            <strong className="text-sm">Riwayat versi</strong>
            {versions.filter((doc) => doc.status === 'superseded').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version} · {doc.document_date}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/closing-kabupaten/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
          </div>}

          {status === 'ready' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="Closing Kabupaten" />}

          <Button className="justify-start" variant="ghost" onClick={() => setShowRows((value) => !value)}>{showRows ? 'Sembunyikan rincian' : `Lihat ${rowItems.length} baris titik serah`}</Button>
        </CardContent>
      </Card>
    </div>

    {showRows && <DataTable label={`Closing Kabupaten ${regencyName}`} minimumWidth={680}>
      <thead><tr><th>Lokasi / Titik Serah</th><th>Merek Mesin</th><th>Tipe Mesin</th><th>Jumlah Paket</th></tr></thead>
      <tbody>{rowItems.map((row, index) => <tr key={`${row.location}-${row.machine_brand}-${row.machine_type}-${index}`}>
        <td>{row.location}</td>
        <td>{row.machine_brand}</td>
        <td>{row.machine_type}</td>
        <td className="tabular-nums">{row.count}</td>
      </tr>)}</tbody>
    </DataTable>}
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
