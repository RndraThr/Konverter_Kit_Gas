import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '../../components/ui/dropdown-menu';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { PermissionGroup, RegencyOption, RoleDialog, RoleRecord, RoleValues } from './RoleDialog';

export function RolesPage() {
  const client = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<RoleRecord>();
  const [actionsOpen, setActionsOpen] = useState<string>();
  const [pendingDelete, setPendingDelete] = useState<RoleRecord>();
  const canManage = useCan('roles.manage');
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => apiRequest<{ data: RoleRecord[] }>('/api/v1/admin/roles') });
  const permissions = useQuery({ queryKey: ['permissions'], queryFn: () => apiRequest<{ data: PermissionGroup[] }>('/api/v1/admin/permissions'), enabled: canManage });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies'), enabled: canManage });
  const save = useMutation({ mutationFn: (values: RoleValues) => apiRequest(selected ? `/api/v1/admin/roles/${selected.id}` : '/api/v1/admin/roles', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['roles'] }); toast.success('Role berhasil disimpan.'); } });
  const remove = useMutation({ mutationFn: (id: string) => apiRequest(`/api/v1/admin/roles/${id}`, { method: 'DELETE' }), onSuccess: () => { client.invalidateQueries({ queryKey: ['roles'] }); toast.success('Role berhasil dihapus.'); } });
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  return <div className="space-y-5"><PageHeader title="Role & akses" description="Susun hak akses berdasarkan tanggung jawab pengguna." actions={canManage ? <Button onClick={() => { save.reset(); setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah role</Button> : undefined} />
    {roles.isError ? <DataState kind="error" title="Role belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => roles.refetch() }} /> : roles.isPending ? <DataState kind="loading" title="Memuat role" description="Menyiapkan daftar hak akses." /> : <DataTable label="Daftar role" minimumWidth={760}><thead><tr><th>Role</th><th>Hak akses</th><th>Pengguna</th><th>Jenis</th>{canManage && <th className="text-right">Aksi</th>}</tr></thead><tbody>{roles.data?.data.map((role) => <tr key={role.id}><td><strong>{role.name}</strong><br /><span className="text-xs text-muted-foreground">{role.code}</span></td><td>{role.permissions.length} permission</td><td>{role.user_count}</td><td><Badge variant={role.is_system ? 'secondary' : 'outline'}>{role.is_system ? 'Role sistem' : 'Role kustom'}</Badge></td>{canManage && <td className="text-right"><DropdownMenu open={actionsOpen === role.id} onOpenChange={(open) => setActionsOpen(open ? role.id : undefined)}><DropdownMenuTrigger onClick={() => setActionsOpen(role.id)} render={<Button variant="ghost" size="icon" aria-label={`Aksi ${role.name}`} />}><MoreHorizontal /></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-48"><DropdownMenuItem disabled={role.is_system} onClick={() => { save.reset(); setSelected(role); setDialogOpen(true); }}><Pencil />Edit {role.name}</DropdownMenuItem><DropdownMenuItem disabled={role.is_system || role.user_count > 0} variant="destructive" onClick={() => setPendingDelete(role)}><Trash2 />Hapus {role.name}</DropdownMenuItem></DropdownMenuContent></DropdownMenu></td>}</tr>)}</tbody></DataTable>}
    {remove.isError && <p className="text-sm text-destructive" role="alert">{(remove.error as ApiError).message}</p>}
    {canManage && <RoleDialog error={saveMessage} fields={saveFields} open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} groups={permissions.data?.data ?? []} regencies={regencies.data?.data ?? []} role={selected} pending={save.isPending} onSave={(values) => save.mutate(values)} />}
    <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => { if (!open) setPendingDelete(undefined); }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Hapus role {pendingDelete?.name}?</AlertDialogTitle><AlertDialogDescription>Role ini akan dihapus secara permanen dan tidak dapat dipulihkan.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingDelete) remove.mutate(pendingDelete.id); setPendingDelete(undefined); }}>Hapus role</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>;
}
