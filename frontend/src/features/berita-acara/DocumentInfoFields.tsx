import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { FormField } from '@/components/FormField';
import { ApiError, apiRequest } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { uppercaseBusinessText } from '@/lib/text';
import type { ScheduleSettings } from './types';

type FieldKey = 'handover_location' | 'consultant_company_name';
type Field = { key: FieldKey; label: string };
type Props = { scheduleID: string; fields: Field[] };
type DataResponse<T> = { data: T };

/**
 * Keterangan dokumen yang dicetak di kop BA (Lokasi / Titik Serah, Konsultan
 * Distribusi). Dirender sebagai isian lepas agar sejajar dengan tanggal
 * dokumen di grid panel; tersimpan otomatis saat isian ditinggalkan. Nilainya
 * per jadwal dan dipakai semua BA kabupaten ini. Konsultan Distribusi yang
 * dikosongkan otomatis memakai Nama Organisasi (Sistem > Pengaturan).
 */
export function DocumentInfoFields({ scheduleID, fields }: Props) {
  const canManage = useCan('bast.manage');
  const queryClient = useQueryClient();
  const settings = useQuery({
    queryKey: ['bast', 'schedule-settings', scheduleID],
    queryFn: () => apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`),
  });
  const current = settings.data?.data;
  const [draft, setDraft] = useState<Record<FieldKey, string>>({ handover_location: '', consultant_company_name: '' });
  useEffect(() => {
    if (current) setDraft({ handover_location: current.handover_location ?? '', consultant_company_name: current.consultant_company_name ?? '' });
  }, [current]);

  const save = useMutation({
    mutationFn: (next: Record<FieldKey, string>) => {
      if (!current) throw new Error('Pengaturan jadwal belum dimuat.');
      return apiRequest<DataResponse<ScheduleSettings>>(`/api/v1/bast/schedules/${encodeURIComponent(scheduleID)}/settings`, { method: 'PUT', body: JSON.stringify({ ...current, ...next, updated_at: undefined }) });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bast', 'schedule-settings', scheduleID] });
      queryClient.invalidateQueries({ predicate: (query) => ['preview', 'summary', 'dates', 'rows'].some((key) => query.queryKey.includes(key)) });
      toast.success('Keterangan dokumen tersimpan.');
    },
    onError: (error) => toast.error(error instanceof ApiError || error instanceof Error ? error.message : 'Gagal menyimpan keterangan dokumen.'),
  });

  const commit = (key: FieldKey) => {
    if (!current || !canManage) return;
    const value = draft[key].trim();
    if (value === (current[key] ?? '').trim()) return;
    save.mutate({ ...draft, [key]: value });
  };
  const fallback = current?.consultant_company_default?.trim();

  return <>
    {fields.map(({ key, label }) => <FormField
      key={key}
      label={label}
      name={key}
      value={draft[key]}
      disabled={!canManage || !current}
      placeholder={key === 'consultant_company_name' ? fallback : 'Belum diisi'}
      hint={key === 'consultant_company_name' && fallback ? 'Kosong = Nama Organisasi.' : undefined}
      onChange={(event) => setDraft((value) => ({ ...value, [key]: uppercaseBusinessText(event.target.value) }))}
      onBlur={() => commit(key)}
      onKeyDown={(event) => { if (event.key === 'Enter') event.currentTarget.blur(); }}
    />)}
  </>;
}
