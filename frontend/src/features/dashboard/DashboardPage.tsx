import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Search } from 'lucide-react';
import { FormEvent, useState } from 'react';
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
import { allocationStatusLabel, distributionStatusLabel, type Recipient, type RecipientInput, type RecipientPage, type RecipientStats } from './types';

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

export function DashboardPage() {
  const canManage = useCan('recipients.manage');
  const client = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get('search') ?? '');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Recipient>();
  const [pendingCancel, setPendingCancel] = useState<Recipient>();

  const stats = useQuery({ queryKey: ['recipients', 'stats'], queryFn: () => apiRequest<{ data: RecipientStats }>('/api/v1/recipients/stats') });
  const list = useQuery({ queryKey: ['recipients', params.toString()], queryFn: () => apiRequest<{ data: RecipientPage }>(`/api/v1/recipients?${params.toString()}`) });
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

  const setFilter = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); next.set('page', '1'); setParams(next); };
  const submitSearch = (event: FormEvent) => { event.preventDefault(); setFilter('search', search); };
  const setPage = (page: number) => { const next = new URLSearchParams(params); next.set('page', String(page)); setParams(next); };
  const openEdit = (recipient: Recipient) => { save.reset(); setEditing(recipient); setDialogOpen(true); };
  const openCreate = () => { save.reset(); setEditing(undefined); setDialogOpen(true); };

  const page = list.data?.data.page ?? 1;
  const pageSize = list.data?.data.page_size ?? 20;
  const total = list.data?.data.total ?? 0;
  const saveError = save.error as ApiError | null;
  const saveFields = saveError?.fields ?? {};
  const saveMessage = save.isError && Object.keys(saveFields).length === 0 ? save.error.message : undefined;
  const scheduleOptions: ScheduleOption[] = (schedules.data?.data ?? []).map((item) => ({ id: item.id, name: item.name, regency_name: item.regency?.name ?? '', program_type: item.program?.program_type ?? 'farmer' }));

  return <div className="space-y-6">
    <PageHeader title="Data Penerima" description="Pusat data penerima bantuan lintas program dan wilayah." actions={canManage ? <Button onClick={openCreate}><Plus />Tambah penerima</Button> : undefined} />

    <section aria-label="Statistik penerima" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.total ?? '-'}</strong><span className="block text-sm text-muted-foreground">Total penerima</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.distributed ?? 0}</strong><span className="block text-sm text-muted-foreground">Sudah distribusi</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.ready ?? 0}</strong><span className="block text-sm text-muted-foreground">Siap/menunggu</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.needs_review ?? 0}</strong><span className="block text-sm text-muted-foreground">Perlu ditinjau</span></CardContent></Card>
      <Card><CardContent><strong className="text-2xl font-semibold tabular-nums">{stats.data?.data.by_allocation_status.cancelled ?? 0}</strong><span className="block text-sm text-muted-foreground">Dibatalkan</span></CardContent></Card>
    </section>

    <form className="flex flex-col gap-3 rounded-lg border bg-card p-3 lg:flex-row lg:items-end lg:flex-wrap" onSubmit={submitSearch}>
      <div className="relative min-w-0 flex-1"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-5 text-muted-foreground" /><Input aria-label="Cari penerima" className="pl-10" placeholder="Cari nama, NIK, atau nomor kartu" value={search} onChange={(event) => setSearch(event.target.value)} /></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-regency-label">Kabupaten</Label><Select value={params.get('regency_id') ?? ''} onValueChange={(value) => setFilter('regency_id', value ?? '')}><SelectTrigger aria-labelledby="filter-regency-label"><SelectValue placeholder="Semua kabupaten" /></SelectTrigger><SelectContent><SelectItem value="">Semua kabupaten</SelectItem>{regencies.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.document_code} - {item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-program-label">Program</Label><Select value={params.get('program_id') ?? ''} onValueChange={(value) => setFilter('program_id', value ?? '')}><SelectTrigger aria-labelledby="filter-program-label"><SelectValue placeholder="Semua program" /></SelectTrigger><SelectContent><SelectItem value="">Semua program</SelectItem>{programs.data?.data.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-allocation-label">Status alokasi</Label><Select value={params.get('allocation_status') ?? ''} onValueChange={(value) => setFilter('allocation_status', value ?? '')}><SelectTrigger aria-labelledby="filter-allocation-label"><SelectValue placeholder="Aktif (bukan dibatalkan)" /></SelectTrigger><SelectContent><SelectItem value="">Aktif (bukan dibatalkan)</SelectItem>{Object.entries(allocationStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <div className="grid min-w-0 gap-2"><Label id="filter-distribution-label">Status distribusi</Label><Select value={params.get('distribution_status') ?? ''} onValueChange={(value) => setFilter('distribution_status', value ?? '')}><SelectTrigger aria-labelledby="filter-distribution-label"><SelectValue placeholder="Semua status" /></SelectTrigger><SelectContent><SelectItem value="">Semua status</SelectItem>{Object.entries(distributionStatusLabel).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
      <Button type="submit" variant="outline">Cari</Button>
    </form>

    {list.isError ? <DataState kind="error" title="Data penerima belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => list.refetch() }} /> : list.isPending ? <DataState kind="loading" title="Memuat data penerima" description="Mengambil data dari seluruh kabupaten." /> : list.data?.data.items.length === 0 ? <DataState kind="empty" title="Belum ada penerima yang sesuai" description="Ubah filter atau tambahkan penerima baru." /> : <DataTable label="Daftar penerima" minimumWidth={1200}>
      <thead><tr><th>Kabupaten</th><th>Program</th><th>Jadwal</th><th>No. Pembagian</th><th>Nama</th><th>NIK</th><th>No. Kartu/KUSUKA</th><th>Desa/Kecamatan</th><th>Status alokasi</th><th>Status distribusi</th>{canManage && <th>Aksi</th>}</tr></thead>
      <tbody>{list.data?.data.items.map((item) => <tr key={item.allocation_id}>
        <td><strong>{item.regency_document_code}</strong> {item.regency_name}</td>
        <td>{item.program_name}</td>
        <td>{item.schedule_name}</td>
        <td className="font-medium tabular-nums">{item.distribution_number}</td>
        <td><strong>{item.full_name}</strong></td>
        <td className="tabular-nums">{item.nik || '-'}</td>
        <td>{item.sector_identifier || '-'}</td>
        <td>{[item.village, item.district].filter(Boolean).join(', ') || '-'}</td>
        <td><Badge variant={allocationBadgeVariant(item.allocation_status)}>{allocationStatusLabel[item.allocation_status] ?? item.allocation_status}</Badge></td>
        <td><Badge variant={distributionBadgeVariant(item.distribution_status)}>{item.distribution_status ? distributionStatusLabel[item.distribution_status] ?? item.distribution_status : '-'}</Badge></td>
        {canManage && <td className="flex gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(item)}>Edit</Button>
          {item.allocation_status === 'cancelled'
            ? <Button type="button" variant="ghost" size="sm" onClick={() => restoreMutation.mutate(item.allocation_id)}>Pulihkan</Button>
            : <Button type="button" variant="ghost" size="sm" className="text-destructive" onClick={() => setPendingCancel(item)}>Batalkan</Button>}
        </td>}
      </tr>)}</tbody>
    </DataTable>}

    <nav className="flex flex-col gap-3 border-t pt-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between" aria-label="Pagination penerima">
      <span>{total} penerima</span>
      <span className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</Button>Halaman {page}<Button size="sm" variant="outline" disabled={page * pageSize >= total} onClick={() => setPage(page + 1)}>Berikutnya</Button></span>
    </nav>

    {canManage && <RecipientDialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) save.reset(); }} recipient={editing} schedules={scheduleOptions} pending={save.isPending} error={saveMessage} fields={saveFields} onSave={(values) => save.mutate(values)} />}

    <AlertDialog open={Boolean(pendingCancel)} onOpenChange={(open) => { if (!open) setPendingCancel(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Batalkan {pendingCancel?.full_name}?</AlertDialogTitle><AlertDialogDescription>Penerima ini akan disembunyikan dari daftar aktif dan statistik, tapi tetap tersimpan untuk audit dan dapat dipulihkan.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => { if (pendingCancel) cancelMutation.mutate(pendingCancel.allocation_id); }}>Batalkan penerima</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
