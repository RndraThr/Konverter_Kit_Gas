import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, FileCheck2, MapPin, RefreshCw, Settings2, Signature } from 'lucide-react';
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
import { SignatureSettingsPanel } from './SignatureSettingsPanel';
import { useWetSignatureBlocked, WetSignatureBar } from './WetSignatureBar';
import { handoverQueryKey, HandoverLocationsEditor } from './HandoverLocationsEditor';
import type { AggregateDocument, ClosingRow, HandoverStatus, HandoverView } from './types';

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

const validationMessage: Record<string, string> = {
  zone_not_configured: 'Kabupaten belum dikonfigurasi ke zona.',
  ba_logo_required: 'Minimal satu logo BA aktif harus dikonfigurasi.',
  handover_location_required: 'Titik serah belum diatur. Buka "Atur titik serah".',
  no_location_recipients: 'Belum ada distribusi selesai pada titik serah ini.',
  signatory_required: 'Penandatangan dokumen belum lengkap.',
  no_recipients: 'Belum ada distribusi selesai pada jadwal ini.',
  verification_snapshot_incomplete: 'Snapshot verifikasi distribusi belum lengkap.',
};

const statusBadge: Record<HandoverStatus, { label: string; className: string }> = {
  final: { label: 'Final', className: 'bg-emerald-600 text-white' },
  stale: { label: 'Perlu dibuat ulang', className: 'bg-destructive text-white' },
  none: { label: 'Belum dibuat', className: 'bg-amber-500 text-white' },
  empty: { label: 'Belum ada penerima', className: 'bg-muted text-muted-foreground' },
};

const formatLocalDate = (value: string) => new Intl.DateTimeFormat('id-ID', { day: '2-digit', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));

