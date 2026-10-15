import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { lazy, Suspense, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { apiRequest, ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Button } from '../../components/ui/button';
import { FormField } from '../../components/FormField';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { Checkbox } from '../../components/ui/checkbox';
import type { DataResponse } from '../programs/types';
import type { ActivityMediaPage, ProgramOption, ProgramZone } from '../activities/types';
import { SLIDE_KEYS, SLIDE_KEY_LABELS, SLIDE_KEY_ACTIVITY_TYPES, type FinalReportSettings, type FinalReportPhotoPick, type FinalReportDocument } from './types';

const FinalReportPresent = lazy(() => import('./FinalReportPresent'));

// Empty/NaN input becomes 0 and negatives are clamped; the backend rejects negatives with 400.
const toCount = (value: string) => Math.max(0, Number(value) || 0);

export function FinalReportPage({ programId, regencyId }: { programId: string; regencyId: string }) {
  const canManage = useCan('final_report.manage');
  const client = useQueryClient();
  const [presentOpen, setPresentOpen] = useState(false);

  const settingsQuery = useQuery({
    queryKey: ['final-report', 'settings', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportSettings>>(`/api/v1/final-report/settings?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });
  const [draft, setDraft] = useState({ leader_count: 0, admin_count: 0, technician_count: 0, helper_count: 0, machine_manufacturer_name: '', delivery_hose_date: '', delivery_machine_group_date: '', delivery_gas_cylinder_date: '' });
  useEffect(() => {
    const current = settingsQuery.data?.data;
    if (current) setDraft({ leader_count: current.leader_count, admin_count: current.admin_count, technician_count: current.technician_count, helper_count: current.helper_count, machine_manufacturer_name: current.machine_manufacturer_name, delivery_hose_date: current.delivery_hose_date, delivery_machine_group_date: current.delivery_machine_group_date, delivery_gas_cylinder_date: current.delivery_gas_cylinder_date });
  }, [settingsQuery.data]);

  const saveSettings = useMutation({
    mutationFn: () => apiRequest<DataResponse<FinalReportSettings>>('/api/v1/final-report/settings', { method: 'PUT', body: JSON.stringify({ program_id: programId, regency_id: regencyId, ...draft }) }),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['final-report', 'settings', programId, regencyId] }); toast.success('Pengaturan tersimpan.'); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Pengaturan belum dapat disimpan.'),
  });

  const documentsQuery = useQuery({
    queryKey: ['final-report', 'documents', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportDocument[]>>(`/api/v1/final-report/documents?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });
  const generate = useMutation({
    mutationFn: () => apiRequest<DataResponse<FinalReportDocument>>('/api/v1/final-report/generate', { method: 'POST', body: JSON.stringify({ program_id: programId, regency_id: regencyId }) }),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['final-report', 'documents', programId, regencyId] }); toast.success('PPTX berhasil dibuat.'); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'PPTX belum dapat dibuat.'),
  });

  return <div className="space-y-6">
    {canManage && <section className="space-y-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Susunan Tim & Rincian Pengiriman</h2>
      <div className="grid gap-3 sm:grid-cols-4">
        <FormField label="Leader" name="leader_count" type="number" min={0} value={String(draft.leader_count)} onChange={(e) => setDraft((d) => ({ ...d, leader_count: toCount(e.target.value) }))} />
        <FormField label="Administrasi" name="admin_count" type="number" min={0} value={String(draft.admin_count)} onChange={(e) => setDraft((d) => ({ ...d, admin_count: toCount(e.target.value) }))} />
        <FormField label="Teknisi" name="technician_count" type="number" min={0} value={String(draft.technician_count)} onChange={(e) => setDraft((d) => ({ ...d, technician_count: toCount(e.target.value) }))} />
        <FormField label="Helper" name="helper_count" type="number" min={0} value={String(draft.helper_count)} onChange={(e) => setDraft((d) => ({ ...d, helper_count: toCount(e.target.value) }))} />
      </div>
      <FormField label="Nama Pabrikan Mesin" name="machine_manufacturer_name" value={draft.machine_manufacturer_name} onChange={(e) => setDraft((d) => ({ ...d, machine_manufacturer_name: e.target.value }))} />
      <div className="grid gap-3 sm:grid-cols-3">
        <FormField label="Tanggal Kirim Selang Buang" name="delivery_hose_date" type="date" value={draft.delivery_hose_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_hose_date: e.target.value }))} />
        <FormField label="Tanggal Kirim Mesin/Konkit/Oli/dll" name="delivery_machine_group_date" type="date" value={draft.delivery_machine_group_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_machine_group_date: e.target.value }))} />
        <FormField label="Tanggal Kirim Tabung Gas" name="delivery_gas_cylinder_date" type="date" value={draft.delivery_gas_cylinder_date} onChange={(e) => setDraft((d) => ({ ...d, delivery_gas_cylinder_date: e.target.value }))} />
      </div>
      <Button type="button" onClick={() => saveSettings.mutate()} disabled={saveSettings.isPending}>{saveSettings.isPending ? 'Menyimpan...' : 'Simpan Pengaturan'}</Button>
    </section>}

    {SLIDE_KEYS.map((slideKey) => <PhotoPickerSection key={slideKey} programId={programId} regencyId={regencyId} slideKey={slideKey} canManage={canManage} />)}

    <section className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" onClick={() => setPresentOpen(true)}>Tampilkan</Button>
      {canManage && <Button type="button" onClick={() => generate.mutate()} disabled={generate.isPending}>{generate.isPending ? 'Membuat...' : 'Generate PPTX'}</Button>}
    </section>

    {documentsQuery.data && documentsQuery.data.data.length > 0 && <ul className="space-y-1 text-sm">
      {documentsQuery.data.data.map((doc) => <li key={doc.id} className="flex items-center justify-between rounded border p-2">
        <span>v{doc.version} — {doc.filename}</span>
        <span className="text-muted-foreground">{doc.status === 'active' ? 'Aktif' : 'Lama'}</span>
      </li>)}
    </ul>}

    {presentOpen && <Suspense fallback={null}>
      <FinalReportPresent programId={programId} regencyId={regencyId} onClose={() => setPresentOpen(false)} />
    </Suspense>}
  </div>;
}

