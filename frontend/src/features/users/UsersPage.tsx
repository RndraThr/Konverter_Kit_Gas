import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Plus, Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DataTable } from '../../components/DataTable';
import { StatusBadge } from '../../components/StatusBadge';
import { apiRequest } from '../../lib/api';
import { Role, UserDialog, UserRecord, UserValues } from './UserDialog';

type UserPage = { items: UserRecord[]; page: number; page_size: number; total: number };

export function UsersPage() {
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get('search') ?? '');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<UserRecord>();
  const query = useQuery({ queryKey: ['users', params.toString()], queryFn: () => apiRequest<{ data: UserPage }>(`/api/v1/admin/users?${params.toString()}`) });
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => apiRequest<{ data: Role[] }>('/api/v1/admin/roles') });
  const save = useMutation({ mutationFn: (values: UserValues) => apiRequest(selected ? `/api/v1/admin/users/${selected.id}` : '/api/v1/admin/users', { method: selected ? 'PATCH' : 'POST', body: JSON.stringify(values) }), onSuccess: () => { setDialogOpen(false); setSelected(undefined); client.invalidateQueries({ queryKey: ['users'] }); } });
  const submitSearch = (event: FormEvent) => { event.preventDefault(); const next = new URLSearchParams(params); search ? next.set('search', search) : next.delete('search'); next.set('page', '1'); setParams(next); };
  const openEdit = (user: UserRecord) => { setSelected(user); setDialogOpen(true); };
  return <div className="page">
    <header className="pageHeader"><div><h1>Pengguna</h1><p>Kelola akun internal dan role untuk akses operasional.</p></div><button className="primaryButton" onClick={() => { setSelected(undefined); setDialogOpen(true); }}><Plus />Tambah pengguna</button></header>
    <form className="toolbar" onSubmit={submitSearch}><div className="searchBox"><Search /><input aria-label="Cari pengguna" placeholder="Cari nama, username, atau email" value={search} onChange={(e) => setSearch(e.target.value)} /></div><button className="secondaryButton">Cari</button></form>
    {query.isError ? <div className="errorState">Data pengguna belum dapat dimuat.</div> : query.data?.data.items.length === 0 ? <div className="emptyState">Belum ada pengguna yang sesuai.</div> : <DataTable label="Daftar pengguna"><thead><tr><th>Nama</th><th>Username</th><th>Role</th><th>Status</th><th>Aksi</th></tr></thead><tbody>{query.data?.data.items.map((user) => <tr key={user.id}><td><strong>{user.full_name}</strong><br /><small>{user.email}</small></td><td>{user.username}</td><td>{user.roles.map((role) => role.name).join(', ') || '-'}</td><td><StatusBadge active={user.is_active} /></td><td><button className="iconButton" aria-label={`Edit ${user.full_name}`} onClick={() => openEdit(user)}><Pencil /></button></td></tr>)}</tbody></DataTable>}
    <div className="pagination"><span>{query.data?.data.total ?? 0} pengguna</span><span>Halaman {query.data?.data.page ?? 1}</span></div>
    <UserDialog open={dialogOpen} onOpenChange={setDialogOpen} roles={roles.data?.data ?? []} user={selected} pending={save.isPending} onSave={(values) => save.mutate(values)} />
  </div>;
}
