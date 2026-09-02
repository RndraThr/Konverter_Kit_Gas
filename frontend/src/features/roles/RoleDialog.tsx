import { Dialog } from '@base-ui/react/dialog';
import { FormEvent, useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { FormField } from '../../components/FormField';

export type Permission = { id: string; code: string; name: string };
export type PermissionGroup = { resource: string; permissions: Permission[] };
export type RoleRecord = { id: string; code: string; name: string; description?: string; is_system: boolean; permissions: Permission[]; user_count: number };
export type RoleValues = { code: string; name: string; description: string; permission_codes: string[] };

export function RoleDialog({ open, onOpenChange, groups, role, pending, error, fields = {}, onSave }: { open: boolean; onOpenChange: (value: boolean) => void; groups: PermissionGroup[]; role?: RoleRecord; pending?: boolean; error?: string; fields?: Record<string, string>; onSave: (values: RoleValues) => void }) {
  const empty = { code: '', name: '', description: '', permission_codes: [] as string[] };
  const [values, setValues] = useState<RoleValues>(empty);
  useEffect(() => setValues(role ? { code: role.code, name: role.name, description: role.description ?? '', permission_codes: role.permissions.map((item) => item.code) } : empty), [role, open]);
  const toggle = (code: string) => setValues({ ...values, permission_codes: values.permission_codes.includes(code) ? values.permission_codes.filter((item) => item !== code) : [...values.permission_codes, code] });
  const title = role ? `Edit ${role.name}` : 'Tambah role';
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Backdrop className="dialogBackdrop" /><Dialog.Popup className="dialogPopup" aria-label={title}><form onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(values); }}>
    <header className="dialogHeader"><div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>Pilih hak akses sesuai tanggung jawab.</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
    <div className="dialogBody"><FormField error={fields.code} label="Kode role" name="code" required disabled={Boolean(role)} value={values.code} onChange={(e) => setValues({ ...values, code: e.target.value })} /><FormField error={fields.name} label="Nama role" name="name" required value={values.name} onChange={(e) => setValues({ ...values, name: e.target.value })} /><FormField className="fullField" error={fields.description} label="Deskripsi" name="description" value={values.description} onChange={(e) => setValues({ ...values, description: e.target.value })} />
      {groups.map((group) => <fieldset className="fullField" style={{ border: 0, padding: 0, margin: 0 }} key={group.resource}><legend style={{ fontSize: 13, fontWeight: 750, marginBottom: 8 }}>{group.resource}</legend>{group.permissions.map((permission) => <label className="checkboxField" key={permission.code}><input type="checkbox" checked={values.permission_codes.includes(permission.code)} onChange={() => toggle(permission.code)} />{permission.name}</label>)}</fieldset>)}{fields.permission_codes && <small className="fieldError fullField">{fields.permission_codes}</small>}
      {error && <p className="formNotice fullField">{error}</p>}
    </div><footer className="dialogActions"><Dialog.Close className="secondaryButton">Batal</Dialog.Close><button className="primaryButton" disabled={pending}>Simpan role</button></footer>
  </form></Dialog.Popup></Dialog.Portal></Dialog.Root>;
}
