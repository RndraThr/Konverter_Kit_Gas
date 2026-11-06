import { useMutation, useQuery } from '@tanstack/react-query';
import { Download } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { FormField } from '@/components/FormField';
import { PageHeader } from '@/components/PageHeader';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { apiRequest } from '@/lib/api';
import { downloadExcel, fetchRequests, financeKey, formatRupiah, statusLabels, typeLabels, type DataResponse, type Summary, type Team } from './api';
import { errorMessage, RequestRow } from './shared';

function Stat({ label, value, note }: { label: string; value: string; note?: string }) {
  return <Card><CardContent className="pt-5">
    <p className="text-xs text-muted-foreground">{label}</p>
    <strong className="mt-1 block text-xl">{value}</strong>
    {note && <p className="text-xs text-muted-foreground">{note}</p>}
  </CardContent></Card>;
}

/** Pantauan seluruh pengajuan dan export untuk admin. */
export function MonitoringPage() {
  const [filter, setFilter] = useState({ type: '', status: '', team_id: '', from: '', to: '', q: '' });
  const params = new URLSearchParams({ scope: 'all', limit: '100', ...filter });
  const key = params.toString();
  const set = (field: keyof typeof filter) => (value: string | null) => setFilter((current) => ({ ...current, [field]: value ?? '' }));
  const summary = useQuery({ queryKey: [...financeKey, 'summary', key], queryFn: () => apiRequest<DataResponse<Summary>>(`/api/v1/finance/summary?${key}`) });
  const requests = useQuery({ queryKey: [...financeKey, 'all', key], queryFn: () => fetchRequests(params) });
  const teams = useQuery({ queryKey: [...financeKey, 'teams'], queryFn: () => apiRequest<DataResponse<Team[]>>('/api/v1/finance/teams') });
  const exported = useMutation({
    mutationFn: () => downloadExcel(`/api/v1/finance/reports/export?${key}`, 'laporan-keuangan.xlsx'),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const data = summary.data?.data;
  const rows = requests.data?.data.rows ?? [];
  const teamList = teams.data?.data ?? [];

  return <div className="space-y-6">
    <PageHeader
      title="Monitoring keuangan"
      description="Pantau seluruh pengajuan tim dan ekspor laporannya."
      actions={<Button disabled={exported.isPending} onClick={() => exported.mutate()}><Download />{exported.isPending ? 'Menyiapkan...' : 'Export Excel'}</Button>}
    />
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Stat label="Menunggu review" value={String(data?.waiting_review ?? 0)} />
      <Stat label="Menunggu dibayar" value={String(data?.waiting_payment ?? 0)} />
      <Stat label="Sudah dibayar" value={formatRupiah(data?.total_paid ?? 0)} />
      <Stat label="Kasbon belum selesai" value={formatRupiah(data?.kasbon_open ?? 0)} note={`${data?.kasbon_open_count ?? 0} kasbon, ${data?.kasbon_overdue ?? 0} terlambat`} />
    </div>
    <Card>
      <CardContent className="grid gap-3 pt-5 sm:grid-cols-2 xl:grid-cols-5">
        <div className="grid min-w-0 gap-2">
          <Label id="monitor-type">Jenis</Label>
          <Select value={filter.type} onValueChange={set('type')}>
            <SelectTrigger className="w-full" aria-labelledby="monitor-type"><SelectValue placeholder="Semua jenis">{filter.type ? typeLabels[filter.type as 'cashless'] : 'Semua jenis'}</SelectValue></SelectTrigger>
            <SelectContent><SelectItem value="">Semua jenis</SelectItem><SelectItem value="cashless">Cashless</SelectItem><SelectItem value="kasbon">Kasbon</SelectItem></SelectContent>
          </Select>
        </div>
        <div className="grid min-w-0 gap-2">
          <Label id="monitor-status">Status</Label>
          <Select value={filter.status} onValueChange={set('status')}>
            <SelectTrigger className="w-full" aria-labelledby="monitor-status"><SelectValue placeholder="Semua status">{filter.status ? statusLabels[filter.status] : 'Semua status'}</SelectValue></SelectTrigger>
            <SelectContent><SelectItem value="">Semua status</SelectItem>{Object.entries(statusLabels).filter(([value]) => value !== 'draft').map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className="grid min-w-0 gap-2">
          <Label id="monitor-team">Tim</Label>
          <Select value={filter.team_id} onValueChange={set('team_id')}>
            <SelectTrigger className="w-full" aria-labelledby="monitor-team"><SelectValue placeholder="Semua tim">{teamList.find((team) => team.id === filter.team_id)?.name ?? 'Semua tim'}</SelectValue></SelectTrigger>
            <SelectContent><SelectItem value="">Semua tim</SelectItem>{teamList.map((team) => <SelectItem key={team.id} value={team.id}>{team.name}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <FormField label="Dari" name="from" type="date" value={filter.from} onChange={(event) => set('from')(event.target.value)} />
        <FormField label="Sampai" name="to" type="date" value={filter.to} onChange={(event) => set('to')(event.target.value)} />
        <FormField className="sm:col-span-2 xl:col-span-5" label="Cari" name="q" placeholder="Nomor, keterangan, penerima, atau tim" value={filter.q} onChange={(event) => set('q')(event.target.value)} />
      </CardContent>
    </Card>
    {(data?.teams.length ?? 0) > 0 && <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Per tim</CardTitle></CardHeader>
      <CardContent className="grid gap-2 pt-5">{data?.teams.map((team) => <div key={team.team_id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3 text-sm">
        <strong className="min-w-0 break-words">{team.name}</strong>
        <span className="text-muted-foreground">{team.count} pengajuan · dibayar {formatRupiah(team.paid)} · proses {formatRupiah(team.open)}</span>
      </div>)}</CardContent>
    </Card>}
    <Card>
      <CardHeader className="border-b"><CardTitle className="text-base">Pengajuan ({requests.data?.data.total ?? 0})</CardTitle></CardHeader>
      <CardContent className="grid gap-3 pt-5">
        {requests.isPending && <DataState kind="loading" title="Memuat pengajuan" description="Mengambil data pengajuan." />}
        {requests.isError && <DataState kind="error" title="Pengajuan belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => requests.refetch() }} />}
        {requests.isSuccess && rows.length === 0 && <DataState kind="empty" title="Tidak ada pengajuan" description="Ubah filter untuk melihat data lain." />}
        {rows.length > 0 && <ul className="grid gap-2" aria-label="Seluruh pengajuan">{rows.map((request) => <RequestRow key={request.id} request={request} />)}</ul>}
      </CardContent>
    </Card>
  </div>;
}
