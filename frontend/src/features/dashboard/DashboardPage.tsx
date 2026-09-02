import { useQuery } from '@tanstack/react-query';
import { ShieldCheck, UsersRound } from 'lucide-react';
import { apiRequest } from '../../lib/api';
import styles from './DashboardPage.module.css';

type SummaryResponse = { data: { users: number; roles: number } };

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
      <article><ShieldCheck aria-hidden="true" /><div><strong>{summary.data?.roles ?? '-'}</strong><span>Role tersedia</span></div></article>
    </div>}
    <section className={styles.readiness}><div><h2>Fondasi administrasi</h2><p>Profil, akses pengguna, pengaturan, dan audit telah berada dalam satu sistem.</p></div><span>Siap dikembangkan</span></section>
  </div>;
}
