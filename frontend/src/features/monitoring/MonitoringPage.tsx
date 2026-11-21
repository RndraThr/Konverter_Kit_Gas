import { useQuery } from '@tanstack/react-query';
import { CalendarRange, ClipboardCheck, Radio } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { SearchableSelect } from '@/components/SearchableSelect';
import { getBootstrap } from '@/lib/api';
import { useCan } from '@/lib/permissions';
import { cn } from '@/lib/utils';
import { useMonitoringSignals, useOverview } from './api';
import { CalendarPanel } from './CalendarPanel';
import { LivePanel } from './LivePanel';
import { QueuePanel } from './QueuePanel';

type Tab = 'live' | 'antrean' | 'jadwal';

/**
 * Monitoring Distribusi: live numbers of the kabupaten distributing today,
 * the check queue of each reviewer's kabupaten, and the phase calendar.
 * State sits in the URL (?tab=&program=&schedule=&slot=) so a notification
 * can open one recipient directly.
 */
export function MonitoringPage() {
  const [params, setParams] = useSearchParams();
  const tab = (['live', 'antrean', 'jadwal'].includes(params.get('tab') ?? '') ? params.get('tab') : 'live') as Tab;
  const overview = useOverview(params.get('program') ?? '');
  const bootstrap = useQuery({ queryKey: ['bootstrap'], queryFn: getBootstrap });
  const canManage = useCan('monitoring.manage');
  useMonitoringSignals();

  const update = (next: Record<string, string | null>) => setParams((current) => {
    const merged = new URLSearchParams(current);
    Object.entries(next).forEach(([key, value]) => { if (value) merged.set(key, value); else merged.delete(key); });
    return merged;
  });

  const data = overview.data;
  const userId = bootstrap.data?.data.id ?? '';
  const rows = data?.rows ?? [];
  const mine = rows.filter((row) => row.reviewers.some((r) => r.id === userId));
  const tabs: { value: Tab; label: string; hint: string; icon: typeof Radio; count?: number }[] = [
    { value: 'live', label: 'Live', hint: 'Kabupaten yang distribusi hari ini', icon: Radio, count: rows.filter((row) => row.today_phase === 'distribusi').length },
    { value: 'antrean', label: 'Antrean cek', hint: mine.length ? `Belum dicek di ${mine.length} kabupaten Anda` : 'Cek dokumentasi pelanggan', icon: ClipboardCheck, count: (mine.length ? mine : rows).reduce((sum, row) => sum + row.pending, 0) },
    { value: 'jadwal', label: 'Jadwal', hint: 'Tahapan & pembagian tugas', icon: CalendarRange, count: rows.length },
  ];
  return <div className="space-y-5">
    <PageHeader
      title="Monitoring distribusi"
      description="Pantau kabupaten yang sedang distribusi secara langsung, cek dokumentasi tiap pelanggan, dan atur jadwal tahapan."
      actions={data && data.programs.length > 1 ? <SearchableSelect
        aria-label="Program"
        className="w-64"
        options={data.programs.map((p) => ({ value: p.id, label: p.name }))}
        value={data.program_id}
        onValueChange={(program) => update({ program, schedule: null, slot: null })}
      /> : undefined}
    />
    <div role="tablist" aria-label="Bagian monitoring" className="grid gap-2 sm:grid-cols-3">
      {tabs.map((t) => {
        const active = tab === t.value;
        const Icon = t.icon;
        return <button key={t.value} type="button" role="tab" aria-selected={active} onClick={() => update({ tab: t.value, slot: null })}
          className={cn('group flex items-center gap-3 rounded-xl border p-3 text-left transition',
            active ? 'border-primary bg-primary text-primary-foreground shadow-md shadow-primary/20' : 'bg-card hover:border-primary/40 hover:shadow-sm')}>
          <span className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg', active ? 'bg-primary-foreground/15' : 'bg-primary/10 text-primary')}>
            <Icon className="size-5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2 font-semibold">
              {t.label}
              {t.value === 'live' ? <span className="relative flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75" /><span className="relative inline-flex size-2 rounded-full bg-emerald-500" /></span> : null}
            </span>
            <span className={cn('block truncate text-xs', active ? 'text-primary-foreground/80' : 'text-muted-foreground')}>{t.hint}</span>
          </span>
          {t.count !== undefined ? <span className={cn('rounded-full px-2 py-0.5 text-sm font-semibold tabular-nums', active ? 'bg-primary-foreground/20' : 'bg-muted')}>{t.count}</span> : null}
        </button>;
      })}
    </div>
    {overview.isPending ? <DataState kind="loading" title="Memuat monitoring" description="Mengambil jadwal dan progres kabupaten." />
      : overview.isError || !data ? <DataState kind="error" title="Monitoring belum dapat dimuat" description={overview.error?.message ?? 'Coba lagi.'} action={{ label: 'Muat ulang', onClick: () => void overview.refetch() }} />
      : data.rows.length === 0 ? <DataState kind="empty" title="Belum ada kabupaten" description="Program ini belum punya jadwal kabupaten." />
      : tab === 'live' ? <LivePanel overview={data} onOpen={(schedule) => update({ tab: 'antrean', schedule, slot: null })} />
      : tab === 'antrean' ? <QueuePanel
        overview={data}
        userId={userId}
        canSeeAll={canManage}
        scheduleId={params.get('schedule') ?? ''}
        slot={params.get('slot') ? Number(params.get('slot')) : null}
        onSchedule={(schedule) => update({ schedule, slot: null })}
        onSlot={(slot) => update({ slot: slot === null ? null : String(slot) })}
      />
      : <CalendarPanel overview={data} canManage={canManage} />}
  </div>;
}
