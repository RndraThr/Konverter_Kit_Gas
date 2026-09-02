import { useQuery } from '@tanstack/react-query';
import { Activity, ShieldCheck, UserMinus, UsersRound } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import styles from './DashboardPage.module.css';

type SummaryResponse = { data: { users: number; active_users: number; inactive_users: number; roles: number; system?: { status: string; database: { status: string } }; current_user?: { full_name: string; last_login_at?: string }; recent_activity?: { id: string; action: string; actor_name?: string; created_at: string }[] } };

export function DashboardPage() {
  const summary = useQuery({
    queryKey: ['dashboard-summary'],
    queryFn: async () => {
      const summary = await apiRequest<SummaryResponse>('/api/v1/dashboard/summary');
      return summary.data;
    },
  });
  return <div className="page">
    <header className="pageHeader"><div><h1>Ringkasan program</h1><p>Pantau fondasi akses dan kesiapan sistem Konkit.</p></div></header>
    {summary.isError ? <div className="errorState">Ringkasan belum dapat dimuat.</div> : <div className={styles.metrics}>
      <article><UsersRound aria-hidden="true" /><div><strong>{summary.data?.users ?? '-'}</strong><span>Pengguna terdaftar</span></div></article>
      <article><Activity aria-hidden="true" /><div><strong>{summary.data?.active_users ?? '-'}</strong><span>Pengguna aktif</span></div></article>
      <article><UserMinus aria-hidden="true" /><div><strong>{summary.data?.inactive_users ?? '-'}</strong><span>Pengguna nonaktif</span></div></article>
      <article><ShieldCheck aria-hidden="true" /><div><strong>{summary.data?.roles ?? '-'}</strong><span>Role tersedia</span></div></article>
    </div>}
    <section className={styles.readiness}><div><h2>Status operasional</h2><p>{summary.data?.current_user?.full_name ?? 'Pengguna'}{summary.data?.current_user?.last_login_at ? `, login terakhir ${new Date(summary.data.current_user.last_login_at).toLocaleString('id-ID')}` : ''}</p></div><span>Aplikasi {summary.data?.system?.status ?? '-'} / Database {summary.data?.system?.database.status ?? '-'}</span></section>
    <section className={styles.activity}><h2>Aktivitas terbaru</h2>{summary.data?.recent_activity?.length ? summary.data.recent_activity.map((entry) => <div key={entry.id}><strong>{entry.action}</strong><span>{entry.actor_name || 'Sistem'}, {new Date(entry.created_at).toLocaleString('id-ID')}</span></div>) : <p className="emptyState">Belum ada aktivitas administratif.</p>}</section>
    <section className={styles.preparation}><h2>Data program</h2><p>Data kabupaten dan calon penerima akan tampil di sini setelah modul operasional disiapkan.</p></section>
  </div>;
}
