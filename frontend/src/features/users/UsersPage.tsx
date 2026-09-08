import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { KeyRound, MoreHorizontal, Pencil, Plus, Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { StatusBadge } from '../../components/StatusBadge';
import { Button } from '../../components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '../../components/ui/dropdown-menu';
import { Input } from '../../components/ui/input';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { Role, UserDialog, UserRecord, UserValues } from './UserDialog';
import { UserPasswordDialog } from './UserPasswordDialog';

type UserPage = { items: UserRecord[]; page: number; page_size: number; total: number };

export function UsersPage() {
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get('search') ?? '');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [actionsOpen, setActionsOpen] = useState<string>();
  const [selected, setSelected] = useState<UserRecord>();
  const canManage = useCan('users.manage');
  const query = useQuery({ queryKey: ['users', params.toString()], queryFn: () => apiRequest<{ data: UserPage }>(`/api/v1/admin/users?${params.toString()}`) });
  const roles = useQuery({ queryKey: ['role-options'], queryFn: () => apiRequest<{ data: Role[] }>('/api/v1/admin/role-options') });
  const save = useMutation({ mutationFn: (values: UserValues) => apiRequest(selected ? `/api/v1/admin/users/${selected.id}` : '/api/v1/admin/users', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['users'] }); toast.success('Pengguna berhasil disimpan.'); } });
  const resetPassword = useMutation({ mutationFn: (password: string) => apiRequest(`/api/v1/admin/users/${selected?.id}/password`, { method: 'PUT', body: JSON.stringify({ password }) }), onSuccess: () => { setPasswordOpen(false); setSelected(undefined); toast.success('Password berhasil diperbarui.'); } });
  const submitSearch = (event: FormEvent) => { event.preventDefault(); const next = new URLSearchParams(params); search ? next.set('search', search) : next.delete('search'); next.set('page', '1'); setParams(next); };
  const openEdit = (user: UserRecord) => { save.reset(); setSelected(user); setDialogOpen(true); };
  const setFilter = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const page = query.data?.data.page ?? 1;
  const pageSize = query.data?.data.page_size ?? 20;
  const total = query.data?.data.total ?? 0;
  const saveError = save.error as ApiError | null;
  const passwordError = resetPassword.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const passwordFields = passwordError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  const passwordMessage = resetPassword.isError && Object.keys(passwordFields).length === 0 ? resetPassword.error.message : undefined;
  const openPassword = (user: UserRecord) => { resetPassword.reset(); setSelected(user); setPasswordOpen(true); };

  return <div className="space-y-5">
    <PageHeader title="Pengguna" description="Kelola akun internal dan role untuk akses operasional." actions={canManage ? <Button onClick={() => { save.reset(); setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah pengguna</Button> : undefined} />
    <form className="flex flex-col gap-3 rounded-lg border bg-card p-3 sm:flex-row sm:items-end" onSubmit={submitSearch}>
      <div className="relative min-w-0 flex-1"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 text-muted-foreground" /><Input aria-label="Cari pengguna" className="pl-10" placeholder="Cari nama, username, atau email" value={search} onChange={(event) => setSearch(event.target.value)} /></div>
      <select className="h-11 rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50" aria-label="Filter status" value={params.get('active') ?? ''} onChange={(event) => setFilter('active', event.target.value)}><option value="">Semua status</option><option value="true">Aktif</option><option value="false">Nonaktif</option></select>
      <select className="h-11 rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50" aria-label="Filter role" value={params.get('role') ?? ''} onChange={(event) => setFilter('role', event.target.value)}><option value="">Semua role</option>{roles.data?.data.map((role) => <option key={role.id} value={role.code}>{role.name}</option>)}</select>
      <Button type="submit" variant="outline">Cari</Button>
    </form>
    {query.isError ? <DataState kind="error" title="Data pengguna belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} /> : query.isPending ? <DataState kind="loading" title="Memuat pengguna" description="Menyiapkan daftar akun internal." /> : query.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada pengguna yang sesuai" description="Ubah filter atau tambahkan pengguna baru." /> : <DataTable label="Daftar pengguna" minimumWidth={760}><thead><tr><th>Nama</th><th>Username</th><th>Role</th><th>Status</th>{canManage && <th className="w-14 text-right">Aksi</th>}</tr></thead><tbody>{query.data?.data.items.map((user) => <tr key={user.id}><td><strong>{user.full_name}</strong><br /><span className="text-xs text-muted-foreground">{user.email}</span></td><td>{user.username}</td><td>{user.roles.map((role) => role.name).join(', ') || '-'}</td><td><StatusBadge active={user.is_active} /></td>{canManage && <td className="text-right"><DropdownMenu open={actionsOpen === user.id} onOpenChange={(open) => setActionsOpen(open ? user.id : undefined)}><DropdownMenuTrigger onClick={() => setActionsOpen(user.id)} render={<Button variant="ghost" size="icon" aria-label={`Aksi ${user.full_name}`} />}><MoreHorizontal /></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-48"><DropdownMenuItem onClick={() => openEdit(user)}><Pencil />Edit pengguna</DropdownMenuItem><DropdownMenuItem onClick={() => openPassword(user)}><KeyRound />Atur ulang password</DropdownMenuItem></DropdownMenuContent></DropdownMenu></td>}</tr>)}</tbody></DataTable>}
    <nav className="flex flex-col gap-3 border-t pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between" aria-label="Pagination pengguna"><span>{total} pengguna</span><span className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</Button>Halaman {page}<Button size="sm" variant="outline" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</Button></span></nav>
    {canManage && <UserDialog error={saveMessage} fields={saveFields} open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} roles={roles.data?.data ?? []} user={selected} pending={save.isPending} onSave={(values) => { if (selected?.is_active && !values.is_active && !window.confirm(`Nonaktifkan ${selected.full_name}?`)) return; save.mutate(values); }} />}
    {canManage && <UserPasswordDialog error={passwordMessage} fieldError={passwordFields.password} open={passwordOpen} onOpenChange={(open) => { setPasswordOpen(open); if (!open) resetPassword.reset(); }} user={selected} pending={resetPassword.isPending} onSave={(password) => resetPassword.mutate(password)} />}
  </div>;
}
