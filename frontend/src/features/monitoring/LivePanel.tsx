import { Activity, CheckCircle2, ClipboardCheck, PackageCheck, RotateCcw, Target } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { phaseInfo, rakornasStyle, type Overview, type ScheduleRow } from './types';

const percent = (value: number, of: number) => (of > 0 ? Math.min(100, Math.round((value / of) * 100)) : 0);

export function timeAgo(value: string | null, now = Date.now()): string {
  if (!value) return 'belum ada aktivitas';
  const minutes = Math.max(0, Math.round((now - new Date(value).getTime()) / 60_000));
  if (minutes < 1) return 'baru saja';
  if (minutes < 60) return `${minutes} menit lalu`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} jam lalu`;
  return `${Math.round(hours / 24)} hari lalu`;
}

function Stat({ icon, label, value, hint, tone }: { icon: ReactNode; label: string; value: number; hint?: string; tone?: string }) {
  return <Card size="sm">
    <CardContent className="flex items-center gap-3">
      <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary [&_svg]:size-5', tone)}>{icon}</div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="text-2xl font-semibold tabular-nums">{value.toLocaleString('id-ID')}</p>
        {hint ? <p className="truncate text-xs text-muted-foreground">{hint}</p> : null}
      </div>
    </CardContent>
  </Card>;
}

function Bar({ value, of, className }: { value: number; of: number; className: string }) {
  return <div className="h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemin={0} aria-valuemax={of} aria-valuenow={value}>
    <div className={cn('h-full rounded-full transition-[width] duration-500', className)} style={{ width: `${percent(value, of)}%` }} />
  </div>;
}

function RegencyCard({ row, onOpen, duplicates = 0 }: { row: ScheduleRow; onOpen: () => void; duplicates?: number }) {
  const phase = phaseInfo(row.today_phase);
  return <button type="button" onClick={onOpen} className="group rounded-xl border bg-card p-4 text-left transition hover:border-primary/50 hover:shadow-sm focus-visible:outline-2 focus-visible:outline-primary">
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0">
        <p className="truncate font-semibold">{row.regency_name}</p>
        <p className="truncate text-xs text-muted-foreground">{row.province_name}{row.zone_name ? ` · ${row.zone_name}` : ''}</p>
      </div>
      {phase ? <Badge className={cn('shrink-0 border-0', phase.cell)}>
        {phase.short}{row.today_phase === 'distribusi' && row.distribution_of > 0 ? ` hari ${row.distribution_day}/${row.distribution_of}` : ''}
      </Badge> : <Badge variant="outline" className="shrink-0">Tidak ada tahap</Badge>}
    </div>
    <div className="mt-4 space-y-2.5 text-sm">
      <div>
        <div className="mb-1 flex justify-between text-xs"><span className="text-muted-foreground">Selesai / target</span><span className="tabular-nums font-medium">{row.completed} / {row.quota} ({percent(row.completed, row.quota)}%)</span></div>
        <Bar value={row.completed} of={row.quota} className="bg-sky-500" />
      </div>
      <div>
        <div className="mb-1 flex justify-between text-xs"><span className="text-muted-foreground">Sudah dicek / selesai</span><span className="tabular-nums font-medium">{row.approved} / {row.completed}</span></div>
        <Bar value={row.approved} of={row.completed} className="bg-emerald-500" />
      </div>
    </div>
    <div className="mt-3 flex flex-wrap gap-1.5 text-xs">
      <span className="rounded-md bg-sky-50 px-2 py-0.5 text-sky-900 dark:bg-sky-950/40 dark:text-sky-200">+{row.completed_today} hari ini</span>
      {row.pending_photos > 0 ? <span className="rounded-md bg-amber-100 px-2 py-0.5 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">{row.pending_photos} foto belum dicek</span> : null}
      {row.revision > 0 ? <span className="rounded-md bg-destructive/10 px-2 py-0.5 text-destructive">{row.revision} revisi</span> : null}
      {duplicates > 0 ? <span className="rounded-md bg-destructive px-2 py-0.5 font-medium text-white">{duplicates} serial dobel</span> : null}
    </div>
    <div className="mt-3 flex items-center justify-between gap-2 border-t pt-2.5 text-xs text-muted-foreground">
      <span className="truncate">{row.reviewers.length ? row.reviewers.map((r) => r.name).join(', ') : 'Belum ada pemeriksa'}</span>
      <span className="shrink-0">{timeAgo(row.last_activity_at)}</span>
    </div>
  </button>;
}

/** Live: headline numbers and one card per kabupaten distributing today. */
export function LivePanel({ overview, onOpen, duplicates = {} }: { overview: Overview; onOpen: (scheduleId: string) => void; duplicates?: Record<string, number> }) {
  const [showAll, setShowAll] = useState(false);
  const active = overview.rows.filter((row) => row.today_phase === 'distribusi');
  const shown = showAll ? overview.rows : active;
  const sum = (key: 'quota' | 'completed' | 'completed_today' | 'approved' | 'pending' | 'pending_photos' | 'revision') => shown.reduce((total, row) => total + row[key], 0);
  const others = overview.rows.filter((row) => row.today_phase !== '' && row.today_phase !== 'distribusi');
  const rakornas = overview.rakornas.includes(overview.date);

  return <section className="space-y-4" aria-label="Monitoring live">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <span className="relative flex size-2.5"><span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75" /><span className="relative inline-flex size-2.5 rounded-full bg-emerald-500" /></span>
        {new Date(`${overview.date}T00:00:00`).toLocaleDateString('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}
        {' · '}{active.length} kabupaten distribusi hari ini
        {rakornas ? <Badge className={cn('border-0', rakornasStyle.cell)}>Rakornas</Badge> : null}
      </p>
      <Label className="flex items-center gap-2 text-sm font-normal">
        <Checkbox checked={showAll} onCheckedChange={(checked) => setShowAll(checked === true)} />
        Tampilkan semua kabupaten
      </Label>
    </div>
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
      <Stat icon={<Target />} label="Target" value={sum('quota')} hint={`${shown.length} kabupaten`} />
      <Stat icon={<PackageCheck />} label="Selesai" value={sum('completed')} hint={`${percent(sum('completed'), sum('quota'))}% dari target`} tone="bg-sky-100 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300" />
      <Stat icon={<Activity />} label="Selesai hari ini" value={sum('completed_today')} tone="bg-sky-100 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300" />
      <Stat icon={<CheckCircle2 />} label="Sudah dicek" value={sum('approved')} tone="bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300" />
      <Stat icon={<ClipboardCheck />} label="Foto belum dicek" value={sum('pending_photos')} hint={`${sum('pending')} pelanggan`} tone="bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300" />
      <Stat icon={<RotateCcw />} label="Perlu revisi" value={sum('revision')} tone="bg-destructive/10 text-destructive" />
    </div>
    {shown.length === 0
      ? <p className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">Tidak ada kabupaten yang dijadwalkan distribusi hari ini. Atur tahapan di tab Jadwal atau tampilkan semua kabupaten.</p>
      : <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{shown.map((row) => <RegencyCard key={row.schedule_id} row={row} duplicates={duplicates[row.schedule_id]} onOpen={() => onOpen(row.schedule_id)} />)}</div>}
    {!showAll && others.length > 0 ? <div className="rounded-lg border bg-muted/20 p-3">
      <p className="mb-2 text-xs font-medium text-muted-foreground">Tahap lain hari ini</p>
      <div className="flex flex-wrap gap-1.5">{others.map((row) => {
        const phase = phaseInfo(row.today_phase);
        return <span key={row.schedule_id} className="flex items-center gap-1.5 rounded-md border bg-background px-2 py-1 text-xs">
          <span className={cn('size-2 rounded-full', phase?.dot)} />{row.regency_name} · {phase?.short}
        </span>;
      })}</div>
    </div> : null}
  </section>;
}
