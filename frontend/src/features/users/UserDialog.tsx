import { Dialog } from '@base-ui/react/dialog';
import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { FormField } from '../../components/FormField';

export type Role = { id: string; code: string; name: string };
export type UserRecord = { id: string; full_name: string; username: string; email: string; is_active: boolean; roles: Role[] };
export type UserValues = { full_name: string; username: string; email: string; password?: string; is_active: boolean; role_ids: string[] };

export function UserDialog({ open, onOpenChange, roles, user, pending, onSave }: {
  open: boolean; onOpenChange: (open: boolean) => void; roles: Role[]; user?: UserRecord; pending?: boolean; onSave: (values: UserValues) => void;
}) {
  const empty = { full_name: '', username: '', email: '', password: '', is_active: true, role_ids: [] as string[] };
  const [values, setValues] = useState<UserValues>(empty);
  useEffect(() => setValues(user ? { full_name: user.full_name, username: user.username, email: user.email, is_active: user.is_active, role_ids: user.roles.map((role) => role.id) } : empty), [user, open]);
  const toggleRole = (id: string) => setValues({ ...values, role_ids: values.role_ids.includes(id) ? values.role_ids.filter((value) => value !== id) : [...values.role_ids, id] });
  const submit = (event: FormEvent) => { event.preventDefault(); onSave(values); };
  const title = user ? `Edit ${user.full_name}` : 'Tambah pengguna';
  return <Dialog.Root open={open} onOpenChange={onOpenChange}>
    <Dialog.Portal><Dialog.Backdrop className="dialogBackdrop" /><Dialog.Popup className="dialogPopup" aria-label={title}>
      <form onSubmit={submit}>
        <header className="dialogHeader"><div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>Atur identitas dan akses pengguna.</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
        <div className="dialogBody">
          <FormField className="fullField" label="Nama lengkap" name="full_name" required value={values.full_name} onChange={(e) => setValues({ ...values, full_name: e.target.value })} />
          <FormField label="Username" name="username" required value={values.username} onChange={(e) => setValues({ ...values, username: e.target.value })} />
          <FormField label="Email" name="email" type="email" required value={values.email} onChange={(e) => setValues({ ...values, email: e.target.value })} />
          {!user && <FormField className="fullField" label="Password awal" name="password" type="password" minLength={12} required value={values.password} onChange={(e) => setValues({ ...values, password: e.target.value })} hint="Minimal 12 karakter" />}
          <fieldset className="fullField" style={{ border: 0, padding: 0, margin: 0 }}><legend style={{ fontSize: 13, fontWeight: 700, marginBottom: 9 }}>Role</legend>{roles.map((role) => <label className="checkboxField" key={role.id}><input type="checkbox" checked={values.role_ids.includes(role.id)} onChange={() => toggleRole(role.id)} />{role.name}</label>)}</fieldset>
          <label className="checkboxField fullField"><input type="checkbox" checked={values.is_active} onChange={(e) => setValues({ ...values, is_active: e.target.checked })} />Pengguna aktif</label>
        </div>
        <footer className="dialogActions"><Dialog.Close className="secondaryButton">Batal</Dialog.Close><button className="primaryButton" disabled={pending} type="submit">Simpan pengguna</button></footer>
      </form>
    </Dialog.Popup></Dialog.Portal>
  </Dialog.Root>;
}
