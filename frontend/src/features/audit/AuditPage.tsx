import { useQuery } from '@tanstack/react-query';
import { Activity, Bot, Search, X } from 'lucide-react';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { PageHeader } from '../../components/PageHeader';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { apiRequest } from '../../lib/api';
import { AuditDetailSheet } from './AuditDetailSheet';
import { AuditMobileCard } from './AuditMobileCard';
import { AuditEntry, presentActor, presentAuditAction, presentResource } from './auditPresentation';

type AuditPageData = {
  items: AuditEntry[];
  page: number;
  page_size: number;
  total: number;
  summary?: { today?: number; system?: number };
};

const filterLabels: Record<string, string> = {
  query: 'Pencarian',
  action: 'Aksi',
  resource_type: 'Objek',
  actor: 'Pelaku',
  actor_user_id: 'ID pelaku',
  date_from: 'Mulai',
  date_to: 'Sampai',
  page_size: 'Per halaman',
};

const filterKeys = Object.keys(filterLabels);

function SummaryCard({ icon: Icon, value, label }: { icon: typeof Activity; value: number; label: string }) {
  return <div className="flex min-w-0 items-center gap-3 rounded-lg border bg-card p-4">
    <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"><Icon className="size-5" aria-hidden="true" /></span>
    <p className="min-w-0 text-lg font-semibold tabular-nums">{value} {label}</p>
  </div>;
}

