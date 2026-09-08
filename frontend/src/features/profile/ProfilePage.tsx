import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { Card, CardContent } from '../../components/ui/card';
import { PageHeader } from '../../components/PageHeader';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { apiRequest, BootstrapResponse, getBootstrap } from '../../lib/api';

export function ProfilePage() {
  const client = useQueryClient();
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });
  const [values, setValues] = useState({ full_name: '', username: '', email: '' });
  useEffect(() => { if (bootstrap.data) setValues({ full_name: bootstrap.data.data.full_name, username: bootstrap.data.data.username, email: bootstrap.data.data.email }); }, [bootstrap.data]);
  const mutation = useMutation({
    mutationFn: () => apiRequest<{ data: BootstrapResponse['data'] }>('/api/v1/me', { method: 'PATCH', body: JSON.stringify(values) }),
    onSuccess: (result) => client.setQueryData<BootstrapResponse>(['bootstrap'], (old) => old ? { ...old, data: { ...old.data, ...result.data } } : old),
  });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };

  return <div className="max-w-2xl space-y-8"><PageHeader title="Profil saya" description="Identitas ini digunakan pada aktivitas dan dokumen sistem." />
    {bootstrap.data ? <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground" aria-label="Informasi akun"><StatusBadge active={bootstrap.data.data.is_active !== false} /><span><strong className="font-medium text-foreground">Role:</strong> {bootstrap.data.data.roles.join(', ') || '-'}</span><span><strong className="font-medium text-foreground">Login terakhir:</strong> {bootstrap.data.data.last_login_at ? new Date(bootstrap.data.data.last_login_at).toLocaleString('id-ID') : 'Belum tersedia'}</span></div> : null}
    <Card className="gap-0 py-0 shadow-none"><CardContent className="p-5 sm:p-6"><form className="grid gap-5" onSubmit={submit}>
      <FormField label="Nama lengkap" name="full_name" required hint="Nama ini tampil pada aktivitas dan dokumen." value={values.full_name} onChange={(event) => setValues({ ...values, full_name: event.target.value })} />
      <FormField label="Username" name="username" required hint="Gunakan username yang mudah dikenali tim internal." value={values.username} onChange={(event) => setValues({ ...values, username: event.target.value })} />
      <FormField label="Email" name="email" type="email" required hint="Digunakan untuk pemberitahuan akun bila tersedia." value={values.email} onChange={(event) => setValues({ ...values, email: event.target.value })} />
      {mutation.isError ? <Alert variant="destructive"><AlertDescription>Perubahan belum dapat disimpan.</AlertDescription></Alert> : null}
      {mutation.isSuccess ? <Alert><AlertDescription>Profil berhasil diperbarui.</AlertDescription></Alert> : null}
      <div><Button disabled={mutation.isPending} type="submit" className="w-full sm:w-auto">Simpan perubahan</Button></div>
    </form></CardContent></Card>
  </div>;
}
