import { FormEvent, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';

export function ChangePasswordForm() {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const mutation = useMutation({ mutationFn: () => apiRequest('/api/v1/me/password', { method: 'PUT', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) }), onSuccess: () => { setCurrentPassword(''); setNewPassword(''); } });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  return <div className="page"><header className="pageHeader"><div><h1>Ubah password</h1><p>Gunakan minimal 12 karakter yang tidak sama dengan password saat ini.</p></div></header>
    <form className="dialogBody" onSubmit={submit} style={{ maxWidth: 620, padding: 0 }}>
      <FormField className="fullField" label="Password saat ini" name="current_password" type="password" required value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} />
      <FormField className="fullField" label="Password baru" name="new_password" type="password" minLength={12} required value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
      {mutation.isError && <p className="formNotice">Password belum dapat diubah. Periksa password saat ini.</p>}
      {mutation.isSuccess && <span className="successNotice">Password berhasil diubah.</span>}
      <div className="fullField"><button className="primaryButton" disabled={mutation.isPending} type="submit">Ubah password</button></div>
    </form>
  </div>;
}
