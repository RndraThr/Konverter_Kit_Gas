import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { KeyRound, Pencil, Plus, Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DataTable } from '../../components/DataTable';
import { StatusBadge } from '../../components/StatusBadge';
import { apiRequest } from '../../lib/api';
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
  const [selected, setSelected] = useState<UserRecord>();
  const canManage = useCan('users.manage');
  const query = useQuery({ queryKey: ['users', params.toString()], queryFn: () => apiRequest<{ data: UserPage }>(`/api/v1/admin/users?${params.toString()}`) });
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => apiRequest<{ data: Role[] }>('/api/v1/admin/roles') });
  const save = useMutation({ mutationFn: (values: UserValues) => apiRequest(selected ? `/api/v1/admin/users/${selected.id}` : '/api/v1/admin/users', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['users'] }); } });
  const resetPassword = useMutation({ mutationFn: (password: string) => apiRequest(`/api/v1/admin/users/${selected?.id}/password`, { method: 'PUT', body: JSON.stringify({ password }) }), onSuccess: () => { setPasswordOpen(false); setSelected(undefined); } });
  const submitSearch = (event: FormEvent) => { event.preventDefault(); const next = new URLSearchParams(params); search ? next.set('search', search) : next.delete('search'); next.set('page', '1'); setParams(next); };
  const openEdit = (user: UserRecord) => { setSelected(user); setDialogOpen(true); };
  const setFilter = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const page = query.data?.data.page ?? 1;
  const pageSize = query.data?.data.page_size ?? 20;
  const total = query.data?.data.total ?? 0;
  return <div className="page">
    <header className="pageHeader"><div><h1>Pengguna</h1><p>Kelola akun internal dan role untuk akses operasional.</p></div>{canManage && <button className="primaryButton" onClick={() => { setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah pengguna</button>}</header>
    <form className="toolbar" onSubmit={submitSearch}><div className="searchBox"><Search /><input aria-label="Cari pengguna" placeholder="Cari nama, username, atau email" value={search} onChange={(e) => setSearch(e.target.value)} /></div><select className="selectField" aria-label="Filter status" value={params.get('active') ?? ''} onChange={(e) => setFilter('active', e.target.value)}><option value="">Semua status</option><option value="true">Aktif</option><option value="false">Nonaktif</option></select><select className="selectField" aria-label="Filter role" value={params.get('role') ?? ''} onChange={(e) => setFilter('role', e.target.value)}><option value="">Semua role</option>{roles.data?.data.map((role) => <option key={role.id} value={role.code}>{role.name}</option>)}</select><button className="secondaryButton">Cari</button></form>
    {query.isError ? <div className="errorState">Data pengguna belum dapat dimuat.</div> : query.data?.data.items.length === 0 ? <div className="emptyState">Belum ada pengguna yang sesuai.</div> : <DataTable label="Daftar pengguna"><thead><tr><th>Nama</th><th>Username</th><th>Role</th><th>Status</th>{canManage && <th>Aksi</th>}</tr></thead><tbody>{query.data?.data.items.map((user) => <tr key={user.id}><td><strong>{user.full_name}</strong><br /><small>{user.email}</small></td><td>{user.username}</td><td>{user.roles.map((role) => role.name).join(', ') || '-'}</td><td><StatusBadge active={user.is_active} /></td>{canManage && <td style={{ display: 'flex', gap: 7 }}><button className="iconButton" aria-label={`Edit ${user.full_name}`} onClick={() => openEdit(user)}><Pencil /></button><button className="iconButton" aria-label={`Reset password ${user.full_name}`} onClick={() => { setSelected(user); setPasswordOpen(true); }}><KeyRound /></button></td>}</tr>)}</tbody></DataTable>}
    <div className="pagination"><span>{total} pengguna</span><span><button className="secondaryButton" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button> Halaman {page} <button className="secondaryButton" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</button></span></div>
    {canManage && <UserDialog open={dialogOpen} onOpenChange={setDialogOpen} roles={roles.data?.data ?? []} user={selected} pending={save.isPending} onSave={(values) => { if (selected?.is_active && !values.is_active && !window.confirm(`Nonaktifkan ${selected.full_name}?`)) return; save.mutate(values); }} />}
    {canManage && <UserPasswordDialog open={passwordOpen} onOpenChange={setPasswordOpen} user={selected} pending={resetPassword.isPending} onSave={(password) => resetPassword.mutate(password)} />}
  </div>;
}
