import { useQuery } from '@tanstack/react-query';
import { Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { apiRequest } from '../../lib/api';

type Entry = { id: string; actor_name?: string; action: string; resource_type: string; resource_id?: string; metadata: Record<string, unknown>; created_at: string };
type AuditPageData = { items: Entry[]; page: number; page_size: number; total: number };

export function AuditPage() {
  const [params, setParams] = useSearchParams(); const [action, setAction] = useState(params.get('action') ?? ''); const [resourceType, setResourceType] = useState(params.get('resource_type') ?? ''); const [actorUserID, setActorUserID] = useState(params.get('actor_user_id') ?? '');
  const query = useQuery({ queryKey: ['audit', params.toString()], queryFn: () => apiRequest<{ data: AuditPageData }>(`/api/v1/system/audit-logs?${params.toString()}`) });
  const filter = (event: FormEvent) => { event.preventDefault(); const next = new URLSearchParams(params); for (const [key, value] of [['action', action], ['resource_type', resourceType], ['actor_user_id', actorUserID]]) value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const page = query.data?.data.page ?? 1; const pageSize = query.data?.data.page_size ?? 20; const total = query.data?.data.total ?? 0;
  return <div className="space-y-5"><PageHeader title="Riwayat aktivitas" description="Catatan perubahan penting yang bersifat read-only." />
    <form className="grid gap-3 rounded-lg border bg-card p-3 md:grid-cols-[minmax(0,1.5fr)_1fr_1fr_auto]" onSubmit={filter}><div className="relative"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 text-muted-foreground" /><Input aria-label="Filter aksi" className="pl-10" placeholder="Contoh: settings.updated" value={action} onChange={(event) => setAction(event.target.value)} /></div><Input aria-label="Filter jenis objek" placeholder="Jenis objek" value={resourceType} onChange={(event) => setResourceType(event.target.value)} /><Input aria-label="Filter ID pelaku" placeholder="ID pelaku" value={actorUserID} onChange={(event) => setActorUserID(event.target.value)} /><Button type="submit" variant="outline">Terapkan filter</Button></form>
    {query.isError ? <DataState kind="error" title="Riwayat aktivitas belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => query.refetch() }} /> : query.isPending ? <DataState kind="loading" title="Memuat riwayat aktivitas" description="Mengambil catatan perubahan." /> : query.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada aktivitas yang sesuai" description="Ubah filter untuk melihat catatan lain." /> : <DataTable label="Riwayat aktivitas" minimumWidth={920}><thead><tr><th>Waktu</th><th>Pelaku</th><th>Aksi</th><th>Objek</th><th>Detail</th></tr></thead><tbody>{query.data?.data.items.map((entry) => <tr key={entry.id}><td className="whitespace-nowrap tabular-nums">{new Date(entry.created_at).toLocaleString('id-ID')}</td><td>{entry.actor_name || 'Sistem'}</td><td><strong>{entry.action}</strong></td><td><Badge variant="outline">{entry.resource_type}{entry.resource_id ? ` / ${entry.resource_id}` : ''}</Badge></td><td><details><summary className="cursor-pointer text-primary underline-offset-4 hover:underline">Lihat</summary><pre className="mt-2 max-w-80 overflow-x-auto rounded bg-muted p-2 text-xs whitespace-pre-wrap">{JSON.stringify(entry.metadata, null, 2)}</pre></details></td></tr>)}</tbody></DataTable>}
    <nav className="flex flex-col gap-3 border-t pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between" aria-label="Pagination riwayat aktivitas"><span>{total} aktivitas</span><span className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</Button>Halaman {page}<Button size="sm" variant="outline" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</Button></span></nav>
  </div>;
}
