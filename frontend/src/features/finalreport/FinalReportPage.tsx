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
import { SLIDE_KEYS, SLIDE_KEY_LABELS, SLIDE_KEY_ACTIVITY_TYPES, SLIDE_KEY_MAX_PICKS, type FinalReportSettings, type FinalReportPhotoPick, type FinalReportDocument } from './types';

const FinalReportPresent = lazy(() => import('./FinalReportPresent'));

// Empty/NaN input becomes 0 and negatives are clamped; the backend rejects negatives with 400.
const toCount = (value: string) => Math.max(0, Number(value) || 0);

type SettingsDraft = Omit<FinalReportSettings, 'program_id' | 'regency_id'>;
const emptyDraft: SettingsDraft = { leader_count: 0, admin_count: 0, technician_count: 0, helper_count: 0, machine_manufacturer_name: '', delivery_hose_date: '', delivery_machine_group_date: '', delivery_gas_cylinder_date: '' };
const draftFrom = (current: FinalReportSettings): SettingsDraft => ({ leader_count: current.leader_count, admin_count: current.admin_count, technician_count: current.technician_count, helper_count: current.helper_count, machine_manufacturer_name: current.machine_manufacturer_name, delivery_hose_date: current.delivery_hose_date, delivery_machine_group_date: current.delivery_machine_group_date, delivery_gas_cylinder_date: current.delivery_gas_cylinder_date });
const sameDraft = (a: SettingsDraft, b: SettingsDraft) => (Object.keys(emptyDraft) as (keyof SettingsDraft)[]).every((key) => a[key] === b[key]);

// Only JPEG/PNG still images can be placed in the PPTX's .jpg slots.
const isPickableImage = (item: { media_type: string; mime_type: string }) => item.media_type === 'image' && (item.mime_type === 'image/jpeg' || item.mime_type === 'image/png');

