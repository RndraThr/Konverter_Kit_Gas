import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Download, ExternalLink, FileCheck2, ListChecks, RefreshCw, Settings2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { apiBlobRequest, ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ProgramType } from '../programs/types';
import { PdfPreview } from './PdfPreview';
import { PemeriksaanFormsEditor } from './PemeriksaanFormsEditor';
import { ScheduleSettingsPanel } from './ScheduleSettingsPanel';
import type { AggregateDocument, PemeriksaanForm, PemeriksaanProfile, PemeriksaanSummary } from './types';

type Props = { scheduleID: string; programID: string; regencyName: string; programType: ProgramType; defaultDate: string };
type DataResponse<T> = { data: T };

// Setiap form dirender 2 halaman (isi + tanda tangan) pada preview gabungan.
const pagesPerForm = 2;
const quantityFormat = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 2 });

export function PemeriksaanPanel({ scheduleID, programID, regencyName, programType, defaultDate }: Props) {
  const canManage = useCan('bast.manage');
  const canOpenSetup = useCan('programs.view');
  const queryClient = useQueryClient();
  const [documentDate, setDocumentDate] = useState(defaultDate);
  const [forms, setForms] = useState<PemeriksaanForm[]>([]);
  const [previewPage, setPreviewPage] = useState<number | undefined>();
  const [showEditor, setShowEditor] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  const summary = useQuery({
    queryKey: ['bast', 'pemeriksaan', 'summary', scheduleID],
    queryFn: () => apiRequest<DataResponse<PemeriksaanSummary>>(`/api/v1/bast/pemeriksaan/summary?schedule_id=${encodeURIComponent(scheduleID)}`),
    enabled: programType === 'farmer',
  });
  useEffect(() => {
    const data = summary.data?.data;
    if (data) setForms(normalizeForms(data.profile.forms));
  }, [summary.data]);
  const documents = useQuery({
    queryKey: ['bast', 'pemeriksaan', 'documents', scheduleID, documentDate],
    queryFn: () => apiRequest<DataResponse<AggregateDocument[]>>(`/api/v1/bast/pemeriksaan/documents?schedule_id=${encodeURIComponent(scheduleID)}&date=${documentDate}`),
    enabled: programType === 'farmer' && documentDate !== '',
  });

  const totalPackages = summary.data?.data.total_packages ?? 0;
  const profile = summary.data?.data.profile;
  const ready = totalPackages > 0;
  const preview = useQuery({
    queryKey: ['bast', 'pemeriksaan', 'preview', scheduleID, documentDate, profile?.updated_at, summary.dataUpdatedAt],
    queryFn: () => apiBlobRequest('/api/v1/bast/pemeriksaan/preview', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    enabled: programType === 'farmer' && ready && documentDate !== '',
  });
  const saveProfile = useMutation({
    mutationFn: () => apiRequest<DataResponse<PemeriksaanProfile>>('/api/v1/bast/pemeriksaan/profile', { method: 'PUT', body: JSON.stringify({ program_id: programID, forms }) }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['bast', 'pemeriksaan'] }); toast.success('Daftar form BA Pemeriksaan tersimpan.'); },
    onError: (error) => toast.error(errorMessage(error)),
  });
  const finalize = useMutation({
    mutationFn: () => apiRequest<DataResponse<AggregateDocument[]>>('/api/v1/bast/pemeriksaan/finalize', { method: 'POST', body: JSON.stringify({ schedule_id: scheduleID, document_date: documentDate }) }),
    onSuccess: (result) => { queryClient.invalidateQueries({ queryKey: ['bast', 'pemeriksaan'] }); toast.success(`${result.data.length} BA Pemeriksaan difinalisasi dan disinkronkan.`); },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (programType === 'fisherman') return <DataState kind="empty" title="BA Pemeriksaan Nelayan belum tersedia" description="Fase pertama hanya mendukung program Petani." />;
  if (summary.isPending) return <DataState kind="loading" title="Memuat BA Pemeriksaan" description="Mengambil daftar form, barang, dan jumlah paket." />;
  if (summary.isError) return <DataState kind="error" title="BA Pemeriksaan belum dapat dimuat" description="Periksa konfigurasi program atau koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => summary.refetch() }} />;

  // Form tanpa barang di Template Paket tidak diterbitkan dan tidak ada di preview.
  const savedForms = (summary.data.data.forms ?? []).filter((form) => form.rows.length > 0);
  const versions = documents.data?.data ?? [];
  const activeByType = new Map(versions.filter((doc) => doc.status === 'active').map((doc) => [doc.document_type, doc]));
  const finalizedCount = savedForms.filter((form) => activeByType.has(`pemeriksaan:${form.code}`)).length;

  return <div className="grid gap-5">
    {showSettings && <ScheduleSettingsPanel scheduleID={scheduleID} requirePertaminaRep />}

    <div className="grid gap-5 xl:grid-cols-[22rem_minmax(0,1fr)]">
      <div className="grid content-start gap-5">
      <Card>
        <CardHeader className="border-b"><div className="flex items-center justify-between gap-2"><CardTitle className="text-base">Form barang</CardTitle><Badge variant="outline">{finalizedCount}/{savedForms.length} final</Badge></div></CardHeader>
        <CardContent className="grid gap-3 pt-4">
          <ul className="grid divide-y rounded-lg border" aria-label="Form BA Pemeriksaan">
            {savedForms.map((form, index) => {
              const doc = activeByType.get(`pemeriksaan:${form.code}`);
              const page = index * pagesPerForm + 1;
              const quantities = form.rows.map((row) => `${quantityFormat.format(row.quantity_per_package * totalPackages)} ${row.unit.replace(/\s+/g, ' ')}`).join(' · ');
              const active = previewPage === page;
              const info = <>
                <div className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{form.title}</span><span className="block truncate text-xs text-muted-foreground">{quantities}</span><span className={`block truncate text-xs ${form.po_number ? 'text-muted-foreground' : 'text-amber-700 dark:text-amber-300'}`}>PO {form.po_number || 'belum diisi'}</span></div>
                {doc ? <Badge className="shrink-0"><FileCheck2 />V{doc.version}</Badge> : <Badge variant="outline" className="shrink-0">Draft</Badge>}
              </>;
              // Seluruh baris (nama, jumlah, PO, status) membuka halaman form di preview gabungan.
              return <li key={form.code} className={`flex items-center gap-1 pr-2 transition-colors ${active ? 'bg-primary/10' : ''}`}>
                {ready
                  ? <button type="button" aria-label={`Lihat ${form.title}`} aria-pressed={active} onClick={() => setPreviewPage(page)}
                    className={`flex min-w-0 flex-1 items-center gap-2 py-1.5 pr-1 pl-3 text-left outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring/50 ${active ? 'shadow-[inset_3px_0_0_var(--primary)]' : ''}`}>
                    {info}
                  </button>
                  : <div className="flex min-w-0 flex-1 items-center gap-2 py-1.5 pl-3">{info}</div>}
                {doc &&<Button nativeButton={false} render={<a href={`/api/v1/bast/pemeriksaan/documents/${doc.id}/content`} aria-label={`Unduh ${form.title}`} />} variant="ghost" size="icon-sm"><Download /></Button>}
              </li>;
            })}
          </ul>
          <Button variant="outline" aria-expanded={showEditor} onClick={() => setShowEditor((value) => !value)}><ListChecks />{showEditor ? 'Tutup daftar form' : 'Atur daftar form'}</Button>
          {profile?.is_default && <p className="text-xs text-muted-foreground">Memakai daftar bawaan dari contoh dokumen.</p>}
        </CardContent>
      </Card>
      </div>

      <Card className="min-w-0">
        <CardHeader className="border-b"><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle className="text-base">BA Pemeriksaan Barang</CardTitle><p className="mt-1 text-sm text-muted-foreground">{regencyName} · 1 PDF per form, disimpan ke 10. BA PEMERIKSAAN</p></div><Badge variant="outline">{totalPackages} paket</Badge></div></CardHeader>
        <CardContent className="grid gap-4 pt-5">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="grid w-full gap-1.5 sm:w-56">
              <Label htmlFor="pemeriksaan_document_date">Tanggal pemeriksaan</Label>
              <Input id="pemeriksaan_document_date" type="date" value={documentDate} onChange={(e) => setDocumentDate(e.target.value)} />
            </div>
            {canOpenSetup && <div className="flex flex-wrap gap-x-4 gap-y-1 pb-2.5 text-xs font-medium">
              <Link className="inline-flex items-center gap-1 text-primary hover:underline" to={`/persiapan-program?tab=zones&program=${encodeURIComponent(programID)}`}>Isi No. PO per zona<ExternalLink className="size-3" aria-hidden="true" /></Link>
              <Link className="inline-flex items-center gap-1 text-primary hover:underline" to="/persiapan-program?tab=templates">Merk & barang di Template Paket<ExternalLink className="size-3" aria-hidden="true" /></Link>
            </div>}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
            <Button variant="ghost" onClick={() => setShowSettings((value) => !value)}><Settings2 />{showSettings ? 'Tutup pengaturan' : 'Pengaturan penandatangan'}</Button>
            {canManage && <Button disabled={finalize.isPending || !ready || documentDate === ''} onClick={() => finalize.mutate()}><RefreshCw className={finalize.isPending ? 'animate-spin' : ''} />{finalize.isPending ? 'Menyinkronkan...' : 'Finalisasi semua form'}</Button>}
          </div>

          {!ready && <div className="rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-sm text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">Belum ada distribusi selesai pada jadwal ini.</div>}

          {ready && documentDate !== '' && <PdfPreview blob={preview.data} isPending={preview.isPending} isError={preview.isError} label="BA Pemeriksaan" page={previewPage} />}
        </CardContent>
      </Card>
    </div>

    {showEditor && <PemeriksaanFormsEditor forms={forms} entries={summary.data.data.entries ?? []} canManage={canManage} saving={saveProfile.isPending} onChange={setForms} onSave={() => saveProfile.mutate()} />}
  </div>;
}

// Server lama dapat mengirim null untuk daftar kosong; editor mengharapkan array.
function normalizeForms(forms: PemeriksaanForm[] | null | undefined): PemeriksaanForm[] {
  return (forms ?? []).map((form) => ({
    ...form,
    rows: form.rows ?? [],
    note: form.note ?? '',
    checklist: { ...form.checklist, documents: form.checklist?.documents ?? [], other_document: form.checklist?.other_document ?? '' },
  }));
}

function errorMessage(error: unknown) {
  return error instanceof ApiError || error instanceof Error ? error.message : 'Permintaan belum dapat diproses.';
}
