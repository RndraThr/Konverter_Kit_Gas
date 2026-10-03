import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, FileCheck2, RefreshCw, Settings2, Users } from 'lucide-react';
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
import type { AggregateDocument, DP3Recipient, DP3Summary } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

const validationMessage: Record<string, string> = {
  zone_not_configured: 'Kabupaten belum dikonfigurasi ke zona.',
  ba_logo_required: 'Minimal satu logo BA aktif harus dikonfigurasi.',
  handover_location_required: 'Lokasi/titik serah belum diisi.',
  signatory_required: 'Penandatangan dokumen belum lengkap.',
  no_recipients: 'Belum ada penerima pada jadwal ini.',
  recipient_identity_incomplete: 'Ada penerima dengan identitas belum lengkap.',
  allocation_snapshot_incomplete: 'Ada penerima tanpa snapshot varian mesin.',
  verification_snapshot_incomplete: 'Snapshot verifikasi distribusi belum lengkap.',
  machine_power_required: 'Daya mesin belum lengkap pada sebagian penerima.',
  machine_fuel_required: 'Jenis BBM belum lengkap pada sebagian penerima.',
};

export function DP3Panel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [showSettings, setShowSettings] = useState(false);
  const [showRecipients, setShowRecipients] = useState(false);

  const summary = useQuery({
    queryKey: ['bast', 'dp3', 'summary', scheduleID],
    queryFn: () => apiRequest<DataResponse<DP3Summary>>(`/api/v1/bast/dp3/summary?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const recipients = useQuery({
    queryKey: ['bast', 'dp3', 'recipients', scheduleID],
    queryFn: () => apiRequest<DataResponse<DP3Recipient[]>>(`/api/v1/bast/dp3/recipients?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer' && showRecipients,
  });
  const documents = useQuery({
    queryKey: ['bast', 'dp3', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/dp3/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  // Only gate the preview fetch on an explicit 'ready' status — defaulting
  // to 'ready' while summary is still loading (data undefined) would fire
  // the preview prematurely, before the real validation status is known.
  const status = summary.data?.data.validation_status ?? 'ready';
  const preview = useQuery({
    queryKey: ['bast', 'dp3', 'preview', scheduleID, documentDate],
    queryFn: () => apiBlobRequest('/api/v1/bast/dp3/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && summary.data?.data.validation_status === 'ready' && documentDate !== '',
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/dp3/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'dp3'] });
      preview.refetch();
      toast.success('DP3 berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="DP3 Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (summary.isPending) return <DataState kind="loading" title="Memuat DP3" description="Menyiapkan daftar nominatif penerima." />;
  if (summary.isError) return <DataState kind="error" title="DP3 belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => summary.refetch() }} />;

  const data = summary.data?.data;
  const activeDoc = documents.data?.data.find((doc) => doc.status === 'active');
  const versions = documents.data?.data ?? [];
  // preview.isPending is excluded: react-query v5 reports isPending=true for
  // a disabled query (never fetched), which would permanently disable these
  // action buttons whenever the preview isn't ready yet.
  const busy = finalize.isPending;

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} />}

    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-1">
        <CardHeader><CardTitle className="text-base">Ringkasan</CardTitle></CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <SummaryItem label="Total nominatif" value={data?.total_recipients ?? 0} />
          <SummaryItem label="Sudah bernomor bagi" value={data?.numbered_recipients ?? 0} />
          <SummaryItem label="Belum ter-mount" value={data?.unmounted_recipients ?? 0} />
          {status !== 'ready' && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{validationMessage[status] ?? status}</div>}
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-base">Dokumen DP3</CardTitle><Badge variant="outline">Petani</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-3 sm:grid-cols-[16rem_1fr] sm:items-end">
            <FormField label="Tanggal dokumen" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              {canManage && <Button disabled={busy || status !== 'ready'} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
              <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan'}</Button>
            </div>
          </div>

          {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} penerima</p></div>
            <Button nativeButton={false} render={<a href={`/api/v1/bast/dp3/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
          </div>}

          {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
            <strong className="text-sm">Riwayat versi</strong>
            {versions.filter((doc) => doc.status === 'superseded').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version} · {doc.document_date}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/dp3/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
          </div>}

          {status === 'ready' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="DP3" />}

          <Button className="justify-start" variant="ghost" onClick={() => setShowRecipients((value) => !value)}><Users />{showRecipients ? 'Sembunyikan penerima' : `Lihat ${data?.total_recipients ?? 0} penerima`}</Button>
        </CardContent>
      </Card>
    </div>

    {showRecipients && (recipients.isPending ? <DataState kind="loading" title="Memuat penerima" description="Mengurutkan daftar nominatif." /> : recipients.isError ? <DataState kind="error" title="Daftar penerima gagal dimuat" description="Coba muat ulang." action={{ label: 'Coba lagi', onClick: () => recipients.refetch() }} /> : <DataTable label={`Penerima DP3 ${regencyName}`} minimumWidth={840}>
      <thead><tr><th>Nama Petani</th><th>NIK</th><th>Status</th><th>Merek / Tipe</th><th>Daya</th><th>BBM</th><th>Sumber</th></tr></thead>
      <tbody>{(recipients.data?.data ?? []).map((recipient) => <tr key={`${recipient.nik}-${recipient.full_name}`}>
        <td><strong>{recipient.full_name}</strong><small className="subline">{recipient.address || recipient.village || recipient.district || '-'}</small></td>
        <td className="font-mono text-xs">{recipient.nik || '-'}</td>
        <td>{recipient.distribution_number ? <Badge variant="secondary">Bernomor</Badge> : <Badge variant="outline">Belum mount</Badge>}</td>
        <td>{recipient.machine_brand} {recipient.machine_type}</td>
        <td>{recipient.machine_power || '-'}</td>
        <td>{recipient.machine_fuel_type || '-'}</td>
        <td className="text-xs text-muted-foreground">{recipient.source === 'verification_snapshot' ? 'Verifikasi' : 'Alokasi'}</td>
      </tr>)}</tbody>
    </DataTable>)}
  </div>;
}

function SummaryItem({ label, value }: { label: string; value: number }) {
  return <div className="flex items-center justify-between rounded-lg border px-3 py-2"><span className="text-muted-foreground">{label}</span><strong className="tabular-nums">{value}</strong></div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
