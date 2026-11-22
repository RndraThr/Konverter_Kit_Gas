import type { ReactNode } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { DataState } from '@/components/DataState';
import { PageHeader } from '@/components/PageHeader';
import { SearchableSelect } from '@/components/SearchableSelect';
import { activityTypeLabels, type ActivityType } from '../activities/types';
import { useDashboardProgress } from './api';
import { ActivityChart, CumulativeChart, DailyChart, StageChart } from './Charts';
import { cumulative, formatNumber, percent, stages, zoneLabel } from './stats';
import type { Dashboard, DashboardRegency } from './types';

/**
 * Dashboard progres (super admin): how far one program has gone, per zone,
 * per kabupaten grouped by zone, per day, and the activity documentation.
 */
export function SuperDashboardPage() {
  const [params, setParams] = useSearchParams();
  const progress = useDashboardProgress(params.get('program') ?? '');
  const data = progress.data;
  return <div className="space-y-5">
    <PageHeader
      title="Dashboard progres"
      description="Grafik progres pekerjaan per program: berapa penerima yang sudah tertaut, diserahkan, fotonya disetujui, dan BA-nya final, dibagi per zona dan kabupaten."
      actions={data && data.programs.length > 1 ? <SearchableSelect
        aria-label="Program"
        className="w-64"
        options={data.programs.map((p) => ({ value: p.id, label: p.name }))}
        value={data.program_id}
        onValueChange={(program) => setParams(program ? { program } : {})}
      /> : undefined}
    />
    {progress.isPending ? <DataState kind="loading" title="Memuat dashboard" description="Menghitung progres tiap kabupaten." />
      : progress.isError || !data ? <DataState kind="error" title="Dashboard belum dapat dimuat" description={progress.error?.message ?? 'Coba lagi.'} action={{ label: 'Coba lagi', onClick: () => void progress.refetch() }} />
      : data.regencies.length === 0 ? <DataState kind="empty" title="Program ini belum punya jadwal kabupaten" description="Tambahkan jadwal kabupaten di Persiapan program agar progresnya tampil di sini." />
      : <DashboardBody data={data} />}
  </div>;
}

function DashboardBody({ data }: { data: Dashboard }) {
  const navigate = useNavigate();
  const { totals } = data;
  const open = (scheduleId: string) => navigate(`/monitoring-distribusi?${new URLSearchParams({ program: data.program_id, tab: 'antrean', schedule: scheduleId })}`);
  const byZone = data.zones.map((zone) => ({ zone, regencies: data.regencies.filter((r) => r.zone_name === zone.zone_name) }));
  const days = cumulative(data.daily);
  const activities = data.activities.map((a) => ({ label: activityTypeLabels[a.activity_type as ActivityType] ?? a.activity_type, media: a.media }));
  return <>
    <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
      <StatCard label="Kuota" value={totals.quota} />
      {stages.map((s) => <StatCard key={s.key} label={s.label} value={totals[s.key]} share={percent(totals[s.key], totals.quota)} color={s.color} />)}
    </div>

    <Section title="Progres per zona" description="Persentase dari kuota tiap zona.">
      <StageChart rows={data.zones.map((z) => ({ ...z, id: z.zone_name, label: zoneLabel(z.zone_name) }))} />
    </Section>

    <Section title="Per kabupaten" description="Dikelompokkan per zona. Klik bar untuk membuka antrean cek kabupaten di Monitoring.">
      <div className="space-y-6">
        {byZone.map(({ zone, regencies }) => <div key={zone.zone_name || '-'} className="space-y-2">
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">{zoneLabel(zone.zone_name)}</h3>
            <span className="text-xs text-muted-foreground tabular-nums">{formatNumber(zone.completed)} / {formatNumber(zone.quota)} diserahkan</span>
          </div>
          <StageChart rows={regencies.map((r) => ({ ...r, id: r.schedule_id, label: r.regency_name }))} onOpen={open} />
        </div>)}
        <details className="rounded-lg border">
          <summary className="cursor-pointer px-3 py-2 text-sm font-medium">Lihat tabel</summary>
          <RegencyTable regencies={data.regencies} onOpen={open} />
        </details>
      </div>
    </Section>

    <div className="grid gap-5 lg:grid-cols-2">
      <Section title="Penyerahan per hari" description="Jumlah penerima yang diserahkan tiap tanggal (WIB).">
        {days.length ? <DailyChart days={days} /> : <Empty text="Belum ada penyerahan." />}
      </Section>
      <Section title="Total diserahkan" description="Akumulasi penyerahan dibanding kuota program.">
        {days.length ? <CumulativeChart days={days} quota={totals.quota} /> : <Empty text="Belum ada penyerahan." />}
      </Section>
    </div>

    <Section title="Dokumentasi kegiatan" description="Jumlah foto dan video aktif per jenis kegiatan.">
      {activities.length ? <ActivityChart rows={activities} /> : <Empty text="Belum ada dokumentasi kegiatan." />}
    </Section>
  </>;
}

function StatCard({ label, value, share, color }: { label: string; value: number; share?: number; color?: string }) {
  return <div role="group" aria-label={label} className="rounded-xl border bg-card p-4">
    <p className="flex items-center gap-2 text-sm text-muted-foreground">
      {color ? <span className="size-2.5 rounded-full" style={{ background: color }} aria-hidden="true" /> : null}
      {label}
    </p>
    <p className="mt-1 text-2xl font-semibold tabular-nums">{formatNumber(value)}</p>
    {share !== undefined ? <p className="text-xs text-muted-foreground">{share}% dari kuota</p> : <p className="text-xs text-muted-foreground">penerima</p>}
  </div>;
}

function Section({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  const id = `section-${title.toLowerCase().replace(/[^a-z]+/g, '-')}`;
  return <section aria-labelledby={id} className="min-w-0 space-y-3 rounded-xl border bg-card p-4">
    <div>
      <h2 id={id} className="font-semibold">{title}</h2>
      <p className="text-sm text-muted-foreground">{description}</p>
    </div>
    {children}
  </section>;
}

function Empty({ text }: { text: string }) {
  return <p className="py-8 text-center text-sm text-muted-foreground">{text}</p>;
}

function RegencyTable({ regencies, onOpen }: { regencies: DashboardRegency[]; onOpen: (scheduleId: string) => void }) {
  return <div className="overflow-x-auto">
    <table className="w-full text-sm">
      <thead className="text-left text-xs text-muted-foreground">
        <tr><th className="px-3 py-2 font-medium">Kabupaten</th><th className="px-3 py-2 font-medium">Zona</th><th className="px-3 py-2 text-right font-medium">Kuota</th>
          {stages.map((s) => <th key={s.key} className="px-3 py-2 text-right font-medium">{s.label}</th>)}</tr>
      </thead>
      <tbody>
        {regencies.map((r) => <tr key={r.schedule_id} className="border-t">
          <td className="px-3 py-2"><button type="button" className="text-left font-medium text-primary hover:underline" onClick={() => onOpen(r.schedule_id)}>{r.regency_name}</button></td>
          <td className="px-3 py-2">{zoneLabel(r.zone_name)}</td>
          <td className="px-3 py-2 text-right tabular-nums">{formatNumber(r.quota)}</td>
          {stages.map((s) => <td key={s.key} className="px-3 py-2 text-right tabular-nums">{formatNumber(r[s.key])} <span className="text-xs text-muted-foreground">({percent(r[s.key], r.quota)}%)</span></td>)}
        </tr>)}
      </tbody>
    </table>
  </div>;
}
