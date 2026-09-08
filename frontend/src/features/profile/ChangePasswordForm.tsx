import { FormEvent, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { Card, CardContent } from '../../components/ui/card';
import { FormField } from '../../components/FormField';
import { PageHeader } from '../../components/PageHeader';
import { apiRequest } from '../../lib/api';

export function ChangePasswordForm() {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [mismatch, setMismatch] = useState(false);
  const mutation = useMutation({ mutationFn: () => apiRequest('/api/v1/me/password', { method: 'PUT', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) }), onSuccess: () => { setCurrentPassword(''); setNewPassword(''); setConfirmation(''); } });
  const submit = (event: FormEvent) => { event.preventDefault(); const invalid = confirmation !== newPassword; setMismatch(invalid); if (!invalid) mutation.mutate(); };

  return <div className="max-w-2xl space-y-8"><PageHeader title="Ubah password" description="Gunakan minimal 12 karakter yang tidak sama dengan password saat ini." />
    <Card className="gap-0 py-0 shadow-none"><CardContent className="p-5 sm:p-6"><form className="grid gap-5" onSubmit={submit}>
      <FormField label="Password saat ini" name="current_password" type="password" required hint="Masukkan password yang sedang digunakan." value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} />
      <FormField label="Password baru" name="new_password" type="password" minLength={12} required hint="Panjang password minimal 12 karakter." value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
      <FormField label="Konfirmasi password baru" name="password_confirmation" type="password" minLength={12} required error={mismatch ? 'Konfirmasi password baru tidak sama.' : undefined} value={confirmation} onChange={(event) => { setConfirmation(event.target.value); setMismatch(false); }} />
      {mutation.isError ? <Alert variant="destructive"><AlertDescription>Password belum dapat diubah. Periksa password saat ini.</AlertDescription></Alert> : null}
      {mutation.isSuccess ? <Alert><AlertDescription>Password berhasil diubah.</AlertDescription></Alert> : null}
      <div><Button disabled={mutation.isPending} type="submit" className="w-full sm:w-auto">Ubah password</Button></div>
    </form></CardContent></Card>
  </div>;
}
