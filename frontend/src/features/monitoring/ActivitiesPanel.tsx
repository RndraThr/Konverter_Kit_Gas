import { Check, Flag, ImageOff } from 'lucide-react';
import { useMemo, useState } from 'react';
import { DataState } from '@/components/DataState';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { todayWIB, useActivityCells, useActivitySignals } from './api';
import { ActivityReviewDialog } from './ActivityReviewDialog';
import { activityCellState, activityTypes, phaseInfo, type ActivityCell, type ActivityCellState, type Overview } from './types';

const cellTone: Record<ActivityCellState, string> = {
  revision: 'bg-destructive/10 text-destructive hover:bg-destructive/20',
  pending: 'bg-amber-100 text-amber-900 hover:bg-amber-200 dark:bg-amber-950/40 dark:text-amber-200',
  approved: 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-950/30 dark:text-emerald-300',
  missing: 'text-destructive ring-1 ring-inset ring-destructive/40',
  empty: 'text-muted-foreground/50',
};

/**
 * Activity documentation per kabupaten: one cell per activity type with its
 * photos to check (amber), to fix (red) or approved (green); a required type
 * still without photos after its phase is marked missing. A cell opens the
 * album to check it photo by photo. State sits in the URL (?schedule=&activity=).
 */
export function ActivitiesPanel({ overview, userId, scheduleId, activity, onOpen }: {
  overview: Overview; userId: string; scheduleId: string; activity: string;
  onOpen: (scheduleId: string | null, activity: string | null) => void;
}) {
  useActivitySignals();
  const cells = useActivityCells(overview.program_id);
  const today = todayWIB();
  const mine = overview.rows.filter((row) => row.reviewers.some((r) => r.id === userId));
  const [onlyMine, setOnlyMine] = useState(mine.length > 0);
  const rows = onlyMine && mine.length > 0 ? mine : overview.rows;
  const byKey = useMemo(() => new Map((cells.data ?? []).map((c) => [`${c.schedule_id}|${c.activity_type}`, c])), [cells.data]);
  const cellOf = (schedule: string, type: string): ActivityCell | undefined => byKey.get(`${schedule}|${type}`);

  const totals = rows.reduce((sum, row) => {
    activityTypes.forEach((type) => {
      const cell = cellOf(row.schedule_id, type.code);
      const state = activityCellState(cell, type, row.days, today);
      sum.pending += cell?.pending ?? 0;
      sum.revision += cell?.rejected ?? 0;
      if (state === 'missing') sum.missing += 1;
    });
    return sum;
  }, { pending: 0, revision: 0, missing: 0 });

  if (cells.isPending) return <DataState kind="loading" title="Memuat dokumentasi kegiatan" description="Menghitung foto per kabupaten." />;
  if (cells.isError) return <DataState kind="error" title="Dokumentasi kegiatan belum dapat dimuat" description={cells.error.message} action={{ label: 'Muat ulang', onClick: () => void cells.refetch() }} />;

  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">
      <Card className="p-4"><p className="text-xs text-muted-foreground">Foto belum dicek</p><p className="text-2xl font-semibold tabular-nums text-amber-600">{totals.pending}</p></Card>
      <Card className="p-4"><p className="text-xs text-muted-foreground">Foto menunggu revisi lapangan</p><p className="text-2xl font-semibold tabular-nums text-destructive">{totals.revision}</p></Card>
      <Card className="p-4"><p className="text-xs text-muted-foreground">Kegiatan belum ada foto (tahap sudah lewat)</p><p className="text-2xl font-semibold tabular-nums">{totals.missing}</p></Card>
    </div>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
        <span className="flex items-center gap-1"><span className="size-3 rounded bg-amber-200" />Ada foto baru</span>
        <span className="flex items-center gap-1"><span className="size-3 rounded bg-destructive/30" />Perlu revisi</span>
        <span className="flex items-center gap-1"><span className="size-3 rounded bg-emerald-200" />Disetujui</span>
        <span className="flex items-center gap-1"><span className="size-3 rounded ring-1 ring-destructive/60" />Belum ada foto, tahap sudah lewat</span>
      </div>
      {mine.length > 0 ? <Label className="flex items-center gap-2 font-normal">
        <Checkbox checked={onlyMine} onCheckedChange={(checked) => setOnlyMine(checked === true)} />Hanya kabupaten saya ({mine.length})
      </Label> : null}
    </div>
    <Card className="overflow-x-auto p-0">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b bg-muted/40 text-xs">
            <th className="sticky left-0 z-10 bg-muted px-3 py-2 text-left font-medium">Kabupaten</th>
            {activityTypes.map((type) => <th key={type.code} title={type.label} className="px-1 py-2 text-center font-medium whitespace-nowrap">
              <span className={cn('mx-auto mb-1 block h-1 w-6 rounded-full', phaseInfo(type.phase)?.dot)} />{type.short}
            </th>)}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => <tr key={row.schedule_id} className="border-b last:border-0">
            <th scope="row" className="sticky left-0 z-10 bg-card px-3 py-1.5 text-left font-medium whitespace-nowrap">
              {row.regency_name}
              {row.today_phase ? <span className="ml-2 text-[11px] font-normal text-muted-foreground">{phaseInfo(row.today_phase)?.short}</span> : null}
            </th>
            {activityTypes.map((type) => {
              const cell = cellOf(row.schedule_id, type.code);
              const state = activityCellState(cell, type, row.days, today);
              const label = `${type.label} ${row.regency_name}`;
              return <td key={type.code} className="p-0.5 text-center">
                {cell ? <button type="button" aria-label={label} title={`${label}: ${cell.photos} foto, ${cell.pending} baru, ${cell.rejected} revisi`}
                  onClick={() => onOpen(row.schedule_id, type.code)}
                  className={cn('flex h-9 w-full min-w-12 items-center justify-center gap-1 rounded-md text-xs font-semibold tabular-nums transition', cellTone[state])}>
                  {state === 'revision' ? <><Flag className="size-3" />{cell.rejected}</> : state === 'pending' ? cell.pending : <><Check className="size-3.5" />{cell.photos}</>}
                </button> : <span title={state === 'missing' ? `${label}: belum ada foto` : label}
                  className={cn('flex h-9 w-full min-w-12 items-center justify-center rounded-md', cellTone[state])}>
                  {state === 'missing' ? <ImageOff className="size-3.5" /> : '·'}
                </span>}
              </td>;
            })}
          </tr>)}
        </tbody>
      </table>
    </Card>
    <ActivityReviewDialog scheduleId={scheduleId} activity={activity} onClose={() => onOpen(null, null)} />
  </div>;
}
