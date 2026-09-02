import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { FormField } from '../../components/FormField';
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
  return <div className="page"><header className="pageHeader"><div><h1>Profil saya</h1><p>Identitas ini digunakan pada aktivitas dan dokumen sistem.</p></div></header>
    {bootstrap.data && <div className="toolbar" aria-label="Informasi akun"><span><strong>Status:</strong> {bootstrap.data.data.is_active === false ? 'Nonaktif' : 'Aktif'}</span><span><strong>Role:</strong> {bootstrap.data.data.roles.join(', ') || '-'}</span><span><strong>Login terakhir:</strong> {bootstrap.data.data.last_login_at ? new Date(bootstrap.data.data.last_login_at).toLocaleString('id-ID') : 'Belum tersedia'}</span></div>}
    <form className="dialogBody" onSubmit={submit} style={{ maxWidth: 720, padding: 0 }}>
      <FormField className="fullField" label="Nama lengkap" name="full_name" required value={values.full_name} onChange={(e) => setValues({ ...values, full_name: e.target.value })} />
      <FormField label="Username" name="username" required value={values.username} onChange={(e) => setValues({ ...values, username: e.target.value })} />
      <FormField label="Email" name="email" type="email" required value={values.email} onChange={(e) => setValues({ ...values, email: e.target.value })} />
      {mutation.isError && <p className="formNotice">Perubahan belum dapat disimpan.</p>}
      {mutation.isSuccess && <span className="successNotice">Profil berhasil diperbarui.</span>}
      <div className="fullField"><button className="primaryButton" disabled={mutation.isPending} type="submit">Simpan perubahan</button></div>
    </form>
  </div>;
}