function PhotoPickerSection({ programId, regencyId, slideKey, canManage }: { programId: string; regencyId: string; slideKey: (typeof SLIDE_KEYS)[number]; canManage: boolean }) {
  const client = useQueryClient();
  const activityTypes = SLIDE_KEY_ACTIVITY_TYPES[slideKey];

  const available = useQuery({
    queryKey: ['activities', 'media', 'final-report-source', regencyId, slideKey],
    queryFn: async () => {
      const pages = await Promise.all(activityTypes.map((type) => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=${type}&regency_id=${encodeURIComponent(regencyId)}`)));
      return pages.flatMap((page) => page.data.items);
    },
  });
  const picks = useQuery({
    queryKey: ['final-report', 'photo-picks', programId, regencyId, slideKey],
    queryFn: () => apiRequest<DataResponse<FinalReportPhotoPick[]>>(`/api/v1/final-report/photo-picks?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}&slide_key=${slideKey}`),
  });
  const pickedIDs = new Set((picks.data?.data ?? []).map((p) => p.activity_media_id));

  const replace = useMutation({
    mutationFn: (ids: string[]) => apiRequest<DataResponse<FinalReportPhotoPick[]>>('/api/v1/final-report/photo-picks', { method: 'PUT', body: JSON.stringify({ program_id: programId, regency_id: regencyId, slide_key: slideKey, activity_media_ids: ids }) }),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['final-report', 'photo-picks', programId, regencyId, slideKey] }),
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Pilihan foto belum dapat disimpan.'),
  });

  const toggle = (mediaID: string) => {
    const next = new Set(pickedIDs);
    if (next.has(mediaID)) next.delete(mediaID);
    else {
      if (next.size >= 6) { toast.error('Maksimal 6 foto per slide.'); return; }
      next.add(mediaID);
    }
    replace.mutate([...next]);
  };

  return <section className="space-y-2 rounded-lg border p-4">
    <h3 className="text-sm font-semibold">{SLIDE_KEY_LABELS[slideKey]}</h3>
    {available.isPending
      ? <p className="text-sm text-muted-foreground">Memuat foto...</p>
      : (available.data ?? []).length === 0
        ? <DataState kind="empty" title="Belum ada foto" description="Unggah dokumentasi kegiatan ini terlebih dahulu." />
        : <ul className="grid grid-cols-3 gap-2 sm:grid-cols-6">
          {(available.data ?? []).map((item) => <li key={item.id} className="relative">
            <img src={item.content_url} alt={item.display_name} className="aspect-square w-full rounded border object-cover" />
            {canManage && <Checkbox className="absolute right-1 top-1 bg-background" checked={pickedIDs.has(item.id)} onCheckedChange={() => toggle(item.id)} aria-label={`Pilih ${item.display_name}`} />}
          </li>)}
        </ul>}
  </section>;
}

export default function FinalReportRoute() {
  const [params, setParams] = useSearchParams();
  const programID = params.get('program_id') ?? '';
  const regencyID = params.get('regency_id') ?? '';

  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<DataResponse<ProgramOption[]>>('/api/v1/program-setup/programs') });
  const zones = useQuery({
    queryKey: ['program-setup', 'programs', programID, 'zones'],
    queryFn: () => apiRequest<DataResponse<ProgramZone[]>>(`/api/v1/program-setup/programs/${programID}/zones`),
    enabled: programID !== '',
  });
  const regencies = zones.data?.data.flatMap((zone) => zone.regencies ?? []) ?? [];

  const setProgram = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('program_id', value); else next.delete('program_id'); next.delete('regency_id'); return next; });
  const setRegency = (value: string | null) => setParams((prev) => { const next = new URLSearchParams(prev); if (value) next.set('regency_id', value); else next.delete('regency_id'); return next; });

  return <div className="page">
    <PageHeader title="Laporan Akhir" description="Presentasi ringkasan kegiatan per kabupaten, datanya otomatis mengikuti kondisi lapangan terbaru." />
    <section className="grid gap-4 py-5 sm:grid-cols-2" aria-label="Konteks laporan akhir">
      <div className="grid gap-1.5">
        <Label id="final-report-program-label">Program</Label>
        <Select disabled={programs.isPending} value={programID} onValueChange={setProgram}>
          <SelectTrigger className="w-full" aria-labelledby="final-report-program-label"><SelectValue placeholder={programs.isPending ? 'Memuat program...' : 'Pilih program'} /></SelectTrigger>
          <SelectContent>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <div className="grid gap-1.5">
        <Label id="final-report-regency-label">Kabupaten</Label>
        <Select disabled={!programID || zones.isPending} value={regencyID} onValueChange={setRegency}>
          <SelectTrigger className="w-full" aria-labelledby="final-report-regency-label"><SelectValue placeholder={!programID ? 'Pilih program terlebih dahulu' : zones.isPending ? 'Memuat kabupaten...' : 'Pilih kabupaten'} /></SelectTrigger>
          <SelectContent>{regencies.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
    </section>
    {!programID || !regencyID
      ? <DataState kind="empty" title="Pilih program dan kabupaten terlebih dahulu" description="Pilih program lalu kabupaten untuk melihat Laporan Akhir." />
      : <FinalReportPage programId={programID} regencyId={regencyID} />}
  </div>;
}
