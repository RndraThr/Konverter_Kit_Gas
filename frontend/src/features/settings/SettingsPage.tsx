import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarDays, Clock3, Eye, History, RotateCcw, Save, ShieldAlert } from 'lucide-react';
import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { FormField } from '../../components/FormField';
import { PageHeader } from '../../components/PageHeader';
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { Label } from '../../components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { uppercaseBusinessText } from '../../lib/text';
import { formatSettingsPreview } from './settingsPresentation';

type Setting = {
  key: string;
  value: string;
  type: string;
  description: string;
  updated_by?: string;
  updated_by_name?: string;
  updated_at: string;
};

const labels: Record<string, string> = {
  application_name: 'Nama aplikasi',
  organization_name: 'Nama organisasi',
  timezone: 'Zona waktu',
  date_format: 'Format tanggal',
  locale: 'Bahasa',
};
const options: Record<string, string[]> = {
  timezone: ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura'],
  date_format: ['02/01/2006', '02 January 2006'],
  locale: ['id-ID'],
};
const identityKeys = new Set(['application_name', 'organization_name']);

function valuesFromSettings(settings: Setting[] | undefined) {
  return Object.fromEntries((settings ?? []).map((item) => [item.key, item.value]));
}

function SettingsField({ setting, value, disabled, onChange }: {
  setting: Setting;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  if (options[setting.key]) {
    return <div className="grid min-w-0 gap-2">
      <Label id={`setting-${setting.key}-label`}>{labels[setting.key]}</Label>
      <Select disabled={disabled} value={value} onValueChange={(next) => onChange(next ?? '')}>
        <SelectTrigger className="w-full" aria-labelledby={`setting-${setting.key}-label`}><SelectValue /></SelectTrigger>
        <SelectContent>{options[setting.key].map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
      </Select>
      {setting.description ? <small className="text-sm leading-5 text-muted-foreground">{setting.description}</small> : null}
    </div>;
  }
  return <FormField
    label={labels[setting.key] ?? setting.key}
    name={setting.key}
    disabled={disabled}
    maxLength={setting.key === 'organization_name' ? 160 : 120}
    value={value}
    onChange={(event) => onChange(event.target.value)}
    hint={setting.description}
  />;
}

export function SettingsPage() {
  const canManage = useCan('settings.manage');
  const client = useQueryClient();
  const query = useQuery({ queryKey: ['settings'], queryFn: () => apiRequest<{ data: Setting[] }>('/api/v1/system/settings') });
  const serverValues = useMemo(() => valuesFromSettings(query.data?.data), [query.data]);
  const [draft, setDraft] = useState<Record<string, string>>({});

  useEffect(() => {
    if (query.data) setDraft(serverValues);
  }, [query.data, serverValues]);

  const updateDraft = useCallback((key: string, next: string) => {
    const value = identityKeys.has(key) ? uppercaseBusinessText(next) : next;
    setDraft((current) => ({ ...current, [key]: value }));
  }, []);

  const changedValues = useMemo(() => Object.fromEntries(
    Object.keys(serverValues)
      .filter((key) => draft[key] !== undefined && draft[key] !== serverValues[key])
      .map((key) => [key, draft[key]]),
  ), [draft, serverValues]);
  const isDirty = Object.keys(changedValues).length > 0;
  const isValid = Object.values(draft).every((value) => value.trim().length > 0);

  const mutation = useMutation({
    mutationFn: () => apiRequest<{ data: Setting[] }>('/api/v1/system/settings', {
      method: 'PATCH',
      body: JSON.stringify({ values: changedValues }),
    }),
    onSuccess: (response) => {
      client.setQueryData(['settings'], response);
      setDraft(valuesFromSettings(response.data));
      toast.success('Pengaturan berhasil disimpan.');
    },
  });

  const settings = query.data?.data ?? [];
  const identitySettings = settings.filter((setting) => identityKeys.has(setting.key));
  const regionalSettings = settings.filter((setting) => !identityKeys.has(setting.key));
  const latest = [...settings].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())[0];
  const previewTime = latest?.updated_at ?? new Date().toISOString();

  return <div className="space-y-6">
    <PageHeader title="Pengaturan sistem" description="Kelola identitas aplikasi dan format regional yang digunakan secara konsisten." />
    {query.isError
      ? <DataState kind="error" title="Pengaturan belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} />
      : query.isPending
        ? <DataState kind="loading" title="Memuat pengaturan" description="Mengambil konfigurasi sistem." />
        : <form onSubmit={(event: FormEvent) => { event.preventDefault(); if (canManage && isDirty && isValid) mutation.mutate(); }} className="space-y-5">
          {!canManage ? <Alert><ShieldAlert aria-hidden="true" /><AlertTitle>Akses hanya baca</AlertTitle><AlertDescription>Perubahan pengaturan memerlukan izin pengelolaan sistem.</AlertDescription></Alert> : null}

          <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1.55fr)_minmax(18rem,0.75fr)] lg:items-start">
            <div className="min-w-0 space-y-5">
              <section className="overflow-hidden rounded-lg border bg-card" aria-labelledby="settings-identity-title">
                <div className="border-b px-5 py-4"><h2 id="settings-identity-title" className="font-semibold">Identitas</h2><p className="mt-1 text-sm leading-5 text-muted-foreground">Nama yang tampil pada aplikasi dan dokumen operasional.</p></div>
                <div className="grid gap-5 p-5 sm:grid-cols-2">{identitySettings.map((setting) => <SettingsField key={setting.key} setting={setting} value={draft[setting.key] ?? ''} disabled={!canManage} onChange={(value) => updateDraft(setting.key, value)} />)}</div>
              </section>
              <section className="overflow-hidden rounded-lg border bg-card" aria-labelledby="settings-regional-title">
                <div className="border-b px-5 py-4"><h2 id="settings-regional-title" className="font-semibold">Regional</h2><p className="mt-1 text-sm leading-5 text-muted-foreground">Zona waktu, bahasa, dan pola tanggal untuk seluruh sistem.</p></div>
                <div className="grid gap-5 p-5 sm:grid-cols-2">{regionalSettings.map((setting) => <SettingsField key={setting.key} setting={setting} value={draft[setting.key] ?? ''} disabled={!canManage} onChange={(value) => updateDraft(setting.key, value)} />)}</div>
              </section>
            </div>

            <aside className="min-w-0 overflow-hidden rounded-lg border bg-card lg:sticky lg:top-5" aria-labelledby="settings-preview-title">
              <div className="flex items-center gap-3 border-b px-5 py-4"><span className="grid size-9 place-items-center rounded-lg bg-primary/10 text-primary"><Eye className="size-4" aria-hidden="true" /></span><div><h2 id="settings-preview-title" className="font-semibold">Pratinjau tampilan</h2><p className="mt-0.5 text-xs text-muted-foreground">Contoh hasil konfigurasi saat ini.</p></div></div>
              <div className="space-y-5 p-5">
                <div className="rounded-lg border bg-muted/20 p-4"><p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Aplikasi</p><p className="mt-2 break-words text-lg font-semibold">{draft.application_name || 'Nama aplikasi'}</p><p className="mt-1 break-words text-sm text-muted-foreground">{draft.organization_name || 'Nama organisasi'}</p></div>
                <dl className="space-y-3 text-sm">
                  <div className="flex items-start gap-3"><CalendarDays className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><dt className="text-xs text-muted-foreground">Tanggal dan waktu</dt><dd className="mt-0.5 font-medium">{formatSettingsPreview(previewTime, draft.timezone || 'Asia/Jakarta', draft.date_format || '02/01/2006', draft.locale || 'id-ID')}</dd></div></div>
                  <div className="flex items-start gap-3"><Clock3 className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" /><div><dt className="text-xs text-muted-foreground">Zona waktu</dt><dd className="mt-0.5 font-medium">{draft.timezone || '—'}</dd></div></div>
                </dl>
                {latest ? <div className="flex gap-3 border-t pt-4 text-sm"><History className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><p className="leading-5 text-muted-foreground">Terakhir diperbarui oleh <strong className="font-medium text-foreground">{latest.updated_by_name || 'Sistem'}</strong><br />{new Date(latest.updated_at).toLocaleString('id-ID')}</p></div> : null}
              </div>
            </aside>
          </div>

          {mutation.isError ? <Alert variant="destructive"><AlertTitle>Pengaturan belum dapat disimpan</AlertTitle><AlertDescription>Draft Anda tetap tersimpan. Periksa koneksi dan coba kembali.</AlertDescription></Alert> : null}
          <div className="flex flex-col gap-3 border-t pt-5 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">{isDirty ? `${Object.keys(changedValues).length} perubahan belum disimpan` : 'Tidak ada perubahan yang belum disimpan'}</p>
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button type="button" variant="outline" disabled={!canManage || !isDirty || mutation.isPending} onClick={() => setDraft(serverValues)}><RotateCcw aria-hidden="true" />Batalkan perubahan</Button>
              <Button type="submit" disabled={!canManage || !isDirty || !isValid || mutation.isPending}><Save aria-hidden="true" />{mutation.isPending ? 'Menyimpan…' : 'Simpan perubahan'}</Button>
            </div>
          </div>
        </form>}
  </div>;
}
