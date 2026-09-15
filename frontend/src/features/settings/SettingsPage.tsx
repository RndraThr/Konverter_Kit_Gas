import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
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
    {query.isError ? <DataState kind="error" title="Pengaturan belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} /> : query.isPending ? <DataState kind="loading" title="Memuat pengaturan" description="Mengambil konfigurasi sistem." /> : <form onSubmit={(event: FormEvent) => { event.preventDefault(); mutation.mutate(); }} className="space-y-5"><section className="rounded-lg border bg-card"><div className="border-b px-5 py-4"><h2 className="font-semibold">Identitas dan regional</h2><p className="mt-1 text-sm text-muted-foreground">Tetapkan nama dan format yang ditampilkan di seluruh aplikasi.</p></div><div className="grid gap-5 p-5 sm:grid-cols-2">{query.data?.data.map((setting) => options[setting.key] ? <div className="grid min-w-0 gap-2" key={setting.key}><Label id={`setting-${setting.key}-label`}>{labels[setting.key]}</Label><Select disabled={!canManage} value={values[setting.key] ?? ''} onValueChange={(value) => setValues({ ...values, [setting.key]: value ?? '' })}><SelectTrigger className="w-full" aria-labelledby={`setting-${setting.key}-label`}><SelectValue /></SelectTrigger><SelectContent>{options[setting.key].map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select>{setting.description && <small className="text-sm text-muted-foreground">{setting.description}</small>}</div> : <FormField key={setting.key} label={labels[setting.key] ?? setting.key} name={setting.key} disabled={!canManage} maxLength={setting.key === 'organization_name' ? 160 : 120} value={values[setting.key] ?? ''} onChange={(event) => setValues({ ...values, [setting.key]: event.target.value })} hint={setting.description} />)}</div></section>{mutation.isError && <Alert variant="destructive"><AlertTitle>Pengaturan belum dapat disimpan</AlertTitle><AlertDescription>Coba simpan kembali setelah memeriksa nilai yang diisi.</AlertDescription></Alert>}{canManage && <div className="flex justify-end border-t pt-5"><Button disabled={mutation.isPending} type="submit">Simpan pengaturan</Button></div>}</form>}
  </div>;
}
