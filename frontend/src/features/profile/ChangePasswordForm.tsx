import { FormEvent, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { ShieldCheck } from 'lucide-react';
import { Alert, AlertDescription } from '../../components/ui/alert';
import { Button } from '../../components/ui/button';
import { FormField } from '../../components/FormField';
import { apiRequest } from '../../lib/api';

export function ChangePasswordForm() {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [mismatch, setMismatch] = useState(false);
  const mutation = useMutation({ mutationFn: () => apiRequest('/api/v1/me/password', { method: 'PUT', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) }), onSuccess: () => { setCurrentPassword(''); setNewPassword(''); setConfirmation(''); } });
  const submit = (event: FormEvent) => { event.preventDefault(); const invalid = confirmation !== newPassword; setMismatch(invalid); if (!invalid) mutation.mutate(); };

  return <section className="min-w-0 border-t bg-muted/10 p-5 lg:border-t-0 lg:border-l sm:p-6" aria-labelledby="account-security-title">
    <header className="mb-6 flex items-start gap-3">
      <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-secondary text-secondary-foreground"><ShieldCheck className="size-4.5" aria-hidden="true" /></span>
      <div><h2 id="account-security-title" className="text-base font-semibold">Keamanan akun</h2><p className="mt-1 text-sm leading-6 text-muted-foreground">Gunakan minimal 12 karakter yang berbeda dari password saat ini.</p></div>
    </header>
    <form className="grid gap-5" onSubmit={submit}>
      <FormField label="Password saat ini" name="current_password" type="password" required hint="Masukkan password yang sedang digunakan." value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} />
      <FormField label="Password baru" name="new_password" type="password" minLength={12} required hint="Panjang password minimal 12 karakter." value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
      <FormField label="Konfirmasi password baru" name="password_confirmation" type="password" minLength={12} required error={mismatch ? 'Konfirmasi password baru tidak sama.' : undefined} value={confirmation} onChange={(event) => { setConfirmation(event.target.value); setMismatch(false); }} />
      {mutation.isError ? <Alert variant="destructive"><AlertDescription>Password belum dapat diubah. Periksa password saat ini.</AlertDescription></Alert> : null}
      {mutation.isSuccess ? <Alert><AlertDescription>Password berhasil diubah.</AlertDescription></Alert> : null}
      <div className="border-t pt-5"><Button disabled={mutation.isPending} type="submit" className="w-full sm:w-auto">Ubah password</Button></div>
    </form>
  </section>;
}
