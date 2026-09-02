import { Dialog } from '@base-ui/react/dialog';
import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { FormField } from '../../components/FormField';
import { UserRecord } from './UserDialog';

export function UserPasswordDialog({ open, onOpenChange, user, pending, onSave }: {
  open: boolean; onOpenChange: (open: boolean) => void; user?: UserRecord; pending?: boolean; onSave: (password: string) => void;
}) {
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [mismatch, setMismatch] = useState(false);
  useEffect(() => { setPassword(''); setConfirmation(''); setMismatch(false); }, [open]);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const invalid = password !== confirmation;
    setMismatch(invalid);
    if (!invalid) onSave(password);
  };
  return <Dialog.Root open={open} onOpenChange={onOpenChange}>
    <Dialog.Portal><Dialog.Backdrop className="dialogBackdrop" /><Dialog.Popup className="dialogPopup" aria-label="Reset password pengguna">
      <form onSubmit={submit}>
        <header className="dialogHeader"><div><Dialog.Title>Reset password</Dialog.Title><Dialog.Description>Tetapkan password baru untuk {user?.full_name}.</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
        <div className="dialogBody">
          <FormField className="fullField" label="Password baru" name="password" type="password" minLength={12} required value={password} onChange={(e) => setPassword(e.target.value)} />
          <FormField className="fullField" label="Konfirmasi password" name="password_confirmation" type="password" minLength={12} required value={confirmation} onChange={(e) => { setConfirmation(e.target.value); setMismatch(false); }} />
          {mismatch && <p className="formNotice">Konfirmasi password tidak sama.</p>}
        </div>
        <footer className="dialogActions"><Dialog.Close className="secondaryButton">Batal</Dialog.Close><button className="primaryButton" disabled={pending} type="submit">Simpan password</button></footer>
      </form>
    </Dialog.Popup></Dialog.Portal>
  </Dialog.Root>;
}
