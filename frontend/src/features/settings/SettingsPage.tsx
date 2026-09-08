import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { FormField } from '../../components/FormField';
import { PageHeader } from '../../components/PageHeader';
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { apiRequest } from '../../lib/api';
import { useCan } from '../../lib/permissions';

type Setting = { key: string; value: string; type: string; description: string };
const labels: Record<string, string> = { application_name: 'Nama aplikasi', organization_name: 'Nama organisasi', timezone: 'Zona waktu', date_format: 'Format tanggal', locale: 'Bahasa' };
const options: Record<string, string[]> = { timezone: ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura'], date_format: ['02/01/2006', '02 January 2006'], locale: ['id-ID'] };

export function SettingsPage() {
  const canManage = useCan('settings.manage'); const client = useQueryClient();
  const query = useQuery({ queryKey: ['settings'], queryFn: () => apiRequest<{ data: Setting[] }>('/api/v1/system/settings') });
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => { if (query.data) setValues(Object.fromEntries(query.data.data.map((item) => [item.key, item.value]))); }, [query.data]);
  const mutation = useMutation({ mutationFn: () => apiRequest('/api/v1/system/settings', { method: 'PATCH', body: JSON.stringify({ values }) }), onSuccess: () => { client.invalidateQueries({ queryKey: ['settings'] }); toast.success('Pengaturan berhasil disimpan.'); } });
  return <div className="space-y-5"><PageHeader title="Pengaturan sistem" description="Nilai dasar yang digunakan secara konsisten di seluruh program." />
    {query.isError ? <DataState kind="error" title="Pengaturan belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} /> : query.isPending ? <DataState kind="loading" title="Memuat pengaturan" description="Mengambil konfigurasi sistem." /> : <form onSubmit={(event: FormEvent) => { event.preventDefault(); mutation.mutate(); }} className="space-y-5"><section className="rounded-lg border bg-card"><div className="border-b px-5 py-4"><h2 className="font-semibold">Identitas dan regional</h2><p className="mt-1 text-sm text-muted-foreground">Tetapkan nama dan format yang ditampilkan di seluruh aplikasi.</p></div><div className="grid gap-5 p-5 sm:grid-cols-2">{query.data?.data.map((setting) => options[setting.key] ? <label className="grid gap-2" key={setting.key}><span className="text-sm font-medium">{labels[setting.key]}</span><select className="h-11 rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50" disabled={!canManage} aria-label={labels[setting.key]} value={values[setting.key] ?? ''} onChange={(event) => setValues({ ...values, [setting.key]: event.target.value })}>{options[setting.key].map((item) => <option key={item}>{item}</option>)}</select>{setting.description && <small className="text-sm text-muted-foreground">{setting.description}</small>}</label> : <FormField key={setting.key} label={labels[setting.key] ?? setting.key} name={setting.key} disabled={!canManage} maxLength={setting.key === 'organization_name' ? 160 : 120} value={values[setting.key] ?? ''} onChange={(event) => setValues({ ...values, [setting.key]: event.target.value })} hint={setting.description} />)}</div></section>{mutation.isError && <Alert variant="destructive"><AlertTitle>Pengaturan belum dapat disimpan</AlertTitle><AlertDescription>Coba simpan kembali setelah memeriksa nilai yang diisi.</AlertDescription></Alert>}{canManage && <div className="sticky bottom-3 z-10 flex justify-end rounded-lg border bg-background/95 p-3 shadow-sm backdrop-blur sm:static sm:border-0 sm:bg-transparent sm:p-0 sm:shadow-none"><Button disabled={mutation.isPending} type="submit">Simpan pengaturan</Button></div>}</form>}
  </div>;
}
