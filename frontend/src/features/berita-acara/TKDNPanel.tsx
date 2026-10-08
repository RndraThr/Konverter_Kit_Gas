import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, ExternalLink, FileCheck2, ListChecks, Plus, RefreshCw, Save, Settings2, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { DataTable } from '@/components/DataTable';
import { FormField } from '@/components/FormField';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import { TemplateEntrySelect } from './TemplateEntrySelect';
import type { AggregateDocument, TKDNProfile, TKDNRow, TKDNSummary } from './types';

type Props = { scheduleID: string; programID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

const percentValid = (value: number) => Number.isFinite(value) && value >= 0 && value <= 100;
const rowsValid = (rows: TKDNRow[]) => rows.length > 0 && rows.every((row) => row.name.trim() !== '' && row.ref !== '');

export function TKDNPanel({ scheduleID, programID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const canOpenSetup = useCan('programs.view');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [totalTKDN, setTotalTKDN] = useState(0);
  const [rows, setRows] = useState<TKDNRow[]>([]);
  const [showEditor, setShowEditor] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  const summary = useQuery({
    queryKey: ['bast', 'tkdn', 'summary', scheduleID],
    queryFn: () => apiRequest<DataResponse<TKDNSummary>>(`/api/v1/bast/tkdn/summary?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  useEffect(() => {
    const profile = summary.data?.data.profile;
    if (profile) { setTotalTKDN(profile.total_tkdn); setRows(profile.rows ?? []); }
  }, [summary.data]);
  const documents = useQuery({
    queryKey: ['bast', 'tkdn', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/tkdn/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  const totalPackages = summary.data?.data.total_packages ?? 0;
  const profile = summary.data?.data.profile;
  const ready = totalPackages > 0;
  const preview = useQuery({
    queryKey: ['bast', 'tkdn', 'preview', scheduleID, documentDate, profile?.updated_at, summary.dataUpdatedAt],
    queryFn: () => apiBlobRequest('/api/v1/bast/tkdn/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && ready && documentDate !== '',
  });
  const saveProfile = useMutation({
    mutationFn: () => apiRequest<DataResponse<TKDNProfile>>('/api/v1/bast/tkdn/profile', { method: 'PUT', body: JSON.stringify({ program_id: programID, rows, total_tkdn: totalTKDN }) }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'tkdn'] }); toast.success('Susunan TKDN program tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument>>('/api/v1/bast/tkdn/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'tkdn'] }); toast.success('Realisasi TKDN berhasil difinalisasi dan disinkronkan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="TKDN Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (summary.isPending) return <DataState kind="loading" title="Memuat TKDN" description="Mengambil daftar TKDN program dan jumlah paket." />;
  if (summary.isError) return <DataState kind="error" title="TKDN belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => summary.refetch() }} />;

  const versions = documents.data?.data ?? [];
  const activeDoc = versions.find((doc) => doc.status === 'active');
  const items = summary.data.data.items ?? [];
  const entries = summary.data.data.entries ?? [];
  const updateRow = (index: number, patch: Partial<TKDNRow>) => setRows((current) => current.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  const editorValid = rowsValid(rows) && percentValid(totalTKDN);

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} />}

    <Card className="min-w-0">
      <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="text-base">Realisasi TKDN</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · disimpan ke 12. TKDN</p></div><div className="flex flex-wrap gap-2"><Badge variant="outline">{totalPackages} paket</Badge></div></div></CardHeader>
      <CardContent className="grid gap-4 pt-5">
        <div className="flex flex-wrap items-end gap-3">
          <div className="w-full sm:w-56"><FormField label="Tanggal dokumen" name="tkdn_document_date" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} /></div>
          <div className="flex w-full items-end gap-2 sm:w-64">
            <div className="min-w-0 flex-1"><FormField label="TKDN gabungan (%)" name="total_tkdn" type="number" min={0} max={100} step="0.01" value={totalTKDN} disabled={!canManage} onChange={(e) => setTotalTKDN(Number(e.target.value))} /></div>
            {canManage && <Button variant="outline" disabled={!editorValid || saveProfile.isPending || totalTKDN === profile?.total_tkdn} onClick={() => saveProfile.mutate()}><Save />{saveProfile.isPending ? 'Menyimpan...' : 'Simpan'}</Button>}
          </div>
          {canOpenSetup && <div className="flex flex-wrap gap-x-4 gap-y-1 pb-2.5 text-xs font-medium sm:ml-auto">
            <Link className="inline-flex items-center gap-1 text-primary hover:underline" to="/persiapan-program?tab=templates">Merk & % TKDN di Template Paket<ExternalLink className="size-3" aria-hidden="true" /></Link>
          </div>}
        </div>
        <p className="-mt-2 text-xs text-muted-foreground">{items.length} baris TKDN; merk dan % TKDN per barang dari Template Paket jadwal. TKDN gabungan berlaku untuk semua kabupaten program.</p>
        <div className="flex flex-wrap gap-2 border-t pt-4">
            {canManage && <Button disabled={finalize.isPending || !ready || documentDate === ''} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi & sinkronkan'}</Button>}
            <Button variant="outline" aria-expanded={showEditor} onClick={() => setShowEditor((value) => !value)}><ListChecks />{showEditor ? 'Tutup susunan TKDN' : 'Susunan TKDN'}</Button>
            <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan'}</Button>
        </div>

        {!ready && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">Belum ada distribusi selesai pada jadwal ini.</div>}

        {activeDoc && <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-[1fr_auto] sm:items-center">
          <div><div className="flex items-center gap-2"><FileCheck2 className="size-4 text-emerald-600" /><strong className="text-sm">{activeDoc.filename}</strong></div><p className="mt-1 text-xs text-muted-foreground">Versi {activeDoc.version} · {activeDoc.page_count} halaman · {activeDoc.recipient_count} paket</p></div>
          <Button nativeButton={false} render={<a href={`/api/v1/bast/tkdn/documents/${activeDoc.id}/content`} />} variant="outline"><Download />Unduh PDF</Button>
        </div>}

        {versions.length > 1 && <div className="grid gap-2 rounded-lg border p-3">
          <strong className="text-sm">Riwayat versi</strong>
          {versions.filter((doc) => doc.status === 'superseded').map((doc) => <div className="flex items-center justify-between gap-3 text-sm" key={doc.id}><span className="text-muted-foreground">Versi {doc.version} · {doc.document_date}</span><Button nativeButton={false} render={<a href={`/api/v1/bast/tkdn/documents/${doc.id}/content`} />} variant="ghost" size="sm"><Download />Unduh</Button></div>)}
        </div>}

        {ready && documentDate !== '' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="Realisasi TKDN" />}
      </CardContent>
    </Card>

    {showEditor && <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Susunan Realisasi TKDN</CardTitle><p className="text-sm text-muted-foreground">Nama baris dan kelompok di dokumen. Setiap baris menunjuk barang Template Paket; merk, jumlah per paket, dan % TKDN diambil dari template jadwal. Baris berurutan dengan kelompok sama dicetak sebagai satu nomor.</p></CardHeader>
      <CardContent className="grid gap-4 pt-5">
        <DataTable label="Susunan TKDN" minimumWidth={860}>
          <thead><tr><th>Nama baris</th><th>Kelompok (opsional)</th><th>Barang template</th><th><span className="sr-only">Aksi</span></th></tr></thead>
          <tbody>{rows.map((row, index) => <tr key={index}>
            <td><Input aria-label={`Nama baris ${index + 1}`} value={row.name} disabled={!canManage} onChange={(e) => updateRow(index, { name: e.target.value })} /></td>
            <td><Input aria-label={`Kelompok baris ${index + 1}`} value={row.group} disabled={!canManage} onChange={(e) => updateRow(index, { group: e.target.value })} /></td>
            <td className="min-w-72"><TemplateEntrySelect entries={entries} value={row.ref} disabled={!canManage} label={`Barang template baris ${index + 1}`} onChange={(ref) => updateRow(index, { ref })} /></td>
            <td>{canManage && <Button variant="ghost" size="icon-sm" aria-label={`Hapus baris ${index + 1}`} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}><Trash2 /></Button>}</td>
          </tr>)}</tbody>
        </DataTable>
        {canManage && <div className="flex flex-wrap justify-between gap-3">
          <Button variant="outline" disabled={entries.length === 0} onClick={() => setRows((current) => [...current, { name: '', group: '', ref: entries[0]?.ref ?? '' }])}><Plus />Tambah baris</Button>
          <Button disabled={!editorValid || saveProfile.isPending} onClick={() => saveProfile.mutate()}><Save />{saveProfile.isPending ? 'Menyimpan...' : 'Simpan susunan TKDN'}</Button>
        </div>}
        {!rowsValid(rows) && <p className="text-xs text-muted-foreground">Setiap baris wajib punya nama dan barang template.</p>}
      </CardContent>
    </Card>}
  </div>;
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
