import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { DataTable } from '../../components/DataTable';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { PermissionGroup, RoleDialog, RoleRecord, RoleValues } from './RoleDialog';

export function RolesPage() {
  const client = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<RoleRecord>();
  const canManage = useCan('roles.manage');
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => apiRequest<{ data: RoleRecord[] }>('/api/v1/admin/roles') });
  const permissions = useQuery({ queryKey: ['permissions'], queryFn: () => apiRequest<{ data: PermissionGroup[] }>('/api/v1/admin/permissions'), enabled: canManage });
  const save = useMutation({ mutationFn: (values: RoleValues) => apiRequest(selected ? `/api/v1/admin/roles/${selected.id}` : '/api/v1/admin/roles', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['roles'] }); } });
  const remove = useMutation({ mutationFn: (id: string) => apiRequest(`/api/v1/admin/roles/${id}`, { method: 'DELETE' }), onSuccess: () => client.invalidateQueries({ queryKey: ['roles'] }) });
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  return <div className="page"><header className="pageHeader"><div><h1>Role & akses</h1><p>Susun hak akses berdasarkan tanggung jawab pengguna.</p></div>{canManage && <button className="primaryButton" onClick={() => { save.reset(); setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah role</button>}</header>
    {roles.isError ? <div className="errorState">Role belum dapat dimuat.</div> : <DataTable label="Daftar role"><thead><tr><th>Role</th><th>Hak akses</th><th>Pengguna</th><th>Jenis</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{roles.data?.data.map((role) => <tr key={role.id}><td><strong>{role.name}</strong><br /><small>{role.code}</small></td><td>{role.permissions.length} permission</td><td>{role.user_count}</td><td>{role.is_system ? 'Role sistem' : 'Role kustom'}</td>{canManage && <td style={{ display: 'flex', gap: 7, alignItems: 'center' }}><button className="iconButton" aria-label={`Edit ${role.name}`} disabled={role.is_system} onClick={() => { save.reset(); setSelected(role); setDialogOpen(true); }}><Pencil /></button><button className="iconButton" aria-label={`Hapus ${role.name}`} disabled={role.is_system || role.user_count > 0} onClick={() => window.confirm(`Hapus role ${role.name}?`) && remove.mutate(role.id)}><Trash2 /></button></td>}</tr>)}</tbody></DataTable>}
    {remove.isError && <p className="formNotice">{(remove.error as ApiError).message}</p>}
    {canManage && <RoleDialog error={saveMessage} fields={saveFields} open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} groups={permissions.data?.data ?? []} role={selected} pending={save.isPending} onSave={(values) => save.mutate(values)} />}
  </div>;
}
