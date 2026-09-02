import { useQuery } from '@tanstack/react-query';
import { Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DataTable } from '../../components/DataTable';
import { apiRequest } from '../../lib/api';

type Entry = { id: string; actor_name?: string; action: string; resource_type: string; resource_id?: string; metadata: Record<string, unknown>; created_at: string };
type AuditPageData = { items: Entry[]; page: number; page_size: number; total: number };

export function AuditPage() {
  const [params, setParams] = useSearchParams();
  const [action, setAction] = useState(params.get('action') ?? '');
  const [resourceType, setResourceType] = useState(params.get('resource_type') ?? '');
  const [actorUserID, setActorUserID] = useState(params.get('actor_user_id') ?? '');
  const query = useQuery({ queryKey: ['audit', params.toString()], queryFn: () => apiRequest<{ data: AuditPageData }>(`/api/v1/system/audit-logs?${params.toString()}`) });
  const filter = (event: FormEvent) => { event.preventDefault(); const next = new URLSearchParams(params); for (const [key, value] of [['action', action], ['resource_type', resourceType], ['actor_user_id', actorUserID]]) value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const page = query.data?.data.page ?? 1;
  const pageSize = query.data?.data.page_size ?? 20;
  const total = query.data?.data.total ?? 0;
  return <div className="page"><header className="pageHeader"><div><h1>Riwayat aktivitas</h1><p>Catatan perubahan penting yang bersifat read-only.</p></div></header>
    <form className="toolbar" onSubmit={filter}><div className="searchBox"><Search /><input aria-label="Filter aksi" placeholder="Contoh: settings.updated" value={action} onChange={(e) => setAction(e.target.value)} /></div><input className="selectField" aria-label="Filter jenis objek" placeholder="Jenis objek" value={resourceType} onChange={(e) => setResourceType(e.target.value)} /><input className="selectField" aria-label="Filter ID pelaku" placeholder="ID pelaku" value={actorUserID} onChange={(e) => setActorUserID(e.target.value)} /><button className="secondaryButton">Terapkan filter</button></form>
    {query.isError ? <div className="errorState">Riwayat aktivitas belum dapat dimuat.</div> : query.data?.data.items.length === 0 ? <div className="emptyState">Belum ada aktivitas yang sesuai.</div> : <DataTable label="Riwayat aktivitas"><thead><tr><th>Waktu</th><th>Pelaku</th><th>Aksi</th><th>Objek</th><th>Detail</th></tr></thead><tbody>{query.data?.data.items.map((entry) => <tr key={entry.id}><td>{new Date(entry.created_at).toLocaleString('id-ID')}</td><td>{entry.actor_name || 'Sistem'}</td><td><strong>{entry.action}</strong></td><td>{entry.resource_type}{entry.resource_id ? ` / ${entry.resource_id}` : ''}</td><td><details><summary>Lihat</summary><pre style={{ whiteSpace: 'pre-wrap', maxWidth: 320 }}>{JSON.stringify(entry.metadata, null, 2)}</pre></details></td></tr>)}</tbody></DataTable>}
    <div className="pagination"><span>{total} aktivitas</span><span><button className="secondaryButton" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button> Halaman {page} <button className="secondaryButton" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</button></span></div>
  </div>;
}