export function FinalReportPage({ programId, regencyId }: { programId: string; regencyId: string }) {
  const canManage = useCan('final_report.manage');
  const client = useQueryClient();
  const [presentOpen, setPresentOpen] = useState(false);

  const settingsQuery = useQuery({
    queryKey: ['final-report', 'settings', programId, regencyId],
    queryFn: () => apiRequest<DataResponse<FinalReportSettings>>(`/api/v1/final-report/settings?program_id=${encodeURIComponent(programId)}&regency_id=${encodeURIComponent(regencyId)}`),
  });
  const [draft, setDraft] = useState<SettingsDraft>(emptyDraft);
  useEffect(() => {
    const current = settingsQuery.data?.data;
    if (current) setDraft(draftFrom(current));
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
  // Generate reads the saved settings server-side, so it must not run while
  // the form shows edits that are not saved yet.
  const savedSettings = settingsQuery.data?.data;
  const settingsDirty = savedSettings !== undefined && !sameDraft(draft, draftFrom(savedSettings));
  const settingsBusy = settingsDirty || saveSettings.isPending || settingsQuery.isFetching;
  const generate = useMutation({
    mutationFn: () => apiRequest<DataResponse<FinalReportDocument>>('/api/v1/final-report/generate', { method: 'POST', body: JSON.stringify({ program_id: programId, regency_id: regencyId }) }),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ['final-report', 'documents', programId, regencyId] }); toast.success('PPTX berhasil dibuat.'); },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'PPTX belum dapat dibuat.'),
  });

  return <div className="space-y-6">
    {canManage && <section className="space-y-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Susunan Tim & Rincian Pengiriman</h2>
      {settingsQuery.isError
        ? <DataState kind="error" title="Pengaturan belum dapat dimuat" description="Periksa koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void settingsQuery.refetch() }} />
        : <>
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
      <Button type="button" onClick={() => saveSettings.mutate()} disabled={saveSettings.isPending || settingsQuery.isPending}>{saveSettings.isPending ? 'Menyimpan...' : 'Simpan Pengaturan'}</Button>
        </>}
    </section>}

    {SLIDE_KEYS.map((slideKey) => <PhotoPickerSection key={slideKey} programId={programId} regencyId={regencyId} slideKey={slideKey} canManage={canManage} />)}

    <section className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" onClick={() => setPresentOpen(true)}>Tampilkan</Button>
      {canManage && <Button type="button" onClick={() => generate.mutate()} disabled={generate.isPending || !savedSettings || settingsBusy}>{generate.isPending ? 'Membuat...' : 'Generate PPTX'}</Button>}
      {canManage && (settingsDirty || saveSettings.isPending) && <p className="text-sm text-muted-foreground">Simpan pengaturan terlebih dahulu</p>}
    </section>

    {documentsQuery.isError
      ? <DataState kind="error" title="Riwayat versi belum dapat dimuat" description="Periksa koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void documentsQuery.refetch() }} />
      : documentsQuery.data && documentsQuery.data.data.length > 0 && <ul className="space-y-1 text-sm" aria-label="Riwayat versi Laporan Akhir">
        {documentsQuery.data.data.map((doc) => <li key={doc.id} className="flex flex-wrap items-center justify-between gap-2 rounded border p-2">
          <span className="min-w-0 break-words">v{doc.version} — {doc.filename}</span>
          <span className="flex items-center gap-3">
            <span className="tabular-nums text-muted-foreground">{new Date(doc.finalized_at).toLocaleString('id-ID')}</span>
            <span className="text-muted-foreground">{doc.status === 'active' ? 'Aktif' : 'Lama'}</span>
            <a className="font-medium underline-offset-4 hover:underline" href={`/api/v1/final-report/documents/${encodeURIComponent(doc.id)}/content`} aria-label={`Unduh v${doc.version}`}>Unduh</a>
          </span>
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
  const maxPicks = SLIDE_KEY_MAX_PICKS[slideKey];

  const available = useQuery({
    queryKey: ['activities', 'media', 'final-report-source', programId, regencyId, slideKey],
    queryFn: async () => {
      const pages = await Promise.all(activityTypes.map((type) => apiRequest<DataResponse<ActivityMediaPage>>(`/api/v1/activities/media?activity_type=${type}&regency_id=${encodeURIComponent(regencyId)}&program_id=${encodeURIComponent(programId)}&page_size=100`)));
      return pages.flatMap((page) => page.data.items).filter(isPickableImage);
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

  // Block toggles until saved picks are loaded and while a save is in flight,
  // otherwise a click would compute from stale state and overwrite picks.
  const locked = !picks.isSuccess || picks.isFetching || replace.isPending;
  const toggle = (mediaID: string) => {
    if (locked) return;
    const next = new Set(pickedIDs);
    if (next.has(mediaID)) next.delete(mediaID);
    else {
      if (next.size >= maxPicks) { toast.error(`Maksimal ${maxPicks} foto untuk slide ini.`); return; }
      next.add(mediaID);
    }
    replace.mutate([...next]);
  };

  // Saved picks are always shown, even when they fall outside the fetched
  // list (older than its first 100 items, or another source), so they can
  // still be unchecked.
  const listed = available.data ?? [];
  const listedIDs = new Set(listed.map((item) => item.id));
  const savedOnly = (picks.data?.data ?? []).filter((pick) => !listedIDs.has(pick.activity_media_id)).map((pick) => ({ id: pick.activity_media_id, display_name: pick.display_name, content_url: pick.content_url }));
  const items = [...savedOnly, ...listed.map((item) => ({ id: item.id, display_name: item.display_name, content_url: item.content_url }))];

  return <section className="space-y-2 rounded-lg border p-4">
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <h3 className="text-sm font-semibold">{SLIDE_KEY_LABELS[slideKey]}</h3>
      <span className="text-xs text-muted-foreground">Maksimal {maxPicks} foto</span>
    </div>
    {picks.isError && <DataState kind="error" title="Pilihan foto belum dapat dimuat" description="Periksa koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void picks.refetch() }} />}
    {available.isError
      ? <DataState kind="error" title="Foto kegiatan belum dapat dimuat" description="Periksa koneksi, lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => void available.refetch() }} />
      : available.isPending
        ? <p className="text-sm text-muted-foreground">Memuat foto...</p>
        : items.length === 0
          ? <DataState kind="empty" title="Belum ada foto" description="Unggah dokumentasi kegiatan ini terlebih dahulu." />
          : <ul className="grid grid-cols-3 gap-2 sm:grid-cols-6">
            {items.map((item) => <li key={item.id} className="relative">
              <img src={item.content_url} alt={item.display_name} className="aspect-square w-full rounded border object-cover" />
              {canManage && <Checkbox className="absolute right-1 top-1 bg-background" checked={pickedIDs.has(item.id)} disabled={locked} onCheckedChange={() => toggle(item.id)} aria-label={`Pilih ${item.display_name}`} />}
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
