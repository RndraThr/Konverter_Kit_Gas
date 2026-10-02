import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Save, Settings2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import type { ScheduleSettings } from './types';
import { uppercaseBusinessText } from '@/lib/text';

type Props = { scheduleID: string; requirePertaminaRep?: boolean };
type DataResponse<T> = { data: T };

const emptySettings: ScheduleSettings = {
  schedule_id: '', handover_location: '', consultant_company_name: '', agriculture_office_name: '',
  agriculture_office_nip: '', installer_name: '', supervisor_name: '', pertamina_rep_name: '',
};

export function ScheduleSettingsPanel({ scheduleID, requirePertaminaRep = false }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<ScheduleSettings>(emptySettings);

  const settings = useQuery({
    queryKey: ['bast', 'schedule-settings', scheduleID],
    queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`),
  });

  useEffect(() => {
    if (settings.data?.data) setDraft(settings.data.data);
  }, [settings.data]);

  const save = useMutation({
    mutationFn: (input: ScheduleSettings) => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify(input) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] });
      queryClient.invalidateQueries({ queryKey: ['bast', 'dp3'] });
      queryClient.invalidateQueries({ queryKey: ['bast', 'daily-recap'] });
      toast.success('Konfigurasi berita acara tersimpan.');
    },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Gagal menyimpan konfigurasi.'),
  });

  const set = (key: keyof ScheduleSettings) => (value: string) => setDraft((current) => ({
    ...current,
    [key]: key === 'agriculture_office_nip' || key === 'schedule_id' ? value : uppercaseBusinessText(value),
  }));

  if (settings.isPending) return <DataState kind="loading" title="Memuat konfigurasi berita acara" description="Mengambil pengaturan jadwal." />;
  if (settings.isError) return <DataState kind="error" title="Konfigurasi belum dapat dimuat" description="Periksa koneksi, lalu coba kembali." action={{ label: 'Coba lagi', onClick: () => settings.refetch() }} />;

  return <Card>
    <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Settings2 className="size-4 text-primary" />Konfigurasi Berita Acara jadwal</CardTitle></CardHeader>
    <CardContent className="grid gap-4">
      <div className="grid gap-4 md:grid-cols-2">
        <FormField label="Lokasi / Titik Serah" required name="handover_location" value={draft.handover_location} onChange={(e) => set('handover_location')(e.target.value)} disabled={!canManage} />
        <FormField label="Konsultan Distribusi (perusahaan)" name="consultant_company_name" value={draft.consultant_company_name} onChange={(e) => set('consultant_company_name')(e.target.value)} disabled={!canManage} />
        <FormField label="Nama Dinas Pertanian" required name="agriculture_office_name" value={draft.agriculture_office_name} onChange={(e) => set('agriculture_office_name')(e.target.value)} disabled={!canManage} />
        <FormField label="NIP Dinas Pertanian" required name="agriculture_office_nip" value={draft.agriculture_office_nip} onChange={(e) => set('agriculture_office_nip')(e.target.value)} disabled={!canManage} />
        <FormField label="Pelaksana Pemasangan & Pendistribusian" required name="installer_name" value={draft.installer_name} onChange={(e) => set('installer_name')(e.target.value)} disabled={!canManage} />
        <FormField label="Konsultan Pengawas" required name="supervisor_name" value={draft.supervisor_name} onChange={(e) => set('supervisor_name')(e.target.value)} disabled={!canManage} />
        <FormField label="Perwakilan PT Pertamina Patra Niaga" required={requirePertaminaRep} name="pertamina_rep_name" value={draft.pertamina_rep_name} onChange={(e) => set('pertamina_rep_name')(e.target.value)} disabled={!canManage} />
      </div>
      {canManage && <div className="flex justify-end"><Button disabled={save.isPending} onClick={() => save.mutate(draft)}><Save className={save.isPending ? 'animate-pulse' : ''} />{save.isPending ? 'Menyimpan...' : 'Simpan konfigurasi'}</Button></div>}
    </CardContent>
  </Card>;
}