export function ClosingTitikSerahPanel({ scheduleID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [showSignatures, setShowSignatures] = useState(false);
  const [showRows, setShowRows] = useState(false);
  const [editing, setEditing] = useState(false);
  const [pickedLocation, setPickedLocation] = useState('');

  const handover = useQuery({
    queryKey: handoverQueryKey(scheduleID),
    queryFn: () => apiRequest<DataResponse<HandoverView>>(`/api/v1/bast/handover-locations?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  const locations = handover.data?.data.locations ?? [];
  const location = locations.find((item) => item.id === pickedLocation) ?? locations[0];
  const locationID = location?.id ?? '';
  const locationQuery = `schedule_id=${encodeURIComponent(scheduleID)}&location_id=${encodeURIComponent(locationID)}`;

  const rows = useQuery({
    queryKey: ['bast', 'closing-titik-serah', 'rows', scheduleID, locationID],
    queryFn: () => apiRequest<DataResponse<ClosingRow[]>>(`/api/v1/bast/closing-titik-serah/rows?${locationQuery}`),
    enabled: programType === 'farmer' && locationID !== '',
  });
  const documents = useQuery({
    queryKey: ['bast', 'closing-titik-serah', 'documents', scheduleID, locationID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/closing-titik-serah/documents?${locationQuery}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '' && locationID !== '',
  });

  const rowItems = rows.data?.data ?? [];
  const grandTotal = rowItems.reduce((sum, row) => sum + row.count, 0);
  const status = locationID === '' ? 'handover_location_required' : rows.isError || rowItems.length === 0 ? 'no_location_recipients' : 'ready';

  const preview = useQuery({
    queryKey: ['bast', 'closing-titik-serah', 'preview', scheduleID, locationID, documentDate],
    queryFn: () => apiBlobRequest('/api/v1/bast/closing-titik-serah/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, location_id: locationID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && status === 'ready' && documentDate !== '',
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/closing-titik-serah/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, location_id: locationID, document_date: documentDate }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'closing-titik-serah'] });
      queryClient.invalidateQueries({ queryKey: ['bast', 'closing-kabupaten'] });
      preview.refetch();
      toast.success('Closing Titik Serah berhasil difinalisasi dan disinkronkan.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const wetDocument = `closing-titik-serah@${locationID}`;
  const wetBlocked = useWetSignatureBlocked(scheduleID, wetDocument, documentDate);

  if (programType === 'fisherman') return <DataState kind="empty" title="Closing Titik Serah Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (handover.isPending || (locationID !== '' && rows.isPending)) return <DataState kind="loading" title="Memuat Closing Titik Serah" description="Mengumpulkan rekapitulasi closing harian." />;

  const activeDoc = documents.data?.data.find((doc) => doc.status === 'active');
  const versions = documents.data?.data ?? [];
  const historicalDocs = versions.filter((doc) => doc.status !== 'active');
  const busy = finalize.isPending;

  const view = handover.data?.data;
  return <div className="grid gap-5">
    <Card>
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-base">Rekap per titik serah</CardTitle>{canManage && view && <Button variant="outline" size="sm" onClick={() => setEditing((value) => !value)}><Settings2 />{editing ? 'Tutup pengaturan' : 'Atur titik serah'}</Button>}</div></CardHeader>
      <CardContent className="grid gap-3 pt-4">
        {view && view.unassigned > 0 && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{view.unassigned} penerima selesai distribusi belum ada titik serah. Atur cakupan wilayah atau pindahkan penerima secara manual.</div>}
        {locations.length === 0 && <p className="text-sm text-muted-foreground">Belum ada titik serah.{canManage ? ' Tambahkan lewat "Atur titik serah".' : ''}</p>}
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {locations.map((item) => <button type="button" key={item.id} onClick={() => setPickedLocation(item.id)} aria-pressed={item.id === locationID}
            className={`grid gap-1 rounded-lg border p-3 text-left text-sm transition-colors hover:bg-muted/50 ${item.id === locationID ? 'border-primary ring-2 ring-primary/30' : ''}`}>
            <span className="flex items-start justify-between gap-2"><span className="flex items-center gap-2 font-semibold"><MapPin className="size-4 shrink-0 text-primary" aria-hidden="true" />{item.name}</span><Badge className={statusBadge[item.status].className}>{statusBadge[item.status].label}</Badge></span>
            <span className="text-muted-foreground">{item.recipients} penerima selesai{item.assigned > item.recipients ? ` dari ${item.assigned}` : ''}</span>
            <span className="text-xs text-muted-foreground">{item.dates.length === 0 ? 'Belum ada tanggal distribusi' : item.dates.length === 1 ? formatLocalDate(item.dates[0]) : `${item.dates.length} hari · ${formatLocalDate(item.dates[0])} – ${formatLocalDate(item.dates[item.dates.length - 1])}`}</span>
          </button>)}
        </div>
      </CardContent>
    </Card>

    {editing && view && <HandoverLocationsEditor scheduleID={scheduleID} view={view} onClose={() => setEditing(false)} />}

    {showSignatures && <SignatureSettingsPanel scheduleID={scheduleID} date={documentDate} document="closing-titik-serah" documentLabel="Closing Titik Serah" />}

    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-1">
        <CardHeader><CardTitle className="text-base">Ringkasan</CardTitle></CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="flex items-center justify-between rounded-lg border px-3 py-2"><span className="text-muted-foreground">Baris closing harian</span><strong className="tabular-nums">{rowItems.length}</strong></div>
          <div className="flex items-center justify-between rounded-lg border px-3 py-2"><span className="text-muted-foreground">Total paket (set)</span><strong className="tabular-nums">{grandTotal}</strong></div>
          {status !== 'ready' && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">{validationMessage[status] ?? status}</div>}
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-base">BA Serah Terima{location ? ` · ${location.name}` : ''}</CardTitle><Badge variant="outline">Petani</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="grid gap-3 sm:items-end sm:grid-cols-[12rem_minmax(0,1fr)]">
            <FormField label="Tanggal BA" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} />
            <p className="text-xs text-muted-foreground">Satu BA per titik serah, merangkum semua tanggal distribusi penerima lokasi ini.</p>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
            <Button variant="ghost" onClick={() => setShowSignatures((value) => !value)}><Signature />{showSignatures ? 'Tutup penandatangan' : 'Penandatangan'}</Button>
            {canManage && <Button disabled={busy || status !== 'ready' || wetBlocked} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
          </div>
          {locationID !== '' && <WetSignatureBar scheduleID={scheduleID} document={wetDocument} date={documentDate} label="Closing Titik Serah" />}

          {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
            <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} paket</p></div>
            <Button nativeButton={false} render={<a href={`/api/v1/bast/closing-titik-serah/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
          </div>}

          {historicalDocs.length > 0 && <div className="grid gap-2 rounded-lg border p-3">
            <strong className="text-sm">Riwayat versi</strong>
            {historicalDocs.map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="flex flex-wrap items-center gap-2 text-muted-foreground">Versi {doc.version} · {doc.document_date}{doc.status === 'stale' && <Badge variant="destructive">Perlu dibuat ulang</Badge>}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/closing-titik-serah/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
          </div>}

          {status === 'ready' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="Closing Titik Serah" />}

          <Button className="justify-start" variant="ghost" onClick={() => setShowRows((value) => !value)}>{showRows ? 'Sembunyikan rincian' : `Lihat ${rowItems.length} baris closing harian`}</Button>
        </CardContent>
      </Card>
    </div>

    {showRows && <DataTable label={`Closing Titik Serah ${regencyName}`} minimumWidth={680}>
      <thead><tr><th>Tanggal</th><th>Merek Mesin</th><th>Tipe Mesin</th><th>Jumlah Paket</th></tr></thead>
      <tbody>{rowItems.map((row, index) => <tr key={`${row.local_date}-${row.machine_brand}-${row.machine_type}-${index}`}>
        <td>{formatLocalDate(row.local_date)}</td>
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
