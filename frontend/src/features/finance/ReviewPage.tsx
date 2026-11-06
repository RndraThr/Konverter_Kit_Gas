import { useMutation, useQueries, useQuery } from '@tanstack/react-query';
import { Banknote, ClipboardList, Download, FileCheck2, Layers, Wallet, type LucideIcon } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { downloadExcel, fetchRequests, financeKey, today } from './api';
import { errorMessage, RequestRow } from './shared';

const stages: { value: string; label: string; icon: LucideIcon; tone: string }[] = [
  { value: 'submitted', label: 'Perlu review', icon: ClipboardList, tone: 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200' },
  { value: 'approved', label: 'Perlu dibayar', icon: Banknote, tone: 'bg-sky-100 text-sky-800 dark:bg-sky-950/40 dark:text-sky-200' },
  { value: 'settlement_submitted', label: 'Laporan kasbon', icon: FileCheck2, tone: 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-200' },
  { value: 'paid,settlement_revision', label: 'Kasbon berjalan', icon: Wallet, tone: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200' },
  { value: '', label: 'Semua', icon: Layers, tone: 'bg-muted text-muted-foreground' },
];

/** Antrean finance: hanya tim yang ditugaskan kepadanya. */
export function ReviewPage() {
  const [status, setStatus] = useState('submitted');
  const [date, setDate] = useState(today);
  const requests = useQuery({
    queryKey: [...financeKey, 'queue', status],
    queryFn: () => fetchRequests(new URLSearchParams({ scope: 'queue', status, limit: '100' })),
  });
  const report = useMutation({
    mutationFn: () => downloadExcel(`/api/v1/finance/reports/daily?scope=queue&date=${date}`, `laporan-keuangan-${date}.xlsx`),
    onError: (error) => toast.error(errorMessage(error)),
  });
  const counts = useQueries({
    queries: stages.map((stage) => ({
      queryKey: [...financeKey, 'queue-count', stage.value],
      queryFn: () => fetchRequests(new URLSearchParams({ scope: 'queue', status: stage.value, limit: '1' })),
    })),
  });
  const rows = requests.data?.data.rows ?? [];
  const active = stages.find((stage) => stage.value === status);

  return <div className="space-y-6">
    <PageHeader
      title="Review keuangan"
      description="Antrean pengajuan dari tim yang ditugaskan kepada Anda."
      actions={<div className="flex flex-wrap items-center gap-2">
        <Input type="date" aria-label="Tanggal laporan harian" className="w-40" value={date} onChange={(event) => setDate(event.target.value)} />
        <Button variant="outline" disabled={!date || report.isPending} onClick={() => report.mutate()}><Download />{report.isPending ? 'Menyiapkan...' : 'Laporan harian'}</Button>
      </div>}
    />
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-5" role="group" aria-label="Status pengajuan">
      {stages.map((stage, index) => {
        const selected = stage.value === status;
        return <button key={stage.value} type="button" aria-pressed={selected} onClick={() => setStatus(stage.value)} className={cn('flex items-center gap-3 rounded-xl border bg-card p-3 text-left transition-all hover:border-primary/50 focus-visible:outline-2 focus-visible:outline-ring', selected && 'border-primary shadow-sm ring-1 ring-primary')}>
          <span aria-hidden="true" className={cn('grid size-10 shrink-0 place-items-center rounded-full', stage.tone)}><stage.icon className="size-5" /></span>
          <span className="min-w-0">
            <strong className="block text-xl leading-6 tabular-nums">{counts[index].data?.data.total ?? 0}</strong>
            <span className="block truncate text-xs text-muted-foreground">{stage.label}</span>
          </span>
        </button>;
      })}
    </div>
    <section className="grid gap-3" aria-label={active?.label}>
      <h2 className="text-sm font-semibold">{active?.label}</h2>
      {requests.isPending && <DataState kind="loading" title="Memuat antrean" description="Mengambil pengajuan tim Anda." />}
      {requests.isError && <DataState kind="error" title="Antrean belum dapat dimuat" description="Periksa koneksi lalu coba lagi." action={{ label: 'Coba lagi', onClick: () => requests.refetch() }} />}
      {requests.isSuccess && rows.length === 0 && <DataState kind="empty" title="Tidak ada pengajuan" description="Belum ada pengajuan pada status ini." />}
      {rows.length > 0 && <ul className="grid gap-2" aria-label="Antrean pengajuan">{rows.map((request) => <RequestRow key={request.id} request={request} queue />)}</ul>}
    </section>
  </div>;
}
