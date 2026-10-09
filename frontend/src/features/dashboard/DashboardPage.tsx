import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, ArrowUpDown, Ban, ChevronDown, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, LoaderCircle, Pencil, Plus, RotateCcw, Search, SlidersHorizontal, Undo2, X } from 'lucide-react';
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
import type { ProgramZone } from '../programs/types';
import { RecipientDialog, type ScheduleOption } from './RecipientDialog';
import { RecipientMobileCard } from './RecipientMobileCard';
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

// The table cell only has room for the counterpart's name, so the reason and the exact time live in
// the title attribute; the date is short enough to stay visible.
function formatReplacementDate(value: string) {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '-' : parsed.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' });
}

const stickyHeader = 'top-0 z-30 bg-muted';
const filterKeys = ['regency_id', 'program_id', 'zone_id', 'schedule_id', 'district', 'allocation_status', 'distribution_status', 'evidence_status'] as const;

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
  const [filtersOpen, setFiltersOpen] = useState(false);

  const statsParams = useMemo(() => {
    const next = new URLSearchParams(params);
    ['page', 'page_size', 'sort', 'direction'].forEach((key) => next.delete(key));
    return next.toString();
  }, [params]);
  const stats = useQuery({ queryKey: ['recipients', 'stats', statsParams], queryFn: () => apiRequest<{ data: RecipientStats }>(`/api/v1/recipients/stats${statsParams ? `?${statsParams}` : ''}`) });
  const list = useQuery({ queryKey: ['recipients', params.toString()], queryFn: () => apiRequest<{ data: RecipientPage }>(`/api/v1/recipients?${params.toString()}`), placeholderData: keepPreviousData });
  const regencies = useQuery({ queryKey: ['program-setup', 'regencies'], queryFn: () => apiRequest<{ data: RegencyOption[] }>('/api/v1/program-setup/regencies') });
  const programs = useQuery({ queryKey: ['program-setup', 'programs'], queryFn: () => apiRequest<{ data: ProgramOption[] }>('/api/v1/program-setup/programs') });
  const selectedProgramID = params.get('program_id') ?? '';
  const zones = useQuery({
    queryKey: ['program-setup', 'programs', selectedProgramID, 'zones'],
    queryFn: () => apiRequest<{ data: ProgramZone[] }>(`/api/v1/program-setup/programs/${selectedProgramID}/zones`),
    enabled: Boolean(selectedProgramID),
  });
  const schedules = useQuery({ queryKey: ['program-setup', 'schedules'], queryFn: () => apiRequest<{ data: ScheduleListItem[] }>('/api/v1/program-setup/schedules') });

  const save = useMutation({
    mutationFn: (values: RecipientInput) => {
      const { schedule_id: _schedule_id, ...updateOnly } = values;
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
  const setProgramFilter = (value: string) => {
    const next = new URLSearchParams(params);
    value ? next.set('program_id', value) : next.delete('program_id');
    next.delete('zone_id');
    next.set('page', '1');
    setParams(next);
  };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const setPageSize = (value: string | null) => {
    if (!value || !['50', '100', 'all'].includes(value)) return;
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
  const setSortSelection = (value: string | null) => {
    if (!value) return;
    const [column, direction] = value.split(':');
    const next = new URLSearchParams(params);
    next.set('sort', column);
    next.set('direction', direction);
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
  const pageSize = list.data?.data.page_size ?? 50;
  const total = list.data?.data.total ?? 0;
  const showAll = params.get('page_size') === 'all' || list.data?.data.all === true;
  const totalPages = showAll ? 1 : Math.max(1, Math.ceil(total / pageSize));
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
  const activeFilterChips = [
    params.get('search') && { key: 'search', label: `Pencarian: ${params.get('search')}` },
    params.get('program_id') && { key: 'program_id', label: `Program: ${programs.data?.data.find((item) => item.id === params.get('program_id'))?.name ?? 'Dipilih'}` },
    params.get('zone_id') && { key: 'zone_id', label: `Zona: ${zones.data?.data.find((item) => item.id === params.get('zone_id'))?.name ?? 'Dipilih'}` },
    params.get('regency_id') && { key: 'regency_id', label: `Kabupaten: ${regencies.data?.data.find((item) => item.id === params.get('regency_id'))?.name ?? 'Dipilih'}` },
    params.get('schedule_id') && { key: 'schedule_id', label: `Jadwal: ${schedules.data?.data.find((item) => item.id === params.get('schedule_id'))?.name ?? 'Dipilih'}` },
    params.get('district') && { key: 'district', label: `Kecamatan: ${params.get('district')}` },
    params.get('allocation_status') && { key: 'allocation_status', label: allocationStatusLabel[params.get('allocation_status') ?? ''] ?? 'Status alokasi' },
    params.get('distribution_status') && { key: 'distribution_status', label: distributionStatusLabel[params.get('distribution_status') ?? ''] ?? 'Status distribusi' },
    params.get('evidence_status') && { key: 'evidence_status', label: `Dokumentasi: ${{ complete: 'Lengkap', partial: 'Sebagian', empty: 'Belum ada', 'not-configured': 'Belum diatur' }[params.get('evidence_status') ?? ''] ?? 'Dipilih'}` },
  ].filter((item): item is { key: string; label: string } => Boolean(item));
  const clearFilter = (key: string) => {
    if (key === 'search') setSearch('');
    if (key === 'district') setDistrict('');
    if (key === 'program_id') return setProgramFilter('');
    setFilter(key, '');
  };
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

    <section aria-label="Filter penerima" className="space-y-4 rounded-2xl border bg-card p-4 shadow-sm">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(18rem,1.4fr)_repeat(3,minmax(12rem,1fr))] xl:items-end">
        <div className="relative min-w-0 md:col-span-2 xl:col-span-1">
          <Label htmlFor="recipient-search" className="mb-2 block">Cari penerima</Label>
          {list.isFetching && search.trim() === (params.get('search') ?? '') ? <LoaderCircle aria-hidden="true" className="pointer-events-none absolute bottom-3 left-3 size-5 animate-spin text-primary" /> : <Search aria-hidden="true" className="pointer-events-none absolute bottom-3 left-3 size-5 text-muted-foreground" />}
          <Input id="recipient-search" aria-label="Cari penerima" className="pl-10" placeholder="Nama, NIK, atau nomor kartu" value={search} onChange={(event) => setSearch(event.target.value)} />
        </div>
        <div className="grid min-w-0 gap-2"><Label id="filter-program-label">Program</Label><Select value={selectedProgramID} onValueChange={(value) => setProgramFilter(value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-program-label"><SelectValue placeholder="Semua program" /></SelectTrigger><SelectContent><SelectItem value="">Semua program</SelectItem>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
        <div className="grid min-w-0 gap-2"><Label id="filter-zone-label">Zona</Label><Select disabled={!selectedProgramID || zones.isPending} value={params.get('zone_id') ?? ''} onValueChange={(value) => setFilter('zone_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-zone-label"><SelectValue placeholder={!selectedProgramID ? 'Pilih program dahulu' : zones.isPending ? 'Memuat zona...' : 'Semua zona'} /></SelectTrigger><SelectContent><SelectItem value="">Semua zona</SelectItem>{zones.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
        <div className="grid min-w-0 gap-2"><Label id="filter-regency-label">Kabupaten</Label><Select value={params.get('regency_id') ?? ''} onValueChange={(value) => setFilter('regency_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-regency-label"><SelectValue placeholder="Semua kabupaten" /></SelectTrigger><SelectContent><SelectItem value="">Semua kabupaten</SelectItem>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
      </div>

      <Button type="button" variant="outline" className="w-full justify-between md:hidden" aria-expanded={filtersOpen} onClick={() => setFiltersOpen((value) => !value)}><span className="flex items-center gap-2"><SlidersHorizontal />Filter lainnya</span><ChevronDown className={filtersOpen ? 'rotate-180' : ''} /></Button>
      <div className={`${filtersOpen ? 'grid' : 'hidden'} gap-3 border-t pt-4 md:grid md:grid-cols-2 xl:grid-cols-[repeat(4,minmax(11rem,1fr))_auto] xl:items-end`}>
        <div className="grid min-w-0 gap-2"><Label id="filter-schedule-label">Jadwal</Label><Select value={params.get('schedule_id') ?? ''} onValueChange={(value) => setFilter('schedule_id', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-schedule-label"><SelectValue placeholder="Semua jadwal" /></SelectTrigger><SelectContent><SelectItem value="">Semua jadwal</SelectItem>{schedules.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
        <div className="grid min-w-0 gap-2"><Label htmlFor="filter-district">Kecamatan</Label><Input id="filter-district" value={district} placeholder="Semua kecamatan" onChange={(event) => setDistrict(event.target.value)} /></div>
        <div className="grid min-w-0 gap-2"><Label id="filter-allocation-label">Status alokasi</Label><Select value={params.get('allocation_status') ?? ''} onValueChange={(value) => setFilter('allocation_status', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-allocation-label"><SelectValue placeholder="Aktif" /></SelectTrigger><SelectContent><SelectItem value="">Aktif (bukan dibatalkan)</SelectItem>{Object.entries(allocationStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
        <div className="grid min-w-0 gap-2"><Label id="filter-evidence-label">Dokumentasi</Label><Select value={params.get('evidence_status') ?? ''} onValueChange={(value) => setFilter('evidence_status', value ?? '')}><SelectTrigger className="w-full" aria-labelledby="filter-evidence-label"><SelectValue placeholder="Semua kelengkapan" /></SelectTrigger><SelectContent><SelectItem value="">Semua kelengkapan</SelectItem><SelectItem value="complete">Lengkap</SelectItem><SelectItem value="partial">Sebagian</SelectItem><SelectItem value="empty">Belum ada</SelectItem><SelectItem value="not-configured">Belum diatur</SelectItem></SelectContent></Select></div>
        <Button type="button" variant="outline" disabled={activeFilterCount === 0} onClick={resetFilters} aria-label="Reset filter"><RotateCcw />Reset{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''}</Button>
        <div className="grid min-w-0 gap-2 md:col-span-2 xl:col-span-5"><Label id="filter-distribution-label">Status distribusi</Label><Select value={params.get('distribution_status') ?? ''} onValueChange={(value) => setFilter('distribution_status', value ?? '')}><SelectTrigger className="w-full xl:max-w-xs" aria-labelledby="filter-distribution-label"><SelectValue placeholder="Semua status" /></SelectTrigger><SelectContent><SelectItem value="">Semua status</SelectItem>{Object.entries(distributionStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      </div>

      {activeFilterChips.length > 0 && <div className="flex flex-wrap gap-2 border-t pt-3" aria-label="Filter aktif">{activeFilterChips.map((chip) => <Button key={chip.key} type="button" size="sm" variant="secondary" className="h-8 rounded-full px-3" aria-label={`Hapus filter ${chip.label}`} onClick={() => clearFilter(chip.key)}>{chip.label}<X aria-hidden="true" className="size-3.5" /></Button>)}</div>}
    </section>

    <div className="flex flex-col gap-3 rounded-xl border bg-card px-4 py-3 text-sm sm:flex-row sm:items-center sm:justify-between">
      <span className="font-medium tabular-nums text-foreground">{showAll ? `${total} penerima ditemukan` : `${rangeStart}–${rangeEnd} dari ${total} penerima`}</span>
      <div className="flex flex-wrap items-center gap-2">
        <Label id="mobile-sort-label" className="whitespace-nowrap text-muted-foreground lg:hidden">Urutkan</Label>
        <Select value={`${params.get('sort') ?? 'created_at'}:${params.get('direction') ?? 'desc'}`} onValueChange={setSortSelection}>
          <SelectTrigger aria-labelledby="mobile-sort-label" className="w-40 lg:hidden"><SelectValue /></SelectTrigger>
          <SelectContent><SelectItem value="created_at:desc">Data terbaru</SelectItem><SelectItem value="distribution_number:asc">Nomor bagi</SelectItem><SelectItem value="full_name:asc">Nama A–Z</SelectItem><SelectItem value="zone:asc">Zona</SelectItem></SelectContent>
        </Select>
        <Label id="page-size-label" className="whitespace-nowrap text-muted-foreground">Jumlah data per halaman</Label>
        <Select value={showAll ? 'all' : String(pageSize)} onValueChange={setPageSize}>
          <SelectTrigger aria-labelledby="page-size-label" className="w-36"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="50">50 data</SelectItem>
            <SelectItem value="100">100 data</SelectItem>
            <SelectItem value="all">Semua data</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>

    {list.isError ? <DataState kind="error" title="Data penerima belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => list.refetch() }} /> : list.isPending ? <DataState kind="loading" title="Memuat data penerima" description="Mengambil data dari seluruh kabupaten." /> : list.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada penerima yang sesuai" description="Ubah filter atau tambahkan penerima baru." /> : <>
      <ul aria-label="Daftar penerima mobile" className="space-y-3 lg:hidden">
        {list.data?.data.items.map((item) => <RecipientMobileCard key={item.allocation_id} recipient={item} canManage={canManage} onEdit={openEdit} onCancel={setPendingCancel} onRestore={(recipient) => restoreMutation.mutate(recipient.allocation_id)} />)}
      </ul>

      <div className="hidden lg:block">
        <DataTable label="Daftar penerima" minimumWidth={1280} className="table-fixed">
          <colgroup><col className="w-28" /><col className="w-72" /><col className="w-64" /><col className="w-64" /><col className="w-60" /><col className="w-48" />{canManage && <col className="w-28" />}</colgroup>
          <thead><tr>
            {sortableHeader('No. bagi', 'distribution_number')}
            {sortableHeader('Identitas penerima', 'full_name')}
            {sortableHeader('Wilayah', 'zone')}
            {sortableHeader('Program & jadwal', 'program')}
            {sortableHeader('Dokumentasi', 'evidence')}
            {sortableHeader('Status', 'allocation_status')}
            {canManage && <th className={`${stickyHeader} border-l text-center`}>Aksi</th>}
          </tr></thead>
          <tbody>{list.data?.data.items.map((item) => <tr key={item.allocation_id} className="group align-top hover:bg-muted">
            <td className="font-semibold tabular-nums"><span className="text-lg">{item.distribution_number ?? '—'}</span><span className="mt-1 block text-xs font-normal text-muted-foreground">{item.regency_document_code}</span></td>
            <td>
              <strong className="block truncate" title={item.full_name}>{item.full_name}</strong>
              <span className="mt-1 block font-mono text-xs text-muted-foreground">{item.nik || 'NIK belum tersedia'}</span>
              <span className="mt-1 block truncate text-xs text-muted-foreground">{item.sector_identifier || 'No. kartu/KUSUKA belum tersedia'}</span>
              {item.replaced_by && <span className="mt-1 block truncate text-xs text-amber-700 dark:text-amber-300" title={`Digantikan oleh ${item.replaced_by.full_name} pada ${formatReplacementDate(item.replaced_by.replaced_at)}. Alasan: ${item.replaced_by.reason}`}>Digantikan oleh {item.replaced_by.full_name}</span>}
              {item.replaces && <span className="mt-1 block truncate text-xs text-amber-700 dark:text-amber-300" title={`Menggantikan ${item.replaces.full_name} pada ${formatReplacementDate(item.replaces.replaced_at)}. Alasan: ${item.replaces.reason}`}>Menggantikan {item.replaces.full_name}</span>}
            </td>
            <td><strong className="block">{item.zone_name || 'Zona belum diatur'}</strong><span className="mt-1 block text-xs text-muted-foreground">{item.regency_name}</span><span className="mt-1 block text-xs">{[item.village, item.district].filter(Boolean).join(', ') || '-'}</span></td>
            <td><strong className="block font-medium">{item.program_name}</strong><span className="mt-1 block text-xs text-muted-foreground">{item.schedule_name}</span></td>
            <td><EvidenceCell slots={item.evidence_slots ?? []} /></td>
            <td><div className="flex flex-col items-start gap-2"><Badge variant={allocationBadgeVariant(item.allocation_status)}>{allocationStatusLabel[item.allocation_status] ?? item.allocation_status}</Badge><Badge variant={distributionBadgeVariant(item.distribution_status)}>{item.distribution_status ? distributionStatusLabel[item.distribution_status] ?? item.distribution_status : 'Belum distribusi'}</Badge></div></td>
            {canManage && <td className="border-l bg-card group-hover:bg-muted"><div className="flex items-center justify-center gap-1">
              <Button type="button" variant="ghost" size="icon-sm" aria-label="Edit" title="Edit penerima" onClick={() => openEdit(item)}><Pencil /></Button>
              {item.allocation_status === 'cancelled'
                ? <Button type="button" variant="ghost" size="icon-sm" aria-label="Pulihkan" title="Pulihkan penerima" onClick={() => restoreMutation.mutate(item.allocation_id)}><Undo2 /></Button>
                : <Button type="button" variant="ghost" size="icon-sm" aria-label="Batalkan" title="Batalkan penerima" className="text-destructive" onClick={() => setPendingCancel(item)}><Ban /></Button>}
            </div></td>}
          </tr>)}</tbody>
        </DataTable>
      </div>
    </>}

    {!showAll && <nav className="flex justify-end border-t pt-4 text-sm text-muted-foreground" aria-label="Pagination penerima">
      <div className="flex flex-wrap items-center gap-1">
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman pertama" disabled={page <= 1} onClick={() => setPage(1)}><ChevronsLeft /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman sebelumnya" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
        {pageItems.map((item) => typeof item === 'number'
          ? <Button type="button" size="icon-sm" variant={item === page ? 'default' : 'outline'} aria-label={`Halaman ${item}`} aria-current={item === page ? 'page' : undefined} key={item} onClick={() => setPage(item)}>{item}</Button>
          : <span key={item} aria-hidden="true" className="flex size-11 items-center justify-center">…</span>)}
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman berikutnya" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
        <Button type="button" size="icon-sm" variant="outline" aria-label="Halaman terakhir" disabled={page >= totalPages} onClick={() => setPage(totalPages)}><ChevronsRight /></Button>
      </div>
    </nav>}

    {canManage && <RecipientDialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} recipient={editing} schedules={scheduleOptions} pending={save.isPending} error={saveMessage} fields={saveFields} onSave={(values) => save.mutate(values)} />}

    <AlertDialog open={Boolean(pendingCancel)} onOpenChange={(open) => { if (!open) setPendingCancel(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Batalkan {pendingCancel?.full_name}?</AlertDialogTitle><AlertDialogDescription>Penerima ini akan disembunyikan dari daftar aktif dan statistik, tapi tetap tersimpan untuk audit dan dapat dipulihkan.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingCancel) cancelMutation.mutate(pendingCancel.allocation_id); }}>Batalkan penerima</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
