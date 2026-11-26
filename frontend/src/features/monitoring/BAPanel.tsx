import { Check, Flag } from 'lucide-react';
import { useMemo, useState } from 'react';
import { DataState } from '@/components/DataState';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { useBACells } from './api';
import { BAReviewDialog } from './BAReviewDialog';
import { baTypes, type BACell, type Overview } from './types';

/**
 * BA check per kabupaten: one cell per BA type with the uploaded documents to
 * check (amber), to fix (red) or approved (green). A BA appears once its
 * signed version is uploaded. BA Perorangan is checked in each recipient's
 * review, so its cell opens the kabupaten's queue.
 */
export function BAPanel({ overview, userId, scheduleId, type, onOpen, onQueue }: {
  overview: Overview; userId: string; scheduleId: string; type: string;
  onOpen: (scheduleId: string | null, type: string | null) => void; onQueue: (scheduleId: string) => void;
}) {
  const cells = useBACells(overview.program_id);
  const mine = overview.rows.filter((row) => row.reviewers.some((r) => r.id === userId));
  const [onlyMine, setOnlyMine] = useState(mine.length > 0);
  const rows = onlyMine && mine.length > 0 ? mine : overview.rows;
  const byKey = useMemo(() => new Map((cells.data ?? []).map((c) => [`${c.schedule_id}|${c.type}`, c])), [cells.data]);
  const totals = (cells.data ?? []).filter((c) => rows.some((r) => r.schedule_id === c.schedule_id))
    .reduce((sum, c) => ({ pending: sum.pending + c.pending, rejected: sum.rejected + c.rejected, approved: sum.approved + c.approved }), { pending: 0, rejected: 0, approved: 0 });
  const regency = overview.rows.find((r) => r.schedule_id === scheduleId)?.regency_name ?? '';

  if (cells.isPending) return <DataState kind="loading" title="Memuat BA" description="Mengambil BA yang sudah diunggah per kabupaten." />;
  if (cells.isError) return <DataState kind="error" title="Daftar BA belum dapat dimuat" description={cells.error.message} action={{ label: 'Muat ulang', onClick: () => void cells.refetch() }} />;

  const cell = (c: BACell | undefined) => {
    if (!c) return <span className="flex h-9 items-center justify-center text-muted-foreground/50">·</span>;
    const state = c.rejected > 0 ? 'revision' : c.pending > 0 ? 'pending' : 'approved';
    return <span className={cn('flex h-9 w-full min-w-12 items-center justify-center gap-1 rounded-md text-xs font-semibold tabular-nums',
      state === 'revision' && 'bg-destructive/10 text-destructive', state === 'pending' && 'bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200',
      state === 'approved' && 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300')}>
      {state === 'revision' ? <><Flag className="size-3" />{c.rejected}</> : state === 'pending' ? c.pending : <><Check className="size-3.5" />{c.items}</>}
    </span>;
  };

  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">
      <Card className="p-4"><p className="text-xs text-muted-foreground">BA belum dicek</p><p className="text-2xl font-semibold tabular-nums text-amber-600">{totals.pending}</p></Card>
      <Card className="p-4"><p className="text-xs text-muted-foreground">BA menunggu revisi lapangan</p><p className="text-2xl font-semibold tabular-nums text-destructive">{totals.rejected}</p></Card>
      <Card className="p-4"><p className="text-xs text-muted-foreground">BA disetujui</p><p className="text-2xl font-semibold tabular-nums text-emerald-600">{totals.approved}</p></Card>
    </div>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-xs text-muted-foreground">BA muncul setelah versi bertanda tangannya diunggah (pindaian lengkap, halaman tanda tangan, atau lembar daftar hadir). BA Perorangan dicek di antrean pelanggan.</p>
      {mine.length > 0 ? <Label className="flex items-center gap-2 font-normal"><Checkbox checked={onlyMine} onCheckedChange={(checked) => setOnlyMine(checked === true)} />Hanya kabupaten saya ({mine.length})</Label> : null}
    </div>
    <Card className="overflow-x-auto p-0">
      <table className="w-full border-collapse text-sm">
        <thead><tr className="border-b bg-muted/40 text-xs">
          <th className="sticky left-0 z-10 bg-muted px-3 py-2 text-left font-medium">Kabupaten</th>
          {baTypes.map((t) => <th key={t.code} title={t.label} className="px-1 py-2 text-center font-medium whitespace-nowrap">{t.short}</th>)}
        </tr></thead>
        <tbody>
          {rows.map((row) => <tr key={row.schedule_id} className="border-b last:border-0">
            <th scope="row" className="sticky left-0 z-10 bg-card px-3 py-1.5 text-left font-medium whitespace-nowrap">{row.regency_name}</th>
            {baTypes.map((t) => {
              const c = byKey.get(`${row.schedule_id}|${t.code}`);
              return <td key={t.code} className="p-0.5 text-center">
                {c ? <button type="button" className="w-full transition hover:opacity-80" aria-label={`${t.label} ${row.regency_name}`}
                  title={`${t.label} ${row.regency_name}: ${c.items} dokumen, ${c.pending} baru, ${c.rejected} revisi`}
                  onClick={() => (t.code === 'individual' ? onQueue(row.schedule_id) : onOpen(row.schedule_id, t.code))}>{cell(c)}</button> : cell(undefined)}
              </td>;
            })}
          </tr>)}
        </tbody>
      </table>
    </Card>
    <BAReviewDialog scheduleId={scheduleId} type={type} regencyName={regency} onClose={() => onOpen(null, null)} />
  </div>;
}
