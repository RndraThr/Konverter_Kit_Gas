import { useQuery } from '@tanstack/react-query';
import { Activity, CircleCheck, Clock3, Database, ShieldCheck, UserMinus, UsersRound, type LucideIcon } from 'lucide-react';
import { DataState } from '../../components/DataState';
import { PageHeader } from '../../components/PageHeader';
import { Badge } from '../../components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/card';
import { Separator } from '../../components/ui/separator';
import { apiRequest } from '../../lib/api';

type ActivityEntry = { id: string; action: string; actor_name?: string; created_at: string };
type SystemStatus = { status: string; database: { status: string } };
type SummaryData = { users: number; active_users: number; inactive_users: number; roles: number; system?: SystemStatus; current_user?: { full_name: string; last_login_at?: string }; recent_activity?: ActivityEntry[] };
type SummaryResponse = { data: SummaryData };

function Metric({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: number | undefined }) {
  return <Card className="gap-0 py-0 shadow-none"><CardContent className="flex min-h-27 items-center gap-4 px-4 py-4"><span className="grid size-10 place-items-center rounded-lg bg-secondary text-secondary-foreground"><Icon className="size-5" aria-hidden="true" /></span><div className="grid gap-1"><strong className="text-2xl font-semibold tracking-tight" data-numeric>{value ?? '-'}</strong><span className="text-sm text-muted-foreground">{label}</span></div></CardContent></Card>;
}

function RecentActivity({ entries }: { entries: ActivityEntry[] }) {
  return <Card className="gap-0 py-0 shadow-none"><CardHeader className="border-b py-4"><CardTitle>Aktivitas terbaru</CardTitle><p className="text-sm text-muted-foreground">Perubahan administratif terakhir yang tercatat.</p></CardHeader><CardContent className="px-4 py-2">
    {entries.length === 0 ? <DataState kind="empty" title="Belum ada aktivitas administratif." description="Aktivitas pengguna akan muncul di sini setelah tersedia." /> : <div className="divide-y">{entries.map((entry) => <article key={entry.id} className="flex gap-3 py-3"><Clock3 className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><div className="min-w-0"><p className="font-medium">{entry.action}</p><p className="mt-1 text-sm text-muted-foreground">{entry.actor_name || 'Sistem'}, {new Date(entry.created_at).toLocaleString('id-ID')}</p></div></article>)}</div>}
  </CardContent></Card>;
}

function OperationalStatus({ system, currentUser }: { system?: SystemStatus; currentUser?: SummaryData['current_user'] }) {
  const systemReady = system?.status.toLowerCase() === 'ready' || system?.status.toLowerCase() === 'healthy';
  const databaseReady = system?.database.status.toLowerCase() === 'ready' || system?.database.status.toLowerCase() === 'healthy';
  return <Card className="gap-0 py-0 shadow-none"><CardHeader className="border-b py-4"><CardTitle>Status operasional</CardTitle><p className="text-sm text-muted-foreground">Kesiapan layanan untuk pekerjaan administrasi.</p></CardHeader><CardContent className="space-y-4 px-4 py-4">
    <div className="space-y-1"><p className="text-sm font-medium">{currentUser?.full_name ?? 'Pengguna'}</p><p className="text-sm text-muted-foreground">{currentUser?.last_login_at ? `Login terakhir ${new Date(currentUser.last_login_at).toLocaleString('id-ID')}` : 'Login terakhir belum tersedia'}</p></div>
    <Separator />
    <div className="space-y-3"><StatusLine icon={CircleCheck} label="Aplikasi" value={system?.status ?? '-'} active={systemReady} /><StatusLine icon={Database} label="Database" value={system?.database.status ?? '-'} active={databaseReady} /></div>
  </CardContent></Card>;
}

function StatusLine({ icon: Icon, label, value, active }: { icon: LucideIcon; label: string; value: string; active: boolean }) {
  return <div className="flex items-center justify-between gap-3"><span className="flex items-center gap-2 text-sm"><Icon className="size-4 text-muted-foreground" aria-hidden="true" />{label}</span><Badge variant={active ? 'default' : 'outline'} className="capitalize">{value}</Badge></div>;
}

export function DashboardPage() {
  const summary = useQuery({ queryKey: ['dashboard-summary'], queryFn: async () => (await apiRequest<SummaryResponse>('/api/v1/dashboard/summary')).data });
  const metrics = [
    { icon: UsersRound, label: 'Pengguna terdaftar', value: summary.data?.users },
    { icon: Activity, label: 'Pengguna aktif', value: summary.data?.active_users },
    { icon: UserMinus, label: 'Pengguna nonaktif', value: summary.data?.inactive_users },
    { icon: ShieldCheck, label: 'Role tersedia', value: summary.data?.roles },
  ];

  return <div className="space-y-8"><PageHeader title="Ringkasan program" description="Pantau fondasi akses dan kesiapan sistem Konkit." />
    {summary.isLoading ? <DataState kind="loading" title="Memuat ringkasan program" description="Mengambil data operasional terbaru." /> : summary.isError ? <DataState kind="error" title="Ringkasan belum dapat dimuat." description="Muat ulang halaman untuk mencoba kembali." action={{ label: 'Coba lagi', onClick: () => summary.refetch() }} /> : <>
      <section aria-label="Ringkasan pengguna" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{metrics.map((metric) => <Metric key={metric.label} {...metric} />)}</section>
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,.6fr)]"><RecentActivity entries={summary.data?.recent_activity ?? []} /><OperationalStatus system={summary.data?.system} currentUser={summary.data?.current_user} /></div>
    </>}
  </div>;
}
