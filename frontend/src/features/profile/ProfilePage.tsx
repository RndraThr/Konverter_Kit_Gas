import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Clock3, Mail, UserRound } from 'lucide-react';
import { FormEvent, useEffect, useState } from 'react';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent, CardHeader } from '../../components/ui/card';
import { PageHeader } from '../../components/PageHeader';
import { FormField } from '../../components/FormField';
import { StatusBadge } from '../../components/StatusBadge';
import { apiRequest, BootstrapResponse, getBootstrap } from '../../lib/api';
import { ChangePasswordForm } from './ChangePasswordForm';

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
  const account = bootstrap.data?.data;

  return <div className="mx-auto max-w-6xl space-y-6">
    <PageHeader title="Profil saya" description="Kelola identitas dan keamanan akun Anda dalam satu tempat." />

    <Card className="gap-0 overflow-hidden py-0 shadow-none">
      <CardHeader className="bg-muted/20 px-5 py-5 sm:px-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-14 shrink-0 place-items-center rounded-xl bg-primary text-xl font-bold text-primary-foreground" aria-hidden="true">{account?.full_name.slice(0, 1).toUpperCase() || <UserRound className="size-6" />}</span>
            <div className="min-w-0">
              <h2 className="truncate text-lg font-semibold">{account?.full_name || 'Memuat profil'}</h2>
              <p className="mt-1 flex items-center gap-2 truncate text-sm text-muted-foreground"><Mail className="size-4 shrink-0" aria-hidden="true" />{account?.email || '-'}</p>
            </div>
          </div>
          {account ? <div className="flex flex-wrap items-center gap-2" aria-label="Informasi akun">
            <StatusBadge active={account.is_active !== false} />
            {account.roles.map((role) => <Badge variant="outline" key={role}>{role}</Badge>)}
            <span className="flex items-center gap-2 text-xs text-muted-foreground"><Clock3 className="size-3.5" aria-hidden="true" />{account.last_login_at ? `Login ${new Date(account.last_login_at).toLocaleString('id-ID')}` : 'Login terakhir belum tersedia'}</span>
          </div> : null}
        </div>
      </CardHeader>

      <CardContent className="grid p-0 lg:grid-cols-2">
        <section className="min-w-0 p-5 sm:p-6" aria-labelledby="profile-information-title">
          <header className="mb-6">
            <h2 id="profile-information-title" className="text-base font-semibold">Informasi profil</h2>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">Identitas ini digunakan pada aktivitas dan dokumen sistem.</p>
          </header>
          <form className="grid gap-5" onSubmit={submit}>
            <FormField label="Nama lengkap" name="full_name" required hint="Nama ini tampil pada aktivitas dan dokumen." value={values.full_name} onChange={(event) => setValues({ ...values, full_name: event.target.value })} />
            <FormField label="Username" name="username" required hint="Gunakan username yang mudah dikenali tim internal." value={values.username} onChange={(event) => setValues({ ...values, username: event.target.value })} />
            <FormField label="Email" name="email" type="email" required hint="Digunakan untuk pemberitahuan akun bila tersedia." value={values.email} onChange={(event) => setValues({ ...values, email: event.target.value })} />
            {mutation.isError ? <Alert variant="destructive"><AlertDescription>Perubahan belum dapat disimpan.</AlertDescription></Alert> : null}
            {mutation.isSuccess ? <Alert><AlertDescription>Profil berhasil diperbarui.</AlertDescription></Alert> : null}
            <div className="border-t pt-5"><Button disabled={mutation.isPending} type="submit" className="w-full sm:w-auto">Simpan perubahan</Button></div>
          </form>
        </section>

        <ChangePasswordForm />
      </CardContent>
    </Card>
  </div>;
}
