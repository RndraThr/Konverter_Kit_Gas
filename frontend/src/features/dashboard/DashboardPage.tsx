import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, ArrowUpDown, Ban, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, LoaderCircle, Pencil, Plus, RotateCcw, Search, Undo2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent } from '../../components/ui/card';
import { DataState } from '../../components/DataState';
import { DataTable } from '../../components/DataTable';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { PageHeader } from '../../components/PageHeader';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select';
import { apiRequest, type ApiError } from '../../lib/api';
import { useCan } from '../../lib/permissions';
import { RecipientDialog, type ScheduleOption } from './RecipientDialog';
import { buildPageItems, summarizeEvidence } from './recipientTable';
import { allocationStatusLabel, distributionStatusLabel, type EvidenceSlot, type Recipient, type RecipientInput, type RecipientPage, type RecipientStats } from './types';

type RegencyOption = { id: string; name: string; document_code: string };
type ProgramOption = { id: string; name: string; program_type: 'farmer' | 'fisherman' };
type ScheduleListItem = { id: string; name: string; regency?: { name: string }; program?: { program_type: 'farmer' | 'fisherman' } };

function allocationBadgeVariant(status: string) {
  if (status === 'distributed') return 'default' as const;
  if (status === 'cancelled') return 'destructive' as const;
  if (status === 'ready' || status === 'replaced') return 'secondary' as const;
  return 'outline' as const;
}

function distributionBadgeVariant(status: string | null) {
  if (status === 'completed') return 'default' as const;
  if (status === 'cancelled') return 'destructive' as const;
  return 'outline' as const;
}

const stickyNumber = 'sticky left-0 z-20 w-[72px] min-w-[72px] max-w-[72px] bg-card group-hover:bg-muted md:w-44 md:min-w-44 md:max-w-44';
const stickyName = 'sticky left-[72px] z-20 w-[120px] min-w-[120px] max-w-[120px] bg-card group-hover:bg-muted md:left-44 md:w-56 md:min-w-56 md:max-w-56';
const stickyNIK = 'sticky left-[192px] z-20 w-[140px] min-w-[140px] max-w-[140px] border-r bg-card text-xs shadow-[6px_0_10px_-10px_rgba(15,23,42,0.55)] group-hover:bg-muted md:left-[400px] md:w-48 md:min-w-48 md:max-w-48 md:text-sm';
const stickyHeader = 'top-0 z-30 bg-muted';
const filterKeys = ['regency_id', 'program_id', 'schedule_id', 'district', 'allocation_status', 'distribution_status', 'evidence_status'] as const;

function EvidenceCell({ slots }: { slots: EvidenceSlot[] }) {
  const summary = summarizeEvidence(slots);
  const labels = {
    complete: 'Lengkap', partial: 'Sebagian', empty: 'Belum ada', 'not-configured': 'Belum diatur',
  } as const;
  const tone = {
    complete: 'bg-primary/12 text-primary',
    partial: 'bg-amber-100 text-amber-800 dark:bg-amber-950/45 dark:text-amber-300',
    empty: 'bg-muted text-muted-foreground',
    'not-configured': 'bg-muted text-muted-foreground',
  } as const;
  const requiredLabel = summary.state === 'not-configured'
    ? 'Belum diatur'
    : `${summary.requiredComplete}/${summary.requiredTotal} wajib`;
  const progress = summary.requiredTotal === 0 ? (slots.length > 0 ? 100 : 0) : (summary.requiredComplete / summary.requiredTotal) * 100;

  if (slots.length === 0) {
    return <div className="space-y-1.5"><span className="font-medium text-muted-foreground">{requiredLabel}</span><span className={`block w-fit rounded-full px-2 py-0.5 text-xs ${tone[summary.state]}`}>{labels[summary.state]}</span></div>;
  }

  return <details className="group/evidence min-w-36">
    <summary className="cursor-pointer list-none rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/50" aria-label={`Rincian evidence: ${requiredLabel}`}>
      <div className="flex items-center justify-between gap-2"><strong className="font-semibold tabular-nums">{requiredLabel}</strong><span className={`rounded-full px-2 py-0.5 text-xs ${tone[summary.state]}`}>{labels[summary.state]}</span></div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden="true"><span className="block h-full rounded-full bg-primary transition-[width]" style={{ width: `${progress}%` }} /></div>
      {summary.optionalTotal > 0 && <span className="mt-1 block text-xs text-muted-foreground tabular-nums">Opsional {summary.optionalComplete}/{summary.optionalTotal}</span>}
    </summary>
    <ul className="mt-3 space-y-2 border-t pt-2 text-xs">
      {slots.map((slot) => <li key={slot.slot_code} className="flex items-start justify-between gap-3">
        <span className="min-w-0"><span className="block truncate font-medium" title={slot.label}>{slot.label}</span><span className="text-muted-foreground">{slot.is_required ? 'Wajib' : 'Opsional'}</span></span>
        <span className={slot.complete ? 'text-primary tabular-nums' : 'text-muted-foreground tabular-nums'}>{slot.accepted_files}/{slot.min_files}</span>
      </li>)}
    </ul>
  </details>;
}