export function AuditPage() {
  const [params, setParams] = useSearchParams();
  const [queryText, setQueryText] = useState(params.get('query') ?? '');
  const [action, setAction] = useState(params.get('action') ?? '');
  const [resourceType, setResourceType] = useState(params.get('resource_type') ?? '');
  const [actor, setActor] = useState(params.get('actor') ?? '');
  const [actorUserID, setActorUserID] = useState(params.get('actor_user_id') ?? '');
  const [dateFrom, setDateFrom] = useState(params.get('date_from') ?? '');
  const [dateTo, setDateTo] = useState(params.get('date_to') ?? '');
  const [pageSizeDraft, setPageSizeDraft] = useState(params.get('page_size') ?? '20');
  const [selectedEntry, setSelectedEntry] = useState<AuditEntry | null>(null);

  useEffect(() => {
    setQueryText(params.get('query') ?? '');
    setAction(params.get('action') ?? '');
    setResourceType(params.get('resource_type') ?? '');
    setActor(params.get('actor') ?? '');
    setActorUserID(params.get('actor_user_id') ?? '');
    setDateFrom(params.get('date_from') ?? '');
    setDateTo(params.get('date_to') ?? '');
    setPageSizeDraft(params.get('page_size') ?? '20');
  }, [params]);

  const queryString = params.toString();
  const auditQuery = useQuery({
    queryKey: ['audit', queryString],
    queryFn: () => apiRequest<{ data: AuditPageData }>(`/api/v1/system/audit-logs?${queryString}`),
  });

  const applyFilters = (event: FormEvent) => {
    event.preventDefault();
    const next = new URLSearchParams(params);
    const values: Record<string, string> = {
      query: queryText,
      action,
      resource_type: resourceType,
      actor,
      actor_user_id: actorUserID,
      date_from: dateFrom,
      date_to: dateTo,
      page_size: pageSizeDraft,
    };
    for (const [key, value] of Object.entries(values)) value.trim() ? next.set(key, value.trim()) : next.delete(key);
    next.set('page', '1');
    setParams(next);
  };

  const removeFilter = (key: string) => {
    const next = new URLSearchParams(params);
    next.delete(key);
    next.set('page', '1');
    setParams(next);
  };

  const resetFilters = () => {
    const next = new URLSearchParams();
    next.set('page', '1');
    setParams(next);
  };

  const setPage = (nextPage: number) => {
    const next = new URLSearchParams(params);
    next.set('page', String(nextPage));
    setParams(next);
  };

  const activeFilters = useMemo(() => filterKeys.flatMap((key) => {
    const value = params.get(key);
    if (!value || (key === 'page_size' && value === '20')) return [];
    return [{ key, label: `${filterLabels[key]}: ${value}` }];
  }), [params]);

  const data = auditQuery.data?.data;
  const entries = data?.items ?? [];
  const page = data?.page ?? Number(params.get('page') ?? 1);
  const pageSize = data?.page_size ?? Number(params.get('page_size') ?? 20);
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const hasFilters = activeFilters.length > 0;

  return <div className="space-y-5">
    <PageHeader title="Riwayat aktivitas" description="Pantau perubahan penting dan telusuri detailnya tanpa mengubah data sumber." />

    <section className="grid gap-3 sm:grid-cols-3" aria-label="Ringkasan aktivitas">
      <SummaryCard icon={Activity} value={total} label="aktivitas" />
      <SummaryCard icon={Activity} value={data?.summary?.today ?? 0} label="hari ini" />
      <SummaryCard icon={Bot} value={data?.summary?.system ?? 0} label="oleh sistem" />
    </section>

    <form className="rounded-lg border bg-card p-4" onSubmit={applyFilters}>
      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative min-w-0 flex-1">
          <Search aria-hidden="true" className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Cari aktivitas" className="pl-9" placeholder="Cari aksi, objek, atau pelaku" value={queryText} onChange={(event) => setQueryText(event.target.value)} />
        </div>
        <Button type="submit">Terapkan</Button>
      </div>

      <details className="mt-3 border-t pt-3">
        <summary className="cursor-pointer text-sm font-medium text-primary focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50">Filter lanjutan</summary>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <label className="grid gap-1.5 text-sm font-medium">Aksi<Input aria-label="Filter aksi" placeholder="Contoh: auth.login" value={action} onChange={(event) => setAction(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">Objek<Input aria-label="Filter jenis objek" placeholder="Jenis objek" value={resourceType} onChange={(event) => setResourceType(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">Pelaku<Input aria-label="Filter pelaku" placeholder="Nama pelaku" value={actor} onChange={(event) => setActor(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">ID pelaku<Input aria-label="Filter ID pelaku" placeholder="ID pengguna" value={actorUserID} onChange={(event) => setActorUserID(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">Tanggal mulai<Input aria-label="Tanggal mulai" type="date" value={dateFrom} onChange={(event) => setDateFrom(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">Tanggal akhir<Input aria-label="Tanggal akhir" type="date" value={dateTo} onChange={(event) => setDateTo(event.target.value)} /></label>
          <label className="grid gap-1.5 text-sm font-medium">Jumlah per halaman<select aria-label="Jumlah per halaman" className="h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" value={pageSizeDraft} onChange={(event) => setPageSizeDraft(event.target.value)}>{[10, 20, 50, 100].map((size) => <option key={size} value={size}>{size}</option>)}</select></label>
        </div>
      </details>
    </form>

    {hasFilters ? <div className="flex flex-wrap items-center gap-2" aria-label="Filter aktif">{activeFilters.map((filter) => <Badge key={filter.key} variant="secondary" className="gap-1 py-1 pl-2.5 pr-1">{filter.label}<button type="button" className="rounded p-0.5 hover:bg-background/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`Hapus filter ${filter.label}`} onClick={() => removeFilter(filter.key)}><X className="size-3.5" aria-hidden="true" /></button></Badge>)}<Button type="button" variant="ghost" size="sm" onClick={resetFilters}>Reset semua</Button></div> : null}

    {auditQuery.isError ? <DataState kind="error" title="Riwayat aktivitas belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => auditQuery.refetch() }} /> : auditQuery.isPending ? <DataState kind="loading" title="Memuat riwayat aktivitas" description="Mengambil catatan perubahan." /> : entries.length === 0 ? <DataState kind="empty" title="Belum ada aktivitas yang sesuai" description={hasFilters ? 'Ubah atau reset filter untuk melihat catatan lain.' : 'Aktivitas penting akan tampil di sini saat sistem mulai digunakan.'} /> : <>
      <section aria-label="Tabel aktivitas" className="hidden md:block">
        <DataTable label="Daftar aktivitas" minimumWidth={780}><thead><tr><th>Waktu</th><th>Pelaku</th><th>Aksi</th><th>Objek</th><th><span className="sr-only">Tindakan</span></th></tr></thead><tbody>{entries.map((entry) => { const actionPresentation = presentAuditAction(entry.action); return <tr key={entry.id}><td className="whitespace-nowrap tabular-nums">{new Date(entry.created_at).toLocaleString('id-ID')}</td><td>{presentActor(entry.actor_name)}</td><td><strong className="block">{actionPresentation.label}</strong><span className="text-xs text-muted-foreground">{actionPresentation.code}</span></td><td><Badge variant="outline">{presentResource(entry.resource_type)}{entry.resource_id ? ` / ${entry.resource_id}` : ''}</Badge></td><td className="text-right"><Button type="button" size="sm" variant="outline" onClick={() => setSelectedEntry(entry)}>Lihat detail</Button></td></tr>; })}</tbody></DataTable>
      </section>
      <section aria-label="Kartu aktivitas mobile" className="grid gap-3 md:hidden">{entries.map((entry) => <AuditMobileCard key={entry.id} entry={entry} onOpen={() => setSelectedEntry(entry)} />)}</section>
    </>}

    {!auditQuery.isPending && !auditQuery.isError ? <nav className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-sm text-muted-foreground sm:flex-row" aria-label="Pagination riwayat aktivitas"><span>Menampilkan {total === 0 ? 0 : ((page - 1) * pageSize) + 1}–{Math.min(page * pageSize, total)} dari {total}</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</Button><span className="whitespace-nowrap">Halaman {page} dari {totalPages}</span><Button size="sm" variant="outline" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Berikutnya</Button></div></nav> : null}

    <AuditDetailSheet entry={selectedEntry} onClose={() => setSelectedEntry(null)} />
  </div>;
}