export function DashboardPage() {
  const canManage = useCan('recipients.manage');
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get('search') ?? '');
  const [district, setDistrict] = useState(params.get('district') ?? '');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Recipient>();
  const [pendingCancel, setPendingCancel] = useState<Recipient>();

  const statsParams = useMemo(() => {
    const next = new URLSearchParams(params);
    ['page', 'page_size', 'sort', 'direction'].forEach((key) => next.delete(key));
    return next.toString();
  }, [params]);
  const stats = useQuery({ queryKey: ['recipients', 'stats', statsParams], queryFn: () => apiRequest<{ data: RecipientStats }>(`/api/v1/recipients/stats${statsParams ? `?${statsParams}` : ''}`) });
  const list = useQuery({ queryKey: ['recipients', params.toString()], queryFn: () => apiRequest<{ data: RecipientPage }>(`/api/v1/recipients?${params.toString()}`), placeholderData: keepPreviousData });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: ScheduleListItem[] }>('/api/v1/program-setup/schedules') });

  const save = useMutation({
    mutationFn: (values: RecipientInput) => {
      const { schedule_id, ...updateOnly } = values;
      return apiRequest(editing ? `/api/v1/recipients/${editing.allocation_id}` : '/api/v1/recipients', {
        method: editing ? 'PATCH' : 'POST', body: JSON.stringify(editing ? updateOnly : values),
      });
    },
    onSuccess: () => { setDialogOpen(false); setEditing(undefined); client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Data penerima berhasil disimpan.'); },
  });
  const cancelMutation = useMutation({
    mutationFn: (allocationID: string) => apiRequest(`/api/v1/recipients/${allocationID}/cancel`, { method: 'POST' }),
    onSuccess: () => { setPendingCancel(undefined); client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Penerima dibatalkan.'); },
  });
  const restoreMutation = useMutation({
    mutationFn: (allocationID: string) => apiRequest(`/api/v1/recipients/${allocationID}/restore`, { method: 'POST' }),
    onSuccess: () => { client.invalidateQueries({ queryKey: ['recipients'] }); toast.success('Penerima dipulihkan.'); },
  });

  useEffect(() => {
    const urlSearch = params.get('search') ?? '';
    setSearch((current) => current === urlSearch ? current : urlSearch);
    const urlDistrict = params.get('district') ?? '';
    setDistrict((current) => current === urlDistrict ? current : urlDistrict);
  }, [params]);

  useEffect(() => {
    const urlSearch = params.get('search') ?? '';
    const normalized = search.trim();
    if (normalized === urlSearch) return;
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(params);
      normalized ? next.set('search', normalized) : next.delete('search');
      next.set('page', '1');
      setParams(next, { replace: true });
    }, 350);
    return () => window.clearTimeout(timer);
  }, [params, search, setParams]);

  useEffect(() => {
    const urlDistrict = params.get('district') ?? '';
    const normalized = district.trim();
    if (normalized === urlDistrict) return;
    const timer = window.setTimeout(() => {
      const next = new URLSearchParams(params);
      normalized ? next.set('district', normalized) : next.delete('district');
      next.set('page', '1');
      setParams(next, { replace: true });
    }, 350);
    return () => window.clearTimeout(timer);
  }, [district, params, setParams]);

  const setFilter = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const setPageSize = (value: string | null) => {
    if (!value || !['10', '20', '50', '100'].includes(value)) return;
    const next = new URLSearchParams(params);
    next.set('page_size', value);
    next.set('page', '1');
    setParams(next);
  };
  const setSort = (column: string) => {
    const next = new URLSearchParams(params);
    const isCurrent = next.get('sort') === column;
    next.set('sort', column);
    next.set('direction', isCurrent && next.get('direction') === 'asc' ? 'desc' : 'asc');
    next.set('page', '1');
    setParams(next);
  };
  const resetFilters = () => {
    const next = new URLSearchParams(params);
    filterKeys.forEach((key) => next.delete(key));
    next.delete('search');
    next.set('page', '1');
    setSearch('');
    setDistrict('');
    setParams(next);
  };
  const openEdit = (recipient: Recipient) => { save.reset(); setEditing(recipient); setDialogOpen(true); };
  const openCreate = () => { save.reset(); setEditing(undefined); setDialogOpen(true); };

  const page = list.data?.data.page ?? 1;
  const pageSize = list.data?.data.page_size ?? 20;
  const total = list.data?.data.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pageItems = useMemo(() => buildPageItems(page, totalPages), [page, totalPages]);
  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  const scheduleOptions: ScheduleOption[] = (schedules.data?.data ?? []).map((item) => ({ id: item.id, name: item.name, regency_name: item.regency?.name ?? '', program_type: item.program?.program_type ?? 'farmer' }));
  const evidenceStats = stats.data?.data.by_evidence_status ?? {};
  const evidenceNeedsCompletion = (evidenceStats.partial ?? 0) + (evidenceStats.empty ?? 0) + (evidenceStats['not-configured'] ?? 0);
  const activeFilterCount = filterKeys.filter((key) => Boolean(params.get(key))).length + (params.get('search') ? 1 : 0);
  const sortableHeader = (label: string, column: string, className = stickyHeader) => {
    const active = params.get('sort') === column;
    const direction = active ? (params.get('direction') === 'asc' ? 'asc' : 'desc') : undefined;
    const Icon = !active ? ArrowUpDown : direction === 'asc' ? ArrowUp : ArrowDown;
    const nextDirection = active && direction === 'asc' ? 'descending' : 'ascending';
    return <th className={className} aria-sort={direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : 'none'}>
      <Button type="button" variant="ghost" size="sm" className="h-8 w-full min-w-0 justify-between gap-2 overflow-hidden px-0 font-bold hover:bg-transparent" aria-label={`Urutkan ${label} ${nextDirection}`} onClick={() => setSort(column)}>
        <span className="truncate text-left">{label.toLocaleUpperCase('id-ID')}</span><Icon aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </th>;
  };

  return <div className="space-y-6">
    <PageHeader title="Data Penerima" description="Pusat data penerima bantuan lintas program dan wilayah." actions={canManage ? <Button onClick={openCreate}><Plus />Tambah penerima</Button> : undefined} />

    <section aria-label="Statistik penerima" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.total ?? '-'}</strong><span className="block text-sm text-muted-foreground">Total hasil</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.ready ?? 0}</strong><span className="block text-sm text-muted-foreground">Siap/menunggu</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.distributed ?? 0}</strong><span className="block text-sm text-muted-foreground">Sudah didistribusikan</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{evidenceStats.complete ?? 0}</strong><span className="block text-sm text-muted-foreground">Evidence lengkap</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{evidenceNeedsCompletion}</strong><span className="block text-sm text-muted-foreground">Evidence perlu dilengkapi</span></CardContent></Card>
    </section>

    <div className="space-y-3 rounded-xl border bg-card p-4">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-end">
      <div className="relative min-w-64 flex-1">{list.isFetching && search.trim() === (params.get('search') ?? '') ? <LoaderCircle aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 animate-spin text-primary" /> : <Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 text-muted-foreground" />}<Input aria-label="Cari penerima" className="pl-10 pr-24" placeholder="Cari nama, NIK, atau nomor kartu" value={search} onChange={(event) => setSearch(event.target.value)} /><span className="pointer-events-none absolute top-3 right-3 text-xs text-muted-foreground">Realtime</span></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-regency-label">Kabupaten</Label><Select value={params.get('regency_id') ?? ''} onValueChange={(value) => setFilter('regency_id', value ?? '')}><SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder="Semua kabupaten" /></SelectTrigger><SelectContent><SelectItem value="">Semua kabupaten</SelectItem>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-program-label">Program</Label><Select value={params.get('program_id') ?? ''} onValueChange={(value) => setFilter('program_id', value ?? '')}><SelectTrigger aria-labelledby="filter-program-label"><SelectValue placeholder="Semua program" /></SelectTrigger><SelectContent><SelectItem value="">Semua program</SelectItem>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-schedule-label">Jadwal</Label><Select value={params.get('schedule_id') ?? ''} onValueChange={(value) => setFilter('schedule_id', value ?? '')}><SelectTrigger aria-labelledby="filter-schedule-label"><SelectValue placeholder="Semua jadwal" /></SelectTrigger><SelectContent><SelectItem value="">Semua jadwal</SelectItem>{schedules.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-[minmax(12rem,1fr)_repeat(3,minmax(11rem,auto))_auto] xl:items-end">
      <div className="grid min-w-0 gap-2"><Label htmlFor="filter-district">Kecamatan</Label><Input id="filter-district" value={district} placeholder="Semua kecamatan" onChange={(event) => setDistrict(event.target.value)} /></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-allocation-label">Status alokasi</Label><Select value={params.get('allocation_status') ?? ''} onValueChange={(value) => setFilter('allocation_status', value ?? '')}><SelectTrigger aria-labelledby="filter-allocation-label"><SelectValue placeholder="Aktif (bukan dibatalkan)" /></SelectTrigger><SelectContent><SelectItem value="">Aktif (bukan dibatalkan)</SelectItem>{Object.entries(allocationStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-distribution-label">Status distribusi</Label><Select value={params.get('distribution_status') ?? ''} onValueChange={(value) => setFilter('distribution_status', value ?? '')}><SelectTrigger aria-labelledby="filter-distribution-label"><SelectValue placeholder="Semua status" /></SelectTrigger><SelectContent><SelectItem value="">Semua status</SelectItem>{Object.entries(distributionStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-evidence-label">Kelengkapan evidence</Label><Select value={params.get('evidence_status') ?? ''} onValueChange={(value) => setFilter('evidence_status', value ?? '')}><SelectTrigger aria-labelledby="filter-evidence-label"><SelectValue placeholder="Semua kelengkapan" /></SelectTrigger><SelectContent><SelectItem value="">Semua kelengkapan</SelectItem><SelectItem value="complete">Lengkap</SelectItem><SelectItem value="partial">Sebagian</SelectItem><SelectItem value="empty">Belum ada</SelectItem><SelectItem value="not-configured">Belum diatur</SelectItem></SelectContent></Select></div>
      <Button type="button" variant="outline" disabled={activeFilterCount === 0} onClick={resetFilters} aria-label="Reset filter"><RotateCcw />Reset{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''}</Button>
      </div>
    </div>

    {list.isError ? <DataState kind="error" title="Data penerima belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => list.refetch() }} /> : list.isPending ? <DataState kind="loading" title="Memuat data penerima" description="Mengambil data dari seluruh kabupaten." /> : list.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada penerima yang sesuai" description="Ubah filter atau tambahkan penerima baru." /> : <DataTable label="Daftar penerima" minimumWidth={2078} className="table-fixed">
      <colgroup><col className="w-[72px] md:w-44" /><col className="w-[120px] md:w-56" /><col className="w-[140px] md:w-48" /><col className="w-60" /><col className="w-44" /><col className="w-52" /><col className="w-44" /><col className="w-56" /><col className="w-56" /><col className="w-40" /><col className="w-40" />{canManage && <col className="w-28" />}</colgroup>
      <thead className="[&_th]:uppercase [&_th]:tracking-wide"><tr>
        {sortableHeader('No. Pembagian', 'distribution_number', `${stickyNumber} ${stickyHeader}`)}
        {sortableHeader('Nama', 'full_name', `${stickyName} ${stickyHeader}`)}
        {sortableHeader('NIK', 'nik', `${stickyNIK} ${stickyHeader}`)}
        {sortableHeader('Kelengkapan evidence', 'evidence')}
        <th className={stickyHeader}>No. Kartu/KUSUKA</th>
        {sortableHeader('Desa/Kecamatan', 'district')}
        {sortableHeader('Kabupaten', 'regency')}
        {sortableHeader('Program', 'program')}
        {sortableHeader('Jadwal', 'schedule')}
        {sortableHeader('Status alokasi', 'allocation_status')}
        {sortableHeader('Status distribusi', 'distribution_status')}
        {canManage && <th className={`${stickyHeader} border-l text-center shadow-[-6px_0_10px_-10px_rgba(15,23,42,0.55)] md:sticky md:right-0`}>Aksi</th>}
      </tr></thead>
      <tbody>{list.data?.data.items.map((item) => <tr key={item.allocation_id} className="group hover:bg-muted">
        <td className={`${stickyNumber} font-semibold tabular-nums`}>{item.distribution_number ?? '-'}</td>
        <td className={stickyName}><strong className="block truncate" title={item.full_name}>{item.full_name}</strong></td>
        <td className={`${stickyNIK} tabular-nums`}>{item.nik || '-'}</td>
        <td><EvidenceCell slots={item.evidence_slots ?? []} /></td>
        <td>{item.sector_identifier || '-'}</td>
        <td>{[item.village, item.district].filter(Boolean).join(', ') || '-'}</td>
        <td><strong>{item.regency_document_code}</strong><span className="ml-1">{item.regency_name}</span></td>
        <td>{item.program_name}</td>
        <td>{item.schedule_name}</td>
        <td><Badge variant={allocationBadgeVariant(item.allocation_status)}>{allocationStatusLabel[item.allocation_status] ?? item.allocation_status}</Badge></td>
        <td><Badge variant={distributionBadgeVariant(item.distribution_status)}>{item.distribution_status ? distributionStatusLabel[item.distribution_status] ?? item.distribution_status : '-'}</Badge></td>
        {canManage && <td className="z-20 border-l bg-card shadow-[-6px_0_10px_-10px_rgba(15,23,42,0.55)] group-hover:bg-muted md:sticky md:right-0"><div className="flex items-center justify-center gap-1">
          <Button type="button" variant="ghost" size="icon-sm" aria-label="Edit" title="Edit penerima" onClick={() => openEdit(item)}><Pencil /></Button>
          {item.allocation_status === 'cancelled'
            ? <Button type="button" variant="ghost" size="icon-sm" aria-label="Pulihkan" title="Pulihkan penerima" onClick={() => restoreMutation.mutate(item.allocation_id)}><Undo2 /></Button>
            : <Button type="button" variant="ghost" size="icon-sm" aria-label="Batalkan" title="Batalkan penerima" className="text-destructive" onClick={() => setPendingCancel(item)}><Ban /></Button>}
        </div></td>}
      </tr>)}</tbody>
    </DataTable>}

    <nav className="flex flex-col gap-4 border-t pt-4 text-sm text-muted-foreground xl:flex-row xl:items-center xl:justify-between" aria-label="Pagination penerima">
      <div className="flex flex-wrap items-center gap-4"><span className="tabular-nums">{rangeStart}–{rangeEnd} dari {total} penerima</span><div className="flex items-center gap-2"><Label id="page-size-label" className="whitespace-nowrap">Jumlah data per halaman</Label><Select value={String(pageSize)} onValueChange={setPageSize}><SelectTrigger aria-labelledby="page-size-label" className="w-40"><SelectValue /></SelectTrigger><SelectContent>{[10, 20, 50, 100].map((size) => <SelectItem key={size} value={String(size)}>{size} per halaman</SelectItem>)}</SelectContent></Select></div></div>
      <div className="flex flex-wrap items-center gap-1">
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman pertama" disabled={page <= 1} onClick={() => setPage(1)}><ChevronsLeft /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman sebelumnya" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
        {pageItems.map((item) => typeof item === 'number'
          ? <Button type="button" size="icon-sm" variant={item === page ? 'default' : 'outline'} aria-label={`Halaman ${item}`} aria-current={item === page ? 'page' : undefined} key={item} onClick={() => setPage(item)}>{item}</Button>
          : <span key={item} aria-hidden="true" className="flex size-11 items-center justify-center">…</span>)}
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman berikutnya" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman terakhir" disabled={page >= totalPages} onClick={() => setPage(totalPages)}><ChevronsRight /></Button>
      </div>
    </nav>

    {canManage && <RecipientDialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} recipient={editing} schedules={scheduleOptions} pending={save.isPending} error={saveMessage} fields={saveFields} onSave={(values) => save.mutate(values)} />}

    <AlertDialog open={Boolean(pendingCancel)} onOpenChange={(open) => { if (!open) setPendingCancel(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Batalkan {pendingCancel?.full_name}?</AlertDialogTitle><AlertDialogDescription>Penerima ini akan disembunyikan dari daftar aktif dan statistik, tapi tetap tersimpan untuk audit dan dapat dipulihkan.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingCancel) cancelMutation.mutate(pendingCancel.allocation_id); }}>Batalkan penerima</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
